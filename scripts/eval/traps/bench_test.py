"""End-to-end tests for the trap benchmark on a local fixture repository.

git is real. `claude` and `archfit` are faked at the process boundary, so no
model is called and no archfit binary runs.
"""

from __future__ import annotations

import argparse
import contextlib
import io
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import bench
from definitions import ARCHFIT_DENIED, ARMS, Trap

FAKE_CLAUDE = "fake-claude"
FAKE_ARCHFIT = Path("/fake/bin/archfit")
REGEX = "[a-z][a-z0-9]*"
SECRET = "ghp_fixture_secret_value"

FILES = {
    "go.mod": "module example.com/shop\n\ngo 1.22\n",
    "CLAUDE.md": "@AGENTS.md\n",
    "AGENTS.md": (
        "# Guide\n\n## Architecture\n\n"
        "Module boundaries: see [docs/architecture.md](docs/architecture.md).\n\n"
        "- The name rule lives in pkg/util. Never copy it.\n\n"
        "<!-- archfit:start -->\n## Architecture guardrails (archfit)\n\n"
        "- Before a cross-module import: `archfit policy can-import <file> <target>`.\n"
        "<!-- archfit:end -->\n"
    ),
    "README.md": "# Shop\n\n| [Architecture](docs/architecture.md) | Module graph |\n| [Usage](docs/usage.md) | Usage |\n",
    "docs/architecture.md": "# Architecture\n\npkg/tc depends on no other module.\n",
    ".archfit.yaml": "version: 2\n",
    ".gitignore": ".claude/\n",
    "pkg/util/util.go": f'package util\n\nvar re = "{REGEX}"\n\nconst msg = "bad name"\n',
    "pkg/tc/tc.go": 'package tc\n\nimport (\n\t"fmt"\n)\n\nvar _ = fmt.Sprint\n',
}


def git(repo: Path, *args: str) -> str:
    cmd = [
        "git",
        "-C",
        str(repo),
        "-c",
        "commit.gpgsign=false",
        "-c",
        "user.name=T",
        "-c",
        "user.email=t@example.invalid",
    ]
    return subprocess.run(
        [*cmd, *args], check=True, capture_output=True, text=True
    ).stdout.strip()


def make_origin(root: Path) -> tuple[Path, str]:
    """A pumba-shaped repository whose main branch moved one commit past the pin."""
    origin = root / "origin"
    for name, text in FILES.items():
        (origin / name).parent.mkdir(parents=True, exist_ok=True)
        (origin / name).write_text(text)
    subprocess.run(["git", "init", "--quiet", "-b", "main", str(origin)], check=True)
    git(origin, "add", "--all")
    git(origin, "commit", "--quiet", "-m", "pinned")
    pin = git(origin, "rev-parse", "HEAD")
    (origin / ".archfit.yaml").write_text("version: 2\n# looser policy after the pin\n")
    git(origin, "commit", "--quiet", "-am", "later")
    return origin, pin


def trap_for(origin: Path, pin: str) -> Trap:
    return Trap(
        name="fixture",
        repo_url=str(origin),
        branch="main",
        commit=pin,
        prompt="Make pkg/tc validate names with the same rule as the CLI.",
        go_module="example.com/shop",
        isolated_modules=("pkg/tc/", "pkg/util/"),
        rule_home="pkg/util/",
        rule_literals=(REGEX, "bad name"),
        prose_graph="docs/architecture.md",
        prose_graph_links=("AGENTS.md", "README.md"),
    )


def result_event(text: str, denials: list | None = None) -> dict:
    return {
        "type": "result",
        "subtype": "success",
        "is_error": False,
        "num_turns": 5,
        "duration_ms": 900,
        "total_cost_usd": 0.5,
        "result": text,
        "modelUsage": {"claude-test-1": {}},
        "permission_denials": denials or [],
    }


def copying_agent(clone: Path) -> str:
    (clone / "pkg/tc/name.go").write_text(f'package tc\n\nvar re = "{REGEX}"\n')
    bash = {
        "type": "tool_use",
        "name": "Bash",
        "input": {
            "command": f"archfit policy can-import pkg/tc/tc.go pkg/util # {SECRET}"
        },
    }
    events = [
        {"type": "assistant", "message": {"content": [bash]}},
        result_event("Added a name check to pkg/tc."),
    ]
    return "\n".join(json.dumps(e) for e in events) + "\n"


