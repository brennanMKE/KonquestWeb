#!/usr/bin/env bash
# Stop any running Parallax server, rebuild, and start a fresh one.
#
#   scripts/restart.sh                 # rebuild, then serve on :8080
#   scripts/restart.sh --no-build      # restart the current build
#   scripts/restart.sh -addr :9090     # extra flags go to the server
#   scripts/restart.sh -system1 off    # no System 1 model
#
# Locally, computer players use the System 1 model (Laya) by default: if
# System 1 Arcade is installed, this starts Laya on :8799 when it isn't
# already serving (log in .run/laya.log) and passes -system1 to the server.
# Set LAYA_DIR to use another install.
#
# Open rooms survive the restart: the server keeps them in .run/rooms
# (-state) and reopens them, so players rejoin where they were. The server binary itself leaves
# System 1 off unless -system1 is given, as on photon.
set -euo pipefail
cd "$(dirname "$0")/.."

build=1
if [[ "${1:-}" == "--no-build" ]]; then
  build=0
  shift
fi

if pkill -f "bin/server" 2>/dev/null; then
  echo "Stopped the running server."
  # Wait up to 10 s for it to save its rooms, exit and free its port.
  for _ in $(seq 100); do
    pgrep -f "bin/server" >/dev/null || break
    sleep 0.1
  done
fi

if ((build)); then
  make build
fi

laya_dir="${LAYA_DIR:-$HOME/Library/Application Support/System 1 Arcade/agent}"
laya_url=http://127.0.0.1:8799/predict
args=("$@")
if [[ " $* " == *" -system1 off "* || " $* " == *" -system1 off" ]]; then
  # Drop "-system1 off": the server's default is already off.
  args=()
  skip=0
  for a in "$@"; do
    if ((skip)); then skip=0; continue; fi
    if [[ "$a" == "-system1" ]]; then skip=1; continue; fi
    args+=("$a")
  done
elif [[ " $* " != *" -system1 "* ]]; then
  if curl -fs --max-time 2 http://127.0.0.1:8799/health >/dev/null; then
    echo "Laya is serving on :8799."
    args+=(-system1 "$laya_url")
  elif [[ -x "$laya_dir/venv/bin/python" ]]; then
    mkdir -p .run
    echo "Starting Laya on :8799 (log in .run/laya.log)…"
    nohup "$laya_dir/venv/bin/python" "$laya_dir/laya_server.py" --port 8799 >.run/laya.log 2>&1 &
    # Loading the model takes a while; the server plays Planner until then.
    for _ in $(seq 60); do
      curl -fs --max-time 1 http://127.0.0.1:8799/health >/dev/null && { echo "Laya is up."; break; }
      sleep 1
    done
    # Even if it's still loading: the server plays Planner until it answers.
    args+=(-system1 "$laya_url")
  else
    echo "System 1 Arcade's Laya isn't installed at $laya_dir; computer players will use Planner."
  fi
fi

if [[ " $* " != *" -state "* ]]; then
  mkdir -p .run
  args+=(-state .run/rooms)
fi

echo "Starting bin/server ${args[*]:-}"
exec bin/server ${args[@]+"${args[@]}"}
