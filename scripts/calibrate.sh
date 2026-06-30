#!/bin/sh
# calibrate.sh — run scorer calibration across one or more repos.
#
# Usage:
#   bash scripts/calibrate.sh [repo-path ...]
#
# When no arguments are given, defaults to archfit itself plus two optional
# external repos under CALIBRATE_REPOS_DIR (skipped gracefully when absent):
#   $CALIBRATE_REPOS_DIR/redwoodjs-redwood
#   $CALIBRATE_REPOS_DIR/saleor-saleor
#
# Environment:
#   CALIBRATE_REPOS_DIR  directory that may contain external repo clones
#                        (default: $HOME/tmp/calibration-repos)
#   CALIBRATE_OUTPUT     output JSON file (default: calibration-report.json)
#
# The binary must already be built; `make calibrate` does `make build` first.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$REPO_ROOT/.bin/calibrate"
CALIBRATE_REPOS_DIR="${CALIBRATE_REPOS_DIR:-${HOME}/tmp/calibration-repos}"
CALIBRATE_OUTPUT="${CALIBRATE_OUTPUT:-calibration-report.json}"

if [ ! -x "$BIN" ]; then
	echo "error: binary not found at $BIN — run 'make build-calibrate' first" >&2
	exit 1
fi

# Collect repo paths: positional args or defaults.
if [ $# -gt 0 ]; then
	repos="$*"
else
	repos="$REPO_ROOT $CALIBRATE_REPOS_DIR/redwoodjs-redwood $CALIBRATE_REPOS_DIR/saleor-saleor"
fi

# Build --repo flags for all paths (pass all to one invocation so the binary
# writes a single combined JSON report).
repo_flags=""
for repo in $repos; do
	repo_flags="$repo_flags --repo $repo"
done

# shellcheck disable=SC2086
"$BIN" $repo_flags --output "$CALIBRATE_OUTPUT"
