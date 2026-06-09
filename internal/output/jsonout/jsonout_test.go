package jsonout_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/output/jsonout"
)

const (
	metricCohesionSpread = "cohesion_spread"
	metricSharedStateHub = "shared_state_hub"
	bandInfo             = "info"
	bandNA               = "n/a"
	confidenceHigh       = "high"
	confidenceLow        = "low"
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

func TestJSONRenderer_Render_NewInfoMetrics(t *testing.T) {
	// Confirm cohesion_spread and shared_state_hub serialize to JSON with correct
	// name, display, band, and confidence fields, and round-trip cleanly.
	tests := []struct {
		name       string
		metric     diagnostic.MetricResult
		wantFields []string // substrings expected in raw JSON
	}{
		{
			name: "cohesion_spread present",
			metric: diagnostic.MetricResult{
				Name:       metricCohesionSpread,
				Display:    "2 high-spread file(s): pkg/handlers [spread 5 subsystems, LOC 320]",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantFields: []string{metricCohesionSpread, "spread 5 subsystems", bandInfo},
		},
		{
			name: "cohesion_spread n/a",
			metric: diagnostic.MetricResult{
				Name:       metricCohesionSpread,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantFields: []string{metricCohesionSpread, bandNA, confidenceLow},
		},
		{
			name: "shared_state_hub present",
			metric: diagnostic.MetricResult{
				Name:       metricSharedStateHub,
				Display:    "1 shared-state hub(s): pkg/polling [top_symbol fan-in=22, 3 hot]",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantFields: []string{metricSharedStateHub, "fan-in=22", bandInfo},
		},
		{
			name: "shared_state_hub n/a",
			metric: diagnostic.MetricResult{
				Name:       metricSharedStateHub,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantFields: []string{metricSharedStateHub, bandNA, confidenceLow},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Verdict = diagnostic.VerdictPass
			d.Metrics = []diagnostic.MetricResult{tt.metric}

			r := jsonout.New()
			var buf bytes.Buffer
			if err := r.Render(d, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			raw := buf.String()

			// Must be valid JSON.
			var parsed map[string]any
			if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v\noutput: %s", err, raw)
			}

			// All expected substrings must appear in the raw JSON.
			for _, want := range tt.wantFields {
				if !strings.Contains(raw, want) {
					t.Errorf("JSON output missing %q\nfull output: %s", want, raw)
				}
			}

			// Round-trip: metric must survive unmarshal with correct name.
			var got diagnostic.Diagnostic
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
			}
			if len(got.Metrics) != 1 {
				t.Fatalf("expected 1 metric after round-trip, got %d", len(got.Metrics))
			}
			if got.Metrics[0].Name != tt.metric.Name {
				t.Errorf("metric name = %q, want %q", got.Metrics[0].Name, tt.metric.Name)
			}
			if got.Metrics[0].Band != tt.metric.Band {
				t.Errorf("metric band = %q, want %q", got.Metrics[0].Band, tt.metric.Band)
			}
		})
	}
}
