#!/usr/bin/env python

import argparse
import sys
import os
import json
from pathlib import Path
from matplotlib import pyplot as plt
import uproot3
import numpy as np
import torch
import torch.nn as nn
import pandas as pd
import onnx
import wandb


def get_env_path(var_name, default):
    path = os.getenv(var_name, default)
    if not os.path.exists(path):
        raise ValueError(f"Environment variable {var_name} points to a non-existent path: {path}")
    return path

def do_train(data_dir, results_dir, device, config_common, data_preparation, config, model_class, model_args):
    pt_models_dir = os.path.join(data_dir, "models")
    os.makedirs(pt_models_dir, exist_ok=True)

    onnx_models_dir = os.path.join(results_dir, "models")
    os.makedirs(onnx_models_dir, exist_ok=True)

    wandb_config = {**config_common, **config}

    train_loader, val_loader = data_preparation.prepare_dataloaders(
        wandb_config["bs"], NUM_WORKERS, [Split.TRAIN, Split.VAL])

    thresholds_df_list = []

    for target_code in TARGET_CODES:
        pt_path = os.path.join(pt_models_dir, f"{PARTICLES_DICT[target_code]}.pt")
        onnx_path = os.path.join(onnx_models_dir, f"{PARTICLES_DICT[target_code]}.onnx")
        with wandb.init(project="pdi",
                        config=wandb_config,
                        name=PARTICLES_DICT[target_code],
                        anonymous="allow") as run:
            pos_weight = torch.tensor(1.0).to(device)
            wandb.log({"pos_weight": pos_weight.item()})

            model_init_args = model_args(data_preparation)
            model = model_class(*model_init_args).to(device)

            train(model, target_code, device, train_loader, val_loader,
                  pos_weight)

            save_dict = {
                "state_dict": model.state_dict(),
                "model_args": model_init_args,
                "model_thres": model.thres
            }

            thresholds_df_list.append(pd.DataFrame([(target_code, model.thres)], columns=["pdgPid", "threshold"]))

            # save .pt file to data_dir
            torch.save(save_dict, pt_path)

            # device cpu for export
            device = torch.device("cpu")

            # load and prepare previously saved .pt model
            saved_model = torch.load(pt_path)
            model = AttentionModel(*saved_model["model_args"]).to(device)
            model.thres = saved_model["model_thres"]
            model.load_state_dict(saved_model["state_dict"])
            model_with_sigmoid = nn.Sequential(model, nn.Sigmoid())

            # prepare dummy input
            data_preparation = FeatureSetPreparation()
            (train_loader, ) = data_preparation.prepare_dataloaders(1, 0, [Split.TRAIN])
            input_data, _, _ = next(iter(train_loader))
            dummy_input = input_data.to(device)
            print("Dummy input shape: ", dummy_input.shape)

            # export to ONNX
            input_name = 'input'
            output_name = 'output'
            torch.onnx.export(model_with_sigmoid, dummy_input, onnx_path, 
                              export_params=True,
                              opset_version=14,
                              do_constant_folding=True,
                              input_names=[input_name],
                              output_names=[output_name],
                              dynamic_axes={input_name: {0: 'batch size'}})

            # verify exported ONNX
            onnx_model = onnx.load(onnx_path)
            onnx.checker.check_model(onnx_model)

    thresholds_df = pd.concat(thresholds_df_list, ignore_index=True)
    thresholds_df.to_csv(os.path.join(pt_models_dir, "thresholds.csv"), index=False)

def train_main(cfg_file: str):
    results_dir = get_env_path("RESULTS_DIR", "results")
    data_dir = get_env_path("DATA_DIR", "data")

    print("CWD: ", os.getcwd())
    with open(cfg_file) as f:
        cfg = json.load(f)

    torch.multiprocessing.set_sharing_strategy('file_system')
    device = torch.device('cuda:0' if torch.cuda.is_available() else 'cpu')    

    proposed_config = {
        "data_preparation": FeatureSetPreparation(undersample=cfg["undersample"]),
        "config": {
            "embed_in": N_COLUMNS + 1,
            "embed_hidden": cfg["embed_hidden"],
            "d_model": cfg["d_model"],
            "ff_hidden": cfg["ff_hidden"],
            "pool_hidden": cfg["pool_hidden"],
            "num_heads": cfg["num_heads"],
            "num_blocks": cfg["num_blocks"],
            "start_lr": cfg["start_lr"],
        },
        "model_class": AttentionModel,
        "model_args": lambda d_prep: [
            wandb.config.embed_in,
            wandb.config.embed_hidden,
            wandb.config.d_model,
            wandb.config.ff_hidden,
            wandb.config.pool_hidden,
            wandb.config.num_heads,
            wandb.config.num_blocks,
            nn.ReLU,
            wandb.config.dropout,
        ],
    }

    print("Starting training of Neural Networks:")
    do_train(data_dir, results_dir, device, cfg, **proposed_config)

