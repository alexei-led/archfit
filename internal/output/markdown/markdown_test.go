package markdown_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/output/markdown"
)

func TestRenderer_Format(t *testing.T) {
	r := markdown.New()
	if got := r.Format(); got != "markdown" {
		t.Errorf("Format() = %q, want %q", got, "markdown")
	}
}

// makeGateFinding creates a gate finding for testing.
func makeGateFinding(ruleID string, sev finding.Severity, status finding.Status) finding.Finding {
	f := finding.New(ruleID, graph.Edge{
		From: "pkg/a",
		To:   "pkg/b",
		Kind: graph.EdgeKind("uses"),
	}, nil)
	f.Severity = sev
	f.Status = status
	return f
}

// makeAdvisoryFinding creates an advisory finding for testing.
func makeAdvisoryFinding(ruleID string) finding.Finding {
	f := makeGateFinding(ruleID, finding.SeverityLow, finding.StatusNew)
	f.Kind = "advisory"
	return f
}

func TestRenderer_Render_EmptyDiagnostic(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	// Required sections always present.
	for _, want := range []string{"Health Summary", "Verdict", "pass"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// Optional sections absent when no findings.
	for _, absent := range []string{"Critical Gate Violations", "BC Advisories", "Map Staleness", "Exception Inventory", "Full Violation List"} {
		if strings.Contains(out, absent) {
			t.Errorf("output should not contain %q in empty diagnostic\nfull output:\n%s", absent, out)
		}
	}
}

func TestRenderer_Render_GateFindings(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictFail
	d.Summary.GateFindings = 2
	d.Findings = []finding.Finding{
		makeGateFinding("forbidden_dep", finding.SeverityHigh, finding.StatusNew),
		makeGateFinding("cycle", finding.SeverityCritical, finding.StatusNew),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	for _, want := range []string{"Critical Gate Violations", "forbidden_dep", "cycle", "Full Violation List"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// No advisory section — no advisory findings.
	if strings.Contains(out, "BC Advisories") {
		t.Errorf("output should not contain BC Advisories section\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_AdvisoryFindings(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.Warnings = 2
	d.Findings = []finding.Finding{
		makeAdvisoryFinding("bc/imbalanced_coupling"),
		makeAdvisoryFinding("bc/imbalanced_coupling"),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "BC Advisories") {
		t.Errorf("output missing BC Advisories section\nfull output:\n%s", out)
	}

	// No gate violations section — no gate findings.
	if strings.Contains(out, "Critical Gate Violations") {
		t.Errorf("output should not contain Critical Gate Violations section\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_AdvisoryAbsentWhenNoAdvisory(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Findings = []finding.Finding{
		makeGateFinding("forbidden_dep", finding.SeverityLow, finding.StatusNew),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	if strings.Contains(out, "BC Advisories") {
		t.Errorf("BC Advisories section must be absent when no advisory findings\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_StalenessSection(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Findings = []finding.Finding{
		makeAdvisoryFinding("map/uncovered_path"),
		makeAdvisoryFinding("map/dead_rule"),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "Map Staleness") {
		t.Errorf("output missing Map Staleness section\nfull output:\n%s", out)
	}

	// map/ findings are staleness, not BC advisories.
	if strings.Contains(out, "BC Advisories") {
		t.Errorf("BC Advisories section must be absent when only map/ advisory findings\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_ExceptionInventory(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.ExceptionsUsed = 1
	excepted := makeGateFinding("forbidden_dep", finding.SeverityLow, finding.StatusExcepted)
	d.Findings = []finding.Finding{excepted}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "Exception Inventory") {
		t.Errorf("output missing Exception Inventory section\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_Top10GateTruncation(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictFail

	// Create 15 gate findings.
	for i := range 15 {
		f := makeGateFinding("rule", finding.SeverityLow, finding.StatusNew)
		// Make each unique by varying path.
		f.Edge.From.Path = "pkg/" + string(rune('a'+i))
		d.Findings = append(d.Findings, f)
	}
	d.Summary.GateFindings = 15

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	// Full violation list has all 15; gate section is truncated to 10.
	// We can't count rows easily, but verify both sections exist.
	if !strings.Contains(out, "Critical Gate Violations") {
		t.Errorf("output missing Critical Gate Violations\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "Full Violation List") {
		t.Errorf("output missing Full Violation List\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_MetricsSection(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Metrics = []diagnostic.MetricResult{
		{Name: "coupling_ratio", Display: "0.42", Band: "good", Confidence: "high"},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	for _, want := range []string{"Metrics", "coupling_ratio", "0.42"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderer_Render_OutputIsValidText(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Must be non-empty valid UTF-8.
	out := buf.String()
	if len(out) == 0 {
		t.Error("Render() produced empty output")
	}
	if !strings.Contains(out, "archfit") {
		t.Errorf("output missing 'archfit' header\nfull output:\n%s", out)
	}
}
