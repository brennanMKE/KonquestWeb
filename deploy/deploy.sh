#!/usr/bin/env bash
# shellcheck disable=SC2329 # install_/rollback_ functions are called by name
# Deploy the Parallax game server to photon or yoyo, or roll one back.
#
#   deploy/deploy.sh photon            # build main's HEAD, ship it, restart, check
#   deploy/deploy.sh yoyo
#   deploy/deploy.sh both
#   deploy/deploy.sh photon --rollback # back to the release before this one
#   deploy/deploy.sh photon --check    # only run the checks against what's live
#   deploy/deploy.sh yoyo --no-build   # ship bin/parallax-linux-arm64 as it is
#
# It only replaces the binary. The systemd units, Apache vhosts, tailscale
# serve, the Laya model and the System 1 key belong to the ops repos
# (~/Developer/AWS for photon, ~/Developer/Homelab for yoyo) and are never
# touched here; the script only checks that the unit has what this build
# needs (-state, so rooms survive the restart).
#
# Open games survive a deploy: the server saves its rooms on SIGTERM and
# reopens them at start, and browsers reconnect with their seat tokens.
#
# Hosts:
#   photon  EC2, https://parallax.sstools.co. Releases live in
#           /opt/parallax/releases/<commit>/parallax; /opt/parallax/current
#           links to the live one. The live release and the 3 before it
#           are kept for --rollback. Releases are root:root 0755 and get
#           their SELinux labels back with restorecon.
#   yoyo    Raspberry Pi 5, https://yoyo.sawfish-byzantine.ts.net (tailnet).
#           The binary is /usr/local/bin/parallax; the one before it is kept
#           as /usr/local/bin/parallax.prev for --rollback. Note that the
#           Homelab repo's yoyo/parallax/deploy.sh pins a sha256: after a
#           deploy from here, update that pin before running it again.
#
# Both are linux/arm64. Needs ssh aliases `photon` and `yoyo` with sudo on the
# far side. Brennan runs it: it's a production deploy.
#
# Never touched: units and drop-ins, /etc/photon (ports), /etc/parallax (the
# key: never read), vhosts and certs, /etc/hosts, Tailscale, and the saved
# rooms in /var/lib/parallax (they hold seat tokens).
set -euo pipefail
cd "$(dirname "$0")/.."

die() { printf 'deploy: %s\n' "$*" >&2; exit 1; }
say() { printf '==> %s\n' "$*"; }

usage() { sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

target="${1:-}"
[[ -n "$target" ]] || usage
shift
mode=deploy
build=1
for a in "$@"; do
  case "$a" in
    --rollback) mode=rollback ;;
    --check) mode=check ;;
    --no-build) build=0 ;;
    -h | --help) usage ;;
    *) die "unknown option $a" ;;
  esac
done
case "$target" in
  photon | yoyo) hosts=("$target") ;;
  both) hosts=(photon yoyo) ;;
  *) usage ;;
esac

BIN=bin/parallax-linux-arm64
KEEP=4 # the live release and the 3 before it (photon)

# Per-host settings.
setup() {
  case "$1" in
    photon)
      HOST="${PHOTON_HOST:-photon}"
      URL=https://parallax.sstools.co
      UNIT=parallax
      # The port comes from photon's port registry (/etc/photon/ports.conf).
      LOCAL=http://127.0.0.1:8114
      ;;
    yoyo)
      HOST="${YOYO_HOST:-yoyo}"
      URL=https://yoyo.sawfish-byzantine.ts.net
      UNIT=parallax
      LOCAL=http://127.0.0.1:8080
      ;;
  esac
  SSH=(ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST")
}

