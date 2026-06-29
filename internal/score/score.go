// Package score synthesises an already-computed Diagnostic into the architect
// skill's seven-dimension banded scorecard (boundary_integrity, coupling_balance,
// dependency_graph_health, cohesion_modularity, change_locality,
// architecture_fitness, analysis_confidence), each with a 0-100 value, a band, a
// confidence level, and at least one evidence reference.
//
// It is a pure decision over collected facts: it reads the Diagnostic's metrics,
// gate findings, Balanced-Coupling advisories, and tool coverage — it never runs
// a tool, an LLM, or touches the filesystem. coupling_balance is derived strictly
// from Vlad Khononov's balance rule over the BC edges (integration strength ×
// distance × volatility maintenance-effort distribution, plus the worst-case
// high/high/high count), NOT a generic metric average; cohesion_modularity treats
// high-strength + low-distance coupling as healthy cohesion and never penalises it.
//
// Bands and rules mirror the architect scorecard contract (scorecard.yaml,
// rubric_version 1): band must match value; serviceable/strong require at least
// medium confidence; every non-meta dimension carries at least one evidence ref.
package score

import (
	"math"
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
	DimBoundaryIntegrity     = "boundary_integrity"
	DimCouplingBalance       = "coupling_balance"
	DimDependencyGraphHealth = "dependency_graph_health"
	DimCohesionModularity    = "cohesion_modularity"
	DimChangeLocality        = "change_locality"
	DimArchitectureFitness   = "architecture_fitness"
	DimAnalysisConfidence    = "analysis_confidence"
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

// Scorecard is the synthesised banded assessment across all seven dimensions.
type Scorecard struct {
	RubricVersion int `json:"rubric_version"`
	// Overall is the mean of the six non-meta dimension values.
	Overall     int         `json:"overall"`
	OverallBand Band        `json:"overall_band"`
	Dimensions  []Dimension `json:"dimensions"`
}

// Synthesize derives the seven-dimension scorecard from a computed Diagnostic.
// The Diagnostic must include Balanced-Coupling advisories (run with advisory
// mode on) for coupling_balance to see the BC edges; without them that dimension
// reports "no unbalanced coupling detected" at medium confidence.
func Synthesize(d diagnostic.Diagnostic) Scorecard {
	mi := indexMetrics(d.Metrics)
	edges := bcEdges(d.Findings)
	gate := activeGateFindings(d.Findings)
	base := coverageConfidence(d, mi)

	dims := []Dimension{
		boundaryIntegrity(mi, gate, base),
		couplingBalance(edges, mi, d.ClassifiedEdges),
		dependencyGraphHealth(mi, base),
		cohesionModularity(mi, base),
		changeLocality(mi, base),
		architectureFitness(mi, base),
	}
	for i := range dims {
		dims[i] = finalize(dims[i])
	}

	// A partial Rust module graph (some crates' cargo-modules failed) means the
	// structural dimensions were computed over an incomplete graph — and the surviving
	// nodes can defeat the degenerate-graph guard. Don't present those dims at high
	// confidence; cap to medium so partial coverage cannot read as a confident verdict.
	if cargoModulesPartial(d) {
		for i := range dims {
			// Every graph-derived dimension is built over the same incomplete module
			// graph when cargo-modules only partially succeeded — coupling and boundary
			// included, not just dep-graph/cohesion. Cap them all to medium so partial
			// coverage never reads as a confident verdict.
			if structuralDimensions[dims[i].Name] && dims[i].Confidence == ConfidenceHigh {
				dims[i].Confidence = ConfidenceMedium
				dims[i].Evidence = append(dims[i].Evidence,
					"module graph partial (some crates failed cargo-modules) — confidence capped to medium")
			}
		}
	}

	meta := finalizeMeta(analysisConfidence(d, mi, dims))

	overall := meanValue(dims)
	all := make([]Dimension, 0, len(dims)+1)
	all = append(all, dims...)
	all = append(all, meta)

	return Scorecard{
		RubricVersion: RubricVersion,
		Overall:       overall,
		OverallBand:   bandFor(overall),
		Dimensions:    all,
	}
}

// finalize enforces the scorecard rules on a non-meta dimension: low confidence
// caps the value at mixed (serviceable/strong require ≥ medium confidence), the
// band is always recomputed from the final value (band_matches_value), and a
// dimension with no evidence gets a placeholder so score_requires_evidence holds.
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

// finalizeMeta finalises the analysis_confidence dimension. It is exempt from the
// evidence requirement and the confidence cap (it scores the review itself), but
// its band must still match its value.
func finalizeMeta(dim Dimension) Dimension {
	dim.Meta = true
	dim.Value = clamp(dim.Value)
	dim.Band = bandFor(dim.Value)
	return dim
}

// naDimensionPenalty is the meta-confidence points deducted per unmeasured structural
// dimension. Set so all four structural dimensions blind lands at 60 — the same ceiling
// the degenerate-graph guard applies — while one design-driven n/a (e.g. Rust's
// encapsulation) degrades gently to 90.
const naDimensionPenalty = 10

// structuralDimensions are the graph-derived dimensions whose n/a state means the graph
// was too sparse to measure architecture (a fidelity gap), as opposed to change_locality
// and architecture_fitness, whose n/a is a deliberate scope choice (no --base / no
// enforcement signals) and must not lower review confidence.
var structuralDimensions = map[string]bool{
	DimBoundaryIntegrity:     true,
	DimCouplingBalance:       true,
	DimDependencyGraphHealth: true,
	DimCohesionModularity:    true,
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

// measured returns the metric only when it ran AND produced a real value (not
// the n/a band, which means "no evidence").
func (mi metricIndex) measured(name string) (diagnostic.MetricResult, bool) {
	m, ok := mi[name]
	if !ok || m.Band == "n/a" {
		return diagnostic.MetricResult{}, false
	}
	return m, true
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
	_, br := mi.measured("blast_radius")
	_, inst := mi.measured("instability")
	return !br && !inst
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

// meanValue returns the rounded mean of the dimension values.
func meanValue(dims []Dimension) int {
	if len(dims) == 0 {
		return 0
	}
	sum := 0
	for _, d := range dims {
		sum += d.Value
	}
	return int(math.Round(float64(sum) / float64(len(dims))))
}

// pct maps a [0,1] fraction to a rounded 0-100 percentage.
func pct(f float64) int { return int(math.Round(f * 100)) }

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

// orAbsent returns the status string or "absent" when empty (tool never reported).
func orAbsent(s string) string {
	if s == "" {
		return diagnostic.StatusAbsent
	}
	return s
}
