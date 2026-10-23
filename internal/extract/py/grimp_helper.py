# /// script
# requires-python = ">=3.14"
# dependencies = ["grimp==3.17"]
# ///
"""Collect static imports without installing or importing target dependencies.

All selected packages must have regular source in root/package or root/src/package.
External and absent first-party targets are literals, not verified runtime modules.
The helper interpreter must understand the target's syntax; the image supplies 3.14.
"""

import argparse
import ast
import importlib.abc
import importlib.machinery
import importlib.metadata
import importlib.util
import json
import os
import stat
import sys
import tokenize
from dataclasses import dataclass, field
from types import ModuleType


def _package_dir(root: str, package: str) -> str:
    src_pkg = os.path.join(root, "src", package)
    return src_pkg if os.path.isdir(src_pkg) else os.path.join(root, package)


class _NoExecutionLoader(importlib.abc.Loader):
    def exec_module(self, module: ModuleType) -> None:
        raise ImportError(f"target package execution is disabled: {module.__name__}")


class _StaticPackageFinder(importlib.abc.MetaPathFinder):
    def __init__(self, specs: dict[str, importlib.machinery.ModuleSpec]) -> None:
        self.specs = specs

    def find_spec(self, fullname: str, path=None, target=None):
        return self.specs.get(fullname)


def _ensure_importable(root: str, package: str) -> importlib.machinery.ModuleSpec:
    """Bind grimp to repository source, never an installed same-named package."""
    if not package.isidentifier():
        raise ValueError(f"expected a top-level Python package, got {package!r}")
    package_dir = _package_dir(root, package)
    if os.path.islink(package_dir):
        raise ValueError(f"symlinked package root {package!r} is unsupported")
    initializer = os.path.realpath(os.path.join(package_dir, "__init__.py"))
    if os.path.commonpath((root, initializer)) != root or not os.path.isfile(
        initializer
    ):
        raise ValueError(
            f"package {package!r} has no regular initializer inside the analysis root"
        )
    if package in sys.modules:
        raise ValueError(f"cannot bind package {package!r} to its repository source")
    # Expose package metadata to grimp, not source on the general import path:
    # a lazy analyzer import must never execute a shadowing target module.
    spec = importlib.machinery.ModuleSpec(
        package, _NoExecutionLoader(), origin=initializer, is_package=True
    )
    spec.submodule_search_locations = [package_dir]
    return spec


def _source_files(root: str, package: str) -> list[str]:
    """Preflight before grimp can follow directory links or read escaped source."""

    def walk_error(error: OSError) -> None:
        raise error

    package_dir = _package_dir(root, package)
    if os.path.islink(package_dir):
        raise ValueError(f"symlinked package root {package!r} is unsupported")
    files = []
    for dirpath, directories, filenames in os.walk(package_dir, onerror=walk_error):
        directories[:] = sorted(name for name in directories if name.isidentifier())
        for name in directories:
            path = os.path.join(dirpath, name)
            if os.path.islink(path):
                raise ValueError(
                    f"directory symlink in Python source: {os.path.relpath(path, root)}"
                )
        for name in sorted(filenames):
            # Match grimp's file rules. Init-less identifier directories remain
            # inventoried: their unsupported source must not silently disappear.
            if not name.endswith(".py") or name.startswith(".") or name.count(".") != 1:
                continue
            path = os.path.join(dirpath, name)
            if (
                os.path.commonpath((root, os.path.realpath(path))) != root
                or os.path.islink(path)
                or not stat.S_ISREG(os.stat(path).st_mode)
            ):
                raise ValueError(
                    f"Python source is not a regular file inside the analysis root: {os.path.relpath(path, root)}"
                )
            files.append(path)
    return files


def _module_name(package: str, package_dir: str, path: str) -> str:
    mod = os.path.relpath(path, package_dir)[:-3].replace(os.sep, ".")
    return (
        package if mod == "__init__" else f"{package}.{mod.removesuffix('.__init__')}"
    )


