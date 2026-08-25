// Package policy owns the domain projections of archfit's architecture policy.
// It has no YAML or filesystem knowledge; config adapters build these values.
package policy

import (
	"maps"
	"slices"
	"time"

	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/view"
)

// TopologyView is the structural policy used to resolve module boundaries.
type TopologyView struct {
	Modules         map[string]module.ModuleDef
	Layers          []string
	ModuleMap       module.Map
	ExternalSystems map[string]view.ExternalSystemDef
	ExplicitOwners  map[string]bool
}

// RelationshipPolicy contains only declarations needed to classify relationships.
type RelationshipPolicy struct {
	Topology                 TopologyView
	MinimumSeverity          string
	VolatilityCascadeEnabled bool
	DuplicatedKnowledge      view.DuplicatedKnowledgePolicy
	PinnedLabels             []labels.Label
	ApprovedLabels           map[string]string
	LLMLabels                map[string]string
	LLMLabelConfidence       map[string]string
}

// ClassifyConfig adapts the relationship projection for the transitional
// classifier seam. It keeps runtime labels outside the static topology.
func (p RelationshipPolicy) ClassifyConfig() view.ClassifyConfig {
	return view.ClassifyConfig{
		Modules: p.Topology.Modules, Layers: p.Topology.Layers, ModuleMap: p.Topology.ModuleMap,
		BCAdvisoryMinSeverity: p.MinimumSeverity, ExplicitOwners: p.Topology.ExplicitOwners,
		VolatilityCascadeEnabled: p.VolatilityCascadeEnabled, ExternalSystems: p.Topology.ExternalSystems,
		DuplicatedKnowledgePolicy: p.DuplicatedKnowledge, ApprovedLabels: maps.Clone(p.ApprovedLabels),
		LLMLabels: maps.Clone(p.LLMLabels), LLMLabelConfidence: maps.Clone(p.LLMLabelConfidence),
	}
}

// AssessmentPolicy contains declarations needed by assessment consumers.
type AssessmentPolicy struct {
	Topology  TopologyView
	Waivers   view.WaiverSet
	Staleness StalenessPolicy
}

// StalenessPolicy controls map-quality review findings.
type StalenessPolicy struct {
	Enabled   bool
	Threshold time.Duration
}

// CouplingGate is the domain representation of coupling.gate. It intentionally
// uses score-independent primitives so config and policy do not depend on the
// score implementation.
type CouplingGate struct {
	Enabled bool
	MinBand string
	MaxDrop *int
}

// GatePolicy contains rule and metric gate declarations.
type GatePolicy struct {
	Rules        view.RuleConfig
	Metrics      map[string]view.MetricConfig
	Coupling     CouplingGate
	ModuleReview view.GateMode
}

// PolicySnapshot is the per-run policy contract. Ownership and deploy units are
// explicit projections rather than callers reaching through a configuration
// aggregate for module metadata.
//
//nolint:revive // the public name is the domain term and is required at boundaries.
type PolicySnapshot struct {
	Topology     TopologyView
	Ownership    map[string]string
	DeployUnits  map[string]string
	Relationship RelationshipPolicy
	Assessment   AssessmentPolicy
	Gates        GatePolicy
}

// New constructs an immutable-by-convention snapshot from narrow stage views.
// It copies maps and slices because pipeline stages augment their local maps.
func New(topology TopologyView, relationship RelationshipPolicy, assessment AssessmentPolicy, gates GatePolicy, ownership, deployUnits map[string]string) PolicySnapshot {
	topology = cloneTopology(topology)
	relationship.Topology = topology
	relationship.ApprovedLabels = maps.Clone(relationship.ApprovedLabels)
	relationship.LLMLabels = maps.Clone(relationship.LLMLabels)
	relationship.LLMLabelConfidence = maps.Clone(relationship.LLMLabelConfidence)
	relationship.PinnedLabels = slices.Clone(relationship.PinnedLabels)
	assessment.Topology = topology
	assessment.Waivers = cloneWaivers(assessment.Waivers)
	gates.Rules = cloneRuleConfig(gates.Rules)
	gates.Rules.ModuleMap = topology.ModuleMap
	gates.Metrics = cloneMetricConfigs(gates.Metrics)
	if gates.Coupling.MaxDrop != nil {
		maxDrop := *gates.Coupling.MaxDrop
		gates.Coupling.MaxDrop = &maxDrop
	}
	return PolicySnapshot{
		Topology: topology, Ownership: maps.Clone(ownership), DeployUnits: maps.Clone(deployUnits),
		Relationship: relationship, Assessment: assessment, Gates: gates,
	}
}

// Clone returns an independent snapshot for per-run enrichment such as inferred
// ownership and deploy units.
func (p PolicySnapshot) Clone() PolicySnapshot {
	return New(p.Topology, p.Relationship, p.Assessment, p.Gates, p.Ownership, p.DeployUnits)
}

func cloneTopology(in TopologyView) TopologyView {
	modules := make(map[string]module.ModuleDef, len(in.Modules))
	for name, def := range in.Modules {
		def.Paths = slices.Clone(def.Paths)
		def.Public = slices.Clone(def.Public)
		def.Internal = slices.Clone(def.Internal)
		modules[name] = def
	}
	external := make(map[string]view.ExternalSystemDef, len(in.ExternalSystems))
	for name, def := range in.ExternalSystems {
		def.Targets = slices.Clone(def.Targets)
		external[name] = def
	}
	return TopologyView{Modules: modules, Layers: slices.Clone(in.Layers), ModuleMap: module.BuildMap(modules), ExternalSystems: external, ExplicitOwners: maps.Clone(in.ExplicitOwners)}
}

func cloneWaivers(in view.WaiverSet) view.WaiverSet {
	return view.WaiverSet{Waivers: slices.Clone(in.Waivers)}
}

func cloneRuleConfig(in view.RuleConfig) view.RuleConfig {
	return view.RuleConfig{Rules: slices.Clone(in.Rules), Layers: slices.Clone(in.Layers), ModuleMap: in.ModuleMap}
}

func cloneMetricConfigs(in map[string]view.MetricConfig) map[string]view.MetricConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]view.MetricConfig, len(in))
	for name, cfg := range in {
		out[name] = cfg
	}
	return out
}
