// Package policy owns the domain projections of archfit's architecture policy:
// topology, module and layer semantics, ownership, deploy units, gates, rule
// definitions, and waivers. It has no YAML, filesystem, or run-context
// knowledge; config adapters decode into these values and stages consume them.
package policy

import (
	"maps"
	"slices"
	"time"
)

// TopologyView is the structural policy used to resolve module boundaries.
type TopologyView struct {
	Modules         map[string]ModuleDef
	Layers          []string
	ModuleMap       ModuleMap
	ExternalSystems map[string]ExternalSystemDef
	ExplicitOwners  map[string]bool
}

// RelationshipPolicy contains only declarations needed to classify relationships.
// Per-run enrichment (pinned labels, clone pairs) is a run signal, not policy,
// and reaches relationship analysis through its own stage input.
type RelationshipPolicy struct {
	Topology                 TopologyView
	MinimumSeverity          string
	VolatilityCascadeEnabled bool
	DuplicatedKnowledge      DuplicatedKnowledgePolicy
}

// AssessmentPolicy contains declarations needed by assessment consumers.
type AssessmentPolicy struct {
	Topology  TopologyView
	Waivers   WaiverSet
	Staleness StalenessPolicy
}

// StalenessPolicy controls map-quality review findings.
type StalenessPolicy struct {
	Enabled   bool
	Threshold time.Duration // zero value defaults to 90*24*time.Hour in Check
}

// DistributedMonolithMode is the posture of the distributed-monolith seam rule.
type DistributedMonolithMode string

// Distributed-monolith modes. warn is the default and is diagnostic; fail is
// opt-in and blocks only against a comparable reference.
const (
	DistributedMonolithWarn DistributedMonolithMode = "warn"
	DistributedMonolithFail DistributedMonolithMode = "fail"
)

// CouplingGate is the domain representation of coupling.gate. It intentionally
// uses score-independent primitives so config and policy do not depend on the
// score implementation.
//
// There is no Enabled flag: the distributed-monolith rule always evaluates. An
// absent config block means the warn default, not an absent policy, so a
// missing stanza cannot silently disable the only coupling gate that exists.
type CouplingGate struct {
	Mode        DistributedMonolithMode
	MaxNewSeams int
}

// GatePolicy contains rule and metric gate declarations.
type GatePolicy struct {
	Rules        RuleConfig
	Metrics      map[string]MetricConfig
	Coupling     CouplingGate
	ModuleReview GateMode
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
	assessment.Topology = topology
	assessment.Waivers = cloneWaivers(assessment.Waivers)
	gates.Rules = cloneRuleConfig(gates.Rules)
	gates.Rules.ModuleMap = topology.ModuleMap
	gates.Metrics = cloneMetricConfigs(gates.Metrics)
	if gates.Coupling.Mode == "" {
		// A zero-value GatePolicy still carries the real default: the
		// distributed-monolith rule always evaluates, diagnostically.
		gates.Coupling.Mode = DistributedMonolithWarn
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

// WithResolvedTopology returns an independent snapshot whose modules carry the
// resolved owners and deploy units, with every topology projection rebuilt from
// the same augmented module map. Ownership and deploy-unit resolution runs once
// per run; every stage then reads the same immutable result.
func (p PolicySnapshot) WithResolvedTopology(owners, deployUnits map[string]string) PolicySnapshot {
	out := p.Clone()
	if out.Ownership == nil {
		out.Ownership = map[string]string{}
	}
	if out.DeployUnits == nil {
		out.DeployUnits = map[string]string{}
	}
	for name, owner := range owners {
		if def, ok := out.Topology.Modules[name]; ok && def.Owner == "" {
			def.Owner = owner
			out.Topology.Modules[name] = def
			out.Ownership[name] = owner
		}
	}
	// A discovered deploy unit only fills a gap: a module that declares one keeps
	// it, and a unit reported for a name no module covers is not policy at all.
	for name, unit := range deployUnits {
		if def, ok := out.Topology.Modules[name]; ok && def.DeployUnit == "" {
			def.DeployUnit = unit
			out.Topology.Modules[name] = def
			out.DeployUnits[name] = unit
		}
	}
	out.Topology.ModuleMap = BuildModuleMap(out.Topology.Modules)
	out.Relationship.Topology, out.Assessment.Topology = out.Topology, out.Topology
	out.Gates.Rules.ModuleMap = out.Topology.ModuleMap
	return out
}

// NeedsOwnerResolution reports whether any path-bearing module leaves `owner:`
// undeclared, which is what makes repository-history ownership resolution worth
// its cost.
func (p PolicySnapshot) NeedsOwnerResolution() bool {
	for _, def := range p.Topology.Modules {
		if len(def.Paths) > 0 && def.Owner == "" {
			return true
		}
	}
	return false
}

func cloneTopology(in TopologyView) TopologyView {
	modules := make(map[string]ModuleDef, len(in.Modules))
	for name, def := range in.Modules {
		def.Paths = slices.Clone(def.Paths)
		def.Public = slices.Clone(def.Public)
		def.Internal = slices.Clone(def.Internal)
		modules[name] = def
	}
	external := make(map[string]ExternalSystemDef, len(in.ExternalSystems))
	for name, def := range in.ExternalSystems {
		def.Targets = slices.Clone(def.Targets)
		external[name] = def
	}
	return TopologyView{Modules: modules, Layers: slices.Clone(in.Layers), ModuleMap: BuildModuleMap(modules), ExternalSystems: external, ExplicitOwners: maps.Clone(in.ExplicitOwners)}
}

func cloneWaivers(in WaiverSet) WaiverSet {
	return WaiverSet{Waivers: slices.Clone(in.Waivers)}
}

func cloneRuleConfig(in RuleConfig) RuleConfig {
	return RuleConfig{Rules: slices.Clone(in.Rules), Layers: slices.Clone(in.Layers), ModuleMap: in.ModuleMap}
}

func cloneMetricConfigs(in map[string]MetricConfig) map[string]MetricConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]MetricConfig, len(in))
	for name, cfg := range in {
		out[name] = cfg
	}
	return out
}
