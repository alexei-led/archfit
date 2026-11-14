// Package score turns a Diagnostic into the banded scorecard.
// It is a pure, deterministic decision over collected facts.
// coupling_balance follows the scorecard contract and stays LLM-free.
package score

import (
	"strconv"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// RubricVersion is the scorecard contract version.
const RubricVersion = 1

// Band is a qualitative scorecard band.
type Band string

const (
	// BandCritical marks the lowest score band.
	BandCritical Band = "critical"
	// BandPoor marks a poor score band.
	BandPoor Band = "poor"
	// BandMixed marks a mixed score band.
	BandMixed Band = "mixed"
	// BandServiceable marks a serviceable score band.
	BandServiceable Band = "serviceable"
	// BandStrong marks the highest score band.
	BandStrong Band = "strong"
	// BandNA marks an unmeasured score.
	BandNA Band = "n/a"
)

// Unmeasured reports whether the scorecard dimension has no measurement.
func (b Band) Unmeasured() bool { return b == BandNA }

// Confidence describes how trustworthy a scorecard dimension is.
type Confidence string

const (
	// ConfidenceLow marks thin evidence.
	ConfidenceLow Confidence = "low"
	// ConfidenceMedium marks moderate evidence.
	ConfidenceMedium Confidence = "medium"
	// ConfidenceHigh marks strong evidence.
	ConfidenceHigh Confidence = "high"
)

// Dimension is one scored axis of the architecture.
type Dimension struct {
	Name       string     `json:"name"`
	Value      int        `json:"value"`
	Band       Band       `json:"band"`
	Confidence Confidence `json:"confidence"`
	Evidence   []string   `json:"evidence"`
	Summary    string     `json:"summary"`
	RawValue   int        `json:"raw_value,omitempty"`
	CapApplied string     `json:"cap_applied,omitempty"`
	Meta       bool       `json:"meta,omitempty"`
}

// Scorecard is the synthesised banded assessment across all dimensions.
type Scorecard struct {
	RubricVersion int         `json:"rubric_version"`
	Overall       int         `json:"overall"`
	OverallBand   Band        `json:"overall_band"`
	Dimensions    []Dimension `json:"dimensions"`
}

// DimCouplingBalance names the coupling score dimension.
const DimCouplingBalance = "coupling_balance"

// Synthesize derives the scorecard from a computed Diagnostic.
// coupling_balance is measured from d.ClassifiedEdges, which the engine
// populates before advisory filtering — so running with advisory mode off
// (--no-advisories) never moves the value, band, or confidence. Advisory
// findings still supply the worst-case edge count quoted in the evidence of an
// unmeasured (n/a) dimension, and they are the only input on the legacy
// nil-ClassifiedEdges path used by calibration suites.
func Synthesize(d result.Result) Scorecard {
	mi := indexMetrics(d.Metrics)
	edges := bcEdges(d.Findings)

	cb := finalize(couplingBalance(edges, mi, d.ClassifiedEdges))

	// A partial Rust module graph (some crates' cargo-modules failed) means the
	// graph was incomplete when coupling was computed. Cap to medium so partial
	// coverage cannot read as a confident verdict.
	if cargoModulesPartial(d) {
		applyMediumConfidenceCap(&cb,
			"module graph partial (some crates failed cargo-modules) — high confidence disallowed")
	}

	// Any unread TypeScript imports or configuration can hide an internal edge.
	if TSUnresolvedPartial(d) {
		applyMediumConfidenceCap(&cb,
			"TypeScript imports or configuration incomplete (unresolved-specifier ratio or unread inputs) — high confidence disallowed")
	}

	// A partial or timed-out Roslyn run left symbols unresolved or projects
	// unloaded: an unresolved reference may hide a cross-boundary edge. Cap to
	// medium (mirrors the caps above): a partial run is never confident.
	if RoslynIncomplete(d) {
		applyMediumConfidenceCap(&cb,
			"roslyn type graph partial (unresolved symbols or unloaded projects) — high confidence disallowed")
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

func applyMediumConfidenceCap(dim *Dimension, reason string) {
	if dim.Band == BandNA {
		return
	}
	if dim.Confidence == ConfidenceHigh {
		dim.Confidence = ConfidenceMedium
	}
	dim.Evidence = append(dim.Evidence, reason)
}

// metricIndex is metric results keyed by name for O(1) lookup.
type metricIndex map[string]result.MetricResult

func indexMetrics(ms []result.MetricResult) metricIndex {
	mi := make(metricIndex, len(ms))
	for _, m := range ms {
		mi[m.Name] = m
	}
	return mi
}

// cargoModulesPartial reports whether the Rust module-graph tool ran but only
// covered some crates (status "partial") — the structural dimensions are then built
// over an incomplete graph.
func cargoModulesPartial(d result.Result) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool == "cargo-modules" {
			return c.Status == evidence.StatusPartial
		}
	}
	return false
}

const toolTSImports = "tsimports"

// TSUnresolvedPartial keeps incomplete TypeScript evidence from raising confidence.
// The coupling dimension and the score use the same predicate.
func TSUnresolvedPartial(d result.Result) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool == toolTSImports && (c.Status == evidence.StatusPartial || c.Status == evidence.StatusTimedOut) {
			return true
		}
	}
	return false
}

// toolRoslyn is the ToolCoverage name the .NET type graph reports under.
const toolRoslyn = "roslyn"

// RoslynIncomplete reports a Roslyn row that is partial (unresolved symbols or
// unloaded projects) or timed out (the run failed). Unlike a TypeScript
// unresolved specifier, an unresolved symbol has no ratio ceiling: it is a
// missing candidate, not lower precision, so any count counts.
//
// Exported for the same reason as TSUnresolvedPartial: the architecture-state
// coupling envelope must reach the conclusion this package's confidence cap
// does.
func RoslynIncomplete(d result.Result) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool == toolRoslyn && (c.Status == evidence.StatusPartial || c.Status == evidence.StatusTimedOut) {
			return true
		}
	}
	return false
}

// degenerateGraph reports whether the dependency graph is too small to assess
// structure — fewer than two connected first-party modules. blast_radius goes n/a
// exactly in that case (it needs ≥2 modules joined by an edge), so a blast_radius
// result carrying the n/a band is the proxy. On such a graph cycle=0 and coverage
// is trivially true, carrying no signal: the graph-shape dimensions must report
// n/a, not a vacuous strong. The canonical case is a single-crate Rust binary,
// which archfit's crate-level model sees as one node (see internal/extract/rust).
//
// An ABSENT blast_radius (metrics.blast_radius.enabled: false) is not degeneracy
// evidence — treating it as such would let an unrelated config knob silently
// force coupling_balance to n/a and defuse coupling.gate. Without the proxy,
// couplingBalance falls through to the summary path, whose Scored==0 check
// still reports n/a on genuinely degenerate graphs.
func degenerateGraph(mi metricIndex) bool {
	m, ok := mi["blast_radius"]
	return ok && m.Band == "n/a"
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
// (e.g. internal/assessment/decision) order bands consistently.
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
