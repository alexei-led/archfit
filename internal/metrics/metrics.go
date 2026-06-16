package metrics

import (
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/metrics/boundary"
	"github.com/alexei-led/archfit/internal/metrics/intramodule"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/metrics/risk"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// Metric is the interface every Phase 1 metric implements.
// Name returns the bare metric name (e.g. "encapsulation").
// Version returns the full version string (e.g. "encapsulation.v1").
// Calculate computes the metric from the provided input.
type Metric interface {
	Name() string
	Version() string
	Calculate(in signal.MetricInput) diagnostic.MetricResult
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New returns all Phase 1 metrics. Each metric reads its per-metric config
// via cfg.ForMetric(name) when needed (gate thresholds etc. are consumed by engine).
//
// Volatility for RiskHubMetric is captured here, before any call to
// config.ApplyVolatility, so churn-derived values never contaminate the
// risk_hub signal (that would double-count with change_amplification).
func New(cfg config.Config) []Metric {
	all := []Metric{
		boundary.EncapsulationMetric{},
		boundary.UnbalancedEdgeMetric{},
		boundary.CycleMetric{},
		boundary.CoverageMetric{},
		modularity.BlastRadiusMetric{},
		modularity.ChangeAmplificationMetric{},
		modularity.HiddenCouplingMetric{},
		modularity.StructuralWeightMetric{},
		intramodule.ComplexityMetric{},
		risk.NewMetric(cfg),
		intramodule.ArchitectureFitnessMetric{},
		modularity.FunctionalCandidatesMetric{},
		boundary.ChangeLocalityMetric{},
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
