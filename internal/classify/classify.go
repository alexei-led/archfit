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

// subdomain constants are the accepted Khononov subdomain values used throughout
// the classify package to derive volatility and detect generic targets.
const (
	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"
)

// volatility level literals accepted in config (`volatility:` on modules and
// external_systems entries; "legacy" is a module-only alias for frozen).
const (
	volatilityHigh   = "high"
	volatilityMedium = "medium"
	volatilityLow    = "low"
	volatilityFrozen = "frozen"
	volatilityLegacy = "legacy"
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
//     (core→high, supporting→low, generic→low, ""/"unknown"→unknown).
//   - Explicitness: explicit when strength=contract; implicit when strength=intrusive;
//     unknown otherwise.
//   - Score: continuous EdgeScore from the configured Scorer (default: BookScorer).
//     Applied to every known-distance edge; unknown-distance edges are zero.
//     Same-module edges are scored (local_coupling report block) but keep
//     SeverityNone — the advisory pipeline stays cross-boundary.
func Run(g *graph.Graph, c config.ClassifyConfig) coupling.Index {
	mm := buildModuleIndex(c.Modules)
	idx := make(coupling.Index)
	scorer := c.Scorer
	if scorer == nil {
		scorer = coupling.DefaultScorer()
	}

	// Pre-compute per-run invariants so classifyDistance does not rebuild maps on
	// every edge. Both results depend only on config (loaded once before Run).
	degenerateExplicit, degenerateOwners := ownerDegeneracy(c)

	effectiveVol := computeEffectiveVolatility(g, mm, c)
	extSystems := buildExternalSystemIndex(c.ExternalSystems)

	for _, e := range g.Edges() {
		cl := classify(e, mm, c, degenerateExplicit, degenerateOwners, effectiveVol, extSystems)
		// Score every edge whose distance is known. Same-module edges score at
		// the book's same-module rung (D=2) and surface the Ch10 local-complexity
		// quadrant in the local_coupling report block; unknown-distance edges are
		// not scored (zero EdgeScore). Abstain rules are identical at both levels.
		// Severity is derived from cl.Score.Band so the book formula and the
		// advisory severity are always identical — the single source of truth.
		if cl.Distance != coupling.DistanceUnknown {
			cl.Score = scorer.Score(cl)
			// Set Severity from the book score band for cross-boundary edges only.
			// Same-module edges keep SeverityNone: the bc/imbalanced_coupling
			// advisory pipeline and coupling_balance stay cross-module (fractal
			// level separation); local complexity is report-only. Abstained edges
			// (Scored=false, Band="") remain SeverityNone — abstain-not-fake.
			if cl.Distance != coupling.DistanceSameModule {
				cl.Severity = cl.Score.Band
			}
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
// Path→module resolution delegates to the shared config.ModuleMap so there is a
// single most-specific-match implementation; names/modules remain for the
// public/internal glob strength scan in classifyStrength.
type moduleIndex struct {
	names   []string
	modules map[string]config.ModuleDef
	mm      config.ModuleMap
}

// buildModuleIndex builds a sorted module name index from the Modules map.
func buildModuleIndex(modules map[string]config.ModuleDef) moduleIndex {
	names := make([]string, 0, len(modules))
	for n := range modules {
		names = append(names, n)
	}
	sort.Strings(names)
	return moduleIndex{names: names, modules: modules, mm: config.BuildModuleMap(modules)}
}

// moduleFor returns the most-specific module name whose Paths globs match path,
// delegating to config.ModuleMap (single source of truth for resolution).
// Returns ("", false) if no module matches.
func (mi moduleIndex) moduleFor(path string) (string, bool) {
	return mi.mm.ModuleFor(path)
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
			out[path] = inheritAncestorAttrs(ancestorByKey(path, modules), []string{path})
		}
	}
	return out
}

// AugmentGoWorkspaceModules returns a Modules map extended with a synthetic module
// for each Go workspace member whose directory is not already covered by a configured
// module's path globs. Called when ≥2 workspace members were loaded so cross-member
// edges can classify with a real Distance for coupling_balance.
//
// The gate is len(GoModules) >= 2 — single-module repos and archfit's own self-scan
// (which collapses to 1 surviving member after exclusion) are byte-identical to
// before, mirroring the Rust "::" gate in AugmentModulesFromGraph.
//
// Members at the repo root (RelDir == ".") are skipped: a root-relative glob would
// over-match sibling members (abstain-not-fake). Existing config modules keep
// precedence — a synthetic module is only added when nothing already covers the
// member's directory. The input map is not mutated; a copy is returned only if
// something is added.
func AugmentGoWorkspaceModules(g *graph.Graph, modules map[string]config.ModuleDef) map[string]config.ModuleDef {
	if g == nil {
		return modules
	}
	goMods := g.GoModules()
	if len(goMods) < 2 {
		return modules // ≥2 gate: single-module repos and self-scan are untouched
	}
	mi := buildModuleIndex(modules)
	out := modules
	cloned := false
	for _, m := range goMods {
		if m.RelDir == "." {
			continue // root member: no precise glob boundary; abstain-not-fake
		}
		// "already covered" check: does any configured module glob match a
		// representative path inside this member's directory?
		if _, ok := mi.moduleFor(m.RelDir + "/x"); ok {
			continue
		}
		if !cloned {
			out = make(map[string]config.ModuleDef, len(modules)+len(goMods))
			maps.Copy(out, modules)
			cloned = true
		}
		if _, exists := out[m.Path]; !exists {
			// Inherit attributes from the nearest config-declared ancestor module.
			// For Go workspace members the "already covered" check above uses
			// mi.moduleFor(m.RelDir+"/x") — if there were a covering ancestor
			// we'd have skipped this member. Instead we do a direct prefix scan
			// on the member's RelDir so partial ancestors (e.g. a module whose
			// glob covers a parent dir) can still donate their attributes.
			out[m.Path] = inheritAncestorAttrs(ancestorByPath(m.RelDir, modules), []string{m.RelDir + "/**"})
		}
	}
	return out
}

// AugmentCargoCrateNodes binds crate-level Rust nodes (a `package:<crate>` node
// whose path is the bare crate name, no "::") to a declared module when the config
// used a directory-path glob (e.g. "crates/ruff_python_ast/**") that matches the
// crate's source dir but NOT the bare crate-name node the cargo-metadata extractor
// emits. Without this, such edges classify distance-unknown → external → 0 scored,
// and coupling_balance reports n/a even though the architect declared the crates.
//
// For each crate node not already covered by a configured glob: if a configured
// module covers the crate's repo-relative directory (from graph CrateRoots), the
// bare crate name is appended to THAT module's Paths so the node binds while keeping
// the module's owner/subdomain/volatility/layer. If no module covers the dir, a
// synthetic module is registered (bare name, ancestor-inherited owner).
//
// Gate: ≥2 first-party crate nodes — a single-crate repo is a degenerate graph
// (coupling unmeasurable → n/a) and is left untouched. Configs that already use
// bare crate-name globs (tokio, yazi) bind on the first check and are no-ops.
// Intra-crate "<crate>::<mod>" nodes are handled by AugmentModulesFromGraph.
// The input map is not mutated; a copy is returned only if something is added.
func AugmentCargoCrateNodes(g *graph.Graph, modules map[string]config.ModuleDef) map[string]config.ModuleDef {
	if g == nil {
		return modules
	}
	crateNodes := make([]string, 0)
	for _, n := range g.Nodes() {
		if n.Kind != graph.NodeKindPackage || n.Language != graph.LangRust || strings.Contains(n.Path, "::") {
			continue // non-package, non-Rust (e.g. Go), or intra-crate module node (handled elsewhere)
		}
		crateNodes = append(crateNodes, n.Path)
	}
	if len(crateNodes) < 2 {
		return modules // <2 crates: degenerate graph → coupling unmeasurable (n/a)
	}
	dirByName := make(map[string]string, len(crateNodes))
	for _, cr := range g.CrateRoots() {
		dirByName[cr.Name] = cr.Dir
	}
	mi := buildModuleIndex(modules)
	out := modules
	cloned := false
	ensureClone := func() {
		if !cloned {
			out = make(map[string]config.ModuleDef, len(modules)+len(crateNodes))
			maps.Copy(out, modules)
			cloned = true
		}
	}
	for _, name := range crateNodes {
		if _, ok := mi.moduleFor(name); ok {
			continue // already bound by a bare crate-name glob (e.g. tokio, yazi)
		}
		dir := dirByName[name]
		if dir != "" {
			if modName, ok := mi.moduleFor(dir + "/x"); ok {
				// A configured module covers the crate dir but not its bare-name node;
				// bind the node to it, preserving owner/subdomain/volatility/layer.
				ensureClone()
				def := out[modName]
				def.Paths = append(append([]string{}, def.Paths...), name)
				out[modName] = def
				continue
			}
		}
		// No configured module covers the crate: register a synthetic one.
		ensureClone()
		if _, exists := out[name]; !exists {
			out[name] = inheritAncestorAttrs(ancestorByPath(dir, modules), []string{name})
		}
	}
	return out
}

// ancestorByKey finds the nearest config-declared ancestor of a Rust
// module-graph node (key uses "::" separator). It returns the ModuleDef of the
// config module whose key is the longest "::"-prefix of path, or the zero
// ModuleDef if none. Only modules that declare an Owner are considered
// ancestor candidates — an owner-less module carries no inheritable identity.
func ancestorByKey(path string, modules map[string]config.ModuleDef) config.ModuleDef {
	var best config.ModuleDef
	bestLen := 0
	for name, def := range modules {
		if def.Owner == "" {
			continue
		}
		// A module is an ancestor when path starts with name+"::" or equals name.
		prefix := name + "::"
		if path == name || strings.HasPrefix(path, prefix) {
			if len(name) > bestLen {
				bestLen = len(name)
				best = def
			}
		}
	}
	return best
}

// ancestorByPath finds the nearest config-declared ancestor for a Go workspace
// member or Rust crate directory, matching by directory path prefix. It
// returns the ModuleDef of the config module whose glob paths share the
// longest directory prefix with relDir, or the zero ModuleDef if none. This is
// a fallback for the case where no module glob fully covers the child
// (otherwise the caller would have skipped it as already-covered), but a
// parent-directory module may still donate its attributes. Only modules that
// declare an Owner are considered ancestor candidates.
func ancestorByPath(relDir string, modules map[string]config.ModuleDef) config.ModuleDef {
	var best config.ModuleDef
	bestLen := 0
	for _, def := range modules {
		if def.Owner == "" {
			continue
		}
		for _, p := range def.Paths {
			// Strip trailing glob suffixes to get the directory root.
			dir := strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(p, "**"), "/"), "/")
			if dir == "" {
				continue
			}
			if relDir == dir || strings.HasPrefix(relDir, dir+"/") {
				if len(dir) > bestLen {
					bestLen = len(dir)
					best = def
				}
			}
		}
	}
	return best
}

