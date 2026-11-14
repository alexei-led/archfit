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
# so a cache hit only verifies the restored tree, reinstates the pinned
# interpreter and exports. The interpreter is not in the cached tree — it lives
# in the runner's uv install directory, which the cache does not cover — so a
# hit that skipped it left every later `uv` step with no Python of the pinned
# version (2026-11-14).
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
        test -x "$prefix/dotnet/dotnet" &&
        test -x "$prefix/rust/bin/rustc" &&
        test -x "$prefix/analyzers/node/node_modules/.bin/sg"
}

export_toolchain() {
    printf '%s\n' \
        "$prefix/analyzers/node/node_modules/.bin" \
        "$prefix/bin" \
        "$prefix/dotnet" \
        "$prefix/go/bin" \
        "$prefix/node/bin" \
        "$prefix/rust/bin" >>"$GITHUB_PATH"
    # No GOCACHE/GOPATH here: the workflow caches the Go defaults that setup-go
    # already restores, and pointing them at /tmp rebuilt every package per run.
    printf '%s\n' \
        GOTOOLCHAIN=local \
        CARGO_HOME=/tmp/archfit-cargo \
        "RUST_SRC_PATH=$prefix/rust/lib/rustlib/src/rust/library" \
        "DOTNET_ROOT=$prefix/dotnet" \
        UV_PYTHON_DOWNLOADS=never >>"$GITHUB_ENV"
}

ci_arch() {
    case $(uname -m) in
    x86_64) printf amd64 ;;
    aarch64) printf arm64 ;;
    *)
        echo "unsupported CI architecture" >&2
        exit 1
        ;;
    esac
}

# The pinned interpreter runs the build-time provisioning scripts.
ensure_python() {
    local version python
    version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["versions"]["python"])' "$root/tools/manifest.json")
    uv python install "$version" || return 1
    python=$(uv python find "$version") || return 1
    printf '%s\n' "$python"
}

install_toolchain() {
    local python
    # Assigning from a command substitution does not propagate its status in every
    # shell, so failures are checked explicitly.
    python=$(ensure_python) || return 1
    "$python" "$root/scripts/provision_tools.py" sdks --arch "$(ci_arch)" --prefix "$prefix" || return 1
    bash "$root/scripts/provision-analyzers.sh" "$prefix" "$python"
}

case $mode in
auto)
    # A restored toolchain is verified before it is trusted and rebuilt when it
    # does not verify. A verified tree still needs the pinned interpreter: the
    # cache cannot carry it, so a hit installs it before anything runs `uv`.
    if [ "${ARCHFIT_TOOLCHAIN_CACHE_HIT:-}" = true ] && toolchain_ready; then
        ensure_python >/dev/null || exit 1
        export_toolchain
    else
        install_toolchain || exit 1
        export_toolchain
    fi
    ;;
install)
    install_toolchain || exit 1
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

