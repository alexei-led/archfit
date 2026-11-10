#!/usr/bin/env python3
"""Count the requests the 4.0 `graph` rule would produce on real history (design 6.2, 15.2, 18).

A module pair (A, B): a production file of module A imports a file of module B. An approximate
import reader finds the pairs after each unit of change: Go imports through go.mod module paths,
Python absolute and relative imports, TS/JS relative imports. TS/JS aliases and bare specifiers,
Rust, and C# are not read. Imports of a file are re-read only when the file changes; targets are
resolved again whenever code files are added, removed or renamed.

An importer whose path looks like a test (corpus_git.is_test_path) is skipped: the counts assume
the 4.0 policy declares its tests. `test_only_pairs_at_head` lists the pairs that leaves out.

In 4.0 `archfit init` writes `depends_on` as the dependencies that exist, so the load is a pair
that first appears after a policy is set. `pairs` reports that for policies set 3, 6 and 12 months
before the last commit, splits pairs that involve a module new since then (itself a policy change),
and also counts pairs seen in history and missing from today's `depends_on`.

    pairs    one repository: pair timeline and the counts above, as JSON
    balance  v3.4.0 seam scores for the pairs, and the reader checked against archfit at HEAD

    uv run python scripts/eval/copies/request_load.py pairs --repo pumba=/tmp/c/pumba \\
        --config /tmp/policy/pumba/.archfit.yaml --out pumba-pairs.json
    uv run python scripts/eval/copies/request_load.py balance --pairs pumba-pairs.json \\
        --repo pumba=/tmp/c/pumba --config /tmp/policy/pumba/.archfit.yaml --out pumba-balance.json

Run on disposable clones: balance adds git worktrees to the clone. Standard library only.
"""

from __future__ import annotations

import argparse
import json
import posixpath
import re
import statistics
import subprocess
import sys
import tempfile
from collections import Counter, defaultdict
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path, PurePosixPath

sys.path.insert(0, str(Path(__file__).resolve().parent))

import corpus_git as cg

READ_LANGUAGES = ("go", "python", "js")
FREEZE_MONTHS = (3, 6, 12)
SECONDS_PER_MONTH = 30.44 * 86400
MIN_BALANCE = 7

Pair = tuple[str, str]

# --- import reader --------------------------------------------------------------------------

