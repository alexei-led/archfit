package reportschema_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestMeasurementPartialBasisVocabulary(t *testing.T) {
	compiled := compileStateSchema(t)
	for _, tc := range []struct {
		basis string
		valid bool
	}{{"degraded_precision", true}, {"unresolved_specifiers", true}, {"invented_precision", false}} {
		t.Run(tc.basis, func(t *testing.T) {
			doc := report.NewArchitectureState()
			doc.Verdict = report.StateNeedsAttention
			doc.Comparison.MeasurementProfile = &report.MeasurementProfile{
				Version: "archfit.measurement.v1", SettingsHash: "fixture",
				Producers: []report.MeasurementProducer{{Tool: "go/packages", SemanticsVersion: "go/packages.v2", ToolVersion: "go1.26.0", Status: "partial", PartialBasis: tc.basis}},
				Unknowns:  []string{},
			}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var instance any
			if err := json.Unmarshal(raw, &instance); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(instance); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, want %v: %v", err == nil, tc.valid, err)
			} else if !tc.valid && !strings.Contains(err.Error(), "partial_basis") {
				t.Fatalf("unexpected validation failure: %v", err)
			}
		})
	}
}
