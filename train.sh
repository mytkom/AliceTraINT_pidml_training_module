#!/bin/bash

ALIBUILD_WORK_DIR="/wd/alice/sw"
VIRTUAL_ENV_ACTIVATE="/home/alice/.virtualenvs/bin/activate"

export PIDML_TRAINING_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd $ALIBUILD_WORK_DIR
alienv setenv O2Physics/latest -c $PIDML_TRAINING_DIR/scripts/download-multiple-grid-data.sh
alienv setenv O2Physics/latest -c $PIDML_TRAINING_DIR/scripts/run-pidml-mc-producer.sh

CSV_FILE=preprocessed_ao2ds.csv


# cd $PIDML_TRAINING_DIR
# root to csv
source $VIRTUAL_ENV_ACTIVATE && \
alienv setenv Python/latest -c \
  $PIDML_TRAINING_DIR/scripts/preprocessing.py $PIDML_TRAINING_DIR/data/preprocessed_ao2ds.root -o $PIDML_TRAINING_DIR/data/$CSV_FILE
# data preparation for models
alienv setenv Python/latest -c \
  source $VIRTUAL_ENV_ACTIVATE && \
  $PIDML_TRAINING_DIR/scripts/prepare_data.py $PIDML_TRAINING_DIR/data/$CSV_FILE -o $PIDML_TRAINING_DIR/data/processed
# training
alienv setenv Python/latest -c \
  source $VIRTUAL_ENV_ACTIVATE && \
  $PIDML_TRAINING_DIR/scripts/train.py -c $PIDML_TRAINING_DIR/scripts/train_default_cfg.json
exit

# TODO: data clean up
# TODO: run benchmark task (generate graphs)
