#!/usr/bin/bash
set -euo pipefail

DIR_THIS="$(dirname "$(realpath "$0")")"
# shellcheck disable=SC1091
source "$DIR_THIS/lib/args.sh"

USAGE="Usage: $0 <event_count> <output_file> <is_data:true|false> <input_root_1> [input_root_2 ...]"
EV_NUMB="${1:-}"
OUTPUT_FILE="${2:-}"
IS_DATA="${3:-}"
shift 3 || true
INPUT_FILES=( "$@" )

require_nonempty "$EV_NUMB" "event_count"
require_nonempty "$OUTPUT_FILE" "output_file"
require_bool "$IS_DATA" "is_data"
[ "${#INPUT_FILES[@]}" -gt 0 ] || die "$USAGE"

if [ "$IS_DATA" = "true" ]; then
  echo "using O2pidtracksdata (assuming experimental data)"
  OBS_TREE="O2pidtracksdata"
else
  echo "using O2pidtracksmc (assuming MC data)"
  OBS_TREE="O2pidtracksmc"
fi

for input_file in "${INPUT_FILES[@]}"; do
  [ -f "$input_file" ] || die "Input ROOT file not found: $input_file"
done

mkdir -p "$(dirname "$OUTPUT_FILE")"

SAMPLING_ARGS=()
if [ "$EV_NUMB" -gt 0 ]; then
  # multiply by it to get event number sampled by DataFrames
  DF_MULT="1.5"
  DF_SUB_NUMB=$(echo "($EV_NUMB * $DF_MULT)/1" | bc)
  SAMPLING_ARGS=( --by-dataframes-up-to-events "$DF_SUB_NUMB" )
else
  # event_count equals 0 - full dataset, only merge the inputs
  echo "event_count equals 0, merging inputs without subsampling"
fi

output_stem="$(basename "$OUTPUT_FILE" .root)"
LOG_FILE="$(dirname "$OUTPUT_FILE")/${output_stem}-subsampling.log"
TMP_FILE="$(dirname "$OUTPUT_FILE")/${output_stem}-subsampling.tmp.root"
SUBSAMPLE_BIN="$DIR_THIS/subsample"

[ -x "$SUBSAMPLE_BIN" ] || die "subsample binary not executable: $SUBSAMPLE_BIN"

"$SUBSAMPLE_BIN" ${SAMPLING_ARGS[@]+"${SAMPLING_ARGS[@]}"} \
  --tree-name "$OBS_TREE" \
  "$TMP_FILE" \
  "${INPUT_FILES[@]}" \
  2>&1 | tee "$LOG_FILE"

# Second-pass by-events trim kept commented (same as upstream scripts).
# Promote the temp output to the requested output path.
[ -f "$TMP_FILE" ] || die "Subsample temp output missing: $TMP_FILE"
mv -f -- "$TMP_FILE" "$OUTPUT_FILE"
echo "Wrote dataset: $OUTPUT_FILE"

