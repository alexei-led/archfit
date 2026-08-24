// Package boundary implements the boundary-health metrics:
// encapsulation, unbalanced_edge, cycle, and coverage.
// Every metric is a pure function of signal.CommonInput; absent inputs
// yield n/a, never a false zero.
package boundary

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/assessment/metrics/internal/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/report"
)

// ---------------------------------------------------------------------------
// EncapsulationMetric (encapsulation.v1)
// ---------------------------------------------------------------------------

// EncapsulationMetric measures, among cross-boundary edges that take a stance on
// boundary respect, the fraction that go through a contract (spec §10.4).
//
//	value = contract / (contract + intrusive) cross-boundary edges
//	Functional and model coupling are normal public use, not boundary verdicts,
//	so they are excluded from the denominator (alongside unknown).
//	No cross-boundary edges at all  → result.BandNA (no boundary signal: independent
//	  modules, or — commonly — edge distance never classified).
//	Cross-boundary edges but none contract/intrusive → result.BandNA (no boundary signal).
//	Confidence scales with the contract+intrusive fraction of cross-boundary edges.
type EncapsulationMetric struct{}

// Name returns "encapsulation".
func (m EncapsulationMetric) Name() string { return "encapsulation" }

// Version returns "encapsulation.v1".
func (m EncapsulationMetric) Version() string { return "encapsulation.v1" }

// Calculate computes the encapsulation ratio from cross-boundary edge classifications.
//
// The ratio is contract edges over the edges that take a boundary stance
// (contract + intrusive). Functional, model, and unknown strengths are excluded:
// functional/model are normal public coupling and unknown is absence of evidence —
// none is a boundary violation, so none should drag the ratio toward 0.
//
// The metric is indeterminate (result.BandNA) in two cases: (1) no cross-boundary edge is
// contract or intrusive (no signal); (2) there are no intrusive edges at all — then
// the ratio is trivially ~1.0 and cannot distinguish earned encapsulation from the
// compiler-boundary case (Go/TS, where every cross-package import is forced through
// an exported API). Reporting 10/10 there is the over-score this avoids; the
// discriminating modularity signal lives in change-amplification, hidden-coupling,
// and cycles instead.
func (m EncapsulationMetric) Calculate(in signal.CommonInput) report.MetricResult {
	// A nil graph is absent evidence, not an encapsulated codebase — report n/a,
	// never a false-green 1.0 (matches this package's "absent inputs yield n/a"
	// contract).
	if in.Graph == nil {
		return m.naResult(in.Baseline)
	}

	var depEdges, allCross, classifiedCross, contractCross, intrusiveCross int
	for _, e := range in.Graph.Edges() {
		// Only dependency-type edges contribute to encapsulation measurement.
		if e.Kind != graph.EdgeKindImports &&
			e.Kind != graph.EdgeKindDependsOn &&
			e.Kind != graph.EdgeKindUsesInternal {
			continue
		}
		depEdges++
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := in.Classifications[key]
		if !ok {
			continue
		}
		// Cross-boundary: any distance that is not same_module. Declared external
		// systems are skipped too — encapsulation measures whether access to YOUR
		// modules goes through their public surface; a vendor seam has no
		// public/internal glob boundary to honor.
		if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown ||
			cl.Distance == coupling.DistanceExternal {
			continue
		}
		allCross++
		if isClassifiedStrength(cl.Strength) {
			classifiedCross++
			switch cl.Strength {
			case coupling.StrengthContract:
				contractCross++
			case coupling.StrengthIntrusive:
				intrusiveCross++
			}
		}
	}

	// depEdges == 0: the extractor ran but produced a graph with no dependency
	// edges — distinct from the nil-graph case above, where no extractor ran at
	// all. Happens for a non-Go repo, an unanalysed root, or a module with no
	// imports. Report n/a rather than a vacuous 1.0 — an empty graph is absence of
	// evidence, not earned encapsulation.
	if depEdges == 0 {
		return m.naResult(in.Baseline)
	}
	// Dependency edges exist but none cross a module boundary. This is not earned
	// encapsulation — it is absence of a boundary signal: either the modules are
	// genuinely independent, or (the common case) edge distance was never classified
	// (no SCIP/owner config), so every edge fell back to same-module. Reporting a
	// vacuous 1.0 here is the false-green that lets an unclassified module graph
	// (e.g. a Rust crate's module nodes with no owner/strength data) read as perfectly
	// encapsulated. Report n/a instead — the boundary baseline is unmeasured.
	if allCross == 0 {
		return m.naResult(in.Baseline)
	}
	// Coupling exists but no edge strength could be classified → no signal.
	if classifiedCross == 0 {
		return m.naResult(in.Baseline)
	}
	// No intrusive edge to contrast against → the ratio is trivially ~1.0 and
	// non-discriminating. This is the compiler-boundary case (Go/TS: every cross-
	// package import is through an exported API, so "contract" is forced, not
	// earned). Report n/a rather than a false 10/10; the realistic modularity signal
	// lives in change-amplification, hidden-coupling, and cycles.
	if intrusiveCross == 0 {
		return m.naResult(in.Baseline)
	}

	value := float64(contractCross) / float64(classifiedCross)
	return m.encResult(value, classificationConfidence(classifiedCross, allCross), in.Baseline, contractCross, intrusiveCross)
}

