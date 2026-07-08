# /// script
# requires-python = ">=3.11"
# dependencies = ["grimp>=3"]
# ///
"""grimp_helper.py — emit the internal import graph as JSON.

Usage (uv-managed project):
  uv run --with grimp --directory <project_root> grimp_helper.py --packages <pkg1> [<pkg2> ...]

Usage (direct Python, grimp must be installed):
  python3.12 grimp_helper.py --packages <pkg1> [<pkg2> ...] --root <project_root>

Output JSON: {"edges": [{"importer": "...", "imported": "...", "line": N, "line_contents": "..."}], "unresolved": N, "unresolved_imports": [...]}

SHARED-VENV CONSTRAINT: All package names passed via --packages must be importable
from a single Python environment. In a monorepo where each service has its own
virtualenv (e.g. ~42 services, each isolated), cross-service Python coupling cannot
be measured in one grimp run — each service would need its own invocation. This is
a grimp limitation; archfit does not promise cross-service analysis in that setup.
"""

import argparse
import ast
import importlib.util
import json
import os
import sys


def _ensure_importable(root: str, package: str) -> None:
    """Add the correct directory to sys.path if package is not yet importable.

    Handles both flat layout (root/package/) and src layout (root/src/package/).
    When running via uv run --directory, the project venv already has the package
    importable, so this is a no-op in that case.
    """
    if importlib.util.find_spec(package) is not None:
        return  # already importable — venv or existing sys.path covers it
    flat = os.path.join(root, package)
    src = os.path.join(root, "src")
    src_pkg = os.path.join(src, package)
    if os.path.isdir(src_pkg):
        if src not in sys.path:
            sys.path.insert(0, src)
    elif os.path.isdir(flat):
        if root not in sys.path:
            sys.path.insert(0, root)


def _package_dir(root: str, package: str) -> str:
    src_pkg = os.path.join(root, "src", package)
    if os.path.isdir(src_pkg):
        return src_pkg
    return os.path.join(root, package)


def _module_name(package: str, package_dir: str, path: str) -> str:
    rel = os.path.relpath(path, package_dir)
    mod = rel[:-3].replace(os.sep, ".")
    if mod == "__init__":
        return package
    if mod.endswith(".__init__"):
        mod = mod[: -len(".__init__")]
    return f"{package}.{mod}"


def _is_type_checking_test(expr: ast.AST) -> bool:
    return (isinstance(expr, ast.Name) and expr.id == "TYPE_CHECKING") or (
        isinstance(expr, ast.Attribute) and expr.attr == "TYPE_CHECKING"
    )


class _ImportCollector(ast.NodeVisitor):
    def __init__(self) -> None:
        self.imports: list[tuple[str, int]] = []
        self._type_checking_depth = 0

    def visit_If(self, node: ast.If) -> None:  # noqa: N802
        if _is_type_checking_test(node.test):
            self._type_checking_depth += 1
            for stmt in node.body:
                self.visit(stmt)
            self._type_checking_depth -= 1
            for stmt in node.orelse:
                self.visit(stmt)
            return
        self.generic_visit(node)

    def visit_Import(self, node: ast.Import) -> None:  # noqa: N802
        if self._type_checking_depth > 0:
            return
        for alias in node.names:
            if alias.name:
                self.imports.append((alias.name, node.lineno))

    def visit_ImportFrom(self, node: ast.ImportFrom) -> None:  # noqa: N802
        if self._type_checking_depth > 0 or node.level > 0 or not node.module:
            return
        self.imports.append((node.module, node.lineno))


def _find_spec(name: str) -> importlib.machinery.ModuleSpec | None:
    try:
        return importlib.util.find_spec(name)
    except Exception:  # noqa: BLE001
        return None


def _scan_unresolved_imports(
    root: str, packages: list[str], first_party_packages: list[str]
) -> list[dict[str, object]]:
    first_party_roots = set(first_party_packages)
    stdlib_roots = set(getattr(sys, "stdlib_module_names", ()))
    unresolved = []
    seen = set()

    for package in packages:
        package_dir = _package_dir(root, package)
        if not os.path.isdir(package_dir):
            continue
        for dirpath, _, filenames in os.walk(package_dir):
            for filename in filenames:
                if not filename.endswith(".py"):
                    continue
                path = os.path.join(dirpath, filename)
                try:
                    with open(path, encoding="utf-8") as fh:
                        source = fh.read()
                except OSError:
                    continue
                try:
                    tree = ast.parse(source, filename=path)
                except SyntaxError:
                    continue

                importer = _module_name(package, package_dir, path)
                collector = _ImportCollector()
                collector.visit(tree)
                lines = source.splitlines()
                for imported, lineno in collector.imports:
                    root_name = imported.split(".", 1)[0]
                    if root_name in first_party_roots or root_name in stdlib_roots:
                        continue
                    if _find_spec(root_name) is not None:
                        continue
                    key = (importer, imported, lineno)
                    if key in seen:
                        continue
                    seen.add(key)
                    line_contents = lines[lineno - 1].strip() if 0 < lineno <= len(lines) else ""
                    unresolved.append(
                        {
                            "importer": importer,
                            "imported": imported,
                            "line": lineno,
                            "line_contents": line_contents,
                        }
                    )

    unresolved.sort(key=lambda item: (item["importer"], item["line"], item["imported"]))
    return unresolved


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--packages", nargs="+", required=True)
    parser.add_argument("--first-party-packages", nargs="*", default=None)
    parser.add_argument("--root", default=".")
    args = parser.parse_args()

    root = os.path.abspath(args.root)
    first_party_packages = args.first_party_packages or args.packages
    for pkg in args.packages:
        _ensure_importable(root, pkg)

    try:
        import grimp  # type: ignore[import]
    except ImportError:
        print(
            json.dumps({"error": "grimp not installed", "edges": [], "unresolved": 0})
        )
        sys.exit(1)

    try:
        g = grimp.build_graph(*args.packages, exclude_type_checking_imports=True)
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"error": str(exc), "edges": [], "unresolved": 0}))
        sys.exit(1)

    edges = []
    unresolved = 0
    for importer in g.modules:
        for imported in g.find_modules_directly_imported_by(importer):
            try:
                # grimp 3.x: get_import_details requires both importer and imported;
                # returns a list of dicts with keys: importer, imported, line_number, line_contents
                details = g.get_import_details(importer=importer, imported=imported)
                for d in details:
                    edges.append(
                        {
                            "importer": importer,
                            "imported": imported,
                            "line": d["line_number"],
                            # line_contents lets the Go consumer detect symbol-level private
                            # imports ("from x import _sym" → intrusive strength hint).
                            # Use .get so older grimp versions that omit this key degrade
                            # to abstain rather than dropping the edge as unresolved.
                            "line_contents": d.get("line_contents", ""),
                        }
                    )
            except Exception:  # noqa: BLE001
                unresolved += 1

    unresolved_imports = _scan_unresolved_imports(
        root, args.packages, first_party_packages
    )
    unresolved = max(unresolved, len(unresolved_imports))
    print(
        json.dumps(
            {
                "edges": edges,
                "unresolved": unresolved,
                "unresolved_imports": unresolved_imports,
            }
        )
    )


if __name__ == "__main__":
    main()
