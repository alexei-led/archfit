// Package decision converts an already-computed Diagnostic and Scorecard into a
// human-decision view-model (Report) that renderers format for display.
//
// It is a pure decision-synthesis package in the core ring: no I/O, no
// subprocess, no YAML, no internal/llm. It reads only already-gathered facts.
package decision

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
)

// Band is the top-level decision band.
type Band string

const (
	// BandFail means blocking work remains.
	BandFail Band = "FAIL"
	// BandNeedsAttention means the overall score is poor or critical.
	BandNeedsAttention Band = "NEEDS_ATTENTION"
	// BandHealthy means the score is serviceable or strong without warnings.
	BandHealthy Band = "HEALTHY"
	// BandAcceptable means the run has watch items but no blocker.
	BandAcceptable Band = "ACCEPTABLE_WITH_WATCH_ITEMS"
)

// Report is the decision value before application projection.
type Report struct {
	Band            Band
	Headline        string
	Blocking        int
	Advisory        int
	Overall         int
	OverallBand     score.Band
	Dimensions      []DimReport
	Recommendations Recommendations
	Delta           *Delta
}

// DimReport is a decision dimension.
type DimReport struct {
	Name       string
	Value      int
	Band       score.Band
	Confidence score.Confidence
	RawValue   int
	CapApplied string
	Meta       bool
	Why        string
	WhatMoves  string
}

// Rec is one actionable recommendation.
type Rec struct {
	Title  string
	Detail string
	RuleID string
}

// Recommendations groups report recommendations.
type Recommendations struct {
	MustFix   []Rec
	ShouldFix []Rec
	Watch     []Rec
	Calibrate []Rec
	Ignore    []Rec
}

// Delta is a score delta.
type Delta struct {
	Overall    int
	Dimensions []DimDelta
}

// DimDelta is the before, after, and signed change for one dimension.
type DimDelta struct {
	Name   string
	Before int
	After  int
	Change int
}

// Build converts an already-computed Diagnostic and Scorecard into a Report.
// base is nil unless a delta is requested (then it's the base-ref scorecard).
// hardGate forces FAIL even when the verdict is not fail (e.g. a tripped opt-in
// tool gate).
func Build(diag result.Result, sc score.Scorecard, base *score.Scorecard, hardGate bool) Report {
	band := decideBand(diag, sc, hardGate)
	dims := buildDims(sc.Dimensions)
	recs := buildRecommendations(diag.Findings)
	var delta *Delta
	if base != nil {
		delta = buildDelta(*base, sc)
	}
	return Report{
		Band:            band,
		Headline:        headlineFor(band),
		Blocking:        diag.Summary.GateFindings,
		Advisory:        diag.Summary.Warnings,
		Overall:         sc.Overall,
		OverallBand:     sc.OverallBand,
		Dimensions:      dims,
		Recommendations: recs,
		Delta:           delta,
	}
}

// ---------------------------------------------------------------------------
// Band logic (total order, exhaustive — every input lands in exactly one band)
// ---------------------------------------------------------------------------

// decideBand applies the band-logic total order. Band ordering comes from
// score.BandRank (the canonical source) so decision and score never diverge.
func decideBand(diag result.Result, sc score.Scorecard, hardGate bool) Band {
	// 1. FAIL: hard gate tripped OR verdict is fail.
	if hardGate || diag.Verdict == result.VerdictFail {
		return BandFail
	}
	// 2. NEEDS_ATTENTION: the OVERALL band is poor or critical. A single low
	//    dimension does NOT escalate here — soft/advisory criticals (e.g.
	//    cohesion clone-scoring) are calibration-sensitive and belong in WATCH +
	//    "why the score is low", not the headline verdict. Genuine hard problems
	//    surface as gate findings (→ FAIL). The decision keys off overall health.
	// An n/a overall (coupling could not be measured) is "unknown", not "poor" —
	// it must not escalate to NEEDS_ATTENTION as if the architecture were bad.
	// BandRank(BandNA) is -1, which would otherwise satisfy this check.
	if !sc.OverallBand.Unmeasured() && score.BandRank(sc.OverallBand) <= score.BandRank(score.BandPoor) {
		return BandNeedsAttention
	}
	// 3. HEALTHY: overall is serviceable or strong AND no advisory warnings AND verdict is not warn.
	if score.BandRank(sc.OverallBand) >= score.BandRank(score.BandServiceable) &&
		diag.Summary.Warnings == 0 &&
		diag.Verdict != result.VerdictWarn {
		return BandHealthy
	}
	// 4. Default arm — covers mixed overall, warnings present, warn verdict, etc.
	return BandAcceptable
}

// headlineFor returns a short summary line keyed by Band.
func headlineFor(b Band) string {
	switch b {
	case BandFail:
		return "Gate violations. Fix before merge."
	case BandNeedsAttention:
		return "Significant structural risks detected. Review required."
	case BandHealthy:
		return "Architecture is healthy. No action required."
	default: // BandAcceptable
		return "Acceptable with watch items. Monitor flagged areas."
	}
}

