package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
)

// coupledModulesCfg declares two modules with different owners and no rules, so
// Any FAIL from `archfit check` on the fixture repo comes from the coupling gate alone.
const coupledModulesCfg = `version: 1
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team-a
  b:
    paths: ["pkg/b/**"]
    internal: ["pkg/b/internal/**"]
    owner: team-b
`

// writeCoupledRepo creates a minimal Go repo with one cross-module edge
// (pkg/a → pkg/b/internal: intrusive strength across different owners — a
// measured, unbalanced coupling_balance score) and the given config body.
// Returns the config path.
func writeCoupledRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		markerGoMod: "module example.com/test\n\ngo 1.21\n",
		filePkgAA: "package a\n\nimport \"example.com/test/pkg/b/internal/impl\"\n\n" +
			"func UseSecret() string { return impl.Secret() }\n",
		filePkgBImpl:      implSource(),
		defaultConfigPath: cfgBody,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}

// TestCouplingGateView verifies the coupling.gate projection: absent block =
// disabled view (byte-identical pre-gate behavior), present block carries the
// knobs through with Enabled set.
func TestCouplingGateView(t *testing.T) {
	t.Parallel()
	if g := couplingGateView(config.Config{}); g.Enabled || g.MinBand != "" || g.MaxDrop != nil {
		t.Fatalf("nil gate block must project to a zero view, got %+v", g)
	}
	cfg := config.Config{Coupling: config.CouplingConfig{
		Gate: &config.CouplingGateDef{MinBand: "mixed", MaxDrop: new(5)},
	}}
	g := couplingGateView(cfg)
	if !g.Enabled || g.MinBand != score.BandMixed || g.MaxDrop == nil || *g.MaxDrop != 5 {
		t.Fatalf("gate view = %+v, want Enabled min_band=mixed max_drop=5", g)
	}
}

// TestRun_Analyze_CouplingGate_MinBandTrips verifies the V2 fix end to end: a
// measured coupling_balance band below coupling.gate.min_band fails the gate,
// and the triggering Balanced-Coupling findings surface as gate findings with
// agent tasks carrying file evidence.
func TestRun_Analyze_CouplingGate_MinBandTrips(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")

	var buf, errBuf bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, fmtJSON, "-c", cfgPath, flagRefresh}, &buf, &errBuf)
	if code != 1 {
		t.Fatalf("check with tripped coupling gate: exit = %d, want 1\noutput:\n%s", code, buf.String())
	}
	// Trip reasons are an analyze-only stderr contract (see analyze.go): they
	// must be disclosed on stderr and must not pollute the JSON on stdout.
	if !strings.Contains(errBuf.String(), "coupling gate: ") {
		t.Errorf("stderr carries no coupling-gate trip reason:\n%s", errBuf.String())
	}

	var diag diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("unmarshal JSON output: %v", err)
	}
	if diag.Verdict != diagnostic.VerdictFail {
		t.Fatalf("verdict = %q, want fail", diag.Verdict)
	}
	var bcTask *diagnostic.AgentTask
	for i := range diag.AgentTasks {
		if diag.AgentTasks[i].RuleID == engine.RuleIDBCImbalanced {
			bcTask = &diag.AgentTasks[i]
			break
		}
	}
	if bcTask == nil {
		t.Fatalf("agent_tasks carries no %s task: %+v", engine.RuleIDBCImbalanced, diag.AgentTasks)
	}
	if len(bcTask.Files) == 0 {
		t.Errorf("promoted coupling task has no files: %+v", *bcTask)
	}
	if bcTask.Goal == "" {
		t.Errorf("promoted coupling task has no goal: %+v", *bcTask)
	}
}

