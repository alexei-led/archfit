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


class TestToolManifest(unittest.TestCase):
    def test_fixture_compiler_is_exact_and_compatible(self):
        import json

        fixture = next(spec for spec in lc.FIXTURES if spec.name == "typescript")
        deps = json.loads(fixture.files["package.json"])["devDependencies"]
        self.assertEqual(deps["typescript"], "6.0.3")
        self.assertEqual(deps["typescript"], lc.TOOL_MANIFEST["node"]["typescript"])
        self.assertEqual(deps["dependency-cruiser"], "18.7.0")
        self.assertEqual(lc.RUST_TOOLCHAIN, "1.99.1")

    def test_javascript_is_indexable_for_deep_contracts(self):
        import json

        fixture = next(spec for spec in lc.FIXTURES if spec.name == "javascript")
        self.assertTrue(
            json.loads(fixture.files["tsconfig.json"])["compilerOptions"]["allowJs"]
        )


def helper_doc():
    def project(path, tfm, status, reason="", unresolved=0, files=()):
        return {
            "path": path,
            "tfm": tfm,
            "status": status,
            "reason": reason,
            "unresolved_symbols": unresolved,
            "files": [
                {
                    "path": source,
                    "generated": False,
                    "edges": [
                        {"to": to, "hint": hint, "data_hint": data}
                        for to, hint, data in edges
                    ],
                }
                for source, edges in files
            ],
        }

    return {
        "projects": [
            project("Domain/Domain.csproj", "net10.0", "complete"),
            project(
                "Application/Application.csproj",
                "net10.0",
                "complete",
                files=[
                    (
                        "Application/Contracts.cs",
                        [("Domain/Domain.cs", "model", "model")],
                    )
                ],
            ),
            project(
                "Infrastructure/Infrastructure.csproj",
                "net10.0",
                "complete",
                files=[
                    (
                        "Infrastructure/Repository.cs",
                        [
                            ("Application/Contracts.cs", "dto", "dto"),
                            ("Domain/Domain.cs", "intrusive", "model"),
                        ],
                    )
                ],
            ),
            project(
                "Legacy/Legacy.csproj",
                "net10.0",
                "complete",
                files=[
                    ("Legacy/Legacy.cs", [("Domain/Domain.cs", "functional", "model")])
                ],
            ),
            project(
                "Legacy/Legacy.csproj",
                "net8.0",
                "partial",
                "31 unresolved symbols; reference pack Microsoft.NETCore.App.Ref 8.0 is not installed",
                31,
            ),
            project(
                "Broken/Broken.csproj",
                "",
                "failed",
                "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable",
            ),
            project(
                "Tests/Tests.csproj",
                "net10.0",
                "partial",
                "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; 1 unresolved symbol",
                1,
            ),
            project(
                "Unresolved/Unresolved.csproj",
                "net10.0",
                "partial",
                "3 unresolved symbols",
                3,
            ),
        ]
    }


def state_doc():
    return {
        "coverage": {
            "tools": [
                {
                    "tool": "dotnet-msbuild",
                    "status": "partial",
                    "reason": "Broken/Broken.csproj: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable",
                },
                {
                    "tool": "roslyn",
                    "status": "partial",
                    "reason": "3 of 7 projects partial, 1 failed, 35 unresolved symbols; Broken/Broken.csproj failed: MSB4236",
                },
            ]
        },
        "measurement": {
            "tool_versions": {
                "dotnet-msbuild": lc.DOTNET_VERSION,
                "roslyn": "archfit-roslyn 1.0.0; Microsoft.CodeAnalysis 5.6.0",
            }
        },
        "seams": [
            {
                "from_module": "Infrastructure",
                "to_module": "Domain",
                "strength": "intrusive",
            },
            {"from_module": "Application", "to_module": "Domain", "strength": "model"},
            {"from_module": "Legacy", "to_module": "Domain", "strength": "functional"},
        ],
    }


def failed_names(check, doc):
    result = lc.ContractResult("dotnet")
    check(result, doc)
    return [failure.split(":", 1)[0] for failure in result.failures]


