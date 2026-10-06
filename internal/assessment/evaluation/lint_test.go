package evaluation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
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

func TestLintPolicyReportsEachDefectOnce(t *testing.T) {
	snapshot, facts := lintFixture()
	type key struct{ code, severity, path string }
	diagnostics := evaluation.LintPolicy(snapshot, facts)
	got := make([]key, 0, len(diagnostics))
	for _, d := range diagnostics {
		got = append(got, key{d.Code, d.Severity, d.Path})
	}
	want := []key{
		{evaluation.LintUndeclaredLayer, evaluation.LintSeverityError, "modules.a.layer"},
		{evaluation.LintPublicMatchesNothing, evaluation.LintSeverityError, "modules.a.public[1]"},
		{evaluation.LintPublicOutsideModule, evaluation.LintSeverityError, "modules.a.public[1]"},
		{evaluation.LintPublicMatchesNothing, evaluation.LintSeverityError, "modules.a.public[2]"},
		{evaluation.LintUnknownSubdomain, evaluation.LintSeverityError, "modules.a.subdomain"},
		{evaluation.LintUnknownVolatility, evaluation.LintSeverityError, "modules.a.volatility"},
		{evaluation.LintAmbiguousOwnership, evaluation.LintSeverityError, "modules.b.paths"},
		{evaluation.LintGuardRule, evaluation.LintSeverityInfo, "rules[guard]"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityError, "rules[modpath].to"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityWarning, "rules[off_typo].from"},
		{evaluation.LintGuardMatchesSource, evaluation.LintSeverityWarning, "rules[stale_guard]"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityError, "rules[typo].to"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityError, "rules[warn_typo].to"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("LintPolicy diagnostics:\n got  %v\n want %v", got, want)
	}
}

func TestLintPolicyMessagesNameTheDefect(t *testing.T) {
	snapshot, facts := lintFixture()
	messages := map[string]string{}
	for _, d := range evaluation.LintPolicy(snapshot, facts) {
		messages[d.Path] += d.Message
	}
	for path, want := range map[string]string{
		"rules[typo].to":       "to: b/missing/** matches no scanned source",
		"rules[modpath].to":    "drop the Go module path example.com/m/",
		"modules.a.volatility": `volatility "hgih" is not one of`,
		"modules.a.layer":      `layer "domian" is not declared`,
		"modules.b.paths":      "modules b, twin",
	} {
		if !strings.Contains(messages[path], want) {
			t.Errorf("%s message = %q, want it to contain %q", path, messages[path], want)
		}
	}
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
	diagnostics := evaluation.LintPolicy(snapshot, facts)
	messages := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		messages = append(messages, d.Path+" "+d.Message)
	}
	got := strings.Join(messages, "\n")
	if !strings.Contains(got, "rules[dep].from from: web/ui/** matches only source no dependency producer analyses") {
		t.Errorf("lint does not name the unanalysed source:\n%s", got)
	}
	if strings.Contains(got, "rules[pattern_rule]") {
		t.Errorf("forbidden_pattern over unanalysed source must lint clean:\n%s", got)
	}
	warnings := strings.Join(evaluation.PolicyWarnings(snapshot, facts), "\n")
	if want := "rules[dep] is not evaluated: selector matches only source no dependency producer analyses: from web/ui/**"; !strings.Contains(warnings, want) {
		t.Errorf("PolicyWarnings = %q, want %q", warnings, want)
	}
}

// TestLintAndCheckAgreeOnVacuousRules is the parity test: every gated
// non-guard rule lint reports as a dead selector or an unknown module selector
// is exactly a rule the decision lists as "selector matches nothing", because
// both read one predicate.
func TestLintAndCheckAgreeOnVacuousRules(t *testing.T) {
	diag, in := vacuityFixture()
	in.Policy.Gates.Rules.Rules = nil
	for i, rule := range []policy.RuleDef{
		{From: selShipping, To: "internal/billing/domain"},
		{From: selShipping, To: "internal/biling/domain"},
		{From: selCatalog, To: selBilling},
		{From: selShipping, To: "example.com/shop/internal/billing"},
		{From: selShipping, To: "!(internal/billing/api)"},
		{From: selPyCore, To: "app.adaptr"},
		{From: selShipping, To: "net/http"},
		{From: selJava, To: selBilling},
		{From: assessCore, To: selBilling},
		{FromModule: modWarehouse, To: selBilling},
		{From: selShipping, ToModule: selLayerNowhere},
	} {
		rule.ID, rule.Type, rule.Gate = "r"+string(rune('a'+i)), ruleForbidden, string(policy.GateFail)
		in.Policy.Gates.Rules.Rules = append(in.Policy.Gates.Rules.Rules, rule)
	}

	var linted []string
	for _, d := range evaluation.LintPolicy(in.Policy, in.Facts) {
		if d.Code == evaluation.LintDeadSelector || d.Code == evaluation.LintUnknownModule && strings.HasPrefix(d.Path, "rules[") {
			id, _, _ := strings.Cut(strings.TrimPrefix(d.Path, "rules["), "]")
			linted = append(linted, id)
		}
	}
	var decided []string
	for _, rule := range evaluation.BuildState(diag, in).Decision.UnevaluatedRequiredRules {
		if strings.HasPrefix(rule.Reason, "selector matches nothing: ") {
			decided = append(decided, rule.RuleID)
		}
	}
	slices.Sort(linted)
	if len(linted) == 0 || !slices.Equal(linted, decided) {
		t.Fatalf("lint dead selectors %v, check vacuous rules %v: the two must agree", linted, decided)
	}
}

