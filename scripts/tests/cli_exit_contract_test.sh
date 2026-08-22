#!/usr/bin/env bash
# cli_exit_contract_test.sh — regression test for the archfit exit-code
# contract in scripts/bench-gate.sh and scripts/eval/gap-closure.sh.
#
# Uses a fake archfit binary (no real analysis) to cover all four exit codes:
#   0 = pass, 1 = gate violations, 2 = warnings  → valid results, output kept
#   3 = parser/config/tool error                 → invalid, output removed,
#                                                  script exits 3
#
# Usage: bash scripts/tests/cli_exit_contract_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/cli_exit_contract.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT

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

# make_fake_archfit <exit_code>: writes an executable fake archfit that emits
# canned stdout/stderr and exits with the given code. Prints its path.
make_fake_archfit() {
	local rc="$1" path="${TMP}/bin/archfit-rc$1"
	mkdir -p "${TMP}/bin"
	cat >"${path}" <<EOF
#!/bin/sh
echo '{"fake":true,"rc":${rc}}'
echo "fake archfit rc=${rc}" >&2
exit ${rc}
EOF
	chmod +x "${path}"
	echo "${path}"
}

# ---------------------------------------------------------------------------
# bench-gate.sh: exit 0/1/2 produce timings; exit 3 aborts the benchmark.
# ---------------------------------------------------------------------------

BENCH_DIR="${TMP}/benchrepo"
mkdir -p "${BENCH_DIR}"
: >"${BENCH_DIR}/.archfit.yaml"

for rc in 0 1 2; do
	bin="$(make_fake_archfit "${rc}")"
	if out=$(ARCHFIT_BIN="${bin}" BENCH_CONFIG="${BENCH_DIR}/.archfit.yaml" BENCH_ROOT="${BENCH_DIR}" \
		sh "${REPO_ROOT}/scripts/bench-gate.sh" 2>/dev/null); then
		case "${out}" in
		*"cold:"*"warm:"*) ok "bench-gate: archfit exit ${rc} yields timings" ;;
		*) fail "bench-gate: archfit exit ${rc} — timing lines missing in output: ${out}" ;;
		esac
	else
		fail "bench-gate: archfit exit ${rc} should benchmark successfully, script exited $?"
	fi
done

bin="$(make_fake_archfit 3)"
brc=0
ARCHFIT_BIN="${bin}" BENCH_CONFIG="${BENCH_DIR}/.archfit.yaml" BENCH_ROOT="${BENCH_DIR}" \
	sh "${REPO_ROOT}/scripts/bench-gate.sh" >/dev/null 2>&1 || brc=$?
if [[ "${brc}" -eq 3 ]]; then
	ok "bench-gate: archfit exit 3 aborts the benchmark with exit 3"
else
	fail "bench-gate: archfit exit 3 should abort with exit 3, got ${brc}"
fi

# ---------------------------------------------------------------------------
# gap-closure.sh: exit 0/1/2 keep JSON and Markdown output; exit 3 removes
# invalid output and propagates exit 3.
# ---------------------------------------------------------------------------

WS="${TMP}/workspace"
FAKE_REPO="${WS}/fakerepo"
mkdir -p "${FAKE_REPO}"
: >"${FAKE_REPO}/go.mod"
: >"${FAKE_REPO}/.archfit.yaml"

# check_toolchain always requires sg (ast-grep) on PATH — fake it.
printf '#!/bin/sh\nexit 0\n' >"${TMP}/bin/sg"
chmod +x "${TMP}/bin/sg"

run_gap_closure() {
	local bin="$1" out_base="$2" rc=0
	PATH="${TMP}/bin:${PATH}" \
		ARCHFIT_BIN="${bin}" \
		ARCHFIT_EVAL_WORKSPACE="${WS}" \
		ARCHFIT_EVAL_OUTPUT="${out_base}" \
		ARCHFIT_EVAL_REPOS="fakerepo" \
		bash "${REPO_ROOT}/scripts/eval/gap-closure.sh" >/dev/null 2>&1 || rc=$?
	echo "${rc}"
}

for rc in 0 1 2; do
	bin="$(make_fake_archfit "${rc}")"
	out_base="${TMP}/out-rc${rc}"
	grc="$(run_gap_closure "${bin}" "${out_base}")"
	if [[ "${grc}" -eq 0 ]]; then
		ok "gap-closure: archfit exit ${rc} keeps the sweep at exit 0"
	else
		fail "gap-closure: archfit exit ${rc} should keep the sweep at exit 0, got ${grc}"
	fi
	for f in full.json full.md delta.json delta.md; do
		if [[ -s "${out_base}/fakerepo/${f}" ]]; then
			ok "gap-closure: archfit exit ${rc} keeps ${f}"
		else
			fail "gap-closure: archfit exit ${rc} should keep ${f}, but it is missing or empty"
		fi
	done
done

bin="$(make_fake_archfit 3)"
out_base="${TMP}/out-rc3"
grc="$(run_gap_closure "${bin}" "${out_base}")"
if [[ "${grc}" -eq 3 ]]; then
	ok "gap-closure: archfit exit 3 propagates as sweep exit 3"
else
	fail "gap-closure: archfit exit 3 should propagate as sweep exit 3, got ${grc}"
fi
for f in full.json full.md delta.json delta.md; do
	if [[ -e "${out_base}/fakerepo/${f}" ]]; then
		fail "gap-closure: archfit exit 3 should remove ${f}, but it exists"
	else
		ok "gap-closure: archfit exit 3 removes ${f}"
	fi
done

echo ""
echo "${PASS} passed, ${FAILURES} failed"
[[ "${FAILURES}" -eq 0 ]]
