#!/usr/bin/env bash
# gap-closure.sh — run archfit full + delta across all 6 eval repos and write results.
#
# Writes to docs/archived/reports/eval/gap-closure/<repo>/{full,delta}.{json,md}
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
OUTPUT_BASE="${REPO_ROOT}/docs/archived/reports/eval/gap-closure"
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

	# Create output directory.
	mkdir -p "${out_dir}"

	# Run full analyze — JSON (coverage generator reads this).
	# Exit-code contract (stable): 0 = clean, 1 = gate violations, 3 = config/tool error.
	# Only 0 and 1 produce valid JSON output. Exit 3 (crash) leaves empty/invalid JSON;
	# remove it so the coverage generator does not silently process stale or empty output.
	echo "  full analyze (json)..."
	full_json_exit=0
	"${ARCHFIT}" analyze --gate \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--full \
		--format json \
		>"${out_dir}/full.json" \
		2>"${out_dir}/full.stderr" ||
		full_json_exit=$?
	if [[ "${full_json_exit}" -eq 0 ]]; then
		echo "  full json: OK → ${out_dir}/full.json"
	elif [[ "${full_json_exit}" -eq 1 ]]; then
		echo "  full json: exit 1 (gate violations) — output still written → ${out_dir}/full.json"
	else
		echo "  full json: UNEXPECTED exit ${full_json_exit} (config/tool error) — removing output to prevent stale JSON"
		rm -f "${out_dir}/full.json"
		cat "${out_dir}/full.stderr" >&2
	fi

	# Run full analyze — Markdown (human-readable report).
	echo "  full analyze (md)..."
	if "${ARCHFIT}" analyze --gate \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--full \
		--format md \
		>"${out_dir}/full.md" \
		2>>"${out_dir}/full.stderr"; then
		echo "  full md: OK → ${out_dir}/full.md"
	else
		exit_code=$?
		echo "  full md: archfit exited ${exit_code} (gate violations expected — output still written)"
	fi

	# Run delta analyze (smoke: HEAD~1 base) — JSON.
	echo "  delta analyze (HEAD~1, json)..."
	if (cd "${repo_dir}" && "${ARCHFIT}" analyze --gate \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--base HEAD~1 \
		--format json \
		>"${out_dir}/delta.json" \
		2>"${out_dir}/delta.stderr"); then
		echo "  delta json: OK → ${out_dir}/delta.json"
	else
		exit_code=$?
		echo "  delta json: archfit exited ${exit_code} (expected for gate violations or shallow history)"
	fi

	# Run delta analyze — Markdown.
	echo "  delta analyze (HEAD~1, md)..."
	if (cd "${repo_dir}" && "${ARCHFIT}" analyze --gate \
		--config "${config_file}" \
		--root "${repo_dir}" \
		--base HEAD~1 \
		--format md \
		>"${out_dir}/delta.md" \
		2>>"${out_dir}/delta.stderr"); then
		echo "  delta md: OK → ${out_dir}/delta.md"
	else
		exit_code=$?
		echo "  delta md: archfit exited ${exit_code} (expected for gate violations or shallow history)"
	fi

	echo ""
	PROCESSED=$((PROCESSED + 1))
done

echo "==> Done: ${PROCESSED} processed, ${SKIPPED} skipped"
