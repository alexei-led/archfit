#!/usr/bin/env python3
"""Run archfit v1 architecture-state corpus sweeps against the branch binary.

The sweep is the product-level acceptance gate for the v1 cutover: unit
fixtures and self-dogfooding cannot prove that extraction,
state projection, format rendering, and exit semantics work on real Go,
TypeScript, Python, and Rust repositories.

Contract this harness enforces per repo (see
docs/plans/architecture-state-reporting.md, Task 6):

- the target's config is schema v2 and loads successfully;
- `analyze --json` exits 0 and validates `archfit.architecture-state.v1`: nine
  dimensions, coverage counts summing to nine, and typed metric arrays;
- `check` exits exactly what its own verdict says: healthy 0, needs-attention 2,
  blocked 1;
- every selected format reports the same verdict, dimension statuses, coverage
  split, comparison state, and canonical finding sequence;
- a repeated `analyze --json` is byte-identical.

Target repositories are read-only. The migration candidate is written to the
sweep output directory, never into the target tree. An AI-summary overlay is a
SEPARATE temp copy and can never be a delivery candidate.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
from collections.abc import Callable
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[2]
WORKSPACE_ROOT = Path.home() / "workspace"
DEFAULT_ARCHFIT = REPO_ROOT / ".bin" / "archfit"
DEFAULT_OUTPUT_DIR = Path("/tmp/archfit-corpus-eval")
DEFAULT_SUMMARY_FILE = Path("/tmp/archfit-corpus-results.json")
# Keep Rust corpus results reproducible without modifying third-party repos.
# Callers can override the pin explicitly through RUSTUP_TOOLCHAIN.
PINNED_RUST_TOOLCHAIN = "1.98.0"

# The contract constants are duplicated from the Go model on purpose: the
# harness is an INDEPENDENT check, and importing the values it verifies would
# make the assertions vacuous.
STATE_SCHEMA_VERSION = "archfit.architecture-state.v1"
DIMENSION_KEYS = (
    "intent",
    "structure",
    "modularity",
    "coupling",
    "change_locality",
    "complexity",
    "testability",
    "operations",
    "drift",
)
# Independent copy of the source-fixed facts that may remain unknown without
# weakening a measured dimension's claim. Any other unknown is conservatively
# in-claim (or undeclared) and must prevent promotion.
OUT_OF_CLAIM_UNKNOWN_FACTS = {
    "intent": frozenset({"disabled rule conformance"}),
    "structure": frozenset({"external dependency structure"}),
    "modularity": frozenset({"inferred public surface"}),
    "coupling": frozenset({"local and undeclared-external coupling"}),
    "change_locality": frozenset({"essential vs accidental volatility"}),
    "complexity": frozenset(
        {"code size tail", "function length distribution", "cognitive complexity"}
    ),
    "testability": frozenset({"assertion quality", "boundary test semantics"}),
    "operations": frozenset(
        {"observed runtime topology", "supply-chain inventory", "analyzer health"}
    ),
    "drift": frozenset({"base comparison"}),
}
# check exit code per state verdict (application.outcomeFor).
VERDICT_EXITS = {"healthy": 0, "needs_attention": 2, "blocked": 1}

FORMATS = ("json", "text", "markdown", "sarif", "scorecard")

STATUS_PASS = "pass"
STATUS_FAIL = "fail"
STATUS_UNVERIFIED = "unverified"
STATUS_ACCEPTED_UNVERIFIED = "accepted_unverified"


@dataclass(frozen=True)
class RepoSpec:
    label: str
    root: Path
    language: str


CORPUS: dict[str, RepoSpec] = {
    "spotinfo": RepoSpec("spotinfo", WORKSPACE_ROOT / "spotinfo", "go"),
    "pumba": RepoSpec("pumba", WORKSPACE_ROOT / "pumba", "go"),
    "omni/scheduled-tasks": RepoSpec(
        "omni/scheduled-tasks",
        WORKSPACE_ROOT / "omni/server/services/scheduled-tasks",
        "go",
    ),
    "prometheus": RepoSpec("prometheus", WORKSPACE_ROOT / "prometheus", "go"),
    "ccgram": RepoSpec("ccgram", WORKSPACE_ROOT / "ccgram", "python"),
    "prefect": RepoSpec("prefect", WORKSPACE_ROOT / "prefect", "python"),
    "storybook": RepoSpec("storybook", WORKSPACE_ROOT / "storybook", "ts"),
    "yazi": RepoSpec("yazi", WORKSPACE_ROOT / "yazi", "rust"),
    "herdr": RepoSpec("herdr", WORKSPACE_ROOT / "herdr", "rust"),
    "ruff": RepoSpec("ruff", WORKSPACE_ROOT / "ruff", "rust"),
    "tokio": RepoSpec("tokio", WORKSPACE_ROOT / "tokio", "rust"),
}


# --------------------------------------------------------------------------
# Pure helpers. Everything below this line up to `Sweep` is free of subprocess
# and filesystem access so the unit tests can drive it directly.
# --------------------------------------------------------------------------


def parse_csv_list(raw: str | None) -> list[str]:
    """Split a comma-separated flag value. An empty value is an empty list."""
    if not raw:
        return []
    return [item.strip() for item in raw.split(",") if item.strip()]


def parse_allow_unverified(entries: list[str]) -> dict[str, str]:
    """Parse `--allow-unverified label=reason` pairs.

    A reason is mandatory: an accepted gap with no stated reason is an
    undisclosed gap, which is exactly what strict mode exists to prevent.
    """
    out: dict[str, str] = {}
    for entry in entries:
        label, sep, reason = entry.partition("=")
        label, reason = label.strip(), reason.strip()
        if not sep or not label or not reason:
            raise ValueError(f"--allow-unverified needs label=reason, got {entry!r}")
        out[label] = reason
    return out


def sanitize(label: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]+", "_", label)


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def blank_record(label: str, root: str, language: str) -> dict[str, Any]:
    """Return the frozen per-repo record with every field explicitly absent.

    The shape is the contract asserted by the plan's jq gates. Absent values are
    explicit `null` or empty arrays so a consumer never has to distinguish
    "missing key" from "not measured".
    """
    return {
        "label": label,
        "root": root,
        "language": language,
        "status": STATUS_UNVERIFIED,
        "failures": [],
        "unverified": None,
        "config": {
            "source": None,
            "source_config_sha256": None,
            "target_head": None,
        },
        "analyze": {
            "exit": None,
            "schema_version": None,
            "verdict": None,
            "dimension_keys": [],
        },
        "check": {"exit": None, "verdict": None},
        "formats": {name: {"exit": None, "parity": None} for name in FORMATS},
        "determinism": {"json_byte_identical": None},
        "ai": {"requested": False, "exit": None},
    }


def validate_state(doc: Any) -> list[str]:
    """Validate one `analyze --json` document against the v1 state contract."""
    failures: list[str] = []
    if not isinstance(doc, dict):
        return [f"analyze json is {type(doc).__name__}, want an object"]

    if doc.get("schema_version") != STATE_SCHEMA_VERSION:
        failures.append(
            f"schema_version={doc.get('schema_version')!r}, want {STATE_SCHEMA_VERSION!r}"
        )
    verdict = doc.get("verdict")
    if verdict not in VERDICT_EXITS:
        failures.append(f"verdict={verdict!r} is not one of {sorted(VERDICT_EXITS)}")

    # A repository scalar is the exact claim v1 removed. Its return would be
    # invisible to every other check here, so name it explicitly.
    for retired in ("score", "score_overall", "score_band"):
        if retired in doc:
            failures.append(
                f"retired repository scalar {retired!r} is back in the state"
            )

    dims = doc.get("dimensions")
    if not isinstance(dims, dict):
        failures.append("dimensions is missing or not an object")
        return failures
    if list(dims.keys()) != list(DIMENSION_KEYS):
        failures.append(
            f"dimension keys are {list(dims.keys())}, want {list(DIMENSION_KEYS)}"
        )
    failures.extend(_validate_dimensions(dims))

    coverage = doc.get("coverage")
    if not isinstance(coverage, dict):
        failures.append("coverage is missing or not an object")
    else:
        # Required, not defaulted. Defaulting a missing key to 0 made the sum
        # and the dimensions-agree check pass on a document that omitted the
        # count entirely — an independent harness cannot supply the value it
        # exists to verify.
        missing = [
            k for k in ("measured", "partial", "unmeasured") if not isinstance(coverage.get(k), int)
        ]
        if missing:
            failures.append(f"coverage block is missing integer counts for {missing}")
        else:
            declared = (
                coverage["measured"],
                coverage["partial"],
                coverage["unmeasured"],
            )
            counted = sum(declared)
            if counted != len(DIMENSION_KEYS):
                failures.append(
                    f"coverage counts sum to {counted}, want {len(DIMENSION_KEYS)}"
                )
            observed = _status_counts(dims)
            if observed != declared:
                failures.append(
                    f"coverage block says {declared} but the dimensions are {observed}"
                )

    for key in ("findings", "agent_tasks", "seams"):
        if not isinstance(doc.get(key), list):
            failures.append(f"{key} is missing or not an array")

    comparison = doc.get("comparison")
    if not isinstance(comparison, dict):
        failures.append("comparison is missing or not an object")
    elif comparison.get("status") not in (
        "comparable",
        "non_comparable",
        "not_requested",
    ):
        failures.append(
            f"comparison.status={comparison.get('status')!r} is not a known status"
        )

    return failures


def _status_counts(dims: dict[str, Any]) -> tuple[int, int, int]:
    measured = partial = unmeasured = 0
    for dim in dims.values():
        status = dim.get("status") if isinstance(dim, dict) else None
        if status == "measured":
            measured += 1
        elif status == "partial":
            partial += 1
        else:
            unmeasured += 1
    return measured, partial, unmeasured


def _validate_dimensions(dims: dict[str, Any]) -> list[str]:
    failures: list[str] = []
    for name, dim in dims.items():
        if not isinstance(dim, dict):
            failures.append(f"dimension {name} is not an object")
            continue
        if dim.get("name") != name:
            failures.append(f"dimension {name} carries name={dim.get('name')!r}")
        if not dim.get("owner"):
            failures.append(f"dimension {name} has no evidence owner")
        status = dim.get("status")
        if status not in ("measured", "partial", "unmeasured"):
            failures.append(f"dimension {name} status={status!r}")

        unknown = dim.get("unknown")
        if not isinstance(unknown, list):
            failures.append(f"dimension {name} unknown is not an array")
        else:
            for fact in unknown:
                if not isinstance(fact, dict):
                    failures.append(f"dimension {name} has an untyped unknown fact")
                    continue
                missing = [
                    key for key in ("fact", "reason", "owner") if not fact.get(key)
                ]
                if missing:
                    failures.append(
                        f"dimension {name} unknown fact is missing {missing}"
                    )
            if status == "measured":
                allowed = OUT_OF_CLAIM_UNKNOWN_FACTS.get(name, frozenset())
                in_claim = [
                    fact.get("fact")
                    for fact in unknown
                    if isinstance(fact, dict) and fact.get("fact") not in allowed
                ]
                if in_claim:
                    failures.append(
                        f"dimension {name} is measured with in-claim or undeclared unknown facts {in_claim}"
                    )
            elif status == "partial" and not unknown:
                failures.append(
                    f"dimension {name} is partial without a named unknown fact"
                )

        metrics = dim.get("metrics")
        if not isinstance(metrics, list):
            failures.append(f"dimension {name} metrics is not an array")
            continue
        for metric in metrics:
            if not isinstance(metric, dict):
                failures.append(f"dimension {name} has an untyped metric entry")
                continue
            missing = [k for k in ("name", "value", "unit") if k not in metric]
            if missing:
                failures.append(
                    f"dimension {name} metric {metric.get('name')!r} is missing {missing}"
                )
                continue
            # Presence is not the contract: MetricValue.Value is a float64 and
            # Unit a string, so a metric carrying a band name where a number
            # belongs is a schema break the key check cannot see.
            if not isinstance(metric["value"], (int, float)) or isinstance(
                metric["value"], bool
            ):
                failures.append(
                    f"dimension {name} metric {metric['name']!r} value is {metric['value']!r}, want a number"
                )
            if not isinstance(metric["unit"], str):
                failures.append(
                    f"dimension {name} metric {metric['name']!r} unit is {metric['unit']!r}, want a string"
                )
    return failures


def expected_check_exit(verdict: str | None) -> int | None:
    return VERDICT_EXITS.get(verdict or "")


def canonical_finding_ids(doc: dict[str, Any]) -> list[str]:
    return [
        f.get("id", "") for f in doc.get("findings", []) or [] if isinstance(f, dict)
    ]


COVERAGE_TRIPLE_RE = re.compile(
    r"(\d+)\s*(?:measured)\D+?(\d+)\s*(?:partial)\D+?(\d+)\s*(?:unmeasured)"
)


def rendered_coverage_triple(out: str) -> tuple[int, int, int] | None:
    """Parse the coverage headline a human format printed.

    Parsing the numbers off their own labels, rather than testing whether three
    strings appear in order somewhere, is what makes this an assertion: a
    dimension row carrying "63/70" used to satisfy a wanted (6, 3, 0).
    """
    m = COVERAGE_TRIPLE_RE.search(out)
    if m is None:
        return None
    return (int(m.group(1)), int(m.group(2)), int(m.group(3)))


def dimension_line(out: str, dim_name: str) -> str | None:
    """Return the rendered line that names dim_name, or None.

    Every human format puts a dimension's name, status, and gate on one line
    (text and scorecard as a heading, Markdown as a table row), so the line is
    the unit a parity assertion can be scoped to.
    """
    for line in out.splitlines():
        if re.search(rf"\b{re.escape(dim_name)}\b", line):
            return line
    return None


def check_finding_sequence(out: str, canonical: list[str]) -> list[str]:
    """Check the canonical finding appendix in a rendered human format.

    Only the LAST occurrence of each ID is ordered: a format may lead with an
    actionable, severity-sorted excerpt, but the appendix that closes it must
    carry every finding in the document's canonical order.
    """
    failures: list[str] = []
    previous = -1
    for fid in canonical:
        at = out.rfind(fid)
        if at < 0:
            failures.append(f"finding {fid} is absent from the finding index")
            return failures
        if at <= previous:
            failures.append(
                f"finding {fid} is out of canonical order in the finding index"
            )
            return failures
        previous = at
    return failures


def check_text_parity(name: str, out: str, state: dict[str, Any]) -> list[str]:
    """Check one human format against the state JSON it must agree with."""
    failures: list[str] = []
    verdict = state.get("verdict") or ""
    label = verdict.replace("_", " ").upper()
    if label and label not in out:
        failures.append(f"{name} does not report the verdict {label!r}")

    decision = state.get("decision") or {}
    hard_gates = decision.get("hard_gates")
    if hard_gates and hard_gates not in out:
        failures.append(f"{name} does not report hard gates {hard_gates!r}")

    for dim_name, dim in (state.get("dimensions") or {}).items():
        line = dimension_line(out, dim_name)
        if line is None:
            failures.append(f"{name} omits dimension {dim_name}")
            continue
        # The status and gate must sit on the DIMENSION'S OWN line. Searching
        # the whole document can never fail: nine dimensions share three status
        # words, and the coverage headline prints all three of them, so a format
        # that drops a row still finds the word somewhere else.
        for field in ("status", "gate"):
            want = dim.get(field)
            if want and not re.search(rf"\b{re.escape(str(want))}\b", line):
                failures.append(
                    f"{name} reports {dim_name} without {field}={want!r}: {line.strip()!r}"
                )

    coverage = state.get("coverage") or {}
    missing_coverage = [
        k for k in ("measured", "partial", "unmeasured") if k not in coverage
    ]
    if missing_coverage:
        failures.append(f"state coverage block is missing {missing_coverage}")
    else:
        triple = (
            int(coverage["measured"]),
            int(coverage["partial"]),
            int(coverage["unmeasured"]),
        )
        rendered = rendered_coverage_triple(out)
        if rendered is None:
            failures.append(f"{name} renders no coverage split, want {triple}")
        elif rendered != triple:
            failures.append(f"{name} reports the coverage split {rendered}, want {triple}")

    comparison_status = (state.get("comparison") or {}).get("status")
    # Word-boundary match, not `in`: "comparable" is a substring of
    # "non_comparable", so a plain containment test passes when the renderer
    # prints the OPPOSITE status — exactly the contradiction this check exists
    # to catch. `_` is a word character, so \b does not fire mid-token.
    if comparison_status and not re.search(
        rf"\b{re.escape(comparison_status)}\b", out
    ):
        failures.append(f"{name} omits the comparison status {comparison_status!r}")

    failures.extend(
        f"{name}: {failure}"
        for failure in check_finding_sequence(out, canonical_finding_ids(state))
    )
    return failures


def check_sarif_parity(raw: str, state: dict[str, Any]) -> list[str]:
    """Check SARIF's fact parity. SARIF is exempt from layout, not from facts."""
    failures: list[str] = []
    try:
        log = json.loads(raw)
    except json.JSONDecodeError as exc:
        return [f"sarif is not valid json: {exc}"]
    runs = log.get("runs") or []
    if len(runs) != 1:
        return [f"sarif has {len(runs)} runs, want 1"]
    props = runs[0].get("properties") or {}

    if props.get("verdict") != state.get("verdict"):
        failures.append(
            f"sarif verdict={props.get('verdict')!r}, state says {state.get('verdict')!r}"
        )
    for key in ("hard_gates", "active_blockers"):
        want = (state.get("decision") or {}).get(key)
        got = (props.get("decision") or {}).get(key)
        if got != want:
            failures.append(f"sarif decision.{key}={got!r}, state says {want!r}")
    for key in ("measured", "partial", "unmeasured"):
        want = (state.get("coverage") or {}).get(key)
        got = (props.get("coverage") or {}).get(key)
        if got != want:
            failures.append(f"sarif coverage.{key}={got!r}, state says {want!r}")
    dims = props.get("dimensions") or []
    if len(dims) != len(DIMENSION_KEYS):
        failures.append(
            f"sarif carries {len(dims)} dimensions, want {len(DIMENSION_KEYS)}"
        )

    fingerprints = {
        (r.get("fingerprints") or {}).get("archfit/v1")
        for r in runs[0].get("results") or []
    }
    for fid in canonical_finding_ids(state):
        if fid not in fingerprints:
            failures.append(f"sarif has no result fingerprinted {fid}")
            break
    return failures


