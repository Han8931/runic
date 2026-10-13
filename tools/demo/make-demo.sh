#!/usr/bin/env bash
# Record the README demo GIF.
#
#   tools/demo/make-demo.sh [steps-file] [output.gif]
#
# Builds ru, sets up a throwaway daemon + sample repo, drives the terminal from
# the steps file, and renders the recording with agg (https://docs.asciinema.org/manual/agg/).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STEPS="$(realpath "${1:-$REPO_ROOT/tools/demo/demo.steps}")"
OUT="$(realpath -m "${2:-$REPO_ROOT/assets/runic_demo.gif}")"
CAST="${CAST:-/tmp/runic-demo.cast}"

AGG="${AGG:-$(command -v agg || echo "$HOME/.cargo/bin/agg")}"
if [ ! -x "$AGG" ]; then
	echo "agg not found; install with: cargo install --git https://github.com/asciinema/agg" >&2
	exit 1
fi

# 140x34 at agg's default 16px font renders to 1363x784.
export DEMO_COLS="${DEMO_COLS:-140}"
export DEMO_ROWS="${DEMO_ROWS:-34}"

BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT
echo "building ru..."
(cd "$REPO_ROOT" && go build -o "$BIN_DIR/ru" ./cmd/ru/)

# shellcheck source=/dev/null
source "$REPO_ROOT/tools/demo/sandbox.sh"
export PATH="$BIN_DIR:$PATH"
export PS1='$ '

cd "$DEMO_WORKDIR"
echo "recording $STEPS..."
python3 "$REPO_ROOT/tools/demo/record.py" "$STEPS" "$CAST"

# Stop the throwaway daemon. It hosts the demo's shell panes, so it can outlive
# the shutdown request; match on the temp build path so only it is targeted.
"$BIN_DIR/ru" -K >/dev/null 2>&1 || true
sleep 1
pkill -f "^$BIN_DIR/ru --server$" >/dev/null 2>&1 || true

mkdir -p "$(dirname "$OUT")"
echo "rendering $OUT..."
"$AGG" \
	--theme dracula \
	--font-size 16 \
	--idle-time-limit 1.5 \
	--last-frame-duration 3 \
	--fps-cap 20 \
	"$CAST" "$OUT"

ls -lh "$OUT"
