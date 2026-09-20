package application

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
)

func TestBaselineReferenceDistinguishesMissingFileAndMissingState(t *testing.T) {
	for _, tc := range []struct {
		present bool
		reason  string
	}{
		{false, "no baseline file was loaded"},
		{true, "stored baseline has no architecture-state snapshot"},
	} {
		got := baselineComparison(Baseline{Present: tc.present}, headContext())
		if got.Status != result.StateComparisonNonComparable || len(got.Reasons) != 1 || got.Reasons[0] != tc.reason {
			t.Fatalf("present=%v comparison=%+v", tc.present, got)
		}
	}
}
