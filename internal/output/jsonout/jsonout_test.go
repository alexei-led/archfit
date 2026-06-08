package jsonout_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/output/jsonout"
)

func TestJSONRenderer_Format(t *testing.T) {
	r := jsonout.New()
	if got := r.Format(); got != "json" {
		t.Errorf("Format() = %q, want %q", got, "json")
	}
}

func TestJSONRenderer_Render(t *testing.T) {
	tests := []struct {
		name    string
		diag    diagnostic.Diagnostic
		verdict diagnostic.Verdict
	}{
		{
			name:    "pass verdict",
			diag:    func() diagnostic.Diagnostic { d := diagnostic.New(); d.Verdict = diagnostic.VerdictPass; return d }(),
			verdict: diagnostic.VerdictPass,
		},
		{
			name:    "fail verdict",
			diag:    func() diagnostic.Diagnostic { d := diagnostic.New(); d.Verdict = diagnostic.VerdictFail; return d }(),
			verdict: diagnostic.VerdictFail,
		},
		{
			name:    "warn verdict",
			diag:    func() diagnostic.Diagnostic { d := diagnostic.New(); d.Verdict = diagnostic.VerdictWarn; return d }(),
			verdict: diagnostic.VerdictWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := jsonout.New()
			var buf bytes.Buffer

			if err := r.Render(tt.diag, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			// Output must be valid JSON.
			var raw map[string]any
			if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
				t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
			}

			// schema_version must be present.
			sv, ok := raw["schema_version"]
			if !ok {
				t.Fatal("schema_version field missing from JSON output")
			}
			if sv != diagnostic.SchemaVersion {
				t.Errorf("schema_version = %q, want %q", sv, diagnostic.SchemaVersion)
			}

			// Verify round-trip: unmarshal back into Diagnostic.
			var got diagnostic.Diagnostic
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
			}
			if got.Verdict != tt.verdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.verdict)
			}
			if got.SchemaVersion != diagnostic.SchemaVersion {
				t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, diagnostic.SchemaVersion)
			}

			// agent_tasks must serialize as [] not null.
			agentTasks, ok := raw["agent_tasks"]
			if !ok {
				t.Fatal("agent_tasks field missing from JSON output")
			}
			tasks, ok := agentTasks.([]any)
			if !ok {
				t.Fatalf("agent_tasks is not an array, got %T", agentTasks)
			}
			if len(tasks) != 0 {
				t.Errorf("agent_tasks len = %d, want 0", len(tasks))
			}
		})
	}
}