// TestRun_Baseline_NoTripReasonOnStderr pins the analyze-only half of the
// stderr contract: baseline shares runPipeline with analyze, but a tripped
// coupling gate must not echo "coupling gate:" trip reasons from baseline —
// there the stderr line is noise (see the comment in analyze.go).
func TestRun_Baseline_NoTripReasonOnStderr(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")

	var buf, errBuf bytes.Buffer
	code := RunWithStderr([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf, &errBuf)
	if code != 0 {
		t.Fatalf("baseline: exit = %d, want 0\nstderr:\n%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "coupling gate: ") {
		t.Errorf("baseline echoed coupling-gate trip reasons to stderr (analyze-only contract):\n%s", errBuf.String())
	}
}

// TestRun_Analyze_CouplingGate_OffByDefault verifies backward compatibility:
// without a coupling.gate block the same unbalanced repo passes the gate —
// coupling stays advisory-only.
func TestRun_Analyze_CouplingGate_OffByDefault(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, fmtJSON, "-c", cfgPath, flagRefresh}, &buf)
	if code != 0 {
		t.Fatalf("check without coupling.gate: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	var diag diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("unmarshal JSON output: %v", err)
	}
	for _, task := range diag.AgentTasks {
		if task.RuleID == engine.RuleIDBCImbalanced {
			t.Fatalf("coupling advisory produced an agent task without a gate: %+v", task)
		}
	}
}

// TestRun_Analyze_CouplingGate_BandNANeverTrips verifies the abstain rule: a
// repo where coupling cannot be measured (no analyzable source → band n/a)
// never fails the coupling gate, even with the strictest floor configured.
func TestRun_Analyze_CouplingGate_BandNANeverTrips(t *testing.T) {
	t.Parallel()
	cfgPath := writeNonGoRepo(t, "version: 1\ncoupling:\n  gate:\n    min_band: strong\n    max_drop: 0\n")

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh}, &buf)
	if code != 0 {
		t.Fatalf("check on unmeasured (n/a) coupling: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
}

// TestRun_Analyze_CouplingGate_MaxDrop verifies the drop knob: a stored
// baseline score anchors max_drop (trip on regression beyond it), and a
// baseline without a score snapshot cannot anchor a drop (no trip). It also
// pins the stderr disclosure contract per case (see analyze.go): a trip
// prints the max_drop-specific reason, a stale scorer version prints the
// "max_drop skipped" disclosure instead of gating silent, and a missing
// snapshot prints neither.
func TestRun_Analyze_CouplingGate_MaxDrop(t *testing.T) {
	t.Parallel()
	const (
		tripFragment = "exceeding coupling.gate.max_drop"
		skipFragment = "max_drop skipped"
	)
	cases := []struct {
		name         string
		score        *baseline.ScoreSnapshot
		wantCode     int
		wantStderr   []string // substrings that must be present
		wantNoStderr []string // substrings that must be absent
	}{
		{
			name:       "stored score anchors the drop",
			score:      &baseline.ScoreSnapshot{CouplingBalance: 95, Band: "strong", ScoreVersion: coupling.ScoreVersion},
			wantCode:   1,
			wantStderr: []string{tripFragment},
		},
		{
			name:         "no stored score skips the check",
			score:        nil,
			wantCode:     0,
			wantNoStderr: []string{tripFragment, skipFragment},
		},
		// A snapshot from a different scorer version is a methodology change,
		// not a regression — it must never anchor a drop.
		{
			name:       "stale scorer version skips the check",
			score:      &baseline.ScoreSnapshot{CouplingBalance: 95, Band: "strong", ScoreVersion: "bc_score.v3"},
			wantCode:   0,
			wantStderr: []string{skipFragment},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    max_drop: 5\n")
			base := baseline.Baseline{Score: tc.score}
			// Accept the current findings so only the score drop can trip.
			bPath := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)
			if err := baseline.Save(context.Background(), bPath, base); err != nil {
				t.Fatal(err)
			}

			var buf, errBuf bytes.Buffer
			code := RunWithStderr([]string{cmdCheck, "-c", cfgPath, flagRefresh}, &buf, &errBuf)
			if code != tc.wantCode {
				t.Fatalf("check: exit = %d, want %d\noutput:\n%s\nstderr:\n%s", code, tc.wantCode, buf.String(), errBuf.String())
			}
			for _, frag := range tc.wantStderr {
				if !strings.Contains(errBuf.String(), frag) {
					t.Errorf("stderr missing %q:\n%s", frag, errBuf.String())
				}
			}
			for _, frag := range tc.wantNoStderr {
				if strings.Contains(errBuf.String(), frag) {
					t.Errorf("stderr unexpectedly contains %q:\n%s", frag, errBuf.String())
				}
			}
		})
	}
}

// TestRun_Baseline_WritesScoreSnapshot verifies that `archfit baseline`
// persists the measured coupling_balance score, giving coupling.gate.max_drop
// its anchor.
func TestRun_Baseline_WritesScoreSnapshot(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
	}

	b, err := baseline.Load(context.Background(), filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath))
	if err != nil {
		t.Fatal(err)
	}
	if b.Score == nil {
		t.Fatal("baseline written without a score snapshot on a measured repo")
	}
	if b.Score.Band == "" || b.Score.Band == "n/a" {
		t.Fatalf("baseline score band = %q, want a measured band", b.Score.Band)
	}
	if b.Score.ScoreVersion != coupling.ScoreVersion {
		t.Fatalf("baseline score_version = %q, want %q", b.Score.ScoreVersion, coupling.ScoreVersion)
	}
	if got := b.CouplingScore(); got == nil || *got != b.Score.CouplingBalance {
		t.Fatalf("CouplingScore() = %v, want %d", got, b.Score.CouplingBalance)
	}
}

