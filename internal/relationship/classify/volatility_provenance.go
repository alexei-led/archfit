package classify

import (
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// VolatilitySource names where one module's base volatility came from. Cascade
// is deliberately absent: it is an overlay on a base source, not a fourth base
// bucket, so it rides ModuleVolatility.Cascade instead.
type VolatilitySource string

// Base volatility sources.
const (
	VolatilitySourceDeclared   VolatilitySource = "declared"
	VolatilitySourceInherited  VolatilitySource = "inherited"
	VolatilitySourceUndeclared VolatilitySource = "undeclared"
	// VolatilitySourceCascade is the reported source of a module the cascade
	// raised. It is a display value produced from ModuleVolatility, never a
	// base bucket.
	VolatilitySourceCascade VolatilitySource = "cascade"
)

// ModuleVolatility is one module's volatility provenance: which base source
// declared it, and whether the inferred-volatility cascade then raised it.
type ModuleVolatility struct {
	Source  VolatilitySource
	Cascade bool
}

// Reported is the single provenance label to show for this module. The cascade
// overlay wins the display because it is the fact a reader would otherwise
// mistake for a measured judgment.
func (m ModuleVolatility) Reported() VolatilitySource {
	if m.Cascade {
		return VolatilitySourceCascade
	}
	return m.Source
}

// VolatilityProvenanceByModule resolves every classified module's volatility
// provenance.
//
// declared is the PRE-augmentation config module map; c.Modules is the
// augmented map the classification ran with (the Augment* functions are
// copy-on-write, so the caller's original map is intact). A module present in
// declared with a base volatility is Declared; a synthetic module with a base
// volatility (donated by inheritAncestorAttrs) is Inherited; everything else is
// Undeclared. Cascade re-derives Run's own computeEffectiveVolatility pass
// (cheap O(edges), same inputs) and marks the modules it raised.
//
// Returns nil when c.Modules is empty: nothing resolved, nothing to disclose.
func VolatilityProvenanceByModule(g *graph.Graph, declared map[string]policy.ModuleDef, c Config) map[string]ModuleVolatility {
	if len(c.Modules) == 0 {
		return nil
	}
	out := make(map[string]ModuleVolatility, len(c.Modules))
	for name, def := range c.Modules {
		base := volatilityFromDef(def)
		_, isDeclared := declared[name]
		switch {
		case base == coupling.VolatilityUndeclared:
			out[name] = ModuleVolatility{Source: VolatilitySourceUndeclared}
		case isDeclared:
			out[name] = ModuleVolatility{Source: VolatilitySourceDeclared}
		default:
			out[name] = ModuleVolatility{Source: VolatilitySourceInherited}
		}
	}
	if c.VolatilityCascadeEnabled && g != nil {
		mi := buildModuleIndex(c.Modules)
		effective := computeEffectiveVolatility(g, mi, c)
		for name, def := range c.Modules {
			if effective[name] == coupling.VolatilityHigh && volatilityFromDef(def) != coupling.VolatilityHigh {
				entry := out[name]
				entry.Cascade = true
				out[name] = entry
			}
		}
	}
	return out
}

// ComputeVolatilityProvenance counts the modules classify ran with by the
// source of their volatility, for the coupling_balance triage disclosure: a
// repo where every edge carries the same volatility must be visibly
// uniform-by-inheritance (one declared ancestor fanned out to N synthetic
// submodules), not mistaken for N measured judgments.
//
// Declared/Inherited/Undeclared partition the modules; Cascade is an overlay
// count on top of them, so the four numbers deliberately do not sum to the
// module count.
//
// Returns nil when no module resolved: nothing to disclose.
func ComputeVolatilityProvenance(g *graph.Graph, declared map[string]policy.ModuleDef, c Config) *relationship.VolatilityProvenance {
	byModule := VolatilityProvenanceByModule(g, declared, c)
	if byModule == nil {
		return nil
	}
	vp := &relationship.VolatilityProvenance{}
	for _, m := range byModule {
		switch m.Source {
		case VolatilitySourceDeclared:
			vp.Declared++
		case VolatilitySourceInherited:
			vp.Inherited++
		default:
			vp.Undeclared++
		}
		if m.Cascade {
			vp.Cascade++
		}
	}
	return vp
}
