package console

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alexei-led/archfit/internal/model/report"
)

const (
	confHigh     = "high"
	confMedium   = "medium"
	statusNew    = "new"
	ruleInternal = "no_internal_access"
)

// stateWith returns a populated architecture state: one flagged dimension, one
// blocker, one diagnostic, one seam, and one unmeasured envelope.
func stateWith() report.ArchitectureState {
	s := report.NewArchitectureState()
	s.Verdict = report.StateBlocked
	s.Decision = report.StateDecision{
		HardGates: report.HardGateFail, ActiveBlockers: 1, AttentionDimensions: 2, UnknownDimensions: 1,
	}
	s.Dimensions.Structure = report.DimensionState{
		Name: report.DimensionStructure, Owner: report.OwnerStructure,
		Status: report.MeasurementMeasured, Confidence: confHigh, Gate: report.GateFail,
		Coverage: report.DimensionCoverage{Basis: "classified edges", Observed: 7, Total: 9},
		Findings: []report.FindingRef{{ID: "gate-1", RuleID: ruleInternal, Kind: report.FindingKindGate, Severity: confHigh, Status: statusNew}},
	}
	s.Dimensions.Coupling = report.DimensionState{
		Name: report.DimensionCoupling, Owner: report.OwnerCoupling,
		Status: report.MeasurementPartial, Confidence: confMedium, Gate: report.GateWarn,
		Coverage: report.DimensionCoverage{Basis: "scored edges", Observed: 4, Total: 6},
		Findings: []report.FindingRef{{ID: "adv-1", RuleID: "bc/imbalanced_coupling", Kind: report.FindingKindAdvisory, Severity: confMedium, Status: statusNew}},
	}
	s.Dimensions.Drift.Unknown = []report.UnknownFact{{
		Fact: "architecture drift", Reason: "no comparable architecture-state reference is stored", Owner: report.OwnerDrift,
	}}
	s.Findings = []report.Finding{
		{ID: "gate-1", RuleID: ruleInternal, Kind: report.FindingKindGate, Severity: confHigh, Status: statusNew, Why: "pkg/a reaches into pkg/b/internal"},
		{ID: "adv-1", RuleID: "bc/imbalanced_coupling", Kind: report.FindingKindAdvisory, Severity: confMedium, Status: statusNew, Why: "functional coupling across owners"},
		{ID: "accepted-1", RuleID: ruleInternal, Kind: report.FindingKindGate, Severity: confHigh, Status: "baseline", Why: "already accepted"},
	}
	s.Seams = []report.Seam{{
		ID: "seam-a", FromModule: "a", ToModule: "b", Strength: "functional", Distance: "different_owner",
		Volatility: confHigh, ScoredEdges: 6, CriticalEdges: 3, DistributedMonolith: true,
		Scores:     report.SeamScoreDistribution{N: 6, Median: 8},
		Hypothesis: "reduce_distance",
	}}
	s.Coverage = report.StateCoverage{Measured: 1, Partial: 1, Unmeasured: 7}
	return s
}

func render(t *testing.T, s report.ArchitectureState) string {
	t.Helper()
	var b strings.Builder
	if err := RenderState(s, &b); err != nil {
		t.Fatalf("RenderState: %v", err)
	}
	return b.String()
}

// TestRenderState_Headline pins the five headline lines the state contract
// requires, and that the verdict displays upper case while JSON stores lower.
func TestRenderState_Headline(t *testing.T) {
	out := render(t, stateWith())
	for _, want := range []string{
		"ARCHITECTURE STATE",
		"VERDICT    BLOCKED",
		"BLOCKING   1 active  ·  hard gates: fail",
		"ATTENTION  2 dimension(s) flagged  ·  1 diagnostic(s)",
		"COVERAGE   1 measured · 1 partial · 7 unmeasured  (of 9)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("headline is missing %q:\n%s", want, out)
		}
	}
}

