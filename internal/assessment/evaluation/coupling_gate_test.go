package evaluation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/policy"
)

const ruleNoCycles = "no-cycles"

// mismatchScoreVersion is the baseline-snapshot input name the anchor reports
// when a stored score was written by a different scorer version.
const mismatchScoreVersion = "score_version"

// poorCard is a synthesised scorecard whose coupling band is below every
// configurable floor, so any enabled min_band trips on it.
var poorCard = score.Scorecard{Overall: 25, OverallBand: score.BandPoor}

func poorCouplingDiag() result.Result {
	return result.Result{
		Verdict: result.VerdictPass,
		Findings: []finding.Finding{
			{ID: "bc-active", RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew},
			{ID: "bc-baselined", RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusBaseline},
			{ID: "rule-gate", RuleID: ruleNoCycles, Kind: finding.KindGate, Status: finding.StatusNew},
		},
		Summary: result.Summary{GateFindings: 1, Warnings: 2},
	}
}

// TestFinalize_CouplingGatePromotionScope pins what a tripped coupling gate may
// escalate: only ACTIVE Balanced-Coupling advisories become gate findings.
// Baselined BC advisories stay triaged, non-BC findings are untouched, and the
// summary counters move with the promoted findings.
func TestFinalize_CouplingGatePromotionScope(t *testing.T) {
	t.Parallel()
	tripping := policy.CouplingGate{Enabled: true, MinBand: string(score.BandMixed)}

	t.Run("tripped gate promotes only active BC advisories", func(t *testing.T) {
		t.Parallel()
		diag := poorCouplingDiag()
		evaluation.ApplyCouplingGate(&diag, poorCard, tripping, evaluation.BaselineAnchor{})
		if diag.Verdict != result.VerdictFail {
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
		if len(evaluation.Finalize(&diag, evaluation.FinalizeInput{}).GateReasons) != 0 {
			t.Error("a disabled gate must not report trip reasons")
		}
	})

	t.Run("disabled gate is a no-op", func(t *testing.T) {
		t.Parallel()
		diag := poorCouplingDiag()
		out := evaluation.Finalize(&diag, evaluation.FinalizeInput{})
		evaluation.ApplyCouplingGate(&diag, poorCard, policy.CouplingGate{}, evaluation.BaselineAnchor{})
		if diag.Verdict != result.VerdictPass ||
			diag.Findings[0].Kind != finding.KindAdvisory ||
			diag.Summary != (result.Summary{GateFindings: 1, Warnings: 2}) {
			t.Errorf("disabled gate mutated the diagnostic: verdict=%q findings[0].Kind=%q summary=%+v",
				diag.Verdict, diag.Findings[0].Kind, diag.Summary)
		}
		if len(out.GateReasons) != 0 {
			t.Errorf("gate reasons = %v, want none for a disabled gate", out.GateReasons)
		}
	})

	t.Run("tripped gate with no promotable advisory synthesizes a gate finding", func(t *testing.T) {
		t.Parallel()
		// Advisory output off (or coupling.min_severity filtered everything):
		// the score still trips — it is computed from ClassifiedEdges, not from
		// the advisory findings — so the fail verdict must carry its own evidence.
		diag := poorCouplingDiag()
		diag.Findings = diag.Findings[1:2]
		diag.Summary = result.Summary{}
		evaluation.ApplyCouplingGate(&diag, poorCard, tripping, evaluation.BaselineAnchor{})
		if diag.Verdict != result.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		if len(diag.Findings) != 2 {
			t.Fatalf("findings = %d, want 2 (baselined advisory + synthetic gate finding)", len(diag.Findings))
		}
		if got := diag.Findings[0].Kind; got != finding.KindAdvisory {
			t.Errorf("baselined BC advisory kind = %q, want advisory", got)
		}
		syn := diag.Findings[1]
		if syn.ID != evaluation.FindingIDCouplingGate || syn.RuleID != finding.RuleIDCouplingGate ||
			syn.Kind != finding.KindGate || syn.Status != finding.StatusNew {
			t.Errorf("synthetic finding = %+v, want rule %s kind gate status new", syn, finding.RuleIDCouplingGate)
		}
		if syn.Why == "" {
			t.Error("synthetic finding carries no trip reason")
		}
		if diag.Summary.GateFindings != 1 {
			t.Errorf("summary.GateFindings = %d, want 1", diag.Summary.GateFindings)
		}
	})
}

// TestFinalize_CouplingGateMaxDrop pins the baseline-anchored half of the gate:
// max_drop compares against the stored coupling score, and an unmeasured band
// never trips (abstain is not failure).
func TestFinalize_CouplingGateMaxDrop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		card      score.Scorecard
		maxDrop   *int
		anchor    *int
		wantTrip  bool
		wantWhyIn string
	}{
		{name: "drop beyond max_drop trips", card: score.Scorecard{Overall: 60, OverallBand: score.BandMixed}, maxDrop: intPtr(5), anchor: intPtr(95), wantTrip: true, wantWhyIn: "max_drop"},
		{name: "drop within max_drop passes", card: score.Scorecard{Overall: 60, OverallBand: score.BandMixed}, maxDrop: intPtr(50), anchor: intPtr(95)},
		{name: "no stored anchor cannot trip", card: score.Scorecard{Overall: 60, OverallBand: score.BandMixed}, maxDrop: intPtr(1)},
		{name: "unmeasured band never trips", card: score.Scorecard{OverallBand: score.BandNA}, maxDrop: intPtr(1), anchor: intPtr(95)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diag := result.Result{Verdict: result.VerdictPass}
			gate := policy.CouplingGate{Enabled: true, MaxDrop: test.maxDrop}
			anchor := evaluation.BaselineAnchor{CouplingScore: test.anchor}
			evaluation.ApplyCouplingGate(&diag, test.card, gate, anchor)
			tripped := diag.Verdict == result.VerdictFail
			if tripped != test.wantTrip {
				t.Fatalf("tripped = %v (verdict %q), want %v", tripped, diag.Verdict, test.wantTrip)
			}
			if !test.wantTrip {
				return
			}
			if len(diag.Findings) != 1 || diag.Findings[0].ID != evaluation.FindingIDCouplingGate {
				t.Fatalf("findings = %+v, want one synthetic coupling-gate finding", diag.Findings)
			}
			if !strings.Contains(diag.Findings[0].Why, test.wantWhyIn) {
				t.Errorf("why = %q, want it to name %q", diag.Findings[0].Why, test.wantWhyIn)
			}
		})
	}
}