def strict_exit_code(records: list[dict[str, Any]]) -> int:
    """Strict mode returns 0 only when every record passes or is an accepted gap."""
    if not records:
        # Nothing measured is not a pass. Without this a selection that produced
        # no records exits 0 and reads as a green sweep.
        return 1
    for record in records:
        if record.get("status") in (STATUS_PASS, STATUS_ACCEPTED_UNVERIFIED):
            continue
        return 1
    return 0


def finalize_status(
    record: dict[str, Any], allow_unverified: dict[str, str]
) -> dict[str, Any]:
    """Resolve a record's terminal status from its failures and allow-list.

    An allowed gap NEVER becomes a pass: `accepted_unverified` is a disclosed
    hole in the evidence, and the final review has to be able to see it.
    """
    if record["status"] == STATUS_UNVERIFIED:
        reason = allow_unverified.get(record["label"])
        if reason:
            record["status"] = STATUS_ACCEPTED_UNVERIFIED
            record["unverified"] = {"reason": reason}
        return record
    record["status"] = STATUS_FAIL if record["failures"] else STATUS_PASS
    return record


# --------------------------------------------------------------------------
# Command execution.
# --------------------------------------------------------------------------


@dataclass(frozen=True)
class CommandResult:
    returncode: int
    stdout: str
    stderr: str


