#!/usr/bin/env python3
"""Tests for the request-load measurement. Git runs for real in temporary repositories; the policy is faked."""

from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import corpus_git as cg
import request_load as rl
from gitfixture import Repo, prefix_policy


def go(pkg: str, *imports: str) -> str:
    block = "".join(f'\t"example.com/m/{i}"\n' for i in imports)
    return f'package {pkg}\n\nimport (\n\t"fmt"\n{block})\n'


class ReadImportsTest(unittest.TestCase):
    def test_read_imports(self):
        cases = [
            (
                "go",
                'package x\nimport (\n\t"fmt"\n\tc "example.com/m/c"\n)\nimport "os"\n',
                ["fmt", "example.com/m/c", "os"],
            ),
            (
                "python",
                "import a.b as c, d\nfrom ..x.y import (z,\n    w)\nfrom . import q\nfrom .. import v  # c\nfrom ..m import n\n",
                [
                    rl.PyImport(0, "a.b", ()),
                    rl.PyImport(0, "d", ()),
                    rl.PyImport(2, "x.y", ("z", "w")),
                    rl.PyImport(1, "", ("q",)),
                    rl.PyImport(2, "", ("v",)),
                    rl.PyImport(2, "m", ("n",)),
                ],
            ),
            (
                "js",
                "import {a} from './a';\nexport * from '../b'\nrequire('./c'); import('lodash')",
                ["./a", "../b", "./c", "lodash"],
            ),
        ]
        for lang, text, want in cases:
            with self.subTest(lang=lang):
                self.assertEqual(rl.read_imports(lang, text), want)


class ResolveTest(unittest.TestCase):
    def setUp(self):
        self.tree = rl.Tree()
        self.tree.files = {
            "go.mod",
            "pkg/a/a.go",
            "pkg/a/a_test.go",
            "libs/foo/go.mod",
            "libs/foo/foo.go",
            "src/app/__init__.py",
            "src/app/core/__init__.py",
            "src/app/core/rules.py",
            "src/app/core/store.py",
            "src/app/web.py",
            "web/util/index.ts",
            "web/util/date.ts",
            "web/main.ts",
        }
        self.tree.go_mods = {"example.com/m": "", "example.com/foo": "libs/foo"}

    def test_resolve(self):
        web, rules = "src/app/web.py", "src/app/core/rules.py"
        cases = [
            ("go", "cmd/x.go", "example.com/m/pkg/a", ["pkg/a/a.go"]),
            ("go", "cmd/x.go", "example.com/foo", ["libs/foo/foo.go"]),
            ("go", "cmd/x.go", "example.com/other/pkg", []),
            ("go", "cmd/x.go", "fmt", []),
            ("python", web, rl.PyImport(0, "app.core.rules", ()), [rules]),
            ("python", web, rl.PyImport(0, "app.core", ("rules",)), [rules]),
            ("python", web, rl.PyImport(0, "app.core", ("rules", "store")), [rules, "src/app/core/store.py"]),
            ("python", web, rl.PyImport(0, "app.core", ("Rule",)), ["src/app/core/__init__.py"]),
            ("python", web, rl.PyImport(0, "app.core", ("Rule", "rules")), [rules, "src/app/core/__init__.py"]),
            ("python", rules, rl.PyImport(2, "web", ()), [web]),
            ("python", rules, rl.PyImport(1, "", ("rules",)), [rules]),
            ("python", web, rl.PyImport(0, "os.path", ()), []),
            ("js", "web/main.ts", "./util", ["web/util/index.ts"]),
            ("js", "web/main.ts", "./util/date.js", ["web/util/date.ts"]),
            ("js", "web/main.ts", "@/util/date", []),
        ]
        for lang, path, spec, want in cases:
            with self.subTest(spec=spec):
                self.assertEqual(self.tree.resolve(path, lang, spec), want)


class TimelineTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.repo = Repo(Path(self._tmp.name))
        self.policy = prefix_policy(
            {"a/": "A", "b/": "B", "c/": "C"}, depends_on={"A": ["B"]}
        )

    def tearDown(self):
        self._tmp.cleanup()

    def history(self) -> list[str]:
        r = self.repo
        return [
            r.commit(
                "init",
                {
                    "go.mod": "module example.com/m\n",
                    "a/a.go": go("a"),
                    "b/b.go": go("b"),
                },
                date="2024-12-15T00:00:00Z",
            ),
            r.commit("a uses b", {"a/a.go": go("a", "b")}, date="2025-03-01T00:00:00Z"),
            r.commit("drop it", {"a/a.go": go("a")}, date="2025-05-01T00:00:00Z"),
            r.commit(
                "a uses b again", {"a/a.go": go("a", "b")}, date="2025-07-01T00:00:00Z"
            ),
            r.commit(
                "new module c uses b",
                {"c/c.go": go("c", "b")},
                date="2025-09-01T00:00:00Z",
            ),
            r.commit(
                "b uses c; a test uses c",
                {"b/b.go": go("b", "c"), "a/a_test.go": go("a", "c")},
                date="2025-11-01T00:00:00Z",
            ),
            r.commit(
                "inside one module",
                {"a/x/x.go": go("x", "a")},
                date="2026-01-01T00:00:00Z",
            ),
        ]

    def test_pair_timeline(self):
        shas = self.history()
        report = rl.pairs_report(self.repo.root, "t", self.policy)
        pairs = {(p["from"], p["to"]): p for p in report["pairs"]}
        self.assertEqual(sorted(pairs), [("A", "B"), ("B", "C"), ("C", "B")])
        ab = pairs[("A", "B")]
        self.assertEqual(
            (ab["first_sha"], ab["intervals"], ab["in_depends_on"]),
            (shas[1], [[1, 2], [3, None]], True),
        )
        self.assertTrue(pairs[("C", "B")]["with_new_module"])
        self.assertFalse(pairs[("B", "C")]["with_new_module"])
        self.assertEqual(report["missing_from_depends_on"], {"pairs": 2, "commits": 2})
        self.assertEqual(report["test_only_pairs_at_head"], ["A->C"])
        self.assertEqual(report["first_appearances_after_root"]["existing_modules"], 2)

    def test_freeze_counts_pairs_that_appear_after_the_policy(self):
        shas = self.history()
        six = rl.pairs_report(self.repo.root, "t", self.policy)["freeze"]["6m"]
        self.assertEqual(six["policy_at"], shas[3])
        self.assertEqual(
            [(r["from"], r["to"]) for r in six["requests_existing_modules"]], []
        )
        self.assertEqual(
            [(r["from"], r["to"]) for r in six["requests_new_module"]],
            [("B", "C"), ("C", "B")],
        )
        self.assertEqual(
            (six["new_modules"], six["units_with_new_module_pairs"]), (["C"], 2)
        )
        twelve = rl.pairs_report(self.repo.root, "t", self.policy)["freeze"]["12m"]
        self.assertEqual(twelve["policy_at"], shas[0])
        self.assertEqual(
            [(r["from"], r["to"]) for r in twelve["requests_existing_modules"]],
            [("A", "B")],
        )

    def test_rename_moves_the_pair_with_the_file(self):
        r = self.repo
        r.commit(
            "init",
            {
                "go.mod": "module example.com/m\n",
                "b/b.go": go("b"),
                "c/c.go": go("c", "b"),
            },
        )
        r.commit("rename", {"c/c.go": None, "a/c.go": go("c", "b")})
        report = rl.pairs_report(r.root, "t", self.policy)
        rows = {(p["from"], p["to"]): p["intervals"] for p in report["pairs"]}
        self.assertEqual(rows, {("C", "B"): [[0, 1]], ("A", "B"): [[1, None]]})

    def test_units_follow_the_first_parent(self):
        r = self.repo
        r.commit(
            "init",
            {"go.mod": "module example.com/m\n", "a/a.go": go("a"), "b/b.go": go("b")},
        )
        r.branch("f")
        r.commit("a uses b", {"a/a.go": go("a", "b")})
        r.checkout("main")
        merge = r.merge("f")
        report = rl.pairs_report(r.root, "t", self.policy)
        self.assertEqual((report["units"], report["merges"]), (2, 1))
        self.assertEqual(report["pairs"][0]["first_sha"], merge)


