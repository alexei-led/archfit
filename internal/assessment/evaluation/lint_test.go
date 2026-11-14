package evaluation_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
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
	topology := policy.TopologyView{Modules: modules, Layers: []string{"domain", "app"}, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{}, nil, nil)
	facts := evaluation.Observations{
		FileClassIndex: map[string]fileclass.FileClass{
			"a/a.go": fileclass.Production, "a/api/api.go": fileclass.Production, "b/b.go": fileclass.Production,
		},
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
