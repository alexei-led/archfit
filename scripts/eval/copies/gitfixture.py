"""Temporary git repositories for the copy and request-load tests."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import corpus_git as cg

ENV = {
    **os.environ,
    "GIT_CONFIG_GLOBAL": os.devnull,
    "GIT_CONFIG_NOSYSTEM": "1",
    "GIT_AUTHOR_NAME": "t",
    "GIT_AUTHOR_EMAIL": "t@example.com",
    "GIT_COMMITTER_NAME": "t",
    "GIT_COMMITTER_EMAIL": "t@example.com",
}


class Repo:
    def __init__(self, root: Path) -> None:
        self.root = root
        self._git("init", "-q", "-b", "main")

    def _git(self, *args: str, date: str | None = None) -> str:
        env = dict(ENV)
        if date:
            env["GIT_AUTHOR_DATE"] = env["GIT_COMMITTER_DATE"] = date
        return subprocess.run(
            [
                "git",
                "-C",
                str(self.root),
                "-c",
                "core.hooksPath=/dev/null",
                "-c",
                "commit.gpgsign=false",
                *args,
            ],
            env=env,
            capture_output=True,
            text=True,
            check=True,
        ).stdout

    def commit(
        self,
        message: str,
        files: dict[str, str | None],
        date: str = "2026-01-01T00:00:00Z",
    ) -> str:
        """Write (or delete, for None) files and commit them. Returns the commit sha."""
        for path, text in files.items():
            target = self.root / path
            if text is None:
                target.unlink()
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text)
        self._git("add", "-A")
        self._git("commit", "-q", "--allow-empty", "-m", message, date=date)
        return self._git("rev-parse", "HEAD").strip()

    def branch(self, name: str) -> None:
        self._git("checkout", "-q", "-b", name)

    def checkout(self, name: str) -> None:
        self._git("checkout", "-q", name)

    def merge(self, name: str, date: str = "2026-01-01T00:00:00Z") -> str:
        self._git("merge", "-q", "--no-ff", "-m", f"Merge {name}", name, date=date)
        return self._git("rev-parse", "HEAD").strip()


def prefix_policy(
    modules: dict[str, str],
    depends_on: dict[str, list[str]] | None = None,
    excluded: tuple[str, ...] = (),
) -> cg.PolicyMap:
    """A stand-in for `archfit policy where`: the module is the one whose path prefix matches."""
    depends_on = depends_on or {}

    def resolve(paths: list[str]) -> dict[str, cg.Owner]:
        out = {}
        for path in paths:
            module = next(
                (m for prefix, m in modules.items() if path.startswith(prefix)), None
            )
            deps = depends_on.get(module) if module else None
            out[path] = cg.Owner(
                module,
                path.startswith(excluded) if excluded else False,
                None if deps is None else tuple(deps),
            )
        return out

    return cg.PolicyMap(resolve)
