package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/policy"
)

// ApplyCouplingGate exposes the gate escalation over an explicit scorecard, so
// promotion scope can be pinned without synthesising a score first.
func ApplyCouplingGate(diag *result.Result, card score.Scorecard, gate policy.CouplingGate, anchor BaselineAnchor) {
	applyCouplingGate(diag, card, couplingGateFor(gate), anchor.CouplingScore)
}

// RulesetOf and MetricsetOf let the external test package drive Evaluate with
// explicit fakes. They are test-only: this file is not compiled into the
// package's production surface.
func RulesetOf(rs ...rules.Rule) Ruleset { return Ruleset{rules: rs} }

// MetricsetOf builds a Metricset from explicit metric implementations.
func MetricsetOf(ms ...metrics.Metric) Metricset { return Metricset{metrics: ms} }

// Test-only aliases for the evaluation internals. Assess and Score are their
// only production callers; the behavior tests address them directly so a
// disclosure, gate, or projection rule can be pinned in isolation.
var (
	Evaluate                = evaluate
	Finalize                = finalize
	NewMetricset            = newMetricset
	CouplingGateAnchorStale = couplingGateAnchorStale
	HealthWarnings          = healthWarnings
	ValidationCommand       = validationCommand
	ApplyToolGate           = applyToolGate
)
