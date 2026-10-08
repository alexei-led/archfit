package application

import (
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func accepted(ids ...string) []BaselineFinding {
	out := make([]BaselineFinding, 0, len(ids))
	for _, id := range ids {
		out = append(out, BaselineFinding{Fingerprint: id, RuleID: "r"})
	}
	return out
}

func current(id string, keys ...string) findingKeys {
	if len(keys) == 0 {
		keys = []string{id}
	}
	return findingKeys{ReanchorFinding: ReanchorFinding{ID: id, RuleID: "r"}, keys: keys}
}

func fingerprints(in []BaselineFinding) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, a.Fingerprint)
	}
	return out
}

func notAcceptedIDs(in []ReanchorFinding) []string {
	out := make([]string, 0, len(in))
	for _, f := range in {
		out = append(out, f.ID)
	}
	return out
}

func TestReanchorAcceptsOnlyStoredDebt(t *testing.T) {
	tests := []struct {
		name            string
		capture         []string
		waived          []string
		current         []findingKeys
		stored          []string
		wantAccepted    []string
		wantNotAccepted []string
		wantDropped     []string
	}{
		{
			name:    "a new finding stays new",
			capture: []string{"a", "b"}, current: []findingKeys{current("a"), current("b")}, stored: []string{"a"},
			wantAccepted: []string{"a"}, wantNotAccepted: []string{"b"}, wantDropped: []string{},
		},
		{
			name:    "stored debt no longer observed is dropped and listed",
			capture: []string{"a"}, current: []findingKeys{current("a")}, stored: []string{"a", "x"},
			wantAccepted: []string{"a"}, wantNotAccepted: []string{}, wantDropped: []string{"x"},
		},
		{
			// A new edge in an old Balanced Coupling group is new debt: only the
			// stored edge is accepted, and the group is listed for review.
			name:    "a new edge in an accepted group stays new",
			capture: []string{"m1", "m2"}, current: []findingKeys{current("m1", "m1", "m2")}, stored: []string{"m1"},
			wantAccepted: []string{"m1"}, wantNotAccepted: []string{"m2"}, wantDropped: []string{},
		},
		{
			// check ranks a baselined status above a waiver, so stored debt a
			// waiver also covers stays accepted, never turns temporary.
			name:    "stored debt a waiver also covers stays accepted",
			capture: []string{}, waived: []string{"w", "x"}, stored: []string{"w"},
			wantAccepted: []string{"w"}, wantNotAccepted: []string{}, wantDropped: []string{},
		},
		{
			name:    "an empty stored file accepts nothing",
			capture: []string{"a"}, current: []findingKeys{current("a")}, stored: nil,
			wantAccepted: []string{}, wantNotAccepted: []string{"a"}, wantDropped: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, rep := reanchor(BaselineSnapshot{Accepted: accepted(tc.capture...)}, accepted(tc.waived...), tc.current, nil,
				StoredBaseline{Accepted: accepted(tc.stored...)}, nil)
			if ids := fingerprints(got.Accepted); !slices.Equal(ids, tc.wantAccepted) {
				t.Errorf("accepted = %v, want %v", ids, tc.wantAccepted)
			}
			if rep.Kept != len(tc.wantAccepted) {
				t.Errorf("kept = %d, want %d", rep.Kept, len(tc.wantAccepted))
			}
			if ids := notAcceptedIDs(rep.NotAccepted); !slices.Equal(ids, tc.wantNotAccepted) {
				t.Errorf("not accepted = %v, want %v", ids, tc.wantNotAccepted)
			}
			if ids := fingerprints(rep.Dropped); !slices.Equal(ids, tc.wantDropped) {
				t.Errorf("dropped = %v, want %v", ids, tc.wantDropped)
			}
		})
	}
}

