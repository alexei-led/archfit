#!/usr/bin/env python3
"""Fast, stdlib-only tests for the real-language acceptance harness."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import language_contracts as lc


class TestHarnessOwnership(unittest.TestCase):
    def test_refuses_to_wipe_unowned_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "output"
            path.mkdir()
            with self.assertRaises(RuntimeError):
                lc.prepare_output_dir(path)

    def test_recreates_owned_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "output"
            path.mkdir()
            (path / lc.OUTPUT_MARKER).write_text("", encoding="utf-8")
            (path / "old").write_text("old", encoding="utf-8")
            lc.prepare_output_dir(path)
            self.assertTrue((path / lc.OUTPUT_MARKER).is_file())
            self.assertFalse((path / "old").exists())


class TestFixtureContracts(unittest.TestCase):
    def test_failed_waived_baseline_capture_fails_the_contract(self):
        result = lc.ContractResult("fixture")
        command = lc.CommandResult(
            "waived-baseline",
            ["archfit", "baseline"],
            "/tmp/fixture",
            3,
            "",
            "config error",
        )
        self.assertFalse(lc.baseline_capture_succeeded(result, command))
        self.assertTrue(
            any(
                "waived baseline capture succeeds" in failure
                for failure in result.failures
            )
        )

    def test_five_fixture_languages_are_explicit(self):
        self.assertEqual(
            [spec.name for spec in lc.FIXTURES],
            ["go", "typescript", "javascript", "python", "rust"],
        )

    def test_every_fixture_uses_schema_v2_and_disables_optional_depth_tools(self):
        for spec in lc.FIXTURES:
            with self.subTest(spec=spec.name):
                config = lc.config_text(spec)
                self.assertRegex(config, r"(?m)^version: 2$")
                self.assertIn("scip:\n    enabled: false", config)
                self.assertIn("clones:\n    enabled: false", config)
                self.assertIn("cargo_modules:\n    enabled: false", config)

    def test_tool_environment_pins_rust_without_overriding_caller(self):
        original = lc.os.environ.get("RUSTUP_TOOLCHAIN")
        try:
            lc.os.environ.pop("RUSTUP_TOOLCHAIN", None)
            self.assertEqual(
                lc.command_environment()["RUSTUP_TOOLCHAIN"], lc.RUST_TOOLCHAIN
            )
            lc.os.environ["RUSTUP_TOOLCHAIN"] = "nightly"
            self.assertEqual(lc.command_environment()["RUSTUP_TOOLCHAIN"], "nightly")
        finally:
            if original is None:
                lc.os.environ.pop("RUSTUP_TOOLCHAIN", None)
            else:
                lc.os.environ["RUSTUP_TOOLCHAIN"] = original


if __name__ == "__main__":
    unittest.main()
