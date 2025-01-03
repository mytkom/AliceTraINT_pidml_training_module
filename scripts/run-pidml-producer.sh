#!/bin/bash

# ALICE::FZK::SE (default) takes ages to load or crash
# export alien_CLOSE_SE=ALICE::GSI::SE2

config_file="$1"
DATA_DIR="$2"
AO2D_LIST_FILE="@$3"
OUTPUT_NAME="$4"

sed -i 's/"aod-file-private": ".*",$/"aod-file-private": "'"${AO2D_LIST_FILE//\//\\/}"'",/' $config_file

o2-analysis-event-selection --configuration json://$config_file -b |
o2-analysis-mccollision-converter --configuration json://$config_file -b |
o2-analysis-track-propagation --configuration json://$config_file -b |
o2-analysis-trackselection --configuration json://$config_file -b |
o2-analysis-mft-tracks-converter --configuration json://$config_file -b |
o2-analysis-tracks-extra-v002-converter --configuration json://$config_file -b |
o2-analysis-bc-converter --configuration json://$config_file -b |
o2-analysis-zdc-converter --configuration json://$config_file -b |
o2-analysis-pid-tof-base --configuration json://$config_file -b |
o2-analysis-pid-tof-beta --configuration json://$config_file -b |
o2-analysis-timestamp --configuration json://$config_file -b |
o2-analysis-pid-ml-producer --configuration json://$config_file -b \
  --aod-writer-keep AOD/PIDTRACKSMCML/0:::$OUTPUT_NAME --aod-writer-resdir $DATA_DIR
mv AnalysisResults.root $DATA_DIR/producer_task_analysis_results.root
