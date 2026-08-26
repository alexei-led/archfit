package report_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

// TestStateSchemaVersionIsPinned pins the published contract identifier. It is
// consumed by every downstream reader, so it changes only with a deliberate
// contract migration.
func TestStateSchemaVersionIsPinned(t *testing.T) {
	if report.StateSchemaVersion != "archfit.architecture-state.v1" {
		t.Fatalf("StateSchemaVersion = %q, want archfit.architecture-state.v1", report.StateSchemaVersion)
	}
}

// TestNewArchitectureStateIsHonestlyUnmeasured asserts the zero contract claims
// nothing: nine named, owned, unmeasured envelopes and no measured coverage. A
// caller that measures nothing must not be able to publish a green result.
func TestNewArchitectureStateIsHonestlyUnmeasured(t *testing.T) {
	state := report.NewArchitectureState()

	if state.SchemaVersion != report.StateSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", state.SchemaVersion, report.StateSchemaVersion)
	}
	if state.Verdict == report.StateHealthy {
		t.Error("an unmeasured state must never construct as healthy")
	}
	if state.Decision.HardGates != report.HardGateUnmeasured {
		t.Errorf("Decision.HardGates = %q, want unmeasured", state.Decision.HardGates)
	}
	if state.Comparison.Status != report.ComparisonNotRequested {
		t.Errorf("Comparison.Status = %q, want not_requested", state.Comparison.Status)
	}

	want := []struct{ name, owner string }{
		{report.DimensionIntent, report.OwnerIntent},
		{report.DimensionStructure, report.OwnerStructure},
		{report.DimensionModularity, report.OwnerModularity},
		{report.DimensionCoupling, report.OwnerCoupling},
		{report.DimensionChangeLocality, report.OwnerChangeLocality},
		{report.DimensionComplexity, report.OwnerComplexity},
		{report.DimensionTestability, report.OwnerTestability},
		{report.DimensionOperations, report.OwnerOperations},
		{report.DimensionDrift, report.OwnerDrift},
	}
	dims := state.Dimensions.All()
	if len(dims) != report.DimensionCount {
		t.Fatalf("Dimensions.All() = %d envelopes, want %d", len(dims), report.DimensionCount)
	}
	for i, dim := range dims {
		if dim.Name != want[i].name || dim.Owner != want[i].owner {
			t.Errorf("dimension %d = (%q, %q), want (%q, %q)", i, dim.Name, dim.Owner, want[i].name, want[i].owner)
		}
		if dim.Status != report.MeasurementUnmeasured {
			t.Errorf("%s status = %q, want unmeasured", dim.Name, dim.Status)
		}
		if dim.Confidence != report.ConfidenceUnrated {
			t.Errorf("%s confidence = %q, want unrated", dim.Name, dim.Confidence)
		}
		if dim.Gate != report.GateNotApplicable {
			t.Errorf("%s gate = %q, want not_applicable", dim.Name, dim.Gate)
		}
		if dim.Metrics == nil || dim.Findings == nil || dim.Unknown == nil {
			t.Errorf("%s has a nil collection: metrics=%v findings=%v unknown=%v",
				dim.Name, dim.Metrics == nil, dim.Findings == nil, dim.Unknown == nil)
		}
	}
}

