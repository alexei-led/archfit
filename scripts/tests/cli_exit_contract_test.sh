#!/usr/bin/env bash
# Exercise 4.0 benchmark and sweep exit codes with a boundary fake.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/cli_exit_contract.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
mkdir -p "${TMP}/repo" "${TMP}/workspace/fakerepo"
: >"${TMP}/repo/.archfit.yaml"
: >"${TMP}/workspace/fakerepo/.archfit.yaml"

cat >"${TMP}/archfit" <<'FAKE'
#!/bin/sh
command="$1"
shift
case "$command" in check | report) ;; *) exit 99 ;; esac
for arg do
 case "$arg" in --config | --root | --json | --quiet | md | markdown) exit 98 ;; esac
done
echo '{"status":"fake"}'
rc="$FAKE_RC"
# A change failing a rule does not make the report fail.
if [ "$command" = report ] && [ "$rc" = 1 ]; then rc=0; fi
exit "$rc"
FAKE
chmod +x "${TMP}/archfit"

for rc in 0 1 2 3; do
	expected=0
	if [[ "${rc}" -ge 2 ]]; then expected=2; fi
	actual=0
	FAKE_RC="${rc}" ARCHFIT_BIN="${TMP}/archfit" BENCH_CONFIG="${TMP}/repo/.archfit.yaml" BENCH_ROOT="${TMP}/repo" \
		sh "${REPO_ROOT}/scripts/bench-gate.sh" >"${TMP}/bench.log" 2>&1 || actual=$?
	if [[ "${actual}" -ne "${expected}" ]]; then
		cat "${TMP}/bench.log" >&2
		echo "bench exit ${actual}, expected ${expected} for archfit ${rc}" >&2
		exit 1
	fi
	if [[ "${expected}" -eq 0 ]]; then
		grep -q 'cold:' "${TMP}/bench.log"
		grep -q 'warm:' "${TMP}/bench.log"
	elif grep -q 'warm:' "${TMP}/bench.log"; then
		echo 'unknown benchmark must not report a completed timing' >&2
		exit 1
	fi

	out="${TMP}/output-${rc}"
	actual=0
	FAKE_RC="${rc}" ARCHFIT_BIN="${TMP}/archfit" ARCHFIT_EVAL_WORKSPACE="${TMP}/workspace" \
		ARCHFIT_EVAL_OUTPUT="${out}" ARCHFIT_EVAL_REPOS=fakerepo \
		bash "${REPO_ROOT}/scripts/eval/gap-closure.sh" >"${TMP}/sweep.log" 2>&1 || actual=$?
	if [[ "${actual}" -ne "${expected}" ]]; then
		cat "${TMP}/sweep.log" >&2
		echo "sweep exit ${actual}, expected ${expected} for archfit ${rc}" >&2
		exit 1
	fi
	for name in report.json report.txt check.json check.txt; do
		path="${out}/fakerepo/${name}"
		if [[ "${expected}" -eq 0 ]]; then
			test -s "${path}"
		else
			test ! -e "${path}"
			test -f "${path}.stderr"
		fi
	done
	echo "ok: archfit ${rc}, benchmark/sweep ${expected}"
done