GO_BLOCK = re.compile(r"^\s*import\s*\(([^)]*)\)", re.MULTILINE)
GO_SINGLE = re.compile(r'^\s*import\s+(?:[\w.]+\s+)?"([^"]+)"', re.MULTILINE)
GO_SPEC = re.compile(r'"([^"]+)"')
PY_IMPORT = re.compile(r"^[ \t]*import[ \t]+([\w. \t,]+)", re.MULTILINE)
# Names run to the end of the line, or to ")" for a parenthesised list.
PY_FROM = re.compile(
    r"^[ \t]*from[ \t]+(\.*)([\w.]*)[ \t]+import[ \t]*(?:\(([^)]*)\)|([^\n#;]*))", re.MULTILINE
)
JS_SPEC = re.compile(
    r"""(?:\bfrom\s*|\bimport\s*\(?\s*|\brequire\s*\(\s*)['"]([^'"\n]+)['"]"""
)
JS_EXTS = (".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".d.ts")
JS_TO_TS = {
    ".js": (".ts", ".tsx"),
    ".jsx": (".tsx",),
    ".mjs": (".mts",),
    ".cjs": (".cts",),
}


@dataclass(frozen=True)
class PyImport:
    level: int
    name: str
    names: tuple[str, ...]


Spec = str | PyImport


def read_imports(lang: str, text: str) -> list[Spec]:
    if lang == "go":
        specs: list[Spec] = []
        for block in GO_BLOCK.findall(text):
            specs += GO_SPEC.findall(block)
        specs += GO_SINGLE.findall(text)
        return specs
    if lang == "python":
        out: list[Spec] = []
        for m in PY_IMPORT.finditer(text):
            for part in m.group(1).split(","):
                words = part.split()
                if words:
                    out.append(PyImport(0, words[0], ()))
        for m in PY_FROM.finditer(text):
            names = tuple(
                w.split()[0]
                for w in (m.group(3) or m.group(4) or "").replace("\n", ",").split(",")
                if w.split() and w.split()[0] != "*"
            )
            out.append(PyImport(len(m.group(1)), m.group(2), names))
        return out
    if lang == "js":
        return list(JS_SPEC.findall(text))
    return []


class Tree:
    """The code files of one tree, with what import resolution needs."""

    def __init__(self) -> None:
        self.files: set[str] = set()
        self.go_mods: dict[str, str] = {}  # module path -> directory ("" for the root)
        self._go_dirs: dict[str, list[str]] | None = None
        self._py_roots: list[str] | None = None

    def changed(self) -> None:
        self._go_dirs = None
        self._py_roots = None

    def go_dirs(self) -> dict[str, list[str]]:
        if self._go_dirs is None:
            dirs: dict[str, list[str]] = defaultdict(list)
            for f in self.files:
                if f.endswith(".go") and not f.endswith("_test.go"):
                    dirs[posixpath.dirname(f)].append(f)
            self._go_dirs = {d: sorted(fs) for d, fs in dirs.items()}
        return self._go_dirs

    def py_roots(self) -> list[str]:
        if self._py_roots is None:
            inits = {
                posixpath.dirname(f) for f in self.files if f.endswith("__init__.py")
            }
            roots = {""}
            for pkg in inits:
                parent = posixpath.dirname(pkg)
                if pkg and parent not in inits:
                    roots.add(parent)
            self._py_roots = sorted(roots)
        return self._py_roots

    def resolve(self, path: str, lang: str, spec: Spec) -> list[str]:
        """The files an import names; empty when they are outside the tree or not read."""
        if lang == "python" and isinstance(spec, PyImport):
            return self._python(path, spec)
        hit = None
        if lang == "go" and isinstance(spec, str):
            hit = self._go(spec)
        elif lang == "js" and isinstance(spec, str):
            hit = self._js(path, spec)
        return [hit] if hit else []

    def _go(self, spec: str) -> str | None:
        best = max(
            (m for m in self.go_mods if spec == m or spec.startswith(m + "/")),
            key=len,
            default=None,
        )
        if best is None:
            return None
        rel = spec[len(best) :].lstrip("/")
        base = self.go_mods[best]
        target = posixpath.join(base, rel) if base and rel else base or rel
        files = self.go_dirs().get(target)
        return files[0] if files else None

    def _py_file(self, base: str) -> str | None:
        for cand in (base + ".py", base + "/__init__.py"):
            if cand in self.files:
                return cand
        return None

    def _py_dotted(self, root: str, parts: list[str]) -> str | None:
        """The deepest existing module of a dotted name: a.b.c, else a.b, else a."""
        for n in range(len(parts), 0, -1):
            hit = self._py_file(posixpath.join(root, *parts[:n]))
            if hit:
                return hit
        return None

    def _python(self, path: str, spec: PyImport) -> list[str]:
        parts = [p for p in spec.name.split(".") if p]
        if spec.level:
            pkg = posixpath.dirname(path)
            for _ in range(spec.level - 1):
                pkg = posixpath.dirname(pkg)
            roots = [pkg]
        else:
            roots = self.py_roots()
        for root in roots:
            # `from a import b, c` where b and c are submodules: each one is a dependency.
            subs = [self._py_file(posixpath.join(root, *parts, name)) for name in spec.names]
            found = [hit for hit in subs if hit]
            if found:
                return found
            if parts:
                hit = self._py_dotted(root, parts)
                if hit:
                    return [hit]
            elif posixpath.join(root, "__init__.py") in self.files:
                return [posixpath.join(root, "__init__.py")]
        return []

    def _js(self, path: str, spec: str) -> str | None:
        if not spec.startswith("."):
            return None
        base = posixpath.normpath(posixpath.join(posixpath.dirname(path), spec))
        if base.startswith(".."):
            return None
        cands = [
            base,
            *(base + e for e in JS_EXTS),
            *(f"{base}/index{e}" for e in JS_EXTS),
        ]
        stem, ext = posixpath.splitext(base)
        cands += [stem + e for e in JS_TO_TS.get(ext, ())]
        return next((c for c in cands if c in self.files), None)


def is_bare_js(spec: Spec) -> bool:
    return isinstance(spec, str) and not spec.startswith(".")


# --- history --------------------------------------------------------------------------------


def tracked(path: str) -> bool:
    return cg.language(path) is not None or PurePosixPath(path).name == "go.mod"


GO_MODULE = re.compile(r"^\s*module\s+(\S+)", re.MULTILINE)


@dataclass
class PairState:
    first: int
    example: str
    starts: list[int]
    ends: list[int | None]  # None: still present
    past_public: list[bool]  # per start: some import reaches past the target's public part


class Timeline:
    def __init__(self, repo: Path, policy: cg.PolicyMap) -> None:
        self.repo = repo
        self.policy = policy
        self.tree = Tree()
        self.specs: dict[
            str, tuple[str, str, list[Spec]]
        ] = {}  # path -> (module, lang, imports)
        self.resolved: dict[str, frozenset[str]] = {}
        self.pairs: dict[Pair, PairState] = {}
        self.module_first: dict[str, int] = {}
        self.modules_at: list[frozenset[str]] = []
        self.present_at: list[frozenset[Pair]] = []
        self.unresolved = Counter()

    def _module_files(self) -> Counter:
        return Counter(module for module, _, _ in self.specs.values())

    def _targets(self, path: str) -> frozenset[str]:
        if path not in self.resolved:
            _, lang, imports = self.specs[path]
            self.resolved[path] = frozenset(t for imp in imports for t in self.tree.resolve(path, lang, imp))
        return self.resolved[path]

    def current(self) -> dict[Pair, tuple[str, bool]]:
        """Each pair present now: an example importing file, and whether any use is past public."""
        found: dict[Pair, tuple[str, bool]] = {}
        target_files = {t for p in self.specs for t in self._targets(p)}
        owners = self.policy.owners(sorted(target_files))
        for path in sorted(self.specs):
            module = self.specs[path][0]
            for target in self._targets(path):
                owner = owners[target]
                if owner.production and owner.module != module:
                    pair = (module, owner.module)
                    example, past = found.get(pair, (path, False))
                    found[pair] = (example, past or not owner.is_public(target))
        return found

    def apply(self, idx: int, unit: cg.Unit, blobs: cg.BlobReader) -> None:
        rows = cg.changes(self.repo, unit, tracked)
        owners = self.policy.owners(p for _, p, _ in rows)
        structure = False
        for status, path, old in rows:
            if old is not None:
                self._drop(old)
                structure = True
            if status == "D":
                self._drop(path)
                structure = True
                continue
            if path not in self.tree.files:
                structure = True
            self.tree.files.add(path)
            if PurePosixPath(path).name == "go.mod":
                self._go_mod(path, blobs.read(unit.sha, path) or "")
                structure = True
                continue
            lang, owner = cg.language(path), owners[path]
            self.resolved.pop(path, None)
            if (
                lang in READ_LANGUAGES
                and owner.production
                and not cg.is_test_path(path)
            ):
                text = blobs.read(unit.sha, path)
                self.specs[path] = (owner.module, lang, read_imports(lang, text or ""))
            else:
                self.specs.pop(path, None)
        if structure:
            self.tree.changed()
            self.resolved.clear()
        now = self.current()
        for module in self._module_files():
            self.module_first.setdefault(module, idx)
        self.modules_at.append(frozenset(self._module_files()))
        previous = self.present_at[-1] if self.present_at else frozenset()
        for pair in now.keys() - previous:
            state = self.pairs.setdefault(pair, PairState(idx, now[pair][0], [], [], []))
            state.starts.append(idx)
            state.ends.append(None)
            state.past_public.append(now[pair][1])
        for pair in previous - now.keys():
            self.pairs[pair].ends[-1] = idx
        self.present_at.append(frozenset(now))

    def _drop(self, path: str) -> None:
        self.tree.files.discard(path)
        self.specs.pop(path, None)
        self.resolved.pop(path, None)
        if PurePosixPath(path).name == "go.mod":
            self.tree.go_mods = {
                m: d
                for m, d in self.tree.go_mods.items()
                if d != posixpath.dirname(path)
            }

    def _go_mod(self, path: str, text: str) -> None:
        directory = posixpath.dirname(path)
        self.tree.go_mods = {
            m: d for m, d in self.tree.go_mods.items() if d != directory
        }
        m = GO_MODULE.search(text)
        if m:
            self.tree.go_mods[m.group(1)] = directory

    def test_only_pairs(self, rev: str, blobs: cg.BlobReader) -> list[str]:
        """Pairs that only importers skipped as tests would add to the tree at rev."""
        tests = [f for f in sorted(self.tree.files) if cg.language(f) in READ_LANGUAGES and cg.is_test_path(f)]
        owners = self.policy.owners(tests)
        reads = {f: read_imports(cg.language(f) or "", blobs.read(rev, f) or "") for f in tests if owners[f].production}
        targets = {f: [t for imp in imports for t in self.tree.resolve(f, cg.language(f) or "", imp)] for f, imports in reads.items()}
        target_owners = self.policy.owners(t for ts in targets.values() for t in ts)
        present = self.present_at[-1] if self.present_at else frozenset()
        pairs = {
            (owners[f].module, target_owners[t].module)
            for f, ts in targets.items()
            for t in ts
            if target_owners[t].production and target_owners[t].module != owners[f].module
        }
        return sorted(f"{a}->{b}" for a, b in pairs - present)

    def unresolved_at_head(self) -> dict[str, int]:
        counts: Counter = Counter()
        for path, (_, lang, imports) in self.specs.items():
            for imp in imports:
                if not self.tree.resolve(path, lang, imp):
                    counts[
                        f"{lang}_bare" if lang == "js" and is_bare_js(imp) else f"{lang}_outside"
                    ] += 1
        return dict(counts)


def month_index(units: list[cg.Unit], months: int) -> int | None:
    """Index of the last unit at or before `months` before the last unit, if history reaches back."""
    cutoff = units[-1].timestamp - months * SECONDS_PER_MONTH
    if units[0].timestamp > cutoff:
        return None
    return max(i for i, u in enumerate(units) if u.timestamp <= cutoff)


VOLATILITY_ORDER = ("low", "medium", "high")
SUBDOMAIN_VOLATILITY = {"core": "high", "supporting": "low", "generic": "low"}
MODULE_HEADER = re.compile(r"^  ([\w.-]+):\s*(#.*)?$")
MODULE_FIELD = re.compile(r"^    (volatility|subdomain):\s*([\w-]+)")


def declared_volatility(policy_text: str) -> dict[str, str]:
    """Volatility of each module in a policy's `modules:` block, with the 4.0 defaults (design 6.1)."""
    fields: dict[str, dict[str, str]] = {}
    module, in_modules = None, False
    for line in policy_text.split("\n"):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if not line[0].isspace():
            in_modules, module = line.startswith("modules:"), None
            continue
        if not in_modules:
            continue
        header = MODULE_HEADER.match(line)
        if header:
            module = header.group(1)
            fields[module] = {}
            continue
        field = MODULE_FIELD.match(line)
        if field and module:
            fields[module][field.group(1)] = field.group(2)
    return {
        m: f.get("volatility") or SUBDOMAIN_VOLATILITY.get(f.get("subdomain", ""), "high") for m, f in fields.items()
    }


def coupling_4(past_public: bool, from_volatility: str, to_volatility: str) -> tuple[str, str]:
    """The 4.0 `coupling` rule for a new dependency (design 6.2): verdict and the balance it implies."""
    if not past_public:
        return "pass", "9 or 8" if to_volatility == "low" else "9 or 7"
    worst = max(from_volatility, to_volatility, key=VOLATILITY_ORDER.index)
    return {"low": ("pass", "8"), "medium": ("fail", "5"), "high": ("fail", "2")}[worst]


def freeze(tl: Timeline, units: list[cg.Unit], at: int, volatility: dict[str, str]) -> dict:
    """Requests after a policy written at unit `at` with depends_on = the pairs present then."""
    allowed, modules = tl.present_at[at], tl.modules_at[at]
    existing, new_module = [], []
    for pair, state in sorted(tl.pairs.items()):
        if pair in allowed:
            continue
        starts = [s for s in state.starts if s > at]
        if not starts:
            continue
        past = state.past_public[state.starts.index(starts[0])]
        verdict, balance = coupling_4(past, volatility.get(pair[0], "high"), volatility.get(pair[1], "high"))
        row = {
            "from": pair[0],
            "to": pair[1],
            "unit": starts[0],
            "sha": units[starts[0]].sha,
            "past_public": past,
            "coupling_4": verdict,
            "balance_4": balance,
        }
        (existing if pair[0] in modules and pair[1] in modules else new_module).append(
            row
        )
    months = (units[-1].timestamp - units[at].timestamp) / SECONDS_PER_MONTH
    return {
        "policy_at": units[at].sha,
        "policy_date": iso(units[at].timestamp),
        "months_after": round(months, 1),
        "units_after": len(units) - 1 - at,
        "requests_existing_modules": existing,
        "requests_new_module": new_module,
        "per_month_existing": round(len(existing) / months, 2) if months else None,
        "units_with_requests_existing": len({r["unit"] for r in existing}),
        "new_modules": sorted(m for m, first in tl.module_first.items() if first > at),
        "units_with_new_module_pairs": len({r["unit"] for r in new_module}),
        "coupling_4_existing": dict(Counter(r["coupling_4"] for r in existing)),
    }


def iso(ts: int) -> str:
    return datetime.fromtimestamp(ts, UTC).date().isoformat()


def pairs_report(repo: Path, label: str, policy: cg.PolicyMap, volatility: dict[str, str] | None = None) -> dict:
    volatility = volatility or {}
    units = cg.first_parent_units(repo)
    tl = Timeline(repo, policy)
    blobs = cg.BlobReader(repo)
    try:
        for idx, unit in enumerate(units):
            tl.apply(idx, unit, blobs)
        test_only = tl.test_only_pairs(units[-1].sha, blobs)
    finally:
        blobs.close()
    last = len(units) - 1
    depends: dict[str, tuple[str, ...] | None] = {}
    for path, (module, _, _) in tl.specs.items():
        depends.setdefault(module, policy.owner(path).depends_on)
    pair_rows = []
    for pair, state in sorted(tl.pairs.items()):
        allowed = pair[1] in (depends.get(pair[0]) or ())
        pair_rows.append(
            {
                "from": pair[0],
                "to": pair[1],
                "first_unit": state.first,
                "first_sha": units[state.first].sha,
                "first_date": iso(units[state.first].timestamp),
                "first_subject": units[state.first].subject,
                "example": state.example,
                "intervals": [list(iv) for iv in zip(state.starts, state.ends)],
                "at_head": pair in tl.present_at[last],
                "in_depends_on": allowed,
                "with_new_module": state.first
                in (tl.module_first.get(pair[0]), tl.module_first.get(pair[1])),
                "past_public_first": state.past_public[0],
                "coupling_4_first": coupling_4(
                    state.past_public[0], volatility.get(pair[0], "high"), volatility.get(pair[1], "high")
                )[0],
            }
        )
    later = [r for r in pair_rows if r["first_unit"] > 0]
    span = (units[-1].timestamp - units[0].timestamp) / SECONDS_PER_MONTH
    scenarios = {}
    for months in FREEZE_MONTHS:
        at = month_index(units, months)
        if at is not None:
            scenarios[f"{months}m"] = freeze(tl, units, at, volatility)
    missing = [r for r in pair_rows if not r["in_depends_on"]]
    return {
        "repo": label,
        "head": units[-1].sha,
        "units": len(units),
        "merges": sum(u.is_merge for u in units),
        "first_date": iso(units[0].timestamp),
        "last_date": iso(units[-1].timestamp),
        "modules": sorted(tl.module_first),
        "volatility": {m: volatility.get(m, "high") for m in sorted(tl.module_first)},
        # `policy where` omits an empty list; empty and absent both allow nothing in 4.0.
        "nonempty_depends_on": sorted(m for m, d in depends.items() if d),
        "pairs_ever": len(pair_rows),
        "pairs_at_head": len(tl.present_at[last]),
        "first_appearances_after_root": {
            "total": len(later),
            "existing_modules": sum(not r["with_new_module"] for r in later),
            "per_month_existing": round(
                sum(not r["with_new_module"] for r in later) / span, 2
            )
            if span
            else None,
            "per_100_units_existing": round(
                100 * sum(not r["with_new_module"] for r in later) / len(units), 2
            ),
        },
        "freeze": scenarios,
        "missing_from_depends_on": {
            "pairs": len(missing),
            "commits": len({r["first_sha"] for r in missing}),
        },
        "unresolved_at_head": tl.unresolved_at_head(),
        "test_only_pairs_at_head": test_only,
        "pairs": pair_rows,
    }


# --- balance --------------------------------------------------------------------------------


def seam_scores(archfit: str, config: Path, root: Path) -> dict[Pair, dict]:
    result = subprocess.run(
        [
            archfit,
            "analyze",
            "--format",
            "json",
            "-c",
            str(config),
            "--root",
            str(root),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"archfit analyze: exit {result.returncode}: {result.stderr.strip()[:300]}"
        )
    out = {}
    for seam in json.loads(result.stdout).get("seams", []):
        scores = seam.get("scores") or {}
        out[(seam["from_module"], seam["to_module"])] = {
            "n": scores.get("n", 0),
            "min": scores.get("min"),
            "median": scores.get("median"),
            "strength": seam.get("strength"),
            "volatility": seam.get("volatility"),
            "volatility_provenance": seam.get("volatility_provenance"),
            "edges": seam.get("edges"),
        }
    return out


def at_commit(repo: Path, sha: str, archfit: str, config: Path) -> dict[Pair, dict]:
    with tempfile.TemporaryDirectory(prefix="archfit-balance-") as tmp:
        tree = Path(tmp) / "tree"
        cg.git(repo, "worktree", "add", "--detach", "-q", str(tree), sha)
        try:
            return seam_scores(archfit, config, tree)
        finally:
            cg.git(repo, "worktree", "remove", "--force", str(tree))


def balance_report(
    report: dict, repo: Path, archfit: str, config: Path, cap: int
) -> dict:
    head = seam_scores(archfit, config, repo)
    reader_head = {(r["from"], r["to"]) for r in report["pairs"] if r["at_head"]}
    seam_head = {pair for pair, s in head.items() if s.get("edges")}
    wanted = {(r["from"], r["to"]): r for r in report["pairs"]}
    gone = {r["first_sha"] for r in wanted.values() if not r["at_head"]}
    scores: dict[Pair, dict] = {pair: head[pair] for pair in wanted if pair in head}
    measured_gone = len(gone) <= cap
    if measured_gone:
        for sha in sorted(gone):
            found = at_commit(repo, sha, archfit, config)
            for pair, row in wanted.items():
                if row["first_sha"] == sha and not row["at_head"] and pair in found:
                    scores[pair] = found[pair]
    return {
        "repo": report["repo"],
        "label": "v3.4.0 seam score, an approximation of the 4.0 balance",
        "gone_pairs_measured": measured_gone,
        "gone_pair_commits": len(gone),
        "reader_vs_archfit_at_head": {
            "both": len(reader_head & seam_head),
            "reader_only": sorted(f"{a}->{b}" for a, b in reader_head - seam_head),
            "archfit_only": sorted(f"{a}->{b}" for a, b in seam_head - reader_head),
        },
        "scores": [{"from": a, "to": b, **s} for (a, b), s in sorted(scores.items())],
    }


def distribution(rows: list[dict], scores: dict[Pair, dict]) -> dict:
    """Balance of a set of pairs against the minimum: the weakest edge of each pair decides."""
    mins, unknown, missing = [], 0, 0
    for row in rows:
        s = scores.get((row["from"], row["to"]))
        if s is None:
            missing += 1
        elif not s["n"]:
            unknown += 1
        else:
            mins.append(s["min"])
    return {
        "pairs": len(rows),
        "scored": len(mins),
        "pass": sum(m >= MIN_BALANCE for m in mins),
        "below": sum(m < MIN_BALANCE for m in mins),
        "abstained": unknown,
        "not_measured": missing,
        "median_of_min": statistics.median(mins) if mins else None,
        "histogram": dict(sorted(Counter(mins).items())),
    }


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
    for name in ("pairs", "balance"):
        p = sub.add_parser(name)
        p.add_argument("--repo", type=repo_arg, required=True)
        p.add_argument("--config", type=Path, required=True)
        p.add_argument("--archfit", default="archfit")
        p.add_argument("--out", type=Path, required=True)
        if name == "balance":
            p.add_argument("--pairs", type=Path, required=True)
            p.add_argument("--max-commits", type=int, default=30)
    args = ap.parse_args(argv)
    label, repo = args.repo
    if args.cmd == "pairs":
        policy = cg.PolicyMap(cg.archfit_resolver(args.archfit, args.config, repo))
        report = pairs_report(repo, label, policy, declared_volatility(args.config.read_text()))
        args.out.write_text(json.dumps(report, indent=1))
        print(
            json.dumps(
                {k: v for k, v in report.items() if k not in ("pairs", "freeze")}
            )
        )
    else:
        report = json.loads(args.pairs.read_text())
        bal = balance_report(report, repo, args.archfit, args.config, args.max_commits)
        scores = {(s["from"], s["to"]): s for s in bal["scores"]}
        bal["missing_from_depends_on"] = distribution(
            [r for r in report["pairs"] if not r["in_depends_on"]], scores
        )
        bal["freeze"] = {
            key: distribution(sc["requests_existing_modules"], scores)
            for key, sc in report["freeze"].items()
        }
        args.out.write_text(json.dumps(bal, indent=1))
        print(json.dumps({k: v for k, v in bal.items() if k != "scores"}, indent=1))
    return 0


if __name__ == "__main__":
    sys.exit(main())