def process_main(input_file, cfg_file):
    data_dir = get_env_path("DATA_DIR", "data")
    print("CWD: ", os.getcwd())
    with open(cfg_file) as f:
        cfg = json.load(f)

    # ROOT -> CSV
    print("Converting preprocessed ROOT file to CSV file")
    dataframes = []
    file = uproot3.open(input_file)
    for dirname in file:
        dirname = dirname.decode("utf-8")
        pure_dirname = dirname.split(";")[0]
        if pure_dirname.startswith("DF_"):
            tree_data = file["%s/O2pidtracksmcml" % (dirname)].pandas.df()
            dataframes.append(tree_data)

    data = pd.concat(dataframes, ignore_index=True)
    print(data.head())
    print(data.columns)

    # TRDPattern is uint8, so cannot use NaN in producer -> need to preprocess it here
    data["fTRDPattern"].mask(np.isclose(data["fTRDPattern"], 0), inplace=True)
    data = data[data["fTPCSignal"] > 0]
    csv_filepath = os.path.join(data_dir, f"{Path(input_file).stem}.csv")
    print("Saving CSV file to: ", csv_filepath)
    data.to_csv(csv_filepath)

    print("Preparing data for training")
    # Data preparation
    prep = FeatureSetPreparation(undersample=cfg["undersample"])
    prep.prepare_data(csv_filepath)
    prep.save_data(os.path.join(data_dir, f"processed/feature_set/run{RUN}"))

def data_exploration_main():
    results_dir = get_env_path("RESULTS_DIR", "results")
    save_dir = os.path.join(results_dir, "data-exploration")
    
    # general statistics
    splits = [Split.TRAIN]
    prep = FeatureSetPreparation()
    prep._load_preprocessed_data(splits)
    ungrouped_data = prep.data_to_ungrouped_df(splits)
    print(ungrouped_data.shape)
    classes = ungrouped_data[TARGET_COLUMN].value_counts()
    print(classes)
    num_chosen = classes[TARGET_CODES].sum()
    print(num_chosen / ungrouped_data.shape[0])
    nulls = ungrouped_data.isnull().sum()
    print(nulls)
    all_nulls = ungrouped_data.isnull().any(axis=1).sum()
    print(all_nulls)
    print(all_nulls/ungrouped_data.shape[0])

    # plot missing detectors distribution
    null_rows = ungrouped_data.isnull().value_counts()
    columns = ungrouped_data.columns
    missing_values = [columns[list(index)] for index in null_rows.index]
    missing_detectors = []
    for mv in missing_values:
        dets = columns_to_detectors(mv)
        dets = [d.name for d in dets]
        missing_detectors.append(dets)
    print(missing_detectors, null_rows.values)
    plt.pie(null_rows)
    labels = ["Missing detectors: " + ", ".join(v) for i, v in enumerate(missing_detectors)]
    print(labels)
    plt.legend(
        [l + f": {100*null_rows[i]/sum(null_rows):.3f}%" for i, l in enumerate(labels)]
        , loc="lower right", bbox_to_anchor=(2.2, -0.5), prop={'size': 20}
    )
    os.makedirs(save_dir, exist_ok=True)
    plt.savefig(os.path.join(save_dir, "missing_dets.png"), bbox_inches = "tight")
    plt.clf()

    # plot class distribution
    particles = [classes[i] for i in classes.index if i in PARTICLES_DICT]
    labels_percent = [
        PARTICLES_DICT[i] + f": {100*classes[i]/sum(classes):.3f}%" for i in classes.index if i in PARTICLES_DICT
    ]
    plt.pie(particles)
    plt.legend(
        labels_percent, loc="lower right", bbox_to_anchor=(2.2, -0.5), prop={'size': 20}
    )
    plt.savefig(os.path.join(save_dir, "particles.png"), bbox_inches = "tight")
    plt.clf()

    # plot particle distribution to pt
    dir_vs_pt_dir = os.path.join(save_dir, "distribution_vs_pt")
    os.makedirs(dir_vs_pt_dir, exist_ok=True)
    for target_code in TARGET_CODES:
        plot_particle_distribution(target_code, prep, splits, "fPt", f"{PARTICLES_DICT[target_code]}", dir_vs_pt_dir)
    
    # plot correlation matrices
    cor_save_dir = os.path.join(save_dir, "correlation_matrices")
    os.makedirs(cor_save_dir, exist_ok=True)
    plot_cor_matrix(ungrouped_data, "all_particles", cor_save_dir)
    for target_code in TARGET_CODES:
        one_particle = ungrouped_data.loc[ungrouped_data[TARGET_COLUMN] == target_code]
        title = PARTICLES_DICT[target_code]
        plot_cor_matrix(one_particle, title, cor_save_dir)

