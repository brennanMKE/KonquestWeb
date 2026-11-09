# Laya model server

The System 1 computer players ask a model whether planets are in danger and
whether attacks are good bets. These two files serve System 1 Arcade's Laya
model (same author, MIT) over HTTP:

    POST /predict   decisions, batched
    GET  /health    {"ok": true}

Set up and run:

    python3 -m venv venv
    venv/bin/pip install torch==2.14.0 --index-url https://download.pytorch.org/whl/cpu   # no GPU
    venv/bin/pip install -r requirements.txt
    HF_HOME=/path/to/hf-cache venv/bin/python laya_server.py --port 8799   # binds 127.0.0.1

Then start Parallax with `-system1 http://127.0.0.1:8799/predict`.

- The first start downloads the model (convaiinnovations/laya, 804 MB) into
  `HF_HOME`. Plan on about 1.5 GB of memory.
- Flags: `--host` (default 127.0.0.1), `--port`, `--device` (cpu, cuda, mps;
  default auto), `--cache-size` (answers to cache, default 10000).
- `--key` or `SYSTEM1_LAYA_KEY` requires `Authorization: Bearer <key>` on
  /predict and /stats (/health stays open). Give Parallax the same key in
  `PARALLAX_SYSTEM1_KEY` (or `-system1-key`); keep both in root-only
  environment files rather than on the command line.
- Environment: `SYSTEM1_LAYA_MODEL` (another checkpoint),
  `SYSTEM1_LAYA_WEIGHTS` (tuned head), `SYSTEM1_LAYA_CACHE` (cache size).
- On an Apple M-series CPU with no GPU, a decision on a 32³ map takes about
  0.4 s; expect a few times that on a Raspberry Pi 5. Parallax waits up to
  15 s per decision, then plays Planner for that turn.
