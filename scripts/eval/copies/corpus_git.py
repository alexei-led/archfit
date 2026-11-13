"""Git history and module mapping shared by the copy-noise and request-load measurements.

A unit of change is one commit on the first-parent line of the default branch, compared with its
first parent: a merge stands for its whole pull request, a squash commit for its pull request, a
direct push for itself. The module of a path comes from `archfit rules`, so the shipped
policy mapping decides it. Standard library only.
"""

from __future__ import annotations

import fnmatch
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

# Name heuristics. The 4.0 policy excludes only declared paths; these stand in for what a declared
# list would have to cover. copy_noise records them as flags; request_load skips test importers and
# reports the pairs that leaves out.
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
MAX_BLOB = 1_000_000  # larger files are bundles or data, not code


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


def tree_files(repo: Path | str, rev: str, keep: Callable[[str], bool]) -> list[str]:
    out = git(repo, "ls-tree", "-r", "-z", "--name-only", rev)
    return [p for p in out.split("\0") if p and keep(p)]


def changes(
    repo: Path, unit: Unit, tracked: Callable[[str], bool]
) -> list[tuple[str, str, str | None]]:
    """(status, path, old path) for tracked files; the root commit adds its whole tree."""
    if unit.parent is None:
        return [("A", p, None) for p in tree_files(repo, unit.sha, tracked)]
    out = git(
        repo,
        "diff",
        "--name-status",
        "-z",
        "-M",
        "--no-ext-diff",
        unit.parent,
        unit.sha,
    )
    tokens = out.split("\0")
    result = []
    i = 0
    while i < len(tokens) and tokens[i]:
        status = tokens[i][0]
        if status in "RC":
            old, new = tokens[i + 1], tokens[i + 2]
            i += 3
            if status == "C":
                result.append(("A", new, None))
            elif tracked(old) or tracked(new):
                result.append(("R", new, old))
            continue
        path = tokens[i + 1]
        i += 2
        if tracked(path):
            result.append((status, path, None))
    return result


class BlobReader:
    """One `git cat-file --batch` process for the whole run."""

    def __init__(self, repo: Path) -> None:
        self._proc = subprocess.Popen(
            ["git", "-C", str(repo), "cat-file", "--batch"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
        )

    def read(self, rev: str, path: str) -> str | None:
        assert self._proc.stdin and self._proc.stdout
        self._proc.stdin.write(f"{rev}:{path}\n".encode())
        self._proc.stdin.flush()
        header = self._proc.stdout.readline().decode().split()
        if len(header) < 3 or header[1] != "blob":
            return None
        size = int(header[2])
        data = self._proc.stdout.read(size + 1)[:size]
        return None if size > MAX_BLOB else data.decode("utf-8", errors="replace")

    def close(self) -> None:
        if self._proc.stdin:
            self._proc.stdin.close()
        if self._proc.stdout:
            self._proc.stdout.close()
        self._proc.wait()


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
    selector: str | None = (
        None  # the name `public` patterns match: package, dotted module, or path
    )
    public: tuple[str, ...] | None = None  # None: the whole module is public

    def is_public(self, path: str) -> bool:
        if self.public is None:
            return True
        return any(
            fnmatch.fnmatchcase(self.selector or path, pattern)
            for pattern in self.public
        )

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


def archfit_resolver(archfit: str, config: Path | str, root: Path | str) -> Resolver:
    """Read the policy through rules, one path at a time to avoid grouped headers.

    The 4.0 rules command reads only the repository policy. A different candidate
    must be reviewed in its own checkout, not silently substituted here.
    """
    if Path(config).resolve() != (Path(root) / ".archfit.yaml").resolve():
        raise ValueError("rules reads .archfit.yaml at the repository root")

    def resolve(paths: list[str]) -> dict[str, Owner]:
        out: dict[str, Owner] = {}
        for path in paths:
            result = subprocess.run(
                [archfit, "rules", "-C", str(root), "--", path],
                capture_output=True,
                text=True,
                check=False,
            )
            if result.returncode != 0:
                raise RuntimeError(
                    f"archfit rules: exit {result.returncode}: {result.stderr.strip()[:300]}"
                )
            header, _, details = result.stdout.partition("\n")
            prefix = path + ": "
            if not header.startswith(prefix):
                raise ValueError(f"rules gave no ownership record for {path!r}")
            ownership = header[len(prefix) :]
            if ownership in {"declared tests", "declared generated"}:
                out[path] = Owner(None, True)
                continue
            if ownership == "in no module":
                out[path] = Owner(None, False)
                continue
            if not ownership.startswith("module "):
                raise ValueError(f"unknown rules ownership: {ownership!r}")
            fields = dict(
                line.strip().split(": ", 1)
                for line in details.splitlines()
                if line.startswith(("  public: ", "  allowed dependencies: "))
            )
            if "public" not in fields or "allowed dependencies" not in fields:
                raise ValueError(f"incomplete rules record for {path!r}")
            public = fields["public"]
            public_paths = tuple(public.split(", "))
            if public_paths is not None:
                public_paths = tuple(
                    pattern
                    if any(c in pattern for c in "*?[{")
                    or PurePosixPath(pattern).suffix
                    else pattern.rstrip("/") + "/**"
                    for pattern in public_paths
                )
            deps = fields["allowed dependencies"]
            out[path] = Owner(
                ownership.removeprefix("module "),
                False,
                () if deps == "none" else tuple(deps.split(", ")),
                path,
                public_paths,
            )
        return out

    return resolve
