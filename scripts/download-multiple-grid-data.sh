#!/bin/bash

set -euo pipefail

DIR_THIS="$(dirname "$(realpath "$0")")"
# shellcheck disable=SC1091
source "$DIR_THIS/config.sh"
# shellcheck disable=SC1091
source "$DIR_THIS/lib/args.sh"

require_env PIDML_TRAINING_DIR
require_env DATA_DIR
echo "training directory: $PIDML_TRAINING_DIR"
echo "data directory: $DATA_DIR"
mkdir -p "$DATA_DIR"

DOWNLOAD_SCRIPT="$DIR_THIS/download-from-grid.sh"
# DATA_DIR="$PIDML_TRAINING_DIR/data_2"
AO2DS_LOCAL_LIST_FILE="$DATA_DIR/local_ao2ds_list.txt"
export XRD_REQUESTTIMEOUT=3600
export XRD_TIMEOUT=3600
export XRD_CONNECTIONWINDOW=120

AO2DS_REMOTE_LIST_FILE="${1:-}"
require_args_count "$#" 1 "Usage: $0 <ao2ds_remote_list_file>"
require_nonempty "$AO2DS_REMOTE_LIST_FILE" "ao2ds_remote_list_file"

# Download until complete (transient Grid timeouts are common).
# Only missing/failed files are retried on subsequent attempts.
# You can cap retries by setting DOWNLOAD_RETRY_MAX_ATTEMPTS (0 = infinite).
DOWNLOAD_RETRY_MAX_ATTEMPTS="${DOWNLOAD_RETRY_MAX_ATTEMPTS:-0}"
DOWNLOAD_RETRY_INITIAL_SLEEP_S="${DOWNLOAD_RETRY_INITIAL_SLEEP_S:-30}"
DOWNLOAD_RETRY_MAX_SLEEP_S="${DOWNLOAD_RETRY_MAX_SLEEP_S:-900}"

remaining_list="$(mktemp)"
trap 'rm -f "$remaining_list"' EXIT

attempt=1
sleep_s="$DOWNLOAD_RETRY_INITIAL_SLEEP_S"
while true; do
  : > "$remaining_list"

  # Build list of files that are still missing locally.
  while read -r remote_url; do
    remote_url="${remote_url//$'\r'/}"
    remote_url="${remote_url#"${remote_url%%[![:space:]]*}"}"
    remote_url="${remote_url%"${remote_url##*[![:space:]]}"}"
    [ -z "$remote_url" ] && continue

    expected_path="$DATA_DIR$remote_url"
    if [ ! -f "$expected_path" ]; then
      echo "$remote_url" >> "$remaining_list"
    fi
  done < "$AO2DS_REMOTE_LIST_FILE"

  remaining_count=$(grep -cve '^[[:space:]]*$' "$remaining_list" || true)
  if [ "$remaining_count" -eq 0 ]; then
    if [ "$attempt" -eq 1 ]; then
      echo "All AO2D files already present locally; no download needed."
    else
      echo "All AO2D files downloaded successfully after $((attempt - 1)) attempts."
    fi
    break
  fi

  if [ "$DOWNLOAD_RETRY_MAX_ATTEMPTS" -gt 0 ] && [ "$attempt" -gt "$DOWNLOAD_RETRY_MAX_ATTEMPTS" ]; then
    echo "Download failed to complete after ${DOWNLOAD_RETRY_MAX_ATTEMPTS} attempts."
    exit 1
  fi

  if [ "$attempt" -gt 1 ]; then
    echo "Retrying ${remaining_count} missing AO2D files (attempt ${attempt}) in ${sleep_s}s."
    sleep "$sleep_s"
    sleep_s=$((sleep_s * 2))
    if [ "$sleep_s" -gt "$DOWNLOAD_RETRY_MAX_SLEEP_S" ]; then
      sleep_s="$DOWNLOAD_RETRY_MAX_SLEEP_S"
    fi
  fi

  echo "Downloading ${remaining_count} AO2Ds (attempt ${attempt}) from list: ${AO2DS_REMOTE_LIST_FILE}"
  "$DOWNLOAD_SCRIPT" "$remaining_list" "$DATA_DIR" || true

  attempt=$((attempt + 1))
done

# Clear or create local list file
> "$AO2DS_LOCAL_LIST_FILE"

while read -r remote_url; do
  remote_url="${remote_url//$'\r'/}"
  remote_url="${remote_url#"${remote_url%%[![:space:]]*}"}"
  remote_url="${remote_url%"${remote_url##*[![:space:]]}"}"
  [ -z "$remote_url" ] && continue

  expected_path="$DATA_DIR$remote_url"
  if [ ! -f "$expected_path" ]; then
    echo "Expected downloaded file missing: $expected_path"
    exit 1
  fi

  stripped="${remote_url#/}"
  local_path="$DATA_DIR/${stripped//\//-}"
  mv -f -- "$expected_path" "$local_path"
  echo "$local_path" >> "$AO2DS_LOCAL_LIST_FILE"
done < "$AO2DS_REMOTE_LIST_FILE"

rm -rf -- "$DATA_DIR/alice"
