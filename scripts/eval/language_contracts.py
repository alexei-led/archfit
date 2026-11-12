#!/usr/bin/env python3
"""Run the bounded, real-tool language acceptance contract.

This gate creates five disposable repositories (Go, TypeScript, JavaScript,
Python, and a Rust workspace) and runs the branch binary's `archfit check` on
each.  Every fixture has a schema 3 policy with one custom deny rule.  Its
target branch holds a clean tree; its change branch adds the forbidden
dependency.  The check must give the deny error and exit 1, honour a policy
exception until its date, and pass (exit 0) once the change is undone.
A sixth case runs the .NET SDK and the Roslyn helper on a copy of
internal/testdata/dotnet: it asserts the helper's project statuses, file edges
and strength hints, then runs the same check with a new project reference as
the forbidden dependency.
Every required producer is checked before the run; a missing tool is an
environment failure, never an accepted skip.  Go, TypeScript, JavaScript and
Python need `git` and `ast-grep` only.  `--only LANGUAGE` runs one case and
checks only the producers that case uses.
"""

from __future__ import annotations

import argparse
import dataclasses
import json
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parents[1]
DEFAULT_ARCHFIT = REPO_ROOT / ".bin" / "archfit"
DEFAULT_OUTPUT = Path("/tmp/archfit-language-contracts")
OUTPUT_MARKER = ".archfit-language-contracts"
TOOL_MANIFEST = json.loads((REPO_ROOT / "tools/manifest.json").read_text())
RUST_TOOLCHAIN = TOOL_MANIFEST["versions"]["rust"]
DOTNET_VERSION = TOOL_MANIFEST["versions"]["dotnet"]
DOTNET_FIXTURE = REPO_ROOT / "internal" / "testdata" / "dotnet"
DOTNET_HELPER_DEFAULT = REPO_ROOT / ".bin" / "dotnet-roslyn"
DOTNET_HELPER_DLL = "archfit-roslyn.dll"
DOTNET_HELPER_ENV = "ARCHFIT_DOTNET_ROSLYN"
RESULT_SCHEMA_PATH = REPO_ROOT / "archfit.result.schema.json"
RESULT_SCHEMA = "archfit.result.v1"
RULE_ID = "core_no_adapter"
EXCEPTION_RULE = "exception"
POLICY_OWNER = "archfit-acceptance"
TARGET_BRANCH = "main"
CHANGE_BRANCH = "change"
ACTIVE_UNTIL = "2099-12-31"
EXPIRED_ON = "2000-01-01"
GITIGNORE = ".archfit-cache/\n"


@dataclass(frozen=True, slots=True)
class FixtureSpec:
    name: str
    language: str
    # The clean tree on the target branch.
    files: dict[str, str]
    # What the change writes over files: the forbidden dependency.
    forbidden: dict[str, str]
    core_paths: str
    adapter_paths: str
    python_package: str = ""


@dataclass(slots=True)
class AssertionResult:
    name: str
    passed: bool
    detail: Any = None


@dataclass(slots=True)
class ContractResult:
    language: str
    assertions: list[AssertionResult] = field(default_factory=list)
    failures: list[str] = field(default_factory=list)


@dataclass(frozen=True, slots=True)
class CommandResult:
    label: str
    argv: list[str]
    cwd: str
    returncode: int
    stdout: str
    stderr: str


@dataclass(frozen=True, slots=True)
class ChangeContract:
    """One repository, its policy, and the change that adds a forbidden dependency."""

    policy: str
    forbidden: dict[str, str]
    rule: str


