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
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
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

// ---------------------------------------------------------------------------
// Dimension scorers
// ---------------------------------------------------------------------------

// boundaryIntegrity scores whether code respects intended module/layer/ownership
// boundaries. Encapsulation (contract / (contract+intrusive) cross-boundary
// edges) sets the baseline; each active gate violation (a forbidden dependency
// that crossed a boundary) is a measured breach and subtracts a fixed penalty.
func boundaryIntegrity(mi metricIndex, gate []finding.Finding, base Confidence) Dimension {
	dim := Dimension{Name: DimBoundaryIntegrity, Confidence: base}
	// On a <2-module graph the encapsulation metric reports a vacuous 1.0 ("no
	// cross-boundary edges, so nothing leaks"). That is absence of evidence, not
	// earned encapsulation, so don't let it produce a strong band. Gate findings,
	// if any, are still real boundary breaches and take the normal path below.
	if degenerateGraph(mi) && len(gate) == 0 {
		return Dimension{
			Name: DimBoundaryIntegrity, Value: 50, Confidence: ConfidenceLow,
			Evidence: []string{"encapsulation vacuous: no cross-module boundaries on a graph with fewer than two connected modules"},
			Summary:  "boundary integrity unconfirmed: no internal module boundaries to assess",
		}
	}
	var value int
	encMeasured := false
	if enc, ok := mi.measured("encapsulation"); ok {
		encMeasured = true
		value = pct(enc.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("encapsulation %.2f (%s)", enc.Value, enc.Band))
		dim.Confidence = minConf(base, metricConf(enc.Confidence))
	} else {
		// Encapsulation could not be measured (no classified cross-boundary edges —
		// e.g. modules omit owner). Don't fabricate a perfect baseline; start neutral
		// and rely on gate findings for the only measured boundary signal.
		value = 50
		dim.Evidence = append(dim.Evidence,
			"encapsulation: n/a (no classified cross-boundary edges) — boundary baseline unmeasured")
		dim.Confidence = ConfidenceLow
	}

	if n := len(gate); n > 0 {
		pen := capInt(n*15, 70)
		value -= pen
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("%d active gate violation(s): %s", n, sampleIDs(gate)))
		dim.Summary = "boundary violations present: forbidden dependencies cross intended boundaries"
	} else {
		dim.Evidence = append(dim.Evidence, "0 active gate violations")
		if encMeasured {
			dim.Summary = "no gate-level boundary violations; intended boundaries hold"
		} else {
			dim.Summary = "no gate-level boundary violations; encapsulation unmeasured, so boundary integrity is unconfirmed"
		}
	}
	dim.Value = value
	return dim
}