Runner = Callable[[list[str], Path], CommandResult]


# WORK_DIR_MARKER names a directory this sweep created. process() wipes its work
# directory before each run, and the work path is `--output-dir / <repo label>` —
# labels that are also the directory names the corpus itself lives under. Without
# an ownership marker, `--output-dir ~/workspace` rmtree'd the repositories the
# sweep was measuring.
WORK_DIR_MARKER = ".archfit-sweep-workdir"


def prepare_work_dir(work: Path) -> Path:
    """Create (or re-create) a work directory this sweep owns.

    Refuses to delete anything the sweep did not create.
    """
    if work.exists():
        if not (work / WORK_DIR_MARKER).exists():
            raise RuntimeError(
                f"refusing to wipe {work}: not a sweep work directory "
                f"(no {WORK_DIR_MARKER}). Choose an empty --output-dir."
            )
        shutil.rmtree(work)
    work.mkdir(parents=True, exist_ok=True)
    (work / WORK_DIR_MARKER).write_text("", encoding="utf-8")
    return work


# COMMAND_TIMEOUT bounds one archfit invocation. The sweep runs repos
# concurrently with --progress=none, so a hung analyze occupies a worker
# silently and forever; a recorded timeout is a finding, a hang is not.
COMMAND_TIMEOUT = 3600


