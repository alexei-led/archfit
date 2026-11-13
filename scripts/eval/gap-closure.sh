#!/usr/bin/env bash
# gap-closure.sh — collect repository reports and change checks across eval repos.
#
# Writes report.{json,txt}, check.{json,txt} and their stderr logs per repository.
# Skips absent repositories or policies and discloses every skip.
#
# Usage (from repo root):
#   ./scripts/eval/gap-closure.sh
#
# The archfit binary is built with `make build` before running this script.
#
# Test seams (defaults preserve the normal command):
#   ARCHFIT_BIN            override the archfit binary path
#   ARCHFIT_EVAL_WORKSPACE override the workspace directory holding the repos
#   ARCHFIT_EVAL_OUTPUT    override the output base directory
#   ARCHFIT_EVAL_REPOS     space-separated repo list override

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ARCHFIT="${ARCHFIT_BIN:-${REPO_ROOT}/.bin/archfit}"
WORKSPACE="${ARCHFIT_EVAL_WORKSPACE:-${HOME}/Workspace}"
OUTPUT_BASE="${ARCHFIT_EVAL_OUTPUT:-${REPO_ROOT}/docs/archived/reports/eval/gap-closure}"
if [[ -n "${ARCHFIT_EVAL_REPOS:-}" ]]; then
	read -r -a REPOS <<<"${ARCHFIT_EVAL_REPOS}"
else
	REPOS=(archfit pumba codegraph ccgram yazi herdr)
fi

# Unknown and tool errors fail the sweep; a failing change is still a valid result.
FAILED=0

run_record() {
	local command="$1" out_file="$2" repo_dir="$3"
	shift 3
	local rc=0
	"${ARCHFIT}" "${command}" -C "${repo_dir}" "$@" >"${out_file}" 2>"${out_file}.stderr" || rc=$?
	case "${command}:${rc}" in
	report:0 | check:0 | check:1) ;;
	*)
		echo "${command}: exit ${rc}, removing invalid output ${out_file}" >&2
		cat "${out_file}.stderr" >&2
		rm -f "${out_file}"
		FAILED=1
		;;
	esac
}

if [[ ! -x "${ARCHFIT}" ]]; then
	echo "ERROR: archfit binary not found at ${ARCHFIT}" >&2
	echo "       Run 'make build' from the repo root first." >&2
	exit 1
fi

echo "==> gap-closure sweep: archfit ${ARCHFIT}"
echo "    workspace: ${WORKSPACE}"
echo "    output:    ${OUTPUT_BASE}"
echo ""

SKIPPED=0
PROCESSED=0

for repo in "${REPOS[@]}"; do
	repo_dir="${WORKSPACE}/${repo}"
	config_file="${repo_dir}/.archfit.yaml"
	out_dir="${OUTPUT_BASE}/${repo}"

	echo "--- ${repo} ---"

	# Check repo directory exists.
	if [[ ! -d "${repo_dir}" ]]; then
		echo "SKIP: ${repo_dir} does not exist"
		echo ""
		# Remove stale output so a prior run's full.json is never silently reused.
		rm -rf "${out_dir}"
		SKIPPED=$((SKIPPED + 1))
		continue
	fi

	# Check config exists.
	if [[ ! -f "${config_file}" ]]; then
		echo "SKIP: .archfit.yaml missing at ${config_file}"
		echo ""
		rm -rf "${out_dir}"
		SKIPPED=$((SKIPPED + 1))
		continue
	fi

	mkdir -p "${out_dir}"
	run_record report "${out_dir}/report.json" "${repo_dir}" --format json
	run_record report "${out_dir}/report.txt" "${repo_dir}" --format text
	run_record check "${out_dir}/check.json" "${repo_dir}" --base HEAD~1 --format json
	run_record check "${out_dir}/check.txt" "${repo_dir}" --base HEAD~1 --format text

	echo ""
	PROCESSED=$((PROCESSED + 1))
done

echo "==> Done: ${PROCESSED} processed, ${SKIPPED} skipped"
if [[ "${FAILED}" -ne 0 ]]; then
	echo "==> ERROR: at least one archfit run failed with a parser, config, or tool error (exit 2)" >&2
	exit 2
fi