// TestFinalize_BuildsRepairTasks pins that Finalize is the single place repair
// work is attached: every gate finding leaves an agent task carrying the
// validation command, and advisories leave advisory tasks.
func TestFinalize_BuildsRepairTasks(t *testing.T) {
	t.Parallel()
	const validate = "archfit check -c .archfit.yaml"
	diag := result.Result{
		Verdict: result.VerdictFail,
		Findings: []finding.Finding{
			{ID: "g1", RuleID: ruleNoCycles, Kind: finding.KindGate, Status: finding.StatusNew,
				Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: "a", Path: "pkg/a/a.go"}, To: finding.Endpoint{Module: "b", Path: "pkg/b/b.go"}}},
			{ID: "a1", RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew,
				MatchedBy: map[string]string{"group_count": "3", "group_members": "a1,a2,a3"}},
		},
	}
	evaluation.Finalize(&diag, evaluation.FinalizeInput{
		RuleTypes:          map[string]string{ruleNoCycles: ruleForbidden},
		ValidationCommands: []string{validate},
		KnownFiles:         map[string]struct{}{"pkg/a/a.go": {}, "pkg/b/b.go": {}},
		OnDisk:             func(string) bool { return true },
	})
	if len(diag.AgentTasks) != 1 || diag.AgentTasks[0].FindingID != "g1" {
		t.Fatalf("agent tasks = %+v, want one for the gate finding", diag.AgentTasks)
	}
	if !slices.Contains(diag.AgentTasks[0].Validation, validate) {
		t.Errorf("validation = %v, want the supplied command", diag.AgentTasks[0].Validation)
	}
	if len(diag.AdvisoryTasks) != 1 || diag.AdvisoryTasks[0].FindingID != "a1" {
		t.Fatalf("advisory tasks = %+v, want one for the BC advisory", diag.AdvisoryTasks)
	}
}

// TestCouplingGateAnchorStale pins when max_drop is silently skipped: only an
// enabled gate that actually declares max_drop and reads an incompatible stored
// snapshot needs the stale-baseline notice.
func TestCouplingGateAnchorStale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		gate       policy.CouplingGate
		mismatches []string
		want       bool
	}{
		{name: "enabled with max_drop and mismatch", gate: policy.CouplingGate{Enabled: true, MaxDrop: intPtr(5)}, mismatches: []string{mismatchScoreVersion}, want: true},
		{name: "no mismatch", gate: policy.CouplingGate{Enabled: true, MaxDrop: intPtr(5)}},
		{name: "no max_drop", gate: policy.CouplingGate{Enabled: true}, mismatches: []string{mismatchScoreVersion}},
		{name: "gate disabled", gate: policy.CouplingGate{MaxDrop: intPtr(5)}, mismatches: []string{mismatchScoreVersion}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := evaluation.CouplingGateAnchorStale(test.gate, evaluation.BaselineAnchor{SnapshotMismatches: test.mismatches})
			if got != test.want {
				t.Errorf("CouplingGateAnchorStale = %v, want %v", got, test.want)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
