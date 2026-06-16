// Package metricstest provides shared test helpers for metric family packages.
// Import this package from *_test.go files using package <pkg>_test.
package metricstest

import (
	"math"

	"github.com/alexei-led/archfit/internal/model/graph"
)

// BuildGraph builds a graph from a single Facts value for test convenience.
func BuildGraph(nodes []graph.Node, edges []graph.Edge) *graph.Graph {
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: edges}})
}

// ImportKey returns the coupling index key for an imports edge.
func ImportKey(from, to string) string {
	return from + "\x00" + to + "\x00" + string(graph.EdgeKindImports)
}

// ApproxEqual reports whether two float64 values are within 1e-9 of each other.
func ApproxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
