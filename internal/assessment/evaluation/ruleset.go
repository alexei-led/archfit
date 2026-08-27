package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/policy"
)

// RuleEvidence is the acquired evidence rules match against. It is
// assessment-owned so stage adapters never name a rule implementation type.
type RuleEvidence struct {
	PatternMatches []pattern.Match
	// SyntaxFacts is nil/empty when the syntax pass is off.
	SyntaxFacts []evidence.SyntaxFact
}

// Ruleset is the compiled policy rule set. Stage adapters build one and hand it
// back to evaluate; the rule implementations stay inside assessment.
type Ruleset struct{ rules []rules.Rule }

// NewRuleset compiles the declared policy rules and reports the first
// configuration error.
func NewRuleset(cfg policy.RuleConfig) (Ruleset, error) {
	compiled, err := rules.New(cfg)
	if err != nil {
		return Ruleset{}, err
	}
	return Ruleset{rules: compiled}, nil
}

// Len reports how many rules compiled.
func (r Ruleset) Len() int { return len(r.rules) }

// Metricset is the compiled metric set, built from the same policy the gate
// evaluates. It hides the metric implementations from stage adapters.
type Metricset struct{ metrics []metrics.Metric }

// newMetricset builds the enabled metrics declared by policy.
func newMetricset(cfg map[string]policy.MetricConfig) Metricset {
	return Metricset{metrics: metrics.New(cfg)}
}

// Len reports how many metrics are enabled.
func (m Metricset) Len() int { return len(m.metrics) }