build_binary() {
  if ((build)); then
    [[ -z "$(git status --porcelain --untracked-files=no)" ]] || die "uncommitted changes; commit them or use --no-build"
    [[ "$(git rev-parse --abbrev-ref HEAD)" == main ]] || die "not on main"
    git fetch -q origin main
    [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main isn't the same as origin/main; push or pull first"
    say "Building $BIN"
    make -s linux-arm64
  fi
  [[ -f "$BIN" ]] || die "no $BIN; run without --no-build"
  COMMIT=$(git rev-parse --short=7 HEAD)
  SHA=$(shasum -a 256 "$BIN" | cut -d' ' -f1)
  say "Parallax $COMMIT, sha256 $SHA"
}

# The unit must start the server with -state, or a restart ends every game.
check_unit() {
  if ! "${SSH[@]}" "systemctl cat $UNIT 2>/dev/null | grep -q -- '-state '"; then
    die "$HOST: the $UNIT unit has no -state flag, so restarting would end every open game. Add it in the ops repo first."
  fi
}

# Copies the binary to /tmp/parallax-<commit> on the host and checks it
# arrived intact.
ship() {
  STAGE=/tmp/parallax-$COMMIT
  scp -q "$BIN" "$HOST:$STAGE"
  "${SSH[@]}" "echo '$SHA  $STAGE' | sha256sum -c --quiet" || die "$HOST: the uploaded binary doesn't match sha256 $SHA"
}

install_photon() {
  "${SSH[@]}" bash -s -- "$COMMIT" "$SHA" "$KEEP" <<'REMOTE'
set -euo pipefail
commit=$1 sha=$2 keep=$3
stage=/tmp/parallax-$commit
dir=/opt/parallax/releases/$commit
if [[ -f "$dir/parallax" ]] && echo "$sha  $dir/parallax" | sha256sum -c --quiet 2>/dev/null; then
  echo "photon: releases/$commit is already installed; switching to it"
else
  sudo install -d -m 0755 "$dir"
  sudo install -o root -g root -m 0755 "$stage" "$dir/parallax"
fi
rm -f "$stage"
sudo restorecon -R /opt/parallax
sudo ln -sfn "releases/$commit" /opt/parallax/current
sudo systemctl restart parallax
sleep 3
echo "photon: current -> $(readlink /opt/parallax/current)"
# Keep the newest few releases; never the live one.
cd /opt/parallax/releases
ls -1t | grep -vx "$commit" | tail -n +"$keep" | while read -r old; do
  echo "photon: pruning releases/$old"
  sudo rm -rf -- "/opt/parallax/releases/$old"
done
REMOTE
}

install_yoyo() {
  "${SSH[@]}" bash -s -- "$COMMIT" <<'REMOTE'
set -euo pipefail
stage=/tmp/parallax-$1
if sudo cmp -s "$stage" /usr/local/bin/parallax; then
  echo "yoyo already runs this build; restarting anyway"
else
  sudo cp -p /usr/local/bin/parallax /usr/local/bin/parallax.prev 2>/dev/null || true
  sudo install -o root -g root -m 755 "$stage" /usr/local/bin/parallax
fi
rm -f "$stage"
sudo systemctl restart parallax
REMOTE
}

rollback_photon() {
  "${SSH[@]}" bash -s <<'REMOTE'
set -euo pipefail
cd /opt/parallax
live=$(basename "$(readlink current)")
prev=$(ls -1t releases | grep -vx "$live" | head -1)
[[ -n "$prev" ]] || { echo "no earlier release to roll back to" >&2; exit 1; }
# The unit passes -state; builds before e847c8e don't know it and won't start.
if ! "releases/$prev/parallax" -h 2>&1 | grep -q -- '-state'; then
  echo "releases/$prev predates -state, which the unit passes, so it wouldn't start. Rolling back that far needs the unit changed too (the AWS session's part)." >&2
  exit 1
fi
echo "photon: $live -> $prev"
sudo ln -sfn "releases/$prev" current.new
sudo mv -T current.new current
sudo systemctl restart parallax
REMOTE
}

rollback_yoyo() {
  "${SSH[@]}" bash -s <<'REMOTE'
set -euo pipefail
[[ -f /usr/local/bin/parallax.prev ]] || { echo "no /usr/local/bin/parallax.prev to roll back to" >&2; exit 1; }
# The unit passes -state; builds before e847c8e don't know it and won't start.
if ! /usr/local/bin/parallax.prev -h 2>&1 | grep -q -- '-state'; then
  echo "parallax.prev predates -state, which the unit passes, so it wouldn't start. Rolling back that far needs the unit changed too (the Homelab session's part)." >&2
  exit 1
fi
sudo mv /usr/local/bin/parallax /usr/local/bin/parallax.rolledback
sudo mv /usr/local/bin/parallax.prev /usr/local/bin/parallax
sudo mv /usr/local/bin/parallax.rolledback /usr/local/bin/parallax.prev
sudo systemctl restart parallax
echo "yoyo: swapped in the previous build (the rolled-back one is now parallax.prev)"
REMOTE
}

# Checks: problems are printed and counted; nothing here changes anything.
checks() {
  local bad=0
  for _ in $(seq 1 30); do
    "${SSH[@]}" "curl -fs -m 3 $LOCAL/healthz >/dev/null" && break
    sleep 1
  done
  "${SSH[@]}" "curl -fs -m 3 $LOCAL/healthz >/dev/null" || { echo "PROBLEM: $HOST /healthz fails"; bad=1; }
  "${SSH[@]}" "systemctl is-active -q $UNIT" || { echo "PROBLEM: $HOST $UNIT is not active"; bad=1; }
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$URL/healthz" || true)
  [[ "$code" == 200 ]] || { echo "PROBLEM: $URL/healthz returns $code"; bad=1; }
  code=$(curl -s -o /dev/null -w '%{http_code} %{content_type}' -m 10 "$URL/og.jpg" || true)
  [[ "$code" == "200 image/jpeg" ]] || { echo "PROBLEM: $URL/og.jpg returns $code"; bad=1; }
  code=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$URL/api/rooms/ZZZZ/replay" || true)
  [[ "$code" == 404 ]] || { echo "PROBLEM: $URL/api/rooms/ZZZZ/replay returns $code, expected 404"; bad=1; }
  # What the server said since it started: rooms reopened, the model, errors.
  "${SSH[@]}" "sudo journalctl -u $UNIT --since '-2min' --no-pager -o cat" |
    grep -E 'reopened saved rooms|System 1 model|level=ERROR|listening' | tail -5 | sed "s/^/   $HOST: /" || true
  if ((bad)); then
    echo "$HOST: checks failed. Roll back with: deploy/deploy.sh $HOST --rollback"
    return 1
  fi
  say "$HOST: all checks pass"
}

status=0
if [[ "$mode" == deploy ]]; then
  build_binary
fi
for h in "${hosts[@]}"; do
  setup "$h"
  case "$mode" in
    deploy)
      check_unit
      say "Deploying $COMMIT to $h"
      ship
      "install_$h"
      ;;
    rollback)
      say "Rolling back $h"
      "rollback_$h"
      ;;
  esac
  checks || status=1
done
exit $status
