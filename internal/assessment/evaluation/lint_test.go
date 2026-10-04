package evaluation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
)

const gateFail = string(policy.GateFail)

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

// TestLintAndCheckAgreeOnVacuousRules is the parity test: every gated
// non-guard rule lint reports as a dead selector is exactly a rule the decision
// lists as "selector matches nothing", because both read one predicate.
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
	} {
		rule.ID, rule.Type, rule.Gate = "r"+string(rune('a'+i)), ruleForbidden, string(policy.GateFail)
		in.Policy.Gates.Rules.Rules = append(in.Policy.Gates.Rules.Rules, rule)
	}

	var linted []string
	for _, d := range evaluation.LintPolicy(in.Policy, in.Facts) {
		if d.Code == evaluation.LintDeadSelector {
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
		if d.Code == evaluation.LintDeadSelector {
			dead = append(dead, d.Path)
		}
	}
	if want := []string{"rules[internal_typo].to", "rules[schema_typo].to"}; !slices.Equal(dead, want) {
		t.Fatalf("dead selectors = %v, want %v", dead, want)
	}
}
