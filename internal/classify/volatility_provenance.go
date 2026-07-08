package classify

import (
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
)

// ComputeVolatilityProvenance counts the modules classify ran with by the
// source of their volatility, for the coupling_balance triage disclosure: a
// repo where every edge carries the same volatility must be visibly
// uniform-by-inheritance (one declared ancestor fanned out to N synthetic
// submodules), not mistaken for N measured judgments.
//
// declared is the PRE-augmentation config module map; c.Modules is the
// augmented map the classification ran with (the Augment* functions are
// copy-on-write, so the caller's original map is intact). A module present in
// declared with a base volatility counts as Declared; a synthetic module with
// a base volatility (donated by inheritAncestorAttrs) counts as Inherited;
// everything else is Undeclared. Cascade re-derives Run's own
// computeEffectiveVolatility pass (cheap O(edges), same inputs) and counts the
// modules it raised — an overlay, not a fourth base bucket.
//
// Returns nil when c.Modules is empty: nothing resolved, nothing to disclose.
func ComputeVolatilityProvenance(g *graph.Graph, declared map[string]module.ModuleDef, c config.ClassifyConfig) *diagnostic.VolatilityProvenance {
	if len(c.Modules) == 0 {
		return nil
	}
	vp := &diagnostic.VolatilityProvenance{}
	for name, def := range c.Modules {
		base := volatilityFromDef(def)
		_, isDeclared := declared[name]
		switch {
		case base == coupling.VolatilityUndeclared:
			vp.Undeclared++
		case isDeclared:
			vp.Declared++
		default:
			vp.Inherited++
		}
	}
	if c.VolatilityCascadeEnabled && g != nil {
		mi := buildModuleIndex(c.Modules)
		effective := computeEffectiveVolatility(g, mi, c)
		for name, def := range c.Modules {
			if effective[name] == coupling.VolatilityHigh && volatilityFromDef(def) != coupling.VolatilityHigh {
				vp.Cascade++
			}
		}
	}
	return vp
}
