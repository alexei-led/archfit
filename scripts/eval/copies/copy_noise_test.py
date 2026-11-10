#!/usr/bin/env python3
"""Tests for the copy-noise measurement. Git runs for real in temporary repositories; the policy is faked."""

from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import copy_noise as cn
import corpus_git as cg
from gitfixture import Repo, prefix_policy

# The pumba code the trap agents copied: pkg/util/util.go.
REGEX = "[a-zA-Z][a-zA-Z0-9.:_-]*"
ERROR = "bad network interface name: must match '%s'"
UTIL_GO = f"""package util

import (
\t"fmt"
\t"regexp"
)

var reInterface = regexp.MustCompile(`{REGEX}`)

func ValidateInterfaceName(name string) error {{
\tif name == "" || name != reInterface.FindString(name) {{
\t\treturn fmt.Errorf("{ERROR}", reInterface.String())
\t}}
\treturn nil
}}
"""
SEEDED_TC_GO = f"""package tc

import (
\t"fmt"
\t"regexp"
)

var ifaceName = regexp.MustCompile(`{REGEX}`)

func checkIface(name string) error {{
\tif name != ifaceName.FindString(name) {{
\t\treturn fmt.Errorf("{ERROR}", ifaceName.String())
\t}}
\treturn nil
}}
"""
CHAOS_RUNID = 'package chaos\n\nimport "regexp"\n\nvar runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)\n'
TC_RUNID = 'package tc\n\nimport "regexp"\n\nvar recoveryRunID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,48}$`)\n'
MODULES = {
    "pkg/util/": "pkg_util",
    "pkg/tc/": "pkg_tc",
    "pkg/chaos/": "pkg_chaos",
    "cmd/": "cmd",
}


class LiteralsTest(unittest.TestCase):
    def test_literals_per_language(self):
        cases = [
            ("go", 'x := "a\\"b" + `raw\\d` // "comment"', ['a\\"b', "raw\\d"]),
            ("go", "r := 'x'; s := \"y\"", ["y"]),
            ("go", "s := `unterminated", []),
            ("go", 'a /* "hidden" */ b := "seen"', ["seen"]),
            ("python", 'x = r\'\\d+\' + f"{y}" + b\'z\' + u"w"  # "c"', ["\\d+", "w"]),
            ("python", 'doc = """one line"""', ["one line"]),
            ("python", 'doc = """starts here', []),
            ("python", "name = 'it\\'s'", ["it\\'s"]),
            (
                "js",
                "const a = `x${y}`, b = 'q', c = \"z\", d = `plain` // 'no'",
                ["q", "z", "plain"],
            ),
            (
                "rust",
                'let s = r#"a"b"#; let c = \'x\'; fn f<\'a>(v: &\'a str) { "lit" }',
                ['a"b', "lit"],
            ),
            ("rust", 'let b = b"bytes"; let s = "text";', ["text"]),
        ]
        for lang, line, want in cases:
            with self.subTest(lang=lang, line=line):
                self.assertEqual(cn.literals(line, lang), want)

    def test_import_paths_are_not_literals(self):
        cases = [
            ("go", '\t"regexp"', []),
            ("go", '\tdocker "github.com/docker/docker/client"', []),
            ("go", 'import "fmt"', []),
            ("go", '\t"a long element of a slice",', ["a long element of a slice"]),
            ("js", "import { a } from './util/a'; const b = './util/b'", ["./util/b"]),
            ("js", "const m = require('node:path')", []),
        ]
        for lang, line, want in cases:
            with self.subTest(lang=lang, line=line):
                self.assertEqual(cn.code_literals(line, lang), want)

    def test_shape(self):
        cases = [
            ('json:"name" yaml:"name"', "tag"),
            ("https://example.com/x", "url"),
            ("^[a-z]+$", "regex"),
            ("bad network interface name", "text"),
            ("/var/run/docker.sock", "path"),
            ("container_id", "word"),
            ("%s=%d", "other"),
        ]
        for literal, want in cases:
            with self.subTest(literal=literal):
                self.assertEqual(cn.shape(literal), want)


