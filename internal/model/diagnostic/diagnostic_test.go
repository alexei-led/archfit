package diagnostic_test

import (
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// gateWarn is the default coverage-gap gate value reused across these tests.
const gateWarn = "warn"

func TestNew_ZeroValue(t *testing.T) {
	d := diagnostic.New()
	if d.SchemaVersion != diagnostic.SchemaVersion {
		t.Errorf("schema_version = %q; want %q", d.SchemaVersion, diagnostic.SchemaVersion)
	}
	if d.SchemaVersion != "archfit.diagnostic.v1" {
		t.Errorf("schema_version = %q; want \"archfit.diagnostic.v1\"", d.SchemaVersion)
	}
	if d.Verdict != "" {
		t.Errorf("verdict = %q; want empty", d.Verdict)
	}
	if d.Metrics == nil {
		t.Error("Metrics must be non-nil (empty slice, not nil)")
	}
	if d.Findings == nil {
		t.Error("Findings must be non-nil (empty slice, not nil)")
	}
	if d.AgentTasks == nil {
		t.Error("AgentTasks must be non-nil (empty slice, not nil)")
	}
	if d.ToolCoverage == nil {
		t.Error("ToolCoverage must be non-nil (empty slice, not nil)")
	}
}

func TestDiagnostic_JSONFieldNames(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Base = "main"
	d.Head = "abc123"

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	requiredFields := []string{
		"schema_version",
		"verdict",
		"base",
		"head",
		"metrics",
		"findings",
		"agent_tasks",
		"tool_coverage",
		"summary",
	}
	for _, f := range requiredFields {
		if _, ok := m[f]; !ok {
			t.Errorf("JSON field %q missing", f)
		}
	}
}

func TestAgentTasks_SerializesAsEmptyArray(t *testing.T) {
	d := diagnostic.New()

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	raw, ok := m["agent_tasks"]
	if !ok {
		t.Fatal("agent_tasks field missing from JSON")
	}
	if string(raw) != "[]" {
		t.Errorf("agent_tasks = %s; want []", raw)
	}
}

func TestVerdict_Constants(t *testing.T) {
	tests := []struct {
		v    diagnostic.Verdict
		want string
	}{
		{diagnostic.VerdictPass, "pass"},
		{diagnostic.VerdictFail, "fail"},
		{diagnostic.VerdictWarn, gateWarn},
	}
	for _, tt := range tests {
		if string(tt.v) != tt.want {
			t.Errorf("Verdict %q = %q; want %q", tt.v, string(tt.v), tt.want)
		}
	}
}

func TestMetricResult_JSONFieldNames(t *testing.T) {
	delta := 0.5
	mr := diagnostic.MetricResult{
		Name:       "encapsulation",
		Value:      8.5,
		Display:    "8.5/10",
		Band:       "serviceable",
		Confidence: "high",
		Version:    "encapsulation.v1",
		Mode:       "delta",
		Definition: "contract cross-boundary edges / all cross-boundary edges",
		Delta:      &delta,
	}

	data, err := json.Marshal(mr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	requiredFields := []string{
		"name", "value", "display", "band", "confidence",
		"metric_version", "mode", "definition", "delta",
	}
	for _, f := range requiredFields {
		if _, ok := m[f]; !ok {
			t.Errorf("MetricResult JSON field %q missing", f)
		}
	}

	// Confirm "version" is NOT used (must be "metric_version")
	if _, ok := m["version"]; ok {
		t.Error("MetricResult must not emit \"version\"; field must be \"metric_version\"")
	}
}

func TestMetricResult_DeltaOmitEmpty(t *testing.T) {
	mr := diagnostic.MetricResult{
		Name:    "encapsulation",
		Version: "encapsulation.v1",
	}

	data, err := json.Marshal(mr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := m["delta"]; ok {
		t.Error("delta must be omitted when nil")
	}
}

func TestSummary_JSONFieldNames(t *testing.T) {
	s := diagnostic.Summary{
		GateFindings:   3,
		Warnings:       1,
		ExceptionsUsed: 2,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, f := range []string{"gate_findings", "warnings", "exceptions_used"} {
		if _, ok := m[f]; !ok {
			t.Errorf("Summary JSON field %q missing", f)
		}
	}
}

func TestCoverage_JSONFieldNames(t *testing.T) {
	c := diagnostic.Coverage{
		Tool:            "dependency-cruiser",
		Version:         "13.0.0",
		FilesSeen:       42,
		FilesApplicable: 40,
		Unresolved:      2,
		Status:          "ok",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, f := range []string{"tool", "version", "files_seen", "files_applicable", "unresolved", "status"} {
		if _, ok := m[f]; !ok {
			t.Errorf("Coverage JSON field %q missing", f)
		}
	}
}

func TestCoverageGap_JSONFieldNames(t *testing.T) {
	g := diagnostic.CoverageGap{
		Tool:            "go/packages",
		InstallCmd:      "https://go.dev/dl",
		AffectedMetrics: []string{"coverage", "coupling_balance"},
		Gate:            gateWarn,
	}

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, f := range []string{"tool", "install_cmd", "affected_metrics", "gate"} {
		if _, ok := m[f]; !ok {
			t.Errorf("CoverageGap JSON field %q missing", f)
		}
	}
}

func TestDiagnostic_CoverageGapsRoundTrip(t *testing.T) {
	d := diagnostic.New()
	d.CoverageGaps = []diagnostic.CoverageGap{
		{Tool: "lizard", InstallCmd: "pip install lizard", AffectedMetrics: []string{"complexity"}, Gate: gateWarn},
	}
	d.ConfigWarnings = []string{`module "internal/a" omits owner`}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(got.CoverageGaps) != 1 || got.CoverageGaps[0].Tool != "lizard" {
		t.Errorf("CoverageGaps round-trip = %+v", got.CoverageGaps)
	}
	if got.CoverageGaps[0].AffectedMetrics[0] != "complexity" {
		t.Errorf("AffectedMetrics round-trip = %v", got.CoverageGaps[0].AffectedMetrics)
	}
	if len(got.ConfigWarnings) != 1 || got.ConfigWarnings[0] != `module "internal/a" omits owner` {
		t.Errorf("ConfigWarnings round-trip = %v", got.ConfigWarnings)
	}
}

func TestDiagnostic_CoverageGapsOmitEmpty(t *testing.T) {
	d := diagnostic.New() // no gaps, no warnings

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := m["coverage_gaps"]; ok {
		t.Error("coverage_gaps must be omitted when empty")
	}
	if _, ok := m["config_warnings"]; ok {
		t.Error("config_warnings must be omitted when empty")
	}
}

func TestDiagnostic_SchemaVersionInJSON(t *testing.T) {
	d := diagnostic.New()
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	var sv string
	if err := json.Unmarshal(m["schema_version"], &sv); err != nil {
		t.Fatalf("unmarshal schema_version: %v", err)
	}
	if sv != "archfit.diagnostic.v1" {
		t.Errorf("schema_version = %q; want \"archfit.diagnostic.v1\"", sv)
	}
}
