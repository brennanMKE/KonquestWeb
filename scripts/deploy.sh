#!/usr/bin/env bash
# shellcheck disable=SC2329 # install_/rollback_ functions are called by name
# Deploy the Parallax game server to a host, or roll one back.
#
#   scripts/deploy.sh <target>             # build main's HEAD, ship it, restart, check
#   scripts/deploy.sh all                  # every target in DEPLOY_TARGETS
#   scripts/deploy.sh <target> --rollback  # back to the build before this one
#   scripts/deploy.sh <target> --check     # only run the checks against what's live
#   scripts/deploy.sh <target> --no-build  # ship bin/parallax-linux-<arch> as it is
#
# Targets, their ssh aliases, URLs, ports and install layouts come from .env
# at the repo root (gitignored). Copy .env.example to .env and fill it in;
# nothing about the hosts lives in this script or the repo.
#
# It only replaces the binary. Units, reverse proxies, VPNs, the System 1
# model and its key belong to the hosts' own ops repos and are never touched
# here; the script only checks that the unit starts the server with -state,
# so open games survive the restart (the server saves its rooms on SIGTERM
# and reopens them at start).
#
# Layouts (TARGET_LAYOUT):
#   releases  TARGET_DIR/releases/<commit>/parallax, root:root 0755, with
#             TARGET_DIR/current linking to the live one; the live release and
#             the 3 before it are kept for --rollback. SELinux labels are
#             restored when restorecon exists.
#   single    TARGET_BIN is the binary; the one before it is kept as
#             TARGET_BIN.prev for --rollback.
#
# Hosts run Linux (TARGET_ARCH arm64, the default, or amd64) and are reached
# over ssh with sudo on the far side. DEPLOY.md covers setting one up.
#
# Never touched: units and drop-ins, port registries, the key's env file
# (never read), proxy and VPN config, and the saved rooms (they hold seat
# tokens).
set -euo pipefail
cd "$(dirname "$0")/.."

die() { printf 'deploy: %s\n' "$*" >&2; exit 1; }
say() { printf '==> %s\n' "$*"; }

usage() { sed -n '3,9p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

ENV_FILE="${DEPLOY_ENV:-.env}"
[[ -f "$ENV_FILE" ]] || die "no $ENV_FILE: copy .env.example to .env and fill in the hosts"
set -a
# shellcheck source=/dev/null
. "$ENV_FILE"
set +a
[[ -n "${DEPLOY_TARGETS:-}" ]] || die "$ENV_FILE sets no DEPLOY_TARGETS"
read -r -a all_targets <<<"$DEPLOY_TARGETS"

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
if [[ "$target" == all ]]; then
  hosts=("${all_targets[@]}")
elif [[ " ${all_targets[*]} " == *" $target "* ]]; then
  hosts=("$target")
else
  die "unknown target $target; $ENV_FILE lists: ${all_targets[*]}"
fi

KEEP=4 # releases layout: the live release and the 3 before it

# setting NAME KEY: the value of <NAME>_<KEY> from .env, e.g. ALPHA_URL.
setting() {
  local var
  var="$(printf '%s_%s' "$1" "$2" | tr '[:lower:]-' '[:upper:]_')"
  printf '%s' "${!var:-}"
}

# Per-target settings, all from .env.
setup() {
  local t=$1
  HOST=$(setting "$t" SSH)
  URL=$(setting "$t" URL)
  PORT=$(setting "$t" PORT)
  LAYOUT=$(setting "$t" LAYOUT)
  UNIT=$(setting "$t" UNIT)
  UNIT=${UNIT:-parallax}
  DIR=$(setting "$t" DIR)
  ARCH=$(setting "$t" ARCH)
  ARCH=${ARCH:-arm64}
  [[ "$ARCH" == arm64 || "$ARCH" == amd64 ]] || die "$t: _ARCH must be arm64 or amd64"
  TBIN=$(setting "$t" BIN)
  [[ -n "$HOST" && -n "$URL" && -n "$PORT" ]] || die "$t: set its _SSH, _URL and _PORT in $ENV_FILE"
  case "$LAYOUT" in
    releases) [[ -n "$DIR" ]] || die "$t: the releases layout needs _DIR" ;;
    single) [[ -n "$TBIN" ]] || die "$t: the single layout needs _BIN" ;;
    *) die "$t: _LAYOUT must be releases or single" ;;
  esac
  LOCAL=http://127.0.0.1:$PORT
  SSH=(ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST")
}

