package analysis_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
)

// TestCouplingAdvisoryNeverCallsACriticalEdgeCheap pins the advisory wording
// at every severity and distance: a critical edge is the worst band the book
// scorer gives, so no text may call it cheap to change.
func TestCouplingAdvisoryNeverCallsACriticalEdgeCheap(t *testing.T) {
	for _, sev := range []relationship.Severity{relationship.SeverityCritical, relationship.SeverityHigh, relationship.SeverityMedium} {
		for _, dist := range []relationship.Distance{relationship.DistanceCrossModuleSameOwner, relationship.DistanceCrossDeployUnit} {
			why := analysis.BCAdvisoryWhy(relationship.Edge{
				Strength: relationship.StrengthModel, Distance: dist, Volatility: relationship.VolatilityHigh, Severity: sev,
			})
			if strings.Contains(why, "cheap") {
				t.Errorf("%s at %s: why calls the edge cheap: %s", sev, dist, why)
			}
		}
	}
}
