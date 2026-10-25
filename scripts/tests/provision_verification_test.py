#!/usr/bin/env python3
"""Checksum enforcement and manifest drift regressions."""

import hashlib
import io
import json
import shutil
import sys
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
