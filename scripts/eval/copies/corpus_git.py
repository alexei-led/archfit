"""Git history and module mapping shared by the copy-noise and request-load measurements.

A unit of change is one commit on the first-parent line of the default branch, compared with its
first parent: a merge stands for its whole pull request, a squash commit for its pull request, a
direct push for itself. The module of a path comes from `archfit policy where`, so the shipped
policy mapping decides it. Standard library only.
"""

from __future__ import annotations

import fnmatch
import json
import re
import subprocess
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from pathlib import Path, PurePosixPath

LANGUAGES = {
    ".go": "go",
    ".py": "python",
    ".rs": "rust",
    ".ts": "js",
    ".tsx": "js",
    ".mts": "js",
    ".cts": "js",
    ".js": "js",
    ".jsx": "js",
    ".mjs": "js",
    ".cjs": "js",
}
CODE_PATHSPECS = tuple(f"*{ext}" for ext in LANGUAGES)

# Name heuristics. The 4.0 policy excludes only declared paths; these flag what a declared list
# would have to cover, so the measurement can show what the heuristics change.
TEST_DIRS = frozenset(
    {
        "test",
        "tests",
        "__tests__",
        "testdata",
        "fixtures",
        "__fixtures__",
        "__mocks__",
        "mocks",
        "e2e",
        "testutil",
    }
)
TEST_NAME = re.compile(
    r"(_test\.go|_test\.py|^test_.*\.py|^conftest\.py|\.(test|spec|stories)\.[cm]?[jt]sx?)$"
)
GENERATED_DIRS = frozenset(
    {"vendor", "node_modules", "dist", "build", "generated", "__generated__"}
)
GENERATED_NAME = re.compile(
    r"(\.pb\.go|_gen\.go|\.gen\.go|^zz_generated.*|\.generated\.\w+|\.min\.js|_pb2(_grpc)?\.py)$"
)
GENERATED_HEADER = re.compile(
    r"^\s*(//|#)\s*Code generated .*DO NOT EDIT|@generated", re.MULTILINE
)
HEADER_LINES = 30


class GitError(RuntimeError):
    pass


def git(repo: Path | str, *args: str, ok: tuple[int, ...] = (0,)) -> str:
    result = subprocess.run(
        ["git", "-C", str(repo), "-c", "core.quotePath=false", *args],
        capture_output=True,
        text=True,
        errors="replace",
        check=False,
    )
    if result.returncode not in ok:
        raise GitError(
            f"git {' '.join(args[:3])}: exit {result.returncode}: {result.stderr.strip()[:300]}"
        )
    return result.stdout


@dataclass(frozen=True)
class Unit:
    sha: str
    parent: str | None
    timestamp: int
    is_merge: bool
    subject: str


def first_parent_units(
    repo: Path | str, ref: str = "HEAD", since: str | None = None
) -> list[Unit]:
    """Units of change, oldest first."""
    args = ["log", "--first-parent", "--reverse", "--format=%H%x00%P%x00%ct%x00%s"]
    if since:
        args.append(f"--since={since}")
    args.append(ref)
    units = []
    for line in git(repo, *args).split("\n"):
        if not line:
            continue
        sha, parents, ts, subject = line.split("\0", 3)
        parent_list = parents.split()
        units.append(
            Unit(
                sha=sha,
                parent=parent_list[0] if parent_list else None,
                timestamp=int(ts),
                is_merge=len(parent_list) > 1,
                subject=subject,
            )
        )
    return units


def language(path: str) -> str | None:
    return LANGUAGES.get(PurePosixPath(path).suffix)


def is_test_path(path: str) -> bool:
    p = PurePosixPath(path)
    return any(part in TEST_DIRS for part in p.parts[:-1]) or bool(
        TEST_NAME.search(p.name)
    )


def is_generated_path(path: str) -> bool:
    p = PurePosixPath(path)
    return any(part in GENERATED_DIRS for part in p.parts[:-1]) or bool(
        GENERATED_NAME.search(p.name)
    )


def has_generated_header(text: str) -> bool:
    head = "\n".join(text.splitlines()[:HEADER_LINES])
    return bool(GENERATED_HEADER.search(head))


@dataclass(frozen=True)
class Owner:
    module: str | None
    excluded: bool
    depends_on: tuple[str, ...] | None = None  # None: the policy declares no list
    selector: str | None = None  # the name `public` patterns match: package, dotted module, or path
    public: tuple[str, ...] | None = None  # None: the whole module is public

    def is_public(self, path: str) -> bool:
        if self.public is None:
            return True
        return any(fnmatch.fnmatchcase(self.selector or path, pattern) for pattern in self.public)

    @property
    def production(self) -> bool:
        return self.module is not None and not self.excluded


Resolver = Callable[[list[str]], dict[str, Owner]]


class PolicyMap:
    """Module of each path, resolved once and cached. The mapping depends on the path only."""

    def __init__(self, resolve: Resolver) -> None:
        self._resolve = resolve
        self._cache: dict[str, Owner] = {}

    def owners(self, paths: Iterable[str]) -> dict[str, Owner]:
        wanted = list(dict.fromkeys(paths))
        missing = [p for p in wanted if p not in self._cache]
        if missing:
            self._cache.update(self._resolve(missing))
        return {p: self._cache.get(p, Owner(None, False)) for p in wanted}

    def owner(self, path: str) -> Owner:
        return self.owners([path])[path]


def archfit_resolver(
    archfit: str, config: Path | str, root: Path | str, chunk: int = 300
) -> Resolver:
    """Resolve paths with `archfit policy where`, the shipped mapping."""

    def resolve(paths: list[str]) -> dict[str, Owner]:
        out: dict[str, Owner] = {}
        for start in range(0, len(paths), chunk):
            batch = paths[start : start + chunk]
            result = subprocess.run(
                [
                    archfit,
                    "policy",
                    "where",
                    "-c",
                    str(config),
                    "--root",
                    str(root),
                    "--",
                    *batch,
                ],
                capture_output=True,
                text=True,
                check=False,
            )
            if result.returncode != 0:
                raise RuntimeError(
                    f"archfit policy where: exit {result.returncode}: {result.stderr.strip()[:300]}"
                )
            answers = json.loads(result.stdout)["paths"]
            if len(answers) != len(batch):
                raise RuntimeError(
                    f"archfit policy where: {len(answers)} answers for {len(batch)} paths"
                )
            for path, answer in zip(batch, answers):
                deps, public = answer.get("depends_on"), answer.get("public")
                out[path] = Owner(
                    answer.get("module"),
                    bool(answer.get("excluded")),
                    None if deps is None else tuple(deps),
                    answer.get("selector"),
                    None if public is None else tuple(public),
                )
        return out

    return resolve
