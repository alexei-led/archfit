package classify

import (
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Config is the classifier input: the static relationship policy projected onto
// this run.
type Config struct {
	Modules               map[string]policy.ModuleDef
	Layers                []string
	ModuleMap             policy.ModuleMap
	BCAdvisoryMinSeverity string // minimum severity to emit BC coupling advisories
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
}

// ConfigFrom projects the static relationship policy into a classifier input.
func ConfigFrom(p policy.RelationshipPolicy) Config {
	return Config{
		Modules:                  p.Topology.Modules,
		Layers:                   p.Topology.Layers,
		ModuleMap:                p.Topology.ModuleMap,
		BCAdvisoryMinSeverity:    p.MinimumSeverity,
		VolatilityCascadeEnabled: p.VolatilityCascadeEnabled,
		ExternalSystems:          p.Topology.ExternalSystems,
	}
}
