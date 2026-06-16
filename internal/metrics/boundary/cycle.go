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
func (m CycleMetric) Calculate(in signal.MetricInput) diagnostic.MetricResult {
	cycles := 0
	if in.Graph != nil {
		cycles = countCycles(in.Graph)
	}

	value := float64(cycles)
	confidence := result.ConfidenceHigh
	var band string
	if cycles == 0 {
		band = result.BandStrong
	} else {
		band = result.BandCritical
	}
	band = result.ApplyConfidenceCap(band, confidence)

	display := fmt.Sprintf("%d import cycles", cycles)
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
