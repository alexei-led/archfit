#!/usr/bin/env python3
"""Local design probes, not release certification. No pulls, builds, or installs.

Requires an exported engine tree at 11e84d11 and the already prepared candidate.
All fixtures and raw output go to a new ~/.cache/archfit-* directory.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
import time
import uuid
from pathlib import Path

IMAGE = "sha256:c206455444ebae08fad47b2b3a4ff503e8b466f615bc4cf8a2a3c6d954302714"
SOURCE = "11e84d1152d96ca724ac27326932ff8a689a6439"
SOURCE_FILES = {
    "scripts/eval/language_contracts.py": "8ee380262e7a44ffee86290d33323c42374bec04ea3515fe80aa4ca4e6276320",
    "scripts/eval/corpus_sweep.py": "a0a8fbd6428633390f97127dba85a260105e5b8545e53a2b01ae26d186adc721",
    "tools/manifest.json": "4f0660a4d9796844669b3d069f8635f9ce96cc5c62afd7e48d52954d7d7dffee",
}


def save_check(label: str, extra: str = "") -> str:
    return (
        f"rc=0; archfit check --json --progress=none {extra} "
        f"> /out/{label}.json 2> /out/{label}.stderr || rc=$?; "
        f"echo $rc > /out/{label}.rc; "
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--case", action="append", default=[])
    args = parser.parse_args()
    output = args.output.resolve()
    cache = Path.home() / ".cache"
    if not output.is_relative_to(cache) or not output.relative_to(cache).parts[
        0
    ].startswith("archfit-"):
        parser.error("output must be under ~/.cache/archfit-*")
    for name, expected in SOURCE_FILES.items():
        actual = hashlib.sha256((args.source / name).read_bytes()).hexdigest()
        if actual != expected:
            parser.error(f"{name} does not match reviewed source {SOURCE}")
    output.mkdir(parents=True, exist_ok=False)
    sys.path.insert(0, str(args.source.resolve()))
    from scripts.eval import language_contracts as lc

    docker = ["docker", "--context", "colima"]
    inspect = subprocess.check_output([*docker, "image", "inspect", IMAGE], text=True)
    (output / "image.json").write_text(inspect)
    specs = {spec.name: spec for spec in lc.FIXTURES}
    observations: list[dict] = []

    def fixture(name: str, language: str, changes: dict[str, str | None]) -> Path:
        root = output / "fixtures" / name
        root.mkdir(parents=True)
        spec = specs[language]
        files = {**spec.files, ".archfit.yaml": lc.config_text(spec)}
        for path, content in changes.items():
            if content is None:
                files.pop(path, None)
            else:
                files[path] = content
        for path, content in files.items():
            target = root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
            target.chmod(0o755 if path.endswith(".sh") else 0o644)
        return root

    def run(
        name: str,
        language: str,
        expected: str,
        changes=None,
        *,
        script: str = "",
        mounts=(),
        network: str = "none",
    ) -> None:
        if args.case and name not in args.case:
            return
        root = fixture(name, language, changes or {})
        out = output / name
        out.mkdir()
        out.chmod(0o777)
        container = "archfit-design-" + uuid.uuid4().hex[:12]
        command = [
            *docker,
            "run",
            "--rm",
            "--pull=never",
            "--name",
            container,
            "--user",
            "27182:27182",
            "--cpus=2",
            "--memory=2g",
            "--pids-limit=256",
            "--read-only",
            "--network",
            network,
            "--cap-drop=ALL",
            "--security-opt=no-new-privileges",
            "--tmpfs",
            "/tmp:rw,exec,nosuid,size=768m,mode=1777",
            "--mount",
            f"type=bind,source={root},target=/fixture,readonly",
            "--mount",
            f"type=bind,source={out},target=/out",
        ]
        for source, target in mounts:
            command += [
                "--mount",
                f"type=bind,source={source},target={target},readonly",
            ]
        command += [
            "--entrypoint",
            "/bin/sh",
            IMAGE,
            "-ec",
            "cp -R /fixture /tmp/repo; cd /tmp/repo; "
            + (script or save_check("check")),
        ]
        (out / "command.json").write_text(json.dumps(command, indent=2) + "\n")
        started = time.monotonic()
        try:
            proc = subprocess.run(
                command, check=False, capture_output=True, text=True, timeout=120
            )
            code, stdout, stderr = proc.returncode, proc.stdout, proc.stderr
        except subprocess.TimeoutExpired:
            subprocess.run(
                [*docker, "rm", "-f", container],
                check=False,
                capture_output=True,
                timeout=15,
            )
            code, stdout, stderr = (
                124,
                "",
                "120s fixture timeout; only owned container removed",
            )
        elapsed = time.monotonic() - started
        (out / "stdout.txt").write_text(stdout)
        (out / "stderr.txt").write_text(stderr)
        states = {}
        for path in sorted(out.glob("*.json")):
            if path.name == "command.json":
                continue
            try:
                state = json.loads(path.read_text())
            except ValueError:
                continue
            states[path.stem] = {
                "verdict": state.get("verdict"),
                "decision": state.get("decision"),
                "coverage": state.get("coverage"),
                "findings": [
                    f
                    for f in state.get("findings", [])
                    if f.get("rule_id") == lc.RULE_ID
                ],
                "exit": (out / f"{path.stem}.rc").read_text().strip(),
            }
        observations.append(
            {
                "name": name,
                "expected": expected,
                "container_exit": code,
                "elapsed_seconds": round(elapsed, 3),
                "states": states,
                "marker": (out / "injected").exists(),
                "fixture_sha256": {
                    str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
                    for p in sorted(root.rglob("*"))
                    if p.is_file()
                },
            }
        )
        (output / "results.json").write_text(
            json.dumps(
                {
                    "image": IMAGE,
                    "source": SOURCE,
                    "observations": observations,
                },
                indent=2,
            )
            + "\n"
        )
        print(
            name,
            code,
            round(elapsed, 3),
            {
                key: (value["verdict"], len(value["findings"]))
                for key, value in states.items()
            },
            "marker",
            (out / "injected").exists(),
            flush=True,
        )

    for language in specs:
        run(
            language,
            language,
            "Known core -> adapter forbidden edge retained; core_no_adapter violated.",
        )

    run(
        "ts-one-invalid",
        "typescript",
        "Invalid required source must not be certified complete.",
        {
            "src/core/bad.ts": "import { value } from '../adapter/index';\nexport const broken = ;\n",
        },
    )
    run(
        "ts-invalid-import",
        "typescript",
        "Malformed import must make whole required scope incomplete, not pass.",
        {
            "src/core/index.ts": "export const run = () => 1;\n",
            "src/core/bad.ts": 'import { value } frox "../adapter/index";\nexport const run = () => value();\n',
        },
    )
    run(
        "ts-empty",
        "typescript",
        "No eligible inputs is distinct from compiler failure.",
        {
            "src/core/index.ts": None,
            "src/adapter/index.ts": None,
        },
    )
    run(
        "ts-zero-edges",
        "typescript",
        "Two valid source files, zero imports, no forbidden finding.",
        {
            "src/core/index.ts": "export const run = () => 1;\n",
        },
    )
    run(
        "ts-orphan",
        "typescript",
        "Sources without package.json must not disappear from required scope.",
        {
            "package.json": None,
        },
    )
    run(
        "ts-executable-config",
        "typescript",
        "Ignore executable analyzer config; keep forbidden edge; no marker.",
        {
            ".dependency-cruiser.cjs": "require('fs').writeFileSync('/out/injected','config'); throw Error('executed');\n",
            "node_modules/typescript/package.json": '{"name":"typescript","version":"999.0.0","main":"index.js"}',
            "node_modules/typescript/index.js": "require('fs').writeFileSync('/out/injected','compiler'); throw Error('executed');\n",
        },
    )
    empty = output / "empty-compiler"
    empty.mkdir()
    run(
        "ts-missing-compiler",
        "typescript",
        "Required rule unevaluated; partial compiler coverage; never healthy.",
        mounts=[(empty, "/opt/archfit/analyzers/node/node_modules/typescript")],
    )
    run(
        "js-no-ts-compiler",
        "javascript",
        "JS import edge still measured without optional TS compiler.",
        mounts=[(empty, "/opt/archfit/analyzers/node/node_modules/typescript")],
    )
    run(
        "ts-local-node",
        "typescript",
        "Refuse project-owned Node; no marker.",
        {
            "node.sh": "#!/bin/sh\necho node > /out/injected\nexit 1\n",
        },
        script="mkdir /tmp/repo/bin; cp node.sh bin/node; chmod +x bin/node; export PATH=/tmp/repo/bin:$PATH; "
        + save_check("check"),
    )
    run(
        "ts-cache",
        "typescript",
        "Cold, warm, refreshed facts agree; cache tampering must not remove forbidden edge.",
        script=save_check("cold")
        + save_check("warm")
        + """
