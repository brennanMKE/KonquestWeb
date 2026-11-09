#!/usr/bin/env bash
# Stop any running Parallax server, rebuild, and start a fresh one.
#
#   scripts/restart.sh                 # rebuild, then serve on :8080
#   scripts/restart.sh --no-build      # restart the current build
#   scripts/restart.sh -addr :9090     # extra flags go to the server
#   scripts/restart.sh -system1 http://127.0.0.1:8799/predict
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

echo "Starting bin/server $*"
exec bin/server "$@"
