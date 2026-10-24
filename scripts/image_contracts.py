#!/usr/bin/env python3
"""Bounded final-image contracts; never builds, publishes, or installs packages.

Run on native amd64 and arm64. Every failed/missing producer is a failure, not a
skip. Trusted fixture inputs are readonly; only disposable /tmp copies change.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shlex
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
            script = "cp -R /trusted /tmp/repo; cd /tmp/repo; " + script
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


def saved_command(label: str, args: str) -> str:
    return f'rc=0; {args} > /out/{label}.json 2> /out/{label}.stderr || rc=$?; echo "$rc" > /out/{label}.rc; '


def metric(state: dict[str, Any], dimension: str, name: str) -> float:
    return next(
        (
            row.get("value", 0)
            for row in state.get("dimensions", {}).get(dimension, {}).get("metrics", [])
            if row.get("name") == name
        ),
        0,
    )


def rows(state: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {row["tool"]: row for row in state.get("coverage", {}).get("tools", [])}


def forbidden(state: dict[str, Any], status: str) -> bool:
    return any(
        f.get("rule_id") == lc.RULE_ID and f.get("status") == status
        for f in state.get("findings", [])
    )


def clone_source(language: str) -> str:
    if language == "go":
        return (
            "\nfunc Calculate(values []int) int {\n total := 0\n"
            + "".join(f" total += len(values) * {i}\n" for i in range(1, 40))
            + " return total\n}\n"
        )
    if language in ("typescript", "javascript"):
        arg = "values: number[]" if language == "typescript" else "values"
        return (
            f"\nexport function calculate({arg}) {{\n let total = 0;\n"
            + "".join(f" total += values.length * {i};\n" for i in range(1, 40))
            + " return total;\n}\n"
        )
    if language == "python":
        return (
            "\ndef calculate(values):\n    total = 0\n"
            + "".join(f"    total += len(values) * {i}\n" for i in range(1, 40))
            + "    return total\n"
        )
    return (
        "\npub fn calculate(values: &[u32]) -> u32 {\n let mut total = 0;\n"
        + "".join(f" total += values.len() as u32 * {i};\n" for i in range(1, 40))
        + " total\n}\n"
    )


def prepare_fixture(spec: lc.FixtureSpec, path: Path) -> None:
    path.mkdir(parents=True)
    for name, content in spec.files.items():
        target = path / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)
    if spec.name == "javascript":
        # SCIP's TypeScript compiler needs a data-only project for JavaScript input.
        (path / "tsconfig.json").write_text(
            json.dumps(
                {
                    "compilerOptions": {
                        "allowJs": True,
                        "checkJs": True,
                        "module": "ESNext",
                        "moduleResolution": "bundler",
                        "target": "ES2022",
                    },
                    "include": ["src/**/*.js"],
                },
                indent=2,
            )
        )
    config = lc.config_text(spec)
    clone_modules = ""
    for side in ("left", "right"):
        if spec.name == "rust":
            relative = f"clone-{side}/src/lib.rs"
            crate = path / f"clone-{side}/Cargo.toml"
            crate.parent.mkdir(parents=True)
            crate.write_text(
                f'[package]\nname = "clone-{side}"\nversion = "0.1.0"\nedition = "2021"\n'
            )
            selector = f"clone-{side}"
            content = clone_source("rust")
        elif spec.name == "python":
            relative = f"smoke/clone_{side}.py"
            selector = f"smoke.clone_{side}**"
            content = clone_source("python")
        else:
            ext = {"go": "go", "typescript": "ts", "javascript": "js"}[spec.name]
            directory = f"clones/{side}" if spec.name == "go" else f"src/clones/{side}"
            relative = f"{directory}/duplicate.{ext}"
            selector = f"{directory}/**"
            content = (
                "package duplicate\n" if spec.name == "go" else ""
            ) + clone_source(spec.name)
        target = path / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)
        selectors = (
            f"'{selector}', '{selector}/**', '{selector.replace('-', '_')}', '{selector.replace('-', '_')}::*'"
            if spec.name == "rust"
            else f"'{selector}'"
        )
        clone_modules += f"  clone_{side}:\n    paths: [{selectors}]\n    layer: core\n    owner: team-{side}\n"
    if spec.name == "rust":
        # Crate-dependency keys use package names; module evidence uses target names.
        for package in ("smoke-core", "smoke-adapter"):
            target_name = package.replace("-", "_")
            config = config.replace(
                f"paths: ['{package}']",
                f"paths: ['{package}', '{target_name}', '{target_name}::*']",
            )
        cargo = path / "Cargo.toml"
        cargo.write_text(
            cargo.read_text().replace(
                '["core", "adapter"]',
                '["core", "adapter", "clone-left", "clone-right"]',
            )
        )
    config = config.replace("rules:\n", clone_modules + "rules:\n")
    (path / ".archfit.yaml").write_text(config)
    deep = config.replace("scip:\n    enabled: false", "scip:\n    enabled: true")
    deep = deep.replace("clones:\n    enabled: false", "clones:\n    enabled: true")
    if spec.name == "rust":
        deep = deep.replace(
            "cargo_modules:\n    enabled: false", "cargo_modules:\n    enabled: true"
        )
    (path / "deep.yaml").write_text(deep)
    if spec.node:
        # No third-party target imports. An empty dependency directory is real,
        # not a copied analyzer installation, and is sufficient for local SCIP.
        (path / "node_modules").mkdir()
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
        f"go version | grep -F go{pins['versions']['go']}; "
        'test "$(go env GOHOSTARCH)" = "$(case $(uname -m) in aarch64) echo arm64;; x86_64) echo amd64;; esac)"; '
        f"rustc --version | grep -F {pins['versions']['rust']}; "
        f"cargo --version | grep -F {pins['versions']['rust']}; rust-analyzer --version; "
        f"cargo modules --version | grep -F {pins['versions']['cargo-modules']}; "
        f"uv --version | grep -F {pins['versions']['uv']}; scip-go --version | grep -F {pins['versions']['scip-go']}; "
        f'node -e {shlex.quote(node_script)}; "$ARCHFIT_TOOL_PYTHON" -I -c {shlex.quote(py_script)}; '
        "depcruise --version; ast-grep --version; jscpd --version; scip-typescript --version; scip-python --version; "
        'mkdir -p "$GOCACHE" "$GOPATH" "$CARGO_HOME" "$XDG_CACHE_HOME" "$UV_CACHE_DIR" "$npm_config_cache"; '
        "archfit --version"
    )
    contract.run("inventory-uid", "27182:27182", script)


def language_cases(contract: ImageContract) -> None:
    common = "--root /tmp/repo --config /tmp/repo/.archfit.yaml"
    for spec in lc.FIXTURES:
        fixture = contract.output / f"fixture-{spec.name}"
        prepare_fixture(spec, fixture)
        before = tree_hash(fixture)
        for uid in ("65532:65532", "27182:27182"):
            script = saved_command("primary", f"archfit check {common} --json --quiet")
            script += saved_command("capture", f"archfit baseline {common}")
            script += saved_command(
                "baseline", f"archfit check {common} --json --quiet"
            )
            script += saved_command("reanchor", f"archfit baseline {common} --reanchor")
            script += saved_command(
                "reanchored", f"archfit check {common} --json --quiet"
            )
            script += saved_command(
                "deep",
                "archfit check --root /tmp/repo --config /tmp/repo/deep.yaml --json --quiet --refresh",
            )
            output = contract.run(
                f"{spec.name}-{uid.split(':')[0]}", uid, script, fixture
            )
            state = contract.read_state(output, "primary")
            contract.expect(
                f"{output.name}: real forbidden import", forbidden(state, "new")
            )
            contract.expect(
                f"{output.name}: gate exit 1",
                (output / "primary.rc").is_file()
                and (output / "primary.rc").read_text().strip() == "1",
            )
            contract.expect(
                f"{output.name}: nonempty syntax facts",
                metric(state, "complexity", "function_loc_max") > 0,
            )
            for name in ("capture", "reanchor"):
                rc = output / f"{name}.rc"
                contract.expect(
                    f"{output.name}: {name} succeeds",
                    rc.is_file() and rc.read_text().strip() == "0",
                )
            for label in ("baseline", "reanchored"):
                captured = contract.read_state(output, label)
                contract.expect(
                    f"{output.name}: {label} comparable accepted debt",
                    forbidden(captured, "baseline")
                    and captured.get("gate_reference", {}).get("status")
                    == "comparable",
                )
            deep = contract.read_state(output, "deep")
            tools = rows(deep)
            contract.expect(
                f"{output.name}: deep SCIP and symbols",
                all(
                    tools.get(t, {}).get("status") == "ok"
                    for t in ("scip", "scip-symbols")
                ),
                tools,
            )
            contract.expect(
                f"{output.name}: semantic functional seam",
                any(
                    s.get("from_module") == "core"
                    and s.get("to_module") == "adapter"
                    and s.get("strength") == "functional"
                    and s.get("scored_edges", 0) > 0
                    for s in deep.get("seams", [])
                ),
                deep.get("seams"),
            )
            contract.expect(
                f"{output.name}: clone positive control",
                tools.get("jscpd", {}).get("status") == "ok"
                and any(
                    f.get("rule_id") == "bc/duplicated_knowledge"
                    for f in deep.get("findings", [])
                ),
            )
            if spec.name == "rust":
                contract.expect(
                    f"{output.name}: module positive control",
                    tools.get("cargo-modules", {}).get("status") == "ok",
                    tools.get("cargo-modules"),
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
    # Preserve the pre-upgrade src-layout/3.14-grammar release regression.
    for uid in ("65532:65532", "27182:27182"):
        output = contract.run(
            f"python314-{uid.split(':')[0]}",
            uid,
            saved_command(
                "check",
                "archfit check --root /tmp/repo --config /tmp/repo/.archfit.yaml --json --quiet",
            ),
            ROOT / "testdata/python314",
        )
        state = contract.read_state(output, "check")
        producers = (
            state.get("comparison", {})
            .get("measurement_profile", {})
            .get("producers", [])
        )
        producer = next((row for row in producers if row.get("tool") == "grimp"), {})
        pin = f"grimp {lc.TOOL_MANIFEST['python']['grimp']}; python {lc.TOOL_MANIFEST['versions']['python']}"
        contract.expect(
            f"{output.name}: src-layout Python 3.14 forbidden import",
            (output / "check.rc").is_file()
            and (output / "check.rc").read_text().strip() == "1"
            and state.get("decision", {}).get("hard_gates") == "fail"
            and not state.get("decision", {}).get("unevaluated_required_rules", [])
            and rows(state).get("grimp", {}).get("status") == "ok"
            and producer.get("semantics_version") == "grimp.v3"
            and pin in producer.get("tool_version", "")
            and any(
                f.get("rule_id") == "no-core-to-adapter" and f.get("kind") == "gate"
                for f in state.get("findings", [])
            ),
            producer,
        )


def negative_cases(contract: ImageContract) -> None:
    spec = next(s for s in lc.FIXTURES if s.name == "typescript")
    fixture = contract.output / "fixture-negative"
    prepare_fixture(spec, fixture)
    (fixture / ".dependency-cruiser.cjs").write_text(
        "require('fs').writeFileSync('/out/POISON', 'executed'); throw Error('HEAD CONFIG');"
    )
    (fixture / ".npmrc").write_text("registry=http://127.0.0.1:9\n")
    (fixture / "sitecustomize.py").write_text(
        "raise RuntimeError('target Python must not load')\n"
    )
    common = "check --root /tmp/repo --config /tmp/repo/.archfit.yaml --json --quiet --refresh"
    output = contract.run(
        "poison-head",
        "27182:27182",
        saved_command("check", f"archfit {common}"),
        fixture,
    )
    state = contract.read_state(output, "check")
    contract.expect(
        "poison-head: config ignored, real edge retained",
        forbidden(state, "new") and not (output / "POISON").exists(),
    )
    python_spec = next(s for s in lc.FIXTURES if s.name == "python")
    python_fixture = contract.output / "fixture-python-poison"
    prepare_fixture(python_spec, python_fixture)
    (python_fixture / "sitecustomize.py").write_text(
        "open('/out/POISON', 'w').write('sitecustomize')\n"
    )
    venv = python_fixture / ".venv/bin"
    venv.mkdir(parents=True)
    interpreter = venv / "python"
    interpreter.write_text(
        "#!/bin/sh\necho target-interpreter > /out/POISON\nexit 88\n"
    )
    interpreter.chmod(0o755)
    output = contract.run(
        "python-poison",
        "27182:27182",
        "export PYTHONPATH=/tmp/repo VIRTUAL_ENV=/tmp/repo/.venv; "
        + saved_command("check", f"archfit {common}"),
        python_fixture,
    )
    state = contract.read_state(output, "check")
    contract.expect(
        "Python isolation ignores target venv/PYTHONPATH/sitecustomize",
        forbidden(state, "new")
        and rows(state).get("grimp", {}).get("status") == "ok"
        and not (output / "POISON").exists(),
    )
    unsupported = contract.output / "fixture-unsupported-rust"
    unsupported.mkdir()
    (unsupported / "Cargo.toml").write_text(
        '[package]\nname = "unsupported-bench"\nversion = "0.1.0"\nedition = "2021"\nautolib = false\nautobins = false\n\n[[bench]]\nname = "only"\npath = "only.rs"\nharness = false\n'
    )
    (unsupported / "only.rs").write_text("fn main() {}\n")
    rust_spec = next(s for s in lc.FIXTURES if s.name == "rust")
    (unsupported / ".archfit.yaml").write_text(
        lc.config_text(rust_spec).replace(
            "cargo_modules:\n    enabled: false", "cargo_modules:\n    enabled: true"
        )
    )
    output = contract.run(
        "unsupported-rust-target",
        "27182:27182",
        saved_command("check", f"archfit {common}"),
        unsupported,
    )
    state = contract.read_state(output, "check")
    module = rows(state).get("cargo-modules", {})
    contract.expect(
        "unsupported bench-only module target is truthfully partial",
        module.get("status") == "partial" and bool(module.get("reason")),
        module,
    )
    compiler_fixture = contract.output / "fixture-compiler"
    prepare_fixture(spec, compiler_fixture)
    healthy = contract.run(
        "compiler-present",
        "27182:27182",
        saved_command("check", f"archfit {common}"),
        compiler_fixture,
    )
    contract.expect(
        "compiler-present: real forbidden edge",
        forbidden(contract.read_state(healthy, "check"), "new"),
    )
    # Overlay only the compiler directory (empty), leaving the producer intact.
    # Mounting an empty directory is reversible and never mutates the image.
    empty = contract.output / "missing-compiler"
    empty.mkdir()
    output = contract.output / "compiler-negative"
    output.mkdir()
    output.chmod(0o777)
    args = [
        *contract.docker,
        "run",
        "--rm",
        "--user",
        "27182:27182",
        "--cap-drop",
        "ALL",
        "--security-opt",
        "no-new-privileges",
        "--read-only",
        "--network",
        "none",
        "--tmpfs",
        "/tmp:rw,exec,nosuid,size=512m,mode=1777",
        "--env",
        "HOME=/tmp",
        "--mount",
        f"type=bind,source={compiler_fixture},target=/trusted,readonly",
        "--mount",
        f"type=bind,source={output},target=/out",
        "--mount",
        f"type=bind,source={empty},target=/opt/archfit/analyzers/node/node_modules/typescript,readonly",
        "--entrypoint",
        "/bin/sh",
        contract.image,
        "-ec",
        "cp -R /trusted /tmp/repo; cd /tmp/repo; "
        + saved_command("check", f"archfit {common}"),
    ]
    (output / "command.json").write_text(json.dumps(args, indent=2))
    proc = subprocess.run(
        args, capture_output=True, text=True, timeout=180, check=False
    )
    (output / "stderr.log").write_text(proc.stderr)
    contract.expect(
        "missing compiler: container execution", proc.returncode == 0, proc.stderr
    )
    state = contract.read_state(output, "check")
    coverage = rows(state).get("dependency-cruiser", {})
    contract.expect(
        "missing compiler: fails closed, never valid empty graph",
        (output / "check.rc").is_file()
        and (output / "check.rc").read_text().strip() == "2"
        and state.get("verdict") == "needs_attention"
        and state.get("decision", {}).get("hard_gates") == "unmeasured"
        and coverage.get("status") == "partial"
        and "compiler" in coverage.get("reason", "").lower()
        and any(
            r.get("rule_id") == lc.RULE_ID
            for r in state.get("decision", {}).get("unevaluated_required_rules", [])
        ),
        coverage,
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
