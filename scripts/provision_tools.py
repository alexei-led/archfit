#!/usr/bin/env python3
"""Install the fixed Linux engine SDKs, verifying official release checksums.

Analyzer dependency installation is in provision-analyzers.sh. This is build/CI
provisioning, never a runtime resolver. Both supported architectures are explicit.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def check_locks(root: Path) -> None:
    manifest = json.loads((root / "tools/manifest.json").read_text())
    package = json.loads((root / "tools/node/package.json").read_text())
    lock = json.loads((root / "tools/node/package-lock.json").read_text())
    if package["dependencies"] != manifest["node"]:
        raise ValueError("npm dependencies differ from tools/manifest.json")
    if lock["packages"][""]["dependencies"] != manifest["node"]:
        raise ValueError("npm lock differs from tools/manifest.json")
    for name, version in manifest["node"].items():
        if lock["packages"][f"node_modules/{name}"]["version"] != version:
            raise ValueError(f"npm locked tool differs from manifest: {name}")
    for name, item in lock["packages"].items():
        if name and (
            not item.get("integrity", "").startswith("sha512-")
            or not item.get("resolved", "").startswith("https://registry.npmjs.org/")
        ):
            raise ValueError(f"npm package lacks registry integrity: {name}")
    inputs = (root / "tools/python/requirements.in").read_text().splitlines()
    if inputs != [f"{name}=={version}" for name, version in manifest["python"].items()]:
        raise ValueError("Python inputs differ from tools/manifest.json")
    lock = (root / "tools/python/requirements.lock").read_text()
    entries = lock.replace("\\\n", "").splitlines()
    for entry in entries:
        if entry and not entry.startswith(("#", " ")) and "--hash=sha256:" not in entry:
            raise ValueError(f"Python package lacks hashes: {entry}")
    for name, version in manifest["python"].items():
        if f"{name}=={version} " not in lock:
            raise ValueError(f"Python lock differs from manifest: {name}")
    go = (root / "tools/scip-go/go.mod").read_text()
    if f"github.com/scip-code/scip-go v{manifest['versions']['scip-go']}" not in go:
        raise ValueError("SCIP Go lock differs from manifest")
    # The Dockerfile pin is checked wherever the file is part of the tree (host,
    # CI). It is deliberately not copied into the image build context: an edit to
    # it must not invalidate the toolchain layer.
    path = root / "Dockerfile"
    if not path.is_file():
        return
    for line in path.read_text().splitlines():
        if line.startswith("FROM ") and manifest["base_image"] not in line:
            raise ValueError(f"base image differs from manifest: {line}")


def fetch_verified(artifact: dict[str, str], destination: Path) -> None:
    with (
        urllib.request.urlopen(artifact["url"], timeout=120) as response,
        destination.open("wb") as output,
    ):
        shutil.copyfileobj(response, output)
    with destination.open("rb") as source:
        digest = hashlib.file_digest(source, "sha256").hexdigest()
    if digest != artifact["sha256"]:
        raise ValueError(f"checksum mismatch for {artifact['url']}: {digest}")


def install_sdks(root: Path, prefix: Path, arch: str) -> None:
    manifest = json.loads((root / "tools/manifest.json").read_text())
    artifacts = manifest["artifacts"][arch]
    prefix.mkdir(parents=True, exist_ok=True)
    (prefix / "bin").mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="archfit-sdk-") as temporary:
        temp = Path(temporary)
        for name in (
            "go",
            "node",
            "uv",
            "rustc",
            "cargo",
            "rust-std",
            "rust-src",
            "rust-analyzer",
        ):
            artifact = artifacts[name]
            archive = temp / artifact["url"].rsplit("/", 1)[1]
            print(f"verify/install {name}: {artifact['url']}", flush=True)
            fetch_verified(artifact, archive)
            unpacked = temp / name
            unpacked.mkdir()
            with tarfile.open(archive) as tar:
                tar.extractall(unpacked, filter="data")
            source = next(unpacked.iterdir())
            if name in ("go", "node"):
                shutil.move(source, prefix / name)
            elif name == "uv":
                for binary in ("uv", "uvx"):
                    shutil.copy2(source / binary, prefix / "bin" / binary)
            else:
                subprocess.run(
                    [
                        "sh",
                        str(source / "install.sh"),
                        f"--prefix={prefix / 'rust'}",
                        "--disable-ldconfig",
                    ],
                    check=True,
                )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("check", "sdks"))
    parser.add_argument("--arch", choices=("amd64", "arm64"))
    parser.add_argument("--prefix", type=Path, default=Path("/opt/archfit"))
    args = parser.parse_args()
    check_locks(ROOT)
    if args.command == "sdks":
        if args.arch is None:
            parser.error("sdks requires --arch (the target, not build platform)")
        install_sdks(ROOT, args.prefix, args.arch)


if __name__ == "__main__":
    main()
