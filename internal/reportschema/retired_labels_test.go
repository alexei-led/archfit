package reportschema_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

const (
	identityParent = "identity"
	retiredValue   = "retired"
)

func TestStateSchemaRejectsRetiredLabelAndHistoryFields(t *testing.T) {
	compiled := compileStateSchema(t)
	base := report.NewArchitectureState()
	base.Verdict = report.StateHealthy
	base.Seams = []report.Seam{{ID: "a-b", FromModule: "a", ToModule: "b", Confidence: "high"}}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		parent, key string
		value       any
	}{
		{identityParent, "labels_digest", retiredValue},
		{"measurement", "history_depth", 12},
		{"measurement", "history_window", "full history"},
		{"seams", "labels", []any{"a\x00b"}},
		{"seams", "label_evidence_hash", retiredValue},
	} {
		t.Run(tc.parent+"."+tc.key, func(t *testing.T) {
			var state map[string]any
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(state); err != nil {
				t.Fatalf("current state rejected: %v", err)
			}
			parent, ok := state[tc.parent].(map[string]any)
			if !ok {
				parent = state[tc.parent].([]any)[0].(map[string]any)
			}
			parent[tc.key] = tc.value
			if err := compiled.Validate(state); err == nil {
				t.Fatalf("retired field %s.%s accepted", tc.parent, tc.key)
			}
		})
	}
}

func TestReportContractsRemoveLabelAndCorroborationFields(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		keys []string
	}{
		{reflect.TypeFor[report.Document](), []string{"volatility_corroboration"}},
		{reflect.TypeFor[report.ClassifiedEdgeSummary](), []string{"llm_approved", "labeled_llm"}},
	} {
		for field := range tc.typ.Fields() {
			key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			for _, retired := range tc.keys {
				if key == retired {
					t.Errorf("%s still publishes %s", tc.typ, key)
				}
			}
		}
	}
}
