package decision_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	gateFail   = "fail"
	gateWarn   = "warn"
	gateOff    = "off"
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
		Metrics:      map[string]decision.MetricDecl{"cycle": {Gate: gateFail}},
		Exclude:      []string{"gen/**"},
		Languages:    map[string]string{"go": "auto"},
		Analyzers:    map[string]string{"scip": "on"},
		Declarations: map[string]string{"layers": `["domain","app"]`, "modules.app.public": `["app/api/**"]`},
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
			edit: func(p *decision.PolicyInput) { p.Metrics["cycle"] = decision.MetricDecl{Gate: gateOff} }},
		{name: "a metric with no entry gaining gate warn is loosened, not tightened", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Metrics["blast_radius"] = decision.MetricDecl{Gate: gateWarn} }},
		{name: "a metric gate cleared to the default is no change", want: decision.PolicyNoChange,
			edit: func(p *decision.PolicyInput) { p.Metrics["cycle"] = decision.MetricDecl{} }},
		{name: "a raised max_new tolerance", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) {
				ten := 10
				p.Metrics["cycle"] = decision.MetricDecl{Gate: gateFail, MaxNew: &ten}
			}},
		{name: "a raised min_delta tolerance", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) {
				half := 0.5
				p.Metrics["cycle"] = decision.MetricDecl{Gate: gateFail, MinDelta: &half}
			}},
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
			edit: func(p *decision.PolicyInput) { p.Languages["go"] = gateOff }},
		{name: "a language from auto to on is raised", want: decision.PolicyTightening,
			edit: func(p *decision.PolicyInput) { p.Languages["go"] = "on" }},
		{name: "an analyzer from on to off", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Analyzers["scip"] = gateOff }},
		{name: "a new module, which may take code from another", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) {
				p.Modules["escape"] = policy.ModuleDef{Paths: []string{"app/billing/**"}}
			}},
		{name: "a path added to a module", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) {
				m := p.Modules[modApp]
				m.Paths = append(m.Paths, "store/private/**")
				p.Modules[modApp] = m
			}},
		{name: "a module's public surface broadened", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Declarations["modules.app.public"] = `["app/**"]` }},
		{name: "the layer order changed", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.Declarations["layers"] = `["app","domain"]` }},
		{name: "a removed module's own declarations are not reported twice", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { delete(p.Modules, modApp); delete(p.Declarations, "modules.app.public") },
			assert: func(t *testing.T, c decision.PolicyChange) {
				if len(c.DeclarationsChanged) != 0 {
					t.Errorf("declarations changed = %+v, want the removal reported once, as coverage lost", c.DeclarationsChanged)
				}
			}},
		{name: "a module removed", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { delete(p.Modules, "store") }},
		{name: "module review lowered", want: decision.PolicyLoosening,
			edit: func(p *decision.PolicyInput) { p.ModuleReviewGate = gateOff }},
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

// TestComparePoliciesLanguageOnToAuto: on fails when the extractor is missing,
// auto skips the language, so on to auto loses coverage.
func TestComparePoliciesLanguageOnToAuto(t *testing.T) {
	t.Parallel()
	cur, cand := basePolicy(), basePolicy()
	cur.Languages["go"] = "on"
	got := decision.ComparePolicies(cur, cand)
	if got.Direction != decision.PolicyLoosening || len(got.CoverageLost) != 1 || got.CoverageLost[0].Kind != decision.CoverageLanguageLowered {
		t.Errorf("on -> auto = %+v, want a language_mode_lowered loosening", got)
	}
}