def command_environment(source: dict[str, str] | None = None) -> dict[str, str]:
    """Return the subprocess environment with the owned Rust corpus pin."""
    env = dict(os.environ if source is None else source)
    env.setdefault("RUSTUP_TOOLCHAIN", PINNED_RUST_TOOLCHAIN)
    return env


def subprocess_runner(cmd: list[str], cwd: Path) -> CommandResult:
    # Capture BYTES and decode UTF-8 explicitly. text=True decodes with the
    # locale encoding, and archfit's output is UTF-8 (em dashes, ×, →) — under a
    # non-UTF-8 locale that raised UnicodeDecodeError and turned a healthy
    # storybook run into a harness crash.
    try:
        proc = subprocess.run(
            cmd,
            cwd=str(cwd),
            capture_output=True,
            check=False,
            env=command_environment(),
            timeout=COMMAND_TIMEOUT,
        )
    except subprocess.TimeoutExpired as exc:
        out = exc.stdout.decode("utf-8", "replace") if exc.stdout else ""
        err = exc.stderr.decode("utf-8", "replace") if exc.stderr else ""
        return CommandResult(
            124, out, f"{err}\nharness: timed out after {COMMAND_TIMEOUT}s"
        )
    return CommandResult(
        proc.returncode,
        proc.stdout.decode("utf-8"),
        proc.stderr.decode("utf-8", "replace"),
    )