# Builds (once per run and architecture) and fingerprints the binary.
built=""
build_binary() {
  BIN=bin/parallax-linux-$ARCH
  if ((build)) && [[ " $built " != *" $ARCH "* ]]; then
    [[ -z "$(git status --porcelain --untracked-files=no)" ]] || die "uncommitted changes; commit them or use --no-build"
    [[ "$(git rev-parse --abbrev-ref HEAD)" == main ]] || die "not on main"
    git fetch -q origin main
    [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main isn't the same as origin/main; push or pull first"
    say "Building $BIN"
    make -s "linux-$ARCH"
    built="$built $ARCH"
  fi
  [[ -f "$BIN" ]] || die "no $BIN; run without --no-build"
  COMMIT=$(git rev-parse --short=7 HEAD)
  SHA=$(shasum -a 256 "$BIN" | cut -d' ' -f1)
  say "Parallax $COMMIT, sha256 $SHA"
}

# The unit must start the server with -state, or a restart ends every game.
check_unit() {
  if ! "${SSH[@]}" "systemctl cat $UNIT 2>/dev/null | grep -q -- '-state '"; then
    die "$1: the $UNIT unit has no -state flag, so restarting would end every open game. Add it in the host's ops repo first."
  fi
}

# Copies the binary to /tmp/parallax-<commit> on the host and checks it
# arrived intact.
ship() {
  local stage=/tmp/parallax-$COMMIT
  scp -q "$BIN" "$HOST:$stage"
  "${SSH[@]}" "echo '$SHA  $stage' | sha256sum -c --quiet" || die "$1: the uploaded binary doesn't match sha256 $SHA"
}

install_releases() {
  "${SSH[@]}" bash -s -- "$COMMIT" "$SHA" "$KEEP" "$DIR" "$UNIT" <<'REMOTE'
set -euo pipefail
commit=$1 sha=$2 keep=$3 base=$4 unit=$5
stage=/tmp/parallax-$commit
dir=$base/releases/$commit
if [[ -f "$dir/parallax" ]] && echo "$sha  $dir/parallax" | sha256sum -c --quiet 2>/dev/null; then
  echo "releases/$commit is already installed; switching to it"
else
  sudo install -d -m 0755 "$dir"
  sudo install -o root -g root -m 0755 "$stage" "$dir/parallax"
fi
rm -f "$stage"
if command -v restorecon >/dev/null; then sudo restorecon -R "$base"; fi
sudo ln -sfn "releases/$commit" "$base/current"
sudo systemctl restart "$unit"
sleep 3
echo "current -> $(readlink "$base/current")"
# Keep the newest few releases; never the live one.
cd "$base/releases"
ls -1t | grep -vx "$commit" | tail -n +"$keep" | while read -r old; do
  echo "pruning releases/$old"
  sudo rm -rf -- "${base:?}/releases/$old"
done
REMOTE
}

install_single() {
  "${SSH[@]}" bash -s -- "$COMMIT" "$TBIN" "$UNIT" <<'REMOTE'
set -euo pipefail
stage=/tmp/parallax-$1 bin=$2 unit=$3
if sudo cmp -s "$stage" "$bin"; then
  echo "already running this build; restarting anyway"
else
  sudo cp -p "$bin" "$bin.prev" 2>/dev/null || true
  sudo install -o root -g root -m 755 "$stage" "$bin"
fi
rm -f "$stage"
sudo systemctl restart "$unit"
REMOTE
}

# A rollback target must accept -state, which the unit passes; builds before
# e847c8e don't, so they wouldn't start. Rolling back that far needs the unit
# changed too, in the host's ops repo.
rollback_releases() {
  "${SSH[@]}" bash -s -- "$DIR" "$UNIT" <<'REMOTE'
set -euo pipefail
base=$1 unit=$2
cd "$base"
live=$(basename "$(readlink current)")
prev=$(ls -1t releases | grep -vx "$live" | head -1)
[[ -n "$prev" ]] || { echo "no earlier release to roll back to" >&2; exit 1; }
if ! "releases/$prev/parallax" -h 2>&1 | grep -q -- '-state'; then
  echo "releases/$prev predates -state, which the unit passes, so it wouldn't start" >&2
  exit 1
fi
echo "$live -> $prev"
sudo ln -sfn "releases/$prev" current.new
sudo mv -T current.new current
sudo systemctl restart "$unit"
REMOTE
}

rollback_single() {
  "${SSH[@]}" bash -s -- "$TBIN" "$UNIT" <<'REMOTE'
set -euo pipefail
bin=$1 unit=$2
[[ -f "$bin.prev" ]] || { echo "no $bin.prev to roll back to" >&2; exit 1; }
if ! "$bin.prev" -h 2>&1 | grep -q -- '-state'; then
  echo "$bin.prev predates -state, which the unit passes, so it wouldn't start" >&2
  exit 1
fi
sudo mv "$bin" "$bin.rolledback"
sudo mv "$bin.prev" "$bin"
sudo mv "$bin.rolledback" "$bin.prev"
sudo systemctl restart "$unit"
echo "swapped in the previous build (the rolled-back one is now $bin.prev)"
REMOTE
}

# Checks: problems are printed and counted; nothing here changes anything.
checks() {
  local t=$1 bad=0 code
  for _ in $(seq 1 30); do
    "${SSH[@]}" "curl -fs -m 3 $LOCAL/healthz >/dev/null" && break
    sleep 1
  done
  "${SSH[@]}" "curl -fs -m 3 $LOCAL/healthz >/dev/null" || { echo "PROBLEM: $t: /healthz fails on the host"; bad=1; }
  "${SSH[@]}" "systemctl is-active -q $UNIT" || { echo "PROBLEM: $t: $UNIT is not active"; bad=1; }
  code=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$URL/healthz" || true)
  [[ "$code" == 200 ]] || { echo "PROBLEM: $t: /healthz returns $code"; bad=1; }
  code=$(curl -s -o /dev/null -w '%{http_code} %{content_type}' -m 10 "$URL/og.jpg" || true)
  [[ "$code" == "200 image/jpeg" ]] || { echo "PROBLEM: $t: /og.jpg returns $code"; bad=1; }
  code=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$URL/api/demo" || true)
  [[ "$code" == 200 ]] || { echo "PROBLEM: $t: /api/demo returns $code"; bad=1; }
  code=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$URL/api/rooms/ZZZZ/replay" || true)
  [[ "$code" == 404 ]] || { echo "PROBLEM: $t: an unknown room's /replay returns $code, expected 404"; bad=1; }
  # What the server said lately: rooms reopened, the model's state, errors.
  # URLs are left out of what's printed.
  "${SSH[@]}" "sudo journalctl -u $UNIT --since '-2min' --no-pager -o cat" |
    grep -E 'reopened saved rooms|System 1 model|level=ERROR|listening' | tail -5 |
    sed -E -e 's/ (url|dir)=[^ ]+//g' -e "s/^/   $t: /" || true
  if ((bad)); then
    echo "$t: checks failed. Roll back with: scripts/deploy.sh $t --rollback"
    return 1
  fi
  say "$t: all checks pass"
}

status=0
for t in "${hosts[@]}"; do
  setup "$t"
  case "$mode" in
    deploy)
      build_binary
      check_unit "$t"
      say "Deploying $COMMIT to $t"
      ship "$t"
      "install_$LAYOUT"
      ;;
    rollback)
      say "Rolling back $t"
      "rollback_$LAYOUT"
      ;;
  esac
  checks "$t" || status=1
done
exit $status