FIXTURES = (
    FixtureSpec(
        "go",
        "go",
        {
            "go.mod": "module example.com/language-contract\n\ngo 1.25\n",
            "core/core.go": "package core\n\nfunc Run() int { return 1 }\n",
            "adapter/adapter.go": "package adapter\n\nfunc Value() int { return 1 }\n",
        },
        {
            "core/core.go": 'package core\n\nimport "example.com/language-contract/adapter"\n\nfunc Run() int { return adapter.Value() }\n',
        },
        "core/**",
        "adapter/**",
    ),
    FixtureSpec(
        "typescript",
        "typescript",
        {
            "package.json": '{\n  "name": "archfit-language-contract-typescript",\n  "private": true\n}\n',
            "tsconfig.json": json.dumps(
                {
                    "compilerOptions": {
                        "module": "ESNext",
                        "moduleResolution": "bundler",
                        "target": "ES2022",
                    },
                    "include": ["src/**/*.ts"],
                },
                indent=2,
            )
            + "\n",
            "src/core/index.ts": "export const run = () => 1;\n",
            "src/adapter/index.ts": "export const value = () => 1;\n",
        },
        {
            "src/core/index.ts": 'import { value } from "../adapter/index";\n\nexport const run = () => value();\n',
        },
        "src/core/**",
        "src/adapter/**",
    ),
    FixtureSpec(
        "javascript",
        "typescript",
        {
            "package.json": '{\n  "name": "archfit-language-contract-javascript",\n  "private": true,\n  "type": "module"\n}\n',
            "tsconfig.json": '{"compilerOptions":{"allowJs":true,"checkJs":true,"module":"NodeNext","moduleResolution":"NodeNext"},"include":["src/**/*.js"]}\n',
            "src/core/index.js": "export const run = () => 1;\n",
            "src/adapter/index.js": "export const value = () => 1;\n",
        },
        {
            "src/core/index.js": 'import { value } from "../adapter/index.js";\n\nexport const run = () => value();\n',
        },
        "src/core/**",
        "src/adapter/**",
    ),
    FixtureSpec(
        "python",
        "python",
        {
            "pyproject.toml": '[project]\nname = "archfit-language-contract-python"\nversion = "0.1.0"\nrequires-python = ">=3.12"\n',
            "smoke/__init__.py": "",
            "smoke/core.py": "def run():\n    return 1\n",
            "smoke/adapter.py": "def value():\n    return 1\n",
        },
        {
            "smoke/core.py": "from smoke.adapter import value\n\ndef run():\n    return value()\n",
        },
        "smoke/core.py",
        "smoke/adapter.py",
        python_package="smoke",
    ),
    FixtureSpec(
        "rust",
        "rust",
        {
            # cargo metadata may write the lock file during the check.
            ".gitignore": GITIGNORE + "Cargo.lock\n",
            "Cargo.toml": '[workspace]\nmembers = ["core", "adapter"]\nresolver = "2"\n',
            "core/Cargo.toml": '[package]\nname = "smoke-core"\nversion = "0.1.0"\nedition = "2021"\n',
            "core/src/lib.rs": "pub fn run() -> u32 { 1 }\n",
            "adapter/Cargo.toml": '[package]\nname = "smoke-adapter"\nversion = "0.1.0"\nedition = "2021"\n',
            "adapter/src/lib.rs": "pub fn value() -> u32 { 1 }\n",
        },
        {
            "core/Cargo.toml": '[package]\nname = "smoke-core"\nversion = "0.1.0"\nedition = "2021"\n\n[dependencies]\nsmoke-adapter = { path = "../adapter" }\n',
            "core/src/lib.rs": "pub fn run() -> u32 { smoke_adapter::value() }\n",
        },
        "core/**",
        "adapter/**",
    ),
)


DOTNET_PROJECTS = (
    "Application",
    "Broken",
    "Domain",
    "Infrastructure",
    "Legacy",
    "Tests",
    "Unresolved",
)
# The projects the check reads. Broken fails evaluation, Tests references it, and
# Unresolved names a missing package: the full set is the unknown case below.
DOTNET_CHECKED_PROJECTS = ("Domain", "Application", "Infrastructure", "Legacy")
DOTNET_RULE_ID = "legacy_no_application"
DOTNET_CHANGED_PROJECT = "Legacy/Legacy.csproj"
DOTNET_EXISTING_REFERENCE = (
    '    <ProjectReference Include="..\\Domain\\Domain.csproj" />\n'
)
DOTNET_FORBIDDEN_REFERENCE = (
    '    <ProjectReference Include="..\\Application\\Application.csproj" />\n'
)
DOTNET_MISSING_SDK = "Archfit.Fixture.Missing.Sdk"
DOTNET_NET8_PACK = "Microsoft.NETCore.App.Ref 8."
DOTNET_EXPECTED_EDGES = (
    # (from file, to file, strength hint, data hint)
    ("Infrastructure/Repository.cs", "Application/Contracts.cs", "dto", "dto"),
    ("Infrastructure/Repository.cs", "Domain/Domain.cs", "intrusive", "model"),
    ("Application/Contracts.cs", "Domain/Domain.cs", "model", "model"),
    ("Legacy/Legacy.cs", "Domain/Domain.cs", "functional", "model"),
)
# Broken fails evaluation too and Tests is a test project: neither has an edge.
DOTNET_EDGELESS_PROJECTS = ("Broken", "Tests")