// TestRenderState_ListsEveryDimension: all nine envelopes are always shown with
// their status, gate, confidence, and denominator. Omitting an unmeasured one
// would make missing evidence invisible.
func TestRenderState_ListsEveryDimension(t *testing.T) {
	out := render(t, stateWith())
	for _, name := range []string{
		report.DimensionIntent, report.DimensionStructure, report.DimensionModularity,
		report.DimensionCoupling, report.DimensionChangeLocality, report.DimensionComplexity,
		report.DimensionTestability, report.DimensionOperations, report.DimensionDrift,
	} {
		if !strings.Contains(out, name) {
			t.Errorf("dimension %q missing from the report:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "classified edges 7/9") {
		t.Errorf("a measured dimension must print its denominator:\n%s", out)
	}
	if !strings.Contains(out, "no denominator") {
		t.Errorf("an unmeasured dimension must say it has no denominator rather than print 0/0:\n%s", out)
	}
}

// TestRenderState_CarriesNoRepositoryScore is the presentation half of the
// migration contract: the terminal report must not regrow a 0-100 headline or a
// "why the score is low" section.
func TestRenderState_CarriesNoRepositoryScore(t *testing.T) {
	out := render(t, stateWith())
	for _, forbidden := range []string{"/ 100", "WHY THE SCORE IS LOW", "WHAT WOULD IMPROVE THE SCORE", "Score", "TARGETS"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("terminal report carries %q: the architecture state has no repository score:\n%s", forbidden, out)
		}
	}
}

// TestRenderState_ShowsOnlyActiveFindings: an accepted finding was already
// decided, so it must not reappear as work to do.
func TestRenderState_ShowsOnlyActiveFindings(t *testing.T) {
	out := render(t, stateWith())
	if !strings.Contains(out, "pkg/a reaches into pkg/b/internal") {
		t.Errorf("the active blocker is missing:\n%s", out)
	}
	if !strings.Contains(out, "functional coupling across owners") {
		t.Errorf("the active diagnostic is missing:\n%s", out)
	}
	if strings.Contains(out, "already accepted") {
		t.Errorf("a baselined finding reappeared as actionable work:\n%s", out)
	}
}

// TestRenderState_SeamsAndUnknowns: the coupling ledger and the honest
// not-measured list both reach the terminal.
func TestRenderState_SeamsAndUnknowns(t *testing.T) {
	out := render(t, stateWith())
	for _, want := range []string{
		"COUPLING SEAMS (1)",
		"a -> b",
		"[distributed monolith]",
		"functional × different_owner × high volatility · 3 critical of 6 scored · median balance 8",
		"try: reduce_distance",
		"NOT MEASURED (1)",
		"drift — architecture drift",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

// TestRenderState_CleanRunSaysSo: with no blocker the report says the run is
// planning input, not a stop signal, and still prints the comparison status.
func TestRenderState_CleanRunSaysSo(t *testing.T) {
	s := report.NewArchitectureState()
	s.Verdict = report.StateNeedsAttention
	out := render(t, s)
	if !strings.Contains(out, "No blockers.") {
		t.Errorf("a run with no blocker must say so:\n%s", out)
	}
	if !strings.Contains(out, "status: not_requested  ·  reference: none") {
		t.Errorf("the comparison block must state that nothing was compared:\n%s", out)
	}
	if strings.Contains(out, "COUPLING SEAMS") {
		t.Errorf("an empty ledger must not print a seam section:\n%s", out)
	}
}

// TestRenderState_IsDeterministic: two renders of the same state must not
// differ, or the format cannot carry a byte-comparable baseline.
func TestRenderState_IsDeterministic(t *testing.T) {
	s := stateWith()
	if first, second := render(t, s), render(t, s); first != second {
		t.Errorf("two renders differ:\n%s\n---\n%s", first, second)
	}
}

// TestCondenseIsRuneSafe pins the UTF-8 contract of the terminal renderer.
//
// The regression: condense sliced the string by BYTES. Every coupling advisory
// carries "×" and "→", so a cut landing inside one emitted invalid UTF-8 — and
// a consumer decoding the document strictly loses the whole report, not one
// line. Found by the corpus sweep on storybook.
func TestCondenseIsRuneSafe(t *testing.T) {
	t.Parallel()
	// The multi-byte runes are positioned so the cut lands inside one at every
	// budget in the loop below.
	subject := strings.Repeat("balanced coupling × distance → volatile target ", 12)
	for budget := 1; budget <= 200; budget++ {
		got := condense(subject, budget)
		if !utf8.ValidString(got) {
			t.Fatalf("condense(_, %d) produced invalid UTF-8: %q", budget, got)
		}
		if len(got) > budget+len("…") {
			t.Fatalf("condense(_, %d) returned %d bytes", budget, len(got))
		}
	}
}

// TestCondenseKeepsShortStringsWhole guards the other direction: the boundary
// fix must not start trimming text that already fits.
func TestCondenseKeepsShortStringsWhole(t *testing.T) {
	t.Parallel()
	const short = "a × b"
	if got := condense(short, 100); got != short {
		t.Fatalf("condense(%q, 100) = %q", short, got)
	}
}
