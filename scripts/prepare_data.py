#!/usr/bin/env python

import argparse
import sys
import os

def main(input_file, output_dir):
    # from pdi.data.preparation import MeanImputation
    # prep = MeanImputation()
    # prep.prepare_data(input_file)
    # prep.save_data(os.path.join(output_dir, "mean"))

    # from pdi.data.preparation import DeletePreparation
    # prep = DeletePreparation()
    # prep.prepare_data(input_file)
    # prep.save_data(os.path.join(output_dir, "deleted"))

    # from pdi.data.preparation import RegressionImputation
    # prep = RegressionImputation()
    # prep.prepare_data(input_file)
    # prep.save_data(os.path.join(output_dir, "regression"))

    # from pdi.data.preparation import EnsemblePreparation
    # prep = EnsemblePreparation()
    # prep.prepare_data(input_file)
    # prep.save_data(os.path.join(output_dir, "ensemble"))

    from pdi.data.preparation import FeatureSetPreparation
    prep = FeatureSetPreparation()
    prep.prepare_data(input_file)
    prep.save_data(os.path.join(output_dir, "feature_set"))

    from pdi.data.utils import DataPreparation
    prep = DataPreparation()
    prep.prepare_data(input_file)
    prep.save_data(os.path.join(output_dir, "basic"))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Process CSV file.")
    parser.add_argument('-p', '--pdi-dir', dest="pdi_dir", type=str, help="Path to directory containing pdi source code")
    parser.add_argument('input_file', type=str, help="CSV file to process")
    parser.add_argument('-o', '--output', type=str, dest="output_dir", help="Output dir name.")
    args = parser.parse_args()

    if args.pdi_dir not in sys.path:
        sys.path.append(args.pdi_dir)

    main(args.input_file, args.output_dir)
