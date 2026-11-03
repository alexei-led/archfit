package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// checks reads the same applicability and delta evidence as the blocking
// ratchets. It does not evaluate a second gate or change finding acceptance.
func (r RatchetReference) checks(current []result.MetricResult, cfg map[string]policy.MetricConfig) []state.MetricRatchetCheck {
	checks := make([]state.MetricRatchetCheck, 0)
	if !r.Present {
		return checks
	}
	byName := make(map[string]result.MetricResult, len(current))
	for _, metric := range current {
		byName[metric.Name] = metric
	}
	gap, _ := r.unevaluated(cfg)
	for _, name := range ratchetMetricNames(r.Metrics, cfg) {
		check := state.MetricRatchetCheck{Metric: name, Status: state.MetricRatchetUnmeasured}
		metric, observed := byName[name]
		if observed {
			check.Threshold = ratchetThreshold(metric, cfg[name])
		}
		switch {
		case !r.Comparable:
			check.Reason = gap.Reason
		case !observed:
			check.Reason = "current metric was not reported"
		case metric.Version != r.Metrics[name].Version:
			check.Reason = "metric version does not match the accepted reference"
		case metric.Delta == nil:
			check.Reason = "current metric has no comparable delta"
		default:
			check.Status = state.MetricRatchetEvaluated
			check.Before = new(r.Metrics[name].Value)
			check.After = new(metric.Value)
			check.Delta = new(*metric.Delta)
		}
		checks = append(checks, check)
	}
	return checks
}
