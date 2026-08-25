// Package metricstest provides shared test helpers for metric family packages.
// Import this package from *_test.go files using package <pkg>_test.
package metricstest

import (
	"math"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// BuildGraph builds a graph from a single Facts value for test convenience.
func BuildGraph(nodes []graph.Node, edges []graph.Edge) *graph.Graph {
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: edges}})
}

// BuildRelationships builds a relationship Set from graph test values and an
// optional coupling index. Missing classifications become unknown relationship
// facts so tests can exercise structure-only metrics without mocks.
func BuildRelationships(nodes []graph.Node, edges []graph.Edge, idx coupling.Index) relationship.Set {
	return BuildRelationshipsFromGraph(BuildGraph(nodes, edges), idx)
}

// BuildRelationshipsFromGraph projects a real graph value into the relationship
// contract used by metrics tests.
func BuildRelationshipsFromGraph(g *graph.Graph, idx coupling.Index) relationship.Set {
	set := relationship.Set{
		Nodes: make([]relationship.Node, 0, len(g.Nodes())),
		Edges: make([]relationship.Edge, 0, len(g.Edges())),
	}
	for _, n := range g.Nodes() {
		id := n.ID()
		set.Nodes = append(set.Nodes, relationship.Node{
			ID: id, Path: n.Path, Kind: string(n.Kind), Language: n.Language,
			Module: relationship.ModuleKey(id), FirstParty: n.Kind != graph.NodeKindExternal,
		})
	}
	for _, e := range g.Edges() {
		key := ImportKeyForKind(e.From, e.To, e.Kind)
		cl := coupling.Classification{Strength: coupling.StrengthUnknown, Distance: coupling.DistanceUnknown, Volatility: coupling.VolatilityUnknown}
		if idx != nil {
			if found, ok := idx[key]; ok {
				cl = found
			}
		}
		set.Edges = append(set.Edges, relationship.Edge{
			FromID: e.From, ToID: e.To, FromPath: graph.NodePath(e.From), ToPath: graph.NodePath(e.To),
			FromModule: relationship.ModuleKey(e.From), ToModule: relationship.ModuleKey(e.To),
			Kind: string(e.Kind), Language: e.Language,
			Strength: cl.Strength, Distance: cl.Distance,
			Volatility: cl.Volatility, Severity: cl.Severity,
			Locations: relationshipTestLocations(e.Locations),
		})
	}
	return set
}

func relationshipTestLocations(in []graph.Location) []relationship.Location {
	if len(in) == 0 {
		return nil
	}
	out := make([]relationship.Location, 0, len(in))
	for _, loc := range in {
		out = append(out, relationship.Location{File: loc.File, Line: loc.Line})
	}
	return out
}

// ImportKey returns the coupling index key for an imports edge.
func ImportKey(from, to string) string {
	return ImportKeyForKind(from, to, graph.EdgeKindImports)
}

// ImportKeyForKind returns the coupling index key for an edge kind.
func ImportKeyForKind(from, to string, kind graph.EdgeKind) string {
	return from + "\x00" + to + "\x00" + string(kind)
}

// ApproxEqual reports whether two float64 values are within 1e-9 of each other.
func ApproxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
