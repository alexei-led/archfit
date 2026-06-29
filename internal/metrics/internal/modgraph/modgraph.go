// Package modgraph provides the shared module-level graph and history helpers
// used by the modularity metrics: collapsing nodes/files to module units,
// aggregating churn, and computing SCC-condensed blast radius. It is nested
// under internal/ so only the metrics subtree can import it.
package modgraph

import (
	"strings"

	"github.com/alexei-led/archfit/internal/model/graph"
)

// FirstPartyModules returns the set of module keys for the nodes archfit actually
// parsed, excluding external nodes (NodeKindExternal: unresolved npm packages,
// uninstalled third-party deps, node builtins an extractor could not tag as core).
// External dependencies must never be treated as owned modules, or the
// blast-radius metric flags them as first-party.
func FirstPartyModules(g *graph.Graph) map[string]struct{} {
	fp := make(map[string]struct{})
	for _, n := range g.Nodes() {
		if n.Kind == graph.NodeKindExternal {
			continue
		}
		fp[ModuleKey(n.ID())] = struct{}{}
	}
	return fp
}

// ModuleKey collapses a graph node id ("kind:path") to its package/module unit so
// blast radius is computed at the granularity the metric reports: Go file nodes
// collapse to their package directory; module/package/TS-file nodes pass through.
func ModuleKey(nodeID string) string {
	path := nodeID
	if _, after, ok := strings.Cut(nodeID, ":"); ok {
		path = after
	}
	if strings.HasSuffix(path, ".go") {
		if j := strings.LastIndexByte(path, '/'); j >= 0 {
			return path[:j] // package = directory
		}
	}
	return path
}

// DominantLanguage returns the language of most edges (drives file→module mapping).
func DominantLanguage(g *graph.Graph) string {
	cnt := map[string]int{}
	for _, e := range g.Edges() {
		cnt[e.Language]++
	}
	best, bn := "", 0
	for l, n := range cnt {
		if n > bn {
			best, bn = l, n
		}
	}
	return best
}

// FileToModuleKey maps a git file path to the same module unit as ModuleKey, so
// per-file churn/co-change aggregates onto graph nodes. The per-language mapping
// lives in the NodeConvention registry (model ring): Go collapses to the package
// directory, Python to the dotted module, and TS/JS (or any unknown language)
// passes the file through unchanged.
func FileToModuleKey(file, lang string) string {
	return graph.BuiltinConventions.Lookup(lang).FileToModuleKey(file)
}

// ModuleKeyResolver returns a file→module-key mapper bound to this graph, computing
// the language (and, for Rust, the crate roots) once instead of per file. For a
// Rust-dominant graph carrying crate roots it resolves to module granularity
// ("<crate>::<mod>") so per-file LOC/churn aggregates onto module nodes rather than
// collapsing to the crate; every other case uses the language convention. The crate
// roots are honoured only when Rust is the dominant language (or the graph has no
// edges to judge), so a stray Cargo.toml in a Go/TS repo never flips the mapping.
func ModuleKeyResolver(g *graph.Graph) func(string) string {
	lang := DominantLanguage(g)
	if crates := g.CrateRoots(); len(crates) > 0 && (lang == graph.LangRust || lang == "") {
		return func(f string) string { return graph.RustFileToModuleKey(f, crates) }
	}
	return func(f string) string { return FileToModuleKey(f, lang) }
}

// ModuleChurn aggregates per-file churn onto module keys using the graph-bound
// resolver, so Rust files resolve to module granularity when crate roots are present
// (identical to the per-language convention otherwise).
func ModuleChurn(g *graph.Graph, fileChurn map[string]int) map[string]int {
	resolve := ModuleKeyResolver(g)
	mc := map[string]int{}
	for f, c := range fileChurn {
		if k := resolve(f); k != "" {
			mc[k] += c
		}
	}
	return mc
}

// OrderedPair returns a canonical [2]string pair with a <= b.
func OrderedPair(a, b string) [2]string {
	if a <= b {
		return [2]string{a, b}
	}
	return [2]string{b, a}
}

