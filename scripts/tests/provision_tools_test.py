#!/usr/bin/env python3
"""Provisioning regressions; stdlib only and no network."""

import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class ProvisionToolsTest(unittest.TestCase):
    def test_owned_node_compiler_and_locked_closure(self):
        package = json.loads((ROOT / "tools/node/package.json").read_text())
        lock = json.loads((ROOT / "tools/node/package-lock.json").read_text())
        self.assertEqual(package["dependencies"]["typescript"], "6.0.3")
        self.assertEqual(package["dependencies"]["dependency-cruiser"], "18.7.0")
        self.assertEqual(lock["packages"][""]["dependencies"], package["dependencies"])
        for name, item in lock["packages"].items():
            if name:
                self.assertRegex(item["integrity"], r"^sha512-")
                self.assertTrue(
                    item["resolved"].startswith("https://registry.npmjs.org/")
                )

    def test_final_image_has_no_bootstrap_or_buildplatform_sdk(self):
        dockerfile = (ROOT / "Dockerfile").read_text()
        self.assertNotIn("nodesource", dockerfile.lower())
        self.assertNotIn("npm install -g", dockerfile)
        self.assertNotIn("COPY --from=go-builder /usr/local/go", dockerfile)
        self.assertIn("ARCHFIT_TOOL_PYTHON=/opt/archfit/python/bin/python", dockerfile)
        self.assertIn("/opt/archfit/analyzers/node/node_modules/.bin", dockerfile)
        for line in dockerfile.splitlines():
            if line.startswith("FROM "):
                self.assertRegex(line, r"@sha256:[0-9a-f]{64}")

    def test_manifest_consumers_and_hash_locked_python(self):
        manifest = json.loads((ROOT / "tools/manifest.json").read_text())
        self.assertEqual(manifest["versions"]["python"], "3.14.9")
        self.assertEqual(manifest["versions"]["rust"], "1.99.1")
        self.assertEqual(
            manifest["python"],
            {"grimp": "3.17", "protobuf": "7.36.2", "grpcio-tools": "1.84.1"},
        )
        lock = (ROOT / "tools/python/requirements.lock").read_text()
        for package, version in manifest["python"].items():
            self.assertIn(f"{package}=={version}", lock)
        for entry in re.split(r"\n(?=[a-z])", lock):
            if "==" in entry:
                self.assertIn("--hash=sha256:", entry)


if __name__ == "__main__":
    unittest.main()
