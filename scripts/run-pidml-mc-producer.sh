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
TRACKEXTRA_VERSION=""

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

# check which O2trackextra table the first AOD has
detect_trackextra_version() {
  local list_file="${AO2D_LIST_FILE#@}"
  local aod
  aod="$(grep -v '^[[:space:]]*$' "$list_file" | head -1 | sed 's/^@//')"
  [ -f "$aod" ] || die "first AOD not found: ${aod:-<empty>}"
  TRACKEXTRA_VERSION="$(python3 -c '
import sys, ROOT
ROOT.gROOT.SetBatch(True)
f = ROOT.TFile.Open(sys.argv[1])
d = f.Get(next(k.GetName() for k in f.GetListOfKeys() if k.GetName().startswith("DF_")))
for v, n in (("v002", "O2trackextra_002"), ("v001", "O2trackextra_001"), ("v000", "O2trackextra")):
    if d.Get(n):
        print(v)
        break
else:
    raise SystemExit("no O2trackextra*")
' "$aod")" || die "trackextra version detect failed for $aod"
  echo "trackextra version: $TRACKEXTRA_VERSION"
}

prepare_config() {
  echo "1. Performing AO2D list substitution in config file"
  CONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/pidml-o2-config.XXXXXX.json")"
  cp "$CONFIG_SRC" "$CONFIG_FILE"
  sed -i 's/"aod-file-private": ".*",$/"aod-file-private": "'"${AO2D_LIST_FILE//\//\\/}"'",/' "$CONFIG_FILE"
  # for v000 AOD, switch converter from V001→V002 to V000→V002
  if [ "$IS_O_NE" != "true" ] && [ "$TRACKEXTRA_VERSION" = "v000" ]; then
    sed -i \
      -e 's/"processV000ToV002": "false"/"processV000ToV002": "true"/' \
      -e 's/"processV001ToV002": "true"/"processV001ToV002": "false"/' \
      "$CONFIG_FILE"
  fi
}

cleanup_config() {
  if [ -n "${CONFIG_FILE:-}" ] && [ -f "$CONFIG_FILE" ]; then
    rm -f "$CONFIG_FILE"
  fi
}

run_pipeline() {
  echo "2. Running producer"
  local shm_segment_size="--aod-memory-rate-limit 2097152000 --shm-segment-size 20000000000"

  # v002 AOD is already modern (also has mccollision_001); skip converters, same as OO/Ne
  if [ "$IS_O_NE" = "true" ] || [ "$TRACKEXTRA_VERSION" = "v002" ]; then
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
if [ "$IS_O_NE" != "true" ]; then
  detect_trackextra_version
fi
prepare_config
echo "config file: $CONFIG_FILE"
run_pipeline
finalize_outputs
