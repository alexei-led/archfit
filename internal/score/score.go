// Package score turns a Diagnostic into the banded scorecard.
// It is a pure, deterministic decision over collected facts.
// coupling_balance follows the scorecard contract and stays LLM-free.
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
	// BandNA marks a dimension that could not be measured (no scored evidence) —
	// e.g. zero scored cross-boundary edges or a degenerate graph. It is NOT a
	// 0-100 value: the tool abstains rather than fabricating a mid-band number.
	BandNA Band = "n/a"
)

// Unmeasured reports whether b is the n/a band (coupling could not be measured).
func (b Band) Unmeasured() bool { return b == BandNA }

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
	// Meta marks a meta-dimension that scores the review process, not the architecture,
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

	// A TypeScript unresolved-specifier ratio above the ceiling means
	// dependency-cruiser could not resolve a meaningful fraction of import
	// specifiers (missing tsconfig path/baseUrl alias, uninstalled dependency) —
	// those edges silently land in the external bucket instead of coupling_balance's
	// internal-edge denominator, so the measured balance reads better than reality.
	// Cap to medium (mirrors the cargo-modules cap above) so a high-noise TS
	// extraction cannot read as a confident verdict.
	if tsUnresolvedPartial(d) && cb.Confidence == ConfidenceHigh {
		cb.Confidence = ConfidenceMedium
		cb.Evidence = append(cb.Evidence,
			"TypeScript unresolved-specifier ratio exceeds threshold — confidence capped to medium "+
				"(path aliases or missing installs may be dropping internal edges as external)")
	}

	return Scorecard{
		RubricVersion: RubricVersion,
		Overall:       cb.Value,
		// cb.Band is bandFor(cb.Value) for a measured dimension (finalize sets it)
		// and BandNA when coupling could not be measured — propagate it verbatim so
		// the overall does not fabricate a band from the n/a sentinel value.
		OverallBand: cb.Band,
		Dimensions:  []Dimension{cb},
	}
}

// finalize enforces the scorecard rules on a dimension: low confidence caps the
// value at mixed (serviceable/strong require ≥ medium confidence), the band is
// always recomputed from the final value (band_matches_value), and a dimension
// with no evidence gets a placeholder so score_requires_evidence holds.
func finalize(dim Dimension) Dimension {
	// n/a dimension: coupling could not be measured. Bypass the value clamp, the
	// low-confidence cap, and the bandFor overwrite — those would turn the n/a
	// sentinel (value 0) into a fabricated "critical" band. Keep Band = BandNA.
	if dim.Band == BandNA {
		if len(dim.Evidence) == 0 {
			dim.Evidence = []string{"no signal available for this dimension"}
		}
		return dim
	}
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

// tsUnresolvedRatioCeiling is the unresolved/total-import-specifiers ratio
// above which dependency-cruiser's coverage is noisy enough to cap
// coupling_balance confidence — the SAME ratio the coverage Reason string
// discloses, so the cap and the disclosure can never contradict each other.
// Deliberate simplification: 10% is a round ceiling, not a
// calibrated figure — raise it if legitimate repos trip this on ordinary
// tsconfig-less noise, lower it if a 10%-noisy extraction still reads as
// confident in practice.
const tsUnresolvedRatioCeiling = 0.10

// toolDepCruiser is the ToolCoverage name the TypeScript extractor reports under.
const toolDepCruiser = "dependency-cruiser"

// tsUnresolvedPartial reports whether the TypeScript extractor (dependency-cruiser)
// reported partial coverage with an unresolved-specifier ratio above
// tsUnresolvedRatioCeiling — a signal that path-alias or module-resolution
// failures are dropping internal edges into the external bucket, which
// coupling_balance excludes from its denominator entirely. SpecifiersSeen 0
// (an extractor that does not track specifier totals) abstains rather than
// divide by a proxy denominator.
func tsUnresolvedPartial(d diagnostic.Diagnostic) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool != toolDepCruiser || c.Status != diagnostic.StatusPartial || c.SpecifiersSeen == 0 {
			continue
		}
		if float64(c.Unresolved)/float64(c.SpecifiersSeen) > tsUnresolvedRatioCeiling {
			return true
		}
	}
	return false
}

// degenerateGraph reports whether the dependency graph is too small to assess
// structure — fewer than two connected first-party modules. blast_radius goes n/a
// exactly in that case (it needs ≥2 modules joined by an edge), so its absence is
// the proxy. On such a graph cycle=0 and coverage is trivially true, carrying no
// signal: the graph-shape dimensions must report n/a, not a vacuous strong. The
// canonical case is a single-crate Rust binary, which archfit's crate-level model
// sees as one node (see internal/extract/rust).
func degenerateGraph(mi metricIndex) bool {
	return !mi.measured("blast_radius")
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
