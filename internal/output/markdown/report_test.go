package markdown

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

func TestRenderReport_DecisionSummary(t *testing.T) {
	r := report.Report{
		Band:        report.DecisionBandAcceptable,
		Headline:    "Acceptable with watch items. Monitor flagged areas.",
		Blocking:    0,
		Advisory:    12,
		Overall:     58,
		OverallBand: report.ScoreBandMixed,
		Dimensions: []report.DimReport{
			{Name: "coupling_balance", Value: 36, Band: report.ScoreBandPoor, Why: "375 unbalanced edges", WhatMoves: "Add stable contracts."},
			{Name: "encapsulation", Value: 90, Band: report.ScoreBandStrong},
		},
		Recommendations: report.Recommendations{
			MustFix:   []report.Rec{},
			ShouldFix: []report.Rec{{RuleID: "coupling_seam", Detail: "high fan-in"}},
			Watch:     []report.Rec{},
		},
		Delta: &report.Delta{
			Overall:    -3,
			Dimensions: []report.DimDelta{{Name: "coupling_balance", Before: 39, After: 36, Change: -3}},
		},
	}
	var b strings.Builder
	if err := RenderReport(r, &b); err != nil {
		t.Fatalf("RenderReport: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"# archfit — decision",
		"**Decision:** ACCEPTABLE WITH WATCH ITEMS",
		"**Gate:** PASS — 0 blocking",
		"**Score:** 58 / 100 (mixed)",
		"## Recommendations",
		"### Must fix\n- none",
		"coupling_seam",
		"## Why the score is low",
		"**coupling_balance** (36/100",
		"_What moves it:_ Add stable contracts.",
		"## Change vs base",
		"**overall:** -3",
		"**coupling_balance:** 39 → 36 (-3)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown report missing %q\n---\n%s", want, out)
		}
	}
	// Serviceable+ dimension must not appear in the why-low section.
	if strings.Contains(out, "encapsulation** (90") {
		t.Errorf("healthy dimension leaked into why-low:\n%s", out)
	}
}
