#!/usr/bin/env python3
"""Run the bounded, real-tool language acceptance contract.

This gate creates five disposable repositories (Go, TypeScript, JavaScript,
Python, and a Rust workspace) and exercises the branch binary through the
public CLI.  The fixtures deliberately contain one forbidden dependency so
the gate proves both a real producer edge and the finding lifecycle around it.
A sixth case runs the .NET SDK, the Roslyn helper, and the branch binary on a
copy of internal/testdata/dotnet and asserts project statuses, file edges, and
strength hints.
Every required producer is checked before the run; a missing tool is an
environment failure, never an accepted skip.  `--only LANGUAGE` runs one case
and checks only the producers that case uses.
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
DEPCRUISE_VERSION = TOOL_MANIFEST["node"]["dependency-cruiser"]
DOTNET_VERSION = TOOL_MANIFEST["versions"]["dotnet"]
DOTNET_FIXTURE = REPO_ROOT / "internal" / "testdata" / "dotnet"
DOTNET_HELPER_DEFAULT = REPO_ROOT / ".bin" / "dotnet-roslyn"
DOTNET_HELPER_DLL = "archfit-roslyn.dll"
DOTNET_HELPER_ENV = "ARCHFIT_DOTNET_ROSLYN"
TYPESCRIPT_VERSION = TOOL_MANIFEST["node"]["typescript"]
RULE_ID = "core_no_adapter"
WARN_RULE_ID = "core_warn_adapter"
OPTIONAL_DISABLED = {
    "scip": "disabled",
    "jscpd": "disabled",
    "cargo-modules": "disabled",
}

sys.path.insert(0, str(SCRIPT_DIR))
import corpus_sweep as corpus


@dataclass(frozen=True, slots=True)
class FixtureSpec:
    name: str
    language: str
    files: dict[str, str]
    from_selector: str
    to_selector: str
    python_package: str = ""
    node: bool = False


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


FIXTURES = (
    FixtureSpec(
        "go",
        "go",
        {
            "go.mod": "module example.com/language-contract\n\ngo 1.25\n",
            "core/core.go": 'package core\n\nimport "example.com/language-contract/adapter"\n\nfunc Run() int { return adapter.Value() }\n',
            "adapter/adapter.go": "package adapter\n\nfunc Value() int { return 1 }\n",
        },
        "core/**",
        "adapter/**",
    ),
    FixtureSpec(
        "typescript",
        "typescript",
        {
            "package.json": json.dumps(
                {
                    "name": "archfit-language-contract-typescript",
                    "private": True,
                    "devDependencies": {
                        "dependency-cruiser": DEPCRUISE_VERSION,
                        "typescript": TYPESCRIPT_VERSION,
                    },
                },
                indent=2,
            )
            + "\n",
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
            "src/core/index.ts": 'import { value } from "../adapter/index";\n\nexport const run = () => value();\n',
            "src/adapter/index.ts": "export const value = () => 1;\n",
        },
        "src/core/**",
        "src/adapter/**",
        node=True,
    ),
    FixtureSpec(
        "javascript",
        "typescript",
        {
            "package.json": json.dumps(
                {
                    "name": "archfit-language-contract-javascript",
                    "private": True,
                    "type": "module",
                    "devDependencies": {"dependency-cruiser": DEPCRUISE_VERSION},
                },
                indent=2,
            )
            + "\n",
            "tsconfig.json": '{"compilerOptions":{"allowJs":true,"checkJs":true,"module":"NodeNext","moduleResolution":"NodeNext"},"include":["src/**/*.js"]}\n',
            "src/core/index.js": 'import { value } from "../adapter/index.js";\n\nexport const run = () => value();\n',
            "src/adapter/index.js": "export const value = () => 1;\n",
        },
        "src/core/**",
        "src/adapter/**",
        node=True,
    ),
    FixtureSpec(
        "python",
        "python",
        {
            "pyproject.toml": '[project]\nname = "archfit-language-contract-python"\nversion = "0.1.0"\nrequires-python = ">=3.12"\n',
            "smoke/__init__.py": "",
            "smoke/core.py": "from smoke.adapter import value\n\ndef run():\n    return value()\n",
            "smoke/adapter.py": "def value():\n    return 1\n",
        },
        "smoke.core**",
        "smoke.adapter**",
        python_package="smoke",
    ),
    FixtureSpec(
        "rust",
        "rust",
        {
            "Cargo.toml": '[workspace]\nmembers = ["core", "adapter"]\nresolver = "2"\n',
            "core/Cargo.toml": '[package]\nname = "smoke-core"\nversion = "0.1.0"\nedition = "2021"\n\n[dependencies]\nsmoke-adapter = { path = "../adapter" }\n',
            "core/src/lib.rs": "pub fn run() -> u32 { smoke_adapter::value() }\n",
            "adapter/Cargo.toml": '[package]\nname = "smoke-adapter"\nversion = "0.1.0"\nedition = "2021"\n',
            "adapter/src/lib.rs": "pub fn value() -> u32 { 1 }\n",
        },
        "smoke-core",
        "smoke-adapter",
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
DOTNET_MISSING_SDK = "Archfit.Fixture.Missing.Sdk"
DOTNET_NET8_PACK = "Microsoft.NETCore.App.Ref 8."
DOTNET_EXPECTED_EDGES = (
    # (from file, to file, strength hint, data hint)
    ("Infrastructure/Repository.cs", "Application/Contracts.cs", "dto", "dto"),
    ("Infrastructure/Repository.cs", "Domain/Domain.cs", "intrusive", "model"),
    ("Application/Contracts.cs", "Domain/Domain.cs", "model", "model"),
    ("Legacy/Legacy.cs", "Domain/Domain.cs", "functional", "model"),
)
DOTNET_EXPECTED_SEAMS = {
    ("Infrastructure", "Domain"): "intrusive",
    ("Application", "Domain"): "model",
    ("Legacy", "Domain"): "functional",
}
# Broken fails evaluation too and Tests is a test project: neither has an edge.
DOTNET_EDGELESS_PROJECTS = ("Broken", "Tests")


def dotnet_config_text() -> str:
    modules = "".join(
        f"  {name}:\n    paths: ['{name}/**']\n    owner: team-{name.lower()}\n"
        for name in DOTNET_PROJECTS
    )
    return f"""version: 2