// TestApplyCouplingGate_PromotionScope pins the promotion filter: a tripped
// gate re-kinds only ACTIVE Balanced-Coupling advisories — baselined BC edges
// stay triaged as advisories, non-BC findings are untouched — and the summary
// counters move with the promoted findings.
func TestApplyCouplingGate_PromotionScope(t *testing.T) {
	t.Parallel()
	newDiag := func() diagnostic.Diagnostic {
		return diagnostic.Diagnostic{
			Verdict: diagnostic.VerdictPass,
			Findings: []finding.Finding{
				{ID: "bc-active", RuleID: engine.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew},
				{ID: "bc-baselined", RuleID: engine.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusBaseline},
				{ID: "rule-gate", RuleID: "no-cycles", Kind: finding.KindGate, Status: finding.StatusNew},
			},
			Summary: diagnostic.Summary{GateFindings: 1, Warnings: 2},
		}
	}
	card := score.Scorecard{Overall: 25, OverallBand: score.BandPoor}

	t.Run("tripped gate promotes only active BC advisories", func(t *testing.T) {
		t.Parallel()
		diag := newDiag()
		applyCouplingGate(&diag, card, score.CouplingGate{Enabled: true, MinBand: score.BandMixed}, baseline.Baseline{})
		if diag.Verdict != diagnostic.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		if got := diag.Findings[0].Kind; got != finding.KindGate {
			t.Errorf("active BC advisory kind = %q, want gate", got)
		}
		if got := diag.Findings[1].Kind; got != finding.KindAdvisory {
			t.Errorf("baselined BC advisory kind = %q, want advisory (triaged edges must not be promoted)", got)
		}
		if got := diag.Findings[2].Kind; got != finding.KindGate {
			t.Errorf("non-BC gate finding kind = %q, want gate (untouched)", got)
		}
		if diag.Summary.GateFindings != 2 || diag.Summary.Warnings != 1 {
			t.Errorf("summary after promotion = %+v, want GateFindings=2 Warnings=1", diag.Summary)
		}
	})

	t.Run("disabled gate is a no-op", func(t *testing.T) {
		t.Parallel()
		diag := newDiag()
		applyCouplingGate(&diag, card, score.CouplingGate{}, baseline.Baseline{})
		if diag.Verdict != diagnostic.VerdictPass ||
			diag.Findings[0].Kind != finding.KindAdvisory ||
			diag.Summary != (diagnostic.Summary{GateFindings: 1, Warnings: 2}) {
			t.Errorf("disabled gate mutated the diagnostic: verdict=%q findings[0].Kind=%q summary=%+v",
				diag.Verdict, diag.Findings[0].Kind, diag.Summary)
		}
	})

	t.Run("tripped gate with no promotable advisory synthesizes a gate finding", func(t *testing.T) {
		t.Parallel()
		// Advisory output off (or coupling.min_severity filtered everything):
		// the score still trips — it is computed from ClassifiedEdges, not from
		// the advisory findings — so the fail verdict must carry its own evidence.
		diag := diagnostic.Diagnostic{
			Verdict: diagnostic.VerdictPass,
			Findings: []finding.Finding{
				{ID: "bc-baselined", RuleID: engine.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusBaseline},
			},
		}
		applyCouplingGate(&diag, card, score.CouplingGate{Enabled: true, MinBand: score.BandMixed}, baseline.Baseline{})
		if diag.Verdict != diagnostic.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		if len(diag.Findings) != 2 {
			t.Fatalf("findings = %d, want 2 (baselined advisory + synthetic gate finding)", len(diag.Findings))
		}
		if got := diag.Findings[0].Kind; got != finding.KindAdvisory {
			t.Errorf("baselined BC advisory kind = %q, want advisory", got)
		}
		syn := diag.Findings[1]
		if syn.RuleID != ruleIDBCCouplingGate || syn.Kind != finding.KindGate || syn.Status != finding.StatusNew {
			t.Errorf("synthetic finding = %+v, want rule %s kind gate status new", syn, ruleIDBCCouplingGate)
		}
		if syn.Why == "" {
			t.Error("synthetic finding carries no trip reason")
		}
		if diag.Summary.GateFindings != 1 {
			t.Errorf("summary.GateFindings = %d, want 1", diag.Summary.GateFindings)
		}
	})
}