// couplingBalance scores integration strength vs distance vs volatility using
// Vlad Khononov's balance formula from _Balancing Coupling in Software Design_ Ch10.
//
// When a ClassifiedEdgeSummary is supplied (populated from the full coupling.Index
// before advisory filtering), the value comes from the mean book balance over all
// scored cross-boundary edges:
//
//	value = round(100 × (MeanBalance − 1) / 9)
//
// This is a transparent linear rescale of the book's own 1–10 per-edge score
// (balance 1→value 0, balance 10→value 100). Confidence scales with the scored
// fraction (high ≥80%, medium 50–79%, low <50%). Zero scored edges → 60/mixed/low
// (unanalyzed sentinel). The advisory worst-edge cap (critical band) is still
// applied on top: any critical edge → cap at 60; pervasive (≥5% of
// scored+abstained) → cap at 40.
//
// When summary is nil (backward compat), the function falls back to the legacy
// advisory-edge path using the edges []bcEdge slice.
func couplingBalance(edges []bcEdge, mi metricIndex, summary *diagnostic.ClassifiedEdgeSummary) Dimension {
	dim := Dimension{Name: DimCouplingBalance}

	// Degenerate graph: <2 connected modules — coupling unmeasurable regardless of summary.
	if degenerateGraph(mi) {
		dim.Confidence = ConfidenceLow
		dim.Value = 50
		dim.Evidence = []string{"no classified coupling edges on a graph with fewer than two connected modules — coupling unmeasurable"}
		dim.Summary = "coupling balance unmeasured: no internal module structure"
		return dim
	}

	// Advisory worst-edge count — used for the cap logic and evidence in both paths.
	worst := 0
	for _, e := range edges {
		if e.worstCase() {
			worst += e.count
		}
	}

	// --- Distribution path (preferred): use the full classified-edge summary ---
	if summary != nil {
		// Internal cross-boundary edges only (external/library edges excluded from
		// coupling_balance — they are a dependency_graph_health concern, not a
		// coupling_balance concern; the book scores YOUR components only).
		crossBoundary := summary.Scored + summary.Abstained

		// Zero internal cross-boundary edges (or zero scored): unanalyzed sentinel.
		if summary.Scored == 0 {
			dim.Confidence = ConfidenceLow
			dim.Value = 60
			dim.Evidence = []string{
				"0 scored internal cross-boundary edges — coupling balance unconfirmed (edge classification absent or all edges abstained)",
				fmt.Sprintf("worst-case (critical band) edges: %d", worst),
			}
			if summary.External > 0 {
				dim.Evidence = append(dim.Evidence,
					fmt.Sprintf("%d external/library edges excluded — see dependency_graph_health", summary.External))
			}
			dim.Summary = "coupling balance unconfirmed: no scored cross-boundary edges"
			return dim
		}

		// value = round(100 × (MeanBalance − 1) / 9): linear rescale of book's 1–10.
		value := int(math.Round(100 * (summary.MeanBalance - 1) / 9))

		// Confidence from internal-only scored fraction (external edges do not count).
		var conf Confidence
		scoredPct := 100 * summary.Scored / crossBoundary
		switch {
		case scoredPct >= 80:
			conf = ConfidenceHigh
		case scoredPct >= 50:
			conf = ConfidenceMedium
		default:
			conf = ConfidenceLow
		}

		// Advisory cap: worst-case (critical) edges.
		criticalCount := summary.BySeverity[string(coupling.SeverityCritical)]
		switch {
		case crossBoundary > 0 && criticalCount*100/crossBoundary >= 5:
			value = capInt(value, 40) // pervasive distributed-monolith risk
		case criticalCount > 0:
			value = capInt(value, 60) // any critical edge present
		}

		// LLM-provenance labels lower confidence by one band: the strength
		// classifications driving the balance came from an LLM judge (human-approved
		// but not human-judged). Only applied when LLM labels account for ≥20% of
		// scored edges so a single stray label does not flip a well-measured repo.
		llmConfLowered := summary.LLMApproved > 0 && summary.Scored > 0 &&
			summary.LLMApproved*100/summary.Scored >= 20
		if llmConfLowered {
			conf = lowerConf(conf)
		}

		dim.Value = value
		dim.Confidence = conf
		dim.Evidence = []string{
			fmt.Sprintf("%d scored internal cross-boundary edges; mean book balance %.1f/10 → value %d",
				summary.Scored, summary.MeanBalance, value),
			fmt.Sprintf("scored fraction: %d%% (%d scored, %d abstained, internal only)",
				scoredPct, summary.Scored, summary.Abstained),
			fmt.Sprintf("worst-case (critical band) edges: %d", criticalCount),
		}
		if summary.External > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("%d external/library edges excluded — see dependency_graph_health", summary.External))
		}
		if llmConfLowered {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("llm-provenance labels in effect: %d (confidence lowered)", summary.LLMApproved))
		} else if summary.LLMApproved > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("llm-provenance labels in effect: %d", summary.LLMApproved))
		}
		if criticalCount > 0 {
			dim.Summary = "unbalanced coupling: critical-band edges (distributed-monolith risk) present"
		} else {
			dim.Summary = fmt.Sprintf("mean book balance %.1f/10 across %d scored internal cross-boundary edges",
				summary.MeanBalance, summary.Scored)
		}
		return dim
	}

	// Legacy fallback: advisory-edge path (summary == nil).
	// This path is unreachable from engine.go — the engine always populates
	// ClassifiedEdges. It exists for calibration test suites that construct
	// a bare Diagnostic without a ClassifiedEdges summary.
	// --- Legacy fallback: advisory-edge path (summary == nil) ---
	if len(edges) == 0 {
		dim.Confidence = ConfidenceLow
		dim.Value = 60
		dim.Evidence = []string{"no classified coupling edges — coupling balance unconfirmed (edge classification absent, e.g. SCIP not run, or all edges balanced)"}
		dim.Summary = "coupling balance unconfirmed: no classified cross-boundary edges"
		return dim
	}

	var totalEffort, totalEdges int
	for _, e := range edges {
		eff := e.effort
		if eff < 0 {
			eff = effortFromSeverity(e.severity)
		}
		totalEffort += eff * e.count
		totalEdges += e.count
	}
	meanEffort := float64(totalEffort) / float64(totalEdges)
	value := int(math.Round((1 - meanEffort/10) * 100))

	switch {
	case totalEdges > 0 && worst*100/totalEdges >= 5:
		value = capInt(value, 40)
	case worst > 0:
		value = capInt(value, 60)
	}

	dim.Value = value
	dim.Confidence = ConfidenceHigh
	dim.Evidence = []string{
		fmt.Sprintf("%d BC edges (%d rollups); weighted mean maintenance-effort %.1f/10", totalEdges, len(edges), meanEffort),
		fmt.Sprintf("worst-case high/high/high (distributed-monolith) edges: %d", worst),
	}
	if worst > 0 {
		dim.Summary = "unbalanced coupling: high strength × high distance × high volatility edges risk cascading changes"
	} else {
		dim.Summary = "coupling carries elevated maintenance effort but no distributed-monolith edges"
	}
	return dim
}

