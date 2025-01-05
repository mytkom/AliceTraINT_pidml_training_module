# AliceTraINT PIDML Training Module
This repository is part of AliceTraINT project, its web interface code is [here](https://github.com/mytkom/AliceTraINT).
AliceTraINT PIDML training module is software for AliceTraINT's training machine, which trains Neural Networks for Particle Identification using Machine Learning in CERN ALICE experiment and sync results with central web interface.

## Cloning repository
Training module to work needs `pdi` repository (PIDML python code), which is added as git submodule under `pdi` subdir to this repository. It means that cloning needs additional step: 
```bash
git clone <this repository url>
git submodule update --init --recursive 
```

## Getting started
You need to configure your machine using `.env` file. First copy defaults:
```bash
cp .env.example .env
```
Three variables must be configured: `MACHINE_ID`, `MACHINE_SECRET_KEY` (both obtainable from web interface) and `ALICETRAINT_BASE_URL` (url of web interface used to obtain machine id and secret key).

To obtain `MACHINE_ID` and `MACHINE_SECRET_KEY` from **AliceTraINT** web interface you need to enter "Training Machines", click "Register Training Machine", set name and submit, copy id and secret key.

Then you should update `.env` file with obtained values.

Training module always requests from web interface (never the other way), because of that queued training tasks are requested periodically (HTTP Pooling). Wait time between requests can be adjusted using `ALICETRAINT_POOLING_WAIT_SECONDS` enviroment variable.

## Running project
Preffered way of interacting with project is building docker image using provided Dockerfile and executing container with enviroment variables overwriting:
### Docker
Take into account that part of **O2Physics** is being build in this docker image, so it can take long time to finish and take great amount of disk space. To build docker image you need to save your GRID certificate in root dir with name `gridCertificate.p12`, it is needed for downloading training data from GRID. Make sure that enviroment variables are configured, it can be done by `.env` file or overwriting variables in environment.
Then you can build your image, assuming that you are in root dir:
```bash
docker build -t alicetraint/training-module .
```
After building you can run a container using this image and adjust configuration using enviroment variables passed to `docker run` command.

## Internals
Golang code is stored in `internal` subdir and its commands' main are stored in `cmd` subdirs. You can locally use GNU Make to run and build project (`make run`, `make mock` and `make build`). PDI submodule is in `pdi` subdir. All scripts which are run during training task execution are stored in `scripts` subdir.

### Used scripts
1. `download-multiple-grid-data.sh` (which needs `download-from-grid.sh` and `utilities.sh`) - script used to efficiently download multiple training data files (AODs) from GRID,
2. `run-pidml-producer.sh` (which needs `ml-mc-config.json` and **O2Physics** intallation) - script running all necessary `O2Physics` tasks pipeline with PIDML producer. It is configured in `ml-mc-config.json` file.
3. `pdi_scripts.py` (which needs venv with all requirements of pdi repository and `uproot3`) - contains 4 scripts, which uses `pdi` code. These are: `process` - processed .root file into .csv file and prepares data for training, `data-exploration` - generates statistical graphs of prepared data, `train` - trains neural network with provided config (default config is in `scripts/train_default_cfg.json`), `benchmark` - generates graphs necessary to evaluate trained neural networks.
 
### Client code
All functions for communication with **AliceTraINT** web interface are stored in `client` go submodule with required structs.

### Command pattern
Golang code uses command pattern. All commands implements `Command` interface (everything in `scripts` go module). List of `Command`s is evaluated in every training task stage (`cmd/AlicaTraINT_pidml_training_module/main.go`).

### Mock command
There is also mock command provided (`cmd/mock/main.go` and `make mock`), which can be useful when testing communication between web interface and training module without any script execution of training task.