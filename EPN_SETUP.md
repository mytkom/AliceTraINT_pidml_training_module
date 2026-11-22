## 0. Worker on EPN (from login)

Build binary first (on **login**; repeat after code changes):

```bash
export PATH=$HOME/go/bin:$PATH
make build
```

Start:

```bash
sbatch scripts/epn-worker.sbatch
```

Status:

```bash
squeue -u $USER
```

Stop (break auto-resubmit chain):

```bash
touch .epn-worker-stop
scancel -n alicetraint-worker
```

Logs: `alicetraint-worker-%j.out` / `.err`. To follow paste:

```bash
jid=$(squeue -u $USER -n alicetraint-worker -t R -h -o %i | head -1)
tail -f alicetraint-worker-${jid}.out alicetraint-worker-${jid}.err
```

## 1. Interactive (debug)

```bash
srun -N1 -p dev --gres=gpu:1 --mem=32G --pty bash -i
make run
```

If something fails, do before `make run`:

```bash
set -a; source .env; set +a
export PATH=$HOME/go/bin:$PATH
module load Python/v3.10.19-7
source .venv/bin/activate
"$ALICETRAINT_ALIENV_BIN" setenv "$ALICETRAINT_XJALIENFS_TAG" -c alien-token-init
"$ALICETRAINT_ALIENV_BIN" setenv "$ALICETRAINT_O2PHYSICS_TAG" -c make subsample
```

## 2. venv + requirements (once on EPN, or after deleting `.venv`)

```bash
module load Python/v3.10.19-7
python3 -m venv .venv
source .venv/bin/activate
pip install -r pdi/requirements.txt
```

## 3. Install Go (once, on EPN or login, if `~/go` is missing)

```bash
cd ~
wget https://go.dev/dl/go1.22.10.linux-amd64.tar.gz
tar -xzf go1.22.10.linux-amd64.tar.gz
```

## 4. New O2Physics tag (once on EPN, when the tag disappears from CVMFS)

```bash
ls /cvmfs/alice.cern.ch/el9-x86_64/Modules/modulefiles/O2Physics | sort | tail
```

Paste `VO_ALICE@O2Physics::<name>` into `.env` as `ALICETRAINT_O2PHYSICS_TAG`.
