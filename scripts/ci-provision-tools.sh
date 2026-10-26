#!/usr/bin/env bash
# CI-only bootstrap of the locked analyzer toolchain. Not a host installer.
#
# The image builds this tree once per release; CI restores the same tree from the
# Actions cache keyed on the lock files the provisioners read. Two things follow:
#
#   * the prefix must be runner-owned, because the cache restores as the runner
#     user and a root-owned prefix under /opt can be neither saved nor replaced;
#   * every path the job needs is derived from the prefix, so the cached and the
#     freshly built tree publish identical PATH and environment entries.
#
# `install` is slow (it compiles cargo-modules from source) and `export` is free,
# so a cache hit only verifies the restored tree and exports it.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
prefix=${ARCHFIT_PREFIX:?ARCHFIT_PREFIX must name a runner-owned tool prefix}
mode=${1:-auto}
if [ -z "${GITHUB_PATH:-}" ] || [ -z "${GITHUB_ENV:-}" ]; then
    echo "GITHUB_PATH and GITHUB_ENV are required" >&2
    exit 2
fi

toolchain_ready() {
    test -x "$prefix/bin/cargo-modules" &&
        test -x "$prefix/bin/scip-go" &&
        test -x "$prefix/rust/bin/rustc" &&
        test -x "$prefix/analyzers/node/node_modules/.bin/sg" &&
        "$prefix/python/bin/python" -I -c 'import grimp, grpc_tools' >/dev/null 2>&1
}

export_toolchain() {
    local python=$prefix/python/bin/python base_bin
    # The venv interpreter symlinks into uv's own tree. The image keeps a base
    # interpreter on PATH and the venv behind ARCHFIT_TOOL_PYTHON, so CI does too.
    base_bin=$(dirname -- "$(readlink -f -- "$python" 2>/dev/null || printf '%s' "$python")")
    printf '%s\n' \
        "$prefix/analyzers/node/node_modules/.bin" \
        "$prefix/bin" \
        "$prefix/go/bin" \
        "$prefix/node/bin" \
        "$prefix/rust/bin" \
        "$base_bin" >>"$GITHUB_PATH"
    # No GOCACHE/GOPATH here: the workflow caches the Go defaults that setup-go
    # already restores, and pointing them at /tmp rebuilt every package per run.
    printf '%s\n' \
        "ARCHFIT_TOOL_PYTHON=$python" \
        GOTOOLCHAIN=local \
        CARGO_HOME=/tmp/archfit-cargo \
        "RUST_SRC_PATH=$prefix/rust/lib/rustlib/src/rust/library" \
        UV_PYTHON_DOWNLOADS=never >>"$GITHUB_ENV"
}

install_toolchain() {
    local arch python_version python
    case $(uname -m) in
    x86_64) arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *)
        echo "unsupported CI architecture" >&2
        exit 1
        ;;
    esac
    python_version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["versions"]["python"])' "$root/tools/manifest.json")
    uv python install "$python_version"
    python=$(uv python find "$python_version")
    "$python" "$root/scripts/provision_tools.py" sdks --arch "$arch" --prefix "$prefix"
    bash "$root/scripts/provision-analyzers.sh" "$prefix" "$python"
}

case $mode in
auto)
    # The venv interpreter is a symlink into uv's own tree, so a restored toolchain
    # is verified before it is trusted and rebuilt when it does not verify.
    if [ "${ARCHFIT_TOOLCHAIN_CACHE_HIT:-}" = true ] && toolchain_ready; then
        export_toolchain
    else
        install_toolchain
        export_toolchain
    fi
    ;;
install)
    install_toolchain
    export_toolchain
    ;;
export)
    export_toolchain
    ;;
*)
    echo "usage: ci-provision-tools.sh [auto|install|export]" >&2
    exit 2
    ;;
esac

