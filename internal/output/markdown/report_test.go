package markdown

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

func stateFixture() report.ArchitectureState {
	s := report.NewArchitectureState()
	s.Verdict = report.StateNeedsAttention
	s.Decision = report.StateDecision{
		HardGates: report.HardGatePass, ActiveBlockers: 0, AttentionDimensions: 1, UnknownDimensions: 4,
	}
	s.Dimensions.Coupling = report.DimensionState{
		Name: report.DimensionCoupling, Owner: report.OwnerCoupling,
		Status: report.MeasurementMeasured, Confidence: "medium", Gate: report.GateWarn,
		Coverage: report.DimensionCoverage{Basis: "scored edges", Observed: 380, Total: 400},
		Findings: []report.FindingRef{{ID: "adv-1", RuleID: "bc/imbalanced_coupling", Kind: report.FindingKindAdvisory}},
	}
	s.Dimensions.Drift.Unknown = []report.UnknownFact{{
		Fact: "architecture drift", Reason: "no comparable architecture-state reference is stored", Owner: report.OwnerDrift,
	}}
	s.Coverage = report.StateCoverage{
		Measured: 1, Partial: 0, Unmeasured: 8,
		Tools: []report.StateToolCoverage{{Tool: "go/packages", Status: "ok"}, {Tool: "scip", Status: "disabled", Reason: "opt-in"}},
	}
	s.Seams = []report.Seam{{
		ID: "seam-a", FromModule: "a", ToModule: "b", Strength: "functional", Distance: "different_owner",
		Volatility: "high", ScoredEdges: 6, CriticalEdges: 3, DistributedMonolith: true,
		Scores: report.SeamScoreDistribution{N: 6, Median: 8}, Quadrant: "tight", Hypothesis: "reduce_distance",
	}}
	s.Comparison = report.StateComparison{
		Status: report.ComparisonNonComparable, BaseRef: "main",
		Reasons: []string{"config_hash differs between the two runs"},
	}
	return s
}

func renderState(t *testing.T, s report.ArchitectureState) string {
	t.Helper()
	var b strings.Builder
	if err := RenderState(s, &b); err != nil {
		t.Fatalf("RenderState: %v", err)
	}
	return b.String()
}

// TestRenderState_HeadlineAndTables pins the Markdown headline, the nine-row
// dimension table, the coverage table, the seam ledger, the comparison, and the
// not-measured list — the same facts --format json publishes, laid out for a
// human.
func TestRenderState_HeadlineAndTables(t *testing.T) {
	out := renderState(t, stateFixture())
	for _, want := range []string{
		"# archfit — architecture state",
		"- **Verdict:** NEEDS ATTENTION",
		"- **Blocking:** 0 active — hard gates: pass",
		"- **Attention:** 1 dimension(s) flagged — 1 diagnostic(s)",
		"- **Coverage:** 1 measured / 0 partial / 8 unmeasured (of 9)",
		"## Dimensions",
		"| coupling | measured | warn | medium | scored edges 380/400 | 1 |",
		"| drift | unmeasured | not_applicable | unrated | _no denominator_ | 0 |",
		"## Evidence coverage",
		"| scip | disabled | opt-in |",
		"## Coupling seams (1)",
		"| a → b ⚠ | functional | different_owner | high | 6 | 3 | 8 | tight | reduce_distance |",
		"## Comparison",
		"- **Status:** non_comparable",
		"- **Reference:** main",
		"- config_hash differs between the two runs",
		"## Not measured (1)",
		"**drift — architecture drift**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown state missing %q\n---\n%s", want, out)
		}
	}
}

// TestRenderState_RendersDimensionMetrics keeps the typed metric families in
// the state renderer, including a metric denominator.
func TestRenderState_RendersDimensionMetrics(t *testing.T) {
	s := stateFixture()
	s.Dimensions.Coupling.Metrics = []report.MetricValue{
		{Name: "scored_edges", Value: 7, Unit: "count", Denominator: &report.MetricDenominator{Observed: 7, Total: 9}},
	}
	out := renderState(t, s)
	for _, want := range []string{"## Dimension metrics", "### coupling", "`scored_edges`: 7 count (7/9)"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown state missing metric %q:\n%s", want, out)
		}
	}
}

// TestRenderState_ListsEveryDimension: the table always has nine rows, so an
// unmeasured envelope cannot be silently omitted.
func TestRenderState_ListsEveryDimension(t *testing.T) {
	out := renderState(t, stateFixture())
	for _, name := range []string{
		report.DimensionIntent, report.DimensionStructure, report.DimensionModularity,
		report.DimensionCoupling, report.DimensionChangeLocality, report.DimensionComplexity,
		report.DimensionTestability, report.DimensionOperations, report.DimensionDrift,
	} {
		if !strings.Contains(out, "| "+name+" | ") {
			t.Errorf("dimension row %q missing:\n%s", name, out)
		}
	}
}

// TestRenderState_CarriesNoRepositoryScore is the presentation half of the
// migration contract for Markdown.
func TestRenderState_CarriesNoRepositoryScore(t *testing.T) {
	out := renderState(t, stateFixture())
	for _, forbidden := range []string{"/ 100", "Why the score is low", "**Score:**", "**Decision:**"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("markdown headline carries %q: the architecture state has no repository score:\n%s", forbidden, out)
		}
	}
}

// TestRenderState_IsDeterministic: two renders of the same state must not
// differ, or the format cannot carry a byte-comparable baseline.
func TestRenderState_IsDeterministic(t *testing.T) {
	s := stateFixture()
	if first, second := renderState(t, s), renderState(t, s); first != second {
		t.Errorf("two renders differ:\n%s\n---\n%s", first, second)
	}
}
