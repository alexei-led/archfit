package acquisition

import (
	"testing"

	"github.com/alexei-led/archfit/internal/policy"
)

const (
	targetA   = "a/**"
	targetB   = "b/**"
	systemAWS = "aws"
)

func TestClassificationHash(t *testing.T) {
	base := policy.PolicySnapshot{
		Relationship: policy.RelationshipPolicy{VolatilityCascadeEnabled: false},
		Assessment:   policy.AssessmentPolicy{FunctionLOCThreshold: policy.DefaultFunctionLOCThreshold},
		Topology: policy.TopologyView{ExternalSystems: map[string]policy.ExternalSystemDef{
			systemAWS: {Targets: []string{targetB, targetA}, Volatility: "low"},
		}},
	}
	with := func(edit func(*policy.PolicySnapshot)) policy.PolicySnapshot {
		p := base
		p.Topology.ExternalSystems = map[string]policy.ExternalSystemDef{systemAWS: {Targets: []string{targetA, targetB}}}
		edit(&p)
		return p
	}
	tests := []struct {
		name    string
		edit    func(*policy.PolicySnapshot)
		changed bool
	}{
		{"same leaves, other target order and default volatility", func(*policy.PolicySnapshot) {}, false},
		{"explicit default duplicated_knowledge", func(p *policy.PolicySnapshot) {
			p.Relationship.DuplicatedKnowledge = policy.DuplicatedKnowledgePolicyScore
		}, false},
		{"gate mode is governance", func(p *policy.PolicySnapshot) { p.Gates.Coupling.Mode = policy.DistributedMonolithFail }, false},
		{"minimum severity is governance", func(p *policy.PolicySnapshot) { p.Relationship.MinimumSeverity = "high" }, false},
		{"waivers are governance", func(p *policy.PolicySnapshot) { p.Assessment.Waivers = policy.WaiverSet{} }, false},
		{"volatility cascade", func(p *policy.PolicySnapshot) { p.Relationship.VolatilityCascadeEnabled = true }, true},
		{"advisory duplicated_knowledge", func(p *policy.PolicySnapshot) { p.Relationship.DuplicatedKnowledge = "advisory" }, true},
		{"metric switched off", func(p *policy.PolicySnapshot) {
			off := false
			p.Gates.Metrics = map[string]policy.MetricConfig{"cycle": {Enabled: &off}}
		}, true},
		{"metric gate is governance", func(p *policy.PolicySnapshot) {
			p.Gates.Metrics = map[string]policy.MetricConfig{"cycle": {Gate: "warn"}}
		}, false},
		{"function loc threshold", func(p *policy.PolicySnapshot) { p.Assessment.FunctionLOCThreshold = 80 }, true},
		{"external system volatility", func(p *policy.PolicySnapshot) {
			p.Topology.ExternalSystems = map[string]policy.ExternalSystemDef{systemAWS: {Targets: []string{targetA, targetB}, Volatility: "high"}}
		}, true},
		{"external system target", func(p *policy.PolicySnapshot) {
			p.Topology.ExternalSystems = map[string]policy.ExternalSystemDef{systemAWS: {Targets: []string{targetA}}}
		}, true},
		{"external system removed", func(p *policy.PolicySnapshot) { p.Topology.ExternalSystems = nil }, true},
	}
	want := ClassificationHash(base)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationHash(with(tt.edit))
			if (got != want) != tt.changed {
				t.Errorf("hash changed = %v, want %v", got != want, tt.changed)
			}
		})
	}
}
