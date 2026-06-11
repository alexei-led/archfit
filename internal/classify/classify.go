// Package classify assigns Balanced Coupling classifications to graph edges:
// strength, distance, volatility, and explicitness.
package classify

import (
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Run classifies every edge in g and returns a coupling.Index keyed by the
// edge canonical key (from + "\x00" + to + "\x00" + kind).
//
// For each edge:
//   - Strength: contract if the target path matches a public glob of any module;
//     intrusive if it matches an internal glob; unknown otherwise.
//   - Distance: derived from module ownership (same_module /
//     cross_module_same_owner / cross_module_different_owner / cross_deploy_unit).
//     When either endpoint cannot be resolved to a module, distance is unknown.
//   - Volatility: derived from the to-module's subdomain field
//     (core→high, supporting→medium, generic→low, ""/"unknown"→unknown).
//   - Explicitness: explicit when strength=contract; implicit when strength=intrusive;
//     unknown otherwise.
func Run(g *graph.Graph, c config.ClassifyConfig) coupling.Index {
	mm := buildModuleIndex(c.Modules)
	idx := make(coupling.Index)

	for _, e := range g.Edges() {
		cl := classify(e, mm, c.Modules)
		idx[edgeKey(e)] = cl
	}

	return idx
}

// edgeKey returns the canonical coupling.Index key for an edge.
// Matches the format documented in coupling.Index: from + "\x00" + to + "\x00" + kind.
func edgeKey(e graph.Edge) string {
	return e.From + "\x00" + e.To + "\x00" + string(e.Kind)
}

// pathFromID extracts the path component from a node ID of the form "kind:path".
// If there is no ":", the entire ID is returned.
func pathFromID(id string) string {
	_, after, ok := strings.Cut(id, ":")
	if ok {
		return after
	}
	return id
}

// moduleIndex is a sorted list of module names for deterministic glob matching.
type moduleIndex struct {
	names   []string
	modules map[string]config.ModuleDef
}

// buildModuleIndex builds a sorted module name index from the Modules map.
func buildModuleIndex(modules map[string]config.ModuleDef) moduleIndex {
	names := make([]string, 0, len(modules))
	for n := range modules {
		names = append(names, n)
	}
	sort.Strings(names)
	return moduleIndex{names: names, modules: modules}
}

// moduleFor returns the first module name whose Paths globs match path.
// Returns ("", false) if no module matches.
func (mi moduleIndex) moduleFor(path string) (string, bool) {
	for _, name := range mi.names {
		for _, pattern := range mi.modules[name].Paths {
			if matched, _ := doublestar.Match(pattern, path); matched {
				return name, true
			}
		}
	}
	return "", false
}

// matchesAnyGlob reports whether path matches any of the given glob patterns.
func matchesAnyGlob(path string, globs []string) bool {
	for _, pattern := range globs {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

// classify computes a Classification for a single edge.
//
// ExplicitnessHint on the edge overrides the config-glob-derived explicitness
// when non-empty ("explicit" or "implicit"). BalanceResult is then called to
// derive advisory Severity for cross-boundary edges.
func classify(e graph.Edge, mi moduleIndex, modules map[string]config.ModuleDef) coupling.Classification {
	fromPath := pathFromID(e.From)
	toPath := pathFromID(e.To)

	// --- Strength ---
	// Config public/internal globs are authoritative. When they do not decide,
	// fall back to an extractor-supplied language-aware hint (e.g. Python
	// underscore-private targets → intrusive) before giving up to unknown.
	str := classifyStrength(toPath, mi)
	if str == coupling.StrengthUnknown {
		str = strengthFromHint(e.StrengthHint)
	}

	// --- Distance ---
	dist := classifyDistance(fromPath, toPath, mi, modules)

	// --- Volatility ---
	vol := classifyVolatility(toPath, mi, modules)

	// --- Explicitness ---
	// ExplicitnessHint from the extractor (AST signal) takes precedence over the
	// config-glob heuristic when it is set.
	exp := classifyExplicitness(str)
	switch e.ExplicitnessHint {
	case "explicit":
		exp = coupling.ExplicitnessExplicit
	case "implicit":
		exp = coupling.ExplicitnessImplicit
	}

	cl := coupling.Classification{
		Strength:     str,
		Distance:     dist,
		Volatility:   vol,
		Explicitness: exp,
	}

	// --- Severity ---
	// Only meaningful for cross-boundary edges; same-module edges are balanced by definition.
	if dist != coupling.DistanceSameModule && dist != coupling.DistanceUnknown {
		cl.Severity = coupling.BalanceResult(cl)
	}

	return cl
}

// classifyStrength determines strength from glob matching against all modules'
// public and internal glob lists.
//
// The two-pass structure (ALL public globs before ANY internal glob) makes the
// result independent of module order; iteration still goes through the sorted
// index so the code is self-evidently deterministic.
func classifyStrength(toPath string, mi moduleIndex) coupling.Strength {
	for _, name := range mi.names {
		if matchesAnyGlob(toPath, mi.modules[name].Public) {
			return coupling.StrengthContract
		}
	}
	for _, name := range mi.names {
		if matchesAnyGlob(toPath, mi.modules[name].Internal) {
			return coupling.StrengthIntrusive
		}
	}
	return coupling.StrengthUnknown
}

// strengthFromHint maps an extractor's strength hint to a coupling.Strength.
//
// Hints come from a trusted symbol-level source: either a SCIP index (which
// resolves the imported symbol — Protocol/ABC → contract, concrete class → model,
// function/method → functional, private → intrusive) or the Python underscore
// heuristic (intrusive only). Both are evidence, not guesses, so all four valid
// strengths are accepted; an unrecognized hint stays unknown. Config public/internal
// globs still take precedence (see classify): the hint is a fallback only.
func strengthFromHint(hint string) coupling.Strength {
	switch coupling.Strength(hint) {
	case coupling.StrengthContract, coupling.StrengthModel,
		coupling.StrengthFunctional, coupling.StrengthIntrusive:
		return coupling.Strength(hint)
	default:
		return coupling.StrengthUnknown
	}
}

// classifyDistance determines how far apart from and to modules are in the
// ownership hierarchy.
func classifyDistance(fromPath, toPath string, mi moduleIndex, modules map[string]config.ModuleDef) coupling.Distance {
	fromMod, fromOK := mi.moduleFor(fromPath)
	toMod, toOK := mi.moduleFor(toPath)

	if !fromOK || !toOK {
		return coupling.DistanceUnknown
	}

	if fromMod == toMod {
		return coupling.DistanceSameModule
	}

	fromDef := modules[fromMod]
	toDef := modules[toMod]

	if fromDef.Owner == toDef.Owner && fromDef.Owner != "" {
		return coupling.DistanceCrossModuleSameOwner
	}

	if fromDef.DeployUnit != toDef.DeployUnit && fromDef.DeployUnit != "" && toDef.DeployUnit != "" {
		return coupling.DistanceCrossDeployUnit
	}

	return coupling.DistanceCrossModuleDiffOwner
}

// classifyVolatility derives volatility from the to-module's subdomain.
func classifyVolatility(toPath string, mi moduleIndex, modules map[string]config.ModuleDef) coupling.Volatility {
	toMod, ok := mi.moduleFor(toPath)
	if !ok {
		return coupling.VolatilityUnknown
	}
	def := modules[toMod]

	// Use explicit Volatility field if set.
	switch strings.ToLower(def.Volatility) {
	case "high":
		return coupling.VolatilityHigh
	case "medium":
		return coupling.VolatilityMedium
	case "low":
		return coupling.VolatilityLow
	}

	// Fall back to subdomain heuristic.
	switch strings.ToLower(def.Subdomain) {
	case "core":
		return coupling.VolatilityHigh
	case "supporting":
		return coupling.VolatilityMedium
	case "generic":
		return coupling.VolatilityLow
	default:
		return coupling.VolatilityUnknown
	}
}

// classifyExplicitness derives explicitness from strength.
func classifyExplicitness(str coupling.Strength) coupling.Explicitness {
	switch str {
	case coupling.StrengthContract:
		return coupling.ExplicitnessExplicit
	case coupling.StrengthIntrusive:
		return coupling.ExplicitnessImplicit
	default:
		return coupling.ExplicitnessUnknown
	}
}
