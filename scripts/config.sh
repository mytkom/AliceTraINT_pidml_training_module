#!/bin/bash

# Overridable by the training-module orchestrator (Go exports these).
# Defaults keep standalone script usage workable.
PIDML_TRAINING_DIR="${PIDML_TRAINING_DIR:-/root/alice/AliceTraINT_pidml_training_module}"

# Directory where raw AO2Ds are temporarily stored during download/produce
DATA_DIR="${DATA_DIR:-$PIDML_TRAINING_DIR/data}"

# Unused by training-module orchestration; kept for standalone compatibility
BATCH_ROOT="${BATCH_ROOT:-$PIDML_TRAINING_DIR/sim-and-data-batch-files}"
SUBSAMPLED_DIR="${SUBSAMPLED_DIR:-$PIDML_TRAINING_DIR/subsampled}"