class AddedLinesTest(unittest.TestCase):
    def test_added_lines_keeps_new_line_numbers(self):
        diff = "\n".join(
            [
                "diff --git a/x.go b/x.go",
                "--- a/x.go",
                "+++ b/x.go",
                "@@ -3,0 +4,2 @@ func f() {",
                '+\ta := "one"',
                '+\tb := "two"',
                "@@ -10 +12 @@",
                '-\tc := "old"',
                '+\tc := "new"',
                "diff --git a/gone.go b/gone.go",
                "--- a/gone.go",
                "+++ /dev/null",
                "@@ -1 +0,0 @@",
                "-package gone",
            ]
        )
        self.assertEqual(
            cn.added_lines(diff),
            {"x.go": [(4, '\ta := "one"'), (5, '\tb := "two"'), (12, '\tc := "new"')]},
        )

    def test_form_feed_does_not_split_a_line(self):
        diff = "+++ b/x.py\n@@ -0,0 +1,2 @@\n+a = 'x\x0cy'\n+b = 'z'\n"
        self.assertEqual(
            cn.added_lines(diff), {"x.py": [(1, "a = 'x\x0cy'"), (2, "b = 'z'")]}
        )


class ScanTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.repo = Repo(Path(self._tmp.name))
        self.repo.commit(
            "base",
            {
                "pkg/util/util.go": UTIL_GO,
                "pkg/tc/tc.go": "package tc\n",
                "pkg/chaos/runid.go": CHAOS_RUNID,
                "cmd/main.go": 'package main\n\nconst ct = "application/json"\n',
                "pkg/chaos/http.go": 'package chaos\n\nconst ct = "application/json"\n',
            },
        )
        self.policy = prefix_policy(MODULES)

    def tearDown(self):
        self._tmp.cleanup()

    def scan(self) -> tuple[list[cn.Event], cn.UnitStats]:
        return cn.scan(self.repo.root, "t", self.policy, since=None)

    def test_seeded_pumba_copy_is_flagged(self):
        self.repo.commit(
            "seed: copy the interface check into pkg/tc",
            {"pkg/tc/iface.go": SEEDED_TC_GO},
        )
        events, _ = self.scan()
        flagged = {(e.literal, e.module, e.other_module) for e in events}
        self.assertEqual(
            flagged, {(REGEX, "pkg_tc", "pkg_util"), (ERROR, "pkg_tc", "pkg_util")}
        )
        regex = next(e for e in events if e.literal == REGEX)
        self.assertEqual(
            (regex.path, regex.line, regex.shape), ("pkg/tc/iface.go", 8, "regex")
        )
        self.assertEqual(regex.other_sites, ["pkg/util/util.go:8"])

    def test_rewritten_copy_is_not_flagged(self):
        self.repo.commit(
            "rewrite the run-id rule in pkg/tc", {"pkg/tc/recovery.go": TC_RUNID}
        )
        events, _ = self.scan()
        self.assertEqual(events, [])

    def test_copy_inside_one_module_is_not_flagged(self):
        self.repo.commit(
            "reuse in util",
            {"pkg/util/more.go": f'package util\n\nconst e = "{ERROR}"\n'},
        )
        events, _ = self.scan()
        self.assertEqual(events, [])

    def test_move_between_modules_is_not_a_copy(self):
        self.repo.commit(
            "move the run-id rule to pkg/tc",
            {
                "pkg/chaos/runid.go": "package chaos\n\nvar keep = 1\n",
                "pkg/tc/tc.go": "package tc\n\nvar runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)\n",
            },
        )
        events, stats = self.scan()
        self.assertEqual(events, [])
        self.assertEqual(stats.moved, 1)

    def test_copy_into_a_test_file_is_flagged_and_marked(self):
        self.repo.commit(
            "test", {"pkg/tc/iface_test.go": f'package tc\n\nvar want = "{ERROR}"\n'}
        )
        events, _ = self.scan()
        self.assertEqual([(e.literal, e.test_added) for e in events], [(ERROR, True)])

    def test_declared_exclusion_removes_the_file(self):
        self.policy = prefix_policy(MODULES, excluded=("pkg/tc/gen/",))
        self.repo.commit(
            "generated", {"pkg/tc/gen/z.go": f'package gen\n\nvar e = "{ERROR}"\n'}
        )
        events, _ = self.scan()
        self.assertEqual(events, [])

    def test_generated_header_is_marked(self):
        header = "// Code generated by tool. DO NOT EDIT.\n"
        self.repo.commit(
            "generated", {"pkg/tc/z.go": f'{header}package tc\n\nvar e = "{ERROR}"\n'}
        )
        events, _ = self.scan()
        self.assertEqual([e.generated_added for e in events], [True])

    def test_short_literal_is_below_the_floor_and_common_literal_counts_holders(self):
        self.repo.commit(
            "add",
            {
                "pkg/tc/web.go": 'package tc\n\nconst ct, x = "application/json", "abc"\n'
            },
        )
        events, _ = self.scan()
        self.assertEqual(
            [(e.literal, e.other_module, e.holder_modules) for e in events],
            [("application/json", "cmd", 2), ("application/json", "pkg_chaos", 2)],
        )

    def test_merge_is_one_unit_compared_with_its_first_parent(self):
        self.repo.branch("feature")
        self.repo.commit("seed on a branch", {"pkg/tc/iface.go": SEEDED_TC_GO})
        self.repo.checkout("main")
        merge = self.repo.merge("feature")
        units = cg.first_parent_units(self.repo.root)
        self.assertEqual([u.is_merge for u in units], [False, True])
        events, stats = self.scan()
        self.assertEqual({e.unit for e in events}, {merge})
        self.assertEqual(stats.merges, 1)


