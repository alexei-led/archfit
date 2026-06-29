package jsonout_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/output/jsonout"
)

// TestJSONRenderer_AdvisoryScoreFields asserts the numeric BC score fields
// (score_value + score_band) on an advisory's matched_by survive JSON encoding.
func TestJSONRenderer_AdvisoryScoreFields(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Findings = []finding.Finding{{
		ID:     "adv1",
		Kind:   "advisory",
		RuleID: "bc/imbalanced_coupling",
		MatchedBy: map[string]string{
			"score":       "multiplicative",
			"score_value": "7",
			"score_band":  "high",
		},
	}}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal: %v", err)
	}
	mb := got.Findings[0].MatchedBy
	if mb["score_value"] != "7" {
		t.Errorf("matched_by.score_value = %q, want %q", mb["score_value"], "7")
	}
	if mb["score_band"] != "high" {
		t.Errorf("matched_by.score_band = %q, want %q", mb["score_band"], "high")
	}
}

// TestJSONRenderer_Delta verifies the delta block round-trips with snake_case
// bucket keys, omits empty buckets, and is omitted entirely when nil.
func TestJSONRenderer_Delta(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictWarn
	d.Delta = &diagnostic.DeltaReport{
		New:      []string{"n1"},
		Resolved: []string{"r1"},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Delta map[string]any `json:"delta"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if raw.Delta == nil {
		t.Fatal("delta block missing from JSON output")
	}
	if _, ok := raw.Delta["new"]; !ok {
		t.Error("delta.new missing")
	}
	if _, ok := raw.Delta["resolved"]; !ok {
		t.Error("delta.resolved missing")
	}
	if _, ok := raw.Delta["existing"]; ok {
		t.Error("delta.existing should be omitted when empty")
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.Delta == nil || len(got.Delta.New) != 1 || got.Delta.New[0] != "n1" {
		t.Errorf("round-trip delta = %+v, want New=[n1]", got.Delta)
	}

	// Omitted entirely when nil.
	plain := diagnostic.New()
	var pbuf bytes.Buffer
	if err := jsonout.New().Render(plain, &pbuf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if bytes.Contains(pbuf.Bytes(), []byte("\"delta\"")) {
		t.Errorf("delta should be omitted when nil\noutput: %s", pbuf.String())
	}
}

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

			// file_facts must serialize as [] not null.
			fileFacts, ok := raw["file_facts"]
			if !ok {
				t.Fatal("file_facts field missing from JSON output")
			}
			if _, ok := fileFacts.([]any); !ok {
				t.Fatalf("file_facts is not an array, got %T", fileFacts)
			}
		})
	}
}

// TestJSONRenderer_Render_FileFacts verifies the full facts block round-trips
// through JSON with snake_case keys.
func TestJSONRenderer_Render_FileFacts(t *testing.T) {
	d := diagnostic.New()
	d.FileFacts = []diagnostic.FileFact{
		{
			Module:               "tui.polling_state",
			Files:                []string{"src/tui/polling_state.py"},
			InboundModuleFanIn:   23,
			OutboundDestinations: 2,
			LOC:                  310,
		},
		{
			Module: "config",
			Files:  []string{},
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		FileFacts []map[string]any `json:"file_facts"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(raw.FileFacts) != 2 {
		t.Fatalf("file_facts len = %d, want 2", len(raw.FileFacts))
	}

	first := raw.FileFacts[0]
	if first["module"] != "tui.polling_state" {
		t.Errorf("module = %v, want tui.polling_state", first["module"])
	}
	if first["inbound_module_fanin"] != float64(23) {
		t.Errorf("inbound_module_fanin = %v, want 23", first["inbound_module_fanin"])
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.FileFacts[0].LOC != 310 {
		t.Errorf("round-trip LOC = %d, want 310", got.FileFacts[0].LOC)
	}
}
