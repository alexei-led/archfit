#!/usr/bin/env python3
"""Validate Archfit 4 reports, change records, formats and determinism on a corpus.

Repositories must already have a reviewed version 3 policy. The sweep never
creates or converts a policy. Only missing repositories may be explicitly
accepted as unverified; a command error or malformed output always fails.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[2]
WORKSPACE_ROOT = Path.home() / "workspace"
DEFAULT_ARCHFIT = REPO_ROOT / ".bin/archfit"
DEFAULT_OUTPUT_DIR = Path("/tmp/archfit-corpus-eval")
DEFAULT_SUMMARY_FILE = Path("/tmp/archfit-corpus-results.json")
PINNED_RUST_TOOLCHAIN = "1.98.0"
COMMAND_TIMEOUT = 3600
WORK_DIR_MARKER = ".archfit-sweep-workdir"
VERDICT_EXITS = {"pass": 0, "fail": 1, "waiting": 1, "unknown": 2}
RESULT_SCHEMA = "archfit.result.v1"
STATE_SCHEMA_VERSION = "archfit.architecture-state.v1"


@dataclass(frozen=True)
class RepoSpec:
    label: str
    root: Path
    language: str


CORPUS = {
    label: RepoSpec(label, WORKSPACE_ROOT / directory, language)
    for label, directory, language in (
        ("spotinfo", "spotinfo", "go"),
        ("pumba", "pumba", "go"),
        ("omni/scheduled-tasks", "omni/server/services/scheduled-tasks", "go"),
        ("prometheus", "prometheus", "go"),
        ("ccgram", "ccgram", "python"),
        ("prefect", "prefect", "python"),
        ("storybook", "storybook", "ts"),
        ("yazi", "yazi", "rust"),
        ("herdr", "herdr", "rust"),
        ("ruff", "ruff", "rust"),
        ("tokio", "tokio", "rust"),
        ("bitwarden", "bitwarden-server", "dotnet"),
    )
}


def corpus_for_workspace(workspace_root: Path = WORKSPACE_ROOT) -> dict[str, RepoSpec]:
    return {
        label: RepoSpec(
            label, workspace_root / spec.root.relative_to(WORKSPACE_ROOT), spec.language
        )
        for label, spec in CORPUS.items()
    }


def parse_csv_list(raw: str | None) -> list[str]:
    return [item.strip() for item in (raw or "").split(",") if item.strip()]


def parse_allow_unverified(entries: list[str]) -> dict[str, str]:
    out = {}
    for entry in entries:
        label, sep, reason = entry.partition("=")
        if not sep or not label.strip() or not reason.strip():
            raise ValueError(f"--allow-unverified needs label=reason, got {entry!r}")
        out[label.strip()] = reason.strip()
    return out


def sanitize(label: str) -> str:
    return re.sub(r"[^a-zA-Z0-9_.-]", "_", label)


def expected_check_exit(verdict: str | None) -> int | None:
    return VERDICT_EXITS.get(verdict or "")


def prepare_work_dir(work: Path) -> Path:
    """Remove only a work directory this harness owns, never a symlink."""
    if work.is_symlink():
        raise RuntimeError(f"refusing to wipe symlink {work}")
    if work.exists():
        marker = work / WORK_DIR_MARKER
        if not marker.is_file() or marker.is_symlink():
            raise RuntimeError(f"refusing to wipe {work}: no {WORK_DIR_MARKER}")
        shutil.rmtree(work)
    work.mkdir(parents=True)
    (work / WORK_DIR_MARKER).write_text("", encoding="utf-8")
    return work


def command_environment(source: dict[str, str] | None = None) -> dict[str, str]:
    env = dict(os.environ if source is None else source)
    env.setdefault("RUSTUP_TOOLCHAIN", PINNED_RUST_TOOLCHAIN)
    return env


@dataclass(frozen=True)
class CommandResult:
    returncode: int
    stdout: str
    stderr: str


def subprocess_runner(cmd: list[str], cwd: Path) -> CommandResult:
    try:
        proc = subprocess.run(
            cmd,
            cwd=cwd,
            capture_output=True,
            check=False,
            env=command_environment(),
            timeout=COMMAND_TIMEOUT,
        )
        return CommandResult(
            proc.returncode,
            proc.stdout.decode("utf-8"),
            proc.stderr.decode("utf-8", "replace"),
        )
    except subprocess.TimeoutExpired as exc:
        return CommandResult(
            124,
            (exc.stdout or b"").decode("utf-8", "replace"),
            f"{(exc.stderr or b'').decode('utf-8', 'replace')}\nharness: timed out after {COMMAND_TIMEOUT}s",
        )


def validate_published_schema(doc: dict[str, Any], path: Path) -> None:
    import jsonschema

    schema = json.loads(path.read_text(encoding="utf-8"))
    jsonschema.Draft202012Validator(schema).validate(doc)


class Sweep:
    def __init__(
        self,
        archfit: Path,
        output_dir: Path,
        *,
        runner=subprocess_runner,
        cwd: Path = REPO_ROOT,
        validate_schema: bool = False,
    ) -> None:
        self.archfit, self.output_dir = archfit, output_dir
        self.runner, self.cwd, self.validate_schema = runner, cwd, validate_schema

    def run(self, args: list[str], log: Path) -> CommandResult:
        result = self.runner([str(self.archfit), *args], self.cwd)
        for suffix, value in (
            ("stdout", result.stdout),
            ("stderr", result.stderr),
            ("rc", str(result.returncode)),
        ):
            log.with_suffix("." + suffix).write_text(value, encoding="utf-8")
        return result

    def process(
        self, spec: RepoSpec, *, want_repeat: bool = False, want_formats: bool = False
    ) -> dict[str, Any]:
        record = {
            "label": spec.label,
            "root": str(spec.root),
            "language": spec.language,
            "status": "fail",
            "report_exit": None,
            "check_exit": None,
            "verdict": None,
            "failures": [],
            "unverified": None,
        }
        if not spec.root.is_dir():
            record["status"] = "unverified"
            record["unverified"] = {"reason": f"repository not present at {spec.root}"}
            return record
        try:
            work = self.output_dir / sanitize(spec.label)
            if spec.root.resolve().is_relative_to(work.resolve()):
                raise RuntimeError(
                    f"work directory {work} contains repository {spec.root}"
                )
            work = prepare_work_dir(work)
            if not (spec.root / ".archfit.yaml").is_file():
                raise ValueError(
                    "policy missing: run archfit init, review and commit it first"
                )
            root = ["-C", str(spec.root)]
            report = self.run(["report", *root, "--format", "json"], work / "report")
            record["report_exit"] = report.returncode
            if report.returncode != 0:
                raise ValueError(
                    f"report exited {report.returncode}: {report.stderr.strip()}"
                )
            state = json.loads(report.stdout)
            if (
                not isinstance(state, dict)
                or state.get("schema_version") != STATE_SCHEMA_VERSION
            ):
                raise ValueError("report did not emit architecture-state.v1")
            if self.validate_schema:
                validate_published_schema(
                    state, REPO_ROOT / "archfit.report.schema.json"
                )

            args = ["check", *root, "--format", "json"]
            check = self.run(args, work / "check")
            record["check_exit"] = check.returncode
            result = json.loads(check.stdout)
            if not isinstance(result, dict) or result.get("schema") != RESULT_SCHEMA:
                raise ValueError("check did not emit archfit.result.v1")
            record["verdict"] = result.get("status")
            want = expected_check_exit(record["verdict"])
            if want is None or check.returncode != want:
                raise ValueError(
                    f"check status {record['verdict']!r}: exit {check.returncode}, expected {want}"
                )
            if self.validate_schema:
                validate_published_schema(
                    result, REPO_ROOT / "archfit.result.schema.json"
                )
            if result["status"] == "unknown":
                record["failures"].append("check is unknown: no verdict")
            if want_repeat:
                repeat = self.run(args, work / "check-repeat")
                if (
                    repeat.returncode != check.returncode
                    or repeat.stdout != check.stdout
                ):
                    record["failures"].append("repeated check is not byte-identical")
            if want_formats:
                for name in ("text", "sarif"):
                    output = self.run(
                        ["check", *root, "--format", name], work / f"check-{name}"
                    )
                    if output.returncode != want:
                        record["failures"].append(
                            f"{name}: exit {output.returncode}, expected {want}"
                        )
                    if name == "sarif":
                        sarif = json.loads(output.stdout)
                        if sarif.get("version") != "2.1.0":
                            record["failures"].append("not SARIF 2.1.0")
                    elif f"archfit: {result['status']}" not in output.stdout:
                        record["failures"].append(
                            f"text lacks status {result['status']}"
                        )
            if not record["failures"]:
                record["status"] = "pass"
        except Exception as exc:
            record["failures"].append(str(exc))
        return record


def finalize_status(
    record: dict[str, Any], allow_unverified: dict[str, str]
) -> dict[str, Any]:
    if record["failures"]:
        record["status"] = "fail"
    elif record["status"] == "unverified" and record["label"] in allow_unverified:
        record["status"] = "accepted_unverified"
        record["unverified"]["accepted_reason"] = allow_unverified[record["label"]]
    return record


def strict_exit_code(records: list[dict[str, Any]]) -> int:
    return (
        0
        if records
        and all(r["status"] in {"pass", "accepted_unverified"} for r in records)
        else 1
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archfit", default=str(DEFAULT_ARCHFIT))
    parser.add_argument("--workspace-root", type=Path, default=WORKSPACE_ROOT)
    parser.add_argument("--repos", default=",".join(CORPUS))
    parser.add_argument("--repeat-repos", default="")
    parser.add_argument("--format-repos", default="")
    parser.add_argument(
        "--allow-unverified", action="append", default=[], metavar="LABEL=REASON"
    )
    parser.add_argument("--strict", action="store_true")
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    parser.add_argument("--summary-file", type=Path, default=DEFAULT_SUMMARY_FILE)
    parser.add_argument("--max-workers", type=int, default=4)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        import jsonschema  # noqa: F401

        corpus = corpus_for_workspace(args.workspace_root.expanduser().resolve())
        selected = parse_csv_list(args.repos)
        if not selected or set(selected) - set(corpus):
            raise ValueError("--repos must select known corpus labels")
        repeats, formats = (
            set(parse_csv_list(args.repeat_repos)),
            set(parse_csv_list(args.format_repos)),
        )
        if (repeats | formats) - set(selected):
            raise ValueError("repeat/format labels must be selected by --repos")
        allowed = parse_allow_unverified(args.allow_unverified)
        archfit = Path(args.archfit).resolve()
        if not archfit.is_file():
            raise ValueError(f"archfit binary not found at {archfit}")
    except (ImportError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    args.output_dir.mkdir(parents=True, exist_ok=True)
    args.summary_file.parent.mkdir(parents=True, exist_ok=True)
    sweep = Sweep(archfit, args.output_dir, validate_schema=True)

    def process(label: str) -> dict[str, Any]:
        return finalize_status(
            sweep.process(
                corpus[label],
                want_repeat=label in repeats,
                want_formats=label in formats,
            ),
            allowed,
        )

    with ThreadPoolExecutor(max_workers=max(1, args.max_workers)) as pool:
        records = sorted(
            pool.map(process, selected), key=lambda record: record["label"]
        )
    args.summary_file.write_text(json.dumps(records, indent=2), encoding="utf-8")
    for record in records:
        print(json.dumps(record), flush=True)
    return strict_exit_code(records) if args.strict else 0


if __name__ == "__main__":
    raise SystemExit(main())
