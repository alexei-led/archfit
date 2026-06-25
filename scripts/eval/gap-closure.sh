#!/usr/bin/env bash
# gap-closure.sh — run archfit full + delta across all 6 eval repos and write results.
#
# Writes to reports/eval/gap-closure/<repo>/{full,delta}.{json,md}
# Skips repos whose toolchain or config is absent; logs each skip reason.
#
# Usage (from repo root):
#   ./scripts/eval/gap-closure.sh
#
# The archfit binary is built with `make build` before running this script.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ARCHFIT="${REPO_ROOT}/.bin/archfit"
WORKSPACE="${HOME}/Workspace"
OUTPUT_BASE="${REPO_ROOT}/reports/eval/gap-closure"
REPOS=(archfit pumba codegraph ccgram yazi herdr)

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
		if ! command -v depcruise &>/dev/null && ! npx depcruise --version &>/dev/null 2>&1; then
			echo "SKIP: dependency-cruiser (depcruise) not found — required for TS repo '${repo}'"
			return 1
		fi
	elif find "${repo_dir}" -maxdepth 2 -name "*.py" -quit 2>/dev/null | grep -q .; then
		# Python repos: grimp is required.
		if ! python3 -c "import grimp" &>/dev/null 2>&1; then
			echo "SKIP: grimp not installed — required for Python repo '${repo}'"
			return 1
		fi
	fi

	return 0
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
		SKIPPED=$((SKIPPED + 1))
		continue
	fi

	# Check config exists.
	if [[ ! -f "${config_file}" ]]; then
		echo "SKIP: .archfit.yaml missing at ${config_file}"
		echo ""
		SKIPPED=$((SKIPPED + 1))
		continue
	fi

	# Check toolchain.
	skip_reason=$(check_toolchain "${repo}" 2>&1)
	if [[ -n "${skip_reason}" ]]; then
		echo "${skip_reason}"
		echo ""
		SKIPPED=$((SKIPPED + 1))
		continue
	fi

	# Create output directory.
	mkdir -p "${out_dir}"

	# Run full check.
	echo "  full check..."
	if "${ARCHFIT}" check \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--full \
		--format json,md \
		--output "${out_dir}/full" \
		2>"${out_dir}/full.stderr"; then
		echo "  full: OK → ${out_dir}/full.{json,md}"
	else
		exit_code=$?
		echo "  full: archfit exited ${exit_code} (gate violations expected — output still written)"
	fi

	# Run delta check (smoke: HEAD~1 base).
	echo "  delta check (HEAD~1)..."
	if (cd "${repo_dir}" && "${ARCHFIT}" check \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--base HEAD~1 \
		--format json,md \
		--output "${out_dir}/delta" \
		2>"${out_dir}/delta.stderr"); then
		echo "  delta: OK → ${out_dir}/delta.{json,md}"
	else
		exit_code=$?
		echo "  delta: archfit exited ${exit_code} (expected for gate violations or shallow history)"
	fi

	echo ""
	PROCESSED=$((PROCESSED + 1))
done

echo "==> Done: ${PROCESSED} processed, ${SKIPPED} skipped"
