#!/usr/bin/env bash
# Linux native CI uses the same verified SDKs and analyzer locks as the image.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
case $(uname -m) in
x86_64) arch=amd64 ;;
aarch64) arch=arm64 ;;
*)
    echo "unsupported CI architecture" >&2
    exit 1
    ;;
esac
python=$(command -v python3)
sudo "$python" "$root/scripts/provision_tools.py" sdks --arch "$arch"
sudo bash "$root/scripts/provision-analyzers.sh" /opt/archfit "$python"
printf '%s\n' /opt/archfit/analyzers/node/node_modules/.bin /opt/archfit/bin \
    /opt/archfit/go/bin /opt/archfit/node/bin /opt/archfit/rust/bin >>"${GITHUB_PATH:?}"
printf '%s\n' ARCHFIT_TOOL_PYTHON=/opt/archfit/python/bin/python GOTOOLCHAIN=local \
    GOCACHE=/tmp/archfit-go-build GOPATH=/tmp/archfit-go CARGO_HOME=/tmp/archfit-cargo \
    RUST_SRC_PATH=/opt/archfit/rust/lib/rustlib/src/rust/library \
    UV_PYTHON_DOWNLOADS=never >>"${GITHUB_ENV:?}"
