#!/usr/bin/env python3
"""Provisioning regressions; stdlib only and no network."""

import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class ProvisionToolsTest(unittest.TestCase):
    def test_owned_node_compiler_and_locked_closure(self):
        package = json.loads((ROOT / "tools/node/package.json").read_text())
        lock = json.loads((ROOT / "tools/node/package-lock.json").read_text())
        self.assertNotIn("typescript", package["dependencies"])
        self.assertNotIn("dependency-cruiser", package["dependencies"])
        self.assertIn("@ast-grep/cli", package["dependencies"])
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
        self.assertNotIn("ARCHFIT_TOOL_PYTHON", dockerfile)
        self.assertIn("/opt/archfit/analyzers/node/node_modules/.bin", dockerfile)
        for line in dockerfile.splitlines():
            if line.startswith("FROM "):
                self.assertRegex(line, r"@sha256:[0-9a-f]{64}")

    def test_manifest_keeps_runtime_pins_and_removes_scip(self):
        manifest = json.loads((ROOT / "tools/manifest.json").read_text())
        self.assertEqual(manifest["versions"]["python"], "3.14.9")
        self.assertEqual(manifest["versions"]["rust"], "1.99.1")
        self.assertNotIn("scip-go", manifest["versions"])
        self.assertNotIn("python", manifest)
        self.assertNotIn("@sourcegraph/scip-typescript", manifest["node"])
        self.assertNotIn("@sourcegraph/scip-python", manifest["node"])
        for artifacts in manifest["artifacts"].values():
            self.assertNotIn("rust-analyzer", artifacts)


if __name__ == "__main__":
    unittest.main()
