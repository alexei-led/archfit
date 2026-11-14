package classify

import (
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Config is the classifier input: the static relationship policy projected onto
// this run, plus the clone evidence acquired for this tree.
type Config struct {
	Modules               map[string]policy.ModuleDef
	Layers                []string
	ModuleMap             policy.ModuleMap
	BCAdvisoryMinSeverity string // minimum severity to emit BC coupling advisories
	// CrossModuleClonePairs is the set of canonical module-pair keys
	// ("[a]\x00[b]" with a≤b) that share duplicated code blocks, derived
	// from the clone-detection signal. Consumed by the Symmetric-strength
	// upgrade, the volatility-cascade clone exclusion, and duplicated-knowledge
	// pairing. Empty when clone detection is disabled or produced no results.
	CrossModuleClonePairs map[string]struct{}
	// CloneEvidence maps each canonical module-pair key (same keying as
	// CrossModuleClonePairs) to the real duplicated-code locations — both sides,
	// as reported by the clone detector — that produced the pairing. classify
	// attaches these onto a Classification when it performs the Symmetric-strength
	// upgrade for that pair, so the downstream finding can cite the actual
	// duplicated file:line instead of only the edge's baseline provenance (e.g. a
	// Rust crate's Cargo.toml:0). Empty when clone detection produced no
	// line-location data, or is disabled.
	CloneEvidence map[string][]graph.Location
	// VolatilityCascadeEnabled enables inferred volatility propagation: a module
	// strongly coupled (strength ≥ functional) to a high-effective-volatility
	// module inherits high effective volatility. The cascade runs to a deterministic
	// fixpoint and only raises volatility; it never lowers configured values.
	VolatilityCascadeEnabled bool
	// ExternalSystems are the declared external integration seams
	// (`external_systems:`). An edge whose target resolves to no module but
	// matches an entry's target glob classifies at DistanceExternal (D=10)
	// with the entry's volatility (default low) and enters scoring.
	ExternalSystems map[string]policy.ExternalSystemDef
	// DuplicatedKnowledgePolicy controls whether clone-only duplicated knowledge
	// is score-bearing or advisory-only.
	DuplicatedKnowledgePolicy policy.DuplicatedKnowledgePolicy
}

// ConfigFrom projects the static relationship policy into a classifier input.
// Per-run clone fields stay zero; the relationship stage fills them once
// evidence exists.
func ConfigFrom(p policy.RelationshipPolicy) Config {
	return Config{
		Modules:                   p.Topology.Modules,
		Layers:                    p.Topology.Layers,
		ModuleMap:                 p.Topology.ModuleMap,
		BCAdvisoryMinSeverity:     p.MinimumSeverity,
		VolatilityCascadeEnabled:  p.VolatilityCascadeEnabled,
		ExternalSystems:           p.Topology.ExternalSystems,
		DuplicatedKnowledgePolicy: p.DuplicatedKnowledge,
	}
}
