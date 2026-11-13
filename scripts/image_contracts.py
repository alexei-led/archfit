#!/usr/bin/env python3
"""Bounded final-image contracts; never builds, publishes, or installs packages.

Run on native amd64 and arm64. Every failed/missing producer is a failure, not a
skip. Trusted fixture inputs are readonly; only disposable /tmp copies change.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shlex
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from scripts.eval import language_contracts as lc


class ImageContract:
    def __init__(self, image: str, context: str, output: Path):
        self.image = image
        self.docker = ["docker", "--context", context]
        self.output = output
        self.assertions: list[dict[str, Any]] = []

    def expect(self, name: str, passed: bool, detail: Any = None) -> None:
        self.assertions.append({"name": name, "passed": bool(passed), "detail": detail})
        print(f"{'PASS' if passed else 'FAIL'}: {name}", flush=True)

    def run(
        self,
        name: str,
        uid: str,
        script: str,
        fixture: Path | None = None,
        *,
        network: str = "none",
        timeout: int = 600,
        copy: bool = True,
    ) -> Path:
        output = self.output / name
        output.mkdir()
        output.chmod(0o777)
        args = [
            *self.docker,
            "run",
            "--rm",
            "--user",
            uid,
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges",
            "--read-only",
            "--network",
            network,
            "--tmpfs",
            "/tmp:rw,exec,nosuid,size=2g,mode=1777",
            "--env",
            "HOME=/tmp",
            "--mount",
            f"type=bind,source={output},target=/out",
            "--entrypoint",
            "/bin/sh",
        ]
        if fixture:
            args += ["--mount", f"type=bind,source={fixture},target=/trusted,readonly"]
            if copy:
                script = "cp -R /trusted /tmp/repo; cd /tmp/repo; " + script
            else:
                script = "cd /tmp; " + script
        args += [self.image, "-ec", script]
        (output / "command.json").write_text(json.dumps(args, indent=2))
        try:
            result = subprocess.run(
                args, capture_output=True, text=True, timeout=timeout, check=False
            )
            (output / "stdout.log").write_text(result.stdout)
            (output / "stderr.log").write_text(result.stderr)
            self.expect(
                f"{name}: container execution",
                result.returncode == 0,
                result.returncode,
            )
        except subprocess.TimeoutExpired as exc:
            self.expect(f"{name}: container execution", False, str(exc))
        return output

    def read_state(self, output: Path, label: str) -> dict[str, Any]:
        try:
            state = json.loads((output / f"{label}.json").read_text())
            if state.get("schema_version") != "archfit.architecture-state.v1":
                raise ValueError("not architecture-state.v1")
            return state
        except (OSError, ValueError) as exc:
            self.expect(f"{output.name}/{label}: valid state", False, str(exc))
            return {}

    def read_result(self, output: Path, label: str) -> dict[str, Any]:
        """A version 3 check record. The host runs plain python3, so this checks
        the document identifier and the lists; the language contracts validate
        the full published schema."""
        try:
            doc = json.loads((output / f"{label}.json").read_text())
            if not isinstance(doc, dict) or doc.get("schema") != lc.RESULT_SCHEMA:
                raise ValueError(f"not {lc.RESULT_SCHEMA}")
            if not all(isinstance(doc.get(key), list) for key in ("errors", "unknown")):
                raise ValueError("errors and unknown must be lists")
            return doc
        except (OSError, ValueError) as exc:
            self.expect(f"{output.name}/{label}: valid result", False, str(exc))
            return {}


def saved_command(label: str, args: str) -> str:
    return f'rc=0; {args} > /out/{label}.json 2> /out/{label}.stderr || rc=$?; echo "$rc" > /out/{label}.rc; '


def rows(state: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {row["tool"]: row for row in state.get("coverage", {}).get("tools", [])}


CHECK = f"archfit check --base {lc.TARGET_BRANCH} --format json"
NO_GO = 'test -z "$(command -v go)"; test ! -e /opt/archfit/go; '


def forbidden(doc: dict[str, Any]) -> bool:
    """The record holds the fixture's deny error, added by the change."""
    return any(
        e.get("rule") == lc.RULE_ID and e.get("from_change") is True
        for e in doc.get("errors", [])
    )


def exit_code(output: Path, label: str) -> str:
    rc = output / f"{label}.rc"
    return rc.read_text().strip() if rc.is_file() else ""