def event(
    id_,
    length=20,
    shape="text",
    holders=1,
    test=False,
    generated=False,
    literal="x",
    module="a",
    other="b",
):
    return {
        "id": id_,
        "length": length,
        "shape": shape,
        "holder_modules": holders,
        "test_added": test,
        "test_other": False,
        "generated_added": generated,
        "generated_other": False,
        "literal": literal,
        "module": module,
        "other_module": other,
    }


class ScoreTest(unittest.TestCase):
    def test_precision_per_rule_uses_only_labelled_events(self):
        events = [
            event("e1", length=30),
            event("e2", length=7, shape="word"),
            event("e3", length=40, test=True),
            event("e4", length=25),
        ]
        labels = {
            "e1": "copy",
            "e2": "noise-convention",
            "e3": "noise-coincidence",
            "e4": "unclear",
        }
        scores = {s.rule: s for s in cn.score(events, labels, [], [])}
        floor = scores["floor: length >= 6"]
        self.assertEqual(
            (floor.flagged, floor.copies, floor.noise, floor.unclear), (4, 1, 2, 1)
        )
        self.assertAlmostEqual(floor.precision, 1 / 3)
        strict = scores["length >= 8 + tests + generated + words"]
        self.assertEqual(
            (strict.flagged, strict.copies, strict.noise, strict.precision),
            (2, 1, 0, 1.0),
        )

    def test_seeded_recall_matches_literal_and_modules(self):
        seeds = [
            {
                "literal": REGEX,
                "module": "pkg_tc",
                "other_module": "pkg_util",
                "kind": "identical",
            },
            {
                "literal": ERROR,
                "module": "pkg_tc",
                "other_module": "pkg_util",
                "kind": "identical",
            },
        ]
        seeded_events = [
            event(
                "s1",
                length=len(REGEX),
                shape="other",
                literal=REGEX,
                module="pkg_tc",
                other="pkg_util",
            )
        ]
        scores = {s.rule: s for s in cn.score([], {}, seeds, seeded_events)}
        self.assertEqual(
            (scores["length >= 24"].seeded_found, scores["length >= 24"].seeded), (1, 2)
        )
        self.assertEqual(scores["length >= 32"].seeded_found, 0)

    def test_unknown_label_is_rejected(self):
        with self.assertRaises(ValueError):
            cn.score([event("e1")], {"e1": "maybe"}, [], [])

    def test_wilson_interval(self):
        low, high = cn.wilson(9, 10)
        self.assertAlmostEqual(low, 0.596, places=3)
        self.assertAlmostEqual(high, 0.982, places=3)


if __name__ == "__main__":
    unittest.main()