// A seam that qualifies only in the new epoch must stay new: storing the
// current seams would accept it silently.
func TestReanchorIntersectsTheStateReference(t *testing.T) {
	capture := BaselineSnapshot{
		Accepted: accepted("a", "b"),
		State:    &BaselineStateSnapshot{QualifyingSeamIDs: []string{"s1", "s3"}, HardGateFindingIDs: []string{"a", "b"}},
	}
	got, rep := reanchor(capture, nil, []findingKeys{current("a"), current("b")}, nil, StoredBaseline{
		Accepted: accepted("a"), QualifyingSeamIDs: []string{"s1", "s2"},
	}, nil)
	if !slices.Equal(got.State.QualifyingSeamIDs, []string{"s1"}) {
		t.Errorf("qualifying seams = %v, want only the stored seam that still qualifies", got.State.QualifyingSeamIDs)
	}
	if !slices.Equal(rep.DroppedSeams, []string{"s2"}) {
		t.Errorf("dropped seams = %v, want [s2]", rep.DroppedSeams)
	}
	if !slices.Equal(rep.NewSeams, []string{"s3"}) {
		t.Errorf("new seams = %v, want [s3] reported, not hidden", rep.NewSeams)
	}
	if !slices.Equal(got.State.HardGateFindingIDs, []string{"a"}) {
		t.Errorf("hard-gate IDs = %v, want only the accepted blocker", got.State.HardGateFindingIDs)
	}
	if !slices.Equal(capture.State.QualifyingSeamIDs, []string{"s1", "s3"}) {
		t.Error("reanchor rewrote the capture it filters")
	}
}

// Metric names of the worsened-metrics table: one higher-is-better, one
// higher-is-worse.
const (
	metricCoverage = "coverage"
	metricCycles   = "cycles"
	metricRescored = "rescored"
)

func TestReanchorReportsWorsenedMetrics(t *testing.T) {
	const version = "v1"
	stored := report.MetricSnapshot{}
	for name, v := range map[string]float64{metricCycles: 1, metricCoverage: 0.8, "same": 3, "na": 4, metricRescored: 1, "undirected": 1} {
		ver := version
		if name == metricRescored {
			ver = "v0"
		}
		stored[name] = struct {
			Value   float64 `json:"value"`
			Version string  `json:"version"`
		}{Value: v, Version: ver}
	}
	metrics := []report.MetricResult{
		{Name: metricCycles, Value: 2, Version: version, Direction: report.DirectionHigherIsWorse},
		{Name: metricCoverage, Value: 0.7, Version: version, Direction: report.DirectionHigherIsBetter},
		{Name: "same", Value: 3, Version: version, Direction: report.DirectionHigherIsWorse},
		{Name: "unstored", Value: 9, Version: version, Direction: report.DirectionHigherIsWorse},
		// An unmeasured metric reads 0; it is not a regression.
		{Name: "na", Value: 0, Version: version, Band: string(report.ScoreBandNA), Direction: report.DirectionHigherIsBetter},
		// Another formula: the values do not compare.
		{Name: metricRescored, Value: 9, Version: version, Direction: report.DirectionHigherIsWorse},
		{Name: "undirected", Value: 0, Version: version},
	}
	_, rep := reanchor(BaselineSnapshot{}, nil, nil, metrics, StoredBaseline{Metrics: stored}, nil)
	want := []MetricChange{{Name: metricCycles, Before: 1, After: 2}, {Name: metricCoverage, Before: 0.8, After: 0.7}}
	if !slices.Equal(rep.WorsenedMetrics, want) {
		t.Errorf("worsened = %+v, want %+v", rep.WorsenedMetrics, want)
	}
}

// Re-anchoring onto its own output changes nothing.
func TestReanchorIsIdempotent(t *testing.T) {
	capture := BaselineSnapshot{Accepted: accepted("a", "b", "c"), State: &BaselineStateSnapshot{QualifyingSeamIDs: []string{"s1"}}}
	cur := []findingKeys{current("a"), current("b"), current("c")}
	first, _ := reanchor(capture, nil, cur, nil, StoredBaseline{Accepted: accepted("a", "c"), QualifyingSeamIDs: []string{"s1"}}, nil)
	second, rep := reanchor(capture, nil, cur, nil, StoredBaseline{Accepted: first.Accepted, QualifyingSeamIDs: first.State.QualifyingSeamIDs}, nil)
	if !slices.Equal(fingerprints(first.Accepted), fingerprints(second.Accepted)) ||
		!slices.Equal(first.State.QualifyingSeamIDs, second.State.QualifyingSeamIDs) || len(rep.Dropped) != 0 {
		t.Errorf("second re-anchor differs: first %v, second %v, dropped %v", fingerprints(first.Accepted), fingerprints(second.Accepted), rep.Dropped)
	}
}
