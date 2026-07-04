#!/bin/sh
# bench-gate.sh — cold vs warm fact-cache timing for the archfit gate.
# Runs `archfit analyze --gate --full` twice: once after clearing the fact
# cache (cold) and once with it populated (warm), and prints both wall times.
# Reported numbers only — machine-dependent, never a CI assert.
#
# Usage:
#   make bench-gate                                    # this repo
#   BENCH_CONFIG=<cfg> BENCH_ROOT=<dir> sh scripts/bench-gate.sh   # another repo
#
# The fact cache lives beside the config (<configDir>/.archfit-cache/facts);
# the cold run deletes ONLY that facts/ subdir — the LLM response cache and
# --base worktrees are untouched. Requires python3 (same as `make test`).
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$REPO_ROOT/.bin/archfit"
CFG="${BENCH_CONFIG:-$REPO_ROOT/.archfit.yaml}"
ROOT="${BENCH_ROOT:-$REPO_ROOT}"

if [ ! -x "$BIN" ]; then
	echo "error: binary not found at $BIN — run 'make build' first" >&2
	exit 1
fi
if [ ! -f "$CFG" ]; then
	echo "error: config not found at $CFG" >&2
	exit 1
fi

FACTS_DIR="$(cd "$(dirname "$CFG")" && pwd)/.archfit-cache/facts"

# run_timed prints elapsed wall seconds; analyze exits 1 (gate) / 2 (warn) by
# design — the bench never fails on the verdict.
run_timed() {
	start=$(python3 -c 'import time; print(time.time())')
	"$BIN" analyze --gate --full --config "$CFG" --root "$ROOT" >/dev/null 2>&1 || true
	end=$(python3 -c 'import time; print(time.time())')
	python3 -c "print('%.1f' % ($end - $start))"
}

rm -rf "$FACTS_DIR"
cold=$(run_timed)
warm=$(run_timed)
echo "config: $CFG"
echo "root:   $ROOT"
echo "cold:   ${cold}s (fact cache cleared)"
echo "warm:   ${warm}s"
