package application

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// gitSource is the volatility-corroboration source name used by the fixtures.
const gitSource = "git"

// TestMeasurementCarriesOnlyDeterministicFields pins the measurement block's
// field set. Every field here must be a property of the measured tree, the
// tools, or the history window — never of the run that happened to produce it.
//
// The failure this prevents is silent: one wall-clock timestamp, absolute path,
// or process ID added to this struct makes two identical runs differ in bytes,
// which retires the byte-identity contract every format baseline depends on
// without any test naming the cause.
func TestMeasurementCarriesOnlyDeterministicFields(t *testing.T) {
	want := []string{"history_depth", "history_window", "source_ref", "tool_versions"}

	typ := reflect.TypeFor[report.StateMeasurement]()
	got := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		got = append(got, name)
	}
	sort.Strings(got)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("StateMeasurement fields = %v, want %v.\n"+
			"A new field belongs here only if two identical runs over the same tree "+
			"produce the same value for it; anything run-specific is a diagnostic, not a measurement.",
			got, want)
	}
}

// TestMeasurementNamesTheTreeAndTheWindow asserts the measurement block always
// says what was measured and over what history window, including when there is
// no history at all.
//
// An empty window and a zero depth are indistinguishable from "the field was
// never populated", so a repository with no git history would publish the same
// bytes as one whose scan was simply never wired up.
func TestMeasurementNamesTheTreeAndTheWindow(t *testing.T) {
	t.Run("every run measures the worktree, even with a head ref", func(t *testing.T) {
		measurement := projectReport(stateFixture(), score.Scorecard{}).State.Measurement
		if measurement.SourceRef != "worktree" {
			t.Errorf("SourceRef = %q, want worktree: archfit measures files on disk, "+
				"and naming a commit would claim the bytes equal it even on a dirty tree", measurement.SourceRef)
		}
	})

	t.Run("no history records the window it did not cover", func(t *testing.T) {
		diagnostic := stateFixture()
		diagnostic.VolatilityCorroboration = nil

		state := projectReport(diagnostic, score.Scorecard{}).State
		if state.Measurement.HistoryWindow != "unavailable" {
			t.Errorf("HistoryWindow = %q, want unavailable", state.Measurement.HistoryWindow)
		}
		if state.Measurement.HistoryDepth != 0 {
			t.Errorf("HistoryDepth = %d, want 0", state.Measurement.HistoryDepth)
		}
		if state.Dimensions.ChangeLocality.Status != report.MeasurementUnmeasured {
			t.Errorf("ChangeLocality.Status = %q, want unmeasured", state.Dimensions.ChangeLocality.Status)
		}
	})

	t.Run("a shallow window is recorded, not rounded away", func(t *testing.T) {
		diagnostic := stateFixture()
		diagnostic.VolatilityCorroboration = &evidence.VolatilityCorroboration{
			Source: gitSource, Status: evidence.StatusPartial, CommitWindow: 20, CommitsScanned: 3,
		}

		measurement := projectReport(diagnostic, score.Scorecard{}).State.Measurement
		if measurement.HistoryWindow != "20 commits" || measurement.HistoryDepth != 3 {
			t.Errorf("window/depth = %q/%d, want 20 commits/3", measurement.HistoryWindow, measurement.HistoryDepth)
		}
	})
}