// TestLintPolicyOrdersOwnershipTiesDeterministically pins a stable order for
// two tied module sets that point at the same config path: both ties name
// module a first, so path and code are equal and only the message differs.
func TestLintPolicyOrdersOwnershipTiesDeterministically(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"a": {Paths: []string{"x/**", "y/**"}},
		"b": {Paths: []string{"x/**"}},
		"c": {Paths: []string{"y/**"}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil)
	facts := evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
		"x/x.go": fileclass.Production, "y/y.go": fileclass.Production,
	}}
	messages := func() []string {
		var out []string
		for _, d := range evaluation.LintPolicy(snapshot, facts) {
			if d.Code == evaluation.LintAmbiguousOwnership {
				out = append(out, d.Path+": "+d.Message)
			}
		}
		return out
	}
	first := messages()
	if len(first) != 2 {
		t.Fatalf("ownership ties = %q, want two (a+b and a+c)", first)
	}
	for range 100 {
		if got := messages(); !slices.Equal(got, first) {
			t.Fatalf("ownership tie order changed between runs:\n first %q\n later %q", first, got)
		}
	}
}

// TestDeadSelectorTellsGoStdlibFromFirstPartySource pins a to: selector that
// names a Go standard-library package as an external ban even when a top-level
// directory shares its first segment, while a typo under that directory, or
// under any other first-party root, is still dead.
func TestDeadSelectorTellsGoStdlibFromFirstPartySource(t *testing.T) {
	const web, netHTTP = "web/**", "net/http"
	modules := map[string]policy.ModuleDef{
		"web":      {Paths: []string{web}},
		"database": {Paths: []string{"database/**"}},
		"core":     {Paths: []string{"internal/**"}},
	}
	rules := []policy.RuleDef{
		{ID: "no_sql", Type: ruleForbidden, Gate: gateFail, From: web, To: "database/sql"},
		{ID: "no_sql_tree", Type: ruleForbidden, Gate: gateFail, From: web, To: "database/sql/**"},
		{ID: "no_net", Type: ruleForbidden, Gate: gateFail, From: web, To: netHTTP},
		{ID: "schema_typo", Type: ruleForbidden, Gate: gateFail, From: web, To: "database/shcema/**"},
		{ID: "internal_typo", Type: ruleForbidden, Gate: gateFail, From: web, To: "internal/nope/**"},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	facts := evaluation.Observations{
		FileClassIndex: map[string]fileclass.FileClass{
			"web/web.go": fileclass.Production, "database/schema/schema.go": fileclass.Production,
			"internal/core/core.go": fileclass.Production, "net/probe.go": fileclass.Production,
		},
		GoStdlibPackages: []string{"database/sql", "database/sql/driver", "net", netHTTP},
	}
	var dead []string
	for _, d := range evaluation.LintPolicy(snapshot, facts) {
		if d.Code == evaluation.LintDeadSelector || d.Code == evaluation.LintUnknownModule && strings.HasPrefix(d.Path, "rules[") {
			dead = append(dead, d.Path)
		}
	}
	if want := []string{"rules[internal_typo].to", "rules[schema_typo].to"}; !slices.Equal(dead, want) {
		t.Fatalf("dead selectors = %v, want %v", dead, want)
	}
}

// TestDeadSelectorJudgesEachSelectorInItsOwnVocabulary pins first-party
// judgement per vocabulary: a slash selector is cut at "/" only, so a Go
// import-path domain beside a same-named directory (go.uber.org/** beside go/)
// stays an external ban, while a dotted selector is cut at "." and judged
// against Python module roots.
func TestDeadSelectorJudgesEachSelectorInItsOwnVocabulary(t *testing.T) {
	const svc = "go/svc/**"
	modules := map[string]policy.ModuleDef{
		"svc": {Paths: []string{svc}},
		"k8s": {Paths: []string{"k8s/**"}},
		"py":  {Paths: []string{"app.**"}},
	}
	rules := []policy.RuleDef{
		{ID: "no_zap", Type: ruleForbidden, Gate: gateFail, From: svc, To: "go.uber.org/**"},
		{ID: "no_zap_root", Type: ruleForbidden, Gate: gateFail, From: svc, To: "go.uber.org"},
		{ID: "no_client_go", Type: ruleForbidden, Gate: gateFail, From: svc, To: "k8s.io/client-go/**"},
		{ID: "go_typo", Type: ruleForbidden, Gate: gateFail, From: svc, To: "go/svx/**"},
		{ID: "py_typo", Type: ruleForbidden, Gate: gateFail, From: selPyCore, To: "app.adaptr.**"},
		{ID: "py_external", Type: ruleForbidden, Gate: gateFail, From: selPyCore, To: "requests.adapters"},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	facts := evaluation.Observations{
		FileClassIndex: map[string]fileclass.FileClass{
			"go/svc/svc.go": fileclass.Production, "k8s/deploy.go": fileclass.Production,
			pyCoreFlat: fileclass.Production, "app/adapter.py": fileclass.Production,
		},
		SourceSelectors: map[string]string{pyCoreFlat: selPyCore, "app/adapter.py": "app.adapter"},
	}
	var dead []string
	for _, d := range evaluation.LintPolicy(snapshot, facts) {
		if d.Code == evaluation.LintDeadSelector || d.Code == evaluation.LintUnknownModule && strings.HasPrefix(d.Path, "rules[") {
			dead = append(dead, d.Path)
		}
	}
	if want := []string{"rules[go_typo].to", "rules[py_typo].to"}; !slices.Equal(dead, want) {
		t.Fatalf("dead selectors = %v, want %v", dead, want)
	}
}

// TestPublicOutsideModuleComparesWhatGlobsMatch pins public_outside_module to
// the nodes a public entry matches, so brace and class globs that select the
// module's own packages are not reported, and an entry reaching another
// module's package still is.
func TestPublicOutsideModuleComparesWhatGlobsMatch(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"ab": {Paths: []string{"internal/{a,b}/**"}, Public: []string{
			"internal/{a,b}/**", "internal/[ab]/api", "internal/{a,c}/api", "internal/z/api",
		}},
		"c": {Paths: []string{"internal/c/**"}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil)
	facts := evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
		"internal/a/api/api.go": fileclass.Production, "internal/b/api/api.go": fileclass.Production,
		"internal/c/api/api.go": fileclass.Production,
	}}
	var outside []string
	for _, d := range evaluation.LintPolicy(snapshot, facts) {
		if d.Code == evaluation.LintPublicOutsideModule {
			outside = append(outside, d.Path)
		}
	}
	if want := []string{"modules.ab.public[2]", "modules.ab.public[3]"}; !slices.Equal(outside, want) {
		t.Fatalf("public_outside_module = %v, want %v", outside, want)
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
	type key struct{ code, severity, path string }
	keysOf := func(diagnostics []evaluation.PolicyDiagnostic) []key {
		out := make([]key, 0, len(diagnostics))
		for _, d := range diagnostics {
			out = append(out, key{d.Code, d.Severity, d.Path})
		}
		return out
	}

	listed := snapshotOf(map[string]policy.ModuleDef{
		"a": {Paths: []string{assessPathsA}, DependsOn: []string{"b", "bb"}},
		"b": {Paths: []string{assessPathsB}, VisibleTo: []string{"a", "layer:domain"}},
	}, policy.RuleDef{ID: "boundaries", Type: typeModDeps, Gate: gateFail})
	want := []key{
		{evaluation.LintUnknownModule, evaluation.LintSeverityError, "modules.a.depends_on[1]"},
		{evaluation.LintUnknownModule, evaluation.LintSeverityError, "modules.b.visible_to[1]"},
	}
	if got := keysOf(evaluation.LintPolicy(listed, facts)); !slices.Equal(got, want) {
		t.Errorf("LintPolicy = %v, want %v", got, want)
	}
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
	want = []key{
		{evaluation.LintDeadSelector, evaluation.LintSeverityWarning, "rules[disabled_rule]"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityError, "rules[fail_rule]"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityError, "rules[warn_rule]"},
	}
	if got := keysOf(evaluation.LintPolicy(unlisted, facts)); !slices.Equal(got, want) {
		t.Errorf("LintPolicy = %v, want %v", got, want)
	}
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
	type key struct{ code, severity, path string }
	diagnostics := evaluation.LintPolicy(snapshot, facts)
	got := make([]key, 0, len(diagnostics))
	for _, d := range diagnostics {
		got = append(got, key{d.Code, d.Severity, d.Path})
	}
	want := []key{
		{evaluation.LintUnknownModule, evaluation.LintSeverityError, "modules.a.depends_on[1]"},
		{evaluation.LintUnknownModule, evaluation.LintSeverityError, "rules[dead].from_module"},
		{evaluation.LintDeadSelector, evaluation.LintSeverityError, "rules[sourceless].to_module"},
		{evaluation.LintUnknownModule, evaluation.LintSeverityError, "rules[warn_dead].to_module"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("LintPolicy = %v, want %v", got, want)
	}
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
