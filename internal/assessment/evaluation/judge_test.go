package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// TestJudgeEdgeModuleCycleProduction pins that the not_decided prediction for
// module_cycle counts an edge by the rule's own production predicate: an
// edge with no location and no file node is production, whatever the
// importing file's class, as module_cycle counts it in check.
func TestJudgeEdgeModuleCycleProduction(t *testing.T) {
	t.Parallel()
	modules := map[string]policy.ModuleDef{
		"legacy": {Paths: []string{"myapp.legacy.**"}},
		"domain": {Paths: []string{"myapp.domain.**"}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{Topology: topology}, policy.AssessmentPolicy{Topology: topology},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{{ID: "no_cycles", Type: "module_cycle", Gate: "fail"}}}}, nil, nil)
	edge := func(locations ...relationship.Location) relationship.Set {
		return relationship.Set{Edges: []relationship.Edge{{
			FromID: "module:myapp.legacy.old", ToID: "module:myapp.domain.x", FromPath: "myapp.legacy.old", ToPath: "myapp.domain.x",
			FromModule: "legacy", ToModule: "domain", Kind: "imports", Language: "python", Locations: locations,
		}}}
	}
	notProduction := map[string]bool{"src/myapp/legacy/old.py": false}
	for _, tc := range []struct {
		name string
		set  relationship.Set
		want string
	}{
		{name: "excluded importer without a location counts", set: edge(), want: AnswerNotDecided},
		{name: "a located test importer does not count", set: edge(relationship.Location{File: "src/myapp/legacy/old.py"}), want: AnswerUnconstrained},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := JudgeEdge(JudgeInput{Relationships: tc.set, Policy: snapshot, UnwalkedSourceProduction: notProduction})
			if err != nil {
				t.Fatal(err)
			}
			if got.Answer != tc.want {
				t.Errorf("answer = %q (%v), want %q", got.Answer, got.Reasons, tc.want)
			}
		})
	}
}
