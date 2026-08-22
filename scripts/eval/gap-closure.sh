#!/usr/bin/env bash
# gap-closure.sh — run archfit check full + delta across all 6 eval repos and write results.
#
# Writes to docs/archived/reports/eval/gap-closure/<repo>/{full,delta}.{json,md}
# Skips repos whose toolchain or config is absent; logs each skip reason.
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

# FAILED flips to 1 when any archfit run exits with a parser, config, or tool
# error (exit 3); the sweep finishes the remaining repos, then exits 3.
FAILED=0

# Toolchain checks per language.
# Returns 0 if all required tools are present, non-zero otherwise.
check_toolchain() {
	local repo="$1"

	# sg (ast-grep) is always required for syntax analysis.
	if ! command -v sg &>/dev/null; then
		echo "SKIP: sg (ast-grep) not found in PATH"
		return 1
	fi

	# Detect language from config or directory heuristics.
	local repo_dir="${WORKSPACE}/${repo}"
	if [[ -f "${repo_dir}/go.mod" ]]; then
		# Go repos: no extra toolchain needed beyond sg.
		:
	elif [[ -f "${repo_dir}/Cargo.toml" ]]; then
		# Rust repos: cargo is required for module graph extraction.
		if ! command -v cargo &>/dev/null; then
			echo "SKIP: cargo not found — required for Rust repo '${repo}'"
			return 1
		fi
	elif [[ -f "${repo_dir}/package.json" ]]; then
		# TypeScript repos: dependency-cruiser is required.
		if ! command -v depcruise &>/dev/null && ! npx --no-install depcruise --version &>/dev/null 2>&1; then
			echo "SKIP: dependency-cruiser (depcruise) not found — required for TS repo '${repo}'"
			return 1
		fi
	elif [[ -n "$(find "${repo_dir}" -maxdepth 2 -name "*.py" -print -quit 2>/dev/null)" ]]; then
		# Python repos: grimp is required. Accept either a direct install or uv (archfit
		# runs `uv run --with grimp` when uv is available, so uv-only repos are valid).
		if ! python3 -c "import grimp" &>/dev/null 2>&1; then
			if ! command -v uv &>/dev/null; then
				echo "SKIP: grimp not installed and uv not found — one is required for Python repo '${repo}'"
				return 1
			fi
		fi
	fi

	return 0
}

# run_check runs `archfit check`, keeping stdout in out_file and appending
# stderr to err_file. Exit-code contract (stable): 0 = pass, 1 = gate
# violations, 2 = warnings, 3 = parser/config/tool error. Exit 0, 1, and 2 all
# produce valid output; exit 3 leaves empty/invalid output, so it is removed —
# the coverage generator must never silently process stale or invalid output.
run_check() {
	local label="$1" out_file="$2" err_file="$3" workdir="$4"
	shift 4
	local rc=0
	# This call's stderr goes to its own file first, then is appended to the
	# shared log. Reading the shared log on the error branch would re-print every
	# earlier call's stderr for the same repo.
	local this_err="${out_file}.stderr"
	(cd "${workdir}" && "${ARCHFIT}" check "$@" >"${out_file}" 2>"${this_err}") || rc=$?
	cat "${this_err}" >>"${err_file}"
	case "${rc}" in
	0)
		echo "  ${label}: OK → ${out_file}"
		;;
	1)
		echo "  ${label}: exit 1 (gate violations) — output still written → ${out_file}"
		;;
	2)
		echo "  ${label}: exit 2 (warnings) — output still written → ${out_file}"
		;;
	*)
		echo "  ${label}: exit ${rc} (parser, config, or tool error) — removing invalid output"
		rm -f "${out_file}"
		cat "${this_err}" >&2
		FAILED=1
		;;
	esac
	rm -f "${this_err}"
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

	# Check toolchain — capture non-zero exit without tripping set -e.
	skip_reason=""
	if ! skip_reason=$(check_toolchain "${repo}" 2>&1); then
		echo "${skip_reason}"
		echo ""
		rm -rf "${out_dir}"
		SKIPPED=$((SKIPPED + 1))
		continue
	fi

	# Create output directory; truncate per-mode stderr logs for this sweep.
	mkdir -p "${out_dir}"
	: >"${out_dir}/full.stderr"
	: >"${out_dir}/delta.stderr"

	# Run full check — JSON (coverage generator reads this).
	echo "  full check (json)..."
	run_check "full json" "${out_dir}/full.json" "${out_dir}/full.stderr" "${REPO_ROOT}" \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--format json

	# Run full check — Markdown (human-readable report).
	echo "  full check (md)..."
	run_check "full md" "${out_dir}/full.md" "${out_dir}/full.stderr" "${REPO_ROOT}" \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--format md

	# Run delta check (smoke: HEAD~1 base) — JSON. Runs from the repo dir so
	# the base ref resolves against that repo's history.
	echo "  delta check (HEAD~1, json)..."
	run_check "delta json" "${out_dir}/delta.json" "${out_dir}/delta.stderr" "${repo_dir}" \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--base HEAD~1 \
		--format json

	# Run delta check — Markdown.
	echo "  delta check (HEAD~1, md)..."
	run_check "delta md" "${out_dir}/delta.md" "${out_dir}/delta.stderr" "${repo_dir}" \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--base HEAD~1 \
		--format md

	echo ""
	PROCESSED=$((PROCESSED + 1))
done

echo "==> Done: ${PROCESSED} processed, ${SKIPPED} skipped"
if [[ "${FAILED}" -ne 0 ]]; then
	echo "==> ERROR: at least one archfit run failed with a parser, config, or tool error (exit 3)" >&2
	exit 3
fi
