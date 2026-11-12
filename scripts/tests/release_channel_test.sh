#!/usr/bin/env bash
# release_channel_test.sh — the tag rule every release job reads.
#
# .github/workflows/release.yaml runs scripts/release-channel.sh once, in the
# tools job, and guards each publishing step on its answer: only "final" tags
# the image "latest", makes the latest GitHub release, and updates Homebrew;
# "prerelease" makes a GitHub prerelease. Any other tag shape must stop the run.
#
# Usage: bash scripts/tests/release_channel_test.sh (run by make workflows)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHANNEL="${SCRIPT_DIR}/../release-channel.sh"

PASS=0
FAILURES=0

ok() {
	echo "ok: $*"
	PASS=$((PASS + 1))
}

fail() {
	echo "FAIL: $*" >&2
	FAILURES=$((FAILURES + 1))
}

# expect <want-exit> <want-stdout> <args...>
expect() {
	local want_rc="$1" want_out="$2" args="(no argument)" out rc=0
	shift 2
	if [[ "$#" -gt 0 ]]; then
		args="$(printf '%q ' "$@")"
	fi
	out="$(sh "$CHANNEL" "$@" 2>/dev/null)" || rc=$?
	if [[ "$rc" == "$want_rc" && "$out" == "$want_out" ]]; then
		ok "${args} -> exit ${rc} '${out}'"
	else
		fail "${args} -> exit ${rc} '${out}', want exit ${want_rc} '${want_out}'"
	fi
}

for tag in v4.0.0 v3.4.1 v10.20.30; do
	expect 0 final "$tag"
done

for tag in v4.0.0-rc.1 v4.0.0-rc.12; do
	expect 0 prerelease "$tag"
done

for tag in v4.0.0-rc v4.0.0-rc.1-x v4.0.0-rc.1.2 v4.0.0-beta.1 v4.0.0-rc1 v4.0 4.0.0 \
	latest v4 "" " v4.0.0" $'v4.0.0\nv4.0.0'; do
	expect 2 "" "$tag"
done
expect 2 ""
expect 2 "" v4.0.0 v4.0.1

echo "${PASS} passed, ${FAILURES} failed"
[[ "$FAILURES" -eq 0 ]]
