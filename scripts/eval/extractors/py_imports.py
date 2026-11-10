"""Parser-based Python import extractor: ast-grep plus a built-in resolver.

A measurement prototype for the Archfit 4 design, section 11.8. It mirrors
today's extractor (internal/extract/py and its grimp helper) without Python
tooling on the target: the packages and their source roots, grimp's module
rules, and its handling of ``TYPE_CHECKING`` imports. Nodes are dotted modules.
"""

from __future__ import annotations

import importlib.util
import os
import sys
from dataclasses import dataclass
from pathlib import Path

from . import common


@dataclass(frozen=True)
class Import:
    """One imported module as written: ``import a.b`` or ``from m import names``."""

    module: str
    names: tuple[str, ...] = ()
    is_from: bool = False


def packages(root: Path, configured: str) -> list[str]:
    """Return the packages today's extractor analyses (grimpPackages)."""
    if configured:
        return [configured]
    return discover_packages(root) or [root.name]


def discover_packages(root: Path) -> list[str]:
    """Top-level dirs with ``__init__.py``; a ``src`` layout wins over the root."""
    for base in (root / "src", root):
        found = (
            sorted(
                d.name
                for d in base.iterdir()
                if d.is_dir() and (d / "__init__.py").is_file()
            )
            if base.is_dir()
            else []
        )
        if found:
            return found
    return []


def package_dir(root: Path, package: str) -> Path:
    """The source dir of a package: ``src/<package>`` if present, else ``<package>``."""
    src = root / "src" / package
    return src if src.is_dir() else root / package


def first_party_roots(root: Path, pkgs: list[str], paths: list[str]) -> set[str]:
    """Top-level names an import must start with to be first-party."""
    roots = {p for p in pkgs + discover_packages(root) if p.isidentifier()}
    for pattern in paths:
        if "/" not in pattern:
            head = pattern.split(".", 1)[0]
            if head.isidentifier():
                roots.add(head)
    return roots


def source_files(root: Path, package: str) -> dict[str, str]:
    """Map each module of a package to its root-relative file, as grimp walks it.

    Directories with non-identifier names are skipped. A directory without
    ``__init__.py`` is still walked: its files are namespace-package modules.
    """
    base = package_dir(root, package)
    modules: dict[str, str] = {}
    for dirpath, dirnames, filenames in os.walk(base):
        dirnames[:] = sorted(d for d in dirnames if d.isidentifier())
        for name in sorted(filenames):
            if not name.endswith(".py") or name.startswith(".") or name.count(".") != 1:
                continue
            path = Path(dirpath) / name
            rel = path.relative_to(base).with_suffix("")
            parts = list(rel.parts)
            if parts[-1] == "__init__":
                parts.pop()
            modules[".".join([package, *parts])] = path.relative_to(root).as_posix()
    return modules


def parse(root: Path, files: list[str]) -> dict[str, list[Import]]:
    """Read the imports of each file with ast-grep, without ``TYPE_CHECKING`` ones."""
    by_file: dict[str, dict[str, list]] = {}
    for m in common.ast_grep("python.yml", root, files):
        by_file.setdefault(m["file"], {}).setdefault(m["ruleId"], []).append(m)
    out: dict[str, list[Import]] = {}
    for file, rules in by_file.items():
        skip = [_span(b) for b in rules.get("type-checking", [])]
        names = rules.get("name", [])
        modules = {_span(m): m["text"] for m in rules.get("from-module", [])}
        imports: list[Import] = []
        for stmt in rules.get("import-stmt", []):
            if _inside_any(_span(stmt), skip):
                continue
            imports.extend(
                Import(n["text"]) for n in names if _inside(_span(n), _span(stmt))
            )
        for stmt in rules.get("from-stmt", []):
            span = _span(stmt)
            if _inside_any(span, skip):
                continue
            module = next(text for s, text in modules.items() if _inside(s, span))
            listed = tuple(
                n["text"]
                for n in names
                if _inside(_span(n), span) and _span(n) not in modules
            )
            imports.append(Import(module, listed, is_from=True))
        out[file] = imports
    return out


def resolve(
    importer: str, is_package: bool, imp: Import, modules: set[str]
) -> list[str]:
    """Return the dotted modules one import uses, as grimp records them.

    ``from m import n`` uses ``m.n`` when that is a module, else ``m``. A target
    may be absent from ``modules``; the caller reports it as unknown. Raises
    ImportError when a relative import climbs above its top-level package.
    """
    target = imp.module
    if imp.is_from and target.startswith("."):
        parent = importer if is_package else importer.rpartition(".")[0]
        target = importlib.util.resolve_name(target, parent)
    if not imp.is_from:
        return [target]
    if not imp.names:
        return [target]
    return [
        f"{target}.{n}" if f"{target}.{n}" in modules else target for n in imp.names
    ]


def extract(root: Path, cfg: dict) -> dict:
    """Extract first-party module edges for the Python packages under root."""
    exclusions = cfg.get("Exclusions") or []
    pkgs = packages(root, cfg.get("PyPackage") or "")
    roots = first_party_roots(root, pkgs, cfg.get("Paths") or [])
    files: dict[str, str] = {}
    for pkg in pkgs:
        if package_dir(root, pkg).is_dir():
            files.update(source_files(root, pkg))
    modules = set(files)
    importers = {
        mod: f for mod, f in files.items() if not _excluded_source(exclusions, f)
    }
    parsed = parse(root, sorted(importers.values()))
    stdlib = set(sys.stdlib_module_names)
    edges, unknown, external = set(), set(), 0
    for mod, file in importers.items():
        is_package = file.endswith("/__init__.py")
        for imp in parsed.get(file, []):
            try:
                targets = resolve(mod, is_package, imp, modules)
            except (ImportError, ValueError):
                unknown.add((mod, imp.module))
                continue
            for target in targets:
                top = target.split(".", 1)[0]
                if top not in roots:
                    external += top not in stdlib
                elif target in modules:
                    edges.add((mod, target))
                else:
                    unknown.add((mod, target))
    return common.facts("python", importers, edges, unknown, external)


def _excluded_source(exclusions: list[str], file: str) -> bool:
    """Today's rule: a file is excluded when it or any parent dir matches."""
    path = file
    while path not in ("", "."):
        if common.excluded(exclusions, path):
            return True
        path = os.path.dirname(path)
    return False


def _span(match: dict) -> tuple[int, int]:
    offsets = match["range"]["byteOffset"]
    return offsets["start"], offsets["end"]


def _inside(inner: tuple[int, int], outer: tuple[int, int]) -> bool:
    return outer[0] <= inner[0] and inner[1] <= outer[1]


def _inside_any(inner: tuple[int, int], outers: list[tuple[int, int]]) -> bool:
    return any(_inside(inner, o) for o in outers)
