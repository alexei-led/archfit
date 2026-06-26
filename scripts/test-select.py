#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from collections import defaultdict, deque
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, cast

DOC_SUFFIXES = {".md", ".mdx", ".rst", ".txt"}
DOC_BASENAMES = {"README.md", "CLAUDE.md", "AGENTS.md", "AGENT.md"}
DOC_PREFIXES = ("docs/",)
FULL_SCOPE_PATHS = {
    "go.mod",
    "go.sum",
    "Makefile",
    ".archfit.yaml",
    ".golangci.yaml",
    ".pre-commit-config.yaml",
}
FULL_SCOPE_PREFIXES = (".github/",)
MAX_FOCUSED_PACKAGES = 10


@dataclass(frozen=True)
class PackageInfo:
    import_path: str
    rel_dir: str
    deps: frozenset[str]

    @property
    def test_target(self) -> str:
        return "." if self.rel_dir == "" else f"./{self.rel_dir}"


@dataclass(frozen=True)
class Decision:
    kind: str
    reason: str
    argv: tuple[str, ...]
    packages: tuple[str, ...] = ()

    @property
    def command(self) -> str:
        return " ".join(self.argv)


ROOT = Path.cwd().resolve()


def run_capture(argv: list[str]) -> str:
    proc = subprocess.run(
        argv, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )
    return proc.stdout


def changed_paths(explicit: Iterable[str]) -> list[str]:
    if explicit:
        return unique_paths(explicit)

    tracked = run_capture(
        ["git", "diff", "--name-only", "--relative", "HEAD"]
    ).splitlines()
    untracked = run_capture(
        ["git", "ls-files", "--others", "--exclude-standard"]
    ).splitlines()
    return unique_paths([*tracked, *untracked])


def unique_paths(paths: Iterable[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for raw in paths:
        path = normalize_path(raw)
        if not path or path in seen:
            continue
        seen.add(path)
        out.append(path)
    return sorted(out)


def normalize_path(raw: str) -> str:
    path = raw.strip()
    if not path:
        return ""
    p = Path(path)
    if p.is_absolute():
        p = p.resolve().relative_to(ROOT)
    return p.as_posix().lstrip("./")


def is_docs_only(path: str) -> bool:
    rel = Path(path)
    if rel.name in DOC_BASENAMES:
        return True
    if rel.suffix in DOC_SUFFIXES:
        return True
    return any(path.startswith(prefix) for prefix in DOC_PREFIXES)


def requires_full_scope(path: str) -> bool:
    if path in FULL_SCOPE_PATHS:
        return True
    return any(path.startswith(prefix) for prefix in FULL_SCOPE_PREFIXES)


def package_inventory() -> dict[str, PackageInfo]:
    raw = run_capture(["go", "list", "-json", "./..."])
    decoder = json.JSONDecoder()
    idx = 0
    pkgs: list[PackageInfo] = []
    names: set[str] = set()
    staged: list[dict[str, object]] = []

    while idx < len(raw):
        while idx < len(raw) and raw[idx].isspace():
            idx += 1
        if idx >= len(raw):
            break
        obj, idx = decoder.raw_decode(raw, idx)
        pkg = cast(dict[str, Any], obj)
        staged.append(pkg)
        names.add(cast(str, pkg["ImportPath"]))

    for obj in staged:
        rel_dir = Path(cast(str, obj["Dir"])).resolve().relative_to(ROOT).as_posix()
        if rel_dir == ".":
            rel_dir = ""
        deps = set(cast(list[str], obj.get("Imports", [])))
        deps.update(cast(list[str], obj.get("TestImports", [])))
        deps.update(cast(list[str], obj.get("XTestImports", [])))
        pkgs.append(
            PackageInfo(
                import_path=cast(str, obj["ImportPath"]),
                rel_dir=rel_dir,
                deps=frozenset(dep for dep in deps if dep in names),
            )
        )

    return {pkg.import_path: pkg for pkg in pkgs}


def package_for_path(path: str, packages: dict[str, PackageInfo]) -> PackageInfo | None:
    candidates = sorted(
        packages.values(), key=lambda pkg: len(pkg.rel_dir), reverse=True
    )
    for pkg in candidates:
        if pkg.rel_dir == "":
            continue
        if path == pkg.rel_dir or path.startswith(pkg.rel_dir + "/"):
            return pkg
    return None


def reverse_dependents(
    packages: dict[str, PackageInfo], changed: set[str]
) -> list[PackageInfo]:
    reverse: dict[str, set[str]] = defaultdict(set)
    for pkg in packages.values():
        for dep in pkg.deps:
            reverse[dep].add(pkg.import_path)

    affected = set(changed)
    queue = deque(changed)
    while queue:
        dep = queue.popleft()
        for importer in reverse.get(dep, ()):
            if importer in affected:
                continue
            affected.add(importer)
            queue.append(importer)

    return sorted(
        (packages[name] for name in affected), key=lambda pkg: pkg.test_target
    )


def decide(paths: list[str]) -> Decision:
    if not paths:
        return Decision("skip", "no changed files", ())
    if all(is_docs_only(path) for path in paths):
        return Decision("skip", "docs-only change", ())
    if any(requires_full_scope(path) for path in paths):
        return Decision("full", "repo-wide or tooling change", ("go", "test", "./..."))

    packages = package_inventory()
    changed_imports: set[str] = set()

    for path in paths:
        if is_docs_only(path):
            continue
        pkg = package_for_path(path, packages)
        if pkg is None:
            return Decision(
                "full", f"can't map {path} to a Go package", ("go", "test", "./...")
            )
        changed_imports.add(pkg.import_path)

    if not changed_imports:
        return Decision("skip", "docs-only change", ())

    affected = reverse_dependents(packages, changed_imports)
    if len(affected) > MAX_FOCUSED_PACKAGES:
        return Decision(
            "full",
            f"broad impact across {len(affected)} packages",
            ("go", "test", "./..."),
        )

    argv = tuple(["go", "test", *[pkg.test_target for pkg in affected]])
    return Decision(
        "focused",
        f"{len(affected)} affected package(s)",
        argv,
        tuple(pkg.test_target for pkg in affected),
    )


def render(decision: Decision) -> str:
    lines = [f"decision: {decision.kind}", f"reason: {decision.reason}"]
    if decision.packages:
        lines.append("packages:")
        lines.extend(f"  - {pkg}" for pkg in decision.packages)
    if decision.argv:
        lines.append(f"command: {decision.command}")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Pick the smallest safe local go test command for a change set."
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Changed paths. Defaults to git diff + untracked files.",
    )
    parser.add_argument(
        "--command", action="store_true", help="Print only the selected command."
    )
    parser.add_argument("--run", action="store_true", help="Run the selected command.")
    args = parser.parse_args()

    decision = decide(changed_paths(args.paths))

    if args.command:
        if decision.argv:
            print(decision.command)
        return 0

    if args.run:
        print(render(decision))
        if not decision.argv:
            return 0
        return subprocess.run(list(decision.argv)).returncode

    print(render(decision))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as exc:
        if exc.stderr:
            sys.stderr.write(exc.stderr)
        elif exc.stdout:
            sys.stderr.write(exc.stdout)
        raise SystemExit(exc.returncode)
