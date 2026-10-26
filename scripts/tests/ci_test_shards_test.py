#!/usr/bin/env python3
"""Regression tests for the CI test-shard split; stdlib only, no network."""

import importlib.util
import shutil
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/ci_test_shards.py"
_SPEC = importlib.util.spec_from_file_location("ci_test_shards", SCRIPT)
shards = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(shards)


class CITestShardsTest(unittest.TestCase):
    def test_every_package_lands_in_exactly_one_shard(self):
        packages = self.packages()
        scheduled_in = [name for name in packages if name not in shards.SPLIT]
        for total in (1, 2, 3, 4):
            with self.subTest(total=total):
                split = [shards.shard_of(packages, total, n) for n in range(1, total + 1)]
                scheduled = [name for part in split for name in part]
                self.assertEqual(sorted(scheduled), sorted(scheduled_in))
                self.assertEqual(len(scheduled), len(set(scheduled)))
                self.assertTrue(all(part for part in split))

    def test_measured_long_poles_are_split_apart(self):
        packages = self.packages()
        placed = {
            name: n
            for n in range(1, 4)
            for name in shards.shard_of(packages, 3, n)
            if name in shards.COSTS
        }
        slowest = sorted(shards.COSTS, key=lambda name: -shards.COSTS[name])[:3]
        self.assertEqual(len({placed[name] for name in slowest}), 3)

    def test_packages_split_by_test_name_are_not_scheduled_here(self):
        packages = self.packages()
        self.assertTrue(shards.SPLIT)
        for name in shards.SPLIT:
            self.assertIn(name, packages)
            scheduled = [p for n in range(1, 4) for p in shards.shard_of(packages, 3, n)]
            self.assertNotIn(name, scheduled)

    def test_unlisted_package_is_still_scheduled(self):
        packages = ["example.com/newpkg", "example.com/other"]
        split = [shards.shard_of(packages, 3, n) for n in range(1, 4)]
        self.assertEqual(sorted(name for part in split for name in part), packages)

    def test_shards_stay_balanced_apart_from_one_indivisible_package(self):
        packages = self.packages()
        for total in (3, 4):
            with self.subTest(total=total):
                loads = [
                    sum(shards.cost(name) for name in shards.shard_of(packages, total, n))
                    for n in range(1, total + 1)
                ]
                mean = sum(loads) / total
                # A single slow package cannot be split, so it sets the floor.
                floor = max(shards.cost(name) for name in packages)
                self.assertLessEqual(max(loads), max(floor, mean * 1.12), loads)

    def test_rejects_a_shard_outside_the_total(self):
        for args in (("--shard", "0", "--total", "3"), ("--shard", "4", "--total", "3")):
            with self.subTest(args=args):
                result = subprocess.run(
                    [sys.executable, str(SCRIPT), *args],
                    input="example.com/a\n",
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("--shard", result.stderr)

    @unittest.skipUnless(shutil.which("go"), "go is needed to list real packages")
    def test_cost_table_names_real_packages(self):
        listed = set(self.packages())
        self.assertEqual(sorted(set(shards.COSTS) - listed), [])

    def packages(self):
        result = subprocess.run(
            ["go", "list", "./..."],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
        )
        if result.returncode != 0:
            self.skipTest("go list is unavailable")
        return [line for line in result.stdout.split("\n") if line]


if __name__ == "__main__":
    unittest.main()

