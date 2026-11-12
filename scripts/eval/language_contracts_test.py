#!/usr/bin/env python3
"""Fast tests for the real-language acceptance harness (stdlib plus jsonschema)."""

from __future__ import annotations

import copy
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


class TestPreflight(unittest.TestCase):
    def test_required_tools_per_case(self):
        cases = {
            "go": {"git", "ast-grep"},
            "typescript": {"git", "ast-grep"},
            "javascript": {"git", "ast-grep"},
            "python": {"git", "ast-grep"},
            "rust": {"git", "ast-grep", "cargo"},
            "dotnet": {"git", "dotnet"},
        }
        for case, want in cases.items():
            with self.subTest(case=case):
                self.assertEqual(lc.required_tools((case,)), want)

    def test_no_case_needs_a_removed_producer(self):
        removed = {"node", "depcruise", "go", "python", "python3", "uv"}
        self.assertFalse(lc.required_tools(lc.ALL_CASES) & removed)

    def test_only_selects_known_cases(self):
        self.assertEqual(
            set(lc.ALL_CASES), {spec.name for spec in lc.FIXTURES} | {"dotnet"}
        )

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


class TestFixturePolicies(unittest.TestCase):
    def test_five_fixture_languages_are_explicit(self):
        self.assertEqual(
            [spec.name for spec in lc.FIXTURES],
            ["go", "typescript", "javascript", "python", "rust"],
        )

    def test_every_fixture_policy_is_schema_3_with_its_deny_rule(self):
        for spec in lc.FIXTURES:
            with self.subTest(spec=spec.name):
                policy = lc.config_text(spec)
                self.assertRegex(policy, r"(?m)^version: 3$")
                self.assertIn(f"owners: [{lc.POLICY_OWNER}]", policy)
                self.assertIn(f"  core:\n    paths: ['{spec.core_paths}']", policy)
                self.assertIn(
                    f"  adapter:\n    paths: ['{spec.adapter_paths}']", policy
                )
                self.assertIn(
                    f"  - id: {lc.RULE_ID}\n    deny: {{ from: core, to: adapter }}",
                    policy,
                )
                for v2_key in ("analyzers:", "layers:", "gate:", "waivers:", "owner:"):
                    self.assertNotIn(v2_key, policy)

    def test_python_policy_names_its_package(self):
        spec = next(spec for spec in lc.FIXTURES if spec.name == "python")
        self.assertIn(
            "languages:\n  python:\n    package: smoke\n", lc.config_text(spec)
        )

    def test_the_change_adds_a_dependency_the_clean_tree_lacks(self):
        for spec in lc.FIXTURES:
            with self.subTest(spec=spec.name):
                self.assertTrue(spec.forbidden)
                for path, content in spec.forbidden.items():
                    self.assertIn(path, spec.files)
                    self.assertNotEqual(spec.files[path], content)

    def test_javascript_is_read_as_typescript(self):
        import json

        fixture = next(spec for spec in lc.FIXTURES if spec.name == "javascript")
        self.assertEqual(fixture.language, "typescript")
        self.assertTrue(
            json.loads(fixture.files["tsconfig.json"])["compilerOptions"]["allowJs"]
        )

    def test_exceptions_cover_every_error_once(self):
        errors = [
            {"rule": "graph", "from": "core", "to": "adapter"},
            {"rule": lc.RULE_ID, "from": "core", "to": "adapter"},
            {"rule": lc.RULE_ID, "from": "core", "to": "adapter"},
            {"rule": lc.EXCEPTION_RULE, "from": "core", "to": "adapter"},
        ]
        self.assertEqual(
            lc.exceptions_text(errors, lc.EXPIRED_ON),
            "exceptions:\n"
            "  - rule: core_no_adapter\n    from: core\n    to: adapter\n"
            '    expires: "2000-01-01"\n    why: acceptance exception lifecycle\n'
            "  - rule: graph\n    from: core\n    to: adapter\n"
            '    expires: "2000-01-01"\n    why: acceptance exception lifecycle\n',
        )


def command(returncode, label="check"):
    return lc.CommandResult(
        label, ["archfit", "check"], "/tmp/repo", returncode, "", ""
    )


def forbidden_doc():
    return {
        "schema": lc.RESULT_SCHEMA,
        "status": "fail",
        "summary": "archfit: fail, 2 errors",
        "errors": [
            {
                "path": "core/core.go",
                "line": 3,
                "rule": "graph",
                "what": "core -> adapter is not an allowed dependency",
                "next": {"kind": "command", "text": "archfit request"},
                "from": "core",
                "to": "adapter",
                "key": "graph core adapter adapter",
                "from_change": True,
            },
            {
                "path": "core/core.go",
                "line": 3,
                "rule": lc.RULE_ID,
                "what": "core must not depend on adapter",
                "next": {"kind": "fix", "text": "fix the code"},
                "from": "core",
                "to": "adapter",
                "key": "core_no_adapter core adapter adapter",
                "from_change": True,
            },
        ],
        "unknown": [],
        "policy": {
            "path": ".archfit.yaml",
            "digest": "sha256:" + "0" * 64,
            "protected": False,
            "differs": False,
            "target": "main",
        },
        "base": "a" * 40,
        "head": "b" * 40,
        "date": "2026-11-12",
        "engine": "dev",
    }