// dependencyGraphHealth scores cycles, hubs, instability, and propagation cost.
// Import cycles are the dominant penalty (they defeat boundaries outright); hubs,
// unstable modules, and high propagation cost each subtract a bounded amount.
func dependencyGraphHealth(mi metricIndex, base Confidence) Dimension {
	dim := Dimension{Name: DimDependencyGraphHealth, Confidence: base}
	if degenerateGraph(mi) {
		// 0 cycles / ~0 propagation are trivially true on a <2-module graph and
		// say nothing about dependency health — report unmeasured, not strong.
		return Dimension{
			Name: DimDependencyGraphHealth, Value: 50, Confidence: ConfidenceLow,
			Evidence: []string{"dependency graph too small to assess: fewer than two connected first-party modules"},
			Summary:  "dependency graph health unmeasured: no internal module structure to analyse",
		}
	}
	value := 100
	measured := false

	if cyc, ok := mi.get("cycle"); ok {
		measured = true
		n := int(cyc.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("import cycles: %d", n))
		if n > 0 {
			pen := capInt(30+(n-1)*5, 60) // crate/package cycles defeat boundaries
			if cyc.Band != string(BandCritical) {
				// Softened cycles (Rust module-level: language-permitted, often just
				// mutual type references) — a real but mild signal, not a boundary defeat.
				pen = capInt(n*3, 20)
			}
			value -= pen
		}
	}
	if br, ok := mi.measured("blast_radius"); ok {
		measured = true
		n := int(br.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("blast-radius hubs: %d", n))
		value -= capInt(n*4, 20)
	}
	if inst, ok := mi.measured("instability"); ok {
		measured = true
		n := int(inst.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("unstable modules (I>0.7): %d", n))
		value -= capInt(n*2, 15)
	}
	if pc, ok := mi.measured("propagation_cost"); ok {
		measured = true
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("propagation cost: %.2f", pc.Value))
		// propagation_cost is a [0,1] density; cap the penalty at 25 like the
		// sibling penalties so a stray out-of-range value can't dominate.
		value -= capInt(int(math.Round(pc.Value*25)), 25)
	}

	// Manifest deprecation markers: report-only evidence, zero score penalty.
	if dd, ok := mi.get("deprecated_dep_count"); ok && dd.Value > 0 {
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("declared deprecation markers: %d (see deprecated_deps in output)", int(dd.Value)))
	}
	if !measured {
		dim.Confidence = ConfidenceLow
		dim.Evidence = append(dim.Evidence, "no dependency-graph metrics available")
	}
	dim.Value = value
	// Clarify scope: this is the shape of the INTERNAL dependency graph (cycles, hubs,
	// instability, propagation among first-party modules) — not external dependency
	// hygiene (versions, unused/vulnerable deps), which archfit does not score here.
	dim.Summary = "internal dependency-graph shape: cycles, blast-radius hubs, instability, and propagation cost (not external dependency hygiene)"
	return dim
}

