package policy

import (
	"testing"
)

const (
	policyTestApp   = "app"
	policyTestUnit  = "cli"
	policyTestOwner = "team-a"

	policyTestDeclared = "declared"
	policyTestTeamB    = "team-b"
	policyTestWorker   = "worker"
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
		map[string]string{policyTestBare: policyTestTeamB, policyTestDeclared: "team-z", "unknown": "team-x"},
		map[string]string{policyTestBare: policyTestWorker, policyTestDeclared: "other", "unknown": "ghost"},
	)

	if got := out.Topology.Modules[policyTestBare].Owner; got != policyTestTeamB {
		t.Errorf("bare module owner = %q, want team-b", got)
	}
	if got := out.Topology.Modules[policyTestDeclared].Owner; got != policyTestOwner {
		t.Errorf("declared owner overwritten: %q", got)
	}
	if got := out.DeployUnits[policyTestBare]; got != policyTestWorker {
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

// TestModelHashCoversResolvedTopology pins a settled decision, not an
// implementation detail: the comparability model hash is taken over the
// RESOLVED module map, so a resolver-filled owner or deploy unit moves it.
//
// Distance — and therefore whether a seam qualifies for the distributed-monolith
// gate — is computed from owner and deploy unit. If the hash covered only the
// declared map, a CODEOWNERS edit, a commit by a new author, or an ownership
// git timeout could re-qualify an existing seam and block a no-op commit under
// `mode: fail`. Covering the resolved map turns the same event into an
// abstention with a named cause. See docs/design/architecture-state-reporting.md.
func TestModelHashCoversResolvedTopology(t *testing.T) {
	declared := map[string]ModuleDef{policyTestBare: {Paths: []string{"b/**"}}}
	base := New(TopologyView{Modules: declared}, RelationshipPolicy{}, AssessmentPolicy{}, GatePolicy{}, nil, nil)

	before := ModelHash(base.Topology.Modules)
	afterOwner := ModelHash(base.WithResolvedTopology(map[string]string{policyTestBare: policyTestTeamB}, nil).Topology.Modules)
	afterUnit := ModelHash(base.WithResolvedTopology(nil, map[string]string{policyTestBare: policyTestWorker}).Topology.Modules)

	if afterOwner == before {
		t.Error("a resolver-filled owner must move the model hash: it changes seam distance")
	}
	if afterUnit == before {
		t.Error("a detected deploy unit must move the model hash: it changes seam distance")
	}
	if afterOwner == afterUnit {
		t.Error("owner and deploy unit must be distinguishable in the model hash")
	}
}
