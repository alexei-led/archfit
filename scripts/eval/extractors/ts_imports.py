"""Parser-based TypeScript and JavaScript import extractor: ast-grep plus a resolver.

A measurement prototype for the Archfit 4 design, section 11.8. It reads no
``node_modules`` and runs no Node or TypeScript. It resolves, in this order:
relative paths with extension and index probing, ``tsconfig`` ``paths``, and
workspace packages through their ``package.json`` ``exports`` or entry fields.
Any other bare specifier is a third-party package. A relative path, alias, or
workspace subpath that names no file is ``unknown``. Nodes are file paths.
"""

from __future__ import annotations

import json
import os
import re
from functools import cache
from pathlib import Path, PurePosixPath

from . import common

# Files whose imports are read, as dependency-cruiser parses them.
PARSED = (".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs")
# Extensions a specifier without one may omit, in resolution order.
PROBE = (
    ".js",
    ".cjs",
    ".mjs",
    ".jsx",
    ".ts",
    ".cts",
    ".mts",
    ".tsx",
    ".d.ts",
    ".json",
    ".vue",
    ".svelte",
)
# A ".js" specifier may name a TypeScript file (TypeScript's ESM convention),
# and a ".ts" one its declaration file.
JS_TO_TS = {
    ".js": (".ts", ".tsx", ".d.ts"),
    ".ts": (".d.ts",),
    ".jsx": (".tsx",),
    ".mjs": (".mts",),
    ".cjs": (".cts",),
}
# Conditions every lookup accepts; a tsconfig adds its customConditions.
CONDITIONS = ("types", "import", "require", "node", "default")
SKIP_DIRS = {"node_modules", ".git"}


def source_files(root: Path) -> list[str]:
    """Root-relative files to parse: every PARSED file outside node_modules."""
    out = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS)
        rel = Path(dirpath).relative_to(root)
        out.extend(
            (rel / n).as_posix() for n in sorted(filenames) if n.endswith(PARSED)
        )
    return out


_REFERENCE_PATH = re.compile(r"""\bpath\s*=\s*["']([^"']+)["']""")


def parse(root: Path, files: list[str]) -> dict[str, list[str]]:
    """Return the import specifiers of each file.

    A triple-slash reference path is relative to its file, as a specifier
    starting with "./" is.
    """
    out: dict[str, list[str]] = {}
    for m in common.ast_grep("typescript.yml", root, files):
        if m["ruleId"].endswith("-reference"):
            found = _REFERENCE_PATH.search(m["text"])
            if not found:
                continue
            path = found.group(1)
            spec = path if path.startswith((".", "/")) else "./" + path
        else:
            spec = m["text"][1:-1]
        out.setdefault(m["file"], []).append(spec)
    return out


class Resolver:
    """Resolve specifiers to root-relative files without node_modules."""

    def __init__(self, root: Path):
        self.root = root
        self.packages = workspace_packages(root)

    def resolve(self, importer: str, spec: str) -> tuple[str, str]:
        """Return ("file", path), ("unknown", spec) or ("external", spec)."""
        spec = spec.split("?", 1)[0]
        if spec.startswith((".", "/")):
            base = (PurePosixPath(importer).parent / spec).as_posix()
            found = self.file(base)
            return ("file", found) if found else ("unknown", spec)
        if spec.startswith("node:"):
            return ("external", spec)
        config = self.tsconfig(PurePosixPath(importer).parent.as_posix())
        if spec.startswith("#"):
            found = self.package_import(importer, spec, config["conditions"])
            return ("file", found) if found else ("unknown", spec)
        pattern, targets = alias_targets(config, spec)
        for target in targets:
            found = self.file(target)
            if found:
                return ("file", found)
        if config["base_url"] is not None:
            found = self.file(f"{config['base_url']}/{spec}")
            if found:
                return ("file", found)
        package = self.package_of(spec)
        if package is not None:
            name, pkg_dir = package
            found = self.package_entry(pkg_dir, "." + spec[len(name) :], config)
            return ("file", found) if found else ("unknown", spec)
        # A configured alias names first-party code; a bare "*" also matches
        # every third-party package, so it alone proves nothing.
        if pattern not in (None, "*"):
            return ("unknown", spec)
        return ("external", spec)

    def file(self, base: str, seen: frozenset[str] = frozenset()) -> str | None:
        """Probe a root-relative base path as a file, with extensions, then as a dir."""
        base = os.path.normpath(base).replace(os.sep, "/")
        if base.startswith("../") or base in seen:
            return None
        path = self.root / base
        if path.is_file():
            return base
        stem, ext = os.path.splitext(base)
        for alt in JS_TO_TS.get(ext, ()):
            if (self.root / (stem + alt)).is_file():
                return stem + alt
        for ext in PROBE:
            if (self.root / (base + ext)).is_file():
                return base + ext
        if path.is_dir():
            manifest = read_json(path / "package.json")
            for field in ("types", "module", "main"):
                entry = manifest.get(field)
                if isinstance(entry, str):
                    found = self.file(f"{base}/{entry}", seen | {base})
                    if found:
                        return found
            for ext in PROBE:
                if (path / ("index" + ext)).is_file():
                    return f"{base}/index{ext}"
        return None

    def package_of(self, spec: str) -> tuple[str, str] | None:
        """The workspace package a bare specifier names, as (name, dir)."""
        for name in sorted(self.packages, key=len, reverse=True):
            if spec == name or spec.startswith(name + "/"):
                return name, self.packages[name]
        return None

    def package_entry(self, pkg_dir: str, subpath: str, config: dict) -> str | None:
        """Resolve "." or "./sub" inside a workspace package.

        ``exports`` decides when present, except under ``node`` (node10)
        module resolution, which ignores it, as TypeScript does.
        """
        manifest = read_json(self.root / pkg_dir / "package.json")
        exports = manifest.get("exports")
        if exports is not None and config["exports"]:
            for target in export_targets(exports, subpath, config["conditions"]):
                found = self.file(f"{pkg_dir}/{target}")
                if found:
                    return found
            return None
        if subpath == ".":
            return self.file(pkg_dir)
        return self.file(f"{pkg_dir}/{subpath[2:]}")

    def package_import(
        self, importer: str, spec: str, conditions: tuple[str, ...]
    ) -> str | None:
        """Resolve a "#name" subpath import through the nearest package.json ``imports``."""
        current = PurePosixPath(importer).parent
        while True:
            manifest = self.root / current / "package.json"
            if manifest.is_file():
                imports = read_json(manifest).get("imports") or {}
                for target in export_targets(imports, spec, conditions):
                    found = self.file(f"{current.as_posix()}/{target}")
                    if found:
                        return found
                return None
            if current.as_posix() in ("", "."):
                return None
            current = current.parent

    @cache  # noqa: B019 - one resolver per run
    def tsconfig(self, rel_dir: str) -> dict:
        """The settings of the nearest tsconfig.json at or above rel_dir."""
        current = PurePosixPath(rel_dir)
        while True:
            candidate = self.root / current / "tsconfig.json"
            if candidate.is_file():
                return load_tsconfig(self.root, candidate)
            if current.as_posix() in ("", "."):
                return _settings({})
            current = current.parent