// inheritAncestorAttrs builds a synthetic ModuleDef for a newly registered
// module, carrying paths plus every inheritable attribute — Owner, Volatility,
// Subdomain, Layer, DeployUnit — from the nearest config-declared ancestor.
// The single shared helper for all three Augment* functions, so a synthetic
// module never silently drops Volatility/Subdomain/Layer/DeployUnit the way an
// Owner-only copy would (undeclared Volatility scores the conservative worst
// case, V=10 — the root cause of the tokio finding flood).
func inheritAncestorAttrs(ancestor config.ModuleDef, paths []string) config.ModuleDef {
	return config.ModuleDef{
		Paths:      paths,
		Owner:      ancestor.Owner,
		Volatility: ancestor.Volatility,
		Subdomain:  ancestor.Subdomain,
		Layer:      ancestor.Layer,
		DeployUnit: ancestor.DeployUnit,
	}
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
// when non-empty ("explicit" or "implicit"). Severity is set in Run after the
// book score is computed (cl.Score.Band → cl.Severity).
func classify(e graph.Edge, mi moduleIndex, c config.ClassifyConfig, degenerateExplicit, degenerateOwners bool, effectiveVol map[string]coupling.Volatility, extSystems externalSystemIndex) coupling.Classification {
	modules := c.Modules
	fromPath := pathFromID(e.From)
	toPath := pathFromID(e.To)

	resolved := resolveStrength(e, mi, c)
	str := resolved.strength
	fromPin := resolved.fromPin
	strengthFromLLM := resolved.fromLLM
	strengthFromNonHighLLM := resolved.fromNonHighLLM

	// --- Symmetric upgrade from clone detection ---
	// A cross-module clone pair (a DRY violation) signals bidirectional
	// coupling at implementation level — book ordinal 9 (Symmetric), between
	// Functional (8) and Intrusive (10). Upgrade only when strength is still
	// functional or unknown; config-authoritative (contract/intrusive) and
	// human-approved pinned labels (including functional) are never overridden —
	// fromPin guards against silently overriding a pinned functional label with Symmetric.
	var cloneLocations []graph.Location
	if !fromPin && (str == coupling.StrengthFunctional || str == coupling.StrengthUnknown) {
		if len(c.CrossModuleClonePairs) > 0 {
			if fromMod, okF := mi.moduleFor(fromPath); okF {
				if toMod, okT := mi.moduleFor(toPath); okT {
					pairKey := modulePairKey(fromMod, toMod)
					if _, hasPair := c.CrossModuleClonePairs[pairKey]; hasPair {
						str = coupling.StrengthSymmetric
						// The real duplicated-code locations (both sides), so the
						// finding downstream can cite them instead of only the
						// edge's baseline provenance (e.g. Cargo.toml:0).
						cloneLocations = c.CloneEvidence[pairKey]
					}
				}
			}
		}
	}

	// --- Distance & volatility ---
	dist, distBasis, vol := resolveDistanceVolatility(fromPath, toPath, e.Language, mi, c, degenerateExplicit, degenerateOwners, effectiveVol, extSystems)

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

	return coupling.Classification{
		Strength:               str,
		Distance:               dist,
		Volatility:             vol,
		Explicitness:           exp,
		ContractRecommended:    contractRecommended,
		DistanceBasis:          distBasis,
		CloneLocations:         cloneLocations,
		StrengthFromLLM:        strengthFromLLM,
		StrengthFromNonHighLLM: strengthFromNonHighLLM,
	}
}

// resolveDistanceVolatility computes the composite distance (with the role cap
// and the declared-external upgrade) and the effective volatility for an edge.
//
// Role-aware downgrade: a composition root (or a generated/test module) reaches
// into the modules it wires by design — that fan-out is cohesion, not high-
// distance coupling — so its outbound edges must never be scored as unbalanced.
// Cap the source's outbound distance below the high-distance threshold; this
// single point flows to the continuous Score (and hence Severity) and every
// distance-reading metric (unbalanced_edge, encapsulation, …).
// The basis stays as-is: it reflects what drove the original signal, not the cap.
//
// Declared external system (`external_systems:`), book Ch10 Example 1: a
// declared cross-vendor integration seam sits at the distance ladder's far end
// and ENTERS scoring, carrying the declared volatility (default low). Only a
// DECLARED external target gets D=10; an undeclared external edge keeps the
// disclosed exclusion (DistanceUnknown → classified_edges.external) — scoring
// every library import at D=10 would flood the metric with vendor noise. The
// match is gated on the TARGET's own resolution, not the composite distance:
// classifyDistance also returns DistanceUnknown when only the SOURCE is
// unresolved, and an edge into a real declared module must never be re-labelled
// external just because an external glob overlaps that module's path space. The
// match runs after the role cap deliberately: a composition root's edge to an
// external vendor system is a real integration seam, not its own cohesive wiring.
func resolveDistanceVolatility(fromPath, toPath, lang string, mi moduleIndex, c config.ClassifyConfig, degenerateExplicit, degenerateOwners bool, effectiveVol map[string]coupling.Volatility, extSystems externalSystemIndex) (coupling.Distance, coupling.DistanceBasis, coupling.Volatility) {
	modules := c.Modules
	dist, distBasis := classifyDistance(fromPath, toPath, lang, mi, modules, c.ExplicitOwners, degenerateExplicit, degenerateOwners)
	if fromMod, ok := mi.moduleFor(fromPath); ok && cohesiveRole(modules[fromMod].Role) {
		dist = capDistanceForRole(dist)
	}
	if dist == coupling.DistanceUnknown {
		if _, toOK := mi.moduleFor(toPath); !toOK {
			if v, ok := extSystems.match(toPath); ok {
				return coupling.DistanceExternal, coupling.DistanceBasisExternal, v
			}
		}
	}
	return dist, distBasis, classifyVolatilityEffective(toPath, mi, modules, effectiveVol)
}

// modulePairKey returns the canonical sorted key for a module pair.
func modulePairKey(a, b string) string {
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

type strengthResolution struct {
	strength       coupling.Strength
	fromPin        bool
	fromLLM        bool
	fromNonHighLLM bool
}

// resolveStrength applies the shared pre-clone strength precedence for one
// edge. classify adds the clone-derived Symmetric upgrade after this helper;
// the volatility cascade uses this pre-clone result and excludes clone pairs
// separately.
func resolveStrength(e graph.Edge, mi moduleIndex, c config.ClassifyConfig) strengthResolution {
	fromPath := pathFromID(e.From)
	toPath := pathFromID(e.To)
	// An internal-glob match is authoritative intrusive. A public-glob match is a
	// NOT-INTRUSIVE floor (str == contract): the edge goes through a declared public
	// surface, but the glob alone cannot say WHICH kind of public coupling it is —
	// a published interface (contract), a shared concrete type (model), or a function
	// (functional). For a public (contract) or unknown edge the KIND is resolved by
	// authority: an approved human label first (a reviewer's verdict beats a tool
	// guess), then the symbol-level hint, then the contract default. An intrusive or
	// human-pinned classification is never refined.
	str := classifyStrength(toPath, mi)
	fromPin := false
	if (str == coupling.StrengthContract || str == coupling.StrengthUnknown) && len(c.ApprovedLabels) > 0 {
		if fromMod, okF := mi.moduleFor(fromPath); okF {
			if toMod, okT := mi.moduleFor(toPath); okT {
				if pinned, ok := c.ApprovedLabels[fromMod+"\x00"+toMod]; ok {
					str = coupling.Strength(pinned)
					fromPin = true
				}
			}
		}
	}
	// Refine a public-glob contract floor to the hint's public-coupling kind when no
	// human label pinned it. This is what makes coupling_balance sensitive to
	// integration strength instead of reading every public edge as the weakest
	// (contract) kind. The hint can only raise the kind among the public kinds; it
	// never lowers a public edge to intrusive (the glob floor). Exception: a
	// pure-data DTO across a declared public boundary IS the book's explicit
	// integration contract — the floor stands unrefined.
	if !fromPin && str == coupling.StrengthContract && e.StrengthHint != graph.StrengthHintDTO {
		if k := strengthFromHint(e.StrengthHint); isPublicKind(k) {
			str = k
		}
	}
	// Unknown (no glob, no label) falls back to the hint.
	if str == coupling.StrengthUnknown {
		str = strengthFromHint(e.StrengthHint)
	}

	// An approved llm-provenance label fills ONLY a cell every static source
	// left unknown: no config glob matched (intrusive/contract handled above)
	// and the hint — Go type-info, SCIP, or extractor heuristic — resolved
	// nothing. It never displaces a static classification.
	strengthFromLLM := false
	strengthFromNonHighLLM := false
	if str == coupling.StrengthUnknown && len(c.LLMLabels) > 0 {
		if fromMod, okF := mi.moduleFor(fromPath); okF {
			if toMod, okT := mi.moduleFor(toPath); okT {
				key := fromMod + "\x00" + toMod
				if pinned, ok := c.LLMLabels[key]; ok {
					str = coupling.Strength(pinned)
					fromPin = true
					strengthFromLLM = true
					strengthFromNonHighLLM = c.LLMLabelConfidence[key] != "high"
				}
			}
		}
	}
	return strengthResolution{
		strength:       str,
		fromPin:        fromPin,
		fromLLM:        strengthFromLLM,
		fromNonHighLLM: strengthFromNonHighLLM,
	}
}

// isPublicKind reports whether a strength is one of the public-coupling kinds —
// contract (published interface), model (shared concrete type), or functional
// (function call). These are the kinds a public-glob edge may legitimately refine
// to. Intrusive (internals reach), symmetric (clone-derived), and unknown are
// excluded: a public-glob floor must never be lowered to intrusive, and the
// symmetric upgrade is applied separately from clone evidence.
func isPublicKind(s coupling.Strength) bool {
	switch s {
	case coupling.StrengthContract, coupling.StrengthModel, coupling.StrengthFunctional:
		return true
	default:
		return false
	}
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
	// A DTO hint without a declared public boundary is just a shared concrete
	// type — model. The boundary declaration is what makes a DTO a contract;
	// that case is handled at the floor-refinement site in classify, which
	// checks the raw hint before calling this mapping.
	if hint == graph.StrengthHintDTO {
		return coupling.StrengthModel
	}
	switch coupling.Strength(hint) {
	case coupling.StrengthContract, coupling.StrengthModel,
		coupling.StrengthFunctional, coupling.StrengthSymmetric, coupling.StrengthIntrusive:
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
//  2. Hand-authored ownership on EITHER endpoint is authoritative — BUT only when
//     the explicit-owner sub-map is NOT degenerate (i.e. at least two distinct
//     owners exist among explicitly-owned modules). A single-owner repo where every
//     module carries the same explicit owner yields no useful signal: Step 2 falls
//     through to Step 3 and Step 4 in that case so code structure dominates.
//  3. A real resolver ownership signal (≥2 distinct owners, not the single
//     git-author degenerate case) is authoritative too → ownership decides.
//  4. Otherwise (degenerate or no ownership) fall back to code structure.
//
// The returned DistanceBasis records which signal drove the final distance value.
func classifyDistance(fromPath, toPath, lang string, mi moduleIndex, modules map[string]config.ModuleDef, explicitOwners map[string]bool, degenerateExplicit, degenerateOwners bool) (coupling.Distance, coupling.DistanceBasis) {
	fromMod, fromOK := mi.moduleFor(fromPath)
	toMod, toOK := mi.moduleFor(toPath)

	if !fromOK || !toOK {
		return coupling.DistanceUnknown, coupling.DistanceBasisUnknown
	}

	if fromMod == toMod {
		return coupling.DistanceSameModule, coupling.DistanceBasisUnknown
	}

	return moduleDistance(fromMod, toMod, lang, modules, explicitOwners, degenerateExplicit, degenerateOwners)
}

// moduleDistance computes the composite distance between two RESOLVED, distinct
// modules — steps 1–4 of the precedence chain documented on classifyDistance.
// Factored out of classifyDistance so CloneOnlyPairs can compute a module-pair
// distance from module names alone: clone evidence carries repo file paths,
// which for Python never match the dotted node-ID globs the path resolution in
// classifyDistance expects.
func moduleDistance(fromMod, toMod, lang string, modules map[string]config.ModuleDef, explicitOwners map[string]bool, degenerateExplicit, degenerateOwners bool) (coupling.Distance, coupling.DistanceBasis) {
	fromDef := modules[fromMod]
	toDef := modules[toMod]

	deploy := deployDistance(fromDef.DeployUnit, toDef.DeployUnit)
	if deploy == coupling.DistanceCrossDeployUnit {
		return deploy, coupling.DistanceBasisDeployUnit // a deploy boundary is absolute
	}

	// Step 2: explicit hand-authored ownership on either endpoint fires only when
	// the explicit-owner sub-map is NOT degenerate (not all same owner). When every
	// explicitly-owned module shares a single owner (e.g. a single-team repo with
	// owner: alexei-led on all modules), Step 2 falls through so code structure
	// dominates — ownershipDistance("alexei-led","alexei-led") → SameOwner for every
	// edge, which would flatten all distances and defeat the structural signal.
	// degenerateExplicit is pre-computed once per Run to avoid rebuilding the map
	// on every edge.
	if explicitOwners[fromMod] || explicitOwners[toMod] {
		if !degenerateExplicit {
			return maxDistance(ownershipDistance(fromDef.Owner, toDef.Owner), deploy), coupling.DistanceBasisOwnership
		}
	}

	// Step 3: a real resolver ownership signal (≥2 distinct owners) is authoritative.
	// degenerateOwners is pre-computed once per Run to avoid rebuilding the full
	// module→owner map on every edge.
	if !degenerateOwners {
		return maxDistance(ownershipDistance(fromDef.Owner, toDef.Owner), deploy), coupling.DistanceBasisOwnership
	}

	// Step 4: git-author degenerate or no ownership → code structure.
	return maxDistance(codeStructureDistance(fromMod, toMod, lang), deploy), coupling.DistanceBasisStructure
}

// classifyVolatility derives domain volatility for the to-module from declared
// metadata only, per Khononov's volatility-from-subdomain mapping (core→high,
// supporting→low, generic→low) with an explicit per-module override:
//
//  1. Explicit `volatility` field on the module definition (hand-authored override).
//  2. Subdomain mapping: core→high, supporting→low, generic→low.
//
// There is NO path/name guessing: archfit does not infer volatility from a
// directory name (a fragile, surprising heuristic). A module that declares
// neither is reported as VolatilityUndeclared so the result is honest and the
// user is nudged to declare it.
//
// Resolution outcomes are deliberately three-valued:
//   - to-module unresolved → VolatilityUnknown (genuinely indeterminate).
//   - to-module resolved but neither volatility nor subdomain declared →
//     VolatilityUndeclared (a config gap; scored conservatively as worst-case,
//     and Lint() surfaces it).
//   - otherwise → high/medium/low/frozen.
//
// No churn or git history is consulted here — volatility is config-declared only.
func classifyVolatility(toPath string, mi moduleIndex, modules map[string]config.ModuleDef) coupling.Volatility {
	toMod, ok := mi.moduleFor(toPath)
	if !ok {
		return coupling.VolatilityUnknown
	}
	def := modules[toMod]

	// Priority 1: explicit Volatility field.
	// Accepted values: high, medium, low, frozen, legacy.
	switch strings.ToLower(def.Volatility) {
	case volatilityHigh:
		return coupling.VolatilityHigh
	case volatilityMedium:
		return coupling.VolatilityMedium
	case volatilityLow:
		return coupling.VolatilityLow
	case volatilityFrozen, volatilityLegacy:
		return coupling.VolatilityFrozen
	}

	// Priority 2: subdomain mapping.
	switch strings.ToLower(def.Subdomain) {
	case subdomainCore:
		return coupling.VolatilityHigh
	case subdomainSupporting:
		return coupling.VolatilityLow
	case subdomainGeneric:
		return coupling.VolatilityLow
	}

	// Nothing declared — a closable config gap, reported honestly (no guessing).
	return coupling.VolatilityUndeclared
}

// isGenericSubdomain reports whether the to-module is classified as a generic
// subdomain — true only when the module explicitly declares `subdomain: generic`.
// (No path/name guessing: archfit does not infer "generic" from a directory name.)
//
// This is used to trigger the contract-recommended advisory when a generic
// target is reached via non-contract strength (BC's anti-corruption-layer guidance).
func isGenericSubdomain(toPath string, mi moduleIndex, modules map[string]config.ModuleDef) bool {
	toMod, ok := mi.moduleFor(toPath)
	if !ok {
		return false
	}
	return strings.ToLower(modules[toMod].Subdomain) == subdomainGeneric
}

// computeEffectiveVolatility computes per-module effective volatility after a
// single-hop inferred-volatility cascade (book Ch9). When cascade is disabled,
// returns the base (config-declared) volatility for each module unchanged.
//
// Propagation rule: for each edge in the graph, if the from-module is strongly
// coupled (strength ≥ functional) to a to-module with high BASE volatility, the
// from-module's effective volatility is raised to high. Config-declared volatility
// always takes precedence and is never lowered.
//
// The cascade reads only the BASE volatility of the to-module — not the
// propagated value — making the result order-independent (single hop, no fixpoint).
//
// Strong strength set for propagation: Functional, Symmetric, Intrusive. An edge
// between a module pair in clonePairs is excluded even if it otherwise qualifies:
// a detected clone is accidental coupling (duplicated code, not a deliberate
// integration point), and volatility in the book's model (Ch9) is a component's
// essential rate of change, driven by its domain role — an incidental clone
// match between two modules says nothing about either module's real volatility,
// so it must not flip a whole module's effective volatility to high.
func computeEffectiveVolatility(g *graph.Graph, mi moduleIndex, c config.ClassifyConfig) map[string]coupling.Volatility {
	modules := c.Modules
	// Seed effective map from config-declared volatility.
	effective := make(map[string]coupling.Volatility, len(modules))
	for name, def := range modules {
		effective[name] = volatilityFromDef(def)
	}
	if !c.VolatilityCascadeEnabled || g == nil {
		return effective
	}
	// Snapshot the base volatility before propagation so reads during the pass
	// always reflect config (not yet-written effective values).
	base := make(map[string]coupling.Volatility, len(effective))
	maps.Copy(base, effective)
	// Propagation pass: iterate edges once, raise effective vol where applicable.
	for _, e := range g.Edges() {
		resolved := resolveStrength(e, mi, c)
		if !isStrongStrength(resolved.strength) {
			continue
		}
		fromPath := pathFromID(e.From)
		toPath := pathFromID(e.To)
		fromMod, okFrom := mi.moduleFor(fromPath)
		toMod, okTo := mi.moduleFor(toPath)
		if !okFrom || !okTo || fromMod == toMod {
			continue
		}
		if _, isClonePair := c.CrossModuleClonePairs[modulePairKey(fromMod, toMod)]; isClonePair {
			continue // accidental coupling — must not trigger the cascade
		}
		// Read the BASE volatility of the to-module (order-independent).
		if base[toMod] != coupling.VolatilityHigh {
			continue
		}
		// Raise the from-module's effective volatility to high (never lower it).
		if effective[fromMod] != coupling.VolatilityHigh {
			effective[fromMod] = coupling.VolatilityHigh
		}
	}
	return effective
}

// volatilityFromDef derives base volatility from a ModuleDef using the same
// priority logic as classifyVolatility but without the path heuristic (the
// module is already resolved; we want the config-declared level only).
func volatilityFromDef(def config.ModuleDef) coupling.Volatility {
	switch strings.ToLower(def.Volatility) {
	case volatilityHigh:
		return coupling.VolatilityHigh
	case volatilityMedium:
		return coupling.VolatilityMedium
	case volatilityLow:
		return coupling.VolatilityLow
	case volatilityFrozen, volatilityLegacy:
		return coupling.VolatilityFrozen
	}
	switch strings.ToLower(def.Subdomain) {
	case subdomainCore:
		return coupling.VolatilityHigh
	case subdomainSupporting:
		return coupling.VolatilityLow
	case subdomainGeneric:
		return coupling.VolatilityLow
	}
	return coupling.VolatilityUndeclared
}

// isStrongStrength reports whether str is at or above the functional threshold
// for the inferred-volatility cascade. Contract and model are below the threshold.
func isStrongStrength(str coupling.Strength) bool {
	return str == coupling.StrengthFunctional ||
		str == coupling.StrengthSymmetric ||
		str == coupling.StrengthIntrusive
}

// classifyVolatilityEffective derives effective volatility for the to-module
// using the pre-computed effectiveVol map (post-cascade). Falls back to
// classifyVolatility when the map is nil or the module is not found.
func classifyVolatilityEffective(toPath string, mi moduleIndex, modules map[string]config.ModuleDef, effectiveVol map[string]coupling.Volatility) coupling.Volatility {
	toMod, ok := mi.moduleFor(toPath)
	if !ok {
		return coupling.VolatilityUnknown
	}
	if effectiveVol != nil {
		if v, found := effectiveVol[toMod]; found {
			return v
		}
	}
	return classifyVolatility(toPath, mi, modules)
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
