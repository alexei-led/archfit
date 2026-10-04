package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// ---------------------------------------------------------------------------
// ForbiddenDependency
// ---------------------------------------------------------------------------

// validateForbiddenDependencyDef validates a RuleDef for the
// forbidden_dependency rule type. An empty from/to glob matches nothing, so
// the rule would load clean yet never fire — a silently-vacuous gate.
func validateForbiddenDependencyDef(def policy.RuleDef) error {
	if def.From == "" || def.To == "" {
		return fmt.Errorf("rules: forbidden_dependency %q requires both from and to globs", def.ID)
	}
	return validateScopeGlobs(def)
}

// validateScopeGlobs rejects malformed from/to globs. doublestar.Match
// returns ErrBadPattern at check time, which Check discards — a malformed
// glob would make the rule silently fire zero findings forever, the same
// silently-vacuous-gate failure the emptiness check above guards against.
func validateScopeGlobs(def policy.RuleDef) error {
	for field, pat := range map[string]string{"from": def.From, "to": def.To} {
		if pat != "" && !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("rules: rule %q has a malformed %s glob %q", def.ID, field, pat)
		}
	}
	return nil
}

type forbiddenDependency struct {
	def policy.RuleDef
}

func (r *forbiddenDependency) ID() string { return r.def.ID }

func (r *forbiddenDependency) Check(s relationship.Set, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range s.Edges {
		fromPath := e.FromPath
		toPath := e.ToPath

		fromMatch, _ := doublestar.Match(r.def.From, fromPath)
		toMatch, _ := doublestar.Match(r.def.To, toPath)
		if !fromMatch || !toMatch {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"from_glob": r.def.From,
			"to_glob":   r.def.To,
		}
		f.Why = "Import from " + r.def.From + " to " + r.def.To + " is explicitly forbidden"
		f.Constraint = "Remove the dependency or move the code"
		out = append(out, f)
	}
	return out
}

// sameModule reports whether fromPath and toPath resolve to the same module —
// a module reaching into its own internal path (e.g. domain importing
// domain/internal) is idiomatic, not a violation; only cross-module access to
// another module's internal surface is. When either endpoint isn't covered by
// the module map, we can't rule out same-module, so callers must treat that
// as "not same module" (module-blind fallback: the edge still fires).
func sameModule(mm policy.ModuleMap, fromPath, toPath string) bool {
	fromModule, fromOK := mm.ModuleFor(fromPath)
	toModule, toOK := mm.ModuleFor(toPath)
	return fromOK && toOK && fromModule == toModule
}

// internalTarget decides whether an edge lands on internal surface for the two
// internal-access rules. Declared surfaces decide first (policy.ModuleMap.
// MatchesInternal): a public: glob of the target's module exempts the edge, and
// any module's internal: glob marks it internal, in every language. Only when no
// declaration speaks does the extractor's uses_internal kind decide — the Go
// `/internal/` path segment, or the TS/Python extractors' own internal-glob test.
//
// Deciding here rather than in the extractor keeps edge kinds, and therefore
// finding fingerprints, unchanged. Callers pass dependency edges only: a
// belongs_to or exposes edge is not an access. glob names the declaration that made the
// target internal; it is empty when the extractor kind decided.
func internalTarget(mm policy.ModuleMap, e relationship.Edge) (internal bool, glob string) {
	if internal, glob := mm.MatchesInternal(e.ToPath); glob != "" {
		return internal, glob
	}
	return e.Kind == relEdgeKindUsesInt, ""
}

// matchedByInternalGlob is the MatchedBy key naming the declared internal: glob
// that made an edge target internal.
const matchedByInternalGlob = "internal_glob"

// ---------------------------------------------------------------------------
// PublicAPIOnly
// ---------------------------------------------------------------------------

type publicAPIOnly struct {
	def policy.RuleDef
	mm  policy.ModuleMap
}

func (r *publicAPIOnly) ID() string { return r.def.ID }