class Sweep:
    """One sweep run. The runner is injected so tests never exec the binary."""

    def __init__(
        self,
        archfit: Path,
        output_dir: Path,
        *,
        runner: Runner = subprocess_runner,
        cwd: Path = REPO_ROOT,
    ) -> None:
        self.archfit = archfit
        self.output_dir = output_dir
        self.runner = runner
        self.cwd = cwd

    def run(self, args: list[str], log: Path | None = None) -> CommandResult:
        result = self.runner([str(self.archfit), *args], self.cwd)
        if log is not None:
            log.with_suffix(log.suffix + ".stdout").write_text(
                result.stdout, encoding="utf-8"
            )
            log.with_suffix(log.suffix + ".stderr").write_text(
                result.stderr, encoding="utf-8"
            )
        return result

    def git_head(self, root: Path) -> str | None:
        result = self.runner(["git", "-C", str(root), "rev-parse", "HEAD"], self.cwd)
        return result.stdout.strip() if result.returncode == 0 else None

    # -- config ------------------------------------------------------------

    def prepare_config(
        self, spec: RepoSpec, work: Path, record: dict[str, Any]
    ) -> Path | None:
        """Stage a copy of the repository config for one repo.

        The target's own `.archfit.yaml` is copied out. The target tree is
        never written to.
        """
        cfg = work / "archfit.yaml"
        source = spec.root / ".archfit.yaml"
        if source.exists():
            raw = source.read_bytes()
            record["config"]["source"] = "copied-existing"
            record["config"]["source_config_sha256"] = sha256_bytes(raw)
            cfg.write_bytes(raw)
        else:
            init = self.run(
                ["config", "init", "--root", str(spec.root), "--output", str(cfg)],
                log=work / "config-init",
            )
            record["config"]["source"] = "init"
            if init.returncode != 0 or not cfg.exists():
                record["failures"].append(f"config init exited {init.returncode}")
                return None
        return cfg

    def analyze_json(
        self, cfg: Path, spec: RepoSpec, work: Path, name: str
    ) -> CommandResult:
        return self.run(
            [
                "analyze",
                "--format=json",
                "--progress=none",
                "-c",
                str(cfg),
                "--root",
                str(spec.root),
            ],
            log=work / name,
        )

    def collect_analyze(
        self, cfg: Path, spec: RepoSpec, work: Path, record: dict[str, Any]
    ) -> dict[str, Any] | None:
        result = self.analyze_json(cfg, spec, work, "analyze")
        record["analyze"]["exit"] = result.returncode
        record["formats"]["json"]["exit"] = result.returncode
        if result.returncode != 0:
            record["failures"].append(f"analyze --json exited {result.returncode}")
            return None
        try:
            state = json.loads(result.stdout)
        except json.JSONDecodeError as exc:
            record["failures"].append(f"analyze --json is not valid json: {exc}")
            return None

        # `null`, an array, and a scalar all decode cleanly, so the shape guard
        # has to run BEFORE the first .get — otherwise the AttributeError is
        # caught by the outer sweep handler and recorded as a harness error,
        # masking the schema violation validate_state would have named.
        if not isinstance(state, dict):
            record["failures"].extend(validate_state(state))
            record["formats"]["json"]["parity"] = False
            return None

        record["analyze"]["schema_version"] = state.get("schema_version")
        record["analyze"]["verdict"] = state.get("verdict")
        dims = state.get("dimensions")
        record["analyze"]["dimension_keys"] = (
            list(dims.keys()) if isinstance(dims, dict) else []
        )
        failures = validate_state(state)
        record["failures"].extend(failures)
        record["formats"]["json"]["parity"] = not failures
        return state

    def collect_check(
        self,
        cfg: Path,
        spec: RepoSpec,
        work: Path,
        record: dict[str, Any],
        state: dict[str, Any],
    ) -> None:
        result = self.run(
            [
                "check",
                "--format=json",
                "--progress=none",
                "-c",
                str(cfg),
                "--root",
                str(spec.root),
            ],
            log=work / "check",
        )
        record["check"]["exit"] = result.returncode
        try:
            doc = json.loads(result.stdout)
        except json.JSONDecodeError:
            doc = None
        # `null`, an array, and a scalar all decode cleanly; .get on any of them
        # raises past the decode guard and is recorded as a harness error,
        # masking the real defect.
        record["check"]["verdict"] = doc.get("verdict") if isinstance(doc, dict) else None

        verdict = record["check"]["verdict"]
        if verdict is None:
            record["failures"].append("check --json produced no readable verdict")
            return
        want = expected_check_exit(verdict)
        if want is None:
            record["failures"].append(f"check reported an unknown verdict {verdict!r}")
        elif result.returncode != want:
            record["failures"].append(
                f"check verdict {verdict} maps to exit {want}, got {result.returncode}"
            )
        if state.get("verdict") and verdict != state.get("verdict"):
            record["failures"].append(
                f"check verdict {verdict!r} disagrees with analyze {state.get('verdict')!r}"
            )

    def collect_formats(
        self,
        cfg: Path,
        spec: RepoSpec,
        work: Path,
        record: dict[str, Any],
        state: dict[str, Any],
    ) -> None:
        for name in FORMATS:
            if name == "json":
                continue
            result = self.run(
                [
                    "analyze",
                    f"--format={name}",
                    "--progress=none",
                    "-c",
                    str(cfg),
                    "--root",
                    str(spec.root),
                ],
                log=work / f"analyze-{name}",
            )
            record["formats"][name]["exit"] = result.returncode
            if result.returncode != 0:
                record["formats"][name]["parity"] = False
                record["failures"].append(
                    f"analyze --format={name} exited {result.returncode}"
                )
                continue
            if name == "sarif":
                failures = check_sarif_parity(result.stdout, state)
            else:
                failures = check_text_parity(name, result.stdout, state)
            record["formats"][name]["parity"] = not failures
            record["failures"].extend(failures)

    def collect_determinism(
        self, cfg: Path, spec: RepoSpec, work: Path, record: dict[str, Any]
    ) -> None:
        """Repeat `analyze --json` and compare BYTES, not parsed values.

        The primary contract carries no wall-clock or run-local field, so there
        is nothing to exclude: a byte difference here is a real defect. The
        second run also reads a warm fact cache the first one filled, which is
        exactly the comparison worth making.
        """
        first = (work / "analyze.stdout").read_bytes()
        result = self.analyze_json(cfg, spec, work, "analyze-repeat")
        if result.returncode != 0:
            record["determinism"]["json_byte_identical"] = False
            record["failures"].append(
                f"repeat analyze --json exited {result.returncode}"
            )
            return
        identical = result.stdout.encode("utf-8") == first
        record["determinism"]["json_byte_identical"] = identical
        if not identical:
            record["failures"].append("repeat analyze --json is not byte-identical")

    def collect_ai(
        self, cfg: Path, spec: RepoSpec, work: Path, record: dict[str, Any]
    ) -> None:
        """Run the AI summary over a SEPARATE copy of the candidate.

        The overlay adds an `ai:` block, which changes the config bytes. Running
        it on the delivery candidate would make the delivered file depend on
        whether AI was selected during the sweep.
        """
        overlay = work / "archfit-ai.yaml"
        overlay.write_text(
            ensure_ai_block(cfg.read_text(encoding="utf-8")), encoding="utf-8"
        )
        record["ai"]["requested"] = True
        # The exit is RECORDED, never appended to failures. A credential or
        # provider failure is an environment fact, and strict mode grades
        # deterministic behaviour only. An AI-path schema or projection defect
        # still surfaces, because it lands in the deterministic checks above.
        result = self.run(
            [
                "analyze",
                "--ai-summary",
                "--format=markdown",
                "--progress=none",
                "-c",
                str(overlay),
                "--root",
                str(spec.root),
            ],
            log=work / "analyze-ai",
        )
        record["ai"]["exit"] = result.returncode

    # -- per repo ----------------------------------------------------------

    def process(
        self, spec: RepoSpec, *, want_ai: bool, want_repeat: bool, want_formats: bool
    ) -> dict[str, Any]:
        record = blank_record(spec.label, str(spec.root), spec.language)
        if not spec.root.exists():
            record["unverified"] = {"reason": f"repository not present at {spec.root}"}
            return record

        work = self.output_dir / sanitize(spec.label)
        # Grade the record FAIL before the first fallible step. A harness error
        # is a failure, not an unmeasured repo: leaving the status `unverified`
        # let finalize_status promote an allow-listed label to
        # accepted_unverified, discarding the recorded failure, and a strict
        # sweep exited 0 over a repo nothing had analysed.
        record["status"] = STATUS_FAIL  # resolved by finalize_status once checks ran
        try:
            work = prepare_work_dir(work)
        except RuntimeError as exc:
            record["failures"].append(str(exc))
            return record

        record["config"]["target_head"] = self.git_head(spec.root)

        cfg = self.prepare_config(spec, work, record)
        if cfg is None:
            return record
        state = self.collect_analyze(cfg, spec, work, record)
        if state is not None:
            self.collect_check(cfg, spec, work, record, state)
            if want_formats:
                self.collect_formats(cfg, spec, work, record, state)
        if want_repeat:
            self.collect_determinism(cfg, spec, work, record)
        if want_ai:
            self.collect_ai(cfg, spec, work, record)
        return record


