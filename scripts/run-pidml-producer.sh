#!/bin/bash

# ALICE::FZK::SE (default) takes ages to load or crash
# export alien_CLOSE_SE=ALICE::GSI::SE2

config_file="$1"
DATA_DIR="$2"
AO2D_LIST_FILE="@$3"
OUTPUT_NAME="$4"
GENERATED_SCRIPT_PATH="$5"

executeAnalysis() {
  local processV000ToV002="$1"
  local processV001ToV002="$2"

  # Update the config_file with sed
  sed -i 's/"processV000ToV002": ".*"/"processV000ToV002": "'"${processV000ToV002}"'"/' $config_file
  sed -i 's/"processV001ToV002": ".*"/"processV001ToV002": "'"${processV001ToV002}"'"/' $config_file

  # Update AO2D_LIST_FILE in config_file
  sed -i 's/"aod-file-private": ".*",$/"aod-file-private": "'"${AO2D_LIST_FILE//\//\\/}"'",/' $config_file

  # Execute the analysis pipeline
  $GENERATED_SCRIPT_PATH $config_file $OUTPUT_NAME $DATA_DIR
}

# First execution: processV000ToV002=true, processV001ToV002=false
# Second execution: processV000ToV002=false, processV001ToV002=true
executeAnalysis "true" "false" || executeAnalysis "false" "true"

# Save the analysis results
mv AnalysisResults.root $DATA_DIR/producer_task_analysis_results.root