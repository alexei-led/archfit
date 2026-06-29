package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// ---------------------------------------------------------------------------
// ForbiddenDependency
// ---------------------------------------------------------------------------

type forbiddenDependency struct {
	def config.RuleDef
}

func (r *forbiddenDependency) ID() string { return r.def.ID }

func (r *forbiddenDependency) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

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

// ---------------------------------------------------------------------------
// PublicAPIOnly
// ---------------------------------------------------------------------------

type publicAPIOnly struct {
	def config.RuleDef
}

func (r *publicAPIOnly) ID() string { return r.def.ID }

func (r *publicAPIOnly) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		if e.Kind != graph.EdgeKindUsesInternal {
			continue
		}
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

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

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"edge_kind": string(e.Kind),
			"to_path":   toPath,
		}
		f.Why = "Cross-module access to internal path " + toPath
		f.Constraint = "Only import from the module's public API"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// ForbiddenLayerDirection
// ---------------------------------------------------------------------------

type forbiddenLayerDirection struct {
	def    config.RuleDef
	layers []string
	mm     config.ModuleMap
}

func (r *forbiddenLayerDirection) ID() string { return r.def.ID }

func (r *forbiddenLayerDirection) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

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

// internalAPIAccess fires on edges with kind == uses_internal, optionally
// filtered by from/to glob. Supports the same from/to glob semantics as
// publicAPIOnly but is a distinct rule type so teams can configure them
// independently with different IDs, severities, and exceptions.
type internalAPIAccess struct {
	def config.RuleDef
}

func (r *internalAPIAccess) ID() string { return r.def.ID }

func (r *internalAPIAccess) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		if e.Kind != graph.EdgeKindUsesInternal {
			continue
		}
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

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

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"edge_kind": string(e.Kind),
			"from_path": fromPath,
			"to_path":   toPath,
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
	def config.RuleDef
	mm  config.ModuleMap
}

func (r *newCrossModuleDependency) ID() string { return r.def.ID }

func (r *newCrossModuleDependency) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

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

// cycleRule detects import cycles using Graph.Cycles() (shared Tarjan SCC).
// It emits one finding per strongly-connected component of size > 1.
// The finding ID is derived from the sorted SCC members for stability.
type cycleRule struct {
	def config.RuleDef
}

func (r *cycleRule) ID() string { return r.def.ID }

