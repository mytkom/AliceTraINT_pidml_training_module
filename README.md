# AliceTraINT PIDML Training Module
This repository is part of AliceTraINT project, its web interface code is [here](https://github.com/mytkom/AliceTraINT).
AliceTraINT PIDML training module is software for AliceTraINT's training machine, which trains Neural Networks for Particle Identification using Machine Learning in CERN ALICE experiment and sync results with central web interface.

## Setting up a training machine
To register a machine and run the module on it with Docker, follow [Setting up a training machine](https://alicetraint.app.cern.ch/docs/setting-up-a-training-machine) in the AliceTraINT docs. How the module works is described in [Training module](https://alicetraint.app.cern.ch/docs/training-module) and [Machine API](https://alicetraint.app.cern.ch/docs/machine-api). The rest of this README is about developing the module.

## Cloning repository
The training code (`pdi`) is a Git submodule with an SSH address, so cloning needs a GitHub SSH key and the submodule step:
```bash
git clone --recurse-submodules <this repository url>
```

## Configuration
Copy `.env.example` to `.env` and set `MACHINE_ID`, `MACHINE_SECRET_KEY` and `ALICETRAINT_BASE_URL`. Every variable in `.env.example` must be set; the module stops at startup when one is missing.

### Dataset cache
Produced (subsampled) ROOT datasets are expensive to rebuild. Set `ALICETRAINT_DATASET_CACHE_DIR_PATH` to a durable directory that is **not** wiped between tasks. Cache key is SHA256 of:
- sorted `AODFiles[].Path` (one per line)
- `is_o_ne`, `is_data`, `subsample_event_count`

Files stored as `{checksum}.root` (+ `{checksum}.meta.json`).

### Training task payload (webapp → module)
Beyond `ID`, `AODFiles`, `Configuration`, webapp must send:
- `IsONe` (bool) — OO/Ne-Ne O2Physics pipeline variant
- `IsData` (bool) — experimental data vs MC
- `SubsampleEventCount` (uint) — target events for `subsample.sh`

Module patches `train.json` `sim_dataset_paths` or `exp_dataset_paths` to the cached ROOT path.

## Building
The Docker image (`Dockerfile`) builds O2Physics, the Python environment and the module, which takes a long time; see the docs for the full `docker build` and `docker run` commands. For local development without Docker you need an O2Physics environment for the scripts.

### Local subsample binary
Dataset pipeline needs `scripts/subsample` (ROOT C++ helper). With O2Physics env:
```bash
alienv setenv O2Physics/latest -c make subsample
```

## Internals
Golang code is stored in `internal` subdir and its commands' main are stored in `cmd` subdirs. You can locally use GNU Make to run and build project (`make run`, `make mock` and `make build`). PDI submodule is in `pdi` subdir. All scripts which are run during training task execution are stored in `scripts` subdir.

### Used scripts
1. `download-multiple-grid-data.sh` (needs `download-from-grid.sh`, `utilities.sh`, `config.sh`) — retrying GRID AO2D download for a remote path list.
2. `run-pidml-mc-producer.sh` (needs `O2configs/{sim,data}-config.json` and **O2Physics**) — PIDML producer pipeline; supports OO/Ne-Ne (`is_o_ne`) and data vs MC (`is_data`).
3. `subsample.sh` (needs `scripts/subsample` binary) — subsample batch producer ROOT outputs to a target event count.
4. `pdi_scripts.py` (needs venv with all requirements of pdi repository) — modern wrapper around the PDI v2 pipeline. It exposes 2 subcommands: `train` - sets up paths and trains neural networks for all particles using the provided JSON config via `train_all_particles.py`, and `plots` - generates SHAP values and performance graphs necessary to evaluate trained models via `generate_plots.py`.

Orchestrator (`DatasetRunner`) splits `AODFiles` into batches of max 10, download+produce each batch, then subsample into the cache.

**PDI tree-name note:** new producer writes `O2pidtracksmc` / `O2pidtracksdata` (tables `PIDTRACKSMC` / `PIDTRACKSDATA`). Older module path used `PIDTRACKSMCML` / `O2pidtracksmcml`. If PDI still expects the ML table name, update PDI loaders separately.

### Client code
All functions for communication with **AliceTraINT** web interface are stored in `client` go submodule with required structs.

### Command pattern
Golang code uses command pattern. All commands implements `Command` interface (everything in `scripts` go module). List of `Command`s is evaluated in every training task stage (`cmd/AlicaTraINT_pidml_training_module/main.go`).

### Mock Testing
There is a mock command provided (`cmd/mock/main.go` and `make mock`), which can be useful when testing communication between web interface and training module without any script execution of training task.

Additionally, a local `mock_server.py` is provided to fully simulate the central AliceTraINT web interface. This allows you to test the complete orchestrator pipeline (from downloading tasks to uploading models) locally without needing a deployed web backend.
