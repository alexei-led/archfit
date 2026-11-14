#!/usr/bin/env python3
"""Fast checks for failure disclosure and real-source image fixture construction."""

import json
import os
import shlex
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image


class ImageContractsTest(unittest.TestCase):
    def test_version2_image_requires_rejection_and_conversion_guidance(self):
        def exercise(rc, message):
            with tempfile.TemporaryDirectory() as temporary:
                output = Path(temporary)
                contract = image.ImageContract("local:contract", "colima", output)

                def run(name, uid, script, fixture):
                    self.assertIn("archfit report -C /tmp/repo --format json", script)
                    self.assertIn("archfit doctor --format json", script)
                    self.assertEqual((fixture / ".archfit.yaml").read_text(), "version: 2\nmodules: {}\n")
                    result = output / name
                    result.mkdir()
                    for command in ("report", "doctor"):
                        (result / f"{command}.rc").write_text(rc)
                        (result / f"{command}.stderr").write_text(message)
                    return result

                with patch.object(contract, "run", side_effect=run):
                    image.version2_cases(contract)
                return all(row["passed"] for row in contract.assertions)

        self.assertTrue(exercise("2", "run archfit init"))
        self.assertFalse(exercise("0", "run archfit init"))
        self.assertFalse(exercise("2", "unsupported format"))

    def test_image_inventory_rejects_removed_tools(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            contract = image.ImageContract("local:contract", "colima", root)
            with patch.object(contract, "run") as run:
                image.inventory(contract)
            script = run.call_args.args[2]
            for name in ("depcruise", "scip-go", "scip-typescript", "scip-python", "rust-analyzer", "jscpd"):
                with self.subTest(tool=name):
                    probe = next(part for part in script.split(";") if f"command -v {name}" in part)
                    tool = root / name
                    tool.write_text("#!/bin/sh\nexit 0\n")
                    tool.chmod(0o755)
                    result = subprocess.run(
                        ["/bin/sh", "-ec", probe + "; printf bypass"],
                        env={**os.environ, "PATH": str(root)},
                        capture_output=True,
                        text=True,
                    )
                    self.assertNotEqual(result.returncode, 0)
                    self.assertNotIn("bypass", result.stdout)
                    tool.unlink()

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
                    "strength": "contract",
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
            "retired gate reasons": (
                {**state, "gate_reasons": ["no comparable reference"]},
                stderr,
            ),
            "retired metric delta": (
                {
                    **state,
                    "dimensions": {"drift": {"delta": {"status": "not_requested"}}},
                },
                stderr,
            ),
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
            "retired unbounded DTO strength": (
                {**state, "seams": [seam("model", target="contracts"), state["seams"][1]]},
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
                "measurement": {
                    "tool_versions": {
                        "pyimports": f"ast-grep {image.lc.TOOL_MANIFEST['node']['@ast-grep/cli']}"
                    }
                },
                "coverage": {"tools": [{"tool": "pyimports", "status": "ok"}]},
                "repository": {
                    "state": "drift",
                    "drift": [
                        {
                            "rule": "no-core-to-adapter",
                            "from": "core",
                            "to": "adapter",
                            "path": "src/modern314/core.py",
                            "line": 2,
                        }
                    ],
                    "unread": [],
                },
            }

            report_rc = "0"

            def run(name, uid, script, fixture):
                self.assertTrue((fixture / ".git").is_dir())
                self.assertTrue((fixture / "src/modern314/core.py").is_file())
                self.assertTrue((fixture / "src/modern314/_version.py").is_file())
                self.assertIn("archfit report -C /tmp/repo --format json", script)
                self.assertNotIn("archfit analyze", script)
                result = output / name
                result.mkdir()
                (result / "report.json").write_text(json.dumps(state))
                (result / "report.rc").write_text(report_rc)
                return result

            with patch.object(contract, "run", side_effect=run) as commands:
                image.python314_case(contract)
            self.assertEqual(commands.call_count, 2)
            self.assertTrue(all(row["passed"] for row in contract.assertions))

            negatives = {
                "missing parser": {"coverage": {"tools": []}},
                "wrong producer": {
                    "coverage": {"tools": [{"tool": "grimp", "status": "ok"}]}
                },
                "partial parser": {
                    "coverage": {"tools": [{"tool": "pyimports", "status": "partial"}]}
                },
                "missing version": {"measurement": {"tool_versions": {}}},
                "wrong version": {
                    "measurement": {"tool_versions": {"pyimports": "ast-grep 0.0.0"}}
                },
                "no repository state": {"repository": {}},
                "clean state": {
                    "repository": {"state": "clean", "drift": [], "unread": []}
                },
                "unread source": {
                    "repository": {
                        **state["repository"],
                        "unread": [{"path": "src/modern314/core.py"}],
                    }
                },
                "unread absent": {
                    "repository": {
                        "state": "drift",
                        "drift": state["repository"]["drift"],
                    }
                },
                "no deny error": {"repository": {**state["repository"], "drift": []}},
                "wrong rule": {
                    "repository": {
                        **state["repository"],
                        "drift": [{**state["repository"]["drift"][0], "rule": "graph"}],
                    }
                },
                "wrong place": {
                    "repository": {
                        **state["repository"],
                        "drift": [
                            {**state["repository"]["drift"][0], "path": "wrong.py"}
                        ],
                    }
                },
                "wrong line": {
                    "repository": {
                        **state["repository"],
                        "drift": [{**state["repository"]["drift"][0], "line": 1}],
                    }
                },
                "wrong source": {
                    "repository": {
                        **state["repository"],
                        "drift": [
                            {**state["repository"]["drift"][0], "from": "adapter"}
                        ],
                    }
                },
                "wrong target": {
                    "repository": {
                        **state["repository"],
                        "drift": [{**state["repository"]["drift"][0], "to": "core"}],
                    }
                },
            }
            good = state.copy()
            for name, change in negatives.items():
                with self.subTest(name=name), tempfile.TemporaryDirectory() as bad_dir:
                    state = {**good, **change}
                    output = Path(bad_dir)
                    bad = image.ImageContract("local:contract", "colima", output)
                    with patch.object(bad, "run", side_effect=run):
                        image.python314_case(bad)
                    self.assertTrue(all(not row["passed"] for row in bad.assertions))
            state = good
            for report_rc in ("1", "2"):
                with (
                    self.subTest(report_rc=report_rc),
                    tempfile.TemporaryDirectory() as bad_dir,
                ):
                    output = Path(bad_dir)
                    bad = image.ImageContract("local:contract", "colima", output)
                    with patch.object(bad, "run", side_effect=run):
                        image.python314_case(bad)
                    self.assertTrue(all(not row["passed"] for row in bad.assertions))

    def test_typescript_image_checks_real_gate_and_report_producer(self):
        spec = next(s for s in image.lc.FIXTURES if s.name == "typescript")
        pin = f"ast-grep {image.lc.TOOL_MANIFEST['node']['@ast-grep/cli']}"
        good = {
            "schema_version": "archfit.architecture-state.v1",
            "measurement": {"tool_versions": {"tsimports": pin}},
            "coverage": {"tools": [{"tool": "tsimports", "status": "ok"}]},
        }
        check = {
            "schema": image.lc.RESULT_SCHEMA,
            "status": "fail",
            "unknown": [],
            "errors": [{"rule": image.lc.RULE_ID, "from_change": True}],
        }

        def exercise(state, record, rc, parser_unknown=True):
            with tempfile.TemporaryDirectory() as temporary:
                output = Path(temporary)
                contract = image.ImageContract("local:contract", "colima", output)

                def run(name, uid, script, fixture):
                    self.assertIn(
                        spec.forbidden["src/core/index.ts"],
                        (fixture / "src/core/index.ts").read_text(),
                    )
                    self.assertIn("archfit report -C /tmp/repo --format json", script)
                    self.assertIn("PATH=/usr/local/bin:/usr/bin:/bin", script)
                    result = output / name
                    result.mkdir()
                    (result / "primary.json").write_text(json.dumps(record))
                    (result / "primary.rc").write_text(rc)
                    (result / "report.json").write_text(json.dumps(state))
                    (result / "report.rc").write_text("0")
                    (result / "no-parser.json").write_text(
                        json.dumps(
                            {
                                "schema": image.lc.RESULT_SCHEMA,
                                "status": "unknown" if parser_unknown else "pass",
                                "errors": [],
                                "unknown": [{"what": "ast-grep is not installed"}]
                                if parser_unknown
                                else [],
                            }
                        )
                    )
                    (result / "no-parser.rc").write_text("2" if parser_unknown else "0")
                    return result

                with (
                    patch.object(image.lc, "FIXTURES", (spec,)),
                    patch.object(contract, "run", side_effect=run),
                ):
                    image.language_cases(contract)
                return all(row["passed"] for row in contract.assertions)

        self.assertTrue(exercise(good, check, "1"))
        self.assertFalse(exercise(good, check, "1", parser_unknown=False))
        for name, state, record, rc in [
            ("missing coverage", {**good, "coverage": {"tools": []}}, check, "1"),
            (
                "old producer",
                {
                    **good,
                    "coverage": {
                        "tools": [{"tool": "dependency-cruiser", "status": "ok"}]
                    },
                },
                check,
                "1",
            ),
            (
                "partial parser",
                {
                    **good,
                    "coverage": {"tools": [{"tool": "tsimports", "status": "partial"}]},
                },
                check,
                "1",
            ),
            (
                "missing version",
                {**good, "measurement": {"tool_versions": {}}},
                check,
                "1",
            ),
            (
                "wrong version",
                {
                    **good,
                    "measurement": {"tool_versions": {"tsimports": "ast-grep 0.0.0"}},
                },
                check,
                "1",
            ),
            ("no forbidden edge", good, {**check, "errors": []}, "1"),
            ("passing exit", good, check, "0"),
        ]:
            with self.subTest(name=name):
                self.assertFalse(exercise(state, record, rc))

    def test_readonly_report_trusts_only_the_host_owned_fixture(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            contract = image.ImageContract("local:contract", "colima", output)
            scripts = []
            state = {
                "schema_version": "archfit.architecture-state.v1",
                "coverage": {
                    "tools": [
                        {
                            "tool": "dotnet-msbuild",
                            "status": "partial",
                            "reason": "Broken/Broken.csproj: Archfit.Fixture.Missing.Sdk",
                        },
                        {"tool": "roslyn", "status": "partial"},
                    ]
                },
                "seams": [
                    {
                        "from_module": "repository",
                        "to_module": "contracts",
                        "strength": "contract",
                    },
                    {
                        "from_module": "repository",
                        "to_module": "domain",
                        "strength": "intrusive",
                    },
                ],
            }
            stderr = (
                "3 of 7 projects partial, 1 failed; "
                "Broken/Broken.csproj failed: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk'; "
                "Legacy/Legacy.csproj partial: net8.0: reference pack Microsoft.NETCore.App.Ref 8.0; "
                "Tests/Tests.csproj partial: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk'; "
                "Unresolved/Unresolved.csproj partial: 3 unresolved symbols"
            )

            def run(name, uid, script, fixture, **kwargs):
                if kwargs.get("copy") is False:
                    scripts.append(script)
                result = output / name
                result.mkdir()
                (result / "report.json").write_text(json.dumps(state))
                (result / "report.rc").write_text("0")
                (result / "report.stderr").write_text(stderr)
                return result

            with patch.object(contract, "run", side_effect=run):
                image.dotnet_extraction_cases(contract)
            self.assertTrue(all(row["passed"] for row in contract.assertions))
            self.assertEqual(len(scripts), 1)
            trust = shlex.split(scripts[0].partition(";")[0])
            self.assertEqual(
                trust,
                ["git", "config", "--global", "--add", "safe.directory", "/trusted"],
            )
            fixture = output / "fixture-dotnet"
            home = output / "home"
            home.mkdir()
            env = {
                key: value
                for key, value in os.environ.items()
                if not key.startswith("GIT_")
            }
            env.update(
                {
                    "HOME": str(home),
                    "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
                    "GIT_CONFIG_NOSYSTEM": "1",
                    "GIT_TEST_ASSUME_DIFFERENT_OWNER": "1",
                }
            )
            command = ["git", "-C", str(fixture), "rev-parse", "--show-toplevel"]
            before = subprocess.run(command, env=env, capture_output=True, text=True)
            self.assertEqual(before.returncode, 128)
            self.assertIn("dubious ownership", before.stderr)
            trust[-1] = str(fixture)
            subprocess.run(trust, env=env, check=True, capture_output=True, text=True)
            after = subprocess.run(command, env=env, capture_output=True, text=True)
            self.assertEqual(after.returncode, 0, after.stderr)
            self.assertEqual(after.stdout.strip(), str(fixture.resolve()))
            trusted = subprocess.run(
                ["git", "config", "--global", "--get-all", "safe.directory"],
                env=env,
                capture_output=True,
                text=True,
                check=True,
            )
            self.assertEqual(trusted.stdout.strip(), str(fixture))


if __name__ == "__main__":
    unittest.main()
