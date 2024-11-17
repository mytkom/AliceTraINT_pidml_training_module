#!/bin/bash

# ALICE::FZK::SE (default) takes ages to load or crash
export alien_CLOSE_SE=ALICE::GSI::SE2

config_file="$PIDML_TRAINING_DIR/scripts/ml-mc-config.json"
DATA_DIR="$PIDML_TRAINING_DIR/data"
AO2D_LIST_FILE="@$DATA_DIR/local_ao2ds_list.txt"

sed -i 's/"aod-file-private": ".*"$/"aod-file": "'"${AO2D_LIST_FILE//\//\\/}"'"/' $config_file

o2-analysis-event-selection --configuration json://$config_file -b |
o2-analysis-timestamp --configuration json://$config_file -b |
o2-analysis-multiplicity-table --configuration json://$config_file -b |
o2-analysis-mccollision-converter --configuration json://$config_file -b |
o2-analysis-track-propagation --configuration json://$config_file -b |
o2-analysis-trackselection --configuration json://$config_file -b |
o2-analysis-pid-tof-base --configuration json://$config_file -b |
o2-analysis-pid-tof-beta --configuration json://$config_file -b |
o2-analysis-pid-tof-full --configuration json://$config_file -b |
o2-analysis-pid-tpc-base --configuration json://$config_file -b |
o2-analysis-pid-tpc --configuration json://$config_file -b |
o2-analysis-pid-ml-producer --configuration json://$config_file -b \
  --aod-writer-keep AOD/PIDTRACKSMCML/0:::preprocessed_ao2ds --aod-writer-resdir $DATA_DIR
mv AnalysisResults.root $DATA_DIR/producer_task_analysis_results.root