func (r *cycleRule) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	sccs := g.Cycles()
	if len(sccs) == 0 {
		return nil
	}

	out := make([]finding.Finding, 0, len(sccs))
	for _, scc := range sccs {
		id := cycleFingerprintID(r.def.ID, scc)
		// Use the first two members of the SCC as representative from/to for the edge evidence.
		fromPath := graph.NodePath(scc[0])
		toPath := graph.NodePath(scc[1%len(scc)])
		f := finding.Finding{
			ID:       id,
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: fromPath},
				To:   finding.Endpoint{Path: toPath},
				Kind: string(graph.EdgeKindImports),
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
// LayerRoleDivergence
// ---------------------------------------------------------------------------

// layerRoleDivergence detects modules whose observed topological rank in the
// import DAG diverges from the rank implied by their declared layer in config.
//
// Rank convention (matches forbidden_layer_direction):
//
//	layers: [domain, application, infrastructure]
//	         rank 0  rank 1        rank 2
//
// domain (rank 0) is imported-by-many; infrastructure (rank 2) imports many.
// Observed topological rank (longest-path depth from sources): a module that
// imports many others has a higher observed rank. A module imported by many
// others (inner, domain-like) has a low observed rank.
//
// Divergence = |observedRank - declaredRank|. Fires when > threshold (default 1).
// Cyclic nodes are skipped (abstain-not-fake, matches cycle rule convention).
type layerRoleDivergence struct {
	def       config.RuleDef
	layers    []string
	mm        config.ModuleMap
	threshold int
}

func (r *layerRoleDivergence) ID() string { return r.def.ID }

func (r *layerRoleDivergence) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	if len(g.Nodes()) == 0 {
		return nil
	}

	// Build module-level DAG: collapse file nodes to modules, drop self/external.
	// successors[m] = set of modules that m imports.
	moduleSet, successors := lrdBuildDAG(g, r.mm)

	// Kahn's BFS for longest-path depth (topological rank).
	// ranked[m] is only true for modules dequeued (all predecessors resolved).
	// Cyclic nodes accumulate a speculative rank but are never marked ranked.
	rank, ranked := lrdTopoRank(moduleSet, successors)

	// Per-WCC max rank: prevents a deep component from inflating the scaling
	// denominator for a shallower disconnected component (false-positive guard).
	componentOf, compMax := lrdWCCMaxRank(moduleSet, successors, rank, ranked)

	// Direction convention (matches forbidden_layer_direction):
	//   layers: [domain(0), app(1), infra(2)]
	//   domain = inner/sink  → imported by many, high observed DAG rank
	//   infra  = outer/source → imports many, low observed DAG rank (near 0)
	//
	// So declared rank and observed rank are inversely related in a healthy arch.
	// We compare scaledObs against (layerCount-1 - declaredRank) so that a
	// well-layered module has delta ≈ 0.
	var out []finding.Finding
	layerCount := len(r.layers)

	for mod := range moduleSet {
		if !ranked[mod] {
			// Cyclic node — abstain.
			continue
		}
		obsRank := rank[mod]
		layer, ok := r.mm.LayerForName(mod)
		if !ok {
			continue
		}
		declaredRank := layerRank(layer, r.layers)
		if declaredRank < 0 {
			// Layer not in config layers list.
			continue
		}

		// Scale observed rank to [0, layerCount-1] within the module's WCC.
		// Using per-component max prevents a deeper disconnected component from
		// inflating the denominator and compressing this component's ranks.
		scaledObs := obsRank
		if cm := compMax[componentOf[mod]]; cm > 0 && layerCount > 1 {
			scaledObs = (obsRank * (layerCount - 1)) / cm
		}

		// Expected: outer layers (high declaredRank) → low scaledObs (sources).
		// Expected: inner layers (low declaredRank) → high scaledObs (sinks).
		expectedObs := (layerCount - 1) - declaredRank

		delta := scaledObs - expectedObs
		if delta < 0 {
			delta = -delta
		}
		if delta <= r.threshold {
			continue
		}

		id := layerRoleDivergenceFingerprintID(r.def.ID, mod)
		f := finding.Finding{
			ID:       id,
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityMedium,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: mod},
				To:   finding.Endpoint{Path: mod},
				Kind: string(graph.EdgeKindImports),
			},
			MatchedBy: map[string]string{
				"module":         mod,
				"declared_layer": layer,
				"declared_rank":  strconv.Itoa(declaredRank),
				"observed_rank":  strconv.Itoa(scaledObs),
				"expected_rank":  strconv.Itoa(expectedObs),
				"delta":          strconv.Itoa(delta),
			},
			Why: fmt.Sprintf(
				"Module %q declared in layer %q (rank %d, expected observed rank %d) but observed at topological rank %d (delta %d > threshold %d)",
				mod, layer, declaredRank, expectedObs, scaledObs, delta, r.threshold,
			),
			Constraint: "Move the module to a layer that reflects its actual position in the dependency DAG, or restructure its imports",
		}
		out = append(out, f)
	}
	return out
}

