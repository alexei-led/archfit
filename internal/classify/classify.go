// Package classify assigns Balanced Coupling classifications to graph edges:
// strength, distance, volatility, and explicitness.
package classify

import (
	"maps"
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
//   - Score: continuous EdgeScore from the configured Scorer (default: MultiplicativeScorer, locked Task 16).
//     Applied to cross-boundary edges only (same-module and unknown-distance are zero).
func Run(g *graph.Graph, c config.ClassifyConfig) coupling.Index {
	mm := buildModuleIndex(c.Modules)
	idx := make(coupling.Index)
	scorer := c.Scorer
	if scorer == nil {
		scorer = coupling.DefaultScorer()
	}

	for _, e := range g.Edges() {
		cl := classify(e, mm, c)
		// Attach a continuous score for cross-boundary edges that have a severity.
		// Same-module and unknown-distance edges are not scored (zero EdgeScore).
		if cl.Distance != coupling.DistanceSameModule && cl.Distance != coupling.DistanceUnknown {
			cl.Score = scorer.Score(cl)
		}
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

// AugmentModulesFromGraph returns a Modules map extended with a synthetic module
// for every first-party module-graph node not already covered by a configured
// module's path globs. Rust intra-crate module nodes ("<crate>::<mod>", produced by
// the cargo-modules extractor) are otherwise unknown to moduleFor, so their edges
// classify as distance-unknown and never count toward coupling_balance or
// encapsulation. The gate is the "::" separator, which only the Rust module-graph
// convention uses (Go/TS use "/", Python "."), so Go/TS/Python graphs and their
// configured modules are untouched. Existing config modules keep precedence — a
// synthetic module is only added when nothing already covers the node. The input
// map is not mutated; a copy is returned only if something is added.
func AugmentModulesFromGraph(g *graph.Graph, modules map[string]config.ModuleDef) map[string]config.ModuleDef {
	if g == nil {
		return modules
	}
	mi := buildModuleIndex(modules)
	out := modules
	cloned := false
	for _, n := range g.Nodes() {
		path := n.Path
		if n.Kind == graph.NodeKindExternal || !strings.Contains(path, "::") {
			continue // external dep, or not a Rust module-graph node
		}
		if _, ok := mi.moduleFor(path); ok {
			continue // already covered by a configured module
		}
		if !cloned {
			out = make(map[string]config.ModuleDef, len(modules)+8)
			maps.Copy(out, modules)
			cloned = true
		}
		if _, exists := out[path]; !exists {
			out[path] = config.ModuleDef{Paths: []string{path}}
		}
	}
	return out
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
func classify(e graph.Edge, mi moduleIndex, c config.ClassifyConfig) coupling.Classification {
	modules := c.Modules
	fromPath := pathFromID(e.From)
	toPath := pathFromID(e.To)

	// --- Strength ---
	// Precedence: config public/internal globs are authoritative; an APPROVED
	// pinned label for the edge's module pair refines what the globs leave
	// undecided; the extractor-supplied language-aware hint (e.g. the blanket
	// "functional" for SCIP call edges) is the last resort before unknown.
	str := classifyStrength(toPath, mi)
	if str == coupling.StrengthUnknown && len(c.ApprovedLabels) > 0 {
		if fromMod, okF := mi.moduleFor(fromPath); okF {
			if toMod, okT := mi.moduleFor(toPath); okT {
				if pinned, ok := c.ApprovedLabels[fromMod+"\x00"+toMod]; ok {
					str = coupling.Strength(pinned)
				}
			}
		}
	}
	if str == coupling.StrengthUnknown {
		str = strengthFromHint(e.StrengthHint)
	}

	// --- Distance ---
	dist := classifyDistance(fromPath, toPath, e.Language, mi, modules, c.ExplicitOwners)
	// Role-aware downgrade: a composition root (or a generated/test module) reaches
	// into the modules it wires by design — that fan-out is cohesion, not high-
	// distance coupling — so its outbound edges must never be scored as unbalanced.
	// Cap the source's outbound distance below the high-distance threshold; this
	// single point flows to Severity (BalanceResult below), the continuous Score,
	// and every distance-reading metric (unbalanced_edge, encapsulation, …).
	if fromMod, ok := mi.moduleFor(fromPath); ok && cohesiveRole(modules[fromMod].Role) {
		dist = capDistanceForRole(dist)
	}

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

	// --- Contract-recommended advisory ---
	// When a generic-subdomain target is reached via non-contract strength (model,
	// functional, intrusive, or unknown), BC's anti-corruption-layer guidance applies:
	// introduce a contract (interface/adapter) so the caller is decoupled from the
	// provider's implementation volatility. This flag is carried on the Classification
	// so the engine can emit a dedicated advisory finding.
	contractRecommended := str != coupling.StrengthContract &&
		dist != coupling.DistanceSameModule &&
		isGenericSubdomain(toPath, mi, modules)

	cl := coupling.Classification{
		Strength:            str,
		Distance:            dist,
		Volatility:          vol,
		Explicitness:        exp,
		ContractRecommended: contractRecommended,
		AsyncBridge:         e.AsyncBridge,
	}

	// --- Severity + Connascence ---
	// Both are only meaningful for cross-boundary edges.
	if dist != coupling.DistanceSameModule && dist != coupling.DistanceUnknown {
		cl.Severity = coupling.BalanceResult(cl)
		// Connascence is report-only descriptive vocabulary — never scored, never gates.
		if fromMod, okF := mi.moduleFor(fromPath); okF {
			if toMod, okT := mi.moduleFor(toPath); okT {
				cl.Connascence = classifyConnascence(e, str, fromMod, toMod, c)
			}
		}
	}

	return cl
}

// classifyConnascence derives the connascence degree for a cross-module edge.
// CoA takes precedence: a clone pair crossing a module boundary is a stronger
// signal than type-level coupling. CoT is assigned when the edge carries a
// SCIP-sourced model or contract strength hint (struct/interface/field use).
// Report-only — never fed into the scorer or gate.
func classifyConnascence(e graph.Edge, str coupling.Strength, fromMod, toMod string, c config.ClassifyConfig) coupling.Connascence {
	// CoA: clone pair crossing this module boundary.
	if len(c.CrossModuleClonePairs) > 0 {
		if _, ok := c.CrossModuleClonePairs[connascencePairKey(fromMod, toMod)]; ok {
			return coupling.ConnascenceAlgorithm
		}
	}
	// CoT: cross-module struct/interface/field use — signalled by a SCIP hint
	// resolving to model or contract strength, or a direct model/contract label.
	if e.StrengthHint == string(coupling.StrengthModel) ||
		e.StrengthHint == string(coupling.StrengthContract) ||
		str == coupling.StrengthModel ||
		str == coupling.StrengthContract {
		return coupling.ConnascenceType
	}
	return coupling.ConnascenceNone
}

// connascencePairKey returns the canonical sorted key for a module pair.
func connascencePairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
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

// classifyDistance computes the composite distance for a cross-module edge using
// a precedence chain rather than a flat max, so that explicit config is never
// overridden by the structural fallback:
//
//  1. A differing deploy unit is an absolute boundary → cross_deploy_unit.
//  2. Hand-authored ownership on EITHER endpoint is authoritative → ownership
//     decides (vs deploy). The user told us something; don't drop it to the
//     code-structure default. (One-sided is fine: ownershipDistance compares the
//     explicit owner against the other endpoint's owner as usual.)
//  3. A real resolver ownership signal (≥2 distinct owners, not the single
//     git-author degenerate case) is authoritative too → ownership decides.
//  4. Otherwise (degenerate or no ownership) fall back to code structure.
//
// This precedence is why a flat-named explicit `owner: same-team` config no
// longer collapses to the code-structure default: explicit ownership is handled
// in step 2, before codeStructureDistance (which treats two flat single-segment
// names as different subtrees) can apply.
//
// runtime_adjust (+1 level for async bridges) is a Phase-4 addition (Task 12).
func classifyDistance(fromPath, toPath, lang string, mi moduleIndex, modules map[string]config.ModuleDef, explicitOwners map[string]bool) coupling.Distance {
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

	deploy := deployDistance(fromDef.DeployUnit, toDef.DeployUnit)
	if deploy == coupling.DistanceCrossDeployUnit {
		return deploy // a deploy boundary is absolute
	}

	// Step 2: explicit hand-authored ownership on either endpoint is authoritative.
	if explicitOwners[fromMod] || explicitOwners[toMod] {
		return maxDistance(ownershipDistance(fromDef.Owner, toDef.Owner), deploy)
	}

	// Step 3: a real resolver ownership signal (≥2 distinct owners) is authoritative.
	// The module→owner map is only built here (not on every edge) — explicit-owner
	// edges short-circuit above.
	owners := make(map[string]string, len(modules))
	for name, def := range modules {
		owners[name] = def.Owner
	}
	if !isDegenerateOwnerMap(owners) {
		return maxDistance(ownershipDistance(fromDef.Owner, toDef.Owner), deploy)
	}

	// Step 4: git-author degenerate or no ownership → code structure.
	return maxDistance(codeStructureDistance(fromMod, toMod, lang), deploy)
}

// classifyVolatility derives domain volatility for the to-module using three
// sources in priority order, per Khononov's volatility-from-subdomain mapping
// (core→high, supporting→medium, generic→low) with an explicit per-module
// override:
//
//  1. Explicit `volatility` field on the module definition (hand-authored override).
//  2. Subdomain heuristic: core→high, supporting→medium, generic→low.
//  3. Path-pattern heuristic (domainVolatilityFromPath) — deterministic,
//     never guesses core/high.
//
// Resolution outcomes are deliberately three-valued:
//   - to-module unresolved → VolatilityUnknown (genuinely indeterminate).
//   - to-module resolved but no volatility/subdomain/path match → VolatilityUndeclared
//     (a config gap the user can close; the scorer advises "declare", not "lower").
//   - otherwise → high/medium/low.
//
// No churn or git history is consulted here. Implementation volatility (git
// churn) feeds only report-only metrics (change_amplification, hidden_coupling).
func classifyVolatility(toPath string, mi moduleIndex, modules map[string]config.ModuleDef) coupling.Volatility {
	toMod, ok := mi.moduleFor(toPath)
	if !ok {
		return coupling.VolatilityUnknown
	}
	def := modules[toMod]

	// Priority 1: explicit Volatility field.
	switch strings.ToLower(def.Volatility) {
	case "high":
		return coupling.VolatilityHigh
	case "medium":
		return coupling.VolatilityMedium
	case "low":
		return coupling.VolatilityLow
	}

	// Priority 2: subdomain heuristic.
	switch strings.ToLower(def.Subdomain) {
	case "core":
		return coupling.VolatilityHigh
	case "supporting":
		return coupling.VolatilityMedium
	case "generic":
		return coupling.VolatilityLow
	}

	// Priority 3: path-pattern heuristic (never guesses core/high).
	if v := domainVolatilityFromPath(toPath); v != coupling.VolatilityUnknown {
		return v
	}

	// Module is known, but nothing declares its volatility — a closable config gap.
	return coupling.VolatilityUndeclared
}

// isGenericSubdomain reports whether the to-module is classified as a generic
// subdomain. Returns true when the module definition's Subdomain is "generic",
// or when the subdomain is absent but the path-pattern heuristic resolves to
// VolatilityLow (generic-indicator paths such as vendor/, lib/, util/).
//
// This is used to trigger the contract-recommended advisory when a generic
// target is reached via non-contract strength (BC's anti-corruption-layer guidance).
func isGenericSubdomain(toPath string, mi moduleIndex, modules map[string]config.ModuleDef) bool {
	toMod, ok := mi.moduleFor(toPath)
	if !ok {
		return false
	}
	def := modules[toMod]
	if strings.ToLower(def.Subdomain) == "generic" {
		return true
	}
	// Treat heuristic-generic paths the same way when no explicit subdomain is set.
	if def.Subdomain == "" && domainVolatilityFromPath(toPath) == coupling.VolatilityLow {
		return true
	}
	return false
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