// TestRun_Analyze_CouplingGate_TripWithoutAdvisories verifies that a tripped
// coupling gate stays self-describing when no BC advisory is available to
// promote (--advisory=false; same shape as coupling.min_severity filtering
// every active edge): the run still fails, and the diagnostic carries a
// synthetic bc/coupling_gate gate finding with an agent task — never a fail
// verdict with zero gate findings.
func TestRun_Analyze_CouplingGate_TripWithoutAdvisories(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, fmtJSON, "-c", cfgPath, flagRefresh, flagNoAdvisories}, &buf)
	if code != 1 {
		t.Fatalf("check --no-advisories with tripped coupling gate: exit = %d, want 1\noutput:\n%s", code, buf.String())
	}

	var diag diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("unmarshal JSON output: %v", err)
	}
	if diag.Verdict != diagnostic.VerdictFail {
		t.Fatalf("verdict = %q, want fail", diag.Verdict)
	}
	var syn *finding.Finding
	for i := range diag.Findings {
		if diag.Findings[i].RuleID == ruleIDBCCouplingGate {
			syn = &diag.Findings[i]
			break
		}
	}
	if syn == nil {
		t.Fatalf("tripped gate left no %s finding: %+v", ruleIDBCCouplingGate, diag.Findings)
	}
	if syn.Kind != finding.KindGate || syn.Why == "" {
		t.Errorf("synthetic finding = %+v, want kind gate with a trip reason", *syn)
	}
	if diag.Summary.GateFindings == 0 {
		t.Error("summary.gate_findings = 0 on a fail verdict")
	}
	found := false
	for _, task := range diag.AgentTasks {
		if task.RuleID == ruleIDBCCouplingGate {
			found = true
			if task.Goal == "" {
				t.Errorf("coupling-gate agent task has no goal: %+v", task)
			}
		}
	}
	if !found {
		t.Fatalf("agent_tasks carries no %s task: %+v", ruleIDBCCouplingGate, diag.AgentTasks)
	}
}

// TestRun_Baseline_KeepsNativeAdvisoryKind guards the finding-lifecycle
// contract: `archfit baseline` (advisories on by default) under a tripped
// coupling.gate must persist BC findings with their native advisory kind, not the per-run gate
// promotion — a stored "gate" kind orphans the entry (status.Assign matches
// stored kind against the pass kind, so the edge would surface as a phantom
// "fixed" gate finding and never resolve on the advisory side).
func TestRun_Baseline_KeepsNativeAdvisoryKind(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")

	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
	}
	b, err := baseline.Load(context.Background(), filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath))
	if err != nil {
		t.Fatal(err)
	}
	sawBC := false
	for _, a := range b.Accepted {
		if a.RuleID != engine.RuleIDBCImbalanced {
			continue
		}
		sawBC = true
		if a.Kind != finding.KindAdvisory {
			t.Errorf("baselined BC finding %s kind = %q, want %q", a.Fingerprint, a.Kind, finding.KindAdvisory)
		}
	}
	if !sawBC {
		t.Fatal("fixture regression: baseline persisted no BC advisory")
	}
}

// TestRun_Baseline_SkipsSyntheticCouplingGateFinding: `archfit baseline
// --no-advisories` under a tripped coupling.gate synthesizes the
// bc/coupling_gate trip finding (no BC advisory exists to promote), but must
// not persist it — the engine never regenerates its fingerprint, so a stored
// entry would orphan and surface as a phantom "fixed" finding on later runs.
func TestRun_Baseline_SkipsSyntheticCouplingGateFinding(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")

	// Guard against a vacuous assertion: prove this fixture + --no-advisories
	// really does synthesize the trip finding before asserting it is not stored.
	var checkBuf bytes.Buffer
	if code := Run([]string{cmdCheck, fmtJSON, "-c", cfgPath, flagRefresh, flagNoAdvisories}, &checkBuf); code != 1 {
		t.Fatalf("check --no-advisories: exit = %d, want 1 (tripped coupling gate)\noutput:\n%s", code, checkBuf.String())
	}
	var diag diagnostic.Diagnostic
	if err := json.Unmarshal(checkBuf.Bytes(), &diag); err != nil {
		t.Fatalf("unmarshal check JSON: %v", err)
	}
	if !slices.ContainsFunc(diag.Findings, func(f finding.Finding) bool { return f.RuleID == ruleIDBCCouplingGate }) {
		t.Fatalf("fixture regression: no %s finding to skip: %+v", ruleIDBCCouplingGate, diag.Findings)
	}

	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh, flagNoAdvisories}, &buf); code != 0 {
		t.Fatalf("baseline --no-advisories: exit = %d\noutput:\n%s", code, buf.String())
	}
	b, err := baseline.Load(context.Background(), filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath))
	if err != nil {
		t.Fatal(err)
	}
	// The trip finding is the ONLY finding this run produces with advisories off,
	// so the baseline must come back empty. Asserting the length (not just
	// scanning for the rule ID) keeps the check from passing on an empty set.
	if len(b.Accepted) != 0 {
		t.Errorf("baseline persisted %d findings, want 0 (the %s trip finding is the only candidate and must be skipped): %+v",
			len(b.Accepted), ruleIDBCCouplingGate, b.Accepted)
	}
}

