"""Shared pieces of the parser-based extractor prototypes.

The prototypes write the same JSON shape as ``gofacts`` (see gofacts/main.go):
sources, first-party edges, unknown imports, and an external count.
"""

from __future__ import annotations

import json
import re
import subprocess
from functools import cache, lru_cache
from pathlib import Path

RULES_DIR = Path(__file__).resolve().parent / "rules"

# ast-grep reads every file it is given, whatever ignore files say.
_NO_IGNORE = [
    arg
    for kind in ("hidden", "dot", "exclude", "global", "parent", "vcs")
    for arg in ("--no-ignore", kind)
]

# Paths per ast-grep call; keeps the command line short on large trees.
_BATCH = 500


def ast_grep(rules: str, root: Path, files: list[str]) -> list[dict]:
    """Run one ast-grep rule file over root-relative files; return its matches."""
    matches: list[dict] = []
    for start in range(0, len(files), _BATCH):
        batch = files[start : start + _BATCH]
        proc = subprocess.run(
            [
                "ast-grep",
                "scan",
                "--rule",
                str(RULES_DIR / rules),
                "--json=stream",
                *_NO_IGNORE,
                *batch,
            ],
            cwd=root,
            capture_output=True,
            text=True,
            check=False,
        )
        if proc.returncode not in (0, 1):
            raise RuntimeError(
                f"ast-grep exited {proc.returncode}: {proc.stderr.strip()}"
            )
        matches.extend(
            json.loads(line) for line in proc.stdout.splitlines() if line.strip()
        )
    return matches


@cache
def _glob_regex(pattern: str) -> re.Pattern[str]:
    """Translate a doublestar glob as the engine matches it to a regex.

    Supports ``**`` segments, ``*``, and ``?``. Brace and class patterns do not
    occur in the corpus configs; they match literally here.
    """
    out = []
    i = 0
    while i < len(pattern):
        if pattern.startswith("**/", i):
            out.append("(?:.*/)?")
            i += 3
        elif pattern.startswith("/**", i) and i + 3 == len(pattern):
            out.append("(?:/.*)?")
            i += 3
        elif pattern.startswith("**", i):
            out.append(".*")
            i += 2
        elif pattern[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pattern[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pattern[i]))
            i += 1
    return re.compile("^" + "".join(out) + "$")


def glob_match(pattern: str, path: str) -> bool:
    """Report whether a slash path matches a doublestar glob."""
    return _glob_regex(pattern).match(path) is not None


def excluded(exclusions: list[str], path: str) -> bool:
    """Report whether a root-relative path matches an exclusion glob."""
    return any(glob_match(p, path) for p in exclusions)


def facts(language: str, sources, edges, unknown, external: int) -> dict:
    """Build the sorted, de-duplicated output both sides of the test write."""
    return {
        "language": language,
        "side": "new",
        "sources": sorted(set(sources)),
        "edges": [{"from": f, "to": t} for f, t in sorted(set(edges))],
        "unknown": [{"from": f, "specifier": s} for f, s in sorted(set(unknown))],
        "external": external,
    }
