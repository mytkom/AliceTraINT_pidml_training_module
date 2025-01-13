#!/usr/bin/env python

import argparse
import uproot3
import os

converters = [
    ("O2bc", "o2-analysis-bc-converter"),
    ("O2collision", "o2-analysis-collision-converter"),
    ("O2fdd", "o2-analysis-fdd-converter"),
    ("O2mccollision", "o2-analysis-mccollision-converter"),
    ("O2mcparticle", "o2-analysis-mc-converter"),
    ("O2mfttrack", "o2-analysis-mft-tracks-converter"),
    ("O2trackextra, O2trackextra_001", "o2-analysis-tracks-extra-v002-converter"),
    ("O2zdc", "o2-analysis-zdc-converter"),
]

def find_trees_recursively(directory, prefix=""):
    """
    Recursively find all TTrees in the ROOT file.
    """
    ttree_names = []
    for key, item in directory.iteritems():
        if isinstance(item, uproot3.tree.TTreeMethods):
            ttree_names.append(key.decode("utf-8")[:-2])
        elif isinstance(item, uproot3.rootio.ROOTDirectory):
            sub_prefix = f"{prefix}{key.decode('utf-8')}/"
            ttree_names.extend(find_trees_recursively(item, sub_prefix))
    return ttree_names

def check_available_trees(file_path, converters):
    try:
        with uproot3.open(file_path) as root_file:
            ttree_names = find_trees_recursively(root_file)

        print(ttree_names)

    except Exception as e:
        print(f"Error opening ROOT file: {e}")
        return []

    matched_converters = []
    for old, converter in converters:
        old_keys = old.split(", ")
        if any(name in ttree_names for name in old_keys):
            matched_converters.append((old, converter))

    return matched_converters

def generate_bash_script(matched_converters, output_script_path):
    """
    Generate a bash script that runs the necessary converters.
    """
    with open(output_script_path, "w") as f:
        f.write("#!/bin/bash\n")
        f.write("# Generated script to run o2-analysis converters\n")
        f.write("CONFIG_FILE=$1\n")
        f.write("OUTPUT_NAME=$2\n")
        f.write("DATA_DIR=$3\n\n")

        f.write("o2-analysis-event-selection --configuration json://$CONFIG_FILE -b |\n")
        f.write("  o2-analysis-track-propagation --configuration json://$CONFIG_FILE -b |\n")
        f.write("  o2-analysis-trackselection --configuration json://$CONFIG_FILE -b |\n")
        f.write("  o2-analysis-pid-tof-base --configuration json://$CONFIG_FILE -b |\n")
        f.write("  o2-analysis-pid-tof-beta --configuration json://$CONFIG_FILE -b |\n")
        f.write("  o2-analysis-timestamp --configuration json://$CONFIG_FILE -b |\n")

        for old, converter in matched_converters:
            f.write(f"  {converter} --configuration json://$CONFIG_FILE -b |\n")
        
        f.write(f"  o2-analysis-pid-ml-producer --configuration json://$CONFIG_FILE -b \\\n")
        f.write("    --aod-writer-keep AOD/PIDTRACKSMCML/0:::$OUTPUT_NAME --aod-writer-resdir $DATA_DIR\n")

    print(f"Bash script generated: {output_script_path}")

    os.chmod(output_script_path, 0o755)
    print(f"Permissions set to executable for: {output_script_path}")

def main():
    parser = argparse.ArgumentParser(description="Recursively check for TTrees in an AOD ROOT file and suggest converters.")
    parser.add_argument("input_file", help="Path to the input AOD ROOT file")
    parser.add_argument("output_script", help="Path to the output bash script")
    args = parser.parse_args()

    matched = check_available_trees(args.input_file, converters)

    if matched:
        print("Matched converters:")
        for old, converter in matched:
            print(f"Old Subdir: {old}, Converter Script: {converter}")
        
        generate_bash_script(matched, args.output_script)
    else:
        print("No matching TTrees found in the ROOT file.")

if __name__ == "__main__":
    main()