def feature_importance(device, data_dir, results_dir):
    split = Split.TEST
    prep = FeatureSetPreparation()
    prep._try_load_preprocessed_data([split])
    groups = prep.data_to_df_dict(split)
    model_load_dir = os.path.join(data_dir, f"models")
    model_class = AttentionModel

    # wrapper for model, explainers don't allow passing tensors
    def predict(input_data):
        new_in = torch.tensor(input_data).to(device)
        return model(new_in).cpu().detach().numpy()

    batch_size = 16 # for bigger number of entries kernel crashes, so here data is split into batches
    batches = 50
    hide_progress_bars = False

    cols = prep.load_columns()

    particles_to_explain = TARGET_CODES

    if not particles_to_explain:
        particles_to_explain = TARGET_CODES
    else:
        particles_to_explain = [p for p in particles_to_explain if p in TARGET_CODES]

    for target_code in particles_to_explain:
        print(PARTICLES_DICT[target_code])
        model_name = f"{PARTICLES_DICT[target_code]}.pt"
        load_path = os.path.join(model_load_dir, model_name)
        saved_model = torch.load(load_path, map_location=torch.device("cpu"))
        model = model_class(*saved_model["model_args"]).to(device)
        model.load_state_dict(saved_model["state_dict"])

        for key, group in groups.items():
            detectors = detector_unmask(key)
            detectors = [d.name for d in detectors]
            label = "_".join(detectors)
            print(label)

            result, data_count = explain_model(predict, group, batch_size, batches, hide_progress_bars)
            result.feature_names = cols

            save_dir = f"{results_dir}/feature_importance/{PARTICLES_DICT[target_code]}"

            file_name = f"{label}"
            title = f"{PARTICLES_DICT[target_code]}, entries: {data_count}"
            plot_and_save_beeswarm(result, save_dir, file_name, title)
            plt.clf()

def comparison_plots(device, data_dir, results_dir):
    benchmark_dir = os.path.join(results_dir, "benchmark")
    particle_names = [PARTICLES_DICT[i] for i in TARGET_CODES]
    metrics = ["precision", "recall", "f1"]
    data_types = ["all", "complete_only"]
    experiment_name = "Proposed"
    exp_dict = {
        "model_class": AttentionModel,
        "data": {
            "all": FeatureSetPreparation,
             "complete_only": lambda: FeatureSetPreparation(complete_only=True),
        }
    }
    model_names = [experiment_name]
        
    metric_results = pd.DataFrame(
        index=pd.MultiIndex.from_product(
            [particle_names, model_names], names=["particle", "model"]
            ),
        columns=pd.MultiIndex.from_product(
            [data_types, metrics], names=["data", "metric"]
            ),
        )
    
    target_codes = TARGET_CODES
    prediction_data = {}
    for target_code in target_codes:
        print(f"Target: {target_code}")
        particle_name = PARTICLES_DICT[target_code]
        prediction_data[target_code] = {}
        print(f"Experiment: {experiment_name}")
        if exp_dict["model_class"] != Traditional:
            load_path = f"{data_dir}/models/{particle_name}.pt"
            saved_model = torch.load(load_path, map_location=torch.device("cpu"))
            model = exp_dict["model_class"](*saved_model["model_args"]).to(device)
            model.thres = saved_model["model_thres"]
            model.load_state_dict(saved_model["state_dict"])
    
        batch_size = 512
    
        prediction_data[target_code][experiment_name] = {}
        for data_type, data_prep in exp_dict["data"].items():
            print(f"Data type: {data_type}")
            test_loader, = data_prep().prepare_dataloaders(batch_size, NUM_WORKERS, [Split.TEST])
            
            if exp_dict["model_class"] != Traditional:
                model_thres = model.thres
                print(model_thres)
                predictions, targets, add_data, _ = get_predictions_data_and_loss(model, test_loader, device)
                selected = predictions > model.thres
            else:
                model_thres = 3.0
                predictions, targets, add_data = get_nsigma_predictions_data(test_loader, target_code)
                is_sign_correct = add_data["fSign"] == np.sign(target_code)
                selected = predictions < model_thres
                selected = np.where(is_sign_correct, selected, 0)
    
            binary_targets = targets == target_code
    
            true_positives = int(np.sum(selected & binary_targets))
            print("TP: ", true_positives)
            selected_positives = int(np.sum(selected))
            print("SP: ", selected_positives)
            positives = int(np.sum(binary_targets))
            print("P: ", positives)
    
            precision, recall, _, _ = calculate_precision_recall(true_positives, selected_positives, positives)
            f1 = 2 * precision * recall / (precision + recall + np.finfo(float).eps)
    
            metric_results.loc[(particle_name, experiment_name), data_type] = precision, recall, f1
            
            prediction_data[target_code][experiment_name][data_type] = {
                "targets": binary_targets,
                "predictions": predictions,
                "momentum": add_data[Additional.fPt.name],
                "threshold": model_thres,
                "selected": selected
            }

    metric_results_path = os.path.join(results_dir, "comparison_metrics.csv")
    metric_results.to_csv(metric_results_path)
    
    p_min, p_max = P_RANGE
    p_range = np.linspace(p_min, p_max, P_RESOLUTION)
    intervals = list(zip(p_range[:-1], p_range[1:]))
    
    for target_code in target_codes:
        particle_name = PARTICLES_DICT[target_code]
        print(f"Plotting {particle_name} for code {target_code}")
        tc_prediction_data = prediction_data[target_code]
        for data_type in data_types:
            print(f"Data type {data_type}")
            data = {}
            for exp_name, exp_dict in tc_prediction_data.items():
                if data_type in exp_dict:
                    data[exp_name] = exp_dict[data_type]
            
            save_dir = os.path.join(benchmark_dir, f"model_comparison/run{RUN}/{data_type}/{particle_name}")
            os.makedirs(save_dir, exist_ok=True)
            plot_purity_comparison(particle_name, data, intervals, save_dir)
            plot_efficiency_comparison(particle_name, data, intervals, save_dir)
            plot_precision_recall_comparison(particle_name, data, save_dir)