def config_text(spec: FixtureSpec) -> str:
    """The schema 3 policy of a fixture: two modules and one deny rule."""
    languages = (
        f"languages:\n  python:\n    package: {spec.python_package}\n"
        if spec.python_package
        else ""
    )
    return f"""version: 3
owners: [{POLICY_OWNER}]
{languages}modules:
  core:
    paths: ['{spec.core_paths}']
  adapter:
    paths: ['{spec.adapter_paths}']
rules:
  - id: {RULE_ID}
    deny: {{ from: core, to: adapter }}
    why: The core must not know its adapter.
    instead: Pass the value in through the core's own API.
"""


def dotnet_policy_text(projects: tuple[str, ...] | None) -> str:
    """The .NET policy: one module per directory; projects limits what is read."""
    languages = ""
    if projects is not None:
        listed = ", ".join(f"{name}/{name}.csproj" for name in projects)
        languages = f"languages:\n  dotnet:\n    projects: [{listed}]\n"
    modules = "".join(
        f"  {name}:\n    paths: ['{name}/**']\n"
        for name in (*DOTNET_PROJECTS, "Shared")
    )
    return f"""version: 3
owners: [{POLICY_OWNER}]
{languages}modules:
{modules}rules:
  - id: {DOTNET_RULE_ID}
    deny: {{ from: Legacy, to: Application }}
    why: Legacy code stays off the application layer.
    instead: Keep Legacy on the domain model.
"""


def dotnet_forbidden(project: str) -> str:
    """Legacy.csproj with a new reference to Application: the forbidden dependency."""
    if DOTNET_EXISTING_REFERENCE not in project:
        raise RuntimeError(
            f"{DOTNET_CHANGED_PROJECT} no longer holds its Domain reference"
        )
    return project.replace(
        DOTNET_EXISTING_REFERENCE,
        DOTNET_EXISTING_REFERENCE + DOTNET_FORBIDDEN_REFERENCE,
    )


def exceptions_text(errors: list[dict[str, Any]], expires: str) -> str:
    """An exceptions block that covers every error of a result until expires."""
    covered = sorted(
        {
            (error.get("rule", ""), error.get("from", ""), error.get("to", ""))
            for error in errors
            if error.get("rule") != EXCEPTION_RULE
        }
    )
    lines = ["exceptions:"]
    for rule, source, target in covered:
        lines += [
            f"  - rule: {rule}",
            f"    from: {source}",
            f"    to: {target}",
            f'    expires: "{expires}"',
            "    why: acceptance exception lifecycle",
        ]
    return "\n".join(lines) + "\n"


def dotnet_environment(home: Path, helper: Path) -> dict[str, str]:
    env = command_environment()
    env.update(
        {
            DOTNET_HELPER_ENV: str(helper),
            "DOTNET_CLI_HOME": str(home / "dotnet-home"),
            "NUGET_PACKAGES": str(home / "nuget"),
            "DOTNET_NOLOGO": "1",
            "DOTNET_CLI_TELEMETRY_OPTOUT": "1",
        }
    )
    return env


def helper_entries(doc: dict[str, Any]) -> dict[tuple[str, str], dict[str, Any]]:
    return {(row["path"], row["tfm"]): row for row in doc.get("projects", [])}


def helper_edges(doc: dict[str, Any]) -> dict[tuple[str, str], tuple[str, str]]:
    edges: dict[tuple[str, str], tuple[str, str]] = {}
    for row in doc.get("projects", []):
        for file in row.get("files", []):
            for edge in file.get("edges", []):
                edges[(file["path"], edge["to"])] = (
                    edge.get("hint", ""),
                    edge.get("data_hint", ""),
                )
    return edges


