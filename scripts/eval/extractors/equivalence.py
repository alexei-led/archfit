"""Go extractor equivalence: the typed extractor versus the parser-based one.

Archfit 4 design, sections 11.8 and 15.2. For one Go repository it
runs both sides, maps every node to its declared module with the engine's own
module map, and compares first-party edges file by file and module pair by
module pair. Run it on a disposable clone: today's extractors may write caches.

    go build -o /tmp/gofacts ./scripts/eval/extractors/gofacts
    PYTHONPATH=scripts/eval uv run python -m extractors.equivalence \\
        --lang go --root CLONE --config CLONE/.archfit.yaml \\
        --gofacts /tmp/gofacts --out OUTDIR
"""

from __future__ import annotations

import argparse
import json
import subprocess
from pathlib import Path

from . import py_imports, ts_imports

NONE = "(none)"
EXAMPLES = 3


def gofacts(
    binary: str,
    command: str,
    lang: str,
    root: Path | None,
    config: str,
    stdin: str = "",
) -> dict:
    """Run one gofacts command and decode its JSON."""
    args = [binary, command, "-lang", lang]
    if root is not None:
        args += ["-root", str(root)]
    if config:
        args += ["-config", config]
    proc = subprocess.run(
        args, input=stdin, capture_output=True, text=True, check=False
    )
    if proc.returncode != 0:
        raise RuntimeError(f"gofacts {command}: {proc.stderr.strip()}")
    return json.loads(proc.stdout)


def new_side(binary: str, lang: str, root: Path, config: str) -> dict:
    """Run the parser-based extractor for one language."""
    if lang == "go":
        return gofacts(binary, "goparse", lang, root, config)
    cfg = gofacts(binary, "config", lang, None, config)
    extractor = {"python": py_imports, "typescript": ts_imports}[lang]
    return extractor.extract(root, cfg)


def compare(ref: dict, new: dict, module_of: dict[str, str]) -> dict:
    """Compare two sides' facts at file level and at module-pair level."""
    ref_edges = {(e["from"], e["to"]) for e in ref["edges"]}
    new_edges = {(e["from"], e["to"]) for e in new["edges"]}
    ref_pairs = _pairs(ref_edges, module_of)
    new_pairs = _pairs(new_edges, module_of)
    first_party = len(new_edges) + len(new["unknown"])
    only_new = new_edges - ref_edges
    return {
        "files": {
            "reference": len(ref_edges),
            "new": len(new_edges),
            "both": len(ref_edges & new_edges),
            "only_reference": _edge_list(ref_edges - new_edges),
            "only_new": _edge_list(only_new),
            "only_new_reference_unresolved": _reference_unresolved(ref, new, only_new),
        },
        "sources": {
            "reference": len(ref["sources"]),
            "new": len(new["sources"]),
            "only_reference": sorted(set(ref["sources"]) - set(new["sources"])),
            "only_new": sorted(set(new["sources"]) - set(ref["sources"])),
        },
        "modules": {
            "mapped": any(module_of.values()),
            "reference": len(ref_pairs),
            "new": len(new_pairs),
            "both": len(ref_pairs.keys() & new_pairs.keys()),
            "only_reference": _pair_list(ref_pairs, new_pairs),
            "only_new": _pair_list(new_pairs, ref_pairs),
        },
        "reference_coverage": ref.get("coverage") or {},
        "unknown": {
            "reference": len(ref["unknown"]),
            "new": len(new["unknown"]),
            "new_share": round(len(new["unknown"]) / first_party, 4)
            if first_party
            else 0.0,
            "new_list": new["unknown"],
            "reference_list": ref["unknown"],
        },
    }


def _reference_unresolved(ref: dict, new: dict, only_new: set) -> int:
    """Count new-only edges whose specifier the reference left unresolved in the same file.

    Needs the new side's ``resolutions`` (file, specifier, target); 0 without them.
    """
    unresolved = {(u["from"], u["specifier"]) for u in ref["unknown"]}
    specifiers: dict[tuple[str, str], set[str]] = {}
    for r in new.get("resolutions", []):
        specifiers.setdefault((r["from"], r["to"]), set()).add(r["specifier"])
    return sum(
        any((src, spec) in unresolved for spec in specifiers.get((src, dst), ()))
        for src, dst in only_new
    )


def nodes(*sides: dict) -> list[str]:
    """Every node path the sides' edges name."""
    return sorted(
        {p for side in sides for e in side["edges"] for p in (e["from"], e["to"])}
    )


def _pairs(
    edges: set[tuple[str, str]], module_of: dict[str, str]
) -> dict[tuple[str, str], list]:
    pairs: dict[tuple[str, str], list] = {}
    for src, dst in sorted(edges):
        pair = (module_of.get(src) or NONE, module_of.get(dst) or NONE)
        if pair[0] != pair[1]:
            pairs.setdefault(pair, []).append([src, dst])
    return pairs


def _pair_list(side: dict, other: dict) -> list[dict]:
    return [
        {"pair": list(p), "edges": side[p][:EXAMPLES], "count": len(side[p])}
        for p in sorted(side.keys() - other.keys())
    ]


def _edge_list(edges: set[tuple[str, str]]) -> list[list[str]]:
    return [list(e) for e in sorted(edges)]


def summary(name: str, lang: str, result: dict) -> str:
    """One Markdown table row for the findings report.

    The last cell is the reference's own coverage status: an absent or partial
    reference makes every other cell a partial comparison.
    """
    f, m, u = result["files"], result["modules"], result["unknown"]
    status = result["reference_coverage"].get("status", "unknown")
    pairs = (
        f"{m['reference']} / {m['new']} / {m['both']} / {len(m['only_reference'])} / {len(m['only_new'])}"
        if m["mapped"]
        else "no mapping"
    )
    return (
        f"| {name} | {lang} | {f['reference']} / {f['new']} / {f['both']} / {len(f['only_reference'])} / {len(f['only_new'])} "
        f"| {pairs} | {u['reference']} / {u['new']} ({u['new_share']:.1%}) | {status} |"
    )


def main() -> None:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("--lang", required=True, choices=["go"])
    parser.add_argument(
        "--root", required=True, type=Path, help="a disposable clone of the repository"
    )
    parser.add_argument(
        "--config",
        default="",
        help="the repository's .archfit.yaml; omit for no module map",
    )
    parser.add_argument(
        "--gofacts", required=True, help="path to the built gofacts binary"
    )
    parser.add_argument(
        "--out", required=True, type=Path, help="directory for the JSON results"
    )
    parser.add_argument(
        "--name",
        default="",
        help="repository name in the output (default: root dir name)",
    )
    args = parser.parse_args()
    root = args.root.resolve()
    name = args.name or root.name
    args.out.mkdir(parents=True, exist_ok=True)
    ref = gofacts(args.gofacts, "reference", args.lang, root, args.config)
    new = new_side(args.gofacts, args.lang, root, args.config)
    module_of = gofacts(
        args.gofacts,
        "modules",
        args.lang,
        None,
        args.config,
        json.dumps(nodes(ref, new)),
    )
    result = compare(ref, new, module_of)
    for side, data in (("reference", ref), ("new", new), ("compare", result)):
        (args.out / f"{name}-{args.lang}-{side}.json").write_text(
            json.dumps(data, indent=1) + "\n"
        )
    print(summary(name, args.lang, result))


if __name__ == "__main__":
    main()
