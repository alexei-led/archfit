// Behavior tests for the hard-gate/diagnostic split the architecture state is
// decided from. They pin which findings enter which population, that a
// required-tool failure blocks without inventing a finding, and that the split
// agrees with the summary counters the existing output already publishes.
package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
)

// TestBuildStateOverNothingMeasuresNothing is the abstain-not-fake contract at
// the run level: a run with no policy, no graph, and no files must report nine
// unmeasured envelopes, never nine measured-and-empty ones.
func TestBuildStateOverNothingMeasuresNothing(t *testing.T) {
	t.Parallel()
	diag := result.Result{}
	st := evaluation.BuildState(&diag, evaluation.StateInput{})
	if _, _, unmeasured := st.Dimensions.CountStatuses(); unmeasured != state.DimensionCount {
		t.Errorf("unmeasured = %d, want %d", unmeasured, state.DimensionCount)
	}
}
