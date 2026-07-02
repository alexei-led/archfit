package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
)

// coupledModulesCfg declares two modules with different owners and no rules, so
// any --gate FAIL on the fixture repo comes from the coupling gate alone.
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
		"pkg/b/internal/impl/impl.go": "package impl\n\nfunc Secret() string { return \"s\" }\n",
		defaultConfigPath:             cfgBody,
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

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, fmtJSON, "-c", cfgPath, flagFull, flagGate}, &buf)
	if code != 1 {
		t.Fatalf("analyze --gate with tripped coupling gate: exit = %d, want 1\noutput:\n%s", code, buf.String())
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
		if diag.AgentTasks[i].RuleID == ruleIDBCImbalanced {
			bcTask = &diag.AgentTasks[i]
			break
		}
	}
	if bcTask == nil {
		t.Fatalf("agent_tasks carries no %s task: %+v", ruleIDBCImbalanced, diag.AgentTasks)
	}
	if len(bcTask.Files) == 0 {
		t.Errorf("promoted coupling task has no files: %+v", *bcTask)
	}
	if bcTask.Goal == "" {
		t.Errorf("promoted coupling task has no goal: %+v", *bcTask)
	}
}

// TestRun_Analyze_CouplingGate_OffByDefault verifies backward compatibility:
// without a coupling.gate block the same unbalanced repo passes the gate —
// coupling stays advisory-only.
func TestRun_Analyze_CouplingGate_OffByDefault(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, fmtJSON, "-c", cfgPath, flagFull, flagGate}, &buf)
	if code != 0 {
		t.Fatalf("analyze --gate without coupling.gate: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	var diag diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("unmarshal JSON output: %v", err)
	}
	for _, task := range diag.AgentTasks {
		if task.RuleID == ruleIDBCImbalanced {
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
	code := Run([]string{cmdAnalyze, "-c", cfgPath, flagFull, flagGate}, &buf)
	if code != 0 {
		t.Fatalf("analyze --gate on unmeasured (n/a) coupling: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
}

// TestRun_Analyze_CouplingGate_MaxDrop verifies the drop knob: a stored
// baseline score anchors max_drop (trip on regression beyond it), and a
// baseline without a score snapshot cannot anchor a drop (no trip).
func TestRun_Analyze_CouplingGate_MaxDrop(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		score    *baseline.ScoreSnapshot
		wantCode int
	}{
		{"stored score anchors the drop", &baseline.ScoreSnapshot{CouplingBalance: 95, Band: "strong"}, 1},
		{"no stored score skips the check", nil, 0},
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

			var buf bytes.Buffer
			code := Run([]string{cmdAnalyze, "-c", cfgPath, flagFull, flagGate}, &buf)
			if code != tc.wantCode {
				t.Fatalf("analyze --gate: exit = %d, want %d\noutput:\n%s", code, tc.wantCode, buf.String())
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
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagFull}, &buf); code != 0 {
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
				{ID: "bc-active", RuleID: ruleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew},
				{ID: "bc-baselined", RuleID: ruleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusBaseline},
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
}

// TestRun_Baseline_KeepsNativeAdvisoryKind guards the finding-lifecycle
// contract: `archfit baseline --advisory` under a tripped coupling.gate must
// persist BC findings with their native advisory kind, not the per-run gate
// promotion — a stored "gate" kind orphans the entry (status.Assign matches
// stored kind against the pass kind, so the edge would surface as a phantom
// "fixed" gate finding and never resolve on the advisory side).
func TestRun_Baseline_KeepsNativeAdvisoryKind(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg+"coupling:\n  gate:\n    min_band: strong\n")

	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagFull, "--advisory"}, &buf); code != 0 {
		t.Fatalf("baseline --advisory: exit = %d\noutput:\n%s", code, buf.String())
	}
	b, err := baseline.Load(context.Background(), filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath))
	if err != nil {
		t.Fatal(err)
	}
	sawBC := false
	for _, a := range b.Accepted {
		if a.RuleID != ruleIDBCImbalanced {
			continue
		}
		sawBC = true
		if a.Kind != finding.KindAdvisory {
			t.Errorf("baselined BC finding %s kind = %q, want %q", a.Fingerprint, a.Kind, finding.KindAdvisory)
		}
	}
	if !sawBC {
		t.Fatal("fixture regression: baseline --advisory persisted no BC advisory")
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
			if code := Run([]string{cmdBaseline, "-c", cfgPath, flagFull}, &buf); code != 0 {
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
			if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagFull, flagGate}, &buf); code != tc.wantCode {
				t.Fatalf("analyze --gate: exit = %d, want %d\noutput:\n%s", code, tc.wantCode, buf.String())
			}
		})
	}
}
