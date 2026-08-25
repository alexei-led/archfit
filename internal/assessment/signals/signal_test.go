package signal

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// TestNewCoverageViewNarrowsAcquiredRows pins the projection metrics read:
// only the fields a metric may reason about cross the boundary, and each one
// arrives with its acquired value. A dropped field here silently degrades every
// coverage-aware metric to "unmeasured".
func TestNewCoverageViewNarrowsAcquiredRows(t *testing.T) {
	rows := []evidence.Coverage{
		{
			Tool: "scip", Status: evidence.StatusPartial, FilesSeen: 10, FilesApplicable: 12,
			Unresolved: 2, SpecifiersSeen: 40,
			// Not part of the metric contract: it must not appear on the view.
			Reason: "empty index",
		},
		{Tool: "go/packages", Status: evidence.StatusOK},
	}

	got := NewCoverageView(rows)

	if len(got) != len(rows) {
		t.Fatalf("view length = %d, want %d", len(got), len(rows))
	}
	want := CoverageRecord{
		Tool: "scip", Status: evidence.StatusPartial, FilesSeen: 10, FilesApplicable: 12,
		Unresolved: 2, SpecifiersSeen: 40,
	}
	if got[0] != want {
		t.Errorf("coverage record = %+v, want %+v", got[0], want)
	}
	if got[1] != (CoverageRecord{Tool: "go/packages", Status: evidence.StatusOK}) {
		t.Errorf("second record = %+v", got[1])
	}
}

func TestNewCoverageViewOnNoRows(t *testing.T) {
	if got := NewCoverageView(nil); len(got) != 0 {
		t.Errorf("view over no rows = %+v, want empty", got)
	}
}
