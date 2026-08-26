// Behavior tests for the hard-gate/diagnostic split the architecture state is
// decided from. They pin which findings enter which population, that a
// required-tool failure blocks without inventing a finding, and that the split
// agrees with the summary counters the existing output already publishes.
package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
)

// TestBuildStateClassifiesFindings is the population table: a finding is a
// blocker only when it is an active gate finding, a diagnostic only when it is
// an active advisory, and neither once its status says the decision was already
// made.
func TestBuildStateClassifiesFindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		finding  finding.Finding
		blocker  bool
		advisory bool
	}{
		{"new gate finding blocks", finding.Finding{ID: "1", Kind: finding.KindGate, Status: finding.StatusNew}, true, false},
		{"expired waiver on a gate finding blocks", finding.Finding{ID: "2", Kind: finding.KindGate, Status: finding.StatusExpiredWaiver}, true, false},
		{"accepted gate finding blocks nothing", finding.Finding{ID: "3", Kind: finding.KindGate, Status: finding.StatusBaseline}, false, false},
		{"waived gate finding blocks nothing", finding.Finding{ID: "4", Kind: finding.KindGate, Status: finding.StatusWaived}, false, false},
		{"fixed gate finding blocks nothing", finding.Finding{ID: "5", Kind: finding.KindGate, Status: finding.StatusFixed}, false, false},
		{"new advisory is a diagnostic", finding.Finding{ID: "6", Kind: finding.KindAdvisory, Status: finding.StatusNew}, false, true},
		{"expired waiver on an advisory is a diagnostic", finding.Finding{ID: "7", Kind: finding.KindAdvisory, Status: finding.StatusExpiredWaiver}, false, true},
		{"accepted advisory is not a diagnostic", finding.Finding{ID: "8", Kind: finding.KindAdvisory, Status: finding.StatusBaseline}, false, false},
		{"waived advisory is not a diagnostic", finding.Finding{ID: "9", Kind: finding.KindAdvisory, Status: finding.StatusWaived}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := result.Result{Findings: []finding.Finding{tc.finding}}
			st := evaluation.BuildState(&diag, false)
			if got := len(st.Blockers) == 1; got != tc.blocker {
				t.Errorf("blockers = %+v, want blocker=%t", st.Blockers, tc.blocker)
			}
			if got := len(st.Diagnostics) == 1; got != tc.advisory {
				t.Errorf("diagnostics = %+v, want diagnostic=%t", st.Diagnostics, tc.advisory)
			}
		})
	}
}

// TestBuildStateReferencesFindingIdentity pins that the split carries the
// finding's published identity rather than a re-derived one: the diagnostic
// stays the single owner of ID, rule, severity, and status.
func TestBuildStateReferencesFindingIdentity(t *testing.T) {
	t.Parallel()
	diag := result.Result{Findings: []finding.Finding{{
		ID: "abc123", Kind: finding.KindGate, RuleID: "core_no_toolrun",
		Status: finding.StatusNew, Severity: finding.SeverityCritical,
	}}}
	st := evaluation.BuildState(&diag, false)
	want := state.FindingRef{ID: "abc123", RuleID: "core_no_toolrun", Kind: finding.KindGate,
		Severity: string(finding.SeverityCritical), Status: string(finding.StatusNew)}
	if len(st.Blockers) != 1 || st.Blockers[0] != want {
		t.Errorf("blocker = %+v, want %+v", st.Blockers, want)
	}
}

// TestBuildStateHardGateResult covers the repository hard-gate result. A
// required-tool policy failure blocks without producing a finding, so it cannot
// be inferred from the blocker count.
func TestBuildStateHardGateResult(t *testing.T) {
	t.Parallel()
	gateFinding := finding.Finding{ID: "1", Kind: finding.KindGate, Status: finding.StatusNew}
	advisory := finding.Finding{ID: "2", Kind: finding.KindAdvisory, Status: finding.StatusNew}
	tests := []struct {
		name     string
		findings []finding.Finding
		toolGate bool
		want     state.HardGateState
		blockers int
	}{
		{"clean run passes", []finding.Finding{advisory}, false, state.HardGatePass, 0},
		{"active gate finding fails", []finding.Finding{gateFinding}, false, state.HardGateFail, 1},
		{"required-tool failure fails with no finding", []finding.Finding{advisory}, true, state.HardGateFail, 0},
		{"both fail once", []finding.Finding{gateFinding, advisory}, true, state.HardGateFail, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := result.Result{Findings: tc.findings}
			st := evaluation.BuildState(&diag, tc.toolGate)
			if st.Decision.HardGates != tc.want {
				t.Errorf("hard gates = %q, want %q", st.Decision.HardGates, tc.want)
			}
			if st.Decision.ActiveBlockers != tc.blockers {
				t.Errorf("active blockers = %d, want %d", st.Decision.ActiveBlockers, tc.blockers)
			}
			if st.RequiredToolFailure != tc.toolGate {
				t.Errorf("required-tool failure = %t, want %t", st.RequiredToolFailure, tc.toolGate)
			}
		})
	}
}

// TestBuildStateLeavesDimensionsUnmeasured pins the slice boundary: the split
// ships before the collectors, so every envelope must still report unmeasured
// rather than measured-and-empty.
func TestBuildStateLeavesDimensionsUnmeasured(t *testing.T) {
	t.Parallel()
	diag := result.Result{}
	st := evaluation.BuildState(&diag, false)
	if st.Decision.UnknownDimensions != state.DimensionCount {
		t.Errorf("unknown dimensions = %d, want %d", st.Decision.UnknownDimensions, state.DimensionCount)
	}
	if _, _, unmeasured := st.Dimensions.CountStatuses(); unmeasured != state.DimensionCount {
		t.Errorf("unmeasured = %d, want %d", unmeasured, state.DimensionCount)
	}
}

// TestScoreStatePopulatedFromTheFinalizedRun is the integration invariant: the
// blocker population and the summary's gate counter are two views of one fact,
// so a coupling-gate promotion or a synthetic trip finding must move both.
func TestScoreStatePopulatedFromTheFinalizedRun(t *testing.T) {
	t.Parallel()
	in := assessInput()
	assessed, err := evaluation.Assess(in)
	if err != nil {
		t.Fatal(err)
	}
	diag := assessed.Diagnostic
	scored := evaluation.Score(&diag, evaluation.ScoreInput{
		Policy: in.Policy, Facts: in.Facts, ConfigSource: assessCfgPath, ScanRoot: assessRoot,
		Root: assessRoot, MarkedCoverage: in.MarkedCoverage, CoverageGaps: in.CoverageGaps,
		RequireTools: true, ApplyToolGate: true,
	})
	if len(diag.State.Blockers) != diag.Summary.GateFindings {
		t.Errorf("blockers = %d, summary gate findings = %d; the split must not re-derive the count",
			len(diag.State.Blockers), diag.Summary.GateFindings)
	}
	if diag.State.RequiredToolFailure != scored.HardGate {
		t.Errorf("required-tool failure = %t, want the scored hard gate %t", diag.State.RequiredToolFailure, scored.HardGate)
	}
	if diag.State.Decision.HardGates != state.HardGateFail {
		t.Errorf("hard gates = %q, want %q for a required-analyzer failure", diag.State.Decision.HardGates, state.HardGateFail)
	}
}
