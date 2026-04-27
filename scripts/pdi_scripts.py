import sys
import os
import subprocess
from pathlib import Path
from dataclasses import dataclass
from typing import Union, Annotated
import tyro

PROJECT_ROOT = Path(__file__).resolve().parent.parent

def get_pdi_env() -> dict:
    pdi_src = str(PROJECT_ROOT / "pdi" / "src")
    pdi_scripts_dir = str(PROJECT_ROOT / "pdi" / "scripts")
    env = os.environ.copy()
    env["PYTHONPATH"] = f"{pdi_src}{os.pathsep}{pdi_scripts_dir}{os.pathsep}{env.get('PYTHONPATH', '')}"
    return env

@dataclass
class TrainSubcommand:
    cfg_file: tyro.conf.Positional[str]
    
    def run(self):
        print("--- [TRAIN] Starting ---")
        
        # Paths Setup
        train_script = str(PROJECT_ROOT / "pdi" / "scripts" / "train_all_particles.py")
        env = get_pdi_env()
        
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
        plots_script = str(PROJECT_ROOT / "pdi" / "scripts" / "generate_plots.py")
        env = get_pdi_env()

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