languages:
  go:
    enabled: false
  typescript:
    enabled: false
  python:
    enabled: false
  rust:
    enabled: false
  dotnet:
    enabled: true
analyzers:
  syntax:
    enabled: false
  scip:
    enabled: false
  clones:
    enabled: false
  cargo_modules:
    enabled: false
  roslyn:
    enabled: true
modules:
{modules}"""


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


def check_dotnet_state(result: ContractResult, state: dict[str, Any]) -> None:
    """Assert what the branch binary reports for the fixture through the public state."""
    rows = {row.get("tool"): row for row in state.get("coverage", {}).get("tools", [])}
    msbuild = rows.get("dotnet-msbuild", {})
    assert_result(
        result,
        "dotnet-msbuild row is partial and names Broken's SDK",
        msbuild.get("status") == "partial"
        and "Broken/Broken.csproj" in msbuild.get("reason", "")
        and DOTNET_MISSING_SDK in msbuild.get("reason", ""),
        msbuild,
    )
    roslyn = rows.get("roslyn", {})
    reason = roslyn.get("reason", "")
    named_pack = " Legacy/Legacy.csproj partial" not in f" {reason}" or (
        "Microsoft.NETCore.App.Ref 8." in reason
    )
    assert_result(
        result,
        "roslyn row is partial with unresolved inputs and names the failed project",
        roslyn.get("status") == "partial"
        and re.search(r"\d+ unresolved symbols?", reason) is not None
        and "Broken/Broken.csproj failed" in reason
        and re.search(
            rf"\d+ of {len(DOTNET_PROJECTS)} projects partial, 1 failed", reason
        )
        and named_pack,
        roslyn,
    )
    versions = state.get("measurement", {}).get("tool_versions", {})
    assert_result(
        result,
        "producer versions are recorded",
        versions.get("dotnet-msbuild") == DOTNET_VERSION
        and versions.get("roslyn", "").startswith("archfit-roslyn "),
        versions,
    )
    seams = {
        f"{seam.get('from_module')}->{seam.get('to_module')}": seam.get("strength")
        for seam in state.get("seams", [])
    }
    assert_result(
        result,
        "seams carry the compiler-derived strengths",
        all(
            seams.get(f"{source}->{target}") == want
            for (source, target), want in DOTNET_EXPECTED_SEAMS.items()
        ),
        seams,
    )
    leaked = sorted(
        pair
        for pair in seams
        if any(name in pair.split("->") for name in DOTNET_EDGELESS_PROJECTS)
    )
    assert_result(
        result,
        "Broken and Tests have no seams, so no fallback edges",
        not leaked,
        leaked,
    )


def copy_dotnet_fixture(destination: Path) -> Path:
    shutil.copytree(DOTNET_FIXTURE, destination)
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
    config = root / ".archfit.yaml"
    config.write_text(dotnet_config_text(), encoding="utf-8")
    validate_config_v2(result, config)
    git_fixture(root, commands)
    cli = run_command(
        "dotnet-analyze",
        [
            str(archfit),
            "analyze",
            "--root",
            str(root),
            "--config",
            str(config),
            "--json",
            "--quiet",
        ],
        root,
        env=env,
        timeout=900,
    )
    commands.append(cli)
    (artifact_dir / "analyze.stderr").write_text(cli.stderr, encoding="utf-8")
    assert_result(result, "analyze exits 0", cli.returncode == 0, cli.stderr)
    state = parse_state(result, cli)
    if state is not None:
        check_dotnet_state(result, state)
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
    "go": ("go",),
    "typescript": ("node", "depcruise"),
    "javascript": ("node", "depcruise"),
    "python": (),
    "rust": ("cargo",),
    "dotnet": ("dotnet",),
}
ALL_CASES = (*PRODUCER_TOOLS,)


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
    tools = {"git"}
    for case in cases:
        tools.update(PRODUCER_TOOLS[case])
    if set(cases) - {"dotnet"}:
        tools.add("ast-grep")
    for tool in sorted(tools):
        if shutil.which(tool) is None:
            failures.append(f"required tool {tool!r} is not on PATH")

    if "node" in tools and shutil.which("node"):
        node = run_command("node-version", ["node", "--version"], root)
        if _version_number(node.stdout) != TOOL_MANIFEST["versions"]["node"]:
            failures.append(
                f"Node {TOOL_MANIFEST['versions']['node']} is required, got {node.stdout.strip()!r}"
            )
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
    if "python" in cases:
        python = os.environ.get("ARCHFIT_TOOL_PYTHON", sys.executable)
        grimp = run_command(
            "grimp-preflight",
            [
                python,
                "-I",
                "-c",
                "import grimp, importlib.metadata, platform; print(importlib.metadata.version('grimp')); print(platform.python_version())",
            ],
            root,
        )
        if grimp.returncode != 0 or grimp.stdout.splitlines() != [
            TOOL_MANIFEST["python"]["grimp"],
            TOOL_MANIFEST["versions"]["python"],
        ]:
            failures.append(
                f"isolated pinned Python/grimp is unavailable: {grimp.stdout} {grimp.stderr}"
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


def config_text(spec: FixtureSpec) -> str:
    enabled = {name: "false" for name in ("go", "typescript", "python", "rust")}
    enabled[spec.language] = "true"
    package_line = (
        f"    package: {spec.python_package}\n" if spec.python_package else ""
    )
    return f"""version: 2
