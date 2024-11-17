#!/bin/bash

DOWNLOAD_SCRIPT="$PIDML_TRAINING_DIR/scripts/download-from-grid.sh"
DATA_DIR="$PIDML_TRAINING_DIR/data/raw_ao2ds"
AO2DS_REMOTE_LIST_FILE="$PIDML_TRAINING_DIR/file_list_mixed.txt"
AO2DS_LOCAL_LIST_FILE="$PIDML_TRAINING_DIR/data/local_ao2ds_list.txt"

echo $DATA_DIR

mkdir -p $DATA_DIR

$DOWNLOAD_SCRIPT $AO2DS_REMOTE_LIST_FILE $DATA_DIR

# Clear or create local list file
> $AO2DS_LOCAL_LIST_FILE

while read remote_url; do
  local_path=$DATA_DIR/$(echo ${remote_url} | tail -c +2 | tr "/" "-")
  mv $DATA_DIR/${remote_url} ${local_path}
  echo ${local_path} >> $AO2DS_LOCAL_LIST_FILE
done < $AO2DS_REMOTE_LIST_FILE

rm -rf $DATA_DIR/alice