func (r *publicAPIOnly) Check(s relationship.Set, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range s.DependencyEdges() {
		fromPath := e.FromPath
		toPath := e.ToPath

		// Apply From/To scope globs when set; empty glob means match-all.
		if r.def.From != "" {
			if matched, _ := doublestar.Match(r.def.From, fromPath); !matched {
				continue
			}
		}
		if r.def.To != "" {
			if matched, _ := doublestar.Match(r.def.To, toPath); !matched {
				continue
			}
		}

		internal, glob := internalTarget(r.mm, e)
		if !internal || sameModule(r.mm, fromPath, toPath) {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"edge_kind": e.Kind,
			"to_path":   toPath,
		}
		if glob != "" {
			f.MatchedBy[matchedByInternalGlob] = glob
		}
		why := "Access to internal path " + toPath
		if fromModule, fromOK := r.mm.ModuleFor(fromPath); fromOK {
			if toModule, toOK := r.mm.ModuleFor(toPath); toOK {
				why = fmt.Sprintf("Cross-module access from %q (%s) to internal path %q (%s)", fromPath, fromModule, toPath, toModule)
			}
		}
		f.Why = why
		f.Constraint = "Only import from the module's public API"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// ForbiddenLayerDirection
// ---------------------------------------------------------------------------

type forbiddenLayerDirection struct {
	def    policy.RuleDef
	layers []string
	mm     policy.ModuleMap
}

func (r *forbiddenLayerDirection) ID() string { return r.def.ID }

func (r *forbiddenLayerDirection) Check(s relationship.Set, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range s.Edges {
		fromPath := e.FromPath
		toPath := e.ToPath

		fromLayer, ok := r.mm.LayerFor(fromPath)
		if !ok {
			continue
		}
		toLayer, ok := r.mm.LayerFor(toPath)
		if !ok {
			continue
		}

		fromRank := layerRank(fromLayer, r.layers)
		toRank := layerRank(toLayer, r.layers)

		// Skip if either layer is not in the ordered list.
		if fromRank < 0 || toRank < 0 {
			continue
		}

		// Violation: dependency flows from lower-index (higher-priority/inner) to
		// higher-index (lower-priority/outer) layer — the wrong direction.
		// E.g. layers=[domain(0), application(1), infrastructure(2)]:
		//   domain(0) → infrastructure(2) is forbidden (fromRank < toRank).
		//   infrastructure(2) → domain(0) is allowed  (fromRank > toRank).
		if fromRank >= toRank {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"from_layer": fromLayer,
			"to_layer":   toLayer,
		}
		f.Why = "Dependency from layer " + fromLayer + " to layer " + toLayer + " violates layer ordering"
		f.Constraint = "Dependencies must flow from higher layers to lower layers"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// InternalAPIAccess
// ---------------------------------------------------------------------------

// internalAPIAccess fires on edges whose target is internal (internalTarget),
// optionally filtered by from/to glob. Supports the same from/to glob and
// module-map semantics as publicAPIOnly but is a distinct rule type so teams can
// configure them independently with different IDs, severities, and exceptions.
type internalAPIAccess struct {
	def policy.RuleDef
	mm  policy.ModuleMap
}

func (r *internalAPIAccess) ID() string { return r.def.ID }

func (r *internalAPIAccess) Check(s relationship.Set, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range s.DependencyEdges() {
		fromPath := e.FromPath
		toPath := e.ToPath

		if r.def.From != "" {
			if matched, _ := doublestar.Match(r.def.From, fromPath); !matched {
				continue
			}
		}
		if r.def.To != "" {
			if matched, _ := doublestar.Match(r.def.To, toPath); !matched {
				continue
			}
		}

		internal, glob := internalTarget(r.mm, e)
		if !internal || sameModule(r.mm, fromPath, toPath) {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"edge_kind": e.Kind,
			"from_path": fromPath,
			"to_path":   toPath,
		}
		if glob != "" {
			f.MatchedBy[matchedByInternalGlob] = glob
		}
		f.Why = "Access to internal API path " + toPath + " from " + fromPath
		f.Constraint = "Only import from the module's public API surface"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// NewCrossModuleDependency
// ---------------------------------------------------------------------------

// newCrossModuleDependency fires when an edge crosses module boundaries.
// It uses ModuleMap to determine module ownership of from/to paths.
//
// "New" semantics are deliberately NOT implemented here: the rule emits every
// cross-module edge, and the status stage (status.Assign) marks edges whose
// fingerprint is in the baseline as StatusBaseline — only StatusNew /
// StatusExpiredWaiver findings gate. Filtering inside the rule would break
// fixed-finding detection (a suppressed finding's fingerprint would vanish
// from the current set and be falsely reported as fixed). Bootstrap behavior:
// with no baseline every cross-module edge fires — run `archfit baseline` to
// accept the current state.
type newCrossModuleDependency struct {
	def policy.RuleDef
	mm  policy.ModuleMap
}

func (r *newCrossModuleDependency) ID() string { return r.def.ID }

func (r *newCrossModuleDependency) Check(s relationship.Set, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range s.Edges {
		fromPath := e.FromPath
		toPath := e.ToPath

		fromModule, fromOK := r.mm.ModuleFor(fromPath)
		toModule, toOK := r.mm.ModuleFor(toPath)

		// Skip edges where either endpoint is unowned or both are in the same module.
		if !fromOK || !toOK || fromModule == toModule {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityMedium
		f.MatchedBy = map[string]string{
			"from_module": fromModule,
			"to_module":   toModule,
		}
		f.Why = fmt.Sprintf("New cross-module dependency from %q (%s) to %q (%s)", fromPath, fromModule, toPath, toModule)
		f.Constraint = "Cross-module dependencies must be reviewed and approved"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// CycleRule
// ---------------------------------------------------------------------------

// cycleRule detects import cycles using relationship.Set.Cycles() (shared Tarjan
// SCC). It emits one finding per strongly-connected component of size > 1.
// The finding ID is derived from the sorted SCC members for stability.
type cycleRule struct {
	def policy.RuleDef
}

func (r *cycleRule) ID() string { return r.def.ID }

func (r *cycleRule) Check(s relationship.Set, _ Evidence) []finding.Finding {
	sccs := s.Cycles()
	if len(sccs) == 0 {
		return nil
	}

	out := make([]finding.Finding, 0, len(sccs))
	for _, scc := range sccs {
		id := cycleFingerprintID(r.def.ID, scc)
		// Use the first two members of the SCC as representative from/to for the edge evidence.
		fromPath := relationship.NodePath(scc[0])
		toPath := relationship.NodePath(scc[1%len(scc)])
		f := finding.Finding{
			ID:       id,
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: fromPath},
				To:   finding.Endpoint{Path: toPath},
				Kind: relEdgeKindImports,
			},
			MatchedBy: map[string]string{
				"cycle_members": strings.Join(scc, ", "),
				"cycle_size":    strconv.Itoa(len(scc)),
			},
			Why:        fmt.Sprintf("Import cycle detected among %d nodes: %s", len(scc), strings.Join(scc, " → ")),
			Constraint: "Break the cycle by introducing an abstraction or reorganizing packages",
		}
		out = append(out, f)
	}
	return out
}

// cycleFingerprintID computes a stable 32-char hex ID for a cycle finding
// from the rule ID and the sorted SCC members.
func cycleFingerprintID(ruleID string, scc []string) string {
	h := sha256.Sum256([]byte(ruleID + "\x00" + strings.Join(scc, "\x00")))
	return hex.EncodeToString(h[:16])
}

// ---------------------------------------------------------------------------
// ModuleCycle
// ---------------------------------------------------------------------------

// edgeKindModuleDependency is the finding edge kind of a module-pair finding:
// its endpoints are modules, not graph nodes.
const edgeKindModuleDependency = "module_dependency"

// validateModuleCycleDef rejects from/to on module_cycle: the rule always checks
// the whole declared-module graph, so a scope glob would load clean and do
// nothing.
func validateModuleCycleDef(def policy.RuleDef) error {
	if def.From != "" || def.To != "" {
		return fmt.Errorf("rules: module_cycle %q takes no from/to: it checks every declared module", def.ID)
	}
	return nil
}

// moduleCycle reports dependency cycles among DECLARED modules
// (relationship.Set.ModuleCycles). cycleRule is node-level and structurally
// silent on Go, whose edges run file -> package; a module cycle closes through
// different files and packages, so only the module graph sees it.
//
// It emits one finding per ordered module pair whose dependency lies inside a
// cycle, keyed on (rule, kind, from module, to module): removing a direction
// fixes that direction's finding, and breaking the cycle fixes the rest.
// Synthetic modules (auto-registered Rust crate::mod nodes, Go workspace
// members) stay out — the rule speaks for the declared architecture, and the
// node-level cycle rule already sees their cycles.
type moduleCycle struct {
	def policy.RuleDef
	mm  policy.ModuleMap
}

func (r *moduleCycle) ID() string { return r.def.ID }

func (r *moduleCycle) Check(s relationship.Set, _ Evidence) []finding.Finding {
	sccs := s.ModuleCycles(r.mm.Has)
	if len(sccs) == 0 {
		return nil
	}
	component := make(map[string]int)
	for i, scc := range sccs {
		for _, module := range scc {
			component[module] = i
		}
	}
	type pair struct{ from, to string }
	sites := make(map[pair][]relationship.Location)
	for _, e := range s.DependencyEdges() {
		i, fromOK := component[e.FromModule]
		j, toOK := component[e.ToModule]
		if !fromOK || !toOK || i != j || e.FromModule == e.ToModule {
			continue
		}
		p := pair{from: e.FromModule, to: e.ToModule}
		sites[p] = append(sites[p], edgeLocations(e)...)
	}
	pairs := make([]pair, 0, len(sites))
	for p := range sites {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a].from != pairs[b].from {
			return pairs[a].from < pairs[b].from
		}
		return pairs[a].to < pairs[b].to
	})

	out := make([]finding.Finding, 0, len(pairs))
	for _, p := range pairs {
		scc := sccs[component[p.from]]
		members := strings.Join(scc, ", ")
		locs, total := sortedCappedLocations(sites[p])
		f := finding.NewKeyed(r.def.ID, edgeKindModuleDependency, p.from, p.to)
		f.Severity = finding.SeverityHigh
		f.Edge.From = finding.Endpoint{Module: p.from}
		f.Edge.To = finding.Endpoint{Module: p.to}
		f.Locations = locs
		f.MatchedBy = map[string]string{
			"from_module":     p.from,
			"to_module":       p.to,
			"cycle_modules":   members,
			"cycle_size":      strconv.Itoa(len(scc)),
			"locations_total": strconv.Itoa(total),
		}
		f.Why = fmt.Sprintf("Module %s depends on %s, and the two are in a dependency cycle among declared modules: %s", p.from, p.to, members)
		f.Constraint = "Remove one direction of the module cycle; routing the dependency through another module keeps the cycle"
		out = append(out, f)
	}
	return out
}

// edgeLocations returns an edge's import sites, or the importing file itself
// (line unknown) for a file node whose extractor reports no sites — the
// TypeScript shape. Dotted Python and crate::mod node IDs are not files, so
// they contribute nothing rather than a fabricated path.
func edgeLocations(e relationship.Edge) []relationship.Location {
	if len(e.Locations) > 0 {
		return e.Locations
	}
	if strings.HasPrefix(e.FromID, relNodeKindFile+":") {
		return []relationship.Location{{File: e.FromPath}}
	}
	return nil
}

// maxFindingLocations caps the locations one finding carries; the full count
// rides in matched_by.locations_total.
const maxFindingLocations = 50

// sortedCappedLocations deduplicates locs, sorts them by file and line, and
// keeps the first maxFindingLocations. total is the deduplicated count.
func sortedCappedLocations(locs []relationship.Location) (out []relationship.Location, total int) {
	seen := make(map[relationship.Location]struct{}, len(locs))
	out = make([]relationship.Location, 0, len(locs))
	for _, l := range locs {
		if _, dup := seen[l]; dup {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].File != out[b].File {
			return out[a].File < out[b].File
		}
		return out[a].Line < out[b].Line
	})
	total = len(out)
	if total > maxFindingLocations {
		out = out[:maxFindingLocations]
	}
	return out, total
}
