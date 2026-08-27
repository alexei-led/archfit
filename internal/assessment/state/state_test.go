// Behavior tests for the architecture-state contract. They pin the two
// properties every later collector depends on: a fresh state measures nothing,
// and the nine envelopes always account for themselves exactly once.
package state_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/state"
)

// TestNewMeasuresNothing is the abstain-not-fake contract at the state level. A
// caller that has not run a collector must not be able to read a green result
// out of the envelope defaults.
func TestNewMeasuresNothing(t *testing.T) {
	t.Parallel()
	st := state.New()
	if st.Verdict != state.NeedsAttention {
		t.Errorf("verdict = %q, want %q: nine unknown dimensions cannot be healthy", st.Verdict, state.NeedsAttention)
	}
	if st.Decision.HardGates != state.HardGateUnmeasured {
		t.Errorf("hard gates = %q, want %q", st.Decision.HardGates, state.HardGateUnmeasured)
	}
	if st.Decision.UnknownDimensions != state.DimensionCount {
		t.Errorf("unknown dimensions = %d, want %d", st.Decision.UnknownDimensions, state.DimensionCount)
	}
	measured, partial, unmeasured := st.Dimensions.CountStatuses()
	if measured != 0 || partial != 0 || unmeasured != state.DimensionCount {
		t.Errorf("statuses = (%d, %d, %d), want (0, 0, %d)", measured, partial, unmeasured, state.DimensionCount)
	}
}

// TestNewNamesAndOwnsEveryDimension pins that no envelope ships anonymous: a
// dimension with no name or no evidence owner cannot be measured by anyone, and
// the report keys are derived from these names.
func TestNewNamesAndOwnsEveryDimension(t *testing.T) {
	t.Parallel()
	want := []string{
		state.DimensionIntent, state.DimensionStructure, state.DimensionModularity,
		state.DimensionCoupling, state.DimensionChangeLocality, state.DimensionComplexity,
		state.DimensionTestability, state.DimensionOperations, state.DimensionDrift,
	}
	all := state.New().Dimensions.All()
	if len(all) != state.DimensionCount {
		t.Fatalf("dimensions = %d, want %d", len(all), state.DimensionCount)
	}
	for i, dim := range all {
		if dim.Name != want[i] {
			t.Errorf("dimension %d = %q, want %q", i, dim.Name, want[i])
		}
		if dim.Owner == "" {
			t.Errorf("dimension %q has no evidence owner", dim.Name)
		}
		if dim.Confidence != state.ConfidenceUnrated || dim.Gate != state.GateNotApplicable {
			t.Errorf("dimension %q = (%q, %q), want an unrated, non-applicable default", dim.Name, dim.Confidence, dim.Gate)
		}
		if dim.Metrics == nil || dim.Findings == nil || dim.Unknown == nil {
			t.Errorf("dimension %q has a nil collection; serialization must be deterministic", dim.Name)
		}
		if len(dim.Unknown) == 0 || dim.Unknown[0].Owner != dim.Owner {
			t.Errorf("dimension %q is unmeasured without a reason owned by %q: %+v", dim.Name, dim.Owner, dim.Unknown)
		}
	}
}

// TestEachAddressesTheSameEnvelopes proves the pointer view collectors write
// through is the same nine envelopes, in the same order, that All() reports.
func TestEachAddressesTheSameEnvelopes(t *testing.T) {
	t.Parallel()
	st := state.New()
	each := st.Dimensions.Each()
	if len(each) != state.DimensionCount {
		t.Fatalf("Each = %d envelopes, want %d", len(each), state.DimensionCount)
	}
	for _, dim := range each {
		dim.Status = state.Measured
	}
	measured, _, _ := st.Dimensions.CountStatuses()
	if measured != state.DimensionCount {
		t.Fatalf("measured = %d after writing through Each, want %d", measured, state.DimensionCount)
	}
	for i, dim := range st.Dimensions.All() {
		if dim.Name != each[i].Name {
			t.Errorf("order mismatch at %d: All=%q Each=%q", i, dim.Name, each[i].Name)
		}
	}
}

// TestCountStatusesAlwaysSumsToNine covers the coverage-summary invariant over
// every status mix, including an envelope left at an unrecognised status: it
// counts as unmeasured rather than vanishing from the total.
func TestCountStatusesAlwaysSumsToNine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                                string
		statuses                            []state.MeasurementStatus
		measured, partial, unmeasuredWanted int
	}{
		{"all unmeasured", nil, 0, 0, 9},
		{"all measured", repeat(state.Measured, 9), 9, 0, 0},
		{"mixed", []state.MeasurementStatus{
			state.Measured, state.Measured, state.Partial, state.Unmeasured, state.Measured,
			state.Partial, state.Unmeasured, state.Measured, state.Measured,
		}, 5, 2, 2},
		{"unrecognised status counts as unmeasured", []state.MeasurementStatus{"bogus"}, 0, 0, 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := state.New()
			for i, status := range tc.statuses {
				st.Dimensions.Each()[i].Status = status
			}
			measured, partial, unmeasured := st.Dimensions.CountStatuses()
			if measured != tc.measured || partial != tc.partial || unmeasured != tc.unmeasuredWanted {
				t.Errorf("statuses = (%d, %d, %d), want (%d, %d, %d)",
					measured, partial, unmeasured, tc.measured, tc.partial, tc.unmeasuredWanted)
			}
			if measured+partial+unmeasured != state.DimensionCount {
				t.Errorf("counts sum to %d, want %d", measured+partial+unmeasured, state.DimensionCount)
			}
		})
	}
}

func repeat(s state.MeasurementStatus, n int) []state.MeasurementStatus {
	out := make([]state.MeasurementStatus, n)
	for i := range out {
		out[i] = s
	}
	return out
}
