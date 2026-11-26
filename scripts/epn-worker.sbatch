#!/bin/bash
#SBATCH -J alicetraint-worker
#SBATCH -p dev
#SBATCH -N 1
#SBATCH --gres=gpu:1
#SBATCH --mem=32G
#SBATCH --time=1-00:00:00
#SBATCH --signal=B:TERM@300
#SBATCH -o alicetraint-worker-%j.out
#SBATCH -e alicetraint-worker-%j.err

set -euo pipefail
cd "${SLURM_SUBMIT_DIR:-$HOME/AliceTraINT_pidml_training_module}"

shopt -s nullglob
for f in alicetraint-worker-*.out alicetraint-worker-*.err; do
  [[ "$f" == *"-${SLURM_JOB_ID}."* ]] || rm -f "$f"
done

set -a; source .env; set +a

LOCK_FILE=.epn-worker.lock
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
  echo "exiting: another worker holds $LOCK_FILE" >&2
  exit 0
fi

STOP_FILE=.epn-worker-stop
if [[ -z "${SLURM_JOB_DEPENDENCY:-}" ]]; then
  rm -f "$STOP_FILE"
elif [[ -f "$STOP_FILE" ]]; then
  echo "exiting: stop file present ($STOP_FILE)"
  exit 0
fi

BIN=./bin/AliceTraINT_pidml_training_module
[[ -x "$BIN" ]] || { echo "error: missing $BIN (run make build)" >&2; exit 1; }

sbatch --dependency=afterany:"${SLURM_JOB_ID}" "${SLURM_SUBMIT_DIR:-$PWD}/scripts/epn-worker.sbatch"

source /etc/profile.d/modules.sh
source /etc/profile.d/epn-modulepath.sh
module load "${ALICETRAINT_PYTHON_MODULE:-Python/v3.10.19-7}"
source .venv-rocm/bin/activate

"${ALICETRAINT_ALIENV_BIN}" setenv "${ALICETRAINT_XJALIENFS_TAG}" -c alien-token-init
"${ALICETRAINT_ALIENV_BIN}" setenv "${ALICETRAINT_O2PHYSICS_TAG}" -c make subsample

"$BIN"
