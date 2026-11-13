"""Evaluation adapters call only the Archfit 4 CLI surface."""

import json
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import corpus_git as cg
import request_load as rl


class CommandContractTest(unittest.TestCase):
    def test_rules_ownership_and_public_paths(self):
        cases = (
            (
                "src/a/a.go: module a\n  public: src/a\n  allowed dependencies: b, c\n",
                cg.Owner("a", False, ("b", "c"), "src/a/a.go", ("src/a/**",)),
            ),
            (
                "src/a/a.go: declared tests\n  the rules skip these paths\n",
                cg.Owner(None, True),
            ),
            ("src/a/a.go: declared generated\n", cg.Owner(None, True)),
            ("src/a/a.go: in no module\n", cg.Owner(None, False)),
        )
        for output, expected in cases:
            with (
                self.subTest(output=output),
                patch.object(
                    cg.subprocess,
                    "run",
                    return_value=subprocess.CompletedProcess([], 0, output, ""),
                ) as run,
            ):
                resolve = cg.archfit_resolver(
                    "/binary/archfit", "/repo/.archfit.yaml", "/repo"
                )
                owner = resolve(["src/a/a.go"])["src/a/a.go"]
                self.assertEqual(owner, expected)
                self.assertTrue(owner.is_public("src/a/a.go"))
                self.assertEqual(
                    run.call_args.args[0],
                    ["/binary/archfit", "rules", "-C", "/repo", "--", "src/a/a.go"],
                )

    def test_rules_fails_on_usage_errors_or_unrecognized_output(self):
        for code, output in ((2, ""), (0, "unexpected"), (0, "src/a/a.go: module a\n")):
            with (
                self.subTest(code=code, output=output),
                patch.object(
                    cg.subprocess,
                    "run",
                    return_value=subprocess.CompletedProcess([], code, output, "error"),
                ),
            ):
                resolve = cg.archfit_resolver("archfit", "/repo/.archfit.yaml", "/repo")
                with self.assertRaises((RuntimeError, ValueError)):
                    resolve(["src/a/a.go"])
        with self.assertRaisesRegex(ValueError, "repository root"):
            cg.archfit_resolver("archfit", "/candidate.yaml", "/repo")

    def test_seam_measurements_use_report_without_removed_flags(self):
        document = {
            "seams": [
                {"from_module": "a", "to_module": "b", "scores": {"n": 1, "min": 9}}
            ]
        }
        with patch.object(
            rl.subprocess,
            "run",
            return_value=subprocess.CompletedProcess([], 0, json.dumps(document), ""),
        ) as run:
            scores = rl.seam_scores(
                "archfit", Path("/repo/.archfit.yaml"), Path("/repo")
            )
        self.assertEqual(scores[("a", "b")]["min"], 9)
        self.assertEqual(
            run.call_args.args[0],
            ["archfit", "report", "-C", "/repo", "--format", "json"],
        )
        with self.assertRaisesRegex(ValueError, "repository root"):
            rl.seam_scores("archfit", Path("/candidate.yaml"), Path("/repo"))


if __name__ == "__main__":
    unittest.main()