// isClassifiedStrength reports whether a strength takes a stance on boundary
// respect, i.e. counts in the encapsulation ratio. Only contract (goes through a
// published interface) and intrusive (reaches into internals) do: encapsulation
// measures contract-vs-internal-leak. Functional (calling a public function) and
// model (using a public data type) are normal public coupling — neither a contract
// nor a leak — so, like unknown, they are excluded from the denominator rather than
// counted against the score. Including them would crush the ratio for any codebase
// that mostly calls public functions (the common case) and produce a false critical.
func isClassifiedStrength(s coupling.Strength) bool {
	switch s {
	case coupling.StrengthContract, coupling.StrengthIntrusive:
		return true
	default:
		return false
	}
}

// classificationConfidence derives confidence from how much of the cross-boundary
// coupling could actually be classified. A score computed from a small classified
// fraction is downgraded by the band cap so the tool never over-claims a good band
// on thin evidence (it never inflates a bad band — see result.ApplyConfidenceCap).
func classificationConfidence(classified, all int) string {
	switch {
	case classified*5 >= all*4: // ≥ 80% classified
		return result.ConfidenceHigh
	case classified*2 >= all: // ≥ 50% classified
		return result.ConfidenceMedium
	default:
		return result.ConfidenceLow
	}
}

func (m EncapsulationMetric) encResult(value float64, confidence string, baseline report.MetricSnapshot, contractCross, intrusiveCross int) report.MetricResult {
	score := value * 10.0
	band := result.ApplyConfidenceCap(result.BandScore(score), confidence)
	delta := result.ComputeDelta(value, baseline, m.Name(), m.Version())
	definition := "contract / (contract + intrusive) cross-boundary edges (functional, model, unknown excluded)"
	// 0 contract with real intrusive evidence reads as "no interface discipline
	// anywhere," which can misread as under-configuration (no public:/internal:
	// globs declared). It usually is not: intrusive has a structural, config-
	// independent signal in every extractor (Python PEP 8-private imports, Go
	// concrete-vs-interface type info, SCIP private-symbol resolution), while
	// contract requires either a declared public: glob or (Go/TS only) a
	// structural interface/type-only signal — for a Python module with no
	// public: glob, contract can ONLY ever be 0 (grimp resolves imports to the
	// defining submodule; a public-API signal cannot be established from
	// structure alone). Attach the raw counts so a reader doesn't dismiss a real
	// 0.0 measurement as missing data.
	if contractCross == 0 && intrusiveCross > 0 {
		definition += fmt.Sprintf("; 0 contract / %d intrusive here is a real measurement on the evidence available, "+
			"not necessarily a config gap", intrusiveCross)
	}
	return report.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    fmt.Sprintf("%.1f/10", score),
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: definition,
		Delta:      delta,
		Direction:  report.DirectionHigherIsBetter,
	}
}

// naResult reports the encapsulation metric as indeterminate: cross-boundary
// coupling exists but no edge strength could be classified, so there is no honest
// score. Band is result.BandNA (not critical), and Delta is nil so the verdict logic does
// not treat an absent score as a regression.
func (m EncapsulationMetric) naResult(_ report.MetricSnapshot) report.MetricResult {
	return report.MetricResult{
		Name:       m.Name(),
		Value:      0,
		Display:    result.BandNA,
		Band:       result.BandNA,
		Confidence: result.ConfidenceLow,
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: "contract / (contract + intrusive) cross-boundary edges (functional, model, unknown excluded)",
		Delta:      nil,
		Direction:  report.DirectionHigherIsBetter,
	}
}
