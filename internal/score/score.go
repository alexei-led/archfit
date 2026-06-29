// Package score synthesises an already-computed Diagnostic into the architect
// skill's single-dimension banded scorecard (coupling_balance), with a 0-100
// value, a band, a confidence level, and at least one evidence reference.
//
// It is a pure decision over collected facts: it reads the Diagnostic's metrics,
// gate findings, and Balanced-Coupling advisories — it never runs a tool, an
// LLM, or touches the filesystem. coupling_balance is derived strictly from Vlad
// Khononov's balance rule over the BC edges (integration strength × distance ×
// volatility maintenance-effort distribution, plus the worst-case
// high/high/high count), NOT a generic metric average.
//
// Bands and rules mirror the architect scorecard contract (scorecard.yaml,
// rubric_version 1): band must match value; serviceable/strong require at least
// medium confidence; every dimension carries at least one evidence ref.
package score

import (
	"strconv"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// RubricVersion is the architect scorecard rubric this synthesis targets. Two
// scorecards are comparable only when their rubric versions match (scorecard.yaml).
const RubricVersion = 1

// Band is a qualitative label for a 0-100 dimension value (scorecard.yaml bands).
type Band string

// Band constants — edges inclusive, matching scorecard.yaml.
const (
	BandCritical    Band = "critical"    // 0-20
	BandPoor        Band = "poor"        // 21-40
	BandMixed       Band = "mixed"       // 41-60
	BandServiceable Band = "serviceable" // 61-80
	BandStrong      Band = "strong"      // 81-100
)

// Confidence describes how trustworthy a dimension assessment is, independent of
// the value itself (scorecard.yaml confidence_levels).
type Confidence string

// Confidence constants.
const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// Dimension names (scorecard.yaml dimensions).
const (
	DimCouplingBalance = "coupling_balance"
)

// Dimension is one scored axis of the architecture.
type Dimension struct {
	Name string `json:"name"`
	// Value is the 0-100 quality score; higher is healthier.
	Value int `json:"value"`
	// Band is the qualitative label for Value (always consistent with Value).
	Band Band `json:"band"`
	// Confidence is the trust level for this assessment.
	Confidence Confidence `json:"confidence"`
	// Evidence cites the metrics/findings/counts the value rests on. Never empty
	// for a non-meta dimension (score_requires_evidence).
	Evidence []string `json:"evidence"`
	// Summary is a one-line, Balanced-Coupling-aware explanation.
	Summary string `json:"summary"`
	// Meta marks analysis_confidence — it scores the review, not the architecture,
	// and is exempt from the evidence requirement.
	Meta bool `json:"meta,omitempty"`
}

// Scorecard is the synthesised banded assessment across all dimensions.
type Scorecard struct {
	RubricVersion int `json:"rubric_version"`
	// Overall is the coupling_balance value.
	Overall     int         `json:"overall"`
	OverallBand Band        `json:"overall_band"`
	Dimensions  []Dimension `json:"dimensions"`
}

// Synthesize derives the scorecard from a computed Diagnostic.
// The Diagnostic must include Balanced-Coupling advisories (run with advisory
// mode on) for coupling_balance to see the BC edges; without them the dimension
// reports "no unbalanced coupling detected" at medium confidence.
func Synthesize(d diagnostic.Diagnostic) Scorecard {
	mi := indexMetrics(d.Metrics)
	edges := bcEdges(d.Findings)

	cb := finalize(couplingBalance(edges, mi, d.ClassifiedEdges))

	// A partial Rust module graph (some crates' cargo-modules failed) means the
	// graph was incomplete when coupling was computed. Cap to medium so partial
	// coverage cannot read as a confident verdict.
	if cargoModulesPartial(d) && cb.Confidence == ConfidenceHigh {
		cb.Confidence = ConfidenceMedium
		cb.Evidence = append(cb.Evidence,
			"module graph partial (some crates failed cargo-modules) — confidence capped to medium")
	}

	return Scorecard{
		RubricVersion: RubricVersion,
		Overall:       cb.Value,
		OverallBand:   bandFor(cb.Value),
		Dimensions:    []Dimension{cb},
	}
}

// finalize enforces the scorecard rules on a dimension: low confidence caps the
// value at mixed (serviceable/strong require ≥ medium confidence), the band is
// always recomputed from the final value (band_matches_value), and a dimension
// with no evidence gets a placeholder so score_requires_evidence holds.
func finalize(dim Dimension) Dimension {
	dim.Value = clamp(dim.Value)
	if dim.Confidence == ConfidenceLow && dim.Value > 60 {
		dim.Value = 60 // cannot present serviceable/strong on thin evidence
	}
	dim.Band = bandFor(dim.Value)
	if len(dim.Evidence) == 0 {
		dim.Evidence = []string{"no signal available for this dimension"}
	}
	return dim
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// metricIndex is metric results keyed by name for O(1) lookup.
type metricIndex map[string]diagnostic.MetricResult

func indexMetrics(ms []diagnostic.MetricResult) metricIndex {
	mi := make(metricIndex, len(ms))
	for _, m := range ms {
		mi[m.Name] = m
	}
	return mi
}

// get returns the metric result for name, present=false when not run.
func (mi metricIndex) get(name string) (diagnostic.MetricResult, bool) {
	m, ok := mi[name]
	return m, ok
}

// measured reports whether the named metric ran AND produced a real value (not
// the n/a band, which means "no evidence").
func (mi metricIndex) measured(name string) bool {
	m, ok := mi[name]
	return ok && m.Band != "n/a"
}

// cargoModulesPartial reports whether the Rust module-graph tool ran but only
// covered some crates (status "partial") — the structural dimensions are then built
// over an incomplete graph.
func cargoModulesPartial(d diagnostic.Diagnostic) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool == "cargo-modules" {
			return c.Status == diagnostic.StatusPartial
		}
	}
	return false
}