// cohesionModularity scores whether modules group related behaviour. God modules
// (structural weight), hidden coupling (co-change without a static edge), and
// cross-module duplication (functional candidates) each subtract a bounded
// penalty. High-strength + low-distance coupling is healthy cohesion — "the good
// coupling" — and is never penalised here.
func cohesionModularity(mi metricIndex, base Confidence) Dimension {
	dim := Dimension{Name: DimCohesionModularity, Confidence: base}
	if degenerateGraph(mi) {
		// "0 god modules / 0 hidden-coupling pairs" is vacuous with <2 modules.
		return Dimension{
			Name: DimCohesionModularity, Value: 50, Confidence: ConfidenceLow,
			Evidence: []string{"cohesion unmeasurable: fewer than two connected first-party modules"},
			Summary:  "cohesion/modularity unmeasured: no internal module structure to analyse",
		}
	}
	value := 100
	measured := false

	if sw, ok := mi.measured("structural_weight"); ok {
		measured = true
		n := int(sw.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("god modules (LOC skew): %d", n))
		value -= capInt(n*12, 40)
	}
	if gf, ok := mi.measured("file_structural_weight"); ok {
		// Evidence-only: file-level god-files are surfaced alongside the module-level
		// signal without double-counting (same root cause, different granularity).
		// No value -= penalty here.
		measured = true
		n := int(gf.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("god files (single-file LOC skew): %d", n))
	}
	if hc, ok := mi.measured("hidden_coupling"); ok {
		measured = true
		n := int(hc.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("hidden-coupling pairs: %d", n))
		value -= capInt(n*8, 30)
	}
	if fc, ok := mi.measured("functional_candidates"); ok {
		measured = true
		n := int(fc.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("cross-module duplication pairs: %d", n))
		value -= capInt(n*5, 25)
	}

	if !measured {
		dim.Confidence = ConfidenceLow
		dim.Evidence = append(dim.Evidence, "no cohesion metrics available (size/history/clone tools absent)")
	} else if _, swMeasured := mi.measured("structural_weight"); !swMeasured {
		// God-module size is the dominant cohesion signal. Without it (e.g. a Rust
		// module graph whose per-module LOC is not yet mapped), "no clones / no hidden
		// coupling" cannot justify a strong band — cap confidence so finalize() holds
		// the value to mixed rather than presenting an unearned strong.
		dim.Confidence = ConfidenceLow
		dim.Evidence = append(dim.Evidence, "god-module size unmeasured (structural_weight n/a) — cohesion unconfirmed")
	}
	dim.Value = value
	dim.Summary = "cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)"
	return dim
}

// changeLocality scores whether changes stay inside intended boundaries. The
// change_locality metric (cross-module edges from changed files, delta runs only)
// is the primary signal; change_coupling and change_amplification (git history)
// fill in for full runs. With no change history and no delta base the dimension
// is unmeasured — neutral value, low confidence.
func changeLocality(mi metricIndex, base Confidence) Dimension {
	dim := Dimension{Name: DimChangeLocality, Confidence: base}
	value := 100
	measured := false

	if cl, ok := mi.measured("change_locality"); ok {
		measured = true
		n := int(cl.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("cross-module edges from changed files: %d", n))
		value -= capInt(n*5, 50)
	}
	if cc, ok := mi.measured("change_coupling"); ok {
		measured = true
		n := int(cc.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("co-changing module pairs: %d", n))
		value -= capInt(n*6, 30)
	}
	if ca, ok := mi.measured("change_amplification"); ok {
		measured = true
		n := int(ca.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("change-amplifying hubs: %d", n))
		value -= capInt(n*4, 20)
	}

	if !measured {
		dim.Value = 50
		dim.Confidence = ConfidenceLow
		dim.Evidence = append(dim.Evidence, "no change-history signal (run with --base, or in a git repo)")
		dim.Summary = "change locality unmeasured: no delta base and no git history"
		return dim
	}
	dim.Value = value
	dim.Summary = "change locality: how much change crosses intended module boundaries"
	return dim
}

