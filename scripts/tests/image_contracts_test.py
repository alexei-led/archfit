#!/usr/bin/env python3
"""Fast checks for failure disclosure and real-source image fixture construction."""

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image


class ImageContractsTest(unittest.TestCase):
    def test_empty_graph_is_not_a_forbidden_dependency_positive_control(self):
        self.assertFalse(image.forbidden({"findings": []}, "new"))
        self.assertEqual(image.metric({}, "complexity", "function_loc_max"), 0)

    def test_fixture_builds_real_clone_sources_and_keeps_depth_opt_in(self):
        with tempfile.TemporaryDirectory() as temporary:
            for spec in image.lc.FIXTURES:
                with self.subTest(language=spec.name):
                    path = Path(temporary) / spec.name
                    image.prepare_fixture(spec, path)
                    self.assertIn("clone_left:", (path / ".archfit.yaml").read_text())
                    self.assertIn(
                        "clones:\n    enabled: false",
                        (path / ".archfit.yaml").read_text(),
                    )
                    self.assertIn(
                        "clones:\n    enabled: true", (path / "deep.yaml").read_text()
                    )
                    self.assertGreater(
                        len(image.clone_source(spec.name).splitlines()), 40
                    )
                    for name, original in spec.files.items():
                        if Path(name).suffix in {".go", ".ts", ".js", ".py", ".rs"}:
                            self.assertEqual((path / name).read_text(), original)
                    if spec.name == "javascript":
                        config = json.loads((path / "tsconfig.json").read_text())
                        self.assertTrue(config["compilerOptions"]["allowJs"])
                    self.assertTrue(image.tree_hash(path))

    def test_helper_statuses_fail_closed(self):
        legacy = {
            "path": "Legacy/Legacy.csproj",
            "tfm": "net8.0",
            "status": "partial",
            "reason": "reference pack Microsoft.NETCore.App.Ref 8.0 is not installed",
        }
        broken = {
            "path": "Broken/Broken.csproj",
            "tfm": "",
            "status": "failed",
            "reason": "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable",
        }
        cases = {
            "expected": ([legacy, broken], 0),
            "empty": ([], 2),
            "legacy complete": ([{**legacy, "status": "complete"}, broken], 1),
            "legacy pack unnamed": ([{**legacy, "reason": "CS0518"}, broken], 1),
            "broken partial": ([legacy, {**broken, "status": "partial"}], 1),
            "broken sdk unnamed": ([legacy, {**broken, "reason": "MSB4236"}], 1),
        }
        for name, (projects, count) in cases.items():
            with self.subTest(name):
                self.assertEqual(
                    len(image.helper_status_failures({"projects": projects})), count
                )

    def test_helper_identity_pins_the_image_sdk(self):
        sdk = image.lc.TOOL_MANIFEST["versions"]["dotnet"]
        good = f"archfit-roslyn 1.0.0; Microsoft.CodeAnalysis 5.6.0; SDK {sdk}"
        self.assertTrue(image.DOTNET_IDENTITY.fullmatch(good))
        self.assertFalse(image.DOTNET_IDENTITY.fullmatch(good.replace(sdk, "9.0.100")))

    def test_container_failure_cannot_be_reported_as_pass(self):
        with tempfile.TemporaryDirectory() as temporary:
            contract = image.ImageContract("local:contract", "colima", Path(temporary))
            with patch.object(
                image.subprocess,
                "run",
                return_value=subprocess.CompletedProcess([], 125, "", "mount failed"),
            ):
                output = contract.run("failed", "27182:27182", "true")
            self.assertFalse(contract.assertions[0]["passed"])
            command = json.loads((output / "command.json").read_text())
            self.assertEqual(command[:3], ["docker", "--context", "colima"])
            self.assertIn("--read-only", command)
            self.assertIn("27182:27182", command)
            self.assertEqual((output / "stderr.log").read_text(), "mount failed")


if __name__ == "__main__":
    unittest.main()