def check_dotnet_helper(result: ContractResult, doc: dict[str, Any]) -> None:
    """Assert D5 statuses, reasons, hints, and edge-less projects from the helper."""
    entries = helper_entries(doc)
    for name in ("Domain", "Application", "Infrastructure"):
        row = entries.get((f"{name}/{name}.csproj", "net10.0"), {})
        assert_result(
            result,
            f"{name}@net10.0 is complete",
            row.get("status") == "complete",
            row,
        )
    legacy10 = entries.get(("Legacy/Legacy.csproj", "net10.0"), {})
    assert_result(
        result,
        "Legacy@net10.0 is complete",
        legacy10.get("status") == "complete",
        legacy10,
    )
    legacy8 = entries.get(("Legacy/Legacy.csproj", "net8.0"), {})
    # A host may carry the 8.0 targeting pack; the exact `partial` is the image contract's.
    packs = re.findall(r"reference pack (\S+ \S+)", legacy8.get("reason", ""))
    assert_result(
        result,
        "Legacy@net8.0 is complete or partial naming only the 8.x reference pack",
        legacy8.get("status") == "complete"
        or (
            legacy8.get("status") == "partial"
            and len(packs) == 1
            and packs[0].startswith(DOTNET_NET8_PACK)
        ),
        legacy8,
    )
    broken = [
        row for (path, _), row in entries.items() if path == "Broken/Broken.csproj"
    ]
    assert_result(
        result,
        "Broken failed naming its SDK with no files",
        len(broken) == 1
        and broken[0].get("status") == "failed"
        and DOTNET_MISSING_SDK in broken[0].get("reason", "")
        and not broken[0].get("files"),
        broken,
    )
    tests = entries.get(("Tests/Tests.csproj", "net10.0"), {})
    assert_result(
        result,
        "Tests is partial naming the missing SDK",
        tests.get("status") == "partial"
        and DOTNET_MISSING_SDK in tests.get("reason", ""),
        tests,
    )
    unresolved = entries.get(("Unresolved/Unresolved.csproj", "net10.0"), {})
    assert_result(
        result,
        "Unresolved is partial with counted unresolved symbols",
        unresolved.get("status") == "partial"
        and unresolved.get("unresolved_symbols", 0) > 0,
        unresolved,
    )
    missing_reason = [
        f"{path}@{tfm}"
        for (path, tfm), row in entries.items()
        if row.get("status") in {"partial", "failed"} and not row.get("reason")
    ]
    assert_result(
        result,
        "every partial or failed project carries a reason",
        not missing_reason,
        missing_reason,
    )
    edges = helper_edges(doc)
    for source, target, hint, data_hint in DOTNET_EXPECTED_EDGES:
        assert_result(
            result,
            f"{source} -> {target} is {hint}/{data_hint or 'none'}",
            edges.get((source, target)) == (hint, data_hint),
            edges.get((source, target)),
        )
    leaked = sorted(
        source
        for source, _ in edges
        if source.split("/", 1)[0] in DOTNET_EDGELESS_PROJECTS
    )
    assert_result(result, "Broken and Tests emit no helper edges", not leaked, leaked)


def check_dotnet_unknown(
    result: ContractResult, command: CommandResult, doc: dict[str, Any]
) -> None:
    """A project that fails evaluation is a reason in the record and never a pass.

    The status is fail when the change also adds an error, else unknown."""
    named = [
        reason
        for reason in doc.get("unknown", [])
        if reason.get("cause") == "tool"
        and "Broken/Broken.csproj" in reason.get("what", "")
        and DOTNET_MISSING_SDK in reason.get("what", "")
    ]
    assert_result(
        result,
        "the full project set never passes and names Broken's SDK",
        command.returncode in (1, 2)
        and doc.get("status") in ("fail", "unknown")
        and bool(named),
        {
            "exit": command.returncode,
            "status": doc.get("status"),
            "unknown": doc.get("unknown"),
        },
    )


def copy_dotnet_fixture(destination: Path) -> Path:
    shutil.copytree(
        DOTNET_FIXTURE, destination, ignore=shutil.ignore_patterns("bin", "obj")
    )
    return destination


