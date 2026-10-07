#!/usr/bin/env python3
"""Count coupling seams per corpus repository, for before/after score comparisons.

`corpus_sweep.py` proves the output contract. This script answers a different
question: how many seams does one scorer version flag, and are they the right
ones. Run it with the old and the new binary over the same checkouts and diff
the two tables.

    seam_census.py --archfit .bin/archfit \
        --repo archfit=. --repo pumba=/tmp/corpus/pumba

Each --repo is LABEL=DIR and the DIR holds .archfit.yaml. Run it on disposable
copies: analyze writes .archfit-cache next to the config. Standard library only.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from collections import Counter
from pathlib import Path
from typing import Any

SEVERITIES = ("critical", "high", "medium", "low", "none")
PINNED_RUST_TOOLCHAIN = "1.98.0"


def metric(dim: dict[str, Any], name: str) -> int:
    for m in dim.get("metrics", []):
        if m.get("name") == name:
            return int(m.get("value", 0))
    return 0


def summarize(state: dict[str, Any]) -> dict[str, Any]:
    """Reduce one architecture-state document to the census numbers."""
    seams = state.get("seams") or []
    coupling = state.get("dimensions", {}).get("coupling", {})
    return {
        "coupling_status": coupling.get("status", "missing"),
        "seams": len(seams),
        "by_severity": {
            s: sum(1 for x in seams if x.get("severity") == s) for s in SEVERITIES
        },
        "qualifying": sum(1 for x in seams if x.get("distributed_monolith")),
        "unrated": sum(1 for x in seams if x.get("confidence") == "unrated"),
        "by_strength": dict(
            sorted(Counter(x.get("strength", "") for x in seams).items())
        ),
        "by_distance": dict(
            sorted(Counter(x.get("distance", "") for x in seams).items())
        ),
        "by_hypothesis": dict(
            sorted(Counter(x.get("hypothesis", "") for x in seams).items())
        ),
        "scored_edges": metric(coupling, "scored_edges"),
        "abstained_edges": metric(coupling, "abstained_edges"),
        "critical_band_edges": metric(coupling, "critical_band_edges"),
    }


def qualifying_pairs(state: dict[str, Any]) -> list[str]:
    """Qualifying seams as 'from -> to', for the by-hand review."""
    return sorted(
        f"{s['from_module']} -> {s['to_module']}"
        for s in state.get("seams") or []
        if s.get("distributed_monolith")
    )


def abstention_rate(row: dict[str, Any]) -> float:
    total = row["scored_edges"] + row["abstained_edges"]
    return row["abstained_edges"] / total if total else 0.0


def run_analyze(archfit: str, label: str, directory: str) -> dict[str, Any]:
    env = dict(os.environ)
    env.setdefault("RUSTUP_TOOLCHAIN", PINNED_RUST_TOOLCHAIN)
    proc = subprocess.run(
        [archfit, "analyze", "--config", ".archfit.yaml", "--format", "json"],
        cwd=directory,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(
            f"{label}: analyze exit {proc.returncode}: {proc.stderr.strip()[-400:]}"
        )
    return json.loads(proc.stdout)


def render_markdown(rows: dict[str, dict[str, Any]]) -> str:
    head = "| repo | coupling | seams | critical | high | qualifying | unrated | scored | abstained | abstain % |"
    lines = [head, "| " + " | ".join(["---"] * 10) + " |"]
    for label, r in rows.items():
        sev = r["by_severity"]
        lines.append(
            f"| {label} | {r['coupling_status']} | {r['seams']} | {sev['critical']} | {sev['high']} "
            f"| {r['qualifying']} | {r['unrated']} | {r['scored_edges']} | {r['abstained_edges']} "
            f"| {abstention_rate(r) * 100:.1f} |"
        )
    return "\n".join(lines) + "\n"


def parse_repos(items: list[str]) -> dict[str, str]:
    repos: dict[str, str] = {}
    for item in items:
        label, sep, directory = item.partition("=")
        if not sep or not label or not directory:
            raise ValueError(f"--repo wants LABEL=DIR, got {item!r}")
        repos[label] = directory
    return repos


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("--archfit", default=".bin/archfit")
    parser.add_argument(
        "--repo", action="append", default=[], help="LABEL=DIR (repeatable)"
    )
    parser.add_argument("--json-out", help="write the full census as JSON")
    args = parser.parse_args(argv)
    try:
        repos = parse_repos(args.repo)
    except ValueError as err:
        print(err, file=sys.stderr)
        return 2
    rows: dict[str, dict[str, Any]] = {}
    detail: dict[str, Any] = {}
    for label, directory in repos.items():
        state = run_analyze(args.archfit, label, directory)
        rows[label] = summarize(state)
        detail[label] = {
            "summary": rows[label],
            "qualifying_pairs": qualifying_pairs(state),
        }
    print(render_markdown(rows))
    if args.json_out:
        Path(args.json_out).write_text(
            json.dumps(detail, indent=2, sort_keys=True) + "\n"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
