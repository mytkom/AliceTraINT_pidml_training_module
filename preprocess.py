import uproot3
import os
import pandas as pd
import numpy as np

print("Converting preprocessed ROOT file to CSV file")
dataframes = []
file = uproot3.open("/home/mytkom/Documents/alice/preprocessed_datasets/MC_ALL_AO2D_LHC24b1-527108-AODS-1-8.root")
for dirname in file:
    dirname = dirname.decode("utf-8")
    pure_dirname = dirname.split(";")[0]
    if pure_dirname.startswith("DF_"):
        tree_data = file["%s/O2pidtracksmcml" % (dirname)].pandas.df()
        dataframes.append(tree_data)

data = pd.concat(dataframes, ignore_index=True)
print(data.head())
print(data.columns)

# TRDPattern is uint8, so cannot use NaN in producer -> need to preprocess it here
data["fTRDPattern"].mask(np.isclose(data["fTRDPattern"], 0), inplace=True)
data = data[data["fTPCSignal"] > 0]
data.to_csv("/home/mytkom/Documents/alice/preprocessed_datasets/MC_ALL_AO2D_LHC24b1-527108-AODS-1-8.csv")