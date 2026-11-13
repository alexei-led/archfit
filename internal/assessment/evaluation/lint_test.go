package evaluation_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	gateFail   = string(policy.GateFail)
	pyCoreFlat = "app/core.py"
)

// lintFixture is a two-module Go tree with one diagnostic per lint code.
func lintFixture() (policy.PolicySnapshot, evaluation.Observations) {
	modules := map[string]policy.ModuleDef{
		"a": {
			Paths: []string{assessPathsA}, Public: []string{"a/api", "b/x", "a/gone"},
			Volatility: "hgih", Subdomain: "cor", Layer: "domian",
		},
		"b":    {Paths: []string{assessPathsB}, Volatility: "Legacy", Subdomain: "core", Layer: "app"},
		"twin": {Paths: []string{assessPathsB}},
	}
	rules := []policy.RuleDef{
		{ID: "live", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: assessPathsB},
		{ID: "typo", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: "b/missing/**"},
		{ID: "off_typo", Type: ruleForbidden, Gate: string(policy.GateOff), From: "c/**", To: assessPathsB},
		{ID: "warn_typo", Type: ruleForbidden, Gate: gateWarnPosture, From: assessPathsA, To: "a/old/**"},
		{ID: "modpath", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: "example.com/m/b"},
		{ID: "guard", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: "a/legacy/**", Guard: true},
		{ID: "stale_guard", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: assessPathsB, Guard: true},
		{ID: "external", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: "github.com/sirupsen/logrus"},
		{ID: "external_prefix", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: selExternalPrefix},
		{ID: "external_prefix_guard", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, To: selExternalPrefix, Guard: true},
		{ID: "cycles", Type: metricCycle, Gate: gateFail, From: "nowhere/**"},
	}
	topology := policy.TopologyView{Modules: modules, Layers: []string{"domain", "app"}, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	facts := evaluation.Observations{
		FileClassIndex: map[string]fileclass.FileClass{
			"a/a.go": fileclass.Production, "a/api/api.go": fileclass.Production, "b/b.go": fileclass.Production,
		},
		GoModulePaths: []string{"example.com/m"},
	}
	return snapshot, facts
}

// TestPolicyWarningsDiscloseWhatLoadingAccepts pins the check-time half: the
// module values schema v2 still loads and the warn-gated vacuous rule become
// config warnings, while a fail-gated vacuous rule is left to the decision.
func TestPolicyWarningsDiscloseWhatLoadingAccepts(t *testing.T) {
	snapshot, facts := lintFixture()
	got := strings.Join(evaluation.PolicyWarnings(snapshot, facts), "\n")
	for _, want := range []string{
		`modules.a.volatility: volatility "hgih"`,
		`modules.a.subdomain: subdomain "cor"`,
		`modules.a.layer: layer "domian"`,
		"rules[warn_typo] is not evaluated: selector matches nothing: to a/old/**",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PolicyWarnings missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"rules[typo]", "rules[off_typo]", "rules[guard]", "modules.b"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("PolicyWarnings must not report %q:\n%s", unwanted, got)
		}
	}
}

