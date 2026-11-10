#!/usr/bin/env python3
"""Measure the noise of an identical-literal copy check on real history (design 7.3, 15.2).

A flagged copy: a unit of change adds a string literal to production code of module M, and
production code of another module O already holds the identical literal in the parent tree and
still holds it after the change (a move is not a copy). Literals come from single lines only: Go
"…" and `…`, Python '…' and "…" (not f- or b-strings), TS/JS '…', "…" and `…` without ${, Rust
"…" and r"…". Tables and multi-line literals are not detected.

    scan    one repository: write flagged events as JSON lines
    sample  pick events for labelling and write the context of each one
    score   precision of each noise rule on the labelled sample, recall on the seeded events

    uv run python scripts/eval/copies/copy_noise.py scan --repo pumba=/tmp/c/pumba \\
        --config /tmp/policy/pumba/.archfit.yaml --out pumba.jsonl
    uv run python scripts/eval/copies/copy_noise.py sample --events all.jsonl --size 120 \\
        --repo pumba=/tmp/c/pumba --out sample.md --labels labels.tsv
    uv run python scripts/eval/copies/copy_noise.py score --events all.jsonl --labels labels.tsv \\
        --seeded seeded.jsonl

Run scan on disposable clones. Standard library only.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import random
import re
import sys
from collections import defaultdict
from collections.abc import Callable, Iterable
from dataclasses import asdict, dataclass, field
from datetime import UTC, datetime
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import corpus_git as cg

FLOOR = 6
MAX_OTHER_SITES = 3

# --- literals -------------------------------------------------------------------------------

PY_STRING = re.compile(r"([rRbBuUfF]{0,2})('''|\"\"\"|'|\")")
IDENT = re.compile(r"\w")


def _read_quoted(
    line: str, start: int, quote: str, escapes: bool
) -> tuple[int | None, str]:
    """Scan from start to the closing quote. Returns (index after it, text) or (None, "")."""
    j = start
    while j < len(line):
        if escapes and line[j] == "\\":
            j += 2
            continue
        if line.startswith(quote, j):
            return j + len(quote), line[start:j]
        j += 1
    return None, ""


def literals(line: str, lang: str) -> list[str]:
    """String literals that open and close on this line, as source text between the quotes."""
    out: list[str] = []
    i, n = 0, len(line)
    while i < n:
        c = line[i]
        if lang == "python":
            if c == "#":
                break
            m = PY_STRING.match(line, i)
            if m and m.group(2) and (i == 0 or not IDENT.match(line[i - 1])):
                prefix, quote = m.group(1).lower(), m.group(2)
                end, text = _read_quoted(line, m.end(), quote, escapes=True)
                if end is None:
                    break
                if "f" not in prefix and "b" not in prefix:
                    out.append(text)
                i = end
                continue
            if c.isalpha() or c == "_":
                while i < n and (line[i].isalnum() or line[i] == "_"):
                    i += 1
                continue
            i += 1
            continue
        if line.startswith("//", i):
            break
        if line.startswith("/*", i):
            close = line.find("*/", i + 2)
            if close < 0:
                break
            i = close + 2
            continue
        if lang == "rust" and c in "rb" and (i == 0 or not IDENT.match(line[i - 1])):
            m = re.match(r"(b?)r(#*)\"", line[i:])
            if m:
                end = line.find('"' + m.group(2), i + m.end())
                if end < 0:
                    break
                if not m.group(1):
                    out.append(line[i + m.end() : end])
                i = end + 1 + len(m.group(2))
                continue
            if line.startswith('b"', i):
                end, _ = _read_quoted(line, i + 2, '"', escapes=True)
                if end is None:
                    break
                i = end
                continue
        if c == '"':
            end, text = _read_quoted(line, i + 1, '"', escapes=True)
            if end is None:
                break
            out.append(text)
            i = end
            continue
        if c == "`" and lang in ("go", "js"):
            end, text = _read_quoted(line, i + 1, "`", escapes=lang == "js")
            if end is None:
                break
            if "${" not in text:
                out.append(text)
            i = end
            continue
        if c == "'":
            if lang == "js":
                end, text = _read_quoted(line, i + 1, "'", escapes=True)
                if end is None:
                    break
                out.append(text)
                i = end
                continue
            # Go rune or Rust char; a Rust lifetime has no closing quote nearby.
            m = re.match(r"'(\\.[^']*|[^\\'])'", line[i:])
            i += m.end() if m else 1
            continue
        i += 1
    return out


GO_IMPORT_SPEC = re.compile(r'\s*(import\s+)?(\(\s*)?([\w.]+\s+)?"[^"]*"\s*\)?\s*(//.*)?')
JS_IMPORT_SPEC = re.compile(r"""(?:\bfrom\s*|\bimport\s*\(?\s*|\brequire\s*\(\s*)['"]([^'"]+)['"]""")


def code_literals(line: str, lang: str) -> list[str]:
    """Literals of a line, without import paths: they name a package, they hold no knowledge."""
    if lang == "go" and GO_IMPORT_SPEC.fullmatch(line):
        return []
    found = literals(line, lang)
    if lang == "js":
        specs = set(JS_IMPORT_SPEC.findall(line))
        found = [lit for lit in found if lit not in specs]
    return found


GO_TAG = re.compile(r'(\w+:"[^"]*"\s*)+')
URL = re.compile(r"^[a-z][a-z0-9+.-]*://")
REGEX_HINT = re.compile(r"^\^|\$$|\\[dwsbDWSB]|\[[^\]]+\][*+?{]|\(\?|\.\*|\.\+")
WORD = re.compile(r"^[\w.:@-]+$")


def shape(literal: str) -> str:
    if GO_TAG.fullmatch(literal):
        return "tag"
    if URL.match(literal):
        return "url"
    if REGEX_HINT.search(literal):
        return "regex"
    if any(ch.isspace() for ch in literal.strip()):
        return "text"
    if "/" in literal or "\\" in literal:
        return "path"
    if WORD.match(literal):
        return "word"
    return "other"


# --- diffs and trees -----------------------------------------------------------------------

HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")


def added_lines(diff: str) -> dict[str, list[tuple[int, str]]]:
    """Added lines per new path from `git diff -U0` output."""
    out: dict[str, list[tuple[int, str]]] = defaultdict(list)
    path: str | None = None
    line_no = 0
    for raw in diff.split("\n"):
        if raw.startswith("+++ "):
            target = raw[4:]
            path = target[2:] if target.startswith("b/") else None
            continue
        if raw.startswith("diff --git ") or raw.startswith("--- "):
            continue
        m = HUNK.match(raw)
        if m:
            line_no = int(m.group(1))
            continue
        if path is None:
            continue
        if raw.startswith("+"):
            out[path].append((line_no, raw[1:]))
            line_no += 1
        elif raw.startswith(" "):
            line_no += 1
    return dict(out)


@dataclass(frozen=True)
class Site:
    path: str
    line: int
    module: str


def code_file(path: str) -> bool:
    return cg.language(path) is not None


class LiteralIndex:
    """Literals of the production code in the current tree: literal -> {path: first line}.

    Kept up to date from each unit's changed files, so a lookup needs no search of the tree."""

    def __init__(self, policy: cg.PolicyMap, blobs: cg.BlobReader) -> None:
        self.policy = policy
        self.blobs = blobs
        self.by_literal: dict[str, dict[str, int]] = defaultdict(dict)
        self.by_file: dict[str, list[str]] = {}

    def _drop(self, path: str) -> None:
        for lit in self.by_file.pop(path, ()):
            sites = self.by_literal[lit]
            sites.pop(path, None)
            if not sites:
                del self.by_literal[lit]

    def apply(self, rev: str, rows: list[tuple[str, str, str | None]]) -> None:
        owners = self.policy.owners(path for _, path, _ in rows)
        for status, path, old in rows:
            if old is not None:
                self._drop(old)
            self._drop(path)
            lang = cg.language(path)
            if status == "D" or lang is None or not owners[path].production:
                continue
            first: dict[str, int] = {}
            for n, line in enumerate((self.blobs.read(rev, path) or "").split("\n"), 1):
                for lit in code_literals(line, lang):
                    if len(lit) >= FLOOR:
                        first.setdefault(lit, n)
            self.by_file[path] = list(first)
            for lit, n in first.items():
                self.by_literal[lit][path] = n

    def sites(self, literals: Iterable[str]) -> dict[str, list[Site]]:
        found = {lit: self.by_literal[lit] for lit in literals if lit in self.by_literal}
        owners = self.policy.owners(path for paths in found.values() for path in paths)
        return {
            lit: [Site(path, line, owners[path].module) for path, line in sorted(paths.items())]
            for lit, paths in found.items()
        }


# --- scan ----------------------------------------------------------------------------------


@dataclass
class Event:
    repo: str
    unit: str
    date: str
    merge: bool
    path: str
    line: int
    module: str
    literal: str
    other_module: str
    other_sites: list[str]
    holder_modules: int
    holder_files: int
    module_had_it: bool
    length: int = 0
    lang: str = ""
    shape: str = ""
    test_added: bool = False
    test_other: bool = False
    generated_added: bool = False
    generated_other: bool = False
    id: str = field(default="")

    def __post_init__(self) -> None:
        self.length = len(self.literal)
        self.lang = cg.language(self.path) or ""
        self.shape = shape(self.literal)
        if not self.id:
            key = "\0".join(
                [
                    self.repo,
                    self.unit,
                    self.path,
                    str(self.line),
                    self.literal,
                    self.other_module,
                ]
            )
            self.id = f"{self.repo}-{hashlib.sha1(key.encode()).hexdigest()[:10]}"


@dataclass
class UnitStats:
    units: int = 0
    merges: int = 0
    added_literals: int = 0
    candidates: int = 0
    moved: int = 0
    events: int = 0


def scan_unit(
    repo: Path, label: str, unit: cg.Unit, policy: cg.PolicyMap, stats: UnitStats, index: LiteralIndex
) -> list[Event]:
    """Events of one unit. The index holds the parent tree on entry and this unit's tree on exit."""
    changed = cg.changes(repo, unit, code_file)
    if unit.parent is None:
        index.apply(unit.sha, changed)
        return []
    diff = cg.git(
        repo,
        "diff",
        "-U0",
        "--no-color",
        "--no-ext-diff",
        "--src-prefix=a/",
        "--dst-prefix=b/",
        "-M",
        unit.parent,
        unit.sha,
        "--",
        *cg.CODE_PATHSPECS,
    )
    added = added_lines(diff)
    owners = policy.owners(added)
    sites: dict[str, list[Site]] = defaultdict(list)
    for path, rows in added.items():
        owner, lang = owners[path], cg.language(path)
        if not owner.production or lang is None:
            continue
        for line_no, text in rows:
            for lit in code_literals(text, lang):
                stats.added_literals += 1
                if len(lit) >= FLOOR:
                    sites[lit].append(Site(path, line_no, owner.module))
    stats.candidates += len(sites)
    before = index.sites(sites)
    index.apply(unit.sha, changed)
    if not before:
        return []
    after = index.sites(before)
    date = datetime.fromtimestamp(unit.timestamp, UTC).date().isoformat()
    events: list[Event] = []
    for lit, added_sites in sites.items():
        held = before.get(lit, [])
        by_module: dict[str, list[Site]] = defaultdict(list)
        for site in held:
            by_module[site.module].append(site)
        still = {site.module for site in after.get(lit, [])}
        seen: set[tuple[str, str]] = set()
        for site in added_sites:
            for other, other_sites in sorted(by_module.items()):
                if other == site.module or (site.module, other) in seen:
                    continue
                seen.add((site.module, other))
                if other not in still:
                    stats.moved += 1
                    continue
                events.append(
                    Event(
                        repo=label,
                        unit=unit.sha,
                        date=date,
                        merge=unit.is_merge,
                        path=site.path,
                        line=site.line,
                        module=site.module,
                        literal=lit,
                        other_module=other,
                        other_sites=[
                            f"{s.path}:{s.line}" for s in other_sites[:MAX_OTHER_SITES]
                        ],
                        holder_modules=len(by_module),
                        holder_files=len({s.path for s in held}),
                        module_had_it=site.module in by_module,
                    )
                )
    stats.events += len(events)
    return events


def mark_heuristics(repo: Path, events: list[Event]) -> None:
    """Test and generated flags; header reads only for files that carry an event."""
    cache: dict[tuple[str, str], bool] = {}

    def generated(rev: str, path: str) -> bool:
        if cg.is_generated_path(path):
            return True
        key = (rev, path)
        if key not in cache:
            text = cg.git(repo, "show", f"{rev}:{path}", ok=(0, 128))
            cache[key] = cg.has_generated_header(text)
        return cache[key]

    parents: dict[str, str] = {}
    for ev in events:
        if ev.unit not in parents:
            parents[ev.unit] = cg.git(repo, "rev-parse", f"{ev.unit}^1").strip()
        others = [s.rsplit(":", 1)[0] for s in ev.other_sites]
        ev.test_added = cg.is_test_path(ev.path)
        ev.test_other = all(cg.is_test_path(p) for p in others)
        ev.generated_added = generated(ev.unit, ev.path)
        ev.generated_other = all(generated(parents[ev.unit], p) for p in others)


def scan(
    repo: Path, label: str, policy: cg.PolicyMap, since: str | None
) -> tuple[list[Event], UnitStats]:
    stats = UnitStats()
    events: list[Event] = []
    units = cg.first_parent_units(repo, since=since)
    blobs = cg.BlobReader(repo)
    try:
        index = LiteralIndex(policy, blobs)
        if units and units[0].parent:
            start = units[0].parent
            index.apply(start, [("A", path, None) for path in cg.tree_files(repo, start, code_file)])
        for unit in units:
            stats.units += 1
            stats.merges += unit.is_merge
            events.extend(scan_unit(repo, label, unit, policy, stats, index))
    finally:
        blobs.close()
    mark_heuristics(repo, events)
    return events, stats


def write_events(path: Path, events: Iterable[Event]) -> None:
    with path.open("w") as fh:
        for ev in events:
            fh.write(json.dumps(asdict(ev), sort_keys=True) + "\n")


def read_events(paths: Iterable[Path]) -> list[dict]:
    out = []
    for path in paths:
        out.extend(
            json.loads(line) for line in path.read_text().splitlines() if line.strip()
        )
    return out


# --- sample ---------------------------------------------------------------------------------


def context(repo: Path, rev: str, path: str, line: int, radius: int = 2) -> str:
    text = cg.git(repo, "show", f"{rev}:{path}", ok=(0, 128)).split("\n")
    lo, hi = max(1, line - radius), min(len(text), line + radius)
    return "\n".join(
        f"{n:5d}{'>' if n == line else ' '} {text[n - 1]}" for n in range(lo, hi + 1)
    )


def sample(events: list[dict], sizes: tuple[int, int], seed: int, stratum: Rule) -> list[tuple[dict, str]]:
    """Uniform samples from the events the stratum rule keeps ("in") and from the rest ("out")."""
    rng = random.Random(seed)
    ordered = sorted(events, key=lambda e: e["id"])
    chosen: list[tuple[dict, str]] = []
    for name, size in zip(("in", "out"), sizes):
        pool = [ev for ev in ordered if stratum.keep(ev) == (name == "in")]
        picked = pool if len(pool) <= size else rng.sample(pool, size)
        chosen += [(ev, name) for ev in picked]
    return sorted(chosen, key=lambda pair: pair[0]["id"])


def write_sample(
    chosen: list[tuple[dict, str]], repos: dict[str, Path], out: Path, labels: Path
) -> None:
    lines = ["# Copy-noise sample", ""]
    for ev, _ in chosen:
        repo = repos[ev["repo"]]
        parent = cg.git(repo, "rev-parse", f"{ev['unit']}^1").strip()
        other_path, other_line = ev["other_sites"][0].rsplit(":", 1)
        lines += [
            f"## {ev['id']}",
            "",
            f"- literal ({ev['length']}, {ev['shape']}): `{ev['literal']}`",
            f"- added in `{ev['module']}` {ev['path']}:{ev['line']} at {ev['unit'][:10]} ({ev['date']})",
            f"- held by `{ev['other_module']}` {', '.join(ev['other_sites'])}"
            f" ({ev['holder_modules']} modules, {ev['holder_files']} files)",
            "",
            "```",
            context(repo, ev["unit"], ev["path"], ev["line"]),
            "-----",
            context(repo, parent, other_path, int(other_line)),
            "```",
            "",
        ]
    out.write_text("\n".join(lines))
    with labels.open("w") as fh:
        fh.write("id\tstratum\tlabel\tnote\n")
        for ev, name in chosen:
            fh.write(f"{ev['id']}\t{name}\t\t\n")


# --- score ----------------------------------------------------------------------------------

COPY = "copy"
NOISE = ("noise-convention", "noise-coincidence")
UNCLEAR = "unclear"


@dataclass(frozen=True)
class Rule:
    name: str
    keep: Callable[[dict], bool]


def rules() -> list[Rule]:
    def rule(name: str, min_len: int = FLOOR, **drop: bool | int) -> Rule:
        def keep(ev: dict) -> bool:
            if ev["length"] < min_len:
                return False
            if drop.get("tests") and (ev["test_added"] or ev["test_other"]):
                return False
            if drop.get("generated") and (
                ev["generated_added"] or ev["generated_other"]
            ):
                return False
            if drop.get("words") and ev["shape"] in ("word", "tag"):
                return False
            common = drop.get("common")
            if common and ev["holder_modules"] >= common:
                return False
            return True

        return Rule(name, keep)

    out = [rule("floor: length >= 6")]
    out += [rule(f"length >= {n}", n) for n in (8, 12, 16, 20, 24, 32)]
    out += [
        rule("drop test paths", tests=True),
        rule("drop generated", generated=True),
        rule("drop single words and tags", words=True),
        rule("drop if held by >= 3 modules", common=3),
        rule("drop if held by >= 2 modules", common=2),
    ]
    for n in (8, 12, 16):
        out.append(
            rule(
                f"length >= {n} + tests + generated + words",
                n,
                tests=True,
                generated=True,
                words=True,
            )
        )
        out.append(
            rule(
                f"length >= {n} + tests + generated + words + held < 3",
                n,
                tests=True,
                generated=True,
                words=True,
                common=3,
            )
        )
    return out


STRATUM_RULE = "length >= 8 + tests + generated + words"


def rule_named(name: str) -> Rule:
    found = [r for r in rules() if r.name == name]
    if not found:
        raise ValueError(f"no rule named {name!r}")
    return found[0]


def wilson(successes: float, n: float, z: float = 1.96) -> tuple[float, float]:
    if n == 0:
        return (0.0, 0.0)
    p = successes / n
    centre = (p + z * z / (2 * n)) / (1 + z * z / n)
    half = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / (1 + z * z / n)
    return (max(0.0, centre - half), min(1.0, centre + half))


def read_labels(path: Path) -> dict[str, str]:
    out = {}
    for row in path.read_text().splitlines()[1:]:
        parts = row.split("\t")
        if len(parts) >= 3 and parts[2].strip():
            out[parts[0].strip()] = parts[2].strip()
    return out


@dataclass
class Score:
    rule: str
    flagged: int
    labelled: int
    copies: int
    noise: int
    unclear: int
    precision: float | None
    low: float | None
    high: float | None
    seeded: int
    seeded_found: int


def seeds_found(
    keep: Callable[[dict], bool], seeds: list[dict], seeded_events: list[dict]
) -> int:
    """Seeds that a kept event reports: same literal, same added module, same holder module."""
    return sum(
        1
        for seed in seeds
        if any(
            keep(ev)
            and (ev["literal"], ev["module"], ev["other_module"])
            == (seed["literal"], seed["module"], seed["other_module"])
            for ev in seeded_events
        )
    )


def weights(events: list[dict], labels: dict[str, str], stratum: Rule | None) -> dict[str, float]:
    """Inverse sampling fraction of each labelled event: stratum size / labelled in the stratum."""
    if stratum is None:
        return {ev["id"]: 1.0 for ev in events if ev["id"] in labels}
    size = {True: 0, False: 0}
    judged = {True: 0, False: 0}
    for ev in events:
        side = stratum.keep(ev)
        size[side] += 1
        judged[side] += ev["id"] in labels
    return {
        ev["id"]: size[stratum.keep(ev)] / judged[stratum.keep(ev)] for ev in events if ev["id"] in labels
    }


def score(
    events: list[dict],
    labels: dict[str, str],
    seeds: list[dict],
    seeded_events: list[dict],
    stratum: Rule | None = None,
    repos: set[str] | None = None,
) -> list[Score]:
    """Precision per rule, weighted by stratum; the interval uses the effective sample size."""
    identical = [seed for seed in seeds if seed["kind"] == "identical"]
    bad = {label for label in labels.values() if label not in (COPY, UNCLEAR, *NOISE)}
    if bad:
        raise ValueError(f"unknown labels: {sorted(bad)}")
    weight = weights(events, labels, stratum)
    if repos:
        events = [ev for ev in events if ev["repo"] in repos]
    out = []
    for r in rules():
        kept = [ev for ev in events if r.keep(ev)]
        judged = [(labels[ev["id"]], weight[ev["id"]]) for ev in kept if ev["id"] in labels]
        decided = [(label, w) for label, w in judged if label != UNCLEAR]
        total = sum(w for _, w in decided)
        copies_w = sum(w for label, w in decided if label == COPY)
        precision = copies_w / total if total else None
        n_eff = total * total / sum(w * w for _, w in decided) if decided else 0.0
        low, high = wilson(precision * n_eff, n_eff) if decided and precision is not None else (None, None)
        out.append(
            Score(
                rule=r.name,
                flagged=len(kept),
                labelled=len(judged),
                copies=sum(label == COPY for label, _ in judged),
                noise=sum(label in NOISE for label, _ in judged),
                unclear=sum(label == UNCLEAR for label, _ in judged),
                precision=precision,
                low=low,
                high=high,
                seeded=len(identical),
                seeded_found=seeds_found(r.keep, identical, seeded_events),
            )
        )
    return out


def render(scores: list[Score]) -> str:
    def pct(x: float | None) -> str:
        return "n/a" if x is None else f"{100 * x:.0f}%"

    rows = [
        "| Rule | Flagged | Labelled | Copy | Noise | Unclear | Precision | 95% interval | Seeded recall |",
        "| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: |",
    ]
    for s in scores:
        recall = f"{s.seeded_found}/{s.seeded}" if s.seeded else "n/a"
        rows.append(
            f"| {s.rule} | {s.flagged} | {s.labelled} | {s.copies} | {s.noise} | {s.unclear} | {pct(s.precision)} "
            f"| {pct(s.low)}–{pct(s.high)} | {recall} |"
        )
    return "\n".join(rows)


# --- CLI ------------------------------------------------------------------------------------


def repo_arg(value: str) -> tuple[str, Path]:
    label, _, path = value.partition("=")
    if not label or not path:
        raise argparse.ArgumentTypeError("expected LABEL=DIR")
    return label, Path(path).resolve()


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    sub = ap.add_subparsers(dest="cmd", required=True)
    sc = sub.add_parser("scan")
    sc.add_argument("--repo", type=repo_arg, required=True)
    sc.add_argument("--config", type=Path, required=True)
    sc.add_argument("--archfit", default="archfit")
    sc.add_argument(
        "--since", help="git date, e.g. '365 days ago'; default: whole history"
    )
    sc.add_argument("--out", type=Path, required=True)
    sm = sub.add_parser("sample")
    sm.add_argument("--events", type=Path, nargs="+", required=True)
    sm.add_argument("--repo", type=repo_arg, action="append", required=True)
    sm.add_argument("--size-in", type=int, default=80, help="events the stratum rule keeps")
    sm.add_argument("--size-out", type=int, default=40, help="events the stratum rule drops")
    sm.add_argument("--stratum-rule", default=STRATUM_RULE)
    sm.add_argument("--seed", type=int, default=20261110)
    sm.add_argument("--out", type=Path, required=True)
    sm.add_argument("--labels", type=Path, required=True)
    so = sub.add_parser("score")
    so.add_argument("--events", type=Path, nargs="+", required=True)
    so.add_argument("--labels", type=Path, required=True)
    so.add_argument("--stratum-rule", default=STRATUM_RULE, help="the rule the sample was drawn with; '' for none")
    so.add_argument("--repos", nargs="*", help="score only these repositories")
    so.add_argument(
        "--seeds",
        type=Path,
        help="expected seeds, JSON lines: literal, module, other_module, kind",
    )
    so.add_argument(
        "--seeded-events",
        type=Path,
        nargs="*",
        default=[],
        help="scan output of the seeded clones",
    )
    args = ap.parse_args(argv)

    if args.cmd == "scan":
        label, repo = args.repo
        policy = cg.PolicyMap(cg.archfit_resolver(args.archfit, args.config, repo))
        events, stats = scan(repo, label, policy, args.since)
        write_events(args.out, events)
        print(json.dumps({"repo": label, **asdict(stats)}))
    elif args.cmd == "sample":
        chosen = sample(
            read_events(args.events), (args.size_in, args.size_out), args.seed, rule_named(args.stratum_rule)
        )
        write_sample(chosen, dict(args.repo), args.out, args.labels)
        print(f"{len(chosen)} events -> {args.out}, label in {args.labels}")
    else:
        events = read_events(args.events)
        seeds = read_events([args.seeds]) if args.seeds else []
        seeded_events = read_events(args.seeded_events)
        stratum = rule_named(args.stratum_rule) if args.stratum_rule else None
        labels = read_labels(args.labels)
        repos = set(args.repos) if args.repos else None
        print(render(score(events, labels, seeds, seeded_events, stratum, repos)))
        rewritten = [seed for seed in seeds if seed["kind"] == "rewritten"]
        if rewritten:
            found = seeds_found(lambda ev: True, rewritten, seeded_events)
            print(f"\nRewritten seeds found at the floor: {found}/{len(rewritten)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
