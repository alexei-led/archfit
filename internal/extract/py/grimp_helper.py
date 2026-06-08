# /// script
# requires-python = ">=3.12"
# dependencies = ["grimp"]
# ///
"""grimp_helper.py — emit the internal import graph as JSON.

Usage:
  uv run grimp_helper.py --package <top_pkg> [--root <project_root>]
  python3.12 grimp_helper.py --package <top_pkg> [--root <project_root>]

Output JSON: {"edges": [{"importer": "...", "imported": "...", "line": N}], "unresolved": N}
"""

import argparse
import json
import sys

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", required=True)
    parser.add_argument("--root", default=".")
    args = parser.parse_args()

    if args.root not in sys.path:
        sys.path.insert(0, args.root)

    try:
        import grimp  # type: ignore[import]
    except ImportError:
        print(json.dumps({"error": "grimp not installed", "edges": [], "unresolved": 0}))
        sys.exit(1)

    try:
        g = grimp.build_graph(args.package, exclude_type_checking_imports=True)
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"error": str(exc), "edges": [], "unresolved": 0}))
        sys.exit(1)

    edges = []
    unresolved = 0
    for module in g.modules:
        try:
            details = g.get_import_details(imported=module)
        except Exception:  # noqa: BLE001
            unresolved += 1
            continue
        for detail in details:
            edges.append({
                "importer": str(detail.importer),
                "imported": str(detail.imported),
                "line": detail.line_number,
            })

    print(json.dumps({"edges": edges, "unresolved": unresolved}))

if __name__ == "__main__":
    main()
