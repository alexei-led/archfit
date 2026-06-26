package console_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/output/console"
)

// String constants used across tests — goconst requires a single definition for
// repeated literals.
const (
	metricRiskHub              = "risk_hub"
	metricArchFitness          = "architecture_fitness"
	metricFunctionalCandidates = "functional_candidates"
	bandInfo                   = "info"
	bandNA                     = "n/a"
	confidenceHigh             = "high"
	confidenceLow              = "low"
	lowConfSuffix              = "low confidence"
	statusAbsent               = "absent"
)

func TestRenderer_Format(t *testing.T) {
	r := console.New()
	if got := r.Format(); got != "text" {
		t.Errorf("Format() = %q, want %q", got, "text")
	}
}

func TestRenderer_Render(t *testing.T) {
	tests := []struct {
		name         string
		verdict      diagnostic.Verdict
		gateFindings int
		wantVerdict  string
		wantCount    string
		wantExitHint string
	}{
		{
			name:         "pass",
			verdict:      diagnostic.VerdictPass,
			gateFindings: 0,
			wantVerdict:  "verdict: PASS",
			wantCount:    "gate findings: 0",
			wantExitHint: "exit 0",
		},
		{
			name:         "fail with findings",
			verdict:      diagnostic.VerdictFail,
			gateFindings: 3,
			wantVerdict:  "verdict: FAIL",
			wantCount:    "gate findings: 3",
			wantExitHint: "exit 1",
		},
		{
			name:         "warn",
			verdict:      diagnostic.VerdictWarn,
			gateFindings: 1,
			wantVerdict:  "verdict: WARN",
			wantCount:    "gate findings: 1",
			wantExitHint: "exit 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Verdict = tt.verdict
			d.Summary.GateFindings = tt.gateFindings

			r := console.New()
			var buf bytes.Buffer

			if err := r.Render(d, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			out := buf.String()

			for _, want := range []string{tt.wantVerdict, tt.wantExitHint, tt.wantCount} {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, out)
				}
			}
		})
	}
}

func TestRenderer_Render_NewInfoMetrics(t *testing.T) {
	// Confirm risk_hub, architecture_fitness, functional_candidates render their
	// display string and band in the metrics section.
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
			wantSub: []string{metricRiskHub, bandNA, lowConfSuffix},
		},
		{
			name: "architecture_fitness n/a",
			metric: diagnostic.MetricResult{
				Name:       metricArchFitness,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricArchFitness, bandNA, lowConfSuffix},
		},
		{
			name: "functional_candidates n/a",
			metric: diagnostic.MetricResult{
				Name:       metricFunctionalCandidates,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricFunctionalCandidates, bandNA, lowConfSuffix},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Verdict = diagnostic.VerdictPass
			d.Metrics = []diagnostic.MetricResult{tt.metric}

			r := console.New()
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
	// Confirm scip-symbols and jscpd (clones) coverage rows render.
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.ToolCoverage = []diagnostic.Coverage{
		{Tool: "scip-symbols", Status: statusAbsent},
		{Tool: "jscpd", Status: statusAbsent},
	}

	r := console.New()
	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"scip-symbols", "jscpd"} {
		if !strings.Contains(out, want) {
			t.Errorf("coverage section missing %q\nfull output:\n%s", want, out)
		}
	}
}
