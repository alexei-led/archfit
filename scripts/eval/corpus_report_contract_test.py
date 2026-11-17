"""The corpus report consumer must migrate independently of its own constant."""

import json
import tempfile
import unittest
from pathlib import Path

import corpus_sweep as cs


class CorpusReportContractTest(unittest.TestCase):
    def test_sweep_requires_v2_before_check(self):
        for version, accepted in [
            ("archfit.architecture-state.v2", True),
            ("archfit.architecture-state.v1", False),
            ("archfit.result.v1", False),
        ]:
            with (
                self.subTest(version=version),
                tempfile.TemporaryDirectory() as temporary,
            ):
                root = Path(temporary)
                repo = root / "repo"
                repo.mkdir()
                (repo / ".archfit.yaml").write_text("version: 3\n", encoding="utf-8")
                calls = []

                def runner(command, cwd, calls=calls, version=version):
                    name = command[1]
                    calls.append(name)
                    doc = (
                        {"schema_version": version}
                        if name == "report"
                        else {"schema": "archfit.result.v1", "status": "pass"}
                    )
                    return cs.CommandResult(0, json.dumps(doc), "")

                sweep = cs.Sweep(Path("/unused/archfit"), root / "out", runner=runner)
                record = sweep.process(cs.RepoSpec("fixture", repo, "go"))
                self.assertEqual(
                    record["status"], "pass" if accepted else "fail", record
                )
                self.assertEqual(calls, ["report", "check"] if accepted else ["report"])
                if not accepted:
                    self.assertIn(
                        "archfit.architecture-state.v2", record["failures"][0]
                    )

    def test_sweep_validates_current_report_evidence_with_published_schema(self):
        state = json.loads(
            (
                cs.REPO_ROOT / "internal/reportschema/testdata/clean.state.json"
            ).read_text(encoding="utf-8")
        )
        self.assertEqual(state["schema_version"], "archfit.architecture-state.v2")
        self.assertIn("evidence", state)
        result = json.loads(
            (cs.REPO_ROOT / "internal/model/result/testdata/pass.json").read_text(
                encoding="utf-8"
            )
        )
        for missing_evidence in (False, True):
            with (
                self.subTest(missing_evidence=missing_evidence),
                tempfile.TemporaryDirectory() as temporary,
            ):
                root = Path(temporary)
                repo = root / "repo"
                repo.mkdir()
                (repo / ".archfit.yaml").write_text("version: 3\n", encoding="utf-8")
                document = dict(state)
                if missing_evidence:
                    del document["evidence"]
                calls = []

                def runner(command, cwd, calls=calls, document=document):
                    name = command[1]
                    calls.append(name)
                    doc = document if name == "report" else result
                    return cs.CommandResult(0, json.dumps(doc), "")

                sweep = cs.Sweep(
                    Path("/unused/archfit"),
                    root / "out",
                    runner=runner,
                    validate_schema=True,
                )
                record = sweep.process(cs.RepoSpec("fixture", repo, "go"))
                self.assertEqual(
                    record["status"], "fail" if missing_evidence else "pass", record
                )
                self.assertEqual(
                    calls, ["report"] if missing_evidence else ["report", "check"]
                )