def benchmark_main():
    data_dir = get_env_path("DATA_DIR", "data")
    results_dir = get_env_path("RESULTS_DIR", "results")

    torch.multiprocessing.set_sharing_strategy('file_system')
    device = torch.device('cuda:0' if torch.cuda.is_available() else 'cpu')    
    
    comparison_plots(device, data_dir, results_dir)
    feature_importance(device, data_dir, results_dir)

def main():
    os.environ['WANDB_MODE'] = "disabled"

    parser = argparse.ArgumentParser(description="PDI Utilities")
    subparsers = parser.add_subparsers(dest="command", required=True, help="Subcommands")

    # Train subcommand
    train_parser = subparsers.add_parser("train", help="Train models")
    train_parser.add_argument('cfg_file', type=str, help="Configuration file")

    # Process subcommand
    process_parser = subparsers.add_parser("process", help="Process ROOT file")
    process_parser.add_argument('input_file', type=str, help="ROOT file to process")
    process_parser.add_argument('cfg_file', type=str, help="Configuration file")

    # Data exploration subcommand
    data_exploration_parser = subparsers.add_parser("data-exploration", help="Data exploration")

    # Benchmark subcommand
    benchmark_parser = subparsers.add_parser("benchmark", help="Benchmark trained models")

    args = parser.parse_args()

    if args.command == "train":
        train_main(args.cfg_file)
    elif args.command == "process":
        process_main(args.input_file, args.cfg_file)
    elif args.command == "data-exploration":
        data_exploration_main()
    elif args.command == "benchmark":
        benchmark_main()
        

if __name__ == "__main__":
    pdi_dir = get_env_path("PDI_DIR", "pdi")
    print("PDI DIR:", pdi_dir)
    if pdi_dir not in sys.path:
        sys.path.append(pdi_dir)

    from pdi.data.preparation import FeatureSetPreparation
    from pdi.data.detector_helpers import columns_to_detectors, detector_unmask
    from pdi.data.data_exploration import plot_particle_distribution, plot_cor_matrix, explain_model, plot_and_save_beeswarm
    from pdi.models import AttentionModel, Traditional
    from pdi.data.constants import N_COLUMNS, TARGET_COLUMN
    from pdi.train import train
    from pdi.constants import (
            PARTICLES_DICT,
            TARGET_CODES,
            NUM_WORKERS,
            P_RANGE,
            P_RESOLUTION
        )
    from pdi.data.types import Split, Additional
    from pdi.data.config import RUN
    from pdi.evaluate import get_predictions_data_and_loss, get_nsigma_predictions_data, calculate_precision_recall
    from pdi.visualise import plot_purity_comparison, plot_efficiency_comparison, plot_precision_recall_comparison

    main()
