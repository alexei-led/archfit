package sarif_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/output/sarif"
	reporttest "github.com/alexei-led/archfit/internal/testutil/report"
)

const (
	ruleInternal = "no_internal_access"
	fileA        = "pkg/a/a.go"
)

func sampleDiagnostic() diagnostic.Diagnostic {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictFail
	d.Base = "main"
	d.Head = "HEAD"
	d.Metrics = []diagnostic.MetricResult{{Name: "cycle", Value: 0, Band: "green"}}
	d.Findings = reporttest.Findings(
		finding.Finding{
			ID: "f-gate", Kind: "gate", RuleID: ruleInternal,
			Status: finding.StatusNew, Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: fileA},
				To:   finding.Endpoint{Path: "pkg/b/internal/impl.go"},
			},
			Locations: []graph.Location{{File: fileA, Line: 5}},
			Why:       "a uses b internals",
		},
		finding.Finding{
			ID: "f-adv", Kind: "advisory", RuleID: "bc/imbalanced_coupling",
			Status: finding.StatusNew, Severity: finding.SeverityMedium,
			Edge: finding.EdgeEvidence{From: finding.Endpoint{Path: "pkg/c/c.go"}},
		},
		finding.Finding{
			ID: "f-base", Kind: "gate", RuleID: ruleInternal,
			Status: finding.StatusBaseline,
			Edge:   finding.EdgeEvidence{From: finding.Endpoint{Path: "pkg/d/d.go"}},
		},
	)
	return d
}

func render(t *testing.T, d diagnostic.Diagnostic) (string, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if err := sarif.New().Render(d, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return buf.String(), doc
}

func TestRenderer_Format(t *testing.T) {
	if got := sarif.New().Format(); got != "sarif" {
		t.Errorf("Format() = %q, want sarif", got)
	}
}

func TestRender_ShapeAndLevels(t *testing.T) {
	out, doc := render(t, sampleDiagnostic())

	if doc["version"] != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", doc["version"])
	}
	if !strings.Contains(out, "sarif-2.1.0.json") {
		t.Error("missing $schema reference")
	}

	runs := doc["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	run := runs[0].(map[string]any)

	// Rules: two distinct IDs, sorted.
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2 distinct", len(rules))
	}
	if rules[0].(map[string]any)["id"] != "bc/imbalanced_coupling" {
		t.Errorf("rules not sorted: first = %v", rules[0])
	}

	// Results: 3 findings → error, warning, note.
	results := run["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	levels := map[string]string{}
	for _, raw := range results {
		res := raw.(map[string]any)
		levels[res["fingerprints"].(map[string]any)["archfit/v1"].(string)] = res["level"].(string)
	}
	want := map[string]string{"f-gate": "error", "f-adv": "warning", "f-base": "note"}
	for fp, lvl := range want {
		if levels[fp] != lvl {
			t.Errorf("level[%s] = %q, want %q", fp, levels[fp], lvl)
		}
	}

	// Location with line for the gate finding.
	first := results[0].(map[string]any)
	loc := first["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	if loc["artifactLocation"].(map[string]any)["uri"] != fileA {
		t.Errorf("location uri = %v", loc)
	}
	if loc["region"].(map[string]any)["startLine"] != float64(5) {
		t.Errorf("startLine = %v, want 5", loc["region"])
	}

	// Metrics + verdict ride in run properties.
	props := run["properties"].(map[string]any)
	if props["verdict"] != "fail" {
		t.Errorf("properties.verdict = %v", props["verdict"])
	}
	if len(props["metrics"].([]any)) != 1 {
		t.Errorf("properties.metrics = %v", props["metrics"])
	}

	// No timestamps anywhere (determinism).
	if strings.Contains(out, "startTimeUtc") || strings.Contains(out, "endTimeUtc") {
		t.Error("SARIF output must not contain timestamps")
	}
}

func TestRender_Deterministic(t *testing.T) {
	d := sampleDiagnostic()
	var a, b bytes.Buffer
	if err := sarif.New().Render(d, &a); err != nil {
		t.Fatal(err)
	}
	if err := sarif.New().Render(d, &b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("double render differs — must be byte-identical")
	}
}

func TestRender_EmptyDiagnostic(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	_, doc := render(t, d)
	run := doc["runs"].([]any)[0].(map[string]any)
	if results := run["results"].([]any); len(results) != 0 {
		t.Errorf("results = %d, want 0", len(results))
	}
}