def prepare_fixture(
    spec: lc.FixtureSpec, path: Path, extra: dict[str, str] | None = None
) -> None:
    """The harness fixture as a git repository: a clean target branch and a
    change branch that adds the forbidden dependency. extra joins the clean tree."""
    path.mkdir(parents=True)
    lc.write_files(path, {**spec.files, **(extra or {})})
    contract = lc.ChangeContract(lc.config_text(spec), spec.forbidden, lc.RULE_ID)
    lc.prepare_change_repo(path, contract, [])
    for file in path.rglob("*"):
        file.chmod(0o755 if file.is_dir() else 0o644)


def inventory(contract: ImageContract) -> None:
    pins = lc.TOOL_MANIFEST
    node_script = "const p='/opt/archfit/analyzers/node/node_modules/'; const m=require('/opt/archfit/manifest.json'); for(const [n,v] of Object.entries(m.node)){if(require(p+n+'/package.json').version!==v) throw Error(n)}; console.log(JSON.stringify(m.node));"
    py_script = "import importlib.metadata as m, json, platform; p=json.load(open('/opt/archfit/manifest.json')); assert platform.python_version()==p['versions']['python']; [(_ for _ in ()).throw(AssertionError(k)) for k,v in p['python'].items() if m.version(k)!=v]; import grimp, google.protobuf, grpc_tools; print(p['python'])"
    script = (
        'test "$HOME" = /tmp; ! getent passwd 27182; '
        "test ! -w /opt/archfit/manifest.json; "
        "test -r /opt/archfit/rust/lib/rustlib/src/rust/library/core/src/lib.rs; "
        f'test "$(node --version)" = v{pins["versions"]["node"]}; '
        f"{NO_GO}"
        f"rustc --version | grep -F {pins['versions']['rust']}; "
        f"cargo --version | grep -F {pins['versions']['rust']}; rust-analyzer --version; "
        f"cargo modules --version | grep -F {pins['versions']['cargo-modules']}; "
        f"uv --version | grep -F {pins['versions']['uv']}; scip-go --version | grep -F {pins['versions']['scip-go']}; "
        f'node -e {shlex.quote(node_script)}; "$ARCHFIT_TOOL_PYTHON" -I -c {shlex.quote(py_script)}; '
        "depcruise --version; ast-grep --version; jscpd --version; scip-typescript --version; scip-python --version; "
        'mkdir -p "$CARGO_HOME" "$XDG_CACHE_HOME" "$UV_CACHE_DIR" "$npm_config_cache"; '
        "archfit --version"
    )
    contract.run("inventory-uid", "27182:27182", script)


def language_cases(contract: ImageContract) -> None:
    for spec in lc.FIXTURES:
        fixture = contract.output / f"fixture-{spec.name}"
        prepare_fixture(spec, fixture)
        before = tree_hash(fixture)
        for uid in ("65532:65532", "27182:27182"):
            script = saved_command("primary", CHECK)
            if spec.name == "go":
                script = NO_GO + script
                files = " ".join(shlex.quote(name) for name in spec.forbidden)
                script += (
                    f"git checkout {lc.TARGET_BRANCH} -- {files}; "
                    + saved_command("clean", CHECK)
                )
            output = contract.run(
                f"{spec.name}-{uid.split(':')[0]}",
                uid,
                script,
                fixture,
            )
            doc = contract.read_result(output, "primary")
            contract.expect(
                f"{output.name}: real forbidden import", forbidden(doc), doc
            )
            contract.expect(
                f"{output.name}: gate exit 1",
                exit_code(output, "primary") == "1"
                and doc.get("status") == "fail"
                and doc.get("unknown") == [],
                doc.get("unknown"),
            )
            if spec.name == "go":
                clean = contract.read_result(output, "clean")
                contract.expect(
                    f"{output.name}: clean check passes without Go executable or SDK",
                    exit_code(output, "clean") == "0"
                    and clean.get("status") == "pass"
                    and clean.get("errors") == []
                    and clean.get("unknown") == [],
                    clean,
                )
        contract.expect(
            f"{spec.name}: trusted inputs unchanged", tree_hash(fixture) == before
        )


def tree_hash(path: Path) -> dict[str, str]:
    return {
        str(p.relative_to(path)): hashlib.sha256(p.read_bytes()).hexdigest()
        for p in path.rglob("*")
        if p.is_file()
    }


