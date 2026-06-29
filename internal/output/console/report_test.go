package console

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/score"
)

const (
	dimCouplingBalance = "coupling_balance"
	recCouplingSeam    = "coupling_seam"
	recLazyCycle       = "lazy_cycle"
)

// acceptableReport is a representative report: passing gate, advisory warnings,
// a mixed overall score, two low dimensions, and recommendations.
func acceptableReport() decision.Report {
	return decision.Report{
		Band:        decision.BandAcceptable,
		Headline:    "Acceptable with watch items. Monitor flagged areas.",
		Blocking:    0,
		Advisory:    55,
		Overall:     43,
		OverallBand: score.BandMixed,
		Dimensions: []decision.DimReport{
			{Name: dimCouplingBalance, Value: 40, Band: score.BandPoor, Confidence: score.ConfidenceMedium,
				Why: "304 warning edges, mostly functional + high volatility.", WhatMoves: "Reduce high-fan-in functional edges."},
			{Name: "dependency_graph_health", Value: 24, Band: score.BandPoor, Confidence: score.ConfidenceHigh,
				Why: "Static cycles exist.", WhatMoves: "Break true static cycles."},
			{Name: "boundary_integrity", Value: 90, Band: score.BandStrong, Confidence: score.ConfidenceHigh,
				Why: "No forbidden deps.", WhatMoves: "n/a"},
			{Name: "analysis_confidence", Value: 70, Band: score.BandServiceable, Confidence: score.ConfidenceHigh, Meta: true},
		},
		Recommendations: decision.Recommendations{
			MustFix:   []decision.Rec{},
			ShouldFix: []decision.Rec{{Title: recCouplingSeam, RuleID: recCouplingSeam, Detail: "high fan-in into session state"}},
			Watch:     []decision.Rec{{Title: recLazyCycle, RuleID: recLazyCycle, Detail: "lazy import SCC"}},
			Calibrate: []decision.Rec{},
			Ignore:    []decision.Rec{},
		},
	}
}

func TestRenderReport_Acceptable(t *testing.T) {
	var b strings.Builder
	if err := RenderReport(acceptableReport(), &b); err != nil {
		t.Fatalf("RenderReport: %v", err)
	}
	out := b.String()

	wantContains := []string{
		"ARCHFIT RESULT",
		"Decision   ACCEPTABLE WITH WATCH ITEMS",
		"PASS  ·  0 blocking",
		"55 advisory",
		"43 / 100  mixed",
		"No blockers.",
		"RECOMMENDATIONS",
		"MUST FIX",
		"none",
		"SHOULD FIX",
		"coupling_seam",
		"WATCH",
		"lazy_cycle",
		"WHY THE SCORE IS LOW",
		"coupling_balance  40/100",
		"dependency_graph_health  24/100",
		"WHAT WOULD IMPROVE THE SCORE",
		"Break true static cycles.",
		"TARGETS",
		"Near-term", // mixed → serviceable target exists
	}
	for _, w := range wantContains {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}

	// Why-low must be ordered worst-first: dependency_graph_health (24) before coupling_balance (40).
	if strings.Index(out, "dependency_graph_health  24/100") > strings.Index(out, "coupling_balance  40/100") {
		t.Errorf("low dimensions not sorted worst-first:\n%s", out)
	}
	// Meta dimension and serviceable+ dims must NOT appear in the why-low section.
	if strings.Contains(out, "analysis_confidence  70") || strings.Contains(out, "boundary_integrity  90") {
		t.Errorf("non-low / meta dimension leaked into why-low:\n%s", out)
	}
	// CALIBRATE/IGNORE are empty here and must not render.
	if strings.Contains(out, "CALIBRATE") || strings.Contains(out, "IGNORE") {
		t.Errorf("empty LLM categories should not render:\n%s", out)
	}
}

func TestRenderReport_Fail(t *testing.T) {
	r := decision.Report{
		Band:        decision.BandFail,
		Headline:    "Gate violations. Fix before merge.",
		Blocking:    2,
		Advisory:    3,
		Overall:     30,
		OverallBand: score.BandPoor,
		Dimensions:  nil,
		Recommendations: decision.Recommendations{
			MustFix:   []decision.Rec{{Title: "forbidden_dependency", RuleID: "forbidden_dependency", Detail: "a -> b"}},
			ShouldFix: []decision.Rec{},
			Watch:     []decision.Rec{},
		},
	}
	var b strings.Builder
	if err := RenderReport(r, &b); err != nil {
		t.Fatalf("RenderReport: %v", err)
	}
	out := b.String()

	if !strings.Contains(out, "Decision   FAIL") {
		t.Errorf("missing FAIL decision:\n%s", out)
	}
	if !strings.Contains(out, "FAIL  ·  2 blocking") {
		t.Errorf("missing gate line:\n%s", out)
	}
	if strings.Contains(out, "No blockers.") {
		t.Errorf("FAIL run must not show the no-blockers line:\n%s", out)
	}
	if !strings.Contains(out, "forbidden_dependency") {
		t.Errorf("MUST FIX should list the gate rule:\n%s", out)
	}
}

func TestRenderReport_DeltaAndHealthy(t *testing.T) {
	r := decision.Report{
		Band:        decision.BandHealthy,
		Headline:    "Architecture is healthy. No action required.",
		Blocking:    0,
		Advisory:    0,
		Overall:     85,
		OverallBand: score.BandStrong,
		Dimensions: []decision.DimReport{
			{Name: dimCouplingBalance, Value: 88, Band: score.BandStrong, Confidence: score.ConfidenceHigh},
		},
		Recommendations: decision.Recommendations{MustFix: []decision.Rec{}, ShouldFix: []decision.Rec{}, Watch: []decision.Rec{}},
		Delta: &decision.Delta{
			Overall: 5,
			Dimensions: []decision.DimDelta{
				{Name: dimCouplingBalance, Before: 83, After: 88, Change: 5},
				{Name: "cohesion_modularity", Before: 50, After: 50, Change: 0},
			},
		},
	}
	var b strings.Builder
	if err := RenderReport(r, &b); err != nil {
		t.Fatalf("RenderReport: %v", err)
	}
	out := b.String()

	if !strings.Contains(out, "CHANGE VS BASE") {
		t.Errorf("missing delta section:\n%s", out)
	}
	if !strings.Contains(out, "overall  +5") {
		t.Errorf("missing signed overall delta:\n%s", out)
	}
	if !strings.Contains(out, "coupling_balance  83 → 88  (+5)") {
		t.Errorf("missing dimension delta:\n%s", out)
	}
	// Zero-change dimension is omitted from the delta block.
	if strings.Contains(out, "cohesion_modularity") {
		t.Errorf("zero-change dimension should be omitted:\n%s", out)
	}
	// Healthy + all dims serviceable+ → no why-low section, no near-term target.
	if strings.Contains(out, "WHY THE SCORE IS LOW") {
		t.Errorf("healthy run should have no why-low section:\n%s", out)
	}
	if strings.Contains(out, "Near-term") {
		t.Errorf("strong band should have no near-term target:\n%s", out)
	}
}
