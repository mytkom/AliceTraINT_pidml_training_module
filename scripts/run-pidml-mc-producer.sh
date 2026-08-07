#!/bin/bash

set -euo pipefail

DIR_THIS="$(dirname "$(realpath "$0")")"

# shellcheck disable=SC1091
source "$DIR_THIS/config.sh"
# shellcheck disable=SC1091
source "$DIR_THIS/lib/args.sh"

AO2D_LIST_FILE="${1:-}"
OUTPUT_NAME="${2:-}"
IS_O_NE="${3:-}"
IS_DATA="${4:-}"
TABLE_NAME=""
CONFIG_SRC=""
CONFIG_FILE=""

validate_inputs() {
  require_args_count "$#" 4 "Usage: $0 <ao2d_list_file> <output_name> <is_o_ne:true|false> <is_data:true|false>"
  require_env PIDML_TRAINING_DIR
  require_env DATA_DIR
  require_nonempty "$AO2D_LIST_FILE" "AO2D_LIST_FILE"
  require_nonempty "$OUTPUT_NAME" "OUTPUT_NAME"
  require_bool "$IS_O_NE" "is_o_ne"
  require_bool "$IS_DATA" "is_data"
}

select_config_and_table() {
  if [ "$IS_DATA" = "true" ]; then
    TABLE_NAME="PIDTRACKSDATA"
    CONFIG_SRC="$DIR_THIS/O2configs/data-config.json"
  else
    TABLE_NAME="PIDTRACKSMC"
    CONFIG_SRC="$DIR_THIS/O2configs/sim-config.json"
  fi
}

prepare_config() {
  echo "1. Performing AO2D list substitution in config file"
  CONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/pidml-o2-config.XXXXXX.json")"
  cp "$CONFIG_SRC" "$CONFIG_FILE"
  sed -i 's/"aod-file-private": ".*",$/"aod-file-private": "'"${AO2D_LIST_FILE//\//\\/}"'",/' "$CONFIG_FILE"
}

cleanup_config() {
  if [ -n "${CONFIG_FILE:-}" ] && [ -f "$CONFIG_FILE" ]; then
    rm -f "$CONFIG_FILE"
  fi
}

run_pipeline() {
  echo "2. Running producer"
  local shm_segment_size="--aod-memory-rate-limit 2097152000 --shm-segment-size 20000000000"

  if [ "$IS_O_NE" = "true" ]; then
    o2-analysis-event-selection-service $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-propagationservice  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-ft0-corrected-table  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-multcenttable  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-trackselection  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-pid-tof-merge  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-pid-tpc-service  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-pid-ml-producer $shm_segment_size --configuration "json://$CONFIG_FILE" --aod-file "$AO2D_LIST_FILE" -b \
      --aod-writer-keep "AOD/$TABLE_NAME/0:::$OUTPUT_NAME" --aod-writer-resdir "$DATA_DIR"
  else
    o2-analysis-tracks-extra-v002-converter  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-mccollision-converter  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-event-selection-service $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-propagationservice  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-ft0-corrected-table  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-multcenttable  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-trackselection  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-pid-tof-merge  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-pid-tpc-service  $shm_segment_size --configuration "json://$CONFIG_FILE" -b |
    o2-analysis-pid-ml-producer $shm_segment_size --configuration "json://$CONFIG_FILE" --aod-file "$AO2D_LIST_FILE" -b \
      --aod-writer-keep "AOD/$TABLE_NAME/0:::$OUTPUT_NAME" --aod-writer-resdir "$DATA_DIR"
  fi
}

finalize_outputs() {
  echo "3. Moving analysis results to training directory"
  mkdir -p "$PIDML_TRAINING_DIR"
  if [ -f "AnalysisResults.root" ]; then
    mv "AnalysisResults.root" "$PIDML_TRAINING_DIR/${OUTPUT_NAME}_analysis_results.root"
  fi
  [ -f "$DATA_DIR/$OUTPUT_NAME.root" ] || die "Producer output missing: $DATA_DIR/$OUTPUT_NAME.root"
  mv "$DATA_DIR/$OUTPUT_NAME.root" "$PIDML_TRAINING_DIR/$OUTPUT_NAME.root"
  echo "4. Done (DATA_DIR left for orchestrator cleanup)"
}


echo "training directory: $PIDML_TRAINING_DIR"
echo "data directory: $DATA_DIR"
echo "ao2d list file: $AO2D_LIST_FILE"
echo "output name: $OUTPUT_NAME"
echo "is O-O or Ne-Ne: $IS_O_NE"
echo "is data: $IS_DATA"
validate_inputs "$@"
select_config_and_table
trap cleanup_config EXIT

echo "config source: $CONFIG_SRC"
prepare_config
echo "config file: $CONFIG_FILE"
run_pipeline
finalize_outputs
