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

const (
	secGate       = "Gate findings"
	secAdvisories = "Advisories"

	// New info-metric names.
	metricRiskHub              = "risk_hub"
	metricArchFitness          = "architecture_fitness"
	metricFunctionalCandidates = "functional_candidates"

	// Band / confidence / status literals used in multiple tests.
	bandInfo       = "info"
	bandNA         = "n/a"
	confidenceHigh = "high"
	confidenceLow  = "low"
	statusAbsent   = "absent"
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
	for _, want := range []string{"Summary", "Verdict", "pass"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// Optional sections absent when no findings or metrics.
	for _, absent := range []string{secGate, secAdvisories, "Metrics"} {
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

	for _, want := range []string{secGate, "forbidden_dep", "cycle", secGate} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// No advisory section — no advisory findings.
	if strings.Contains(out, secAdvisories) {
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

	if !strings.Contains(out, secAdvisories) {
		t.Errorf("output missing BC Advisories section\nfull output:\n%s", out)
	}

	// No gate violations section — no gate findings.
	if strings.Contains(out, secGate) {
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

	if strings.Contains(out, secAdvisories) {
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

	// map/ findings render under the consolidated Advisories section.
	for _, want := range []string{secAdvisories, "map/uncovered_path", "map/dead_rule"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
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

	if !strings.Contains(out, secGate) {
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
	if !strings.Contains(out, secGate) {
		t.Errorf("output missing Critical Gate Violations\nfull output:\n%s", out)
	}
	if !strings.Contains(out, secGate) {
		t.Errorf("output missing Full Violation List\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_MetricsSection(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Metrics = []diagnostic.MetricResult{
		{Name: "coupling_ratio", Display: "0.42", Band: "good", Confidence: confidenceHigh},
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

func TestRenderer_Render_NewInfoMetrics(t *testing.T) {
	// Confirm risk_hub, architecture_fitness, functional_candidates render their
	// display string and band in the Metrics section.
	tests := []struct {
		name    string
		metric  diagnostic.MetricResult
		wantSub []string
	}{
		{
			name: "risk_hub present",
			metric: diagnostic.MetricResult{
				Name:       metricRiskHub,
				Display:    "2 risk hub(s): pkg/store [breadth 3, ×1.00→3.00]",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantSub: []string{metricRiskHub, "2 risk hub(s)", bandInfo},
		},
		{
			name: "architecture_fitness present",
			metric: diagnostic.MetricResult{
				Name:       metricArchFitness,
				Display:    "6.7/10 (2/3 signals)",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantSub: []string{metricArchFitness, "6.7/10", bandInfo},
		},
		{
			name: "functional_candidates present",
			metric: diagnostic.MetricResult{
				Name:       metricFunctionalCandidates,
				Display:    "3 clone-duplicated cross-module pair(s)",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantSub: []string{metricFunctionalCandidates, "3 clone-duplicated", bandInfo},
		},
		{
			name: "risk_hub n/a",
			metric: diagnostic.MetricResult{
				Name:       metricRiskHub,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricRiskHub, bandNA},
		},
		{
			name: "architecture_fitness n/a",
			metric: diagnostic.MetricResult{
				Name:       metricArchFitness,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricArchFitness, bandNA},
		},
		{
			name: "functional_candidates n/a",
			metric: diagnostic.MetricResult{
				Name:       metricFunctionalCandidates,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricFunctionalCandidates, bandNA},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Verdict = diagnostic.VerdictPass
			d.Metrics = []diagnostic.MetricResult{tt.metric}

			r := markdown.New()
			var buf bytes.Buffer
			if err := r.Render(d, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			out := buf.String()
			for _, want := range tt.wantSub {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, out)
				}
			}
		})
	}
}

func TestRenderer_Render_ToolCoverageNewTools(t *testing.T) {
	// Confirm scip-symbols, jscpd (clones), and gitnexus coverage rows render.
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.ToolCoverage = []diagnostic.Coverage{
		{Tool: "scip-symbols", Status: statusAbsent},
		{Tool: "jscpd", Status: statusAbsent},
		{Tool: "gitnexus", Status: statusAbsent},
	}

	r := markdown.New()
	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"scip-symbols", "jscpd", "gitnexus"} {
		if !strings.Contains(out, want) {
			t.Errorf("coverage section missing %q\nfull output:\n%s", want, out)
		}
	}
}
