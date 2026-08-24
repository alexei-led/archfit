// Package score turns a Diagnostic into the banded scorecard.
// It is a pure, deterministic decision over collected facts.
// coupling_balance follows the scorecard contract and stays LLM-free.
package score

import (
	"strconv"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/model/scan"
)

// RubricVersion is the scorecard contract version.
const RubricVersion = report.RubricVersion

// Band is the scorecard band type.
type Band = report.ScoreBand

// Confidence is the scorecard confidence type.
type Confidence = report.Confidence

// Dimension is one scorecard dimension.
type Dimension = report.Dimension

// Scorecard is the scorecard contract.
type Scorecard = report.Scorecard

// BandCritical through BandNA are scorecard band values.
const (
	BandCritical    = report.ScoreBandCritical
	BandPoor        = report.ScoreBandPoor
	BandMixed       = report.ScoreBandMixed
	BandServiceable = report.ScoreBandServiceable
	BandStrong      = report.ScoreBandStrong
	BandNA          = report.ScoreBandNA

	ConfidenceLow    = report.ConfidenceLow
	ConfidenceMedium = report.ConfidenceMedium
	ConfidenceHigh   = report.ConfidenceHigh

	DimCouplingBalance = report.DimCouplingBalance
)

// Synthesize derives the scorecard from a computed Diagnostic.
// coupling_balance is measured from d.ClassifiedEdges, which the engine
// populates before advisory filtering — so running with advisory mode off
// (--no-advisories) never moves the value, band, or confidence. Advisory
// findings still supply the worst-case edge count quoted in the evidence of an
// unmeasured (n/a) dimension, and they are the only input on the legacy
// nil-ClassifiedEdges path used by calibration suites.
func Synthesize(d scan.Diagnostic) Scorecard {
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

	// A TypeScript unresolved-specifier ratio above the ceiling means
	// dependency-cruiser could not resolve a meaningful fraction of import
	// specifiers (missing tsconfig path/baseUrl alias, uninstalled dependency) —
	// those edges silently land in the external bucket instead of coupling_balance's
	// internal-edge denominator, so the measured balance reads better than reality.
	// Cap to medium (mirrors the cargo-modules cap above) so a high-noise TS
	// extraction cannot read as a confident verdict.
	if tsUnresolvedPartial(d) {
		applyMediumConfidenceCap(&cb,
			"TypeScript unresolved-specifier ratio exceeds threshold — high confidence disallowed "+
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
type metricIndex map[string]report.MetricResult

func indexMetrics(ms []report.MetricResult) metricIndex {
	mi := make(metricIndex, len(ms))
	for _, m := range ms {
		mi[m.Name] = m
	}
	return mi
}

// cargoModulesPartial reports whether the Rust module-graph tool ran but only
// covered some crates (status "partial") — the structural dimensions are then built
// over an incomplete graph.
func cargoModulesPartial(d scan.Diagnostic) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool == "cargo-modules" {
			return c.Status == evidence.StatusPartial
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
func tsUnresolvedPartial(d scan.Diagnostic) bool {
	for _, c := range d.ToolCoverage {
		if c.Tool != toolDepCruiser || c.Status != evidence.StatusPartial || c.SpecifiersSeen == 0 {
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