def ensure_ai_block(text: str) -> str:
    if re.search(r"(?m)^ai:\s*$", text):
        return text
    return text.rstrip() + "\nai:\n  provider: anthropic\n  model: claude-opus-4-8\n"


def print_progress(payload: dict[str, Any]) -> None:
    print(json.dumps(payload, sort_keys=True), flush=True)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--archfit", default=str(DEFAULT_ARCHFIT), help="Path to the archfit binary"
    )
    parser.add_argument(
        "--repos", default=",".join(CORPUS), help="Comma-separated corpus labels"
    )
    parser.add_argument(
        "--ai-repos", default="", help="Labels to run analyze --ai-summary on"
    )
    parser.add_argument(
        "--repeat-repos", default="", help="Labels to check byte determinism on"
    )
    parser.add_argument(
        "--format-repos", default="", help="Labels to check format parity on"
    )
    parser.add_argument(
        "--allow-unverified",
        action="append",
        default=[],
        metavar="LABEL=REASON",
        help="Record a missing repo as accepted_unverified with a disclosed reason",
    )
    parser.add_argument(
        "--strict", action="store_true", help="Exit nonzero unless every record passes"
    )
    parser.add_argument("--output-dir", default=str(DEFAULT_OUTPUT_DIR))
    parser.add_argument("--summary-file", default=str(DEFAULT_SUMMARY_FILE))
    parser.add_argument("--max-workers", type=int, default=4)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)

    archfit = Path(args.archfit).resolve()
    if not archfit.exists():
        print(f"error: archfit binary not found at {archfit}", file=sys.stderr)
        return 1
    try:
        allow_unverified = parse_allow_unverified(args.allow_unverified)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    selected = parse_csv_list(args.repos)
    unknown = sorted(set(selected) - set(CORPUS))
    if unknown:
        print(f"error: unknown repo label(s): {', '.join(unknown)}", file=sys.stderr)
        return 1
    if not selected:
        print("error: --repos selected no repositories", file=sys.stderr)
        return 1
    # The three opt-in check lists are validated against the SELECTION, not the
    # corpus: a typo used to silently switch a check off everywhere while strict
    # mode still exited 0, which reads as "the check passed".
    chosen = set(selected)
    subsets = {}
    for flag, value in (
        ("--ai-repos", args.ai_repos),
        ("--repeat-repos", args.repeat_repos),
        ("--format-repos", args.format_repos),
    ):
        labels = set(parse_csv_list(value))
        stray = sorted(labels - chosen)
        if stray:
            print(
                f"error: {flag} names label(s) outside --repos: {', '.join(stray)}",
                file=sys.stderr,
            )
            return 1
        subsets[flag] = labels
    ai_repos = subsets["--ai-repos"]
    repeat_repos = subsets["--repeat-repos"]
    format_repos = subsets["--format-repos"]

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    # Fail on an unwritable summary path BEFORE the sweep, not after: the write
    # is the last statement in main, so a missing parent directory discarded
    # every result the run had just spent hours producing.
    summary_file = Path(args.summary_file)
    summary_file.parent.mkdir(parents=True, exist_ok=True)
    sweep = Sweep(archfit, output_dir)

    specs = [CORPUS[label] for label in CORPUS if label in set(selected)]
    records: list[dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=max(args.max_workers, 1)) as pool:
        futures = {
            pool.submit(
                sweep.process,
                spec,
                want_ai=spec.label in ai_repos,
                want_repeat=spec.label in repeat_repos,
                want_formats=spec.label in format_repos,
            ): spec
            for spec in specs
        }
        for future in as_completed(futures):
            spec = futures[future]
            try:
                record = future.result()
            except Exception as exc:  # noqa: BLE001 — a harness crash is a finding, not a stop
                record = blank_record(spec.label, str(spec.root), spec.language)
                record["status"] = STATUS_FAIL
                record["failures"].append(f"harness error: {exc!r}")
            records.append(finalize_status(record, allow_unverified))
            print_progress(
                {
                    "label": record["label"],
                    "status": record["status"],
                    "analyze_exit": record["analyze"]["exit"],
                    "check_exit": record["check"]["exit"],
                    "failures": record["failures"][:5],
                }
            )

    records.sort(key=lambda item: item["label"])
    summary_file.write_text(
        json.dumps(records, indent=2, sort_keys=True), encoding="utf-8"
    )
    print_progress({"summary_file": args.summary_file, "repos": len(records)})

    if args.strict:
        return strict_exit_code(records)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
