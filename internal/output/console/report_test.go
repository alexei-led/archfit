package console

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

const (
	dimCouplingBalance = "coupling_balance"
	recCouplingSeam    = "coupling_seam"
	recLazyCycle       = "lazy_cycle"
)

// acceptableReport is a representative report: passing gate, advisory warnings,
// a mixed overall score, one low dimension, and recommendations.
func acceptableReport() report.Report {
	return report.Report{
		Band:        report.DecisionBandAcceptable,
		Headline:    "Acceptable with watch items. Monitor flagged areas.",
		Blocking:    0,
		Advisory:    55,
		Overall:     43,
		OverallBand: report.ScoreBandMixed,
		Dimensions: []report.DimReport{
			{Name: dimCouplingBalance, Value: 40, Band: report.ScoreBandPoor, Confidence: report.ConfidenceMedium,
				Why: "304 warning edges, mostly functional + high volatility.", WhatMoves: "Reduce high-fan-in functional edges."},
		},
		Recommendations: report.Recommendations{
			MustFix:   []report.Rec{},
			ShouldFix: []report.Rec{{Title: recCouplingSeam, RuleID: recCouplingSeam, Detail: "high fan-in into session state"}},
			Watch:     []report.Rec{{Title: recLazyCycle, RuleID: recLazyCycle, Detail: "lazy import SCC"}},
			Calibrate: []report.Rec{},
			Ignore:    []report.Rec{},
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
		"WHAT WOULD IMPROVE THE SCORE",
		"Reduce high-fan-in functional edges.",
		"TARGETS",
		"Near-term", // mixed → serviceable target exists
	}
	for _, w := range wantContains {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}

	// coupling_balance is the sole low dimension and must appear in why-low.
	if !strings.Contains(out, "coupling_balance  40/100") {
		t.Errorf("coupling_balance missing from why-low section:\n%s", out)
	}
	// CALIBRATE/IGNORE are empty here and must not render.
	if strings.Contains(out, "CALIBRATE") || strings.Contains(out, "IGNORE") {
		t.Errorf("empty LLM categories should not render:\n%s", out)
	}
}

func TestRenderReport_Fail(t *testing.T) {
	r := report.Report{
		Band:        report.DecisionBandFail,
		Headline:    "Gate violations. Fix before merge.",
		Blocking:    2,
		Advisory:    3,
		Overall:     30,
		OverallBand: report.ScoreBandPoor,
		Dimensions:  nil,
		Recommendations: report.Recommendations{
			MustFix:   []report.Rec{{Title: "forbidden_dependency", RuleID: "forbidden_dependency", Detail: "a -> b"}},
			ShouldFix: []report.Rec{},
			Watch:     []report.Rec{},
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
	r := report.Report{
		Band:        report.DecisionBandHealthy,
		Headline:    "Architecture is healthy. No action required.",
		Blocking:    0,
		Advisory:    0,
		Overall:     85,
		OverallBand: report.ScoreBandStrong,
		Dimensions: []report.DimReport{
			{Name: dimCouplingBalance, Value: 88, Band: report.ScoreBandStrong, Confidence: report.ConfidenceHigh},
		},
		Recommendations: report.Recommendations{MustFix: []report.Rec{}, ShouldFix: []report.Rec{}, Watch: []report.Rec{}},
		Delta: &report.Delta{
			Overall: 5,
			Dimensions: []report.DimDelta{
				{Name: dimCouplingBalance, Before: 83, After: 88, Change: 5},
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
	// Only coupling_balance changed; its delta must be present.
	if !strings.Contains(out, "coupling_balance") {
		t.Errorf("coupling_balance delta missing from output:\n%s", out)
	}
	// Healthy + all dims serviceable+ → no why-low section, no near-term target.
	if strings.Contains(out, "WHY THE SCORE IS LOW") {
		t.Errorf("healthy run should have no why-low section:\n%s", out)
	}
	if strings.Contains(out, "Near-term") {
		t.Errorf("strong band should have no near-term target:\n%s", out)
	}
}
