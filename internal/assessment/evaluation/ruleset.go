package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/metrics"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Metricset holds the report metrics and hides their implementations from
// stage adapters.
type Metricset struct{ metrics []metrics.Metric }

// newMetricset builds the enabled metrics declared by policy.
func newMetricset(cfg map[string]policy.MetricConfig) Metricset {
	return Metricset{metrics: metrics.New(cfg)}
}

// Len reports how many metrics are enabled.
func (m Metricset) Len() int { return len(m.metrics) }
