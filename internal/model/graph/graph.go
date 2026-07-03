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
	Kind     NodeKind `json:"kind"`
	Path     string   `json:"path"`
	Language string   `json:"language"`
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
	// StrengthHint is a language-aware integration-strength signal set by an
	// extractor (e.g. a Python import of a PEP 8-private module → "intrusive").
	// classify honors it only as a fallback when config public/internal globs do
	// not decide, so an architect's explicit declaration always wins.
	StrengthHint string `json:"strength_hint,omitempty"`
}

// StrengthHintDTO marks a reference to a pure-data struct: exported, at least
// one field, only exported data fields, empty method set (Go type-info is the
// only source today). Unlike the other hint values it is not a
// coupling.Strength — it is context-dependent: across a config-declared public
// boundary the edge is the book's canonical integration Contract (the
// public-glob floor stands); without a declared boundary it is just a shared
// concrete type (model).
const StrengthHintDTO = "dto"

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
	LangRust       = "rust"
)

// CrateRoot maps a Rust crate's repo-relative source directory to its crate name.
// The Rust extractor (which alone knows cargo's package→manifest layout) emits one
// per workspace member so the filesystem-free core ring can resolve a .rs file path
// to its module key ("<crate>::<mod>") — the crate name is not derivable from the
// path alone. Dir is repo-relative and slash-separated ("" for a root crate).
type CrateRoot struct {
	Dir  string
	Name string
}

// GoModule maps a Go workspace member's module path to its ScanRoot-relative directory.
// The Go extractor emits one per loaded workspace member so downstream consumers
// (classify stage, Task 8 auto-registration) can resolve the member layout without
// re-reading the filesystem. Path is the Go module path (e.g. "example.com/a").
// RelDir is ScanRoot-relative and slash-separated ("." for the root member).
type GoModule struct {
	Path   string
	RelDir string
}

// Facts holds raw extractor output — nodes, edges, and unresolved counts —
// before deduplication and sorting.
type Facts struct {
	Nodes      []Node
	Edges      []Edge
	Language   string
	Unresolved int
	// CrateRoots carries the Rust workspace members' source dirs and names so the
	// core ring can map .rs files to module keys. Empty for non-Rust facts.
	CrateRoots []CrateRoot
	// GoModules carries the Go workspace members' module paths and ScanRoot-relative
	// dirs so the classify stage can auto-register them as modules (Task 8).
	// Empty for non-Go facts; a single-module repo still carries one entry.
	GoModules []GoModule
}

// Graph is a sealed, immutable dependency graph produced by Build.
// After construction no field is mutated; accessors return copies.
type Graph struct {
	nodes      []Node
	edges      []Edge
	crateRoots []CrateRoot
	goModules  []GoModule
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
			// Extractors stamp Language on every emitted node; backfill from the
			// enclosing Facts for callers that build Node values directly (e.g.
			// hand-rolled test graphs), so a Node's Language is never silently
			// empty when its source Facts declares one.
			if n.Language == "" {
				n.Language = f.Language
			}
			id := n.ID()
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				nodes = append(nodes, n)
			}
		}
	}

	// --- Collect crate roots (Rust) ---
	// Pure carry-through of the Rust extractor's package layout, deduped by Dir.
	// Sorted by Dir for deterministic longest-prefix matching downstream.
	var crateRoots []CrateRoot
	seenCrate := make(map[string]struct{})
	for _, f := range facts {
		for _, c := range f.CrateRoots {
			if _, ok := seenCrate[c.Dir]; ok {
				continue
			}
			seenCrate[c.Dir] = struct{}{}
			crateRoots = append(crateRoots, c)
		}
	}
	slices.SortFunc(crateRoots, func(a, b CrateRoot) int {
		return cmp.Compare(a.Dir, b.Dir)
	})

	// --- Collect Go modules (workspace members) ---
	// Pure carry-through of the Go extractor's workspace layout, deduped by Path.
	// Sorted by Path for determinism; Task 8 consumes this for module auto-registration.
	var goModules []GoModule
	seenGoMod := make(map[string]struct{})
	for _, f := range facts {
		for _, m := range f.GoModules {
			if _, ok := seenGoMod[m.Path]; ok {
				continue
			}
			seenGoMod[m.Path] = struct{}{}
			goModules = append(goModules, m)
		}
	}
	slices.SortFunc(goModules, func(a, b GoModule) int {
		return cmp.Compare(a.Path, b.Path)
	})

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
		prio := BuiltinConventions.Lookup(f.Language).Priority
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

	return &Graph{nodes: nodes, edges: edges, crateRoots: crateRoots, goModules: goModules}
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

// CrateRoots returns a copy of the Rust crate roots carried from extraction,
// sorted by Dir. Empty for graphs with no Rust facts.
func (g *Graph) CrateRoots() []CrateRoot {
	out := make([]CrateRoot, len(g.crateRoots))
	copy(out, g.crateRoots)
	return out
}

// GoModules returns a copy of the Go workspace members carried from extraction,
// sorted by Path. Empty for graphs with no Go facts. A single-module repo
// carries one entry; multi-member workspaces carry one per discovered member.
func (g *Graph) GoModules() []GoModule {
	out := make([]GoModule, len(g.goModules))
	copy(out, g.goModules)
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
