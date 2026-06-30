package boundary

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// ---------------------------------------------------------------------------
// CycleMetric (cycle.v1)
// ---------------------------------------------------------------------------

// CycleMetric detects dependency cycles using Tarjan's SCC algorithm on the
// graph's import edges. Each SCC of size > 1 is counted as one cycle (spec §10.4).
type CycleMetric struct{}

// Name returns "cycle".
func (m CycleMetric) Name() string { return "cycle" }

// Version returns "cycle.v1".
func (m CycleMetric) Version() string { return "cycle.v1" }

// Calculate counts strongly-connected components of size > 1 in the dependency graph.
func (m CycleMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	cycles := 0
	if in.Graph != nil {
		cycles = countCycles(in.Graph)
	}

	value := float64(cycles)
	confidence := result.ConfidenceHigh
	// allRust: every cycle is built solely from Rust edges → intra-crate module
	// cycles (cargo forbids crate cycles), language-permitted. Computed once and
	// reused for the band and the display.
	allRust := cycles > 0 && cyclesAllRust(in.Graph)
	var band string
	switch {
	case cycles == 0:
		band = result.BandStrong
	case allRust:
		// cargo forbids crate cycles, so a cycle built only from Rust edges is
		// module-level — language-permitted, commonly just mutual type references
		// (cargo-modules `uses` edges). Treat as a real but mild signal (poor), not the
		// boundary-defeating critical defect a crate/package cycle is. Any cycle that
		// includes a non-Rust edge (e.g. a Go import cycle in a polyglot repo) stays
		// critical, regardless of which language has the most edges overall.
		band = result.BandPoor
	default:
		band = result.BandCritical
	}
	band = result.ApplyConfidenceCap(band, confidence)

	// Convey WHY the band differs from the raw count: N Rust module cycles band
	// `poor` while fewer real import cycles band `critical`, which reads as inverted
	// if the display shows only the count (F8).
	display := fmt.Sprintf("%d import cycles", cycles)
	if allRust {
		display = fmt.Sprintf("%d intra-crate module cycles (Rust — language-permitted, not import cycles)", cycles)
	}
	delta := result.ComputeDelta(value, in.Baseline, m.Name(), m.Version())

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: "number of import cycles",
		Delta:      delta,
	}
}

// countCycles counts strongly-connected components of size > 1 using
// the shared Tarjan SCC detection in graph.Graph.Cycles.
func countCycles(g *graph.Graph) int {
	return len(g.Cycles())
}

// cyclesAllRust reports whether every detected cycle is built solely from Rust edges
// — the signal that all cycles are intra-crate module cycles (cargo forbids crate
// cycles), which the language permits. It inspects the dependency edges WITHIN each SCC,
// not the global edge-language majority: a polyglot repo whose Rust crate-dep edges
// outnumber a single Go import cycle must still flag that Go cycle as critical. Returns
// false when there are no cycles or any cycle includes a non-Rust edge.
func cyclesAllRust(g *graph.Graph) bool {
	if g == nil {
		return false
	}
	sccs := g.Cycles() // SCCs of size > 1, as node-ID slices
	if len(sccs) == 0 {
		return false
	}
	sccOf := make(map[string]int)
	for i, scc := range sccs {
		for _, id := range scc {
			sccOf[id] = i
		}
	}
	for _, e := range g.Edges() {
		// Only the dependency edges that g.Cycles() considers can form a cycle.
		switch e.Kind {
		case graph.EdgeKindImports, graph.EdgeKindDependsOn, graph.EdgeKindUsesInternal:
		default:
			continue
		}
		// An edge is part of a cycle iff both endpoints sit in the same SCC.
		if fi, ok := sccOf[e.From]; ok {
			if ti, ok2 := sccOf[e.To]; ok2 && fi == ti && e.Language != graph.LangRust {
				return false
			}
		}
	}
	return true
}
