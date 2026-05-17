import sys
import os
import subprocess
from pathlib import Path
from dataclasses import dataclass
from typing import Union, Annotated
import tyro

PROJECT_ROOT = Path(__file__).resolve().parent.parent

def get_pdi_env() -> dict:
    env = os.environ.copy()
    
    pdi_dir = Path(env.get("PDI_DIR", PROJECT_ROOT / "pdi"))
    pdi_src = env.get("PDI_SRC_DIR", str(pdi_dir / "src"))
    pdi_scripts_dir = env.get("PDI_SCRIPTS_DIR", str(pdi_dir / "scripts"))

    existing_pythonpath = env.get("PYTHONPATH", "")
    pythonpath_entries = [pdi_src, pdi_scripts_dir, existing_pythonpath]
    env["PYTHONPATH"] = os.pathsep.join(
        str(entry) for entry in pythonpath_entries if entry
    )
    
    return env

@dataclass
class TrainSubcommand:
    cfg_file: tyro.conf.Positional[str]
    
    def run(self):
        print("--- [TRAIN] Starting ---")
        
        env = get_pdi_env()
        train_script = env.get("PDI_TRAIN_SCRIPT", str(PROJECT_ROOT / "pdi" / "scripts" / "train_all_particles.py"))
        
        cmd = [
            sys.executable,
            train_script,
            "--all", str(Path(self.cfg_file).resolve())
        ]
        
        print(f"Executing training")
        subprocess.run(cmd, check=True, env=env, cwd=PROJECT_ROOT)


@dataclass
class PlotsSubcommand:
    shap_batches: int = 1
    shap_batch_size: int = 2

    def run(self):
        print("--- [PLOTS] Starting ---")
        env = get_pdi_env()
        
        plots_script = env.get("PDI_PLOTS_SCRIPT", str(PROJECT_ROOT / "pdi" / "scripts" / "generate_plots.py"))
        results_dir = Path(env.get("RESULTS_DIR", str(PROJECT_ROOT / "results")))

        if not results_dir.exists():
            print(f"Results directory not found: {results_dir}")
            return

        for particle_dir in results_dir.iterdir():
            if not particle_dir.is_dir() or particle_dir.name == "project":
                continue
            
            run_dir = particle_dir / "run_1"
            if not run_dir.exists():
                continue
            
            print(f"\n>> Generating plots for {particle_dir.name} (SHAP: {self.shap_batches}x{self.shap_batch_size})...")
            cmd = [
                sys.executable,
                plots_script,
                "--target", particle_dir.name,
                "--model-dir", str(run_dir.absolute()),
                "--shap-batches", str(self.shap_batches),
                "--shap-batch-size", str(self.shap_batch_size)
            ]
            subprocess.run(cmd, check=True, env=env, cwd=PROJECT_ROOT)

if __name__ == "__main__":
    subcommand = tyro.cli(
        Union[
            Annotated[TrainSubcommand, tyro.conf.subcommand(name="train")],
            Annotated[PlotsSubcommand, tyro.conf.subcommand(name="plots")],
        ]
    )
    subcommand.run()