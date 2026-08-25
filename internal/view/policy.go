package view

import (
	"maps"
	"slices"

	"github.com/alexei-led/archfit/internal/model/module"
)

// PolicyConfig is the policy-owned view of the static architecture policy that
// the classify stage consumes: module topology, declared ownership, and the
// gate knobs authored in .archfit.yaml. It deliberately excludes the runtime
// enrich signals (approved/LLM labels, clone pairs, clone evidence) that are
// computed per-run and therefore stay on ClassifyConfig.
//
// ClassifyConfig remains the migration-compatible aggregate view: it still
// carries every field classify.Run consumes today, so existing callers keep
// compiling unchanged. New consumers that need only the static policy should
// accept PolicyConfig and project a ClassifyConfig to it via StaticPolicy.
type PolicyConfig struct {
	// Modules maps module name to its declared topology/ownership metadata.
	Modules map[string]module.ModuleDef
	// Layers orders the architecture layers innermost-to-outermost.
	Layers []string
	// ModuleMap is the pre-built path→module resolver for Modules.
	ModuleMap module.Map
	// BCAdvisoryMinSeverity is the minimum severity for BC coupling advisories.
	BCAdvisoryMinSeverity string
	// ExplicitOwners marks modules whose owner was hand-authored in YAML.
	ExplicitOwners map[string]bool
	// VolatilityCascadeEnabled enables the inferred-volatility propagation pass.
	VolatilityCascadeEnabled bool
	// ExternalSystems are the declared external integration seams.
	ExternalSystems map[string]ExternalSystemDef
	// DuplicatedKnowledgePolicy controls whether clone-only duplicated knowledge
	// is score-bearing or advisory-only.
	DuplicatedKnowledgePolicy DuplicatedKnowledgePolicy
}

// StaticPolicy projects only the static topology/ownership/gate fields of a
// ClassifyConfig into the policy-owned PolicyConfig view. Runtime enrich
// signals (labels, clone pairs/evidence) are intentionally not projected:
// those fields stay on ClassifyConfig until consumers migrate to a separate
// runtime-enrich contract.
func (c ClassifyConfig) StaticPolicy() PolicyConfig {
	modules := make(map[string]module.ModuleDef, len(c.Modules))
	for name, def := range c.Modules {
		def.Paths = slices.Clone(def.Paths)
		def.Public = slices.Clone(def.Public)
		def.Internal = slices.Clone(def.Internal)
		modules[name] = def
	}
	externalSystems := make(map[string]ExternalSystemDef, len(c.ExternalSystems))
	for name, def := range c.ExternalSystems {
		def.Targets = slices.Clone(def.Targets)
		externalSystems[name] = def
	}
	return PolicyConfig{
		Modules:                   modules,
		Layers:                    slices.Clone(c.Layers),
		ModuleMap:                 module.BuildMap(modules),
		BCAdvisoryMinSeverity:     c.BCAdvisoryMinSeverity,
		ExplicitOwners:            maps.Clone(c.ExplicitOwners),
		VolatilityCascadeEnabled:  c.VolatilityCascadeEnabled,
		ExternalSystems:           externalSystems,
		DuplicatedKnowledgePolicy: c.DuplicatedKnowledgePolicy,
	}
}
