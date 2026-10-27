#!/usr/bin/env python3
"""Checksum enforcement and manifest drift regressions."""

import hashlib
import io
import json
import shutil
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import provision_tools as provision


class ProvisionVerificationTest(unittest.TestCase):
    def test_download_checksum_mismatch_fails_before_install(self):
        with (
            tempfile.TemporaryDirectory() as temporary,
            patch.object(
                provision.urllib.request, "urlopen", return_value=io.BytesIO(b"wrong")
            ),
            self.assertRaisesRegex(ValueError, "checksum mismatch"),
        ):
            provision.fetch_verified(
                {"url": "https://official.invalid/sdk", "sha256": "0" * 64},
                Path(temporary) / "sdk.tar",
            )

    def test_verified_download_is_preserved(self):
        content = b"verified SDK bytes"
        with tempfile.TemporaryDirectory() as temporary:
            destination = Path(temporary) / "sdk.tar"
            with patch.object(
                provision.urllib.request, "urlopen", return_value=io.BytesIO(content)
            ):
                provision.fetch_verified(
                    {
                        "url": "https://official.invalid/sdk",
                        "sha256": hashlib.sha256(content).hexdigest(),
                    },
                    destination,
                )
            self.assertEqual(destination.read_bytes(), content)

    def test_dotnet_flat_archive_lands_at_prefix_dotnet(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            archive = base / "dotnet-sdk.tar.gz"
            content = base / "content"
            (content / "sdk/10.0.402").mkdir(parents=True)
            (content / "dotnet").write_text("host")
            (content / "sdk/10.0.402/marker").write_text("sdk")
            with tarfile.open(archive, "w:gz") as tar:
                for entry in sorted(content.iterdir()):
                    tar.add(entry, arcname=entry.name)
            artifact = {
                "url": "https://official.invalid/dotnet-sdk.tar.gz",
                "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
            }
            prefix = base / "prefix"
            prefix.mkdir()
            work = base / "work"
            work.mkdir()
            with patch.object(
                provision.urllib.request,
                "urlopen",
                side_effect=lambda *_, **__: io.BytesIO(archive.read_bytes()),
            ):
                provision.install_dotnet(artifact, work, prefix)
            self.assertEqual((prefix / "dotnet/dotnet").read_text(), "host")
            self.assertEqual((prefix / "dotnet/sdk/10.0.402/marker").read_text(), "sdk")

    def test_dotnet_reinstall_replaces_the_existing_directory(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            archive = base / "dotnet-sdk.tar.gz"
            content = base / "content"
            content.mkdir()
            (content / "dotnet").write_text("new")
            with tarfile.open(archive, "w:gz") as tar:
                tar.add(content / "dotnet", arcname="dotnet")
            artifact = {
                "url": "https://official.invalid/dotnet-sdk.tar.gz",
                "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
            }
            prefix = base / "prefix"
            (prefix / "dotnet/sdk/9.0.0").mkdir(parents=True)
            (prefix / "dotnet/dotnet").write_text("old")
            work = base / "work"
            work.mkdir()
            with patch.object(
                provision.urllib.request,
                "urlopen",
                side_effect=lambda *_, **__: io.BytesIO(archive.read_bytes()),
            ):
                provision.install_dotnet(artifact, work, prefix)
            self.assertEqual((prefix / "dotnet/dotnet").read_text(), "new")
            self.assertFalse((prefix / "dotnet/dotnet").is_dir())
            self.assertFalse((prefix / "dotnet/sdk").exists())

    def test_dotnet_checksum_mismatch_installs_nothing(self):
        with (
            tempfile.TemporaryDirectory() as temporary,
            patch.object(
                provision.urllib.request, "urlopen", return_value=io.BytesIO(b"wrong")
            ),
            self.assertRaisesRegex(ValueError, "checksum mismatch"),
        ):
            base = Path(temporary)
            try:
                provision.install_dotnet(
                    {"url": "https://official.invalid/d.tar.gz", "sha256": "0" * 64},
                    base,
                    base / "prefix",
                )
            finally:
                self.assertFalse((base / "prefix/dotnet").exists())

    def test_unknown_architecture_is_rejected(self):
        for arch in ("riscv64", "x86_64", ""):
            with (
                self.subTest(arch=arch),
                tempfile.TemporaryDirectory() as temporary,
                self.assertRaisesRegex(ValueError, "unsupported architecture"),
            ):
                provision.install_sdks(provision.ROOT, Path(temporary), arch)

    def test_manifest_pins_dotnet_for_both_architectures(self):
        manifest = json.loads((provision.ROOT / "tools/manifest.json").read_text())
        version = manifest["versions"]["dotnet"]
        for arch, rid in (("amd64", "x64"), ("arm64", "arm64")):
            artifact = manifest["artifacts"][arch]["dotnet"]
            self.assertEqual(
                artifact["url"],
                f"https://builds.dotnet.microsoft.com/dotnet/Sdk/{version}/"
                f"dotnet-sdk-{version}-linux-{rid}.tar.gz",
            )
            self.assertRegex(artifact["sha256"], r"^[0-9a-f]{64}$")
        self.assertIn("dotnet", manifest["checksum_sources"])

    def test_manifest_drift_rejects_target_compiler_substitution(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            shutil.copytree(provision.ROOT / "tools", root / "tools")
            shutil.copy2(provision.ROOT / "Dockerfile", root / "Dockerfile")
            package_path = root / "tools/node/package.json"
            package = json.loads(package_path.read_text())
            package["dependencies"]["typescript"] = "7.0.2"
            package_path.write_text(json.dumps(package))
            with self.assertRaisesRegex(ValueError, "npm dependencies differ"):
                provision.check_locks(root)


if __name__ == "__main__":
    unittest.main()
