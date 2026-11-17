"""Current report component origins are not a per-seam scalar."""

import json
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import request_load as rl


class ReportProjectionTest(unittest.TestCase):
    def test_seam_scores_does_not_fabricate_component_provenance(self):
        state = {
            "schema_version": "archfit.architecture-state.v2",
            "evidence": {
                "components": [
                    {
                        "name": "source",
                        "volatility": {
                            "raw": None,
                            "effective": "high",
                            "origin": "assumed",
                            "derivation": "default high",
                            "refs": [],
                        },
                    },
                    {
                        "name": "target",
                        "volatility": {
                            "raw": "low",
                            "effective": "low",
                            "origin": "asserted",
                            "derivation": "",
                            "refs": [],
                        },
                    },
                ]
            },
            "seams": [
                {
                    "from_module": "source",
                    "to_module": "target",
                    "scores": {"n": 3, "min": 4, "median": 7},
                    "strength": "functional",
                    "volatility": "high",
                    "edges": 3,
                }
            ],
        }
        with patch.object(
            rl.subprocess,
            "run",
            return_value=subprocess.CompletedProcess([], 0, json.dumps(state), ""),
        ):
            scores = rl.seam_scores(
                "archfit", Path("/repo/.archfit.yaml"), Path("/repo")
            )
            diagnostic = rl.balance_report(
                {
                    "repo": "/repo",
                    "pairs": [{"from": "source", "to": "target", "at_head": True}],
                },
                Path("/repo"),
                "archfit",
                Path("/repo/.archfit.yaml"),
                cap=0,
            )
        self.assertEqual(
            scores,
            {
                ("source", "target"): {
                    "n": 3,
                    "min": 4,
                    "median": 7,
                    "strength": "functional",
                    "volatility": "high",
                    "edges": 3,
                }
            },
        )
        self.assertNotIn("volatility_provenance", scores[("source", "target")])
        self.assertEqual(
            diagnostic["scores"],
            [{"from": "source", "to": "target", **scores[("source", "target")]}],
        )
        self.assertEqual(diagnostic["reader_vs_archfit_at_head"]["both"], 1)
