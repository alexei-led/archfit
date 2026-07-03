#!/bin/sh
# corpus-attrib.sh — coupling_balance attribution snapshot over the four
# Wave-4 attribution repos (archfit Go, ccgram Python, herdr Rust,
# storybook TS). Prints one markdown table row per repo:
# repo | score | band | scored | abstained | external — for the before/after
# tables in docs/plans/20260702-wave4-book-strength-distance.md.
#
# Usage:
#   make corpus-attrib          # builds .bin/archfit first
#   sh scripts/corpus-attrib.sh
#
# Environment:
#   ATTRIB_REPOS_DIR  directory containing the corpus checkouts
#                     (default: $HOME/Workspace; see docs/test-corpus.md)
#   ATTRIB_OUT        directory for the raw per-repo JSON
#                     (default: a fresh mktemp -d, path echoed at the end)
#
# A missing repo checkout is skipped with a warning, not an error — the
# table is for local attribution, not CI. Requires jq.
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$REPO_ROOT/.bin/archfit"
ATTRIB_REPOS_DIR="${ATTRIB_REPOS_DIR:-${HOME}/Workspace}"
ATTRIB_OUT="${ATTRIB_OUT:-$(mktemp -d)}"

if [ ! -x "$BIN" ]; then
	echo "error: binary not found at $BIN — run 'make build' first" >&2
	exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
	echo "error: jq is required" >&2
	exit 1
fi

echo "| repo | score | band | scored | abstained | external |"
echo "|---|---|---|---|---|---|"

# run <name> <config> [extra analyze flags...]
# analyze exits 1 (gate) / 2 (warn) with valid JSON — only treat empty/invalid
# JSON as failure.
run() {
	name="$1"
	cfg="$2"
	shift 2
	json="$ATTRIB_OUT/$name.json"
	"$BIN" analyze --full --json --config "$cfg" "$@" >"$json" 2>"$ATTRIB_OUT/$name.stderr"
	if ! jq -e .score "$json" >/dev/null 2>&1; then
		echo "| $name | run failed — see $ATTRIB_OUT/$name.stderr | | | | |"
		return
	fi
	jq -r '
	  (.coupling_balance // (.score.dimensions[]? | select(.name == "coupling_balance"))) as $cb |
	  (.classified_edges // {}) as $ce |
	  "| \(input_filename | split("/")[-1] | rtrimstr(".json")) | \($cb.value // "n/a") | \($cb.band // "n/a") | \($ce.scored // 0) | \($ce.abstained // 0) | \($ce.external // 0) |"
	' "$json"
}

run archfit "$REPO_ROOT/.archfit.yaml" --root "$REPO_ROOT"

for r in ccgram herdr; do
	if [ -f "$ATTRIB_REPOS_DIR/$r/.archfit.yaml" ]; then
		run "$r" "$ATTRIB_REPOS_DIR/$r/.archfit.yaml"
	else
		echo "warn: $ATTRIB_REPOS_DIR/$r/.archfit.yaml not found — skipping $r" >&2
	fi
done

# storybook: saved eval config (repo has no .archfit.yaml); analysis root is
# the code/ subtree per the config header.
SB_CFG="$REPO_ROOT/reports/eval-2026-06-30-corpus/configs/storybook.yaml"
if [ -d "$ATTRIB_REPOS_DIR/storybook/code" ]; then
	run storybook "$SB_CFG" --root "$ATTRIB_REPOS_DIR/storybook/code"
else
	echo "warn: $ATTRIB_REPOS_DIR/storybook/code not found — skipping storybook" >&2
fi

echo "raw JSON: $ATTRIB_OUT" >&2
