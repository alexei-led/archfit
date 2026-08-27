// Package classify — external_systems.go resolves config-declared external
// integration seams (`external_systems:`) to the distance ladder's far end.
package classify

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// externalSystemIndex matches a classified edge target against the declared
// external systems (book Ch10 Example 1: a cross-vendor integration seam).
// Names are sorted so overlapping target globs resolve deterministically
// (first sorted name wins).
type externalSystemIndex struct {
	names []string
	defs  map[string]policy.ExternalSystemDef
}

// buildExternalSystemIndex builds a sorted index over the external_systems map.
func buildExternalSystemIndex(defs map[string]policy.ExternalSystemDef) externalSystemIndex {
	names := make([]string, 0, len(defs))
	for n := range defs {
		names = append(names, n)
	}
	sort.Strings(names)
	return externalSystemIndex{names: names, defs: defs}
}

// match reports whether toPath (the classified edge target: a Go import path,
// TS package path/specifier, Python dotted module, or Rust crate name — the
// match is language-independent) belongs to a declared external system, and
// returns that system's volatility (default low when the entry declares none,
// per the book's generic-subdomain guidance).
func (x externalSystemIndex) match(toPath string) (coupling.Volatility, bool) {
	for _, name := range x.names {
		if matchesAnyGlob(toPath, x.defs[name].Targets) {
			return externalVolatility(x.defs[name].Volatility), true
		}
	}
	return coupling.VolatilityUnknown, false
}

// externalVolatility maps a validated external_systems volatility value to its
// coupling level. Empty (unset) defaults to low — an external vendor system is
// a generic capability, presumed stable unless declared otherwise.
func externalVolatility(v string) coupling.Volatility {
	switch strings.ToLower(v) {
	case volatilityHigh:
		return coupling.VolatilityHigh
	case volatilityMedium:
		return coupling.VolatilityMedium
	case volatilityFrozen:
		return coupling.VolatilityFrozen
	default:
		return coupling.VolatilityLow
	}
}