// architectureFitness scores whether architecture intent is executable via checks
// rather than only documented. The architecture_fitness metric is a fraction of
// three enforcement signals present (arch tests, import-linter config, an
// arch-linter wired into CI); zero signals means intent is not enforced at all.
func architectureFitness(mi metricIndex, base Confidence) Dimension {
	dim := Dimension{Name: DimArchitectureFitness, Confidence: base}
	if af, ok := mi.measured("architecture_fitness"); ok {
		dim.Value = pct(af.Value)
		dim.Evidence = append(dim.Evidence, "enforcement signals present: "+af.Display)
		dim.Summary = "architecture intent enforced by executable fitness checks"
		return dim
	}
	// Metric n/a means the fitness scan never ran (no enforcement evidence was
	// gathered), not that intent is unenforced. Report poor ("scan didn't run"),
	// not a fabricated critical — critical is reserved for a scan that ran and
	// found 0/3 signals (handled by the measured branch above, value pct(0)=0).
	dim.Value = 40
	dim.Confidence = ConfidenceLow
	dim.Evidence = append(dim.Evidence, "architecture_fitness: n/a (enforcement scan did not run)")
	dim.Summary = "architecture-fitness scan did not run; enforcement of intent is unknown"
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

// analysisConfidence is the meta dimension: how trustworthy this review is given
// tool coverage. When extraction ran, file-extraction coverage sets the baseline.
// When the coverage metric is n/a (no extractor contributed — the repo was not
// analysed), the baseline starts neutral at 60 and each absent primary extractor
// (the injected PrimaryExtractorTools — go/packages, dependency-cruiser, grimp,
// cargo — or defaultPrimaryExtractors when none is injected) subtracts a fixed
// penalty so an all-absent repo lands ~0/critical rather than reading pct(0)=0,
// which hides which extractors are missing. Each absent semantic tool (scip,
// lizard/complexity, jscpd/clones) then lowers confidence in the depth of the
// analysis on top.
func analysisConfidence(d diagnostic.Diagnostic, mi metricIndex, dims []Dimension) Dimension {
	dim := Dimension{Name: DimAnalysisConfidence, Confidence: ConfidenceHigh}
	statuses := toolStatuses(d)
	value := 60

	if cov, ok := mi.measured("coverage"); ok {
		// Extraction ran and produced a real ratio: it sets the baseline.
		value = pct(cov.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("file extraction coverage %.2f", cov.Value))
	} else {
		// Coverage n/a or absent: no extractor analysed the repo. Penalise each
		// missing primary extractor so an all-absent repo collapses to critical.
		dim.Evidence = append(dim.Evidence, "file extraction coverage: n/a (no extractor contributed)")
		primaryAbsent := 0
		for _, tool := range primaryExtractorTools(d) {
			if statuses[tool] != diagnostic.StatusOK {
				primaryAbsent++
			}
		}
		value -= capInt(primaryAbsent*15, 45)
	}

	absent := 0
	for _, tool := range semanticTools {
		st := statuses[tool]
		if st != diagnostic.StatusOK {
			absent++
		}
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("%s: %s", tool, orAbsent(st)))
	}
	value -= capInt(absent*5, 20)

	// n/a-dimension ratio: tool coverage can read high while the graph-derived
	// dimensions still came back unmeasured. Only the STRUCTURAL dimensions signal a
	// measurement-fidelity gap when n/a (a graph too sparse to assess architecture).
	// change_locality (needs --base/git) and architecture_fitness (needs enforcement
	// signals) being n/a is a deliberate scope choice, not a blind spot, so they never
	// lower review confidence. A fully-blind structural set lands at 60 (= the degenerate
	// cap), so the two stay consistent rather than double-penalising.
	naStructural := 0
	for _, dm := range dims {
		if structuralDimensions[dm.Name] && dm.Confidence == ConfidenceLow {
			naStructural++
		}
	}
	if naStructural > 0 {
		ceiling := 100 - naStructural*naDimensionPenalty
		if value > ceiling {
			dim.Evidence = append(dim.Evidence, fmt.Sprintf(
				"%d of %d structural dimensions unmeasured (n/a) — capped at %d",
				naStructural, len(structuralDimensions), ceiling))
			value = ceiling
		}
	}

	// Tool coverage alone overstates trust on a degenerate graph: file extraction can
	// be 100% while the dependency graph is a single node (the single-crate Rust case),
	// leaving the structural dimensions unmeasured. Cap meta-confidence at mixed there
	// so "we read every file" cannot read as "we assessed the architecture".
	if degenerateGraph(mi) && value > 60 {
		dim.Evidence = append(dim.Evidence,
			"capped at 60: dependency graph too small to assess (fewer than two connected modules)")
		value = 60
	}

	dim.Value = value
	dim.Summary = "review trustworthiness given tool coverage and evidence depth"
	return dim
}

