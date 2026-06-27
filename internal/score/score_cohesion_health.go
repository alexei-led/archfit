package score

import (
	"fmt"
	"math"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

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

// toolStatuses maps ToolCoverage.Tool → status for the meta dimension.
func toolStatuses(d diagnostic.Diagnostic) map[string]string {
	out := make(map[string]string, len(d.ToolCoverage))
	for _, c := range d.ToolCoverage {
		out[c.Tool] = c.Status
	}
	return out
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