def run_dotnet_contract(
    archfit: Path, output: Path, commands: list[CommandResult]
) -> ContractResult:
    result = ContractResult("dotnet")
    artifact_dir = output / "dotnet"
    artifact_dir.mkdir(parents=True)
    helper = artifact_dir / "helper"
    # The helper lives outside every analyzed tree, as in the image.
    shutil.copytree(dotnet_helper_source(), helper)
    env = dotnet_environment(artifact_dir, helper)

    helper_root = copy_dotnet_fixture(artifact_dir / "helper-repo")
    run = run_command(
        "dotnet-helper",
        ["dotnet", str(helper / DOTNET_HELPER_DLL), str(helper_root), "Fixture.slnx"],
        helper_root,
        env=env,
    )
    commands.append(run)
    try:
        doc = json.loads(run.stdout)
    except json.JSONDecodeError as exc:
        assert_result(result, "helper JSON parses", False, f"{exc}: {run.stderr}")
        doc = None
    if doc is not None:
        assert_result(result, "helper exits 0", run.returncode == 0, run.stderr)
        check_dotnet_helper(result, doc)

    root = copy_dotnet_fixture(artifact_dir / "repo")
    project = (root / DOTNET_CHANGED_PROJECT).read_text(encoding="utf-8")
    contract = ChangeContract(
        dotnet_policy_text(DOTNET_CHECKED_PROJECTS),
        {DOTNET_CHANGED_PROJECT: dotnet_forbidden(project)},
        DOTNET_RULE_ID,
    )
    prepare_change_repo(root, contract, commands, ignore=GITIGNORE + "bin/\nobj/\n")
    every_project = artifact_dir / "policy-every-project.yaml"
    every_project.write_text(dotnet_policy_text(None), encoding="utf-8")
    unknown = archfit_command(
        archfit,
        "dotnet-every-project",
        check_args(root, every_project),
        root,
        commands,
        env=env,
        timeout=900,
    )
    unknown_doc = parse_result(result, unknown)
    if unknown_doc is not None:
        check_dotnet_unknown(result, unknown, unknown_doc)
    run_change_stages(result, archfit, root, contract, commands, env=env, timeout=900)
    return result


def prepare_output_dir(path: Path) -> Path:
    """Create an owned output directory, refusing to delete user data."""
    if path.exists():
        if not (path / OUTPUT_MARKER).is_file():
            raise RuntimeError(
                f"refusing to wipe {path}: missing {OUTPUT_MARKER}; choose a new output directory"
            )
        shutil.rmtree(path)
    path.mkdir(parents=True)
    (path / OUTPUT_MARKER).write_text("", encoding="utf-8")
    return path


def command_environment() -> dict[str, str]:
    env = dict(os.environ)
    env.setdefault("RUSTUP_TOOLCHAIN", RUST_TOOLCHAIN)
    return env


