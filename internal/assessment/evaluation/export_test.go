package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/metrics"
)

// MetricsetOf builds a Metricset from explicit metric implementations.
func MetricsetOf(ms ...metrics.Metric) Metricset { return Metricset{metrics: ms} }

// StateInput exposes the architecture-state collector input to the behavior
// tests so a dimension can be driven from explicit policy and observations.
type StateInput = stateInput

// Test-only aliases for the evaluation internals. Assess and Score are their
// only production callers; the behavior tests address them directly so a
// disclosure or projection can be pinned in isolation.
var (
	Evaluate          = evaluate
	NewMetricset      = newMetricset
	HealthWarnings    = healthWarnings
	ValidationCommand = validationCommand
	BuildState        = buildState
	BuildDimensions   = buildDimensions
)
