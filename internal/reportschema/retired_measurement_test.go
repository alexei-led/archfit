package reportschema_test

import (
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestStateSchemaRejectsRetiredMeasurementFields(t *testing.T) {
	compiled := compileStateSchema(t)
	base := report.NewArchitectureState()
	base.Findings = []report.Finding{{ID: "advisory", Kind: report.FindingKindAdvisory, Status: report.FindingStatusNew, MatchedBy: map[string]string{}, Locations: []report.Location{}}}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, parent, key string
		value             any
	}{
		{name: "base comparison", key: "comparison", value: map[string]any{"status": "not_requested", "reasons": []any{}}},
		{name: "profile hash", parent: identityParent, key: "measurement_profile_hash", value: retiredValue},
		{name: "finding origin", parent: "findings", key: "origin", value: "introduced"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(document); err != nil {
				t.Fatalf("current document is invalid: %v", err)
			}
			parent := document
			if tc.parent != "" {
				switch value := document[tc.parent].(type) {
				case map[string]any:
					parent = value
				case []any:
					parent = value[0].(map[string]any)
				}
			}
			parent[tc.key] = tc.value
			if err := compiled.Validate(document); err == nil {
				t.Fatalf("retired field %s.%s was accepted", tc.parent, tc.key)
			}
		})
	}
}
