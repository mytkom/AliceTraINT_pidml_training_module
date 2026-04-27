import sys
import os
import json
import subprocess
from pathlib import Path
from dataclasses import dataclass
from typing import Union, Annotated
import tyro

# Project structure
PROJECT_ROOT = Path(__file__).resolve().parent.parent

@dataclass
class TrainSubcommand:
    cfg_file: tyro.conf.Positional[str]
    
    def run(self):
        print("--- [TRAIN] Starting (via PDI train_all_particles) ---")
        
        # 1. Paths Setup
        pdi_dir = str(PROJECT_ROOT / "pdi")
        pdi_src = str(PROJECT_ROOT / "pdi" / "src")
        pdi_scripts_dir = str(PROJECT_ROOT / "pdi" / "scripts")
        train_script = str(Path(pdi_dir) / "scripts" / "train_all_particles.py")
        
        # We run from PROJECT_ROOT so that relative paths in JSON (like "data/...") work
        env = os.environ.copy()
        env["PYTHONPATH"] = f"{pdi_src}{os.pathsep}{pdi_scripts_dir}{os.pathsep}{env.get('PYTHONPATH', '')}"
        
        # 2. Execution (Direct use of your config file)
        cmd = [
            sys.executable,
            train_script,
            "--all", str(Path(self.cfg_file).resolve())
        ]
        
        print(f"Executing PDI training (direct mode)...")
        subprocess.run(cmd, check=True, env=env, cwd=PROJECT_ROOT)


@dataclass
class PlotsSubcommand:
    shap_batches: int = 1
    shap_batch_size: int = 2

    def run(self):
        print("--- [PLOTS] Starting (via PDI generate_plots) ---")
        pdi_dir = str(PROJECT_ROOT / "pdi")
        pdi_src = str(PROJECT_ROOT / "pdi" / "src")
        pdi_scripts_dir = str(PROJECT_ROOT / "pdi" / "scripts")
        plots_script = str(Path(pdi_dir) / "scripts" / "generate_plots.py")

        env = os.environ.copy()
        env["PYTHONPATH"] = f"{pdi_src}{os.pathsep}{pdi_scripts_dir}{os.pathsep}{env.get('PYTHONPATH', '')}"

        # Get results_dir from JSON config (manually for benchmark) or default
        results_dir = str(PROJECT_ROOT / "results")

        for particle_dir in Path(results_dir).iterdir():
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