package decision_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	gateFail   = "fail"
	gateWarn   = "warn"
	ruleNoImpl = "no_impl"
	typeForbid = "forbidden_dependency"
	modApp     = "app"
)

func basePolicy() decision.PolicyInput {
	maxDecls := 10
	return decision.PolicyInput{
		Rules: []policy.RuleDef{
			{ID: ruleNoImpl, Type: typeForbid, Gate: gateFail, From: "app/**", To: "store/impl/**"},
			{ID: "api_ceiling", Type: "public_api_max", Gate: gateWarn, Max: &maxDecls},
		},
		Modules: map[string]policy.ModuleDef{
			modApp:  {Paths: []string{"app/**"}, DependsOn: []string{"store"}},
			"store": {Paths: []string{"store/**"}},
		},
		MetricGates: map[string]string{"cycle": gateFail},
		Exclude:     []string{"gen/**"},
		Languages:   map[string]string{"go": "auto"},
		Analyzers:   map[string]string{"scip": "on"},
	}
}

func TestComparePolicies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		edit   func(*decision.PolicyInput)
		want   decision.PolicyChangeDirection
		assert func(t *testing.T, c decision.PolicyChange)
	}{
		{name: "no change", edit: func(*decision.PolicyInput) {}, want: decision.PolicyNoChange},
		{name: "a rule removed", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Rules = p.Rules[1:] },
			assert: func(t *testing.T, c decision.PolicyChange) {
				if len(c.RulesRemoved) != 1 || c.RulesRemoved[0].ID != ruleNoImpl || c.RulesRemoved[0].Gate != gateFail {
					t.Errorf("rules removed = %+v", c.RulesRemoved)
				}
			}},
		{name: "a gate lowered from fail to warn", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Rules[0].Gate = gateWarn }},
		{name: "an absent gate is fail for a blocking rule", want: decision.PolicyNoChange,
			edit: func(p *decision.PolicyInput) { p.Rules[0].Gate = "" }},
		{name: "a moved selector counts as loosened", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Rules[0].To = "store/internal/**" },
			assert: func(t *testing.T, c decision.PolicyChange) {
				if len(c.RulesLoosened) != 1 || c.RulesLoosened[0].Field != "selector" {
					t.Errorf("rules loosened = %+v", c.RulesLoosened)
				}
			}},
		{name: "a raised API ceiling", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { more := 20; p.Rules[1].Max = &more }},
		{name: "a lowered API ceiling", want: decision.PolicyTightening,
			edit: func(p *decision.PolicyInput) { less := 5; p.Rules[1].Max = &less }},
		{name: "a metric gate switched off", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.MetricGates["cycle"] = "" }},
		{name: "a waiver added", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) {
				p.Waivers = []policy.WaiverDef{{Rule: ruleNoImpl, Reason: "r", ApprovedBy: "o", Expires: "2027-01-01"}}
			}},
		{name: "an allowlist widened to any", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { m := p.Modules[modApp]; m.DependsOn = nil; p.Modules[modApp] = m },
			assert: func(t *testing.T, c decision.PolicyChange) {
				if len(c.AllowlistsWidened) != 1 || c.AllowlistsWidened[0].From != "[store]" || c.AllowlistsWidened[0].To != "any" {
					t.Errorf("allowlists widened = %+v", c.AllowlistsWidened)
				}
			}},
		{name: "an allowlist narrowed to none", want: decision.PolicyTightening,
			edit: func(p *decision.PolicyInput) { m := p.Modules[modApp]; m.DependsOn = []string{}; p.Modules[modApp] = m }},
		{name: "an allowlist entry swapped", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) {
				m := p.Modules[modApp]
				m.DependsOn = []string{"other"}
				p.Modules[modApp] = m
			}},
		{name: "an exclude added", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Exclude = append(p.Exclude, "app/**") }},
		{name: "a language switched off", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Languages["go"] = "off" }},
		{name: "auto to on is not a change", want: decision.PolicyNoChange,
			edit: func(p *decision.PolicyInput) { p.Languages["go"] = "on" }},
		{name: "a module removed", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { delete(p.Modules, "store") }},
		{name: "module review lowered", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.ModuleReviewGate = "off" }},
		{name: "a rule added", want: decision.PolicyTightening,
			edit: func(p *decision.PolicyInput) {
				p.Rules = append(p.Rules, policy.RuleDef{ID: "new", Type: typeForbid, From: "a/**", To: "b/**"})
			}},
		{name: "one loosening and one tightening", want: decision.PolicyMixed,
			edit: func(p *decision.PolicyInput) {
				p.Rules[0].Gate = gateWarn
				p.Rules = append(p.Rules, policy.RuleDef{ID: "new", Type: typeForbid, From: "a/**", To: "b/**"})
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cand := basePolicy()
			tc.edit(&cand)
			got := decision.ComparePolicies(basePolicy(), cand)
			if got.Direction != tc.want {
				t.Errorf("direction = %q, want %q (%+v)", got.Direction, tc.want, got)
			}
			if tc.assert != nil {
				tc.assert(t, got)
			}
		})
	}
}
