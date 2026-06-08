package graph

import (
	"slices"
	"testing"
)

// Test fixture constants — promoted to avoid goconst violations.
const (
	pathPkgA    = "pkg/a/a.go"
	pathPkgB    = "pkg/b/b.go"
	idFilePkgA  = "file:pkg/a/a.go"
	idFilePkgB  = "file:pkg/b/b.go"
	pathAGo     = "a.go"
	idFileAGo   = "file:a.go"
	idFileBGo   = "file:b.go"
	idFileCGo   = "file:c.go"
	idFileZGo   = "file:z.go"
	confHigh    = "high"
	pathMutated = "mutated"
	idMutated   = "mutated"
	idModuleA   = "module:a"
	idModuleB   = "module:b"
)

// nodeIDs extracts IDs from a node slice for easy comparison.
func nodeIDs(nodes []Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID()
	}
	return ids
}

// edgeKeys extracts (from,to,kind) strings from an edge slice.
func edgeKeys(edges []Edge) []string {
	keys := make([]string, len(edges))
	for i, e := range edges {
		keys[i] = e.From + "→" + e.To + ":" + string(e.Kind)
	}
	return keys
}

func TestNodeID(t *testing.T) {
	n := Node{Kind: NodeKindFile, Path: pathPkgA}
	if got := n.ID(); got != idFilePkgA {
		t.Errorf("ID() = %q, want %q", got, idFilePkgA)
	}
}

