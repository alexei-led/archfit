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
    def test_only_the_changes_deny_error_is_a_forbidden_dependency(self):
        def error(rule, from_change):
            return {"rule": rule, "from_change": from_change}

        cases = {
            "empty record": ([], False),
            "deny error from the change": ([error(image.lc.RULE_ID, True)], True),
            "deny error not from the change": ([error(image.lc.RULE_ID, False)], False),
            "only the graph error": ([error("graph", True)], False),
        }
        for name, (errors, want) in cases.items():
            with self.subTest(name):
                self.assertEqual(image.forbidden({"errors": errors}), want)

    def test_read_result_accepts_only_a_version_3_record(self):
        good = {"schema": image.lc.RESULT_SCHEMA, "errors": [], "unknown": []}
        cases = {
            "record": (good, True),
            "version 2 state": (
                {"schema_version": "archfit.architecture-state.v1"},
                False,
            ),
            "errors not a list": ({**good, "errors": None}, False),
            "unknown missing": (
                {"schema": image.lc.RESULT_SCHEMA, "errors": []},
                False,
            ),
        }
        for name, (doc, valid) in cases.items():
            with self.subTest(name), tempfile.TemporaryDirectory() as temporary:
                output = Path(temporary)
                (output / "check.json").write_text(json.dumps(doc))
                contract = image.ImageContract("local:contract", "colima", output)
                self.assertEqual(contract.read_result(output, "check") == doc, valid)
                self.assertEqual(not contract.assertions, valid)

    def test_fixture_is_a_change_on_a_clean_target_branch(self):
        def git(path, *args):
            return subprocess.run(
                ["git", *args], cwd=path, capture_output=True, text=True, check=True
            ).stdout

        with tempfile.TemporaryDirectory() as temporary:
            for spec in image.lc.FIXTURES:
                with self.subTest(language=spec.name):
                    path = Path(temporary) / spec.name
                    extra = {"sitecustomize.py": "raise RuntimeError('poison')\n"}
                    image.prepare_fixture(spec, path, extra)
                    policy = (path / ".archfit.yaml").read_text()
                    self.assertEqual(policy, image.lc.config_text(spec))
                    self.assertRegex(policy, r"(?m)^version: 3$")
                    self.assertEqual(
                        git(path, "branch", "--show-current").strip(),
                        image.lc.CHANGE_BRANCH,
                    )
                    self.assertEqual(git(path, "status", "--porcelain"), "")
                    for name, content in spec.forbidden.items():
                        self.assertEqual((path / name).read_text(), content)
                        self.assertEqual(
                            git(path, "show", f"{image.lc.TARGET_BRANCH}:{name}"),
                            spec.files[name],
                        )
                    self.assertEqual(
                        git(path, "show", f"{image.lc.TARGET_BRANCH}:sitecustomize.py"),
                        extra["sitecustomize.py"],
                    )
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

    def test_dotnet_extraction_fails_closed(self):
        stderr = (
            "3 of 7 projects partial, 1 failed, 35 unresolved symbols; "
            "Broken/Broken.csproj failed: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; "
            "Legacy/Legacy.csproj partial: net8.0: 31 unresolved symbols; "
            "reference pack Microsoft.NETCore.App.Ref 8.0 is not installed; "
            "Tests/Tests.csproj partial: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; "
            "Unresolved/Unresolved.csproj partial: 3 unresolved symbols"
        )
        state = {
            "coverage": {
                "tools": [
                    {
                        "tool": "dotnet-msbuild",
                        "status": "partial",
                        "reason": "Broken/Broken.csproj: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk'",
                    },
                    {"tool": "roslyn", "status": "partial"},
                ]
            },
            "seams": [
                {
                    "from_module": "repository",
                    "to_module": "contracts",
                    "strength": "model",
                },
                {
                    "from_module": "repository",
                    "to_module": "domain",
                    "strength": "intrusive",
                },
            ],
        }
        self.assertEqual(image.dotnet_extraction_failures(state, stderr), [])

        def seam(strength, source="repository", target="domain"):
            return {"from_module": source, "to_module": target, "strength": strength}

        tools = state["coverage"]["tools"]
        cases = {
            "no state": ({}, stderr),
            "roslyn ok": (
                {
                    **state,
                    "coverage": {
                        "tools": [tools[0], {"tool": "roslyn", "status": "ok"}]
                    },
                },
                stderr,
            ),
            "msbuild ok": (
                {
                    **state,
                    "coverage": {
                        "tools": [{"tool": "dotnet-msbuild", "status": "ok"}, tools[1]]
                    },
                },
                stderr,
            ),
            "weak intrusive": (
                {**state, "seams": [state["seams"][0], seam("functional")]},
                stderr,
            ),
            "test edge": (
                {**state, "seams": [*state["seams"], seam("model", "tests")]},
                stderr,
            ),
            "legacy complete": (
                state,
                stderr.replace(
                    "Legacy/Legacy.csproj partial", "Legacy/Legacy.csproj ok"
                ),
            ),
            "complete project named": (
                state,
                stderr + "; Domain/Domain.csproj partial: x",
            ),
        }
        for name, (case_state, case_stderr) in cases.items():
            with self.subTest(name):
                self.assertTrue(
                    image.dotnet_extraction_failures(case_state, case_stderr)
                )

    def test_dotnet_fixture_copy_is_config_ready_and_skips_build_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "fixture"
            image.prepare_dotnet_fixture(path)
            self.assertIn("Fixture.slnx", (path / ".archfit.yaml").read_text())
            self.assertTrue((path / "Fixture.slnx").is_file())
            self.assertFalse(any(path.rglob("obj")) or any(path.rglob("bin")))

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

    def test_python314_uses_report_on_a_git_fixture(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            contract = image.ImageContract("local:contract", "colima", output)
            state = {
                "schema_version": "archfit.architecture-state.v1",
                "comparison": {
                    "measurement_profile": {
                        "producers": [
                            {
                                "tool": "grimp",
                                "semantics_version": "grimp.v3",
                                "tool_version": f"grimp {image.lc.TOOL_MANIFEST['python']['grimp']}; python {image.lc.TOOL_MANIFEST['versions']['python']}",
                            }
                        ]
                    }
                },
                "coverage": {"tools": [{"tool": "grimp", "status": "ok"}]},
                "decision": {"hard_gates": "fail"},
                "findings": [{"rule_id": "no-core-to-adapter", "kind": "gate"}],
            }

            def run(name, uid, script, fixture):
                self.assertTrue((fixture / ".git").is_dir())
                self.assertTrue((fixture / "src/modern314/core.py").is_file())
                self.assertIn("archfit report -C /tmp/repo --format json", script)
                self.assertNotIn("archfit analyze", script)
                result = output / name
                result.mkdir()
                (result / "report.json").write_text(json.dumps(state))
                (result / "report.rc").write_text("0")
                return result

            with patch.object(contract, "run", side_effect=run) as commands:
                image.python314_case(contract)
            self.assertEqual(commands.call_count, 2)
            self.assertTrue(all(row["passed"] for row in contract.assertions))


if __name__ == "__main__":
    unittest.main()