// TestLintSeparatesUnanalysedSourceFromADeadSelector pins that a dependency
// rule aimed only at source no dependency producer analyses is reported as
// such, by lint and by check, not as a selector typo, while forbidden_pattern,
// which reads the ast-grep pattern pass, sees that source.
func TestLintSeparatesUnanalysedSourceFromADeadSelector(t *testing.T) {
	const webFile, webGlob = "web/ui/app.ts", "web/ui/**"
	modules := map[string]policy.ModuleDef{"a": {Paths: []string{assessPathsA}}}
	rules := []policy.RuleDef{
		{ID: "dep", Type: ruleForbidden, Gate: gateWarnPosture, From: webGlob, To: assessPathsA},
		{ID: "pattern_rule", Type: ruleTypePattern, Gate: gateFail, From: webGlob},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	facts := evaluation.Observations{
		FileClassIndex:  map[string]fileclass.FileClass{"a/a.go": fileclass.Production, webFile: fileclass.Production},
		UnanalysedFiles: map[string]struct{}{webFile: {}},
	}
	warnings := strings.Join(evaluation.PolicyWarnings(snapshot, facts), "\n")
	if want := "rules[dep] is not evaluated: selector matches only source no dependency producer analyses: from web/ui/**"; !strings.Contains(warnings, want) {
		t.Errorf("PolicyWarnings = %q, want %q", warnings, want)
	}
}

// TestLintAllowlists pins the allowlist checks: an entry naming no declared
// module is an unknown_module error (loading still accepts it, and check
// discloses it as a config warning), and a module_dependencies rule no module
// gives a list is a dead rule, an error unless the rule is off.
func TestLintAllowlists(t *testing.T) {
	const typeModDeps = "module_dependencies"
	snapshotOf := func(modules map[string]policy.ModuleDef, rules ...policy.RuleDef) policy.PolicySnapshot {
		topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
		return policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
			policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	}
	facts := evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
		assessFileA: fileclass.Production, "b/b.go": fileclass.Production,
	}}
	listed := snapshotOf(map[string]policy.ModuleDef{
		"a": {Paths: []string{assessPathsA}, DependsOn: []string{"b", "bb"}},
		"b": {Paths: []string{assessPathsB}, VisibleTo: []string{"a", "layer:domain"}},
	}, policy.RuleDef{ID: "boundaries", Type: typeModDeps, Gate: gateFail})
	warnings := strings.Join(evaluation.PolicyWarnings(listed, facts), "\n")
	for _, w := range []string{
		`modules.a.depends_on[1]: depends_on entry "bb" names no declared module`,
		`modules.b.visible_to[1]: visible_to entry "layer:domain" names no declared module`,
	} {
		if !strings.Contains(warnings, w) {
			t.Errorf("PolicyWarnings missing %q in:\n%s", w, warnings)
		}
	}

	bare := map[string]policy.ModuleDef{"a": {Paths: []string{assessPathsA}}, "b": {Paths: []string{assessPathsB}}}
	unlisted := snapshotOf(bare,
		policy.RuleDef{ID: "fail_rule", Type: typeModDeps, Gate: gateFail},
		policy.RuleDef{ID: "disabled_rule", Type: typeModDeps, Gate: string(policy.GateOff)},
		policy.RuleDef{ID: "warn_rule", Type: typeModDeps, Gate: gateWarnPosture},
	)
	warnings = strings.Join(evaluation.PolicyWarnings(unlisted, facts), "\n")
	if want := "rules[warn_rule] is not evaluated: selector matches nothing: no module declares depends_on or visible_to"; warnings != want {
		t.Errorf("PolicyWarnings = %q, want %q", warnings, want)
	}
}

// TestLintModuleSelectors pins lint over module selectors: a rule side or an
// allowlist entry that selects no declared module is unknown_module; a
// selector that selects a module by layer, role, or glob is clean; a
// warn-gated rule with such a side is a check config warning.
func TestLintModuleSelectors(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"a": {Paths: []string{assessPathsA}, Layer: layerNameDomain, DependsOn: []string{"layer:" + layerNameApp, selLayerNowhere}},
		"b": {Paths: []string{assessPathsB}, Layer: layerNameApp, Role: policy.RoleAdapter},
		"c": {Paths: []string{"c/main.go"}},
	}
	rules := []policy.RuleDef{
		{ID: "live", Type: ruleForbidden, Gate: gateFail, FromModule: selLayerDomain, ToModule: "role:adapter"},
		{ID: "glob", Type: ruleForbidden, Gate: gateFail, FromModule: "*", To: pkgHTTP},
		{ID: "dead", Type: ruleForbidden, Gate: gateFail, FromModule: "layer:infra", To: pkgHTTP},
		{ID: "warn_dead", Type: ruleForbidden, Gate: gateWarnPosture, From: assessPathsA, ToModule: modWarehouse},
		{ID: "sourceless", Type: ruleForbidden, Gate: gateFail, From: assessPathsA, ToModule: "c"},
	}
	topology := policy.TopologyView{Modules: modules, Layers: []string{layerNameDomain, layerNameApp}, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	facts := evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
		assessFileA: fileclass.Production, assessFileB: fileclass.Production,
	}}
	warnings := strings.Join(evaluation.PolicyWarnings(snapshot, facts), "\n")
	for _, w := range []string{
		"rules[warn_dead] is not evaluated: selector matches nothing: to_module warehouse",
		`modules.a.depends_on[1]: depends_on entry "layer:nowhere" names no declared module, layer, or role`,
	} {
		if !strings.Contains(warnings, w) {
			t.Errorf("PolicyWarnings missing %q in:\n%s", w, warnings)
		}
	}
	if strings.Contains(warnings, "rules[dead]") {
		t.Errorf("a fail-gated vacuous rule is left to the decision, not warned:\n%s", warnings)
	}
}
