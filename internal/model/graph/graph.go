package graph

import (
	"cmp"
	"slices"
	"strings"
)

// NodeKind classifies what a Node represents.
type NodeKind string

// Node kind constants matching spec §7 node types.
const (
	NodeKindRepo     NodeKind = "repo"
	NodeKindModule   NodeKind = "module"
	NodeKindPackage  NodeKind = "package"
	NodeKindFile     NodeKind = "file"
	NodeKindExternal NodeKind = "external"
)

// EdgeKind classifies the relationship between two nodes.
type EdgeKind string

// Edge kind constants matching spec §7 edge types.
const (
	EdgeKindBelongsTo    EdgeKind = "belongs_to"
	EdgeKindImports      EdgeKind = "imports"
	EdgeKindDependsOn    EdgeKind = "depends_on"
	EdgeKindExposes      EdgeKind = "exposes"
	EdgeKindUsesInternal EdgeKind = "uses_internal"
)

// Location is a source-code position for an edge (e.g. an import statement).
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Node is a vertex in the dependency graph. Its identity is Kind + ":" + Path.
type Node struct {
	Kind NodeKind `json:"kind"`
	Path string   `json:"path"`
}

// ID returns the canonical node identity: "<kind>:<path>" (e.g. "file:pkg/a/a.go").
func (n Node) ID() string {
	return string(n.Kind) + ":" + n.Path
}

// NodePath extracts the path component from a node ID of the form "kind:path"
// (e.g. "file:pkg/a/a.go" → "pkg/a/a.go"). If no colon is present, the ID is
// returned unchanged.
func NodePath(id string) string {
	_, after, ok := strings.Cut(id, ":")
	if ok {
		return after
	}
	return id
}

// Edge is a directed dependency between two nodes.
// From and To are node IDs (NodeKind + ":" + path).
type Edge struct {
	From             string     `json:"from"`
	To               string     `json:"to"`
	Kind             EdgeKind   `json:"kind"`
	Language         string     `json:"language"`
	Confidence       string     `json:"confidence"`
	Locations        []Location `json:"locations"`
	ExplicitnessHint string     `json:"explicitness_hint,omitempty"`
}

// canonicalKey uniquely identifies an edge regardless of source.
type canonicalKey struct {
	From string
	To   string
	Kind EdgeKind
}

func edgeKey(e Edge) canonicalKey {
	return canonicalKey{From: e.From, To: e.To, Kind: e.Kind}
}

// Language name constants used for multi-source priority ordering.
const (
	LangGo         = "go"
	LangTypeScript = "typescript"
	LangPython     = "python"
)

// languagePriority returns a numeric priority for language-based tiebreaking.
// Lower number = higher priority.
func languagePriority(lang string) int {
	switch lang {
	case LangGo:
		return 0
	case LangTypeScript:
		return 1
	case LangPython:
		return 2
	default:
		return 3
	}
}

// Facts holds raw extractor output — nodes, edges, and unresolved counts —
// before deduplication and sorting.
type Facts struct {
	Nodes      []Node
	Edges      []Edge
	Language   string
	Unresolved int
}

// Graph is a sealed, immutable dependency graph produced by Build.
// After construction no field is mutated; accessors return copies.
type Graph struct {
	nodes []Node
	edges []Edge
}

