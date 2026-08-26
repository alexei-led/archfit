package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/policy"
)

// ApplySeamGate exposes the seam-gate escalation over an explicit ledger and
// reference, so the block condition can be pinned without a baseline file.
func ApplySeamGate(diag *result.Result, gate policy.CouplingGate, anchor BaselineAnchor) []string {
	trip := score.EvaluateSeamGate(diag.Seams, seamGateFor(gate), anchor.seamReference())
	applySeamGate(diag, trip)
	return trip.Reasons
}

// RulesetOf and MetricsetOf let the external test package drive Evaluate with
// explicit fakes. They are test-only: this file is not compiled into the
// package's production surface.
func RulesetOf(rs ...rules.Rule) Ruleset { return Ruleset{rules: rs} }

// MetricsetOf builds a Metricset from explicit metric implementations.
func MetricsetOf(ms ...metrics.Metric) Metricset { return Metricset{metrics: ms} }

// StateInput exposes the architecture-state collector input to the behavior
// tests so a dimension can be driven from explicit policy and observations.
type StateInput = stateInput

// Test-only aliases for the evaluation internals. Assess and Score are their
// only production callers; the behavior tests address them directly so a
// disclosure, gate, or projection rule can be pinned in isolation.
var (
	Evaluate          = evaluate
	Finalize          = finalize
	NewMetricset      = newMetricset
	HealthWarnings    = healthWarnings
	ValidationCommand = validationCommand
	ApplyToolGate     = applyToolGate
	BuildState        = buildState
	BuildDimensions   = buildDimensions
)
