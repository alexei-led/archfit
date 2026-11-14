// Package policy owns the domain projections of archfit's architecture policy:
// topology, module and layer semantics, ownership, deploy units, gates, rule
// definitions, and waivers. It has no YAML, filesystem, or run-context
// knowledge; config adapters decode into these values and stages consume them.
package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

// TopologyView is the structural policy used to resolve module boundaries.
type TopologyView struct {
	Modules         map[string]ModuleDef
	Layers          []string
	ModuleMap       ModuleMap
	ExternalSystems map[string]ExternalSystemDef
}

// RelationshipPolicy contains only declarations needed to classify relationships.
type RelationshipPolicy struct {
	Topology                 TopologyView
	MinimumSeverity          string
	VolatilityCascadeEnabled bool
}

// DefaultFunctionLOCThreshold is the diagnostic-only size-tail cutoff used
// when metrics.function_loc_threshold is absent.
const DefaultFunctionLOCThreshold = 60

// AssessmentPolicy contains declarations needed by assessment consumers.
type AssessmentPolicy struct {
	Topology             TopologyView
	Staleness            StalenessPolicy
	FunctionLOCThreshold int
}

// StalenessPolicy controls map-quality review findings.
type StalenessPolicy struct {
	Enabled   bool
	Threshold time.Duration // zero value defaults to 90*24*time.Hour in Check
}

// GatePolicy contains rule and metric gate declarations.
type GatePolicy struct {
	Metrics      map[string]MetricConfig
	ModuleReview GateMode
	// UncoveredChange is the module_review.change_gate posture; empty means warn.
	UncoveredChange GateMode
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
	gates.Metrics = cloneMetricConfigs(gates.Metrics)
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
		// slices.Clone keeps nil nil: an absent allowlist stays unconstrained.
		def.DependsOn = slices.Clone(def.DependsOn)
		def.VisibleTo = slices.Clone(def.VisibleTo)
		modules[name] = def
	}
	external := make(map[string]ExternalSystemDef, len(in.ExternalSystems))
	for name, def := range in.ExternalSystems {
		def.Targets = slices.Clone(def.Targets)
		external[name] = def
	}
	return TopologyView{Modules: modules, Layers: slices.Clone(in.Layers), ModuleMap: BuildModuleMap(modules), ExternalSystems: external}
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

// ModelHash is the canonical fingerprint of the module model: names, path and
// surface globs, and the classification metadata that decides distance and
// volatility.
//
// It exists so a comparison can refuse rather than mislead. Seam identity is
// derived from module NAMES, so a rename produces a different seam; without a
// model fingerprint that rename would read as one resolved seam plus one new
// seam, and a new-seam gate would block on a no-op refactor. A changed glob has
// the same effect on which edges cross which boundary.
//
// Determinism is the whole contract: every map is walked in sorted key order
// and every slice is sorted before hashing, so two runs over the same model
// produce the same hash regardless of iteration order.
func ModelHash(modules map[string]ModuleDef) string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		def := modules[name]
		_, _ = io.WriteString(h, name+"\x00")
		for _, globs := range [][]string{def.Paths, def.Public, def.Internal} {
			sorted := append([]string(nil), globs...)
			sort.Strings(sorted)
			_, _ = io.WriteString(h, strings.Join(sorted, "\x01")+"\x00")
		}
		for _, field := range []string{def.Layer, def.Subdomain, def.Volatility, def.Owner, def.DeployUnit, string(def.Role)} {
			_, _ = io.WriteString(h, field+"\x00")
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
