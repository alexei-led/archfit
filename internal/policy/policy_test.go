package policy

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/view"
)

const (
	policyTestApp   = "app"
	policyTestUnit  = "cli"
	policyTestOwner = "team-a"
)

func TestNewOwnsTopologyAndPolicyProjections(t *testing.T) {
	modules := map[string]module.ModuleDef{policyTestApp: {Owner: policyTestOwner, DeployUnit: policyTestUnit, Paths: []string{"cmd/**"}}}
	labels := map[string]string{policyTestApp + "\x00core": "functional"}
	p := New(
		TopologyView{Modules: modules, Layers: []string{"model", "cmd"}},
		RelationshipPolicy{MinimumSeverity: "medium", ApprovedLabels: labels},
		AssessmentPolicy{},
		GatePolicy{Metrics: map[string]view.MetricConfig{"cycle": {}}},
		map[string]string{"app": "team-a"}, map[string]string{"app": "cli"},
	)

	modules[policyTestApp] = module.ModuleDef{}
	labels[policyTestApp+"\x00core"] = "intrusive"
	if p.Topology.Modules[policyTestApp].Owner != policyTestOwner || p.DeployUnits[policyTestApp] != policyTestUnit {
		t.Fatalf("snapshot does not own module projections: %+v", p)
	}
	if p.Relationship.ApprovedLabels[policyTestApp+"\x00core"] != "functional" {
		t.Fatalf("snapshot aliases approved labels: %+v", p.Relationship.ApprovedLabels)
	}
	if _, ok := p.Relationship.Topology.ModuleMap.ModuleFor("cmd/main.go"); !ok || p.Gates.Metrics == nil {
		t.Fatal("snapshot did not build narrow topology and gate views")
	}
}