// lrdBuildDAG collapses file-level graph edges to module-level, dropping
// self-edges and edges to/from unknown modules.
func lrdBuildDAG(g *graph.Graph, mm config.ModuleMap) (moduleSet map[string]struct{}, successors map[string]map[string]struct{}) {
	inDegree := map[string]int{}
	successors = map[string]map[string]struct{}{}
	moduleSet = map[string]struct{}{}

	for _, e := range g.Edges() {
		fromMod, fromOK := mm.ModuleFor(graph.NodePath(e.From))
		toMod, toOK := mm.ModuleFor(graph.NodePath(e.To))
		if !fromOK || !toOK || fromMod == toMod {
			continue
		}
		moduleSet[fromMod] = struct{}{}
		moduleSet[toMod] = struct{}{}
		if _, exists := successors[fromMod]; !exists {
			successors[fromMod] = map[string]struct{}{}
		}
		if _, added := successors[fromMod][toMod]; !added {
			successors[fromMod][toMod] = struct{}{}
			inDegree[toMod]++
		}
	}
	// Ensure every module exists in inDegree even if never imported.
	for m := range moduleSet {
		if _, ok := inDegree[m]; !ok {
			inDegree[m] = 0
		}
	}
	_ = inDegree // consumed below; returned via closure-free design
	return moduleSet, successors
}

// lrdTopoRank runs Kahn's BFS longest-path depth over the module DAG.
// rank[m] is the longest-path depth from any source. ranked[m] is true only
// for modules fully processed (all predecessors resolved); cyclic nodes are
// never marked ranked (abstain-not-fake).
func lrdTopoRank(moduleSet map[string]struct{}, successors map[string]map[string]struct{}) (rank map[string]int, ranked map[string]bool) {
	inDegree := map[string]int{}
	for m := range moduleSet {
		inDegree[m] = 0
	}
	for _, succs := range successors {
		for to := range succs {
			inDegree[to]++
		}
	}

	rank = map[string]int{}
	ranked = map[string]bool{}
	remaining := map[string]int{}
	var queue []string
	for m, d := range inDegree {
		remaining[m] = d
		if d == 0 {
			rank[m] = 0
			ranked[m] = true
			queue = append(queue, m)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for succ := range successors[cur] {
			if rank[cur]+1 > rank[succ] {
				rank[succ] = rank[cur] + 1
			}
			remaining[succ]--
			if remaining[succ] == 0 {
				ranked[succ] = true
				queue = append(queue, succ)
			}
		}
	}
	return rank, ranked
}

// lrdWCCMaxRank labels each module with a weakly-connected component ID and
// returns the maximum ranked rank within each component. Using per-component
// max prevents an unrelated deep component from inflating the scaling
// denominator and compressing a correctly-layered shallow component's ranks.
func lrdWCCMaxRank(
	moduleSet map[string]struct{},
	successors map[string]map[string]struct{},
	rank map[string]int,
	ranked map[string]bool,
) (componentOf map[string]int, compMax map[int]int) {
	// Build undirected adjacency from directed successors.
	undirNeighbors := make(map[string]map[string]struct{}, len(moduleSet))
	for m := range moduleSet {
		undirNeighbors[m] = map[string]struct{}{}
	}
	for from, succs := range successors {
		for to := range succs {
			undirNeighbors[from][to] = struct{}{}
			undirNeighbors[to][from] = struct{}{}
		}
	}

	componentOf = make(map[string]int, len(moduleSet))
	nextComp := 0
	for seed := range moduleSet {
		if _, visited := componentOf[seed]; visited {
			continue
		}
		compID := nextComp
		nextComp++
		bfsQ := []string{seed}
		componentOf[seed] = compID
		for len(bfsQ) > 0 {
			cur := bfsQ[0]
			bfsQ = bfsQ[1:]
			for nb := range undirNeighbors[cur] {
				if _, visited := componentOf[nb]; !visited {
					componentOf[nb] = compID
					bfsQ = append(bfsQ, nb)
				}
			}
		}
	}

	// Per-component max observed rank (ranked nodes only).
	compMax = map[int]int{}
	for m, v := range rank {
		if ranked[m] && v > compMax[componentOf[m]] {
			compMax[componentOf[m]] = v
		}
	}
	return componentOf, compMax
}

// layerRoleDivergenceFingerprintID computes a stable 32-char hex ID for a
// layer_role_divergence finding from the rule ID and module name.
func layerRoleDivergenceFingerprintID(ruleID, mod string) string {
	h := sha256.Sum256([]byte(ruleID + "\x00" + mod))
	return hex.EncodeToString(h[:16])
}
