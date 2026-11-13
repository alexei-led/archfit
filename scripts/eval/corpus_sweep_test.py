"""Archfit 4 corpus harness: verdict exits, failure disclosure and source safety."""

import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import corpus_sweep as cs


class CorpusSweepTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        (self.repo / ".archfit.yaml").write_text("version: 3\n")
        self.spec = cs.RepoSpec("fixture", self.repo, "go")
        self.calls = []

    def sweep(self, status="pass", code=0, corrupt=None, repeat_changes=False):
        def runner(command, cwd):
            self.calls.append(command)
            self.assertEqual(cwd, cs.REPO_ROOT)
            self.assertEqual(command[2:4], ["-C", str(self.repo)])
            self.assertNotIn("--config", command)
            self.assertNotIn("--root", command)
            name = command[1]
            if corrupt == name:
                return cs.CommandResult(2, "", "broken tool")
            if name == "report":
                return cs.CommandResult(
                    0, json.dumps({"schema_version": cs.STATE_SCHEMA_VERSION}), ""
                )
            self.assertEqual(name, "check")
            format_name = command[-1]
            if format_name == "text":
                return cs.CommandResult(code, f"archfit: {status}\n", "")
            if format_name == "sarif":
                return cs.CommandResult(code, '{"version":"2.1.0","runs":[]}', "")
            result = {"schema": cs.RESULT_SCHEMA, "status": status}
            if repeat_changes and len([c for c in self.calls if c[1] == "check"]) > 1:
                result["changed"] = True
            return cs.CommandResult(code, json.dumps(result), "")

        return cs.Sweep(Path("/binary/archfit"), self.root / "output", runner=runner)

    def test_exit_matrix_and_unknown_is_not_verified(self):
        for status, code, expected in (
            ("pass", 0, "pass"),
            ("fail", 1, "pass"),
            ("waiting", 1, "pass"),
            ("unknown", 2, "fail"),
            ("fail", 0, "fail"),
            ("pass", 3, "fail"),
            ("invalid", 0, "fail"),
        ):
            with self.subTest(status=status, code=code):
                record = self.sweep(status, code).process(self.spec)
                self.assertEqual(record["status"], expected, record)
                self.assertEqual(record["verdict"], status)

    def test_repeat_and_formats_use_the_check_contract(self):
        record = self.sweep().process(self.spec, want_repeat=True, want_formats=True)
        self.assertEqual(record["status"], "pass", record)
        self.assertEqual(
            [c[1] for c in self.calls], ["report", "check", "check", "check", "check"]
        )
        self.assertEqual([c[-1] for c in self.calls[-2:]], ["text", "sarif"])

    def test_report_error_and_check_error_fail_closed(self):
        for command in ("report", "check"):
            with self.subTest(command=command):
                record = self.sweep(corrupt=command).process(self.spec)
                self.assertEqual(record["status"], "fail")
                self.assertTrue(record["failures"])
                self.assertEqual(cs.strict_exit_code([record]), 1)
                self.assertEqual(
                    cs.finalize_status(record, {"fixture": "not installed"})["status"],
                    "fail",
                )

    def test_changed_repeat_fails(self):
        record = self.sweep(repeat_changes=True).process(self.spec, want_repeat=True)
        self.assertEqual(record["status"], "fail")
        self.assertIn("repeated check is not byte-identical", record["failures"])

    def test_missing_policy_never_runs_init_or_mutates_source(self):
        (self.repo / ".archfit.yaml").unlink()
        record = self.sweep().process(self.spec)
        self.assertEqual(record["status"], "fail")
        self.assertIn("policy missing", record["failures"][0])
        self.assertEqual(self.calls, [])
        self.assertEqual(list(self.repo.iterdir()), [])

    def test_only_missing_repositories_can_be_accepted_unverified(self):
        missing = cs.RepoSpec("missing", self.root / "absent", "go")
        record = self.sweep().process(missing)
        self.assertEqual(cs.strict_exit_code([record]), 1)
        cs.finalize_status(record, {"missing": "not cloned"})
        self.assertEqual(record["status"], "accepted_unverified")
        self.assertEqual(record["unverified"]["accepted_reason"], "not cloned")
        self.assertEqual(cs.strict_exit_code([record]), 0)
        self.assertEqual(cs.strict_exit_code([]), 1)

    def test_output_cannot_wipe_a_repository_even_with_a_work_marker(self):
        (self.repo / cs.WORK_DIR_MARKER).write_text("")
        sweep = cs.Sweep(Path("/binary/archfit"), self.root)
        record = sweep.process(cs.RepoSpec("repo", self.repo, "go"))
        self.assertEqual(record["status"], "fail")
        self.assertIn("contains repository", record["failures"][0])
        self.assertTrue((self.repo / ".archfit.yaml").is_file())

    def test_work_directories_require_a_marker_and_cannot_be_symlinks(self):
        unowned = self.root / "unowned"
        unowned.mkdir()
        source = unowned / "keep.txt"
        source.write_text("keep")
        with self.assertRaisesRegex(RuntimeError, "refusing to wipe"):
            cs.prepare_work_dir(unowned)
        link = self.root / "link"
        link.symlink_to(unowned, target_is_directory=True)
        with self.assertRaisesRegex(RuntimeError, "symlink"):
            cs.prepare_work_dir(link)
        self.assertEqual(source.read_text(), "keep")
        owned = cs.prepare_work_dir(self.root / "owned")
        (owned / "stale.json").write_text("{}")
        cs.prepare_work_dir(owned)
        self.assertFalse((owned / "stale.json").exists())

    def test_command_timeout_is_a_failure_not_a_hang(self):
        with patch.object(
            cs.subprocess,
            "run",
            side_effect=subprocess.TimeoutExpired(
                "archfit", 1, b"partial", b"tool blocked"
            ),
        ):
            result = cs.subprocess_runner(["archfit"], self.root)
        self.assertEqual(result.returncode, 124)
        self.assertEqual(result.stdout, "partial")
        self.assertIn("tool blocked", result.stderr)
        self.assertIn("timed out", result.stderr)

    def test_inputs_and_rust_pin(self):
        self.assertEqual(cs.parse_csv_list("a, b,, "), ["a", "b"])
        self.assertEqual(
            cs.parse_allow_unverified(["repo=not cloned"]), {"repo": "not cloned"}
        )
        for invalid in ("repo", "repo=", "=reason"):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                cs.parse_allow_unverified([invalid])
        self.assertEqual(
            cs.command_environment({})["RUSTUP_TOOLCHAIN"], cs.PINNED_RUST_TOOLCHAIN
        )
        self.assertEqual(
            cs.command_environment({"RUSTUP_TOOLCHAIN": "nightly"})["RUSTUP_TOOLCHAIN"],
            "nightly",
        )
        corpus = cs.corpus_for_workspace(self.root)
        self.assertEqual(
            corpus["omni/scheduled-tasks"].root,
            self.root / "omni/server/services/scheduled-tasks",
        )

    def test_retired_ai_option_is_rejected(self):
        with self.assertRaises(SystemExit) as exit_code:
            cs.build_parser().parse_args(["--ai-repos", "pumba"])
        self.assertEqual(exit_code.exception.code, 2)


if __name__ == "__main__":
    unittest.main()