// Build merges one or more Facts into a sealed, deterministic Graph.
//
// Determinism guarantees:
//   - Nodes are deduplicated by ID (kind:path) and sorted by ID.
//   - Edges are deduplicated by canonical key (from, to, kind).
//     When the same key appears across multiple Facts, the entry from the
//     highest-priority language (go > typescript > python) is kept and all
//     Locations from every matching entry are merged (sorted, deduplicated).
//   - Edge Locations within each edge are sorted by (file, line).
//   - Edges are sorted by (from, to, kind, firstLocation.File, firstLocation.Line).
func Build(facts []Facts) *Graph {
	// --- Dedup nodes ---
	seen := make(map[string]struct{})
	var nodes []Node
	for _, f := range facts {
		for _, n := range f.Nodes {
			id := n.ID()
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				nodes = append(nodes, n)
			}
		}
	}
	slices.SortFunc(nodes, func(a, b Node) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	// --- Dedup edges ---
	// Two passes: first choose the winning language per key, then merge locations.

	type candidate struct {
		edge     Edge
		priority int
	}
	winners := make(map[canonicalKey]candidate)

	for _, f := range facts {
		prio := languagePriority(f.Language)
		for _, e := range f.Edges {
			k := edgeKey(e)
			existing, ok := winners[k]
			if !ok || prio < existing.priority {
				winners[k] = candidate{edge: e, priority: prio}
			}
		}
	}

	// Merge all locations onto each winner.
	for _, f := range facts {
		for _, e := range f.Edges {
			k := edgeKey(e)
			c := winners[k]
			locSeen := make(map[Location]struct{}, len(c.edge.Locations))
			for _, l := range c.edge.Locations {
				locSeen[l] = struct{}{}
			}
			for _, l := range e.Locations {
				if _, ok := locSeen[l]; !ok {
					locSeen[l] = struct{}{}
					c.edge.Locations = append(c.edge.Locations, l)
				}
			}
			winners[k] = c
		}
	}

	// Collect edges, sort their Locations, then sort the edge slice.
	edges := make([]Edge, 0, len(winners))
	for _, c := range winners {
		e := c.edge
		slices.SortFunc(e.Locations, func(a, b Location) int {
			if n := cmp.Compare(a.File, b.File); n != 0 {
				return n
			}
			return cmp.Compare(a.Line, b.Line)
		})
		edges = append(edges, e)
	}

	slices.SortFunc(edges, func(a, b Edge) int {
		if n := cmp.Compare(a.From, b.From); n != 0 {
			return n
		}
		if n := cmp.Compare(a.To, b.To); n != 0 {
			return n
		}
		if n := cmp.Compare(string(a.Kind), string(b.Kind)); n != 0 {
			return n
		}
		// tiebreak on first location (spec conformance)
		aFile, aLine := firstLoc(a)
		bFile, bLine := firstLoc(b)
		if n := cmp.Compare(aFile, bFile); n != 0 {
			return n
		}
		return cmp.Compare(aLine, bLine)
	})

	return &Graph{nodes: nodes, edges: edges}
}

func firstLoc(e Edge) (string, int) {
	if len(e.Locations) == 0 {
		return "", 0
	}
	return e.Locations[0].File, e.Locations[0].Line
}

// Nodes returns a copy of the graph's node slice.
func (g *Graph) Nodes() []Node {
	out := make([]Node, len(g.nodes))
	copy(out, g.nodes)
	return out
}

// Edges returns a copy of the graph's edge slice.
func (g *Graph) Edges() []Edge {
	out := make([]Edge, len(g.edges))
	copy(out, g.edges)
	return out
}

// EdgesFrom returns all edges whose From field equals id.
func (g *Graph) EdgesFrom(id string) []Edge {
	var out []Edge
	for _, e := range g.edges {
		if e.From == id {
			out = append(out, e)
		}
	}
	return out
}

// EdgesTo returns all edges whose To field equals id.
func (g *Graph) EdgesTo(id string) []Edge {
	var out []Edge
	for _, e := range g.edges {
		if e.To == id {
			out = append(out, e)
		}
	}
	return out
}

// Cycles returns all strongly-connected components of size > 1 found in the
// dependency edges (imports, depends_on, uses_internal; excluding belongs_to).
// Each SCC is returned as a sorted slice of node IDs. The outer slice is sorted
// by the first element of each SCC for determinism.
//
// This is the shared detection primitive consumed by both CycleMetric and CycleRule
// to avoid duplicating Tarjan's algorithm.
func (g *Graph) Cycles() [][]string {
	// Build adjacency list from dependency edges only.
	adj := make(map[string][]string)
	nodeSet := make(map[string]struct{})
	for _, e := range g.edges {
		switch e.Kind {
		case EdgeKindImports, EdgeKindDependsOn, EdgeKindUsesInternal:
		default:
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
		nodeSet[e.From] = struct{}{}
		nodeSet[e.To] = struct{}{}
	}

	// Tarjan's SCC.
	idx := 0
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	var sccs [][]string

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = idx
		lowlink[v] = idx
		idx++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := indices[w]; !visited {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			if len(scc) > 1 {
				slices.Sort(scc)
				sccs = append(sccs, scc)
			}
		}
	}

	// Visit every node in sorted order for determinism.
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	slices.Sort(nodes)

	for _, v := range nodes {
		if _, visited := indices[v]; !visited {
			strongConnect(v)
		}
	}

	// Sort SCCs by their first element for a stable outer order.
	slices.SortFunc(sccs, func(a, b []string) int {
		return cmp.Compare(a[0], b[0])
	})

	return sccs
}
