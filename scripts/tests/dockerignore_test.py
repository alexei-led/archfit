#!/usr/bin/env python3
"""Guard the Docker build context.

The image build reads the module, the Go source trees the binary links (including
every //go:embed target), the two provisioning inputs and the pinned tool locks.
Everything else that dominates a working copy is local tool state and must stay
out: it is not a build input, it makes the context layer churn, and it can carry
session artifacts into a layer.
"""

import fnmatch
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

REQUIRED = (
    "Dockerfile",
    "go.mod",
    "go.sum",
    "cmd/archfit/main.go",
    "internal/extract/astgrep/rules/go.yml",
    "internal/extract/py/grimp_helper.py",
    "internal/extract/runtime/async_libs.yaml",
    "internal/extract/scip/scip.proto",
    "internal/extract/scip/scip_reader.py",
    "skills/skills.go",
    "skills/archfit/SKILL.md",
    "tools/manifest.json",
    "tools/node/package-lock.json",
    "tools/python/requirements.lock",
    "scripts/provision-analyzers.sh",
    "scripts/provision_tools.py",
)

EXCLUDED = (
    ".archfit-cache/facts/go/x.json",
    ".book/index.html",
    ".claude/settings.json",
    ".codegraph/graph.db",
    ".git/config",
    ".github/workflows/ci.yaml",
    ".pi-subagents/session.json",
    ".ralphex/state",
    ".revmux/state.json",
    "coverage.out",
    "docs/development.md",
    "examples/.archfit.yaml",
    "skills/.DS_Store",
    "testdata/fixture.go",
)


def patterns():
    entries = []
    for line in (ROOT / ".dockerignore").read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            entries.append(line)
    return entries


def matches(path, pattern, directory):
    if pattern.startswith("**/"):
        parts = path.split("/")
        tail = pattern[3:]
        return any(matches("/".join(parts[index:]), tail, directory) for index in range(len(parts)))
    if "/" not in pattern:
        return any(fnmatch.fnmatchcase(part, pattern) for part in path.split("/"))
    if directory:
        return path == pattern or path.startswith(pattern + "/")
    return fnmatch.fnmatchcase(path, pattern)


def ignored(path):
    return any(
        matches(path, entry.rstrip("/"), entry.endswith("/")) for entry in patterns()
    )


class DockerignoreTest(unittest.TestCase):
    def test_every_build_input_stays_in_the_context(self):
        for path in REQUIRED:
            with self.subTest(path=path):
                self.assertTrue((ROOT / path).is_file(), f"{path} is missing")
                self.assertFalse(ignored(path), f"{path} is excluded from the context")

    def test_go_source_trees_are_never_excluded(self):
        for path in ("cmd", "internal", "skills", "tools", "scripts"):
            with self.subTest(path=path):
                self.assertFalse(ignored(path))

    def test_local_only_trees_stay_out_of_the_context(self):
        for path in EXCLUDED:
            with self.subTest(path=path):
                self.assertTrue(ignored(path), f"{path} would be copied into the context")


if __name__ == "__main__":
    unittest.main()

