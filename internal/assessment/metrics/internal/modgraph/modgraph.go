// Package modgraph provides the shared module-level graph and history helpers
// used by the modularity metrics: collapsing nodes/files to module units,
// aggregating churn, and computing SCC-condensed blast radius. It is nested
// under internal/ so only the metrics subtree can import it.
package modgraph

import "github.com/alexei-led/archfit/internal/relationship"

// FirstPartyModules returns the set of module keys for the nodes archfit actually
// parsed, excluding external nodes. External dependencies must never be treated as
// owned modules, or the blast-radius metric flags them as first-party.
func FirstPartyModules(set relationship.Set) map[string]struct{} {
	fp := make(map[string]struct{})
	for _, n := range set.Nodes {
		if !n.FirstParty {
			continue
		}
		fp[n.Module] = struct{}{}
	}
	return fp
}

// BlastRadius returns, per first-party module, the number of other first-party
// modules that transitively depend on it. The graph is collapsed to module units
// (ModuleKey), restricted to first-party modules (those that import something — a
// node that only ever appears as an import target is an external dependency, since
// archfit never parses its source), and SCC-condensed so import cycles do not
// inflate the count (Martin's metrics and blast radius assume a DAG). Returns the
// per-module blast and the count of first-party modules.
func BlastRadius(set relationship.Set) (map[string]int, int) {
	// First-party = the nodes archfit actually parsed, minus external nodes. Most
	// external dependencies appear only as edge targets, never as nodes; the TS
	// extractor additionally emits unresolved targets as explicit external nodes,
	// which FirstPartyModules filters out. This excludes stdlib/third-party packages
	// without dropping pure-leaf internal modules.
	firstParty := FirstPartyModules(set)
	// Collapsed module adjacency over first-party modules only.
	adj := make(map[string]map[string]struct{})
	for m := range firstParty {
		adj[m] = make(map[string]struct{})
	}
	for _, e := range set.DependencyEdges() {
		from, to := relationship.ModuleKey(e.FromID), relationship.ModuleKey(e.ToID)
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