def python314_case(contract: ImageContract) -> None:
    fixture = contract.output / "fixture-python314"
    shutil.copytree(ROOT / "testdata/python314", fixture)
    prepare_report_repository(fixture)
    # Preserve the pre-upgrade src-layout/3.14-grammar release regression.
    for uid in ("65532:65532", "27182:27182"):
        output = contract.run(
            f"python314-{uid.split(':')[0]}",
            uid,
            saved_command(
                "report",
                "archfit report -C /tmp/repo --format json",
            ),
            fixture,
        )
        state = contract.read_state(output, "report")
        version = state.get("measurement", {}).get("tool_versions", {}).get("grimp", "")
        pin = f"grimp {lc.TOOL_MANIFEST['python']['grimp']}; python {lc.TOOL_MANIFEST['versions']['python']}"
        contract.expect(
            f"{output.name}: src-layout Python 3.14 forbidden import",
            (output / "report.rc").is_file()
            and (output / "report.rc").read_text().strip() == "0"
            and state.get("decision", {}).get("hard_gates") == "fail"
            and not state.get("decision", {}).get("unevaluated_required_rules", [])
            and rows(state).get("grimp", {}).get("status") == "ok"
            and "comparison" not in state
            and pin in version
            and any(
                f.get("rule_id") == "no-core-to-adapter" and f.get("kind") == "gate"
                for f in state.get("findings", [])
            ),
            version,
        )


DOTNET_FIXTURE = ROOT / "internal/testdata/dotnet"
DOTNET_HELPER = "dotnet /opt/archfit/dotnet-roslyn/archfit-roslyn.dll"
DOTNET_IDENTITY = re.compile(
    rf"archfit-roslyn \S+; Microsoft\.CodeAnalysis \S+; SDK {re.escape(lc.TOOL_MANIFEST['versions']['dotnet'])}"
)


def helper_status_failures(document: dict[str, Any]) -> list[str]:
    """Image statuses are fixed: one SDK, empty NuGet cache, no network (plan D7)."""
    projects = document.get("projects", [])

    def entry(project: str, tfm: str | None = None) -> dict[str, Any]:
        return next(
            (
                p
                for p in projects
                if p.get("path") == f"{project}/{project}.csproj"
                and (tfm is None or p.get("tfm") == tfm)
            ),
            {},
        )

    failures = []
    legacy = entry("Legacy", "net8.0")
    if legacy.get(
        "status"
    ) != "partial" or "Microsoft.NETCore.App.Ref 8.0" not in legacy.get("reason", ""):
        failures.append(
            f"Legacy net8.0 must be partial naming the 8.0 reference pack: {legacy}"
        )
    broken = entry("Broken")
    if broken.get(
        "status"
    ) != "failed" or "Archfit.Fixture.Missing.Sdk" not in broken.get("reason", ""):
        failures.append(f"Broken must be failed naming its SDK: {broken}")
    return failures


DOTNET_CONFIG = """version: 2
languages:
  dotnet:
    enabled: true
    projects: [Fixture.slnx]
analyzers:
  syntax:
    enabled: true
  scip:
    enabled: false
  clones:
    enabled: false
layers: [core, adapter]
modules:
  contracts:
    paths: ['Application/Contracts.cs']
    layer: core
    owner: team-core
  domain:
    paths: ['Domain/Domain.cs']
    layer: core
    owner: team-core
  repository:
    paths: ['Infrastructure/Repository.cs']
    layer: adapter
    owner: team-adapter
  tests:
    paths: ['Tests/**']
    layer: adapter
    owner: team-adapter
"""
# The warning on stderr is the only full per-project text: the roslyn coverage row
# keeps one shortened line.
DOTNET_PROJECT_STATUSES = {
    "Legacy": r"Legacy/Legacy\.csproj partial: net8\.0: .*Microsoft\.NETCore\.App\.Ref 8\.0",
    "Broken": r"Broken/Broken\.csproj failed: MSB4236: SDK 'Archfit\.Fixture\.Missing\.Sdk'",
    "Tests": r"Tests/Tests\.csproj partial: MSB4236: SDK 'Archfit\.Fixture\.Missing\.Sdk'",
    "Unresolved": r"Unresolved/Unresolved\.csproj partial: ",
}
DOTNET_COMPLETE = ("Domain", "Application", "Infrastructure")


def prepare_report_repository(path: Path) -> None:
    commands = []
    lc.git(path, commands, "report-init", "init", "-q")
    lc.git(path, commands, "report-add", "add", "-A")
    lc.git(
        path,
        commands,
        "report-commit",
        "-c",
        "user.name=Archfit Acceptance",
        "-c",
        "user.email=acceptance@example.invalid",
        "commit",
        "-qm",
        "report fixture",
    )