languages:
  go:
    enabled: {enabled["go"]}
  typescript:
    enabled: {enabled["typescript"]}
  python:
    enabled: {enabled["python"]}
{package_line}  rust:
    enabled: {enabled["rust"]}
analyzers:
  syntax:
    enabled: true
  scip:
    enabled: false
  clones:
    enabled: false
  cargo_modules:
    enabled: false
layers: [core, adapter]
modules:
  core:
    paths: ['{spec.from_selector}']
    layer: core
    owner: team-core
    deploy_unit: smoke
  adapter:
    paths: ['{spec.to_selector}']
    layer: adapter
    owner: team-adapter
    deploy_unit: smoke
rules:
  - id: {RULE_ID}
    type: forbidden_dependency
    from: '{spec.from_selector}'
    to: '{spec.to_selector}'
    gate: fail
  - id: {WARN_RULE_ID}
    type: forbidden_dependency
    from: '{spec.from_selector}'
    to: '{spec.to_selector}'
    gate: warn
"""


def assert_result(
    result: ContractResult, name: str, passed: bool, detail: Any = None
) -> None:
    assertion = AssertionResult(name, passed, detail)
    result.assertions.append(assertion)
    if not passed:
        result.failures.append(f"{name}: {detail}")


def baseline_capture_succeeded(result: ContractResult, command: CommandResult) -> bool:
    """Record baseline capture failure before inspecting its output artifact."""
    succeeded = command.returncode == 0
    assert_result(
        result,
        "waived baseline capture succeeds",
        succeeded,
        {"exit": command.returncode, "stderr": command.stderr},
    )
    return succeeded


def parse_state(
    result: ContractResult, command: CommandResult
) -> dict[str, Any] | None:
    try:
        state = json.loads(command.stdout)
    except json.JSONDecodeError as exc:
        assert_result(result, "JSON output parses", False, str(exc))
        return None
    failures = corpus.validate_state(state, schema_path=corpus.STATE_SCHEMA_PATH)
    assert_result(
        result, "published state schema and structural contract", not failures, failures
    )
    if not isinstance(state, dict):
        return None
    return state


def finding_for(state: dict[str, Any], rule_id: str) -> dict[str, Any] | None:
    return next(
        (
            finding
            for finding in state.get("findings", [])
            if finding.get("rule_id") == rule_id
        ),
        None,
    )


def check_exit(
    result: ContractResult, command: CommandResult, state: dict[str, Any]
) -> None:
    want = corpus.expected_check_exit(state.get("verdict"))
    assert_result(
        result,
        "check exit matches published verdict",
        want is not None and command.returncode == want,
        {"exit": command.returncode, "verdict": state.get("verdict"), "expected": want},
    )


def optional_tools_disclosed(result: ContractResult, state: dict[str, Any]) -> None:
    rows = {row.get("tool"): row for row in state.get("coverage", {}).get("tools", [])}
    syntax = rows.get("ast-grep/syntax")
    assert_result(
        result,
        "syntax producer runs with real ast-grep",
        syntax is not None and syntax.get("status") == "ok",
        syntax,
    )
    for tool, status in OPTIONAL_DISABLED.items():
        row = rows.get(tool)
        if tool == "cargo-modules":
            assert_result(
                result,
                "optional analyzer cargo-modules is disclosed",
                row is not None and row.get("status") in {"absent", "disabled"},
                row,
            )
            continue
        assert_result(
            result,
            f"optional analyzer {tool} is explicitly disabled",
            row is not None and row.get("status") == status and bool(row.get("reason")),
            row,
        )


def git_fixture(root: Path, commands: list[CommandResult]) -> None:
    for label, argv in (
        ("git-init", ["git", "init", "-q"]),
        ("git-config-name", ["git", "config", "user.name", "Archfit Acceptance"]),
        (
            "git-config-email",
            ["git", "config", "user.email", "acceptance@example.invalid"],
        ),
        ("git-add", ["git", "add", "."]),
        ("git-commit", ["git", "commit", "-qm", "language acceptance fixture"]),
    ):
        command = run_command(label, argv, root)
        commands.append(command)
        if command.returncode != 0:
            raise RuntimeError(f"{label} failed: {command.stderr.strip()}")


def install_node_fixture(root: Path, commands: list[CommandResult]) -> None:
    # Never install analyzers in the target or select a target-local npm tool.
    version = run_command("depcruise-version", ["depcruise", "--version"], root)
    commands.append(version)
    if DEPCRUISE_VERSION not in version.stdout:
        raise RuntimeError(
            f"fixture dependency-cruiser is not {DEPCRUISE_VERSION}: {version.stdout.strip()!r}"
        )


def archfit_command(
    archfit: Path,
    label: str,
    args: list[str],
    root: Path,
    commands: list[CommandResult],
    *,
    timeout: int = 600,
) -> CommandResult:
    command = run_command(label, [str(archfit), *args], root, timeout=timeout)
    commands.append(command)
    (root.parent / f"{root.name}-{label}.stdout").write_text(
        command.stdout, encoding="utf-8"
    )
    (root.parent / f"{root.name}-{label}.stderr").write_text(
        command.stderr, encoding="utf-8"
    )
    return command


def target_task(state: dict[str, Any]) -> dict[str, Any] | None:
    target = finding_for(state, RULE_ID)
    if target is None:
        return None
    return next(
        (
            task
            for task in state.get("agent_tasks", [])
            if task.get("finding_id") == target.get("id")
        ),
        None,
    )


def repair_files(spec: FixtureSpec, root: Path) -> None:
    if spec.name == "go":
        (root / "core/core.go").write_text(
            "package core\n\nfunc Run() int { return 1 }\n", encoding="utf-8"
        )
    elif spec.name == "typescript":
        (root / "src/core/index.ts").write_text(
            "export const run = () => 1;\n", encoding="utf-8"
        )
    elif spec.name == "javascript":
        (root / "src/core/index.js").write_text(
            "export const run = () => 1;\n", encoding="utf-8"
        )
    elif spec.name == "python":
        (root / "smoke/core.py").write_text(
            "def run():\n    return 1\n", encoding="utf-8"
        )
    elif spec.name == "rust":
        (root / "core/Cargo.toml").write_text(
            '[package]\nname = "smoke-core"\nversion = "0.1.0"\nedition = "2021"\n',
            encoding="utf-8",
        )
        (root / "core/src/lib.rs").write_text(
            "pub fn run() -> u32 { 1 }\n", encoding="utf-8"
        )


def validate_config_v2(result: ContractResult, config: Path) -> None:
    raw = config.read_text(encoding="utf-8")
    assert_result(
        result,
        "fixture config uses schema v2",
        re.search(r"(?m)^version:\s*2\s*$", raw) is not None,
        raw.splitlines()[:2],
    )


def run_fixture(
    spec: FixtureSpec, archfit: Path, output: Path, commands: list[CommandResult]
) -> ContractResult:
    result = ContractResult(spec.name)
    artifact_dir = output / spec.name
    root = artifact_dir / "repo"
    root.mkdir(parents=True)
    for relative, content in spec.files.items():
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
    config = root / ".archfit.yaml"
    config.write_text(config_text(spec), encoding="utf-8")
    validate_config_v2(result, config)
    (root / ".gitignore").write_text(
        "node_modules/\n.archfit-cache/\n.archfit-baseline.json\n", encoding="utf-8"
    )
    if spec.node:
        install_node_fixture(root, commands)
    git_fixture(root, commands)

    common = ["--root", str(root), "--config", str(config), "--json", "--quiet"]
    cold = archfit_command(
        archfit, f"{spec.name}-cold", ["check", *common], root, commands
    )
    state = parse_state(result, cold)
    if state is None:
        return result
    check_exit(result, cold, state)
    target = finding_for(state, RULE_ID)
    assert_result(
        result,
        "real forbidden dependency blocks",
        cold.returncode == 1 and target is not None and target.get("status") == "new",
        target,
    )
    unevaluated = {
        entry.get("rule_id")
        for entry in state.get("decision", {}).get("unevaluated_required_rules", [])
    }
    primary = {
        row.get("tool"): row for row in state.get("coverage", {}).get("tools", [])
    }
    primary_tool = {
        "go": "go/packages",
        "typescript": "dependency-cruiser",
        "python": "grimp",
        "rust": "cargo",
    }[spec.language]
    assert_result(
        result,
        "required rule scope is evaluated with producer success",
        RULE_ID not in unevaluated
        and primary.get(primary_tool, {}).get("status") == "ok",
        {"unevaluated": sorted(unevaluated), "producer": primary.get(primary_tool)},
    )
    optional_tools_disclosed(result, state)
    producer_pin = {
        "go": TOOL_MANIFEST["versions"]["go"],
        "typescript": DEPCRUISE_VERSION,
        "python": f"grimp {TOOL_MANIFEST['python']['grimp']}; python {TOOL_MANIFEST['versions']['python']}",
        "rust": RUST_TOOLCHAIN,
    }[spec.language]
    producers = (
        state.get("comparison", {}).get("measurement_profile", {}).get("producers", [])
    )
    producer = next((row for row in producers if row.get("tool") == primary_tool), {})
    assert_result(
        result,
        "consumed primary producer matches manifest",
        producer_pin in producer.get("tool_version", ""),
        producer,
    )
    if spec.name == "python":
        locations = (target or {}).get("locations") or []
        physical = bool(locations) and all(
            isinstance(location.get("file"), str)
            and location["file"].endswith(".py")
            and (root / location["file"]).is_file()
            for location in locations
        )
        assert_result(
            result,
            "Python finding locations are physical .py files",
            physical,
            locations,
        )
    task = target_task(state)
    assert_result(
        result,
        "forbidden finding has a replayable repair task",
        task is not None and bool(task.get("validation")),
        task,
    )
    if task is None or not task.get("validation"):
        return result
    replay_bin = output / "bin" / "archfit"
    replay_bin.parent.mkdir(exist_ok=True)
    if not replay_bin.exists():
        replay_bin.symlink_to(archfit)
    before_repair = run_command(
        f"{spec.name}-replay-before-repair",
        ["/bin/sh", "-c", task["validation"][0]],
        root,
        env={
            **command_environment(),
            "PATH": f"{replay_bin.parent}:{os.environ.get('PATH', '')}",
        },
    )
    commands.append(before_repair)
    assert_result(
        result,
        "emitted replay blocks before repair",
        before_repair.returncode == 1,
        before_repair.stderr or before_repair.stdout,
    )

    warm = archfit_command(
        archfit, f"{spec.name}-warm", ["check", *common], root, commands
    )
    refresh = archfit_command(
        archfit, f"{spec.name}-refresh", ["check", *common, "--refresh"], root, commands
    )
    assert_result(
        result,
        "cold/warm/refresh JSON is byte-identical",
        cold.stdout == warm.stdout == refresh.stdout,
        {"warm": warm.returncode, "refresh": refresh.returncode},
    )

    baseline = archfit_command(
        archfit,
        f"{spec.name}-baseline",
        ["baseline", "--root", str(root), "--config", str(config)],
        root,
        commands,
    )
    assert_result(
        result,
        "baseline capture succeeds",
        baseline.returncode == 0,
        baseline.stderr or baseline.stdout,
    )
    baseline_check = archfit_command(
        archfit, f"{spec.name}-baseline-check", ["check", *common], root, commands
    )
    baseline_state = parse_state(result, baseline_check)
    if baseline_state is not None:
        check_exit(result, baseline_check, baseline_state)
        baseline_target = finding_for(baseline_state, RULE_ID)
        assert_result(
            result,
            "same-identity baseline is comparable and live ID is baseline",
            baseline_state.get("gate_reference", {}).get("status") == "comparable"
            and baseline_target is not None
            and baseline_target.get("status") == "baseline",
            {
                "reference": baseline_state.get("gate_reference"),
                "finding": baseline_target,
            },
        )
        warning = finding_for(baseline_state, WARN_RULE_ID)
        assert_result(
            result,
            "warn finding is captured exactly once as baseline",
            warning is not None and warning.get("status") == "baseline",
            warning,
        )
        assert_result(
            result,
            "baseline report has unique live finding IDs",
            not corpus.validate_identity_lifecycle(baseline_state),
            corpus.validate_identity_lifecycle(baseline_state),
        )
    saved_baseline = root / ".archfit-baseline.json"
    assert_result(
        result,
        "baseline file is written beside the source config",
        saved_baseline.is_file(),
        str(saved_baseline),
    )
    if saved_baseline.is_file():
        shutil.copy2(saved_baseline, artifact_dir / "normal-baseline.json")
        saved_baseline.unlink()

    original_config = config.read_text(encoding="utf-8")
    waiver = (
        original_config
        + f"""waivers:
  - rule: core_no_adapter
    from: '{spec.from_selector}'
    to: '{spec.to_selector}'
    reason: acceptance lifecycle smoke
    approved_by: acceptance
    expires: "2099-12-31"
