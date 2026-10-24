#!/usr/bin/env bash
# Build-time only; the final image invokes preinstalled, owned tools directly.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
prefix=${1:-/opt/archfit}
python=${2:-python3}
"$python" "$root/scripts/provision_tools.py" check
export PATH="$prefix/go/bin:$prefix/node/bin:$prefix/rust/bin:$prefix/bin:$PATH"
export GOTOOLCHAIN=local GOWORK=off
export CARGO_HOME="${CARGO_HOME:-/tmp/archfit-cargo-build}"
mkdir -p "$prefix/analyzers/node" "$prefix/bin"
cp "$root/tools/node/package.json" "$root/tools/node/package-lock.json" "$prefix/analyzers/node/"
npm ci --prefix "$prefix/analyzers/node" --no-audit --no-fund
"$python" -m venv "$prefix/python"
uv pip sync --python "$prefix/python/bin/python" --require-hashes --only-binary :all: \
    "$root/tools/python/requirements.lock"
(cd "$root/tools/scip-go" && CGO_ENABLED=0 go build -mod=readonly -trimpath \
    -o "$prefix/bin/scip-go" github.com/scip-code/scip-go/cmd/scip-go)
modules_version=$("$python" -c 'import json,sys; print(json.load(open(sys.argv[1]))["versions"]["cargo-modules"])' "$root/tools/manifest.json")
cargo install --locked --version "$modules_version" --root "$prefix" cargo-modules
cp "$root/tools/manifest.json" "$prefix/manifest.json"
# No tool update, compiler download or user-owned site package is needed at runtime.
chmod -R a+rX "$prefix"
