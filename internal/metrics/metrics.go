package metrics

import (
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// Metric is the uniform interface the engine dispatches on: compute a result
// from the full CollectedSignals bag. Concrete metrics do NOT implement this
// directly — they implement the typed Calculator[In] (so each sees only its
// family's input) and are adapted into a Metric by New() via adapt().
type Metric interface {
	Name() string
	Version() string
	Calculate(signal.CollectedSignals) diagnostic.MetricResult
}

// Calculator is a metric that computes from a narrow per-family input In. Each
// metric implements exactly one Calculator[In]; the compiler then forbids it
// from reading any signal outside In.
type Calculator[In any] interface {
	Name() string
	Version() string
	Calculate(In) diagnostic.MetricResult
}

// wrapped adapts a typed Calculator[In] to the uniform Metric by projecting the
// CollectedSignals bag down to the family input In.
type wrapped[In any] struct {
	c       Calculator[In]
	project func(signal.CollectedSignals) In
}

func (w wrapped[In]) Name() string    { return w.c.Name() }
func (w wrapped[In]) Version() string { return w.c.Version() }
func (w wrapped[In]) Calculate(s signal.CollectedSignals) diagnostic.MetricResult {
	return w.c.Calculate(w.project(s))
}

// adapt wraps a typed metric plus its CollectedSignals projection into a uniform
// Metric. Pairing a metric with a projection of the wrong family is a compile
// error, so New() is the single type-checked registry of metric→input families.
func adapt[In any](c Calculator[In], project func(signal.CollectedSignals) In) Metric {
	return wrapped[In]{c: c, project: project}
}

// New returns all metrics in a fixed order — the order the engine reports them,
// which golden output depends on. Each metric is adapted from its typed
// Calculator to the uniform Metric via its family projection.
func New(cfg config.Config) []Metric {
	all := []Metric{
		adapt(boundary.EncapsulationMetric{}, signal.CollectedSignals.AsCommon),
		adapt(boundary.UnbalancedEdgeMetric{}, signal.CollectedSignals.AsCommon),
		adapt(boundary.CycleMetric{}, signal.CollectedSignals.AsCommon),
		adapt(boundary.CoverageMetric{}, signal.CollectedSignals.AsCommon),
		adapt(modularity.BlastRadiusMetric{}, signal.CollectedSignals.AsCommon),
	}

	// Honor explicit `metrics.<name>.enabled: false` config: metrics absent
	// from the config default to enabled; only an explicit entry can disable.
	out := make([]Metric, 0, len(all))
	for _, m := range all {
		if entry, configured := cfg.Metrics[m.Name()]; configured && !entry.Enabled {
			continue
		}
		out = append(out, m)
	}
	return out
}
