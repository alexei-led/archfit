#!/usr/bin/env python3
"""Unit tests for the v1 corpus sweep harness.

The harness is the product-level acceptance gate, so its own checks have to be
provably non-vacuous: every validator below is exercised with an input it must
REJECT as well as one it must accept. A gate nobody has watched fail is a gate
nobody knows still works.

Standard library only, and no test here executes the archfit binary — the
command runner is injected.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import corpus_sweep as cs


def state_doc(**overrides):
    """A minimal valid architecture-state document."""
    dims = {}
    for i, name in enumerate(cs.DIMENSION_KEYS):
        status = "measured" if i < 5 else ("partial" if i < 8 else "unmeasured")
        unknown = []
        if status != "measured":
            unknown.append(
                {
                    "fact": f"required {name} evidence",
                    "reason": "fixture evidence is incomplete",
                    "owner": f"owner/{name}",
                }
            )
        dims[name] = {
            "name": name,
            "owner": f"owner/{name}",
            "status": status,
            "confidence": "high",
            "gate": "pass",
            "coverage": {"basis": "b", "observed": 1, "total": 1},
            "metrics": [{"name": "m", "value": 1.0, "unit": "count"}],
            "findings": [],
            "unknown": unknown,
        }
    doc = {
        "schema_version": cs.STATE_SCHEMA_VERSION,
        "verdict": "needs_attention",
        "decision": {
            "hard_gates": "pass",
            "active_blockers": 0,
            "attention_dimensions": 1,
        },
        "comparison": {"status": "not_requested", "reasons": []},
        "measurement": {"source_ref": "worktree"},
        "dimensions": dims,
        "coverage": {"measured": 5, "partial": 3, "unmeasured": 1, "tools": []},
        "findings": [
            {"id": "aaa", "rule_id": "r1", "status": "new"},
            {"id": "bbb", "rule_id": "r2", "status": "accepted"},
        ],
        "agent_tasks": [],
        "seams": [],
    }
    doc.update(overrides)
    return doc


class TestFlagParsing(unittest.TestCase):
    def test_csv_list(self):
        for raw, want in (
            (None, []),
            ("", []),
            ("a", ["a"]),
            (" a , b ,, c ", ["a", "b", "c"]),
        ):
            with self.subTest(raw=raw):
                self.assertEqual(cs.parse_csv_list(raw), want)

    def test_allow_unverified_needs_a_reason(self):
        self.assertEqual(
            cs.parse_allow_unverified(["tokio=not cloned on this host"]),
            {"tokio": "not cloned on this host"},
        )
        for bad in ("tokio", "tokio=", "=reason", "  =  "):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                cs.parse_allow_unverified([bad])

    def test_migration_only_defaults_on_at_the_argparse_layer(self):
        # The bug lived in the argparse-to-Sweep seam: Sweep's own default was
        # already True while the flag was store_true, so a flagless sweep ran
        # the full `config update --apply` that docs/test-corpus.md forbids for
        # a schema migration. Assert the parser, not the Sweep constructor.
        self.assertTrue(cs.build_parser().parse_args([]).migration_only)
        self.assertTrue(cs.build_parser().parse_args(["--migration-only"]).migration_only)
        self.assertFalse(
            cs.build_parser().parse_args(["--no-migration-only"]).migration_only
        )

    def test_config_version(self):
        self.assertEqual(cs.config_version("version: 2\nmodules: {}\n"), 2)
        self.assertEqual(cs.config_version("# version: 2\nmodules: {}\n"), None)
        self.assertEqual(cs.config_version("modules: {}\n"), None)

    def test_command_environment_pins_rust_without_overriding_owner_choice(self):
        self.assertEqual(
            cs.command_environment({})["RUSTUP_TOOLCHAIN"],
            cs.PINNED_RUST_TOOLCHAIN,
        )
        self.assertEqual(
            cs.command_environment({"RUSTUP_TOOLCHAIN": "nightly"})[
                "RUSTUP_TOOLCHAIN"
            ],
            "nightly",
        )


class TestRecordShape(unittest.TestCase):
    def test_blank_record_is_the_frozen_shape(self):
        record = cs.blank_record("spotinfo", "/r", "go")
        self.assertEqual(
            sorted(record),
            [
                "ai",
                "analyze",
                "check",
                "config",
                "determinism",
                "failures",
                "formats",
                "label",
                "language",
                "root",
                "status",
                "unverified",
            ],
        )
        self.assertEqual(
            sorted(record["config"]),
            [
                "candidate_sha256",
                "second_update_changed",
                "second_update_exit",
                "source",
                "source_config_sha256",
                "target_head",
                "update_exit",
                "version",
            ],
        )
        self.assertEqual(
            sorted(record["analyze"]),
            ["dimension_keys", "exit", "schema_version", "verdict"],
        )
        self.assertEqual(sorted(record["check"]), ["exit", "verdict"])
        self.assertEqual(sorted(record["formats"]), sorted(cs.FORMATS))
        self.assertEqual(sorted(record["determinism"]), ["json_byte_identical"])
        self.assertEqual(sorted(record["ai"]), ["exit", "requested"])
        # Every absent value is explicit, and the record round-trips as JSON.
        self.assertIsNone(record["unverified"])
        self.assertEqual(record["failures"], [])
        json.dumps(record)


class TestStateValidation(unittest.TestCase):
    def test_a_valid_state_has_no_failures(self):
        self.assertEqual(cs.validate_state(state_doc()), [])

    def test_rejects_wrong_schema_version(self):
        failures = cs.validate_state(state_doc(schema_version="archfit.diagnostic.v2"))
        self.assertTrue(any("schema_version" in f for f in failures), failures)

    def test_rejects_a_returned_repository_scalar(self):
        failures = cs.validate_state(state_doc(score={"overall": 61}))
        self.assertTrue(any("repository scalar" in f for f in failures), failures)

    def test_rejects_a_missing_dimension(self):
        doc = state_doc()
        del doc["dimensions"]["drift"]
        failures = cs.validate_state(doc)
        self.assertTrue(any("dimension keys" in f for f in failures), failures)

    def test_rejects_reordered_dimensions(self):
        doc = state_doc()
        dims = doc["dimensions"]
        doc["dimensions"] = {k: dims[k] for k in reversed(list(dims))}
        failures = cs.validate_state(doc)
        self.assertTrue(any("dimension keys" in f for f in failures), failures)

    def test_rejects_coverage_that_does_not_sum_to_nine(self):
        doc = state_doc()
        doc["coverage"] = {"measured": 5, "partial": 3, "unmeasured": 0}
        failures = cs.validate_state(doc)
        self.assertTrue(any("sum to" in f for f in failures), failures)

    def test_rejects_coverage_that_contradicts_the_dimensions(self):
        doc = state_doc()
        doc["coverage"] = {"measured": 9, "partial": 0, "unmeasured": 0}
        failures = cs.validate_state(doc)
        self.assertTrue(
            any("contradict" in f or "but the dimensions" in f for f in failures),
            failures,
        )

    def test_rejects_an_untyped_metric(self):
        doc = state_doc()
        doc["dimensions"]["intent"]["metrics"] = [{"name": "m", "value": 1}]
        failures = cs.validate_state(doc)
        self.assertTrue(any("missing" in f for f in failures), failures)

    def test_rejects_metrics_as_an_object(self):
        doc = state_doc()
        doc["dimensions"]["intent"]["metrics"] = {"m": 1}
        failures = cs.validate_state(doc)
        self.assertTrue(any("not an array" in f for f in failures), failures)

    def test_rejects_a_dimension_without_an_owner(self):
        doc = state_doc()
        doc["dimensions"]["intent"]["owner"] = ""
        failures = cs.validate_state(doc)
        self.assertTrue(any("evidence owner" in f for f in failures), failures)

    def test_measured_allows_a_declared_out_of_claim_unknown(self):
        doc = state_doc()
        doc["dimensions"]["intent"]["unknown"] = [
            {
                "fact": "disabled rule conformance",
                "reason": "the rule is disabled",
                "owner": "owner/intent",
            }
        ]
        self.assertEqual(cs.validate_state(doc), [])

    def test_rejects_measured_with_an_in_claim_unknown(self):
        doc = state_doc()
        doc["dimensions"]["intent"]["unknown"] = [
            {
                "fact": "declared intent inventory",
                "reason": "the required fact is missing",
                "owner": "owner/intent",
            }
        ]
        failures = cs.validate_state(doc)
        self.assertTrue(any("in-claim" in f for f in failures), failures)

    def test_rejects_partial_without_a_named_unknown(self):
        doc = state_doc()
        doc["dimensions"]["complexity"]["unknown"] = []
        failures = cs.validate_state(doc)
        self.assertTrue(any("partial without" in f for f in failures), failures)


class TestExitContract(unittest.TestCase):
    def test_verdict_maps_to_the_frozen_exit(self):
        self.assertEqual(cs.expected_check_exit("healthy"), 0)
        self.assertEqual(cs.expected_check_exit("blocked"), 1)
        self.assertEqual(cs.expected_check_exit("needs_attention"), 2)
        self.assertIsNone(cs.expected_check_exit("mixed"))
        self.assertIsNone(cs.expected_check_exit(None))


class TestFormatParity(unittest.TestCase):
    def rendered(self, ids=("aaa", "bbb")):
        state = state_doc()
        body = [
            "ARCHITECTURE STATE",
            "VERDICT    NEEDS ATTENTION",
            "BLOCKING   0 active · hard gates: pass",
            "COVERAGE   5 measured · 3 partial · 1 unmeasured (of 9)",
        ]
        for name, dim in state["dimensions"].items():
            body.append(f"  {name}  {dim['status']}  gate: {dim['gate']}")
        body.append("COMPARISON")
        body.append("  status: not_requested")
        body.append("FINDING INDEX")
        body.extend(f"  {fid}  new  r" for fid in ids)
        return state, "\n".join(body) + "\n"

    def test_a_parity_respecting_render_passes(self):
        state, out = self.rendered()
        self.assertEqual(cs.check_text_parity("text", out, state), [])

    def test_a_truncated_finding_list_fails_parity(self):
        state, out = self.rendered(ids=("aaa",))
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("bbb" in f for f in failures), failures)

    def test_an_out_of_order_finding_index_fails_parity(self):
        state, out = self.rendered(ids=("bbb", "aaa"))
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("canonical order" in f for f in failures), failures)

    def test_a_disagreeing_verdict_fails_parity(self):
        state, out = self.rendered()
        out = out.replace("NEEDS ATTENTION", "HEALTHY")
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("verdict" in f for f in failures), failures)

    def test_a_disagreeing_coverage_split_fails_parity(self):
        state, out = self.rendered()
        out = out.replace(
            "5 measured · 3 partial · 1 unmeasured",
            "9 measured · 0 partial · 0 unmeasured",
        )
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("coverage split" in f for f in failures), failures)

    def test_a_missing_dimension_fails_parity(self):
        state, out = self.rendered()
        out = out.replace("  testability  partial  gate: pass\n", "")
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("testability" in f for f in failures), failures)

    def test_an_actionable_excerpt_before_the_index_is_allowed(self):
        # A human format may lead with a severity-sorted excerpt. Only the
        # closing appendix has to be in canonical order.
        state, out = self.rendered()
        out = out.replace(
            "FINDING INDEX", "TOP ACTIONABLE\n  bbb\n  aaa\n\nFINDING INDEX"
        )
        self.assertEqual(cs.check_text_parity("text", out, state), [])

    def test_a_dimension_row_cannot_supply_the_coverage_triple(self):
        # The old check looked for the three counts as substrings in order on
        # any line mentioning "measured", so a dimension denominator satisfied
        # it: "63/70" supplies a 6, a 3, and a 0. Parse the labels instead.
        self.assertIsNone(
            cs.rendered_coverage_triple(
                "  structure  measured  gate: pass  packages 63/70"
            )
        )
        self.assertEqual(
            cs.rendered_coverage_triple("COVERAGE 5 measured · 3 partial · 1 unmeasured"),
            (5, 3, 1),
        )

    def test_a_wrong_dimension_gate_fails_parity(self):
        state, out = self.rendered()
        out = out.replace("  coupling  measured  gate: pass", "  coupling  measured  gate: fail")
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("coupling" in f and "gate" in f for f in failures), failures)

    def test_a_status_borrowed_from_another_row_fails_parity(self):
        # Every status word appears in the coverage headline, so a
        # document-global substring test could never fail. Drop one row's own
        # status and the check must still fire.
        state, out = self.rendered()
        out = out.replace("  coupling  measured  gate: pass", "  coupling  gate: pass")
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("coupling" in f and "status" in f for f in failures), failures)

    def test_bare_numbers_are_not_a_coverage_triple(self):
        self.assertIsNone(cs.rendered_coverage_triple("total 5 3 1"))

    def test_the_opposite_comparison_status_fails_parity(self):
        # "comparable" is a substring of "non_comparable", so a containment
        # test passed while the renderer printed the OPPOSITE status.
        state, out = self.rendered()
        state["comparison"]["status"] = "comparable"
        out = out.replace("status: not_requested", "status: non_comparable")
        failures = cs.check_text_parity("text", out, state)
        self.assertTrue(any("comparison status" in f for f in failures), failures)

    def test_the_matching_comparison_status_passes_parity(self):
        state, out = self.rendered()
        state["comparison"]["status"] = "non_comparable"
        out = out.replace("status: not_requested", "status: non_comparable")
        self.assertEqual(cs.check_text_parity("text", out, state), [])


class TestSarifParity(unittest.TestCase):
    def sarif(self, **over):
        state = state_doc()
        log = {
            "runs": [
                {
                    "properties": {
                        "verdict": state["verdict"],
                        "decision": {"hard_gates": "pass", "active_blockers": 0},
                        "dimensions": [{"name": n} for n in cs.DIMENSION_KEYS],
                        "coverage": {"measured": 5, "partial": 3, "unmeasured": 1},
                    },
                    "results": [
                        {"ruleId": "r1", "fingerprints": {"archfit/v1": "aaa"}},
                        {"ruleId": "r2", "fingerprints": {"archfit/v1": "bbb"}},
                    ],
                }
            ]
        }
        log["runs"][0]["properties"].update(over)
        return state, json.dumps(log)

    def test_matching_sarif_passes(self):
        state, raw = self.sarif()
        self.assertEqual(cs.check_sarif_parity(raw, state), [])

    def test_disagreeing_verdict_fails(self):
        state, raw = self.sarif(verdict="healthy")
        self.assertTrue(
            any("verdict" in f for f in cs.check_sarif_parity(raw, state)), raw
        )

    def test_missing_fingerprint_fails(self):
        state, raw = self.sarif()
        log = json.loads(raw)
        log["runs"][0]["results"] = log["runs"][0]["results"][:1]
        failures = cs.check_sarif_parity(json.dumps(log), state)
        self.assertTrue(any("fingerprint" in f for f in failures), failures)

    def test_invalid_sarif_fails(self):
        state, _ = self.sarif()
        self.assertTrue(cs.check_sarif_parity("not json", state))


class TestStatusResolution(unittest.TestCase):
    def test_a_clean_record_passes(self):
        record = cs.blank_record("spotinfo", "/r", "go")
        record["status"] = cs.STATUS_FAIL
        self.assertEqual(cs.finalize_status(record, {})["status"], cs.STATUS_PASS)

    def test_a_failing_record_fails(self):
        record = cs.blank_record("spotinfo", "/r", "go")
        record["status"] = cs.STATUS_FAIL
        record["failures"].append("analyze --json exited 3")
        self.assertEqual(cs.finalize_status(record, {})["status"], cs.STATUS_FAIL)

    def test_a_missing_repo_stays_unverified_without_an_allowance(self):
        record = cs.blank_record("tokio", "/r", "rust")
        self.assertEqual(cs.finalize_status(record, {})["status"], cs.STATUS_UNVERIFIED)

    def test_an_allowed_gap_is_accepted_but_never_a_pass(self):
        record = cs.blank_record("tokio", "/r", "rust")
        out = cs.finalize_status(record, {"tokio": "not cloned here"})
        self.assertEqual(out["status"], cs.STATUS_ACCEPTED_UNVERIFIED)
        self.assertEqual(out["unverified"], {"reason": "not cloned here"})

    def test_an_allowance_cannot_rescue_a_failing_record(self):
        record = cs.blank_record("tokio", "/r", "rust")
        record["status"] = cs.STATUS_FAIL
        record["failures"].append("config update exited 3")
        out = cs.finalize_status(record, {"tokio": "not cloned here"})
        self.assertEqual(out["status"], cs.STATUS_FAIL)


class TestStrictExit(unittest.TestCase):
    def record(self, status):
        r = cs.blank_record("x", "/r", "go")
        r["status"] = status
        return r

    def test_strict_is_zero_when_all_pass(self):
        self.assertEqual(cs.strict_exit_code([self.record(cs.STATUS_PASS)] * 3), 0)

    def test_strict_allows_accepted_gaps(self):
        records = [
            self.record(cs.STATUS_PASS),
            self.record(cs.STATUS_ACCEPTED_UNVERIFIED),
        ]
        self.assertEqual(cs.strict_exit_code(records), 0)

    def test_strict_rejects_unaccepted_unverified(self):
        records = [self.record(cs.STATUS_PASS), self.record(cs.STATUS_UNVERIFIED)]
        self.assertEqual(cs.strict_exit_code(records), 1)

    def test_strict_rejects_failures(self):
        self.assertEqual(cs.strict_exit_code([self.record(cs.STATUS_FAIL)]), 1)


class TestAIIsAdvisory(unittest.TestCase):
    def test_ensure_ai_block_is_idempotent(self):
        once = cs.ensure_ai_block("version: 2\n")
        self.assertIn("ai:", once)
        self.assertEqual(cs.ensure_ai_block(once), once)


class FakeRunner:
    """Records every argv and replays scripted results by command signature."""

    def __init__(self, responses):
        self.responses = responses
        self.calls: list[list[str]] = []

    def __call__(self, cmd, cwd):
        self.calls.append(cmd)
        for match, result in self.responses:
            if match(cmd):
                return result
        return cs.CommandResult(0, "", "")


class TestSweepFlow(unittest.TestCase):
    """Drive the per-repo flow with a fake runner: no binary, no target repo."""

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.tmp = Path(self._tmp.name)
        self.addCleanup(self._tmp.cleanup)
        self.target = self.tmp / "target"
        self.target.mkdir()
        (self.target / ".archfit.yaml").write_text("version: 1\nmodules: {}\n")
        self.spec = cs.RepoSpec("target", self.target, "go")
        self.out = self.tmp / "out"
        self.out.mkdir()
        self.work = self.out / "target"
        self.work.mkdir()

    def sweep(self, *, migration_only: bool = True) -> tuple[cs.Sweep, FakeRunner]:
        runner = FakeRunner([])
        return (
            cs.Sweep(
                Path("/bin/true"),
                self.out,
                runner=runner,
                cwd=self.tmp,
                migration_only=migration_only,
            ),
            runner,
        )

    def staged_candidate(self, sweep: cs.Sweep, record: dict) -> Path:
        cfg = sweep.prepare_config(self.spec, self.work, record)
        assert cfg is not None, record["failures"]
        return cfg

    def test_migration_only_is_the_command_actually_run(self):
        sweep, runner = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        cfg = self.staged_candidate(sweep, record)
        # Stand in for the binary's write: the version check reads the file back.
        cfg.write_text("version: 2\nmodules: {}\n")
        sweep.migrate_config(self.spec, cfg, self.work, record)

        update_calls = [c for c in runner.calls if "update" in c]
        self.assertEqual(len(update_calls), 2, update_calls)
        for call in update_calls:
            self.assertIn("--migration-only", call)
            self.assertIn("--apply", call)
        self.assertEqual(record["config"]["version"], 2)
        self.assertFalse(record["config"]["second_update_changed"])
        self.assertEqual(record["failures"], [])

    def test_a_config_left_at_v1_is_a_failure(self):
        sweep, _ = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        cfg = self.staged_candidate(sweep, record)
        sweep.migrate_config(self.spec, cfg, self.work, record)
        self.assertEqual(record["config"]["version"], 1)
        self.assertTrue(
            any("want 2" in f for f in record["failures"]), record["failures"]
        )

    def test_a_non_idempotent_second_migration_is_a_failure(self):
        sweep, _ = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        cfg = self.staged_candidate(sweep, record)
        passes = {"n": 0}

        def rewriting(_cmd, _cwd):
            passes["n"] += 1
            cfg.write_text(f"version: 2\nmodules: {{}}\n# pass {passes['n']}\n")
            return cs.CommandResult(0, "", "")

        sweep.runner = rewriting
        sweep.migrate_config(self.spec, cfg, self.work, record)
        self.assertTrue(record["config"]["second_update_changed"])
        self.assertTrue(
            any("rewrote" in f for f in record["failures"]), record["failures"]
        )

    def test_the_source_config_hash_is_the_targets_own_file(self):
        sweep, _ = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        self.staged_candidate(sweep, record)
        self.assertEqual(
            record["config"]["source_config_sha256"],
            cs.sha256_bytes((self.target / ".archfit.yaml").read_bytes()),
        )
        self.assertEqual(record["config"]["source"], "copied-existing")
        # The target file is untouched: the sweep is read-only over corpus repos.
        self.assertEqual(
            (self.target / ".archfit.yaml").read_text(), "version: 1\nmodules: {}\n"
        )

    def test_a_check_exit_that_contradicts_its_verdict_is_a_failure(self):
        sweep, _ = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        doc = state_doc()
        # needs_attention must be exit 2; claim 0 instead.
        sweep.runner = lambda cmd, cwd: cs.CommandResult(0, json.dumps(doc), "")
        sweep.collect_check(
            self.work / "archfit.yaml", self.spec, self.work, record, doc
        )
        self.assertEqual(record["check"]["exit"], 0)
        self.assertTrue(
            any("maps to exit 2" in f for f in record["failures"]), record["failures"]
        )

    def test_a_matching_check_exit_passes(self):
        sweep, _ = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        doc = state_doc()
        sweep.runner = lambda cmd, cwd: cs.CommandResult(2, json.dumps(doc), "")
        sweep.collect_check(
            self.work / "archfit.yaml", self.spec, self.work, record, doc
        )
        self.assertEqual(record["failures"], [])

    def test_analyze_json_is_validated_not_merely_parsed(self):
        sweep, _ = self.sweep()
        record = cs.blank_record("target", str(self.target), "go")
        broken = state_doc(schema_version="archfit.diagnostic.v2")
        sweep.runner = lambda cmd, cwd: cs.CommandResult(0, json.dumps(broken), "")
        state = sweep.collect_analyze(
            self.work / "archfit.yaml", self.spec, self.work, record
        )
        self.assertIsNotNone(state)
        self.assertFalse(record["formats"]["json"]["parity"])
        self.assertTrue(
            any("schema_version" in f for f in record["failures"]), record["failures"]
        )

    def test_a_non_byte_identical_repeat_is_a_failure(self):
        sweep, _ = self.sweep()
        (self.work / "analyze.stdout").write_text('{"a": 1}')
        sweep.runner = lambda cmd, cwd: cs.CommandResult(0, '{"a":  1}', "")
        record = cs.blank_record("target", str(self.target), "go")
        sweep.collect_determinism(
            self.work / "archfit.yaml", self.spec, self.work, record
        )
        self.assertFalse(record["determinism"]["json_byte_identical"])
        self.assertTrue(any("byte-identical" in f for f in record["failures"]))

    def test_an_identical_repeat_passes(self):
        sweep, _ = self.sweep()
        (self.work / "analyze.stdout").write_text('{"a": 1}')
        sweep.runner = lambda cmd, cwd: cs.CommandResult(0, '{"a": 1}', "")
        record = cs.blank_record("target", str(self.target), "go")
        sweep.collect_determinism(
            self.work / "archfit.yaml", self.spec, self.work, record
        )
        self.assertTrue(record["determinism"]["json_byte_identical"])
        self.assertEqual(record["failures"], [])

    def test_the_ai_overlay_never_touches_the_delivery_candidate(self):
        sweep, runner = self.sweep()
        cfg = self.work / "archfit.yaml"
        cfg.write_text("version: 2\nmodules: {}\n")
        before = cfg.read_bytes()
        record = cs.blank_record("target", str(self.target), "go")
        sweep.collect_ai(cfg, self.spec, self.work, record)
        self.assertEqual(cfg.read_bytes(), before)
        self.assertIn("ai:", (self.work / "archfit-ai.yaml").read_text())
        self.assertTrue(record["ai"]["requested"])
        # The AI run must not be pointed at the candidate.
        ai_calls = [c for c in runner.calls if "--ai-summary" in c]
        self.assertEqual(len(ai_calls), 1)
        self.assertIn(str(self.work / "archfit-ai.yaml"), ai_calls[0])
        self.assertNotIn(str(cfg), ai_calls[0])

    def test_a_failing_ai_summary_never_fails_a_record(self):
        sweep, _ = self.sweep()
        cfg = self.work / "archfit.yaml"
        cfg.write_text("version: 2\nmodules: {}\n")
        sweep.runner = lambda cmd, cwd: cs.CommandResult(3, "", "no API key")
        record = cs.blank_record("target", str(self.target), "go")
        record["status"] = cs.STATUS_FAIL
        sweep.collect_ai(cfg, self.spec, self.work, record)
        self.assertEqual(record["ai"], {"requested": True, "exit": 3})
        # A credential or provider failure is an environment fact; strict mode
        # grades deterministic behaviour only.
        self.assertEqual(record["failures"], [])
        self.assertEqual(cs.finalize_status(record, {})["status"], cs.STATUS_PASS)

    def test_a_missing_repository_is_unverified_not_a_pass(self):
        sweep, _ = self.sweep()
        spec = cs.RepoSpec("gone", self.tmp / "nope", "rust")
        record = sweep.process(
            spec, want_ai=False, want_repeat=False, want_formats=False
        )
        self.assertEqual(record["status"], cs.STATUS_UNVERIFIED)
        self.assertIsNotNone(record["unverified"])


class TestCorpusInventory(unittest.TestCase):
    def test_the_full_corpus_is_the_eleven_labels_the_plan_names(self):
        self.assertEqual(
            sorted(cs.CORPUS),
            sorted(
                [
                    "spotinfo",
                    "pumba",
                    "omni/scheduled-tasks",
                    "prometheus",
                    "ccgram",
                    "prefect",
                    "storybook",
                    "yazi",
                    "herdr",
                    "ruff",
                    "tokio",
                ]
            ),
        )

    def test_every_mandatory_language_has_a_representative(self):
        languages = {spec.language for spec in cs.CORPUS.values()}
        self.assertEqual(languages, {"go", "python", "ts", "rust"})
        for label, language in (
            ("spotinfo", "go"),
            ("storybook", "ts"),
            ("ccgram", "python"),
            ("herdr", "rust"),
        ):
            self.assertEqual(cs.CORPUS[label].language, language)




class TestHarnessGuards(unittest.TestCase):
    """The guards that keep a misconfigured sweep from lying or destroying data."""

    def test_refuses_to_wipe_a_directory_the_sweep_did_not_create(self):
        with tempfile.TemporaryDirectory() as tmp:
            victim = Path(tmp) / "spotinfo"
            victim.mkdir()
            (victim / "README.md").write_text("a real repository", encoding="utf-8")
            with self.assertRaises(RuntimeError):
                cs.prepare_work_dir(victim)
            self.assertTrue((victim / "README.md").exists())

    def test_re_creates_a_directory_the_sweep_owns(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = cs.prepare_work_dir(Path(tmp) / "w")
            (work / "stale").write_text("x", encoding="utf-8")
            work = cs.prepare_work_dir(work)
            self.assertFalse((work / "stale").exists())
            self.assertTrue((work / cs.WORK_DIR_MARKER).exists())

    def test_strict_mode_fails_on_an_empty_record_set(self):
        self.assertEqual(cs.strict_exit_code([]), 1)

    def test_a_work_dir_error_is_a_failure_an_allow_list_cannot_absorb(self):
        # An unmarked pre-existing work directory makes prepare_work_dir raise.
        # The record used to return still carrying the seeded `unverified`
        # status, so --allow-unverified promoted it to accepted_unverified and
        # strict mode exited 0 over a repo nothing had analysed.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "target"
            root.mkdir()
            out = Path(tmp) / "out"
            (out / "target").mkdir(parents=True)
            sweep = cs.Sweep(Path("/bin/true"), out, runner=FakeRunner([]), cwd=Path(tmp))
            record = sweep.process(
                cs.RepoSpec("target", root, "go"),
                want_ai=False,
                want_repeat=False,
                want_formats=False,
            )

        self.assertEqual(record["status"], cs.STATUS_FAIL)
        self.assertTrue(record["failures"], "the harness error was discarded")
        resolved = cs.finalize_status(record, {"target": "not cloned here"})
        self.assertEqual(resolved["status"], cs.STATUS_FAIL)
        self.assertEqual(cs.strict_exit_code([resolved]), 1)

    def test_an_absent_repository_stays_unverified(self):
        # The counterpart the fix must not break: a repo that is genuinely not
        # on this host is an unmeasured gap, and an allow-list entry may accept
        # it. The FAIL grade has to start after that early return.
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "out"
            out.mkdir()
            sweep = cs.Sweep(Path("/bin/true"), out, runner=FakeRunner([]), cwd=Path(tmp))
            record = sweep.process(
                cs.RepoSpec("target", Path(tmp) / "absent", "go"),
                want_ai=False,
                want_repeat=False,
                want_formats=False,
            )

        self.assertEqual(record["status"], cs.STATUS_UNVERIFIED)
        resolved = cs.finalize_status(record, {"target": "not cloned here"})
        self.assertEqual(resolved["status"], cs.STATUS_ACCEPTED_UNVERIFIED)

    def test_a_missing_coverage_count_is_a_failure(self):
        doc = state_doc()
        del doc["coverage"]["measured"]
        failures = cs.validate_state(doc)
        self.assertTrue(any("measured" in f for f in failures), failures)

    def test_a_non_numeric_metric_value_is_a_failure(self):
        doc = state_doc()
        doc["dimensions"]["intent"]["metrics"] = [
            {"name": "m", "value": "high", "unit": None}
        ]
        failures = cs.validate_state(doc)
        self.assertTrue(any("want a number" in f for f in failures), failures)
        self.assertTrue(any("want a string" in f for f in failures), failures)

    def test_a_check_verdict_from_non_object_json_is_reported(self):
        # `null` decodes fine; .get on it used to raise past the decode guard
        # and be recorded as a harness error, masking the real defect.
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            sweep = cs.Sweep(
                Path("archfit"),
                work,
                runner=lambda _cmd, _cwd: cs.CommandResult(0, "null", ""),
            )
            record = cs.blank_record("t", str(work), "go")
            sweep.collect_check(
                work / "archfit.yaml",
                cs.RepoSpec("t", work, "go"),
                work,
                record,
                state_doc(),
            )
        self.assertTrue(
            any("verdict" in f for f in record["failures"]), record["failures"]
        )


if __name__ == "__main__":
    unittest.main()