// BlastRadius returns, per first-party module, the number of other first-party
// modules that transitively depend on it. The graph is collapsed to module units
// (ModuleKey), restricted to first-party modules (those that import something — a
// node that only ever appears as an import target is an external dependency, since
// archfit never parses its source), and SCC-condensed so import cycles do not
// inflate the count (Martin's metrics and blast radius assume a DAG). Returns the
// per-module blast and the count of first-party modules.
func BlastRadius(g *graph.Graph) (map[string]int, int) {
	// First-party = the nodes archfit actually parsed (g.Nodes()), minus external
	// nodes. Most external dependencies appear only as edge targets, never as
	// nodes; the TS extractor additionally emits unresolved targets as explicit
	// NodeKindExternal nodes, which FirstPartyModules filters out. This excludes
	// stdlib/third-party packages without dropping pure-leaf internal modules.
	firstParty := FirstPartyModules(g)
	// Collapsed module adjacency over first-party modules only.
	adj := make(map[string]map[string]struct{})
	for m := range firstParty {
		adj[m] = make(map[string]struct{})
	}
	for _, e := range g.Edges() {
		from, to := ModuleKey(e.From), ModuleKey(e.To)
		if from == to {
			continue
		}
		if _, ok := firstParty[to]; !ok {
			continue // edge into an external dependency
		}
		adj[from][to] = struct{}{}
	}

	comp := tarjanSCC(adj) // module -> component id
	compMembers := make(map[int]int)
	for _, c := range comp {
		compMembers[c]++
	}
	// Condensed reverse adjacency (component -> set of components that import it).
	crev := make(map[int]map[int]struct{})
	for from, tos := range adj {
		for to := range tos {
			cf, ct := comp[from], comp[to]
			if cf == ct {
				continue
			}
			if crev[ct] == nil {
				crev[ct] = make(map[int]struct{})
			}
			crev[ct][cf] = struct{}{}
		}
	}

	blast := make(map[string]int, len(firstParty))
	for m := range firstParty {
		// Transitive reverse-reach in the condensed DAG, counting member modules.
		start := comp[m]
		seen := map[int]struct{}{start: {}}
		stack := []int{start}
		members := 0
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for pc := range crev[c] {
				if _, ok := seen[pc]; !ok {
					seen[pc] = struct{}{}
					members += compMembers[pc]
					stack = append(stack, pc)
				}
			}
		}
		// Members of m's own SCC (besides m) also depend on m.
		blast[m] = members + (compMembers[start] - 1)
	}
	return blast, len(firstParty)
}

// tarjanSCC assigns each node a strongly-connected-component id over the adjacency.
func tarjanSCC(adj map[string]map[string]struct{}) map[string]int {
	const unvisited = -1
	index := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	comp := make(map[string]int)
	for n := range adj {
		index[n] = unvisited
	}
	var stack []string
	counter, compID := 0, 0

	// Iterative DFS to avoid stack overflow on large graphs.
	strongConnect := func(root string) {
		type frame struct {
			node string
			next []string
			i    int
		}
		neighbors := func(n string) []string {
			ns := make([]string, 0, len(adj[n]))
			for to := range adj[n] {
				ns = append(ns, to)
			}
			return ns
		}
		dfs := []frame{{node: root, next: neighbors(root)}}
		index[root], low[root] = counter, counter
		counter++
		stack = append(stack, root)
		onStack[root] = true
		for len(dfs) > 0 {
			f := &dfs[len(dfs)-1]
			if f.i < len(f.next) {
				w := f.next[f.i]
				f.i++
				if index[w] == unvisited {
					index[w], low[w] = counter, counter
					counter++
					stack = append(stack, w)
					onStack[w] = true
					dfs = append(dfs, frame{node: w, next: neighbors(w)})
				} else if onStack[w] && index[w] < low[f.node] {
					low[f.node] = index[w]
				}
				continue
			}
			if low[f.node] == index[f.node] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp[w] = compID
					if w == f.node {
						break
					}
				}
				compID++
			}
			child := f.node
			dfs = dfs[:len(dfs)-1]
			if len(dfs) > 0 {
				parent := &dfs[len(dfs)-1]
				if low[child] < low[parent.node] {
					low[parent.node] = low[child]
				}
			}
		}
	}
	for n := range adj {
		if index[n] == unvisited {
			strongConnect(n)
		}
	}
	return comp
}