def alias_targets(config: dict, spec: str) -> tuple[str | None, list[str]]:
    """The tsconfig ``paths`` pattern a spec matches and its root-relative candidates.

    An exact pattern wins; otherwise the wildcard with the longest prefix.
    """
    best, star = None, ""
    for pattern in config["paths"]:
        prefix, _, suffix = pattern.partition("*")
        if "*" not in pattern:
            if spec == pattern:
                best, star = pattern, ""
                break
        elif (
            spec.startswith(prefix)
            and spec.endswith(suffix)
            and len(spec) >= len(prefix) + len(suffix)
        ):
            if best is None or len(prefix) > len(best.partition("*")[0]):
                best, star = pattern, spec[len(prefix) : len(spec) - len(suffix)]
    if best is None:
        return None, []
    base = PurePosixPath(config["paths_base"])
    return best, [(base / t.replace("*", star)).as_posix() for t in config["paths"][best]]


def export_targets(exports, subpath: str, conditions: tuple[str, ...]) -> list[str]:
    """Targets of an ``exports`` or ``imports`` map for a subpath.

    Every target whose condition is active, in key order: the caller takes the
    first that names a file, so an unbuilt ``dist`` target falls through. An
    explicit ``null`` blocks the subpath, as in Node.
    """
    if isinstance(exports, (str, list)) or (
        isinstance(exports, dict) and not any(k.startswith((".", "#")) for k in exports)
    ):
        exports = {".": exports}
    if not isinstance(exports, dict):
        return []
    entry, star = exports.get(subpath), ""
    if subpath not in exports:
        for key in sorted(
            (k for k in exports if "*" in k),
            key=lambda k: len(k.partition("*")[0]),
            reverse=True,
        ):
            prefix, _, suffix = key.partition("*")
            if subpath.startswith(prefix) and subpath.endswith(suffix):
                entry, star = (
                    exports[key],
                    subpath[len(prefix) : len(subpath) - len(suffix)],
                )
                break
    return [t.replace("*", star) for t in _condition_targets(entry, conditions)]


def _condition_targets(entry, conditions: tuple[str, ...]) -> list[str]:
    if isinstance(entry, str):
        return [entry]
    if isinstance(entry, list):
        return [t for item in entry for t in _condition_targets(item, conditions)]
    if isinstance(entry, dict):
        targets: list[str] = []
        for key, value in entry.items():
            if key in conditions:
                if value is None:
                    break
                targets.extend(_condition_targets(value, conditions))
        return targets
    return []


