#!/usr/bin/env python

import argparse
import sys
import os
import json
from pathlib import Path
import wandb
import pandas as pd
import matplotlib.pyplot as plt
import matplotlib
import numpy as np
import torch

# Setup for server/headless environments
matplotlib.use('Agg')
os.environ["WANDB_MODE"] = "disabled"

# --- PATH RESOLUTION ---
SCRIPTS_DIR = Path(os.path.abspath(__file__)).parent
PROJECT_ROOT = SCRIPTS_DIR.parent

pdi_dir = os.getenv("PDI_DIR")
if not pdi_dir:
    pdi_dir = str(PROJECT_ROOT / "pdi" / "src")

if pdi_dir not in sys.path:
    sys.path.append(pdi_dir)

# --- PDI IMPORTS ---
try:
    from pdi.config import Config, AttentionConfig
    from pdi.data.data_preparation import DataPreparation
    from pdi.engines import build_engine
    from pdi.constants import PART_NAME_TO_TARGET_CODE, TARGET_CODES, TARGET_CODE_TO_PART_NAME
    from pdi.data.types import Split, InputTarget
    from pdi.data.data_exploration import plot_cor_matrix, plot_group_ratio, explain_model, plot_and_save_beeswarm
    from pdi.visualise import plot_precision_recall_comparison, plot_metrics_vs_pt_comparison
    from pdi.models import build_model
except ImportError as e:
    print(f"ERROR: Cannot import PDI from {pdi_dir}: {e}")
    sys.exit(1)

def map_config(old_cfg, input_file_path):
    """Maps server JSON configuration to PDI Config object."""
    config = Config()
    config.data.is_run_3 = True
    config.data.subset_size = old_cfg.get("subset_size")
    config.sim_dataset_paths = [str(Path(input_file_path).absolute())]
    
    config.model.architecture = "attention"
    config.model.attention = AttentionConfig(
        embed_hidden_layers=[old_cfg.get("embed_hidden", 128)],
        embed_dim=old_cfg.get("d_model", 32),
        encoder_ff_hidden=old_cfg.get("ff_hidden", 128),
        mlp_hidden_layers=[64, 32, 16],
        pool_hidden_layers=[old_cfg.get("pool_hidden", 64)],
        num_heads=old_cfg.get("num_heads", 2),
        num_blocks=old_cfg.get("num_blocks", 2),
        dropout=old_cfg.get("dropout", 0.1)
    )
    
    config.training.batch_size = old_cfg.get("bs", 512)
    config.training.max_epochs = old_cfg.get("max_epochs", 40)
    config.training.start_lr = old_cfg.get("start_lr", 0.001)
    config.training.device = "cuda" if old_cfg.get("use_gpu") else "cpu"
    config.training.undersample_missing_detectors = old_cfg.get("undersample", False)
    
    config.training.num_workers = 0
    config.validation.num_workers = 0
    return config

def get_root_file():
    """Finds the main data file."""
    data_dir = PROJECT_ROOT / "data"
    pref = data_dir / "preprocessed_ao2ds.root"
    if pref.exists(): return str(pref.absolute())
    found = list(data_dir.glob("*.root"))
    if not found: sys.exit(1)
    return str(found[0].absolute())

# --- COMMANDS ---

def process_main(input_file_arg, cfg_file_arg):
    print(f"--- [PROCESS] Starting ---")
    with open(cfg_file_arg, 'rb') as f: old_cfg = json.load(f)
    config = map_config(old_cfg, input_file_arg)
    prep = DataPreparation(config.data, config.sim_dataset_paths, seed=42)
    prep.prepare_data()

def data_exploration_main():
    print(f"--- [DATA-EXPLORATION] Generating plots ---")
    results_root = PROJECT_ROOT / "results"
    results_root.mkdir(parents=True, exist_ok=True)
    
    prep = DataPreparation(Config().data, [get_root_file()], seed=42)
    data = prep.get_prepared_data([Split.TRAIN])[Split.TRAIN]
    
    # Merge groups
    df_all = pd.concat([v[InputTarget.INPUT].assign(fPdgCode=v[InputTarget.TARGET]) for v in data.values()])
    
    # 1. Particles Ratio (Original Name)
    plot_group_ratio([TARGET_CODE_TO_PART_NAME[c] for c in TARGET_CODES], 
                     [df_all["fPdgCode"] == c for c in TARGET_CODES]).savefig(results_root / "particles.png")
    
    # 2. Missing Detectors Ratio (Original Name)
    det_cols = ["fTPCSignal", "fTOFSignal", "fTRDPattern"]
    det_labels = ["TPC", "TOF", "TRD"]
    det_conds = [df_all[c] > 0 for c in det_cols]
    plot_group_ratio(det_labels, det_conds, title="Detector Coverage").savefig(results_root / "missing_dets.png")
    
    # 3. Correlation Matrices
    plot_cor_matrix(df_all, "All Particles").savefig(results_root / "all_particles_correlation.png")
    for code in TARGET_CODES:
        part_name = TARGET_CODE_TO_PART_NAME[code]
        df_part = df_all[df_all["fPdgCode"] == code]
        if not df_part.empty:
            plot_cor_matrix(df_part, part_name).savefig(results_root / f"{part_name}_correlation.png")