// ---------------------------------------------------------------------------
// Dimensions
// ---------------------------------------------------------------------------

// whatMovesTable is the static per-dimension remediation guidance.
var whatMovesTable = map[string]string{
	score.DimCouplingBalance: "Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.",
}

// evidenceCap is the maximum number of evidence strings joined into Why.
const evidenceCap = 5

// buildDims converts Scorecard dimensions into DimReport entries.
func buildDims(dims []score.Dimension) []DimReport {
	out := make([]DimReport, 0, len(dims))
	for _, d := range dims {
		out = append(out, DimReport{
			Name:       d.Name,
			Value:      d.Value,
			Band:       d.Band,
			Confidence: d.Confidence,
			RawValue:   d.RawValue,
			CapApplied: d.CapApplied,
			Meta:       d.Meta,
			Why:        buildWhy(d),
			WhatMoves:  whatMovesTable[d.Name], // empty string for meta dim
		})
	}
	return out
}

// buildWhy combines Summary and a capped Evidence join.
func buildWhy(d score.Dimension) string {
	if d.Summary == "" && len(d.Evidence) == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if d.Summary != "" {
		parts = append(parts, d.Summary)
	}
	if len(d.Evidence) > 0 {
		ev := d.Evidence
		if len(ev) > evidenceCap {
			ev = ev[:evidenceCap]
		}
		parts = append(parts, strings.Join(ev, "; "))
	}
	if d.CapApplied != "" {
		parts = append(parts, fmt.Sprintf("cap applied: %s (raw value %d)", d.CapApplied, d.RawValue))
	}
	return strings.Join(parts, " — ")
}

// ---------------------------------------------------------------------------
// Recommendations
// ---------------------------------------------------------------------------

// buildRecommendations groups findings into MustFix / ShouldFix / Watch by
// RuleID. Calibrate and Ignore are allocated as empty slices (an LLM layer
// fills them). Active-gate membership and band logic come from the score
// package so the buckets match the gate verdict.
func buildRecommendations(findings []finding.Finding) Recommendations {
	mustFix := groupByRule(findings, func(f finding.Finding) bool {
		return f.Kind == finding.KindGate && score.IsActiveGateFinding(f)
	})
	shouldFix := groupByRule(findings, func(f finding.Finding) bool {
		return f.Kind == finding.KindAdvisory &&
			(f.Severity == finding.SeverityCritical || f.Severity == finding.SeverityHigh)
	})
	watch := groupByRule(findings, func(f finding.Finding) bool {
		return f.Kind == finding.KindAdvisory &&
			(f.Severity == finding.SeverityMedium || f.Severity == finding.SeverityLow)
	})
	return Recommendations{
		MustFix:   mustFix,
		ShouldFix: shouldFix,
		Watch:     watch,
		Calibrate: []Rec{},
		Ignore:    []Rec{},
	}
}

// groupByRule collects findings matching pred, deduplicates by RuleID, and
// returns one Rec per unique RuleID sorted by RuleID.
func groupByRule(findings []finding.Finding, pred func(finding.Finding) bool) []Rec {
	seen := make(map[string]Rec)
	for _, f := range findings {
		if !pred(f) {
			continue
		}
		if _, ok := seen[f.RuleID]; !ok {
			seen[f.RuleID] = Rec{
				Title:  f.RuleID,
				Detail: f.Why,
				RuleID: f.RuleID,
			}
		}
	}
	if len(seen) == 0 {
		return []Rec{}
	}
	out := make([]Rec, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out
}

// ---------------------------------------------------------------------------
// Delta
// ---------------------------------------------------------------------------

// buildDelta computes signed dimension and overall deltas (After − Before).
func buildDelta(base, current score.Scorecard) *Delta {
	// Index base dimensions by name.
	baseIdx := make(map[string]score.Dimension, len(base.Dimensions))
	for _, d := range base.Dimensions {
		baseIdx[d.Name] = d
	}

	dimDeltas := make([]DimDelta, 0, len(current.Dimensions))
	for _, cur := range current.Dimensions {
		if b, ok := baseIdx[cur.Name]; ok {
			change := cur.Value - b.Value
			// An n/a side (coupling unmeasured) has no real value — suppress the
			// numeric delta so it does not read as a phantom regression/improvement.
			if cur.Band.Unmeasured() || b.Band.Unmeasured() {
				change = 0
			}
			dimDeltas = append(dimDeltas, DimDelta{
				Name:   cur.Name,
				Before: b.Value,
				After:  cur.Value,
				Change: change,
			})
		}
	}
	overall := current.Overall - base.Overall
	if current.OverallBand.Unmeasured() || base.OverallBand.Unmeasured() {
		overall = 0
	}
	return &Delta{
		Overall:    overall,
		Dimensions: dimDeltas,
	}
}