def run_command(
    label: str,
    argv: list[str],
    cwd: Path,
    *,
    env: dict[str, str] | None = None,
    timeout: int = 300,
) -> CommandResult:
    try:
        proc = subprocess.run(
            argv,
            cwd=cwd,
            env=env or command_environment(),
            capture_output=True,
            timeout=timeout,
            check=False,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        return CommandResult(label, argv, str(cwd), 124, "", str(exc))
    return CommandResult(
        label, argv, str(cwd), proc.returncode, proc.stdout, proc.stderr
    )


def _version_number(output: str) -> str:
    match = re.search(r"\d+\.\d+\.\d+", output)
    return match.group(0) if match else ""


PRODUCER_TOOLS = {
    "go": ("ast-grep",),
    "typescript": ("ast-grep",),
    "javascript": ("ast-grep",),
    "python": ("ast-grep",),
    "rust": ("ast-grep", "cargo"),
    "dotnet": ("dotnet",),
}
ALL_CASES = (*PRODUCER_TOOLS,)


def required_tools(cases: tuple[str, ...]) -> set[str]:
    """The tools on PATH that the selected cases need: git, plus each producer."""
    tools = {"git"}
    for case in cases:
        tools.update(PRODUCER_TOOLS[case])
    return tools


def preflight(
    archfit: Path, root: Path, cases: tuple[str, ...] = ALL_CASES
) -> list[str]:
    """Require every producer used by the selected cases and exact tool pins."""
    failures: list[str] = []
    try:
        import jsonschema  # noqa: F401
    except ModuleNotFoundError:
        failures.append(
            "published schema validation requires Python package 'jsonschema'; "
            "run this gate with `uv run --with jsonschema python ...`"
        )
    if not archfit.is_file() or not os.access(archfit, os.X_OK):
        failures.append(f"archfit binary is missing or not executable: {archfit}")
    tools = required_tools(cases)
    for tool in sorted(tools):
        if shutil.which(tool) is None:
            failures.append(f"required tool {tool!r} is not on PATH")

    if "ast-grep" in tools and shutil.which("ast-grep"):
        sg = run_command("ast-grep-version", ["ast-grep", "--version"], root)
        if "ast-grep" not in (sg.stdout + sg.stderr).lower():
            failures.append("required ast-grep is not the canonical syntax producer")
    if "cargo" in tools and shutil.which("cargo"):
        cargo = run_command("cargo-version", ["cargo", "--version"], root)
        if _version_number(cargo.stdout) != RUST_TOOLCHAIN:
            failures.append(
                f"Cargo/Rust toolchain {RUST_TOOLCHAIN} is required, got {cargo.stdout.strip()!r}"
            )
    if "dotnet" in cases:
        failures.extend(dotnet_preflight(root))
    return failures


def dotnet_helper_source() -> Path:
    configured = os.environ.get(DOTNET_HELPER_ENV)
    return Path(configured) if configured else DOTNET_HELPER_DEFAULT


def dotnet_preflight(root: Path) -> list[str]:
    failures: list[str] = []
    if shutil.which("dotnet"):
        sdk = run_command("dotnet-version", ["dotnet", "--version"], root)
        if sdk.stdout.strip() != DOTNET_VERSION:
            failures.append(
                f".NET SDK {DOTNET_VERSION} is required, got {sdk.stdout.strip()!r}"
            )
    helper = dotnet_helper_source()
    if not (helper / DOTNET_HELPER_DLL).is_file():
        failures.append(
            f"Roslyn helper {DOTNET_HELPER_DLL} is missing under {helper}; "
            f"run `make dotnet-roslyn` or set {DOTNET_HELPER_ENV}"
        )
    if not DOTNET_FIXTURE.is_dir():
        failures.append(f".NET fixture is missing: {DOTNET_FIXTURE}")
    return failures


def assert_result(
    result: ContractResult, name: str, passed: bool, detail: Any = None
) -> None:
    assertion = AssertionResult(name, passed, detail)
    result.assertions.append(assertion)
    if not passed:
        result.failures.append(f"{name}: {detail}")


_RESULT_VALIDATOR: Any = None


def validate_result(doc: Any) -> list[str]:
    """Validate a check result against the published archfit.result.schema.json."""
    global _RESULT_VALIDATOR
    if _RESULT_VALIDATOR is None:
        import jsonschema

        schema = json.loads(RESULT_SCHEMA_PATH.read_text(encoding="utf-8"))
        _RESULT_VALIDATOR = jsonschema.Draft202012Validator(schema)
    errors = sorted(
        _RESULT_VALIDATOR.iter_errors(doc), key=lambda error: list(error.absolute_path)
    )
    return [
        "/" + "/".join(str(part) for part in error.absolute_path) + f": {error.message}"
        for error in errors
    ]


def parse_result(
    result: ContractResult, command: CommandResult
) -> dict[str, Any] | None:
    try:
        doc = json.loads(command.stdout)
    except json.JSONDecodeError as exc:
        assert_result(
            result,
            f"{command.label}: JSON output parses",
            False,
            {"error": str(exc), "exit": command.returncode, "stderr": command.stderr},
        )
        return None
    failures = validate_result(doc)
    assert_result(
        result, f"{command.label}: published result schema", not failures, failures
    )
    if not isinstance(doc, dict):
        return None
    return doc


def check_forbidden(
    result: ContractResult,
    command: CommandResult,
    doc: dict[str, Any],
    rule: str,
    changed: set[str] | frozenset[str],
) -> None:
    """The change adds the forbidden dependency: its error, exit 1, every producer ran."""
    assert_result(
        result,
        "the forbidden dependency fails the check with exit 1",
        command.returncode == 1 and doc.get("status") == "fail",
        {
            "exit": command.returncode,
            "status": doc.get("status"),
            "stderr": command.stderr,
        },
    )
    assert_result(
        result,
        "every producer ran: no unknown reason",
        doc.get("unknown") == [],
        doc.get("unknown"),
    )
    error = next(
        (error for error in doc.get("errors", []) if error.get("rule") == rule), None
    )
    assert_result(
        result,
        f"the record holds the {rule} error, from the change, at the changed file",
        error is not None
        and error.get("from_change") is True
        and error.get("path") in changed,
        doc.get("errors"),
    )


def check_exceptions(
    result: ContractResult,
    active: CommandResult,
    active_doc: dict[str, Any],
    expired: CommandResult,
    expired_doc: dict[str, Any],
    rule: str,
) -> None:
    """An exception holds the errors until its date; after it, the check fails."""
    assert_result(
        result,
        "an active exception passes the change",
        active.returncode == 0
        and active_doc.get("status") == "pass"
        and active_doc.get("errors") == [],
        {
            "exit": active.returncode,
            "errors": active_doc.get("errors"),
            "stderr": active.stderr,
        },
    )
    lapsed = [
        error
        for error in expired_doc.get("errors", [])
        if error.get("rule") == EXCEPTION_RULE
        and (error.get("exception") or {}).get("rule") == rule
        and (error.get("exception") or {}).get("expires") == EXPIRED_ON
    ]
    assert_result(
        result,
        "an expired exception fails the change and names its rule",
        expired.returncode == 1
        and expired_doc.get("status") == "fail"
        and bool(lapsed),
        {"exit": expired.returncode, "errors": expired_doc.get("errors")},
    )


def check_clean(
    result: ContractResult, command: CommandResult, doc: dict[str, Any]
) -> None:
    assert_result(
        result,
        "the clean fixture passes with exit 0",
        command.returncode == 0
        and doc.get("status") == "pass"
        and doc.get("errors") == []
        and doc.get("unknown") == [],
        {"exit": command.returncode, "doc": doc, "stderr": command.stderr},
    )


def write_files(root: Path, files: dict[str, str]) -> None:
    for relative, content in files.items():
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")


def git(root: Path, commands: list[CommandResult], label: str, *args: str) -> None:
    command = run_command(label, ["git", *args], root)
    commands.append(command)
    if command.returncode != 0:
        raise RuntimeError(f"{label} failed: {command.stderr.strip()}")


def prepare_change_repo(
    root: Path,
    contract: ChangeContract,
    commands: list[CommandResult],
    *,
    ignore: str = GITIGNORE,
) -> None:
    """Commit the tree in root and its policy on the target branch, then commit
    the forbidden change on the change branch, which stays checked out."""
    (root / ".archfit.yaml").write_text(contract.policy, encoding="utf-8")
    if not (root / ".gitignore").exists():
        (root / ".gitignore").write_text(ignore, encoding="utf-8")
    git(root, commands, "git-init", "init", "-q", "-b", TARGET_BRANCH)
    git(root, commands, "git-config-name", "config", "user.name", "Archfit Acceptance")
    git(
        root,
        commands,
        "git-config-email",
        "config",
        "user.email",
        "acceptance@example.invalid",
    )
    git(root, commands, "git-add-base", "add", "-A")
    git(root, commands, "git-commit-base", "commit", "-qm", "clean tree")
    git(root, commands, "git-branch", "checkout", "-qb", CHANGE_BRANCH)
    write_files(root, contract.forbidden)
    git(root, commands, "git-add-change", "add", "-A")
    git(root, commands, "git-commit-change", "commit", "-qm", "forbidden dependency")


def check_args(root: Path, policy: Path | None = None) -> list[str]:
    args = ["check", "-C", str(root), "--base", TARGET_BRANCH, "--format", "json"]
    if policy is not None:
        args += ["--policy", str(policy)]
    return args


def archfit_command(
    archfit: Path,
    label: str,
    args: list[str],
    root: Path,
    commands: list[CommandResult],
    *,
    env: dict[str, str] | None = None,
    timeout: int = 600,
) -> CommandResult:
    command = run_command(label, [str(archfit), *args], root, env=env, timeout=timeout)
    commands.append(command)
    (root.parent / f"{root.name}-{label}.stdout").write_text(
        command.stdout, encoding="utf-8"
    )
    (root.parent / f"{root.name}-{label}.stderr").write_text(
        command.stderr, encoding="utf-8"
    )
    return command


def run_change_stages(
    result: ContractResult,
    archfit: Path,
    root: Path,
    contract: ChangeContract,
    commands: list[CommandResult],
    *,
    env: dict[str, str] | None = None,
    timeout: int = 600,
) -> None:
    """Check the change, its repeat, its exceptions, and the clean tree."""
    assert_result(
        result,
        "fixture policy uses schema 3",
        re.search(r"(?m)^version:\s*3\s*$", contract.policy) is not None,
        contract.policy.splitlines()[:1],
    )

    def check(label: str, policy: Path | None = None) -> CommandResult:
        return archfit_command(
            archfit,
            label,
            check_args(root, policy),
            root,
            commands,
            env=env,
            timeout=timeout,
        )

    cold = check(f"{result.language}-forbidden")
    doc = parse_result(result, cold)
    if doc is None:
        return
    check_forbidden(result, cold, doc, contract.rule, set(contract.forbidden))
    repeat = check(f"{result.language}-repeat")
    assert_result(
        result,
        "a repeated check gives byte-identical JSON and the same exit",
        repeat.stdout == cold.stdout and repeat.returncode == cold.returncode,
        {"exit": repeat.returncode, "first exit": cold.returncode},
    )

    # A policy that differs from the target branch is unknown, so the exceptions
    # are judged through the --policy preview, from files outside the repository.
    documents = {}
    for label, expires in (("active", ACTIVE_UNTIL), ("expired", EXPIRED_ON)):
        candidate = root.parent / f"policy-{label}-exception.yaml"
        candidate.write_text(
            contract.policy + exceptions_text(doc.get("errors", []), expires),
            encoding="utf-8",
        )
        command = check(f"{result.language}-{label}-exception", candidate)
        documents[label] = (command, parse_result(result, command))
    (active, active_doc), (expired, expired_doc) = documents.values()
    if active_doc is not None and expired_doc is not None:
        check_exceptions(
            result, active, active_doc, expired, expired_doc, contract.rule
        )

    clean = run_command(
        "git-restore-clean",
        ["git", "checkout", TARGET_BRANCH, "--", *contract.forbidden],
        root,
    )
    commands.append(clean)
    if clean.returncode != 0:
        assert_result(result, "the clean tree is restored", False, clean.stderr)
        return
    repaired = check(f"{result.language}-clean")
    repaired_doc = parse_result(result, repaired)
    if repaired_doc is not None:
        check_clean(result, repaired, repaired_doc)


def run_fixture(
    spec: FixtureSpec, archfit: Path, output: Path, commands: list[CommandResult]
) -> ContractResult:
    result = ContractResult(spec.name)
    root = output / spec.name / "repo"
    root.mkdir(parents=True)
    write_files(root, spec.files)
    contract = ChangeContract(config_text(spec), spec.forbidden, RULE_ID)
    prepare_change_repo(root, contract, commands)
    run_change_stages(result, archfit, root, contract, commands)
    return result


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archfit", type=Path, default=DEFAULT_ARCHFIT)
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument(
        "--only",
        choices=ALL_CASES,
        help="run one case and check only the producers it uses",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        output = prepare_output_dir(args.output_dir.resolve())
    except RuntimeError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    cases = (args.only,) if args.only else ALL_CASES
    failures = preflight(args.archfit.resolve(), REPO_ROOT, cases)
    if failures:
        print("language acceptance preflight failed:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1
    commands: list[CommandResult] = []
    results: list[ContractResult] = []
    for spec in FIXTURES:
        if spec.name not in cases:
            continue
        try:
            results.append(run_fixture(spec, args.archfit.resolve(), output, commands))
        except Exception as exc:  # noqa: BLE001 - one fixture failure must not hide the rest
            result = ContractResult(spec.name)
            result.failures.append(f"fixture harness error: {exc!r}")
            results.append(result)
            print(f"{spec.name}: FAIL ({exc})", file=sys.stderr)
    if "dotnet" in cases:
        try:
            results.append(
                run_dotnet_contract(args.archfit.resolve(), output, commands)
            )
        except Exception as exc:  # noqa: BLE001 - one case failure must not hide the rest
            result = ContractResult("dotnet")
            result.failures.append(f"fixture harness error: {exc!r}")
            results.append(result)
            print(f"dotnet: FAIL ({exc})", file=sys.stderr)
    (output / "commands.json").write_text(
        json.dumps(
            [dataclasses.asdict(command) for command in commands],
            indent=2,
            sort_keys=True,
        ),
        encoding="utf-8",
    )
    summary = {
        "archfit": str(args.archfit.resolve()),
        "toolchain": {"rust": RUST_TOOLCHAIN, "dotnet": DOTNET_VERSION},
        "results": [
            {
                "language": result.language,
                "passed": not result.failures,
                "assertions": [
                    dataclasses.asdict(assertion) for assertion in result.assertions
                ],
                "failures": result.failures,
            }
            for result in results
        ],
    }
    (output / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8"
    )
    for result in results:
        print(
            f"{result.language}: {'PASS' if not result.failures else 'FAIL'} ({len(result.assertions)} assertions)"
        )
        for failure in result.failures:
            print(f"  - {failure}")
    return 0 if results and all(not result.failures for result in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