// defaultPrimaryExtractors is the fallback set of per-language file extractors
// used when a Diagnostic carries no injected PrimaryExtractorTools (older
// diagnostics, direct score callers). The composition root normally supplies the
// authoritative list from the language registry; this keeps score correct when it
// does not. Checked by exact ToolCoverage.Tool name.
var defaultPrimaryExtractors = []string{"go/packages", "dependency-cruiser", "grimp"}

// primaryExtractorTools returns the file extractors whose coverage the scorecard
// treats as load-bearing: the Diagnostic's injected list, or the built-in default
// when none was injected.
func primaryExtractorTools(d diagnostic.Diagnostic) []string {
	if len(d.PrimaryExtractorTools) > 0 {
		return d.PrimaryExtractorTools
	}
	return defaultPrimaryExtractors
}

// semanticTools are the optional deep-analysis tools whose absence lowers the
// meta confidence. Checked by exact ToolCoverage.Tool name.
var semanticTools = []string{"scip", "lizard", "jscpd"}

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

// bcEdge is a parsed Balanced-Coupling advisory edge (a rollup of count
// same-shape edges).
type bcEdge struct {
	strength, distance, volatility string
	band                           string // score_band
	severity                       string // finding severity
	effort                         int    // score_value 0-10, or -1 when absent
	count                          int    // group_count (≥1)
}

// worstCase reports whether the edge is the Balanced-Coupling worst case — high
// integration strength × high distance × high volatility — which the classifier
// scores as the critical band (a distributed-monolith risk).
func (e bcEdge) worstCase() bool {
	return e.severity == string(coupling.SeverityCritical) || e.band == string(coupling.SeverityCritical)
}

// bcEdges extracts the active Balanced-Coupling advisory edges from the findings,
// parsing the strength/distance/volatility/score fields the engine stamped into
// MatchedBy. Only active advisories (status new or expired_exception) count —
// the same filter activeGateFindings and the gate verdict use. Baseline-accepted
// and excepted edges are operator-suppressed debt; counting them would deflate
// coupling_balance for a repo that has triaged its coupling, diverging from the
// gate's own view. Fixed (resolved) advisories are skipped too.
func bcEdges(fs []finding.Finding) []bcEdge {
	var out []bcEdge
	for _, f := range fs {
		if f.RuleID != "bc/imbalanced_coupling" {
			continue
		}
		if f.Status != finding.StatusNew && f.Status != finding.StatusExpiredExcept {
			continue
		}
		e := bcEdge{
			strength:   f.MatchedBy["strength"],
			distance:   f.MatchedBy["distance"],
			volatility: f.MatchedBy["volatility"],
			band:       f.MatchedBy["score_band"],
			severity:   string(f.Severity),
			effort:     atoiDefault(f.MatchedBy["score_value"], -1),
			count:      atoiDefault(f.MatchedBy["group_count"], 1),
		}
		if e.count < 1 {
			e.count = 1
		}
		out = append(out, e)
	}
	return out
}

// activeGateFindings returns gate findings that still count against the verdict
// (status new or expired_exception), sorted by ID for deterministic sampling.
func activeGateFindings(fs []finding.Finding) []finding.Finding {
	var out []finding.Finding
	for _, f := range fs {
		if f.Kind != "gate" {
			continue
		}
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredExcept {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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

// toolStatuses maps ToolCoverage.Tool → status for the meta dimension.
func toolStatuses(d diagnostic.Diagnostic) map[string]string {
	out := make(map[string]string, len(d.ToolCoverage))
	for _, c := range d.ToolCoverage {
		out[c.Tool] = c.Status
	}
	return out
}

// sampleIDs renders up to three short finding IDs for evidence.
func sampleIDs(fs []finding.Finding) string {
	const maxIDs = 3
	ids := make([]string, 0, maxIDs+1)
	for i, f := range fs {
		if i == maxIDs {
			ids = append(ids, "...")
			break
		}
		id := f.ID
		if len(id) > 8 {
			id = id[:8]
		}
		ids = append(ids, id)
	}
	return strings.Join(ids, ", ")
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

// effortFromSeverity approximates a 0-10 maintenance-effort score for a BC edge
// with no scorer value, from its severity band.
func effortFromSeverity(sev string) int {
	switch sev {
	case string(coupling.SeverityCritical):
		return 9
	case string(coupling.SeverityHigh):
		return 7
	case string(coupling.SeverityMedium):
		return 5
	case string(coupling.SeverityLow):
		return 3
	default:
		return 5
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
