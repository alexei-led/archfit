#!/bin/sh
# bench-gate.sh — cold vs warm fact-cache timing for the archfit gate.
# Runs `archfit check` twice: once after clearing the fact cache (cold) and
# once with it populated (warm), and prints both wall times.
# Reported numbers only — machine-dependent, never a CI assert.
#
# Usage:
#   make bench-gate                                    # this repo
#   BENCH_CONFIG=<cfg> BENCH_ROOT=<dir> sh scripts/bench-gate.sh   # another repo
#
# ARCHFIT_BIN overrides the binary path (test seam; default: .bin/archfit).
#
# The fact cache lives beside the config (<configDir>/.archfit-cache/facts);
# the cold run deletes ONLY that facts/ subdir — the LLM response cache and
# --base worktrees are untouched. Requires python3 (same as `make test`).
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="${ARCHFIT_BIN:-$REPO_ROOT/.bin/archfit}"
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

# Time pass (0) and fail (1). Unknown, usage and tool errors (2) are not a completed check.
run_timed() {
	start=$(python3 -c 'import time; print(time.time())')
	"$BIN" check -C "$ROOT" --policy "$CFG" >/dev/null 2>&1
	rc=$?
	end=$(python3 -c 'import time; print(time.time())')
	case "$rc" in
	0 | 1) ;;
	*)
		echo "error: archfit check exited $rc (parser, config, or tool error) — benchmark aborted" >&2
		return 2
		;;
	esac
	python3 -c "print('%.1f' % ($end - $start))"
}

rm -rf "$FACTS_DIR"
cold=$(run_timed) || exit 2
warm=$(run_timed) || exit 2
echo "config: $CFG"
echo "root:   $ROOT"
echo "cold:   ${cold}s (fact cache cleared)"
echo "warm:   ${warm}s"