def failures_of(check):
    result = lc.ContractResult("fixture")
    check(result)
    return [failure.split(":", 1)[0] for failure in result.failures]


class TestResultChecks(unittest.TestCase):
    changed = frozenset({"core/core.go"})

    def forbidden(self, doc, returncode=1):
        return failures_of(
            lambda r: lc.check_forbidden(
                r, command(returncode), doc, lc.RULE_ID, self.changed
            )
        )

    def test_the_forbidden_record_passes(self):
        self.assertEqual(self.forbidden(forbidden_doc()), [])

    def test_forbidden_mutations_are_caught(self):
        def other_rule(doc):
            doc["errors"][1]["rule"] = "graph"

        def not_from_change(doc):
            doc["errors"][1]["from_change"] = False

        def other_file(doc):
            doc["errors"][1]["path"] = "adapter/adapter.go"

        def unknown_reason(doc):
            doc["unknown"] = [{"cause": "tool", "what": "ast-grep is missing"}]

        def passed(doc):
            doc["status"] = "pass"

        for mutate in (other_rule, not_from_change, other_file, unknown_reason, passed):
            with self.subTest(mutation=mutate.__name__):
                doc = forbidden_doc()
                mutate(doc)
                self.assertTrue(self.forbidden(doc))
        self.assertTrue(self.forbidden(forbidden_doc(), returncode=2))

    def test_clean_check(self):
        clean = forbidden_doc()
        clean.update(status="pass", summary="archfit: pass", errors=[])
        self.assertEqual(
            failures_of(lambda r: lc.check_clean(r, command(0), clean)), []
        )
        self.assertTrue(failures_of(lambda r: lc.check_clean(r, command(1), clean)))
        self.assertTrue(
            failures_of(lambda r: lc.check_clean(r, command(0), forbidden_doc()))
        )

    def exception_failures(self, active_rc, active, expired_rc, expired):
        return failures_of(
            lambda r: lc.check_exceptions(
                r, command(active_rc), active, command(expired_rc), expired, lc.RULE_ID
            )
        )

    def test_exception_lifecycle(self):
        active = forbidden_doc()
        active.update(status="pass", errors=[])
        expired = forbidden_doc()
        expired["errors"].append(
            {
                "path": ".archfit.yaml",
                "rule": lc.EXCEPTION_RULE,
                "what": "the exception has expired",
                "next": {"kind": "fix", "text": "fix the code"},
                "key": "exception core_no_adapter core adapter",
                "from_change": True,
                "exception": {"rule": lc.RULE_ID, "expires": lc.EXPIRED_ON},
            }
        )
        self.assertEqual(self.exception_failures(0, active, 1, expired), [])
        self.assertTrue(self.exception_failures(1, forbidden_doc(), 1, expired))
        self.assertTrue(self.exception_failures(0, active, 1, forbidden_doc()))
        self.assertTrue(self.exception_failures(0, active, 0, expired))

    def test_published_schema_accepts_a_record_and_rejects_a_broken_one(self):
        self.assertEqual(lc.validate_result(forbidden_doc()), [])
        broken = copy.deepcopy(forbidden_doc())
        del broken["status"]
        self.assertTrue(lc.validate_result(broken))

    def test_unparsable_output_fails(self):
        result = lc.ContractResult("fixture")
        self.assertIsNone(lc.parse_result(result, command(2)))
        self.assertTrue(result.failures)


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

    def test_checked_policy_lists_only_the_healthy_projects(self):
        policy = lc.dotnet_policy_text(lc.DOTNET_CHECKED_PROJECTS)
        self.assertRegex(policy, r"(?m)^version: 3$")
        self.assertIn(
            "    projects: [Domain/Domain.csproj, Application/Application.csproj, "
            "Infrastructure/Infrastructure.csproj, Legacy/Legacy.csproj]\n",
            policy,
        )
        for name in (*lc.DOTNET_PROJECTS, "Shared"):
            self.assertIn(f"  {name}:\n    paths: ['{name}/**']\n", policy)
        self.assertNotIn("projects:", lc.dotnet_policy_text(None))

    def test_the_change_adds_one_project_reference_to_the_fixture(self):
        original = (lc.DOTNET_FIXTURE / lc.DOTNET_CHANGED_PROJECT).read_text(
            encoding="utf-8"
        )
        changed = lc.dotnet_forbidden(original)
        self.assertEqual(
            changed.replace(lc.DOTNET_FORBIDDEN_REFERENCE, "", 1), original
        )
        with self.assertRaises(RuntimeError):
            lc.dotnet_forbidden("<Project />")

    def test_every_project_result_must_name_broken(self):
        reason = {
            "cause": "tool",
            "what": ".NET projects not read: Broken/Broken.csproj: MSB4236: SDK "
            "'Archfit.Fixture.Missing.Sdk' not resolvable offline",
        }

        def failures(returncode, status, unknown):
            doc = {"status": status, "unknown": unknown}
            return failed_names(
                lambda r, d: lc.check_dotnet_unknown(r, command(returncode), d), doc
            )

        self.assertEqual(failures(2, "unknown", [reason]), [])
        self.assertEqual(failures(1, "fail", [reason]), [])
        self.assertTrue(failures(0, "pass", [reason]))
        self.assertTrue(failures(2, "unknown", []))
        self.assertTrue(failures(2, "unknown", [{**reason, "cause": "import"}]))


if __name__ == "__main__":
    unittest.main()
