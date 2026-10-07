#!/usr/bin/env python3
"""Unit tests for the seam census. No test executes the archfit binary."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import seam_census as sc


def seam(
    frm,
    to,
    severity,
    qualifying=False,
    strength="functional",
    distance="cross_module",
    hypothesis="x",
    confidence="high",
):
    return {
        "from_module": frm,
        "to_module": to,
        "severity": severity,
        "distributed_monolith": qualifying,
        "strength": strength,
        "distance": distance,
        "hypothesis": hypothesis,
        "confidence": confidence,
    }


def state(seams, scored=10, abstained=0):
    return {
        "seams": seams,
        "dimensions": {
            "coupling": {
                "status": "measured",
                "metrics": [
                    {"name": "scored_edges", "value": scored},
                    {"name": "abstained_edges", "value": abstained},
                ],
            }
        },
    }


class SummarizeTest(unittest.TestCase):
    def test_counts_severity_and_qualifying(self):
        s = state(
            [
                seam("a", "b", "critical", True),
                seam("a", "c", "critical"),
                seam("b", "c", "none", confidence="unrated"),
            ]
        )
        got = sc.summarize(s)
        self.assertEqual(got["by_severity"]["critical"], 2)
        self.assertEqual(got["qualifying"], 1)
        self.assertEqual(got["unrated"], 1)
        self.assertEqual(got["seams"], 3)

    def test_no_seams_is_all_zero(self):
        got = sc.summarize({"dimensions": {}})
        self.assertEqual(got["seams"], 0)
        self.assertEqual(got["coupling_status"], "missing")

    def test_qualifying_pairs_sorted(self):
        s = state(
            [
                seam("z", "a", "critical", True),
                seam("a", "b", "critical", True),
                seam("c", "d", "low"),
            ]
        )
        self.assertEqual(sc.qualifying_pairs(s), ["a -> b", "z -> a"])

    def test_abstention_rate(self):
        row = sc.summarize(state([], scored=3, abstained=1))
        self.assertAlmostEqual(sc.abstention_rate(row), 0.25)
        self.assertEqual(sc.abstention_rate(sc.summarize(state([], 0, 0))), 0.0)


class ResolveBinaryTest(unittest.TestCase):
    def test_relative_path_becomes_absolute_from_caller_cwd(self):
        got = sc.resolve_binary(".bin/archfit")
        self.assertTrue(Path(got).is_absolute())
        self.assertEqual(got, str(Path(".bin/archfit").resolve()))


class ParseReposTest(unittest.TestCase):
    def test_accepts_label_dir(self):
        self.assertEqual(sc.parse_repos(["a=/x", "b=/y"]), {"a": "/x", "b": "/y"})

    def test_rejects_malformed(self):
        for bad in ("nolabel", "=dir", "label="):
            with self.assertRaises(ValueError):
                sc.parse_repos([bad])


if __name__ == "__main__":
    unittest.main()