// TestRun_Analyze_MetricGate_ExitCodes exercises the metrics.<name> gate knob
// end to end (config → cfg.Metrics → computeVerdict → CLI exit): an
// encapsulation drop against the stored baseline blocks by default,
// downgrades with gate: warn, and is ignored with gate: off. The knob-only
// entries ({gate: warn}) double as a regression test for the enabled-pointer
// decode: they must not disable the metric.
func TestRun_Analyze_MetricGate_ExitCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		cfgExtra string
		wantCode int
	}{
		{"unset gate blocks on regression", "", 1},
		{"warn gate downgrades to warning", "metrics:\n  encapsulation:\n    gate: warn\n", 2},
		{"off gate ignores the regression", "metrics:\n  encapsulation:\n    gate: off\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeCoupledRepo(t, coupledModulesCfg+tc.cfgExtra)

			var buf bytes.Buffer
			if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
				t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
			}
			bPath := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)
			b, err := baseline.Load(context.Background(), bPath)
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := b.Metrics["encapsulation"]
			if !ok {
				t.Fatalf("fixture regression: no encapsulation snapshot in %+v", b.Metrics)
			}
			// Raise the snapshot so the unchanged current run reads as a
			// 1.0 ratio drop (higher_is_better) past the default min_delta 0.
			entry.Value++
			b.Metrics["encapsulation"] = entry
			if err := baseline.Save(context.Background(), bPath, b); err != nil {
				t.Fatal(err)
			}

			buf.Reset()
			if code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh}, &buf); code != tc.wantCode {
				t.Fatalf("check: exit = %d, want %d\noutput:\n%s", code, tc.wantCode, buf.String())
			}
		})
	}
}

// TestRun_Analyze_MetricGate_ExitCodes_CountDirection mirrors
// TestRun_Analyze_MetricGate_ExitCodes for a count-direction metric (cycle,
// DirectionHigherIsWorse) instead of a ratio metric (encapsulation): gate
// unset blocks on a regression, gate:warn downgrades, gate:off ignores it.
// The fixture has no import cycle, so the current cycle count stays 0; the
// baseline snapshot is tampered DOWN (instead of up, as the ratio case does)
// so the unchanged current value reads as a delta > max_new 0 past the
// default higher-is-worse floor.
func TestRun_Analyze_MetricGate_ExitCodes_CountDirection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		cfgExtra string
		wantCode int
	}{
		{"unset gate blocks on regression", "", 1},
		{"warn gate downgrades to warning", "metrics:\n  cycle:\n    gate: warn\n", 2},
		{"off gate ignores the regression", "metrics:\n  cycle:\n    gate: off\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeCoupledRepo(t, coupledModulesCfg+tc.cfgExtra)

			var buf bytes.Buffer
			if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
				t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
			}
			bPath := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)
			b, err := baseline.Load(context.Background(), bPath)
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := b.Metrics["cycle"]
			if !ok {
				t.Fatalf("fixture regression: no cycle snapshot in %+v", b.Metrics)
			}
			// Lower the snapshot so the unchanged current run (no cycle, value 0)
			// reads as a +1 delta (higher_is_worse) past the default max_new 0.
			entry.Value--
			b.Metrics["cycle"] = entry
			if err := baseline.Save(context.Background(), bPath, b); err != nil {
				t.Fatal(err)
			}

			buf.Reset()
			if code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh}, &buf); code != tc.wantCode {
				t.Fatalf("check: exit = %d, want %d\noutput:\n%s", code, tc.wantCode, buf.String())
			}
		})
	}
}