def _is_type_checking_test(expr: ast.AST) -> bool:
    return (isinstance(expr, ast.Name) and expr.id == "TYPE_CHECKING") or (
        isinstance(expr, ast.Attribute) and expr.attr == "TYPE_CHECKING"
    )


class _ImportCollector(ast.NodeVisitor):
    def __init__(self) -> None:
        self.imports: list[tuple[str, int, int, tuple[str, ...]]] = []
        self._type_checking_depth = 0

    def visit_If(self, node: ast.If) -> None:
        if _is_type_checking_test(node.test):
            self._type_checking_depth += 1
            for stmt in node.body:
                self.visit(stmt)
            self._type_checking_depth -= 1
            for stmt in node.orelse:
                self.visit(stmt)
            return
        self.generic_visit(node)

    def visit_Import(self, node: ast.Import) -> None:
        if self._type_checking_depth == 0:
            self.imports.extend(
                (alias.name, 0, node.lineno, ()) for alias in node.names
            )

    def visit_ImportFrom(self, node: ast.ImportFrom) -> None:
        if self._type_checking_depth == 0:
            self.imports.append(
                (
                    node.module or "",
                    node.level,
                    node.lineno,
                    tuple(alias.name for alias in node.names),
                )
            )


@dataclass
class _ImportInventory:
    external: list[dict[str, object]] = field(default_factory=list)
    literals: list[dict[str, object]] = field(default_factory=list)
    gaps: list[dict[str, object]] = field(default_factory=list)
    explicit: set[tuple[str, int, str]] = field(default_factory=set)
    sources: list[dict[str, object]] = field(default_factory=list)

    def gap(
        self, importer: str, file: str, reason: str, line: int = 0, imported: str = ""
    ) -> None:
        self.gaps.append(
            {
                "importer": importer,
                "imported": imported or importer,
                "line": line,
                "file": file,
                "reason": reason,
            }
        )


def _scan_file(
    root: str,
    package: str,
    path: str,
    packages: list[str],
    first_party_roots: set[str],
    modules: set[str],
    inventory: _ImportInventory,
) -> None:
    importer = _module_name(package, _package_dir(root, package), path)
    file = os.path.relpath(path, root)
    source_status = {"file": file, "module": importer, "parsed": False}
    inventory.sources.append(source_status)
    if package not in packages:
        inventory.gap(importer, file, "first-party package not selected")
        return
    try:
        with tokenize.open(path) as fh:
            source = fh.read()
        tree = ast.parse(source, filename=path)
    except (OSError, UnicodeError, SyntaxError) as exc:
        inventory.gap(
            importer,
            file,
            f"{type(exc).__name__} under Python {sys.version_info.major}.{sys.version_info.minor}",
        )
        return
    source_status["parsed"] = True
    if importer not in modules:
        inventory.gap(importer, file, "source module not scanned by grimp")
    collector = _ImportCollector()
    collector.visit(tree)
    lines = source.splitlines()
    stdlib_roots = set(getattr(sys, "stdlib_module_names", ()))
    seen = set()
    for imported, level, lineno, names in collector.imports:
        if level:
            parent = (
                importer
                if os.path.basename(path) == "__init__.py"
                else importer.rpartition(".")[0]
            )
            try:
                imported = importlib.util.resolve_name("." * level + imported, parent)
            except (ImportError, ValueError):
                inventory.gap(
                    importer,
                    file,
                    "relative import escapes package",
                    lineno,
                    "." * level + imported,
                )
                continue
        inventory.explicit.add((importer, lineno, imported))
        for name in names:
            if imported + "." + name in modules:
                inventory.explicit.add((importer, lineno, imported + "." + name))
        root_name = imported.split(".", 1)[0]
        if root_name in first_party_roots:
            if imported not in modules:
                if root_name not in packages:
                    inventory.gap(
                        importer,
                        file,
                        "first-party package not selected",
                        lineno,
                        imported,
                    )
                else:
                    inventory.literals.append(
                        {
                            "importer": importer,
                            "imported": imported,
                            "line": lineno,
                            "literal": True,
                        }
                    )
            continue
        if root_name in stdlib_roots:
            continue
        key = (importer, imported, lineno)
        if key not in seen:
            seen.add(key)
            inventory.external.append(
                {
                    "importer": importer,
                    "imported": imported,
                    "line": lineno,
                    "line_contents": lines[lineno - 1].strip(),
                }
            )