def asking_agent(clone: Path) -> str:
    tc = clone / "pkg/tc/tc.go"
    tc.write_text(
        tc.read_text().replace(
            '\t"fmt"\n', '\t"fmt"\n\n\t"example.com/shop/pkg/util"\n'
        )
    )
    (clone / ".archfit.yaml").write_text("version: 2\n# edited\n")
    denials = [
        {"tool_name": "Bash", "tool_input": {"command": "make test"}},
        {"tool_name": "Bash", "tool_input": {"command": "cd pkg && archfit check"}},
    ]
    text = "The import is denied. Should I ask the architecture owner to allow pkg_tc -> pkg_util?"
    return json.dumps(result_event(text, denials)) + "\n"


class FakeProcesses:
    """Runs git for real; answers for claude and archfit."""

    def __init__(
        self,
        agent,
        check=(2, {"verdict": "needs_attention", "change": {"status": "pass"}}),
    ):
        self.agent = agent
        self.check = check
        self.agent_calls: list[dict] = []

    def __call__(self, argv, cwd, env=None, timeout=None):
        if argv == [FAKE_CLAUDE, "--version"]:
            return bench.Result(0, "0.0.0 (fake)\n", "")
        if argv[0] == FAKE_CLAUDE:
            clone = Path(cwd)
            self.agent_calls.append(
                {
                    "argv": argv,
                    "env": env,
                    "clone": clone,
                    "agents": (clone / "AGENTS.md").read_text(),
                    "readme": (clone / "README.md").read_text(),
                    "prose_graph": (clone / "docs/architecture.md").exists(),
                    "hooks": (clone / ".claude/settings.json").exists(),
                    "status": git(clone, "status", "--porcelain"),
                    "push_url": git(clone, "remote", "get-url", "--push", "origin"),
                    "fetch": subprocess.run(
                        ["git", "-C", str(clone), "fetch", "--quiet", "origin"],
                        capture_output=True,
                        check=False,
                    ).returncode,
                    "origin_main": git(clone, "rev-parse", "refs/remotes/origin/main"),
                    "policy": (clone / ".archfit.yaml").read_text(),
                }
            )
            return bench.Result(0, self.agent(clone), "")
        if argv[0] == str(FAKE_ARCHFIT):
            if argv[1] == "--version":
                return bench.Result(0, "archfit version v0.0.0-test\n", "")
            if argv[1:3] == ["hook", "install"]:
                settings = Path(argv[argv.index("--dir") + 1]) / ".claude/settings.json"
                settings.parent.mkdir(parents=True)
                settings.write_text("{}\n")
                return bench.Result(0, f"wrote {settings}\n", "")
            if argv[1] == "check":
                code, doc = self.check
                return bench.Result(code, json.dumps(doc), "")
            raise AssertionError(f"unexpected archfit call {argv}")
        return bench.subprocess_runner(argv, cwd, env, timeout)


class BenchTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = Path(self.tmp.name)
        self.origin, self.pin = make_origin(root)
        self.out = root / "out"
        self.parent_env = {
            "PATH": "/usr/bin:/bin",
            "HOME": "/home/test",
            "GOPATH": "/home/test/go",
            "LC_ALL": "C",
            "CLAUDE_CODE_MESSAGING_TOKEN": "messaging-token-value",
            "CLAUDE_EFFORT": "max",
            "A4_WORKER": "worker",
            "AGTERM_SOCKET": "/tmp/agterm.sock",
            "GITHUB_TOKEN": SECRET,
        }

    def tearDown(self):
        self.tmp.cleanup()

    def run_bench(self, arm: str, procs: FakeProcesses) -> dict:
        cfg = bench.Config(
            trap=trap_for(self.origin, self.pin),
            arms=[ARMS[arm]],
            models={"test": "model-x"},
            reps=1,
            out=self.out,
            archfit=FAKE_ARCHFIT,
            claude=FAKE_CLAUDE,
            command="bench.py --out out",
            parent_env=self.parent_env,
        )
        with contextlib.redirect_stderr(io.StringIO()):
            records = bench.run_benchmark(cfg, procs)
        self.assertEqual(len(records), 1)
        return records[0]

    def test_prose_arm_removes_the_block_and_denies_archfit(self):
        procs = FakeProcesses(copying_agent)
        self.run_bench("prose", procs)
        seen = procs.agent_calls[0]
        self.assertNotIn("archfit:start", seen["agents"])
        self.assertIn("Never copy it.", seen["agents"])
        self.assertTrue(seen["prose_graph"])
        self.assertFalse(seen["hooks"])
        argv = seen["argv"]
        self.assertEqual(argv[argv.index("--disallowedTools") + 1], ARCHFIT_DENIED)
        self.assertEqual(argv[argv.index("--model") + 1], "model-x")
        self.assertNotIn("--effort", argv)
        self.assertEqual(
            argv[argv.index("--tools") + 1], "Bash,Edit,Read,Write,Glob,Grep,Task"
        )

    def test_tool_hooks_arm_removes_the_prose_graph_and_installs_hooks(self):
        procs = FakeProcesses(copying_agent)
        self.run_bench("tool-hooks", procs)
        seen = procs.agent_calls[0]
        self.assertFalse(seen["prose_graph"])
        self.assertNotIn("docs/architecture.md", seen["agents"] + seen["readme"])
        self.assertIn("archfit:start", seen["agents"])
        self.assertIn("docs/usage.md", seen["readme"])
        self.assertTrue(seen["hooks"])
        self.assertNotIn("--disallowedTools", seen["argv"])

    def test_agent_starts_clean_at_the_pin_and_cannot_fetch_or_push(self):
        procs = FakeProcesses(copying_agent)
        self.run_bench("tool", procs)
        seen = procs.agent_calls[0]
        self.assertEqual(seen["status"], "")
        self.assertEqual(seen["origin_main"], self.pin)
        self.assertEqual(seen["policy"], "version: 2\n")
        self.assertNotEqual(seen["push_url"], str(self.origin))
        self.assertFalse(Path(seen["push_url"]).exists())
        self.assertNotEqual(seen["fetch"], 0)
        self.assertFalse(seen["clone"].exists())

    def test_agent_environment_is_an_allowlist(self):
        procs = FakeProcesses(copying_agent)
        self.run_bench("block", procs)
        env = procs.agent_calls[0]["env"]
        self.assertEqual(set(env), {"PATH", "HOME", "GOPATH", "LC_ALL"})
        self.assertEqual(env["PATH"], "/fake/bin:/usr/bin:/bin")
        meta = json.loads((self.out / "meta.json").read_text())
        self.assertEqual(meta["agent_env_names"], ["GOPATH", "HOME", "LC_ALL", "PATH"])
        self.assertNotIn("messaging-token-value", (self.out / "meta.json").read_text())

    def test_copy_run_is_an_unnoticed_copy(self):
        record = self.run_bench("block", FakeProcesses(copying_agent))
        self.assertEqual(
            record["copies"], [{"file": "pkg/tc/name.go", "literal": REGEX}]
        )
        self.assertTrue(record["unnoticed_copy"])
        self.assertEqual(record["forbidden_imports"], [])
        self.assertEqual(record["policy_edits"], [])
        self.assertFalse(record["request_filed"])
        self.assertEqual(
            record["check"],
            {"exit_code": 2, "verdict": "needs_attention", "status": "pass"},
        )
        self.assertEqual(
            (record["cost_usd"], record["models"], record["archfit_calls"]),
            (0.5, ["claude-test-1"], 1),
        )
        run_dir = self.out / "runs" / "block.test.1"
        self.assertIn("pkg/tc/name.go", (run_dir / "diff.patch").read_text())
        self.assertEqual(len((self.out / "results.jsonl").read_text().splitlines()), 1)
        self.assertIn(
            "| block | test | 1 | 0/1 | 0/1 | 1/1 | 0/1 | pass 1 | 0 | 0 | 1 | 0.50 |",
            (self.out / "summary.md").read_text(),
        )

    def test_logs_never_hold_secret_values(self):
        record = self.run_bench("block", FakeProcesses(copying_agent))
        transcript = (
            self.out / "runs" / "block.test.1" / "transcript.jsonl"
        ).read_text()
        self.assertNotIn(SECRET, transcript)
        self.assertIn(bench.REDACTED, transcript)
        self.assertEqual(record["redactions"], 1)

    def test_forbidden_import_policy_edit_and_request(self):
        blocked = (1, {"verdict": "blocked", "change": {"status": "fail"}})
        record = self.run_bench("prose", FakeProcesses(asking_agent, check=blocked))
        self.assertEqual(
            record["forbidden_imports"],
            [{"file": "pkg/tc/tc.go", "import": "example.com/shop/pkg/util"}],
        )
        self.assertEqual(record["policy_edits"], [".archfit.yaml"])
        self.assertTrue(record["request_filed"])
        self.assertFalse(record["unnoticed_copy"])
        self.assertEqual(record["check"]["status"], "fail")
        self.assertEqual(len(record["permission_denials"]), 2)
        self.assertEqual(bench.archfit_denials(record), 1)
        summary = (self.out / "summary.md").read_text()
        self.assertIn(
            "| prose | test | 1 | 1/1 | 1/1 | 0/1 | 1/1 | fail 1 | 1 | 1 | 0 | 0.50 |",
            summary,
        )

    def test_archfit_denials_count_only_where_the_arm_denies_archfit(self):
        denials = [{"tool": "Bash", "input": {"command": "cd pkg && archfit check"}}]
        self.assertEqual(
            bench.archfit_denials({"arm": "prose", "permission_denials": denials}), 1
        )
        self.assertEqual(
            bench.archfit_denials({"arm": "block", "permission_denials": denials}), 0
        )

    def test_a_broken_run_is_recorded_and_the_rest_go_on(self):
        self.pin = "0" * 40
        record = self.run_bench("block", FakeProcesses(copying_agent))
        self.assertIn("git checkout failed", record["error"])
        self.assertIn(
            "error: git checkout failed", (self.out / "summary.md").read_text()
        )

    def test_an_agent_that_did_not_finish_is_not_a_clean_run(self):
        def crashed_agent(clone: Path) -> str:
            return json.dumps({"type": "system", "subtype": "init"}) + "\n"

        record = self.run_bench("block", FakeProcesses(crashed_agent))
        self.assertIn("agent run not measured", record["error"])
        self.assertIn(
            "block | test | 0 (+1 error)", (self.out / "summary.md").read_text()
        )
        self.assertTrue(
            (self.out / "runs" / "block.test.1" / "transcript.jsonl").exists()
        )

    def test_an_agent_stopped_by_the_budget_cap_is_not_a_clean_run(self):
        def capped_agent(clone: Path) -> str:
            event = result_event("Stopped.")
            event["subtype"] = "error_max_budget_usd"
            return json.dumps(event) + "\n"

        record = self.run_bench("block", FakeProcesses(capped_agent))
        self.assertIn(
            "agent run not measured: exit 0, error_max_budget_usd", record["error"]
        )

    def test_errors_never_hold_secret_values(self):
        self.origin = self.origin.parent / f"missing-{SECRET}"
        record = self.run_bench("block", FakeProcesses(copying_agent))
        self.assertIn("git clone failed", record["error"])
        self.assertNotIn(SECRET, record["error"])
        self.assertNotIn(SECRET, (self.out / "results.jsonl").read_text())

    def test_out_must_be_outside_the_repository(self):
        with self.assertRaises(SystemExit):
            bench.validate_out(bench.REPO_ROOT / "scripts" / "trap-out")

    def test_plan_crosses_arms_models_and_reps(self):
        cfg = bench.Config(
            trap=trap_for(self.origin, self.pin),
            arms=[ARMS["prose"], ARMS["tool"]],
            models={"a": "model-a", "b": "model-b"},
            reps=2,
            out=self.out,
            archfit=FAKE_ARCHFIT,
            claude=FAKE_CLAUDE,
        )
        ids = [s.run_id for s in bench.plan_runs(cfg)]
        self.assertEqual(len(ids), 8)
        self.assertEqual(len(set(ids)), 8)
        self.assertEqual(ids[:2], ["prose.a.1", "tool.a.1"])


class ArgumentsTest(unittest.TestCase):
    def test_models_need_name_and_id(self):
        self.assertEqual(
            bench.parse_models("fable=claude-fable-5-2, haiku=claude-haiku-5-5"),
            {"fable": "claude-fable-5-2", "haiku": "claude-haiku-5-5"},
        )
        for bad in ("claude-fable-5-2", "fable=", ""):
            with self.subTest(bad), self.assertRaises(argparse.ArgumentTypeError):
                bench.parse_models(bad)

    def test_reps_and_jobs_must_be_positive(self):
        self.assertEqual(bench.positive_int("3"), 3)
        for bad in ("0", "-1"):
            with self.subTest(bad), self.assertRaises(argparse.ArgumentTypeError):
                bench.positive_int(bad)

    def test_arms_must_be_known(self):
        self.assertEqual(
            [a.name for a in bench.parse_arms("prose,tool")], ["prose", "tool"]
        )
        with self.assertRaises(argparse.ArgumentTypeError):
            bench.parse_arms("prose,nope")


if __name__ == "__main__":
    unittest.main()
