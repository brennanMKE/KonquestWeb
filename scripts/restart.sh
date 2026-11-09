#!/usr/bin/env bash
# Stop any running Parallax server, rebuild, and start a fresh one.
#
#   scripts/restart.sh                 # rebuild, then serve on :8080
#   scripts/restart.sh --no-build      # restart the current build
#   scripts/restart.sh -addr :9090     # extra flags go to the server
#   scripts/restart.sh -system1 off    # no System 1 model
#
# Computer players use the System 1 model (Laya) by default. If System 1
# Arcade is installed and Laya isn't already serving on :8799, this starts it
# in the background (log in .run/laya.log). Set LAYA_DIR to use another
# install.
set -euo pipefail
cd "$(dirname "$0")/.."

build=1
if [[ "${1:-}" == "--no-build" ]]; then
  build=0
  shift
fi

if pkill -f "bin/server" 2>/dev/null; then
  echo "Stopped the running server."
  # Wait up to 5 s for it to exit and free its port.
  for _ in $(seq 50); do
    pgrep -f "bin/server" >/dev/null || break
    sleep 0.1
  done
fi

if ((build)); then
  make build
fi

laya_dir="${LAYA_DIR:-$HOME/Library/Application Support/System 1 Arcade/agent}"
if [[ " $* " != *" -system1 "* ]]; then
  if curl -fs --max-time 2 http://127.0.0.1:8799/health >/dev/null; then
    echo "Laya is serving on :8799."
  elif [[ -x "$laya_dir/venv/bin/python" ]]; then
    mkdir -p .run
    echo "Starting Laya on :8799 (log in .run/laya.log)…"
    nohup "$laya_dir/venv/bin/python" "$laya_dir/laya_server.py" --port 8799 >.run/laya.log 2>&1 &
    # Loading the model takes a while; the server plays Planner until then.
    for _ in $(seq 60); do
      curl -fs --max-time 1 http://127.0.0.1:8799/health >/dev/null && { echo "Laya is up."; break; }
      sleep 1
    done
  else
    echo "System 1 Arcade's Laya isn't installed at $laya_dir; computer players will use Planner."
  fi
fi

echo "Starting bin/server $*"
exec bin/server "$@"