def _scan_imports(
    root: str,
    packages: list[str],
    first_party_packages: list[str],
    modules: set[str],
    source_files: dict[str, list[str]],
) -> _ImportInventory:
    inventory = _ImportInventory()
    for package, paths in source_files.items():
        for path in paths:
            _scan_file(
                root,
                package,
                path,
                packages,
                set(first_party_packages),
                modules,
                inventory,
            )
    for entries in (inventory.gaps, inventory.external, inventory.literals):
        entries.sort(
            key=lambda item: (item["importer"], item["line"], item["imported"])
        )
    return inventory


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--packages", nargs="+", required=True)
    parser.add_argument("--first-party-packages", nargs="*", default=None)
    parser.add_argument("--root", default=".")
    args = parser.parse_args()
    root = os.path.realpath(args.root)
    first_party_packages = args.first_party_packages or args.packages
    try:
        # Isolated startup loads the analyzer before exposing target source.
        import grimp  # type: ignore[import]

        grimp_version = importlib.metadata.version("grimp")
        if grimp_version != "3.17":
            raise ValueError(f"unsupported grimp {grimp_version}; install grimp 3.17")
        producer_version = f"grimp {grimp_version}; python {sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}"
        specs = {pkg: _ensure_importable(root, pkg) for pkg in args.packages}
        source_files = {
            pkg: _source_files(root, pkg)
            for pkg in sorted(set(first_party_packages) | set(args.packages))
            if os.path.isdir(_package_dir(root, pkg))
        }
        finder = _StaticPackageFinder(specs)
        sys.meta_path.insert(0, finder)
        try:
            g = grimp.build_graph(*args.packages, exclude_type_checking_imports=True)
        finally:
            sys.meta_path.remove(finder)
    except Exception as exc:  # noqa: BLE001 - producer failures become incomplete evidence
        print(json.dumps({"error": str(exc), "edges": [], "unresolved": 0}))
        sys.exit(1)

    edges, unresolved = [], 0
    for importer in sorted(g.modules):
        for imported in sorted(g.find_modules_directly_imported_by(importer)):
            try:
                for detail in g.get_import_details(
                    importer=importer, imported=imported
                ):
                    edges.append(
                        {
                            "importer": importer,
                            "imported": imported,
                            "line": detail["line_number"],
                            "line_contents": detail.get("line_contents", ""),
                        }
                    )
            except Exception:  # noqa: BLE001 - retain failures with no exact source location
                unresolved += 1

    inventory = _scan_imports(
        root, args.packages, first_party_packages, set(g.modules), source_files
    )
    # Replace grimp's missing-target parent fallback, retaining independent
    # imports of that parent (including from-import aliases on the same line).
    fallbacks = set()
    for literal in inventory.literals:
        parent = literal["imported"].rpartition(".")[0]
        while parent:
            key = (literal["importer"], literal["line"], parent)
            if key not in inventory.explicit:
                fallbacks.add(key)
            parent = parent.rpartition(".")[0]
    edges = [
        edge
        for edge in edges
        if (edge["importer"], edge["line"], edge["imported"]) not in fallbacks
    ]
    edges.extend(inventory.literals)
    print(
        json.dumps(
            {
                "edges": edges,
                "modules": sorted(g.modules),
                "sources": inventory.sources,
                "unresolved": unresolved + len(inventory.gaps),
                "unresolved_imports": inventory.gaps,
                "external_imports": inventory.external,
                "producer_version": producer_version,
            }
        )
    )


if __name__ == "__main__":
    main()