class TestDotnetContract(unittest.TestCase):
    def test_expected_helper_output_passes(self):
        self.assertEqual(failed_names(lc.check_dotnet_helper, helper_doc()), [])

    def test_complete_net8_on_a_host_with_the_pack_passes(self):
        doc = helper_doc()
        legacy8 = doc["projects"][4]
        legacy8.update(status="complete", reason="", unresolved_symbols=0)
        self.assertEqual(failed_names(lc.check_dotnet_helper, doc), [])

    def test_net8_partial_for_another_reason_fails(self):
        doc = helper_doc()
        doc["projects"][4]["reason"] = "31 unresolved symbols"
        self.assertEqual(
            failed_names(lc.check_dotnet_helper, doc),
            ["Legacy@net8.0 is complete or partial naming only the 8.x reference pack"],
        )

    def test_helper_mutations_are_caught(self):
        def downgrade_intrusive(doc):
            doc["projects"][2]["files"][0]["edges"][1]["hint"] = "functional"

        def broken_loses_sdk(doc):
            doc["projects"][5]["reason"] = "failed"

        def tests_leaks_an_edge(doc):
            doc["projects"][6]["files"] = [
                {
                    "path": "Tests/ThingTests.cs",
                    "edges": [
                        {"to": "Broken/Broken.cs", "hint": "model", "data_hint": ""}
                    ],
                }
            ]

        def reasonless_partial(doc):
            doc["projects"][7]["reason"] = ""

        def domain_not_complete(doc):
            doc["projects"][0]["status"] = "partial"

        for mutate in (
            downgrade_intrusive,
            broken_loses_sdk,
            tests_leaks_an_edge,
            reasonless_partial,
            domain_not_complete,
        ):
            with self.subTest(mutation=mutate.__name__):
                doc = helper_doc()
                mutate(doc)
                self.assertTrue(failed_names(lc.check_dotnet_helper, doc))

    def test_expected_state_passes(self):
        self.assertEqual(failed_names(lc.check_dotnet_state, state_doc()), [])

    def test_state_mutations_are_caught(self):
        def roslyn_ok(state):
            state["coverage"]["tools"][1]["status"] = "ok"

        def no_unresolved_count(state):
            state["coverage"]["tools"][1]["reason"] = (
                "1 failed; Broken/Broken.csproj failed"
            )

        def tests_seam(state):
            state["seams"].append(
                {"from_module": "Tests", "to_module": "Broken", "strength": "model"}
            )

        def wrong_strength(state):
            state["seams"][0]["strength"] = "functional"

        for mutate in (roslyn_ok, no_unresolved_count, tests_seam, wrong_strength):
            with self.subTest(mutation=mutate.__name__):
                state = state_doc()
                mutate(state)
                self.assertTrue(failed_names(lc.check_dotnet_state, state))

    def test_missing_helper_is_a_preflight_failure(self):
        original = lc.os.environ.get(lc.DOTNET_HELPER_ENV)
        try:
            lc.os.environ[lc.DOTNET_HELPER_ENV] = "/nonexistent/dotnet-roslyn"
            failures = lc.dotnet_preflight(lc.REPO_ROOT)
        finally:
            if original is None:
                lc.os.environ.pop(lc.DOTNET_HELPER_ENV, None)
            else:
                lc.os.environ[lc.DOTNET_HELPER_ENV] = original
        self.assertTrue(any("Roslyn helper" in failure for failure in failures))

    def test_config_uses_schema_v2_and_one_module_per_project(self):
        config = lc.dotnet_config_text()
        self.assertRegex(config, r"(?m)^version: 2$")
        for name in lc.DOTNET_PROJECTS:
            self.assertIn(f"  {name}:\n    paths: ['{name}/**']", config)

    def test_only_selects_known_cases(self):
        self.assertIn("dotnet", lc.ALL_CASES)
        self.assertEqual(
            set(lc.ALL_CASES), {spec.name for spec in lc.FIXTURES} | {"dotnet"}
        )


if __name__ == "__main__":
    unittest.main()