// degenerateGraph reports whether the dependency graph is too small to assess
// structure — fewer than two connected first-party modules. blast_radius and
// instability both go n/a exactly in that case (they need ≥2 modules joined by an
// edge), so their joint absence is the proxy. On such a graph cycle=0,
// propagation≈0, and "0 hidden-coupling pairs" are trivially true and carry no
// signal: the graph-shape dimensions must report n/a, not a vacuous strong. The
// canonical case is a single-crate Rust binary, which archfit's crate-level model
// sees as one node (see internal/extract/rust).
func degenerateGraph(mi metricIndex) bool {
	return !mi.measured("blast_radius") && !mi.measured("instability")
}

// coverageConfidence derives the baseline confidence level shared by the
// structural dimensions from file-extraction coverage: high ≥0.8, medium ≥0.5,
// else low. Defaults to medium when the coverage metric did not run.
func coverageConfidence(_ diagnostic.Diagnostic, mi metricIndex) Confidence {
	cov, ok := mi.get("coverage")
	if !ok {
		return ConfidenceMedium
	}
	var byValue Confidence
	switch {
	case cov.Value >= 0.8:
		byValue = ConfidenceHigh
	case cov.Value >= 0.5:
		byValue = ConfidenceMedium
	default:
		byValue = ConfidenceLow
	}
	// The coverage metric also carries its own confidence, derived from the
	// unresolved-import ratio: extraction can be 100% (value 1.0) while many
	// edges stayed unresolved (confidence low). Take the lower of the two so
	// unresolved imports cap the scorecard baseline rather than slipping through.
	return minConf(byValue, metricConf(cov.Confidence))
}

// metricConf maps a metric confidence string to a Confidence, defaulting empty
// (unqualified) to high — the same convention the markdown renderer uses.
func metricConf(s string) Confidence {
	switch s {
	case string(ConfidenceLow):
		return ConfidenceLow
	case string(ConfidenceMedium):
		return ConfidenceMedium
	default:
		return ConfidenceHigh
	}
}

// confRank ranks confidence levels for comparison (higher = more trustworthy).
func confRank(c Confidence) int {
	switch c {
	case ConfidenceHigh:
		return 2
	case ConfidenceMedium:
		return 1
	default:
		return 0
	}
}

// minConf returns the lower (less trustworthy) of two confidence levels.
func minConf(a, b Confidence) Confidence {
	if confRank(a) <= confRank(b) {
		return a
	}
	return b
}

// lowerConf drops a confidence level by one band (high→medium, medium→low, low→low).
func lowerConf(c Confidence) Confidence {
	switch c {
	case ConfidenceHigh:
		return ConfidenceMedium
	case ConfidenceMedium:
		return ConfidenceLow
	default:
		return ConfidenceLow
	}
}

// bandFor maps a 0-100 value to its band (scorecard.yaml; edges inclusive).
func bandFor(v int) Band {
	switch {
	case v <= 20:
		return BandCritical
	case v <= 40:
		return BandPoor
	case v <= 60:
		return BandMixed
	case v <= 80:
		return BandServiceable
	default:
		return BandStrong
	}
}

// BandRank returns b's ordinal (0=critical … 4=strong, -1=unknown) for band
// ≥/≤ comparisons. The canonical band ordering lives here so other packages
// (e.g. internal/decision) order bands consistently.
func BandRank(b Band) int {
	switch b {
	case BandCritical:
		return 0
	case BandPoor:
		return 1
	case BandMixed:
		return 2
	case BandServiceable:
		return 3
	case BandStrong:
		return 4
	default:
		return -1
	}
}

// clamp bounds v to [0,100].
func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// capInt bounds v to at most limit (and never below 0 for penalties).
func capInt(v, limit int) int {
	if v > limit {
		return limit
	}
	if v < 0 {
		return 0
	}
	return v
}

// atoiDefault parses s as an int, returning def on failure or empty input.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