// TestCoverageCountsSumToNine is the arithmetic the contract guarantees: every
// dimension lands in exactly one status bucket, so the three counts always sum
// to DimensionCount whatever the mix.
func TestCoverageCountsSumToNine(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		mutate                              func(*report.Dimensions)
		measured, partial, unmeasuredWanted int
	}{
		{name: "all unmeasured", mutate: func(*report.Dimensions) {}, unmeasuredWanted: 9},
		{
			name: "one measured one partial",
			mutate: func(d *report.Dimensions) {
				d.Intent.Status, d.Coupling.Status = report.MeasurementMeasured, report.MeasurementPartial
			},
			measured: 1, partial: 1, unmeasuredWanted: 7,
		},
		{
			name: "all measured",
			mutate: func(d *report.Dimensions) {
				d.Intent.Status, d.Structure.Status, d.Modularity.Status = report.MeasurementMeasured, report.MeasurementMeasured, report.MeasurementMeasured
				d.Coupling.Status, d.ChangeLocality.Status, d.Complexity.Status = report.MeasurementMeasured, report.MeasurementMeasured, report.MeasurementMeasured
				d.Testability.Status, d.Operations.Status, d.Drift.Status = report.MeasurementMeasured, report.MeasurementMeasured, report.MeasurementMeasured
			},
			measured: 9,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := report.NewArchitectureState()
			tc.mutate(&state.Dimensions)
			measured, partial, unmeasured := state.Dimensions.CountStatuses()
			if measured != tc.measured || partial != tc.partial || unmeasured != tc.unmeasuredWanted {
				t.Errorf("CountStatuses() = (%d, %d, %d), want (%d, %d, %d)",
					measured, partial, unmeasured, tc.measured, tc.partial, tc.unmeasuredWanted)
			}
			if sum := measured + partial + unmeasured; sum != report.DimensionCount {
				t.Errorf("coverage counts sum to %d, want %d", sum, report.DimensionCount)
			}
		})
	}
}

// TestArchitectureStateSerializationIsDeterministic asserts two encodings of the
// same value are byte-identical and that the nine dimension keys keep their
// declared order. Ordering is why Dimensions is a struct and not a map.
func TestArchitectureStateSerializationIsDeterministic(t *testing.T) {
	state := report.NewArchitectureState()
	state.Measurement.ToolVersions = map[string]string{"go/packages": "1.24", "loc": "1"}

	first, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("encodings differ:\nfirst:  %s\nsecond: %s", first, second)
	}

	wantOrder := []string{
		`"intent"`, `"structure"`, `"modularity"`, `"coupling"`, `"change_locality"`,
		`"complexity"`, `"testability"`, `"operations"`, `"drift"`,
	}
	body := string(first)
	at := 0
	for _, key := range wantOrder {
		idx := strings.Index(body[at:], key)
		if idx < 0 {
			t.Fatalf("dimension key %s missing or out of contract order in:\n%s", key, body)
		}
		at += idx + len(key)
	}
}

// TestArchitectureStateCarriesNoRepositoryScalar is the structural guard behind
// the whole migration: the state contract must not regrow a decisive
// repository-level number. A scorecard type or an "overall" field reachable
// from ArchitectureState fails here.
func TestArchitectureStateCarriesNoRepositoryScalar(t *testing.T) {
	forbidden := map[string]bool{"overall": true, "overall_band": true, "score": true, "band": true}
	walkStateFields(t, reflect.TypeOf(report.ArchitectureState{}), func(path string, field reflect.StructField, tag string) {
		if forbidden[tag] {
			t.Errorf("%s.%s serialises as %q: the architecture state has no repository scalar", path, field.Name, tag)
		}
	})
}

// TestArchitectureStateFieldsAreVersioned fails when a new field joins the
// contract without a stable serialization rule. Every exported field reachable
// from ArchitectureState must carry an explicit json tag, so no field can enter
// the published schema under an accidental Go-derived name.
func TestArchitectureStateFieldsAreVersioned(t *testing.T) {
	walkStateFields(t, reflect.TypeOf(report.ArchitectureState{}), func(path string, field reflect.StructField, tag string) {
		if tag == "" {
			t.Errorf("%s.%s has no json tag: every architecture-state field needs an explicit wire name", path, field.Name)
		}
	})
}

// walkStateFields visits every exported field of every struct reachable from t,
// reporting the field's json name (empty when the tag is absent).
func walkStateFields(t *testing.T, typ reflect.Type, visit func(path string, field reflect.StructField, tag string)) {
	t.Helper()
	seen := map[reflect.Type]bool{}

	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, path string) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			visit(path, field, tag)
			walk(field.Type, path+"."+field.Name)
		}
	}
	walk(typ, typ.Name())
}
