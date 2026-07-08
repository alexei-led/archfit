package diagnostic_test

import (
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// gateWarn is the default coverage-gap gate value reused across these tests.
const gateWarn = "warn"

// constants for repeated string literals flagged by goconst.
const (
	kindFunction       = "function"
	fileAGo            = "a.go"
	languageTypeScript = "typescript"
)

func TestNew_ZeroValue(t *testing.T) {
	d := diagnostic.New()
	if d.SchemaVersion != diagnostic.SchemaVersion {
		t.Errorf("schema_version = %q; want %q", d.SchemaVersion, diagnostic.SchemaVersion)
	}
	if d.SchemaVersion != "archfit.diagnostic.v2" {
		t.Errorf("schema_version = %q; want \"archfit.diagnostic.v2\"", d.SchemaVersion)
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
	if d.AdvisoryTasks == nil {
		t.Error("AdvisoryTasks must be non-nil (empty slice, not nil)")
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
		"advisory_tasks",
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

	for _, field := range []string{"agent_tasks", "advisory_tasks"} {
		raw, ok := m[field]
		if !ok {
			t.Fatalf("%s field missing from JSON", field)
		}
		if string(raw) != "[]" {
			t.Errorf("%s = %s; want []", field, raw)
		}
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
		GateFindings: 3,
		Warnings:     1,
		WaiversUsed:  2,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, f := range []string{"gate_findings", "warnings", "waivers_used"} {
		if _, ok := m[f]; !ok {
			t.Errorf("Summary JSON field %q missing", f)
		}
	}
}

func TestSemanticStrengthOverlay_JSONFieldNames(t *testing.T) {
	overlay := diagnostic.SemanticStrengthOverlay{
		ByLanguage: map[string]diagnostic.SemanticStrengthOverlayStats{
			languageTypeScript: {
				CandidateEdges: 2,
				Applied:        1,
				Missed:         1,
				Before:         map[string]int{"unknown": 2},
				After:          map[string]int{"model": 1, "unknown": 1},
			},
		},
	}
	data, err := json.Marshal(overlay)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	stats := raw["by_language"][languageTypeScript]
	for _, field := range []string{"candidate_edges", "applied", "missed", "before", "after"} {
		if _, ok := stats[field]; !ok {
			t.Errorf("semantic overlay field %q missing", field)
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
		{Tool: "jscpd", InstallCmd: "npm install -g jscpd", AffectedMetrics: []string{"blast_radius"}, Gate: gateWarn},
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

	if len(got.CoverageGaps) != 1 || got.CoverageGaps[0].Tool != "jscpd" {
		t.Errorf("CoverageGaps round-trip = %+v", got.CoverageGaps)
	}
	if got.CoverageGaps[0].AffectedMetrics[0] != "blast_radius" {
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

// TestCoverageStatusConstants ensures all status constants have distinct,
// stable values. A change here would break the disabled-vs-absent contract.
func TestCoverageStatusConstants(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want string
	}{
		{"StatusOK", diagnostic.StatusOK, "ok"},
		{"StatusPartial", diagnostic.StatusPartial, "partial"},
		{"StatusAbsent", diagnostic.StatusAbsent, "absent"},
		{"StatusDisabled", diagnostic.StatusDisabled, "disabled"},
	}
	for _, tc := range cases {
		if tc.val != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.val, tc.want)
		}
	}
	// All constants must be distinct.
	seen := map[string]string{}
	for _, tc := range cases {
		if prev, dup := seen[tc.val]; dup {
			t.Errorf("duplicate status value %q shared by %s and %s", tc.val, prev, tc.name)
		}
		seen[tc.val] = tc.name
	}
}

func TestSyntaxFact_JSONFieldNames(t *testing.T) {
	sf := diagnostic.SyntaxFact{
		Language:  "go",
		File:      "internal/foo/foo.go",
		Kind:      kindFunction,
		Name:      "HandleRequest",
		Exported:  true,
		StartLine: 10,
		EndLine:   25,
		Framework: "net/http",
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	required := []string{
		"language", "file", "kind", "name",
		"exported", "start_line", "end_line", "framework",
	}
	for _, f := range required {
		if _, ok := m[f]; !ok {
			t.Errorf("SyntaxFact JSON field %q missing", f)
		}
	}
}

func TestSyntaxFact_OmitEmptyFields(t *testing.T) {
	// Only language, file, kind, name, start_line must appear when optional
	// fields are zero/empty.
	sf := diagnostic.SyntaxFact{
		Language:  "go",
		File:      "internal/foo/foo.go",
		Kind:      kindFunction,
		Name:      "internalHelper",
		StartLine: 5,
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// start_line must always be present (no omitempty).
	if _, ok := m["start_line"]; !ok {
		t.Error("start_line must always be present")
	}

	// Optional fields must be absent when zero/empty.
	for _, f := range []string{"exported", "end_line", "role", "role_confidence", "role_evidence", "framework"} {
		if _, ok := m[f]; ok {
			t.Errorf("field %q must be omitted when zero", f)
		}
	}
}

func TestDiagnostic_SyntaxFactsOmitWhenEmpty(t *testing.T) {
	d := diagnostic.New()

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := m["syntax_facts"]; ok {
		t.Error("syntax_facts must be omitted when nil (sg absent / analyzers.syntax off)")
	}
}

func TestDiagnostic_SyntaxFactsRoundTrip(t *testing.T) {
	d := diagnostic.New()
	d.SyntaxFacts = []diagnostic.SyntaxFact{
		{Language: "go", File: "a/b.go", Kind: kindFunction, Name: "Run", Exported: true, StartLine: 1},
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(got.SyntaxFacts) != 1 {
		t.Fatalf("SyntaxFacts len = %d; want 1", len(got.SyntaxFacts))
	}
	sf := got.SyntaxFacts[0]
	if sf.Language != "go" || sf.File != "a/b.go" || sf.Kind != kindFunction || sf.Name != "Run" || !sf.Exported || sf.StartLine != 1 {
		t.Errorf("SyntaxFact round-trip = %+v", sf)
	}
}

func TestSortSyntaxFacts(t *testing.T) {
	facts := []diagnostic.SyntaxFact{
		{File: "b.go", StartLine: 5, Kind: kindFunction, Name: "B"},
		{File: fileAGo, StartLine: 10, Kind: "method", Name: "Z"},
		{File: fileAGo, StartLine: 3, Kind: kindFunction, Name: "A"},
		{File: fileAGo, StartLine: 3, Kind: kindFunction, Name: "B"}, // same file/line/kind — name tie-break
		{File: fileAGo, StartLine: 3, Kind: "class", Name: "A"},      // same file/line — kind tie-break
	}

	diagnostic.SortSyntaxFacts(facts)

	// Expected order: a.go:3:class:A, a.go:3:function:A, a.go:3:function:B, a.go:10:method:Z, b.go:5:function:B
	type key struct {
		file, kind, name string
		line             int
	}
	expected := []key{
		{fileAGo, "class", "A", 3},
		{fileAGo, kindFunction, "A", 3},
		{fileAGo, kindFunction, "B", 3},
		{fileAGo, "method", "Z", 10},
		{"b.go", kindFunction, "B", 5},
	}

	if len(facts) != len(expected) {
		t.Fatalf("len = %d; want %d", len(facts), len(expected))
	}
	for i, want := range expected {
		got := facts[i]
		if got.File != want.file || got.StartLine != want.line || got.Kind != want.kind || got.Name != want.name {
			t.Errorf("[%d] got {%s %d %s %s}; want {%s %d %s %s}",
				i, got.File, got.StartLine, got.Kind, got.Name,
				want.file, want.line, want.kind, want.name)
		}
	}
}

func TestSortSyntaxFacts_StableOnTies(t *testing.T) {
	// Two facts identical on all four keys — stable sort must preserve input order.
	f1 := diagnostic.SyntaxFact{File: "x.go", StartLine: 1, Kind: kindFunction, Name: "F", Language: "go"}
	f2 := diagnostic.SyntaxFact{File: "x.go", StartLine: 1, Kind: kindFunction, Name: "F", Language: "typescript"} // different language, same keys

	facts := []diagnostic.SyntaxFact{f1, f2}
	diagnostic.SortSyntaxFacts(facts)

	if facts[0].Language != "go" || facts[1].Language != "typescript" {
		t.Errorf("stable sort violated: got [%s, %s]; want [go, typescript]",
			facts[0].Language, facts[1].Language)
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
	if sv != "archfit.diagnostic.v2" {
		t.Errorf("schema_version = %q; want \"archfit.diagnostic.v2\"", sv)
	}
}

// TestSyntaxFact_ModuleIncludedInJSON verifies that Module appears in JSON when set
// and is omitted (omitempty) when empty, matching the field contract.
func TestSyntaxFact_ModuleIncludedInJSON(t *testing.T) {
	// With module set: must appear as "module" key.
	sf := diagnostic.SyntaxFact{
		Language:  "go",
		File:      "pkg/svc/svc.go",
		Module:    "svc",
		Kind:      kindFunction,
		Name:      "Handle",
		StartLine: 1,
	}
	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	raw, ok := m["module"]
	if !ok {
		t.Fatal("module field must appear when Module is set")
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal module: %v", err)
	}
	if got != "svc" {
		t.Errorf("module = %q; want %q", got, "svc")
	}

	// With module empty: must be omitted.
	sfNoModule := diagnostic.SyntaxFact{
		Language:  "go",
		File:      "standalone.go",
		Kind:      kindFunction,
		Name:      "Main",
		StartLine: 1,
	}
	data2, err := json.Marshal(sfNoModule)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m2 map[string]json.RawMessage
	if err := json.Unmarshal(data2, &m2); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := m2["module"]; ok {
		t.Error("module field must be omitted when Module is empty")
	}
}
