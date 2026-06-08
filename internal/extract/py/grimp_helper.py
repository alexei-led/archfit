# /// script
# requires-python = ">=3.12"
# dependencies = ["grimp>=3"]
# ///
"""grimp_helper.py — emit the internal import graph as JSON.

Usage (uv-managed project):
  uv run --with grimp --directory <project_root> grimp_helper.py --package <top_pkg>

Usage (direct Python, grimp must be installed):
  python3.12 grimp_helper.py --package <top_pkg> --root <project_root>

Output JSON: {"edges": [{"importer": "...", "imported": "...", "line": N}], "unresolved": N}
"""

import argparse
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


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", required=True)
    parser.add_argument("--root", default=".")
    args = parser.parse_args()

    _ensure_importable(os.path.abspath(args.root), args.package)

    try:
        import grimp  # type: ignore[import]
    except ImportError:
        print(
            json.dumps({"error": "grimp not installed", "edges": [], "unresolved": 0})
        )
        sys.exit(1)

    try:
        g = grimp.build_graph(args.package, exclude_type_checking_imports=True)
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
                        }
                    )
            except Exception:  # noqa: BLE001
                unresolved += 1

    print(json.dumps({"edges": edges, "unresolved": unresolved}))


if __name__ == "__main__":
    main()
