package score

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

func TestTSImportsIncompleteCapsCouplingConfidence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		row  evidence.Coverage
		want bool
	}{
		{"unresolved import below legacy ratio", evidence.Coverage{Tool: toolTSImports, Status: evidence.StatusPartial, Unresolved: 1, UnresolvedInputsMissing: 1, SpecifiersSeen: 100}, true},
		{"unread configuration with no imports", evidence.Coverage{Tool: toolTSImports, Status: evidence.StatusPartial, Unresolved: 1, UnresolvedInputsMissing: 1}, true},
		{"parser failure", evidence.Coverage{Tool: toolTSImports, Status: evidence.StatusPartial, Reason: "parser failed"}, true},
		{"timed out", evidence.Coverage{Tool: toolTSImports, Status: evidence.StatusTimedOut}, true},
		{"complete", evidence.Coverage{Tool: toolTSImports, Status: evidence.StatusOK}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := result.New()
			d.ToolCoverage = []evidence.Coverage{tc.row}
			d.Metrics = []result.MetricResult{metric("blast_radius", 3, "info", "high")}
			d.ClassifiedEdges = &result.ClassifiedEdgeSummary{Total: 50, Scored: 50, MeanBalance: 9, BySeverity: map[string]int{sevLow: 50}}
			if got := TSUnresolvedPartial(d); got != tc.want {
				t.Fatalf("TSUnresolvedPartial = %v, want %v", got, tc.want)
			}
			cb := couplingBalanceDim(t, Synthesize(d))
			if tc.want && cb.Confidence == ConfidenceHigh {
				t.Fatalf("incomplete parser evidence has high confidence: %+v", cb)
			}
		})
	}
}