class CouplingTest(unittest.TestCase):
    def test_declared_volatility_with_defaults(self):
        policy = """version: 2
# a comment at column 0
modules:
  core_mod:
    paths: [a/**]
    subdomain: core
  steady:  # stable
    volatility: low
  helper:
    subdomain: generic
  plain:
    paths:
      - p/**
rules: []
"""
        self.assertEqual(
            rl.declared_volatility(policy),
            {"core_mod": "high", "steady": "low", "helper": "low", "plain": "high"},
        )

    def test_coupling_4_table(self):
        cases = [
            (False, "high", "high", ("pass", "9 or 7")),
            (False, "high", "low", ("pass", "9 or 8")),
            (True, "low", "low", ("pass", "8")),
            (True, "low", "medium", ("fail", "5")),
            (True, "medium", "high", ("fail", "2")),
        ]
        for past, vf, vt, want in cases:
            with self.subTest(past=past, vf=vf, vt=vt):
                self.assertEqual(rl.coupling_4(past, vf, vt), want)

    def test_use_past_the_public_part_is_recorded(self):
        with tempfile.TemporaryDirectory() as tmp:
            r = Repo(Path(tmp))
            r.commit(
                "init",
                {
                    "go.mod": "module example.com/m\n",
                    "a/a.go": go("a"),
                    "b/b.go": go("b"),
                    "b/inner/i.go": go("inner"),
                },
            )
            r.commit("public use", {"a/a.go": go("a", "b")})
            r.commit("past public", {"c/c.go": go("c", "b/inner")})
            policy = prefix_policy(
                {"a/": "A", "b/": "B", "c/": "C"}, public={"B": ("b",)}
            )
            report = rl.pairs_report(
                r.root, "t", policy, {"A": "high", "B": "high", "C": "low"}
            )
        rows = {
            (p["from"], p["to"]): (p["past_public_first"], p["coupling_4_first"])
            for p in report["pairs"]
        }
        self.assertEqual(
            rows, {("A", "B"): (False, "pass"), ("C", "B"): (True, "fail")}
        )


class DistributionTest(unittest.TestCase):
    def test_distribution_against_the_minimum(self):
        rows = [{"from": "a", "to": f"t{i}"} for i in range(5)]
        scores = {
            ("a", "t0"): {"n": 3, "min": 9},
            ("a", "t1"): {"n": 1, "min": 7},
            ("a", "t2"): {"n": 2, "min": 5},
            ("a", "t3"): {"n": 0, "min": 0},
        }
        got = rl.distribution(rows, scores)
        self.assertEqual(
            {
                k: got[k]
                for k in (
                    "pairs",
                    "scored",
                    "pass",
                    "below",
                    "abstained",
                    "not_measured",
                )
            },
            {
                "pairs": 5,
                "scored": 3,
                "pass": 2,
                "below": 1,
                "abstained": 1,
                "not_measured": 1,
            },
        )
        self.assertEqual(got["histogram"], {5: 1, 7: 1, 9: 1})


class UnitsTest(unittest.TestCase):
    def test_month_index_needs_history_that_reaches_back(self):
        units = [
            cg.Unit(str(i), None, ts, False, "")
            for i, ts in enumerate((0, 100 * 86400, 200 * 86400))
        ]
        self.assertEqual(rl.month_index(units, 3), 1)
        self.assertIsNone(rl.month_index(units, 12))


if __name__ == "__main__":
    unittest.main()
