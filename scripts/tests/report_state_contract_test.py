"""Report readers accept the current strict state identifier, never result JSON."""

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image


class ReportStateContractTest(unittest.TestCase):
    def test_read_state_accepts_only_a_v2_object(self):
        cases = [
            ("current", '{"schema_version":"archfit.architecture-state.v2"}', True),
            ("retired", '{"schema_version":"archfit.architecture-state.v1"}', False),
            ("check result", '{"schema":"archfit.result.v1"}', False),
            ("missing identifier", "{}", False),
            ("array", "[]", False),
            ("null", "null", False),
            ("invalid JSON", "{", False),
        ]
        for name, raw, valid in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                output = Path(temporary)
                (output / "report.json").write_text(raw, encoding="utf-8")
                contract = image.ImageContract("unused", "unused", output)
                state = contract.read_state(output, "report")
                if valid:
                    self.assertEqual(state, json.loads(raw))
                    self.assertEqual(contract.assertions, [])
                else:
                    self.assertEqual(state, {})
                    self.assertEqual(len(contract.assertions), 1)
                    self.assertFalse(contract.assertions[0]["passed"])

    def test_read_state_rejects_missing_report(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            contract = image.ImageContract("unused", "unused", output)
            self.assertEqual(contract.read_state(output, "missing"), {})
            self.assertFalse(contract.assertions[0]["passed"])
