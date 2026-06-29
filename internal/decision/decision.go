// Package decision converts an already-computed Diagnostic and Scorecard into a
// human-decision view-model (Report) that renderers format for display.
//
// It is a pure decision-synthesis package in the core ring: no I/O, no
// subprocess, no YAML, no internal/llm. It reads only already-gathered facts.
package decision

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
)

// Band is the human-decision outcome label for a Report. Distinct from
// score.Band (which labels 0-100 dimension values); this is the top-level gate
// verdict for the overall run.
type Band string

// Band constants for the decision outcome.
const (
	BandFail           Band = "FAIL"
	BandNeedsAttention Band = "NEEDS_ATTENTION"
	BandHealthy        Band = "HEALTHY"
	BandAcceptable     Band = "ACCEPTABLE_WITH_WATCH_ITEMS"
)

// Report is the human-decision view-model produced by Build. Renderers format
// this into text, markdown, JSON, or other output — they do not call score.Synthesize.
type Report struct {
	// Band is the top-level decision outcome for the run.
	Band Band
	// Headline is a short (≤ 10 words) human summary keyed by Band.
	Headline string
	// Blocking is the count of active gate findings (diag.Summary.GateFindings).
	Blocking int
	// Advisory is the count of advisory findings (diag.Summary.Warnings).
	Advisory int
	// Overall is the mean of the six non-meta dimension values (0-100).
	Overall int
	// OverallBand is the qualitative label for Overall.
	OverallBand score.Band
	// Dimensions holds a DimReport for each Scorecard dimension.
	Dimensions []DimReport
	// Recommendations groups findings into actionable buckets.
	Recommendations Recommendations
	// Delta is non-nil when a base scorecard was provided, holding signed deltas.
	Delta *Delta
}

// DimReport is the per-dimension slice of Report.
type DimReport struct {
	Name       string
	Value      int
	Band       score.Band
	Confidence score.Confidence
	// Meta marks analysis_confidence — it scores the review, not the architecture.
	Meta bool
	// Why combines Dimension.Summary with condensed Evidence references.
	Why string
	// WhatMoves is a static per-dimension remediation hint. Empty for meta dims.
	WhatMoves string
}

// Rec is one actionable recommendation item.
type Rec struct {
	Title  string
	Detail string
	RuleID string
}

// Recommendations groups findings by urgency tier.
type Recommendations struct {
	// MustFix: active gate findings (new or expired_waiver), grouped by RuleID.
	MustFix []Rec
	// ShouldFix: advisory findings with critical or high severity, grouped by RuleID.
	ShouldFix []Rec
	// Watch: advisory findings with medium or low severity, grouped by RuleID.
	Watch []Rec
	// Calibrate and Ignore are intentionally empty; filled later by an LLM layer.
	Calibrate []Rec
	Ignore    []Rec
}

// Delta holds signed dimension deltas between a base and current scorecard.
type Delta struct {
	// Overall is the signed change (After − Before) in overall score.
	Overall int
	// Dimensions holds a delta entry for each dimension that exists in both scorecards.
	Dimensions []DimDelta
}

// DimDelta is the before/after/change triple for one dimension.
type DimDelta struct {
	Name   string
	Before int
	After  int
	Change int // After − Before
}

// Build converts an already-computed Diagnostic and Scorecard into a Report.
// base is nil unless a delta is requested (then it's the base-ref scorecard).
// hardGate forces FAIL even when the verdict is not fail (e.g. a tripped opt-in
// tool gate).
func Build(diag diagnostic.Diagnostic, sc score.Scorecard, base *score.Scorecard, hardGate bool) Report {
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
func decideBand(diag diagnostic.Diagnostic, sc score.Scorecard, hardGate bool) Band {
	// 1. FAIL: hard gate tripped OR verdict is fail.
	if hardGate || diag.Verdict == diagnostic.VerdictFail {
		return BandFail
	}
	// 2. NEEDS_ATTENTION: the OVERALL band is poor or critical. A single low
	//    dimension does NOT escalate here — soft/advisory criticals (e.g.
	//    cohesion clone-scoring) are calibration-sensitive and belong in WATCH +
	//    "why the score is low", not the headline verdict. Genuine hard problems
	//    surface as gate findings (→ FAIL). The decision keys off overall health.
	if score.BandRank(sc.OverallBand) <= score.BandRank(score.BandPoor) {
		return BandNeedsAttention
	}
	// 3. HEALTHY: overall is serviceable or strong AND no advisory warnings AND verdict is not warn.
	if score.BandRank(sc.OverallBand) >= score.BandRank(score.BandServiceable) &&
		diag.Summary.Warnings == 0 &&
		diag.Verdict != diagnostic.VerdictWarn {
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
// Meta dimension (analysis_confidence) is omitted — it has no WhatMoves.
var whatMovesTable = map[string]string{
	score.DimBoundaryIntegrity:     "Resolve forbidden-dependency and layer-direction violations at the seam.",
	score.DimCouplingBalance:       "Reduce high-fan-in functional edges across module boundaries or introduce stable contracts.",
	score.DimDependencyGraphHealth: "Break true static cycles; mark lazy-import cycles as recognized/advisory.",
	score.DimCohesionModularity:    "Reduce cross-module duplication; exclude generated/static assets from clone scoring.",
	score.DimChangeLocality:        "Keep changes inside module boundaries; split files that change for many reasons.",
	score.DimArchitectureFitness:   "Add/repair executable fitness checks so the intended boundary stays enforced.",
}

// evidenceCap is the maximum number of evidence strings joined into Why.
const evidenceCap = 3

// buildDims converts Scorecard dimensions into DimReport entries.
func buildDims(dims []score.Dimension) []DimReport {
	out := make([]DimReport, 0, len(dims))
	for _, d := range dims {
		out = append(out, DimReport{
			Name:       d.Name,
			Value:      d.Value,
			Band:       d.Band,
			Confidence: d.Confidence,
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
			dimDeltas = append(dimDeltas, DimDelta{
				Name:   cur.Name,
				Before: b.Value,
				After:  cur.Value,
				Change: cur.Value - b.Value,
			})
		}
	}
	return &Delta{
		Overall:    current.Overall - base.Overall,
		Dimensions: dimDeltas,
	}
}