def prepare_dotnet_fixture(path: Path) -> None:
    shutil.copytree(DOTNET_FIXTURE, path, ignore=shutil.ignore_patterns("bin", "obj"))
    (path / ".archfit.yaml").write_text(DOTNET_CONFIG)
    prepare_report_repository(path)
    for file in path.rglob("*"):
        file.chmod(0o755 if file.is_dir() else 0o644)


def dotnet_extraction_failures(state: dict[str, Any], stderr: str) -> list[str]:
    """Deterministic image extraction: one SDK, empty NuGet cache, no network (plan D7)."""
    failures = []
    if "gate_reasons" in state:
        failures.append("report must not contain stored-reference gate reasons")
    for name, dimension in state.get("dimensions", {}).items():
        if "delta" in dimension:
            failures.append(f"{name} must not contain a stored-reference metric delta")
    tools = rows(state)
    msbuild = tools.get("dotnet-msbuild", {})
    if (
        msbuild.get("status") != "partial"
        or "Broken/Broken.csproj" not in msbuild.get("reason", "")
        or "Archfit.Fixture.Missing.Sdk" not in msbuild.get("reason", "")
    ):
        failures.append(
            f"dotnet-msbuild must be partial naming Broken's SDK: {msbuild}"
        )
    if tools.get("roslyn", {}).get("status") != "partial":
        failures.append(f"roslyn must be partial: {tools.get('roslyn')}")
    if "3 of 7 projects partial, 1 failed" not in stderr:
        failures.append("stderr must report 3 of 7 projects partial, 1 failed")
    for project, pattern in DOTNET_PROJECT_STATUSES.items():
        if not re.search(pattern, stderr):
            failures.append(f"{project} status not reported as expected")
    for project in DOTNET_COMPLETE:
        if f"{project}/{project}.csproj" in stderr:
            failures.append(f"{project} must be complete, but stderr names it")
    seams = {
        (s.get("from_module"), s.get("to_module")): s.get("strength")
        for s in state.get("seams", [])
    }
    # The dto hint folds into model at the seam: relationship classification owns that.
    for edge, strength in {
        ("repository", "contracts"): "model",
        ("repository", "domain"): "intrusive",
    }.items():
        if seams.get(edge) != strength:
            failures.append(f"seam {edge} must be {strength}: {seams.get(edge)}")
    if any(source == "tests" for source, _ in seams):
        failures.append("Tests must emit no fallback or file edges")
    return failures


def dotnet_extraction_cases(contract: ImageContract) -> None:
    fixture = contract.output / "fixture-dotnet"
    prepare_dotnet_fixture(fixture)
    before = tree_hash(fixture)
    report = "archfit report --format json"
    for uid in ("65532:65532", "27182:27182"):
        output = contract.run(
            f"dotnet-archfit-{uid.split(':')[0]}",
            uid,
            saved_command(
                "report",
                f"{report} -C /tmp/repo",
            ),
            fixture,
            timeout=900,
        )
        report_extraction(contract, output, "report")
    # D6: the snapshot makes the original tree safe to analyze while read-only.
    output = contract.run(
        "dotnet-archfit-readonly",
        "27182:27182",
        # The host owns the read-only bind. Trust only this known fixture,
        # in this disposable container's HOME, never all repositories.
        "git config --global --add safe.directory /trusted; "
        + saved_command(
            "report",
            f"{report} -C /trusted",
        ),
        fixture,
        timeout=900,
        copy=False,
    )
    report_extraction(contract, output, "report")
    contract.expect(
        "dotnet: analysis of the readonly original leaves it unchanged",
        tree_hash(fixture) == before,
    )


def report_extraction(contract: ImageContract, output: Path, label: str) -> None:
    state = contract.read_state(output, label)
    rc = output / f"{label}.rc"
    contract.expect(
        f"{output.name}: report succeeds",
        rc.is_file() and rc.read_text().strip() == "0",
    )
    stderr_file = output / f"{label}.stderr"
    stderr = stderr_file.read_text() if stderr_file.is_file() else ""
    failures = dotnet_extraction_failures(state, stderr)
    contract.expect(f"{output.name}: deterministic extraction", not failures, failures)