func TestBuild_DeduplicatesNodes(t *testing.T) {
	facts := []Facts{
		{
			Nodes: []Node{
				{Kind: NodeKindFile, Path: pathPkgA},
				{Kind: NodeKindModule, Path: "a"},
			},
			Language: LangGo,
		},
		{
			Nodes: []Node{
				{Kind: NodeKindFile, Path: pathPkgA}, // duplicate
				{Kind: NodeKindFile, Path: pathPkgB},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	ids := nodeIDs(g.Nodes())

	want := []string{idFilePkgA, idFilePkgB, idModuleA}
	if !slices.Equal(ids, want) {
		t.Errorf("Nodes() IDs = %v, want %v", ids, want)
	}
}

func TestBuild_NodesSortedByID(t *testing.T) {
	facts := []Facts{
		{
			Nodes: []Node{
				{Kind: NodeKindModule, Path: "z"},
				{Kind: NodeKindFile, Path: pathAGo},
				{Kind: NodeKindModule, Path: "a"},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	ids := nodeIDs(g.Nodes())
	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Errorf("nodes not sorted at [%d,%d]: %q >= %q", i-1, i, ids[i-1], ids[i])
		}
	}
}

func TestBuild_DeduplicatesEdgesByCanonicalKey(t *testing.T) {
	loc1 := Location{File: pathPkgA, Line: 5}
	loc2 := Location{File: pathPkgA, Line: 7}

	facts := []Facts{
		{
			Edges: []Edge{
				{From: idFilePkgA, To: idFilePkgB, Kind: EdgeKindImports,
					Language: LangGo, Confidence: confHigh, Locations: []Location{loc1}},
			},
			Language: LangGo,
		},
		{
			Edges: []Edge{
				// Same canonical key (from, to, kind) — locations should merge.
				{From: idFilePkgA, To: idFilePkgB, Kind: EdgeKindImports,
					Language: LangGo, Confidence: confHigh, Locations: []Location{loc2}},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("want 1 edge after dedup, got %d", len(edges))
	}
	if len(edges[0].Locations) != 2 {
		t.Errorf("want 2 merged locations, got %d", len(edges[0].Locations))
	}
}

func TestBuild_MergesLocations_NoDuplicates(t *testing.T) {
	sharedLoc := Location{File: pathPkgA, Line: 5}

	facts := []Facts{
		{
			Edges: []Edge{
				{From: idFilePkgA, To: idFilePkgB, Kind: EdgeKindImports,
					Language: LangGo, Locations: []Location{sharedLoc}},
			},
			Language: LangGo,
		},
		{
			Edges: []Edge{
				// Same canonical key, same location — should not duplicate.
				{From: idFilePkgA, To: idFilePkgB, Kind: EdgeKindImports,
					Language: LangGo, Locations: []Location{sharedLoc}},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
	if len(edges[0].Locations) != 1 {
		t.Errorf("want 1 location (no dups), got %d", len(edges[0].Locations))
	}
}

func TestBuild_MultiSourcePriority_GoWinsOverTypeScript(t *testing.T) {
	facts := []Facts{
		{
			Edges: []Edge{
				{From: "file:src/a.ts", To: "file:src/b.ts", Kind: EdgeKindImports,
					Language: LangTypeScript, Confidence: confHigh},
			},
			Language: LangTypeScript,
		},
		{
			Edges: []Edge{
				// Same canonical key from Go (synthetic cross-language collision).
				{From: "file:src/a.ts", To: "file:src/b.ts", Kind: EdgeKindImports,
					Language: LangGo, Confidence: confHigh},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
	if edges[0].Language != LangGo {
		t.Errorf("expected winning language = go, got %q", edges[0].Language)
	}
}

func TestBuild_MultiSourcePriority_TypeScriptWinsOverPython(t *testing.T) {
	facts := []Facts{
		{
			Edges: []Edge{
				{From: idModuleA, To: idModuleB, Kind: EdgeKindImports,
					Language: LangPython, Confidence: confHigh},
			},
			Language: LangPython,
		},
		{
			Edges: []Edge{
				{From: idModuleA, To: idModuleB, Kind: EdgeKindImports,
					Language: LangTypeScript, Confidence: confHigh},
			},
			Language: LangTypeScript,
		},
	}

	g := Build(facts)
	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
	if edges[0].Language != LangTypeScript {
		t.Errorf("expected winning language = typescript, got %q", edges[0].Language)
	}
}

func TestBuild_EdgesSorted(t *testing.T) {
	facts := []Facts{
		{
			Edges: []Edge{
				{From: idFileZGo, To: idFileAGo, Kind: EdgeKindImports},
				{From: idFileAGo, To: idFileBGo, Kind: EdgeKindImports},
				{From: idFileAGo, To: idFileAGo, Kind: EdgeKindBelongsTo},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	edges := g.Edges()
	for i := 1; i < len(edges); i++ {
		prev := edges[i-1]
		curr := edges[i]
		pk := prev.From + "|" + prev.To + "|" + string(prev.Kind)
		ck := curr.From + "|" + curr.To + "|" + string(curr.Kind)
		if pk > ck {
			t.Errorf("edges not sorted at [%d,%d]: %q > %q", i-1, i, pk, ck)
		}
	}
}

func TestBuild_LocationsSortedWithinEdge(t *testing.T) {
	facts := []Facts{
		{
			Edges: []Edge{
				{From: idFileAGo, To: idFileBGo, Kind: EdgeKindImports,
					Locations: []Location{
						{File: pathAGo, Line: 10},
						{File: pathAGo, Line: 3},
						{File: pathAGo, Line: 7},
					}},
			},
			Language: LangGo,
		},
	}

	g := Build(facts)
	locs := g.Edges()[0].Locations
	for i := 1; i < len(locs); i++ {
		if locs[i-1].Line >= locs[i].Line {
			t.Errorf("locations not sorted at [%d,%d]: line %d >= %d", i-1, i, locs[i-1].Line, locs[i].Line)
		}
	}
}

func TestBuild_EmptyFacts(t *testing.T) {
	g := Build(nil)
	if len(g.Nodes()) != 0 {
		t.Errorf("want 0 nodes, got %d", len(g.Nodes()))
	}
	if len(g.Edges()) != 0 {
		t.Errorf("want 0 edges, got %d", len(g.Edges()))
	}
}

func TestGraph_AccessorsCopyNotSlice(t *testing.T) {
	facts := []Facts{
		{
			Nodes: []Node{{Kind: NodeKindFile, Path: pathAGo}},
			Edges: []Edge{{From: idFileAGo, To: idFileBGo, Kind: EdgeKindImports}},
		},
	}
	g := Build(facts)

	// Mutate the returned slices — the graph's internal state must not change.
	nodes1 := g.Nodes()
	nodes1[0] = Node{Kind: NodeKindExternal, Path: pathMutated}
	nodes2 := g.Nodes()
	if nodes2[0].Path == pathMutated {
		t.Error("Nodes() returned a live reference — mutation affected internal state")
	}

	edges1 := g.Edges()
	edges1[0].From = idMutated
	edges2 := g.Edges()
	if edges2[0].From == idMutated {
		t.Error("Edges() returned a live reference — mutation affected internal state")
	}
}

func TestGraph_EdgesFrom(t *testing.T) {
	facts := []Facts{
		{
			Edges: []Edge{
				{From: idFileAGo, To: idFileBGo, Kind: EdgeKindImports},
				{From: idFileAGo, To: idFileCGo, Kind: EdgeKindImports},
				{From: "file:x.go", To: idFileBGo, Kind: EdgeKindImports},
			},
			Language: LangGo,
		},
	}
	g := Build(facts)

	got := g.EdgesFrom(idFileAGo)
	if len(got) != 2 {
		t.Errorf("EdgesFrom(%s) = %d edges, want 2", idFileAGo, len(got))
	}
	for _, e := range got {
		if e.From != idFileAGo {
			t.Errorf("EdgesFrom returned edge with From=%q", e.From)
		}
	}
}

func TestGraph_EdgesTo(t *testing.T) {
	facts := []Facts{
		{
			Edges: []Edge{
				{From: idFileAGo, To: idFileBGo, Kind: EdgeKindImports},
				{From: "file:x.go", To: idFileBGo, Kind: EdgeKindImports},
				{From: idFileAGo, To: idFileCGo, Kind: EdgeKindImports},
			},
			Language: LangGo,
		},
	}
	g := Build(facts)

	got := g.EdgesTo(idFileBGo)
	if len(got) != 2 {
		t.Errorf("EdgesTo(%s) = %d edges, want 2", idFileBGo, len(got))
	}
	for _, e := range got {
		if e.To != idFileBGo {
			t.Errorf("EdgesTo returned edge with To=%q", e.To)
		}
	}
}

func TestBuild_DeterministicAcrossRuns(t *testing.T) {
	facts := []Facts{
		{
			Nodes: []Node{
				{Kind: NodeKindFile, Path: "c.go"},
				{Kind: NodeKindFile, Path: pathAGo},
			},
			Edges: []Edge{
				{From: idFileCGo, To: idFileAGo, Kind: EdgeKindImports},
				{From: idFileAGo, To: idFileBGo, Kind: EdgeKindImports},
			},
			Language: LangGo,
		},
	}

	g1 := Build(facts)
	g2 := Build(facts)

	ids1 := nodeIDs(g1.Nodes())
	ids2 := nodeIDs(g2.Nodes())
	if !slices.Equal(ids1, ids2) {
		t.Errorf("node order differs between runs: %v vs %v", ids1, ids2)
	}

	keys1 := edgeKeys(g1.Edges())
	keys2 := edgeKeys(g2.Edges())
	if !slices.Equal(keys1, keys2) {
		t.Errorf("edge order differs between runs: %v vs %v", keys1, keys2)
	}
}
