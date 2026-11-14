package application

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestMeasurementCarriesOnlyDeterministicFields(t *testing.T) {
	want := []string{"source_ref", "tool_versions"}
	typ := reflect.TypeFor[report.StateMeasurement]()
	got := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		got = append(got, name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("StateMeasurement fields = %v, want %v", got, want)
	}
}

func TestMeasurementNamesTheTreeWithoutInventingHistory(t *testing.T) {
	state := projectReport(stateFixture(), score.Scorecard{}).State
	if state.Measurement.SourceRef != "worktree" {
		t.Errorf("SourceRef = %q, want worktree", state.Measurement.SourceRef)
	}
	if state.Dimensions.ChangeLocality.Status != report.MeasurementUnmeasured ||
		len(state.Dimensions.ChangeLocality.Metrics) != 0 ||
		len(state.Dimensions.ChangeLocality.Unknown) == 0 {
		t.Errorf("change locality claims uncollected history: %+v", state.Dimensions.ChangeLocality)
	}
}