def dotnet_cases(contract: ImageContract) -> None:
    before = tree_hash(DOTNET_FIXTURE)
    contract.run(
        "dotnet-sdk",
        "65532:65532",
        f"dotnet --list-sdks | grep -F {lc.TOOL_MANIFEST['versions']['dotnet']}; "
        + saved_command("version", f"{DOTNET_HELPER} --version"),
    )
    for uid in ("65532:65532", "12345:12345"):
        output = contract.run(
            f"dotnet-helper-{uid.split(':')[0]}",
            uid,
            saved_command("version", f"{DOTNET_HELPER} --version")
            + saved_command("fixture", f"{DOTNET_HELPER} /tmp/repo Fixture.slnx"),
            DOTNET_FIXTURE,
            timeout=900,
        )
        try:
            identity = (output / "version.json").read_text().strip()
            document = json.loads((output / "fixture.json").read_text())
        except (OSError, ValueError) as exc:
            contract.expect(f"{output.name}: helper output", False, str(exc))
            continue
        contract.expect(
            f"{output.name}: helper identity",
            bool(DOTNET_IDENTITY.fullmatch(identity)),
            identity,
        )
        failures = helper_status_failures(document)
        contract.expect(
            f"{output.name}: deterministic statuses", not failures, failures
        )
    contract.expect(
        "dotnet: trusted fixture unchanged", tree_hash(DOTNET_FIXTURE) == before
    )
    dotnet_extraction_cases(contract)


def negative_cases(contract: ImageContract) -> None:
    spec = next(s for s in lc.FIXTURES if s.name == "typescript")
    fixture = contract.output / "fixture-negative"
    prepare_fixture(
        spec,
        fixture,
        {
            ".dependency-cruiser.cjs": "require('fs').writeFileSync('/out/POISON', 'executed'); throw Error('HEAD CONFIG');",
            ".npmrc": "registry=http://127.0.0.1:9\n",
            "sitecustomize.py": "raise RuntimeError('target Python must not load')\n",
        },
    )
    output = contract.run(
        "poison-head",
        "27182:27182",
        saved_command("check", CHECK),
        fixture,
    )
    doc = contract.read_result(output, "check")
    contract.expect(
        "poison-head: config ignored, real edge retained",
        forbidden(doc) and not (output / "POISON").exists(),
    )
    python_spec = next(s for s in lc.FIXTURES if s.name == "python")
    python_fixture = contract.output / "fixture-python-poison"
    prepare_fixture(
        python_spec,
        python_fixture,
        {
            "sitecustomize.py": "open('/out/POISON', 'w').write('sitecustomize')\n",
            ".venv/bin/python": "#!/bin/sh\necho target-interpreter > /out/POISON\nexit 88\n",
        },
    )
    (python_fixture / ".venv/bin/python").chmod(0o755)
    output = contract.run(
        "python-poison",
        "27182:27182",
        "export PYTHONPATH=/tmp/repo VIRTUAL_ENV=/tmp/repo/.venv; "
        + saved_command("check", CHECK),
        python_fixture,
    )
    doc = contract.read_result(output, "check")
    contract.expect(
        "Python isolation ignores target venv/PYTHONPATH/sitecustomize",
        forbidden(doc) and not (output / "POISON").exists(),
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--docker-context", required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    output = args.output_dir.resolve()
    output.mkdir(parents=True, exist_ok=False)
    contract = ImageContract(args.image, args.docker_context, output)
    try:
        inspect = subprocess.run(
            [*contract.docker, "image", "inspect", args.image],
            capture_output=True,
            text=True,
            check=True,
        )
        (output / "image.json").write_text(inspect.stdout)
        daemon = subprocess.run(
            [*contract.docker, "info", "--format", "{{.Architecture}}"],
            capture_output=True,
            text=True,
            check=True,
        )
        architecture = {"aarch64": "arm64", "x86_64": "amd64"}.get(
            daemon.stdout.strip(), daemon.stdout.strip()
        )
        contract.expect(
            "image is tested on native daemon architecture",
            json.loads(inspect.stdout)[0]["Architecture"] == architecture,
            architecture,
        )
        inventory(contract)
        language_cases(contract)
        python314_case(contract)
        dotnet_cases(contract)
        negative_cases(contract)
    except (OSError, subprocess.SubprocessError, ValueError) as exc:
        contract.expect("contract completed", False, str(exc))
    (output / "assertions.json").write_text(json.dumps(contract.assertions, indent=2))
    return (
        0
        if contract.assertions and all(a["passed"] for a in contract.assertions)
        else 1
    )


if __name__ == "__main__":
    raise SystemExit(main())