python3 - <<'PY'
import json
from pathlib import Path
for path in Path('.archfit-cache/facts/typescript').glob('*.json'):
    blob = json.loads(path.read_text())
    Path('/out/cache-original.txt').write_text(path.read_text())
    Path('/out/cache-path.txt').write_text(str(path))
    # Runner serializes tool output as JSON with base64-encoded Stdout.
    import base64
    raw = json.loads(base64.b64decode(blob['stdout']))
    for module in raw.get('modules', []):
        module['dependencies'] = []
    blob['stdout'] = base64.b64encode(json.dumps(raw).encode()).decode()
    path.write_text(json.dumps(blob))
PY
"""
        + save_check("tampered")
        + save_check("refreshed", "--refresh"),
    )
    run(
        "py314",
        "python",
        "Python 3.14 t-string syntax retains known forbidden import.",
        {
            "smoke/core.py": "from smoke.adapter import value\nmessage = t'value={value()}'\n",
        },
    )
    run(
        "py-one-invalid",
        "python",
        "One invalid Python file makes required imports incomplete.",
        {
            "smoke/bad.py": "from smoke.adapter import value\nthis is !!! invalid\n",
        },
    )
    run(
        "py-target-upper-bound",
        "python",
        "Static parser must not reject solely because target requires-python excludes3.14.",
        {
            "pyproject.toml": '[project]\nname="fixture"\nversion="0.1.0"\nrequires-python=">=3.12,<3.14"\n',
        },
    )
    run(
        "go126-grammar",
        "go",
        "Go1.26 new(expression) syntax retains forbidden import.",
        {
            "go.mod": "module example.com/language-contract\n\ngo 1.26\n",
            "core/core.go": 'package core\nimport "example.com/language-contract/adapter"\nfunc Run() int { return *new(adapter.Value()) }\n',
        },
    )
    dependency = {
        "go.mod": "module example.com/language-contract\n\ngo 1.25\n\nrequire github.com/google/uuid v1.6.0\n",
        "core/core.go": 'package core\nimport ("example.com/language-contract/adapter"; "github.com/google/uuid")\nfunc Run() int { return adapter.Value()+int(uuid.Nil[0]) }\n',
    }
    run(
        "go-dependency-offline",
        "go",
        "Offline unprepared dependency is disclosed without hiding internal edge.",
        dependency,
    )
    run(
        "go-dependency-prepared",
        "go",
        "Online exact dependency preparation retains internal edge.",
        dependency,
        network="bridge",
        script="export GOMODCACHE=/out/modcache; go mod download github.com/google/uuid > /out/preparation.txt 2>&1; "
        + "cp go.sum /out/prepared-go.sum; "
        + save_check("online"),
    )
    if not args.case or "go-dependency-prepared" in args.case:
        if args.case:
            args.case.append("go-dependency-prepared-offline")
        run(
            "go-dependency-prepared-offline",
            "go",
            "Prepared module cache works in a separate network=none container.",
            dependency,
            mounts=[(output / "go-dependency-prepared", "/prepared")],
            script="cp /prepared/prepared-go.sum go.sum; export GOMODCACHE=/prepared/modcache GOPROXY=off GOSUMDB=off; "
            + save_check("offline", "--refresh"),
        )
    run(
        "rust2024-grammar",
        "rust",
        "Rust2024 let chains retain crate and module evidence.",
        {
            "core/Cargo.toml": specs["rust"]
            .files["core/Cargo.toml"]
            .replace('edition = "2021"', 'edition = "2024"'),
            "core/src/lib.rs": "pub fn run() -> u32 { if let Some(x) = Some(smoke_adapter::value()) && x > 0 { x } else { 0 } }\n",
            ".archfit.yaml": lc.config_text(specs["rust"]).replace(
                "cargo_modules:\n    enabled: false",
                "cargo_modules:\n    enabled: true",
            ),
        },
    )
    run(
        "go-next-minimum",
        "go",
        "Minimum1.28 exceeds1.27.3: bounded gap, no auto toolchain download.",
        {
            "go.mod": "module example.com/language-contract\n\ngo 1.28\n",
        },
    )
    run(
        "go-mixed-workspace",
        "go",
        "Incompatible workspace member cannot be hidden by compatible root.",
        {
            "go.work": "go 1.27.3\n\nuse (\n .\n ./next\n)\n",
            "next/go.mod": "module example.com/next\n\ngo 1.28\n",
            "next/main.go": "package next\nfunc Value() int { return 1 }\n",
        },
    )
    run(
        "ts-missing-dependency",
        "typescript",
        "Unavailable target dependency visible; analyzer stays preinstalled.",
        {
            "src/core/index.ts": 'import { value } from "../adapter/index";\nimport missing from "fixture-never-installed";\nexport const run = () => value() + missing;\n',
        },
    )
    run(
        "rust-unsupported-edition",
        "rust",
        "Cargo rejects unknown edition; required crate graph unavailable.",
        {
            "core/Cargo.toml": specs["rust"]
            .files["core/Cargo.toml"]
            .replace('edition = "2021"', 'edition = "2099"'),
        },
    )
    run(
        "rust-wrapper",
        "rust",
        "Observe whether target cargo configuration executes; no sandbox claim.",
        {
            ".cargo/config.toml": '[build]\nrustc-wrapper = "/tmp/repo/wrapper.sh"\n',
            "wrapper.sh": '#!/bin/sh\necho wrapper > /out/injected\nexec "$@"\n',
            ".archfit.yaml": lc.config_text(specs["rust"]).replace(
                "cargo_modules:\n    enabled: false",
                "cargo_modules:\n    enabled: true",
            ),
        },
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
