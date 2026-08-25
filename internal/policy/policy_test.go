package policy

import (
	"testing"
)

const (
	policyTestApp   = "app"
	policyTestUnit  = "cli"
	policyTestOwner = "team-a"

	policyTestDeclared = "declared"
	policyTestBare     = "bare"
)

func TestNewOwnsTopologyAndPolicyProjections(t *testing.T) {
	modules := map[string]ModuleDef{policyTestApp: {Owner: policyTestOwner, DeployUnit: policyTestUnit, Paths: []string{"cmd/**"}}}
	waivers := []WaiverDef{{Rule: "no_cycles", From: policyTestApp}}
	p := New(
		TopologyView{Modules: modules, Layers: []string{"model", "cmd"}},
		RelationshipPolicy{MinimumSeverity: "medium"},
		AssessmentPolicy{Waivers: WaiverSet{Waivers: waivers}},
		GatePolicy{Metrics: map[string]MetricConfig{"cycle": {}}},
		map[string]string{"app": "team-a"}, map[string]string{"app": "cli"},
	)

	modules[policyTestApp] = ModuleDef{}
	waivers[0] = WaiverDef{Rule: "mutated"}
	if p.Topology.Modules[policyTestApp].Owner != policyTestOwner || p.DeployUnits[policyTestApp] != policyTestUnit {
		t.Fatalf("snapshot does not own module projections: %+v", p)
	}
	if p.Assessment.Waivers.Waivers[0].Rule != "no_cycles" {
		t.Fatalf("snapshot aliases waivers: %+v", p.Assessment.Waivers)
	}
	if _, ok := p.Relationship.Topology.ModuleMap.ModuleFor("cmd/main.go"); !ok || p.Gates.Metrics == nil {
		t.Fatal("snapshot did not build narrow topology and gate views")
	}
}

func TestWithResolvedTopologyFillsGapsOnceAndLeavesDeclarationsAlone(t *testing.T) {
	base := New(
		TopologyView{Modules: map[string]ModuleDef{
			policyTestDeclared: {Owner: policyTestOwner, DeployUnit: policyTestUnit, Paths: []string{"a/**"}},
			policyTestBare:     {Paths: []string{"b/**"}},
		}},
		RelationshipPolicy{}, AssessmentPolicy{}, GatePolicy{}, nil, nil,
	)

	out := base.WithResolvedTopology(
		map[string]string{policyTestBare: "team-b", policyTestDeclared: "team-z", "unknown": "team-x"},
		map[string]string{policyTestBare: "worker", policyTestDeclared: "other", "unknown": "ghost"},
	)

	if got := out.Topology.Modules[policyTestBare].Owner; got != "team-b" {
		t.Errorf("bare module owner = %q, want team-b", got)
	}
	if got := out.Topology.Modules[policyTestDeclared].Owner; got != policyTestOwner {
		t.Errorf("declared owner overwritten: %q", got)
	}
	if got := out.DeployUnits[policyTestBare]; got != "worker" {
		t.Errorf("bare deploy unit = %q, want worker", got)
	}
	if _, ok := out.DeployUnits["unknown"]; ok {
		t.Error("recorded a deploy unit for a name no module covers")
	}
	if base.Topology.Modules[policyTestBare].Owner != "" {
		t.Error("resolution mutated the prepared snapshot")
	}
	if !base.NeedsOwnerResolution() || out.NeedsOwnerResolution() {
		t.Error("NeedsOwnerResolution did not track the filled ownership gap")
	}
}
