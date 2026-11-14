package reportschema_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestReportSchemaRejectsRetiredRuleEvaluation(t *testing.T) {
	compiled := compileStateSchema(t)
	raw, err := json.Marshal(report.NewArchitectureState())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		parent, key string
		value       any
	}{
		{key: "verdict", value: "blocked"},
		{key: "decision", value: map[string]any{"hard_gates": "fail"}},
		{key: "agent_tasks", value: []any{}},
		{key: "advisory_tasks", value: []any{}},
		{parent: "dimensions.intent", key: "gate", value: "fail"},
	} {
		t.Run(tc.parent+"."+tc.key, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(document); err != nil {
				t.Fatalf("current report rejected: %v", err)
			}
			parent := document
			if tc.parent != "" {
				for _, key := range strings.Split(tc.parent, ".") {
					parent = parent[key].(map[string]any)
				}
			}
			parent[tc.key] = tc.value
			if err := compiled.Validate(document); err == nil {
				t.Fatalf("retired field %s.%s accepted", tc.parent, tc.key)
			}
		})
	}
}