def train_main(cfg_file_arg):
    print(f"--- [TRAIN] Starting ---")
    with open(cfg_file_arg, 'rb') as f: old_cfg = json.load(f)
    config = map_config(old_cfg, get_root_file())
    thresholds_data = []
    
    for part_name, target_code in PART_NAME_TO_TARGET_CODE.items():
        if target_code not in TARGET_CODES: continue
        print(f"\n>> Training {part_name}...")
        wandb.init(project="cern", name=part_name, mode="disabled")
        engine = build_engine(config, target_code)
        engine.train()
        
        # Collect threshold for thresholds.csv
        meta_path = Path(engine._base_dir) / "model_weights" / "metadata.json"
        if meta_path.exists():
            with open(meta_path, 'r') as f:
                meta = json.load(f)
                thresholds_data.append({"pdgPid": target_code, "threshold": meta["threshold"]})
        wandb.finish()
        
    if thresholds_data:
        pd.DataFrame(thresholds_data).to_csv(PROJECT_ROOT / "results" / "thresholds.csv", index=False)

def benchmark_main():
    print(f"--- [BENCHMARK] Starting ---")
    results_root = PROJECT_ROOT / "results"
    any_config = next(results_root.rglob("config.json"), None)
    if not any_config: return
    with open(any_config, 'rb') as f: config = Config.from_dict(json.load(f))
    config.training.device = "cpu"
    prep = DataPreparation(config.data, config.sim_dataset_paths, seed=42)
    
    all_metrics = []
    for part_name, target_code in PART_NAME_TO_TARGET_CODE.items():
        if target_code not in TARGET_CODES: continue
        part_dir = next(results_root.rglob(part_name), None)
        if not part_dir: continue
        runs = sorted(list(part_dir.glob("run_*")))
        if not runs: continue
        run_dir = runs[-1]
        
        engine = build_engine(config, target_code, base_dir=str(run_dir))
        test_res = engine.test()
        
        # Robust data sync
        df = engine._test_dl.unwrap()
        min_l = min(len(df), len(test_res.predictions))
        targets, pt, preds = df["fPdgCode"][:min_l], df["fPt"][:min_l], test_res.predictions[:min_l]
        test_res.targets = (targets == target_code).astype(int)
        test_res.predictions = preds.squeeze()

        # 1. PR Curve (Original style name)
        plot_precision_recall_comparison({"Model": test_res}, mask=np.ones(len(targets), dtype=bool)).savefig(results_root / f"{part_name}_precision_recall.png")
        
        # 2. Efficiency / Purity vs Pt (Original style names)
        for fig, name in plot_metrics_vs_pt_comparison({"Model": test_res}, pt.to_numpy()):
            clean_name = "p_purity_optimized_threshold" if "purity" in name else "p_efficiency_optimized_threshold"
            fig.savefig(results_root / f"{part_name}_{clean_name}.png")

        # 3. Population vs Pt (distribution_vs_pt replacement)
        pt_bins = [0, 1, 2, 3, 5]
        pt_labels = [f"{pt_bins[i]}-{pt_bins[i+1]} GeV/c" for i in range(len(pt_bins)-1)]
        pt_conds = [(pt >= pt_bins[i]) & (pt < pt_bins[i+1]) for i in range(len(pt_bins)-1)]
        plot_group_ratio(pt_labels, pt_conds, title=f"Pt Distribution for {part_name}").savefig(results_root / f"{part_name}_distribution_vs_pt.png")

        # 4. SHAP (Original style name)
        model = build_model(config.model, group_ids=prep.get_group_ids())
        model.load_state_dict(torch.load(run_dir / "model_weights" / "best.pt", map_location="cpu"))
        model.eval()
        def pred_f(x):
            if x.ndim == 1: x = x.reshape(1, -1)
            with torch.no_grad():
                out = model(torch.tensor(x, dtype=torch.float32)).numpy()
                return out.reshape(-1, 1)

        test_data = prep.get_prepared_data([Split.TEST])[Split.TEST]
        columns = pd.read_json(run_dir / "columns_for_training.json")["columns_for_training"].tolist()
        for gid, d in test_data.items():
            if not d[InputTarget.INPUT].empty:
                sv, _ = explain_model(pred_f, d[InputTarget.INPUT], batch_size=16, batches=5)
                sv.feature_names = columns
                plot_and_save_beeswarm(sv, str(results_root), f"{part_name}_feature_importance_GID_{gid}.png", f"SHAP: {part_name} GID {gid}")

        metrics_dict = test_res.test_metrics.to_dict()
        metrics_dict['particle'] = part_name
        all_metrics.append(metrics_dict)

    if all_metrics:
        pd.DataFrame(all_metrics).to_csv(results_root / "comparison_metrics.csv", index=False)

def main():
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)
    p_parser = subparsers.add_parser("process")
    p_parser.add_argument('input_file'); p_parser.add_argument('cfg_file')
    t_parser = subparsers.add_parser("train")
    t_parser.add_argument('cfg_file')
    subparsers.add_parser("data-exploration")
    subparsers.add_parser("benchmark")
    args = parser.parse_args()
    if args.command == "process": process_main(args.input_file, args.cfg_file)
    elif args.command == "train": train_main(args.cfg_file)
    elif args.command == "data-exploration": data_exploration_main()
    elif args.command == "benchmark": benchmark_main()

if __name__ == "__main__":
    main()