def workspace_packages(root: Path) -> dict[str, str]:
    """Map each workspace package name to its root-relative dir.

    Patterns come from package.json ``workspaces`` (npm, Yarn) and from
    pnpm-workspace.yaml ``packages``; a "!" pattern excludes.
    """
    workspaces = read_json(root / "package.json").get("workspaces", [])
    if isinstance(workspaces, dict):
        workspaces = workspaces.get("packages", [])
    patterns = [p.removeprefix("./") for p in [*workspaces, *pnpm_patterns(root)] if isinstance(p, str)]
    excludes = [p[1:].removeprefix("./") for p in patterns if p.startswith("!")]
    packages = {}
    for pattern in (p for p in patterns if not p.startswith("!")):
        for pkg in sorted(root.glob(pattern)):
            rel = pkg.relative_to(root).as_posix()
            name = read_json(pkg / "package.json").get("name")
            if isinstance(name, str) and "node_modules" not in pkg.parts and not common.excluded(excludes, rel):
                packages[name] = rel
    return packages


def pnpm_patterns(root: Path) -> list[str]:
    """The block-list ``packages`` entries of pnpm-workspace.yaml.

    A flow list (``packages: [a, b]``) is not read; the corpus has none.
    """
    try:
        lines = (root / "pnpm-workspace.yaml").read_text().splitlines()
    except OSError:
        return []
    patterns, inside = [], False
    for line in lines:
        text = line.split(" #", 1)[0].rstrip()
        if not text.strip() or text.lstrip().startswith("#"):
            continue
        if not line[0].isspace():
            inside = text == "packages:"
        elif inside and text.lstrip().startswith("- "):
            patterns.append(text.lstrip()[2:].strip().strip("'\""))
    return patterns


def load_tsconfig(root: Path, path: Path) -> dict:
    """The resolution settings of one tsconfig, after its ``extends`` chain."""
    return _settings(_tsconfig_options(root, path, 0))


def _settings(options: dict) -> dict:
    """Resolution settings from merged options, with TypeScript's defaults.

    ``paths`` resolve against ``baseUrl`` when any config in the chain sets it,
    else against the directory of the config that sets ``paths``.
    """
    base_url = options.get("base_url")
    return {
        "paths": options.get("paths", {}),
        "paths_base": base_url if base_url is not None else options.get("paths_dir", ""),
        "base_url": base_url,
        "conditions": options.get("conditions", CONDITIONS),
        "exports": options.get("exports", True),
    }


def _tsconfig_options(root: Path, path: Path, depth: int) -> dict:
    """The options a tsconfig sets, merged over its parents in ``extends`` order.

    Follows relative ``extends`` only; a package ``extends`` would need
    node_modules and is skipped.
    """
    data = read_jsonc(path)
    merged: dict = {}
    extends = data.get("extends")
    for parent in [extends] if isinstance(extends, str) else extends or []:
        if isinstance(parent, str) and parent.startswith(".") and depth < 10:
            target = (path.parent / parent).resolve()
            if target.is_dir():
                target = target / "tsconfig.json"
            if target.suffix != ".json":
                target = target.with_name(target.name + ".json")
            if target.is_file():
                merged.update(_tsconfig_options(root, target, depth + 1))
    options = data.get("compilerOptions") or {}
    here = path.parent.resolve().relative_to(root.resolve()).as_posix()
    if "baseUrl" in options:
        merged["base_url"] = os.path.normpath(os.path.join(here, options["baseUrl"])).replace(os.sep, "/")
    if "paths" in options:
        merged["paths"] = options["paths"]
        merged["paths_dir"] = here
    if "customConditions" in options:
        merged["conditions"] = CONDITIONS + tuple(options["customConditions"])
    if "moduleResolution" in options:
        merged["exports"] = str(options["moduleResolution"]).lower() not in ("node", "node10")
    return merged


_JSONC = re.compile(r'"(?:\\.|[^"\\])*"|//[^\n]*|/\*.*?\*/', re.DOTALL)
_TRAILING_COMMA = re.compile(r",(\s*[}\]])")


def read_jsonc(path: Path) -> dict:
    """Parse JSON with comments and trailing commas, as tsconfig allows."""
    try:
        text = path.read_text()
    except OSError:
        return {}
    text = _JSONC.sub(lambda m: m.group(0) if m.group(0).startswith('"') else "", text)
    try:
        return json.loads(_TRAILING_COMMA.sub(r"\1", text))
    except json.JSONDecodeError:
        return {}


def read_json(path: Path) -> dict:
    try:
        data = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError):
        return {}
    return data if isinstance(data, dict) else {}


def extract(root: Path, cfg: dict) -> dict:
    """Extract first-party file edges for the TypeScript and JavaScript under root."""
    del cfg  # Like today's extractor, the walk ignores archfit exclusions.
    files = source_files(root)
    parsed = parse(root, files)
    resolver = Resolver(root)
    edges, unknown, resolutions, external = set(), set(), set(), 0
    for file in files:
        for spec in parsed.get(file, []):
            kind, target = resolver.resolve(file, spec)
            if kind == "file":
                edges.add((file, target))
                resolutions.add((file, spec, target))
            elif kind == "unknown":
                unknown.add((file, spec))
            else:
                external += 1
    out = common.facts("typescript", files, edges, unknown, external)
    out["resolutions"] = [
        {"from": f, "specifier": s, "to": t} for f, s, t in sorted(resolutions)
    ]
    return out
