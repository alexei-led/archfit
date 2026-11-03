#!/usr/bin/env bash
set -euo pipefail

# Git hooks export repository-local paths that would redirect fixture Git commands.
git_variables=$(git rev-parse --local-env-vars)
while IFS= read -r name; do
	unset "$name"
done <<<"$git_variables"

exec make test