"""
    )
    config.write_text(waiver, encoding="utf-8")
    waived = archfit_command(
        archfit, f"{spec.name}-waived", ["check", *common], root, commands
    )
    waived_state = parse_state(result, waived)
    if waived_state is not None:
        waived_target = finding_for(waived_state, RULE_ID)
        assert_result(
            result,
            "temporary waiver suppresses the hard gate",
            waived.returncode != 1
            and waived_target is not None
            and waived_target.get("status") == "waived",
            waived_target,
        )
    waived_baseline = archfit_command(
        archfit,
        f"{spec.name}-waived-baseline",
        ["baseline", "--root", str(root), "--config", str(config)],
        root,
        commands,
    )
    if baseline_capture_succeeded(result, waived_baseline) and saved_baseline.is_file():
        body = saved_baseline.read_text(encoding="utf-8")
        assert_result(
            result,
            "waived finding is excluded from permanent baseline",
            RULE_ID not in body,
            body,
        )
        shutil.copy2(saved_baseline, artifact_dir / "waived-baseline.json")
        saved_baseline.unlink()
    elif waived_baseline.returncode == 0:
        assert_result(
            result,
            "waived baseline file is written beside the source config",
            False,
            str(saved_baseline),
        )
    config.write_text(waiver.replace("2099-12-31", "2000-01-01"), encoding="utf-8")
    expired = archfit_command(
        archfit, f"{spec.name}-expired", ["check", *common], root, commands
    )
    expired_state = parse_state(result, expired)
    if expired_state is not None:
        expired_target = finding_for(expired_state, RULE_ID)
        assert_result(
            result,
            "expired waiver blocks",
            expired.returncode == 1
            and expired_target is not None
            and expired_target.get("status") == "expired_waiver",
            expired_target,
        )
    config.write_text(original_config, encoding="utf-8")

    repair_files(spec, root)
    repaired = run_command(
        f"{spec.name}-replay-after-repair",
        ["/bin/sh", "-c", task["validation"][0]],
        root,
        env={
            **command_environment(),
            "PATH": f"{replay_bin.parent}:{os.environ.get('PATH', '')}",
        },
    )
    commands.append(repaired)
    assert_result(
        result,
        "emitted replay succeeds after source repair",
        repaired.returncode in (0, 2),
        repaired.stderr or repaired.stdout,
    )
    repaired_check = archfit_command(
        archfit, f"{spec.name}-repaired", ["check", *common], root, commands
    )
    repaired_state = parse_state(result, repaired_check)
    if repaired_state is not None:
        assert_result(
            result,
            "source repair clears forbidden gate",
            finding_for(repaired_state, RULE_ID) is None
            and repaired_state.get("decision", {}).get("active_blockers") == 0,
            repaired_state.get("findings"),
        )
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
        "toolchain": {
            "rust": RUST_TOOLCHAIN,
            "dotnet": DOTNET_VERSION,
            "dependency_cruiser": DEPCRUISE_VERSION,
            "optional_analyzers": OPTIONAL_DISABLED,
        },
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
