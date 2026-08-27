// Package metricstest provides shared test helpers for metric family packages.
// Import this package from *_test.go files using package <pkg>_test.
//
// Fixtures are expressed in the relationship contract metrics actually consume.
// Assessment never sees the extractor graph, so neither do its tests.
package metricstest

import (
	"math"

	"github.com/alexei-led/archfit/internal/relationship"
)

// Node kinds carried by relationship node IDs.
const (
	NodeKindModule   = "module"
	NodeKindPackage  = "package"
	NodeKindFile     = "file"
	NodeKindExternal = "external"
)

// Language tags carried by relationship nodes and edges.
const (
	LangGo         = "go"
	LangRust       = "rust"
	LangTypeScript = "typescript"
	LangPython     = "python"
)

// Edge kinds carried by relationship edges.
const (
	EdgeKindBelongsTo    = "belongs_to"
	EdgeKindImports      = "imports"
	EdgeKindDependsOn    = "depends_on"
	EdgeKindUsesInternal = "uses_internal"
)

// Node is a fixture relationship participant. Its ID follows the contract's
// "kind:path" form, which NodePath and ModuleKey read.
type Node struct {
	Kind     string
	Path     string
	Language string
}

// ID returns the contract node ID for this fixture node.
func (n Node) ID() string { return n.Kind + ":" + n.Path }

// Edge is a fixture relationship between two node IDs.
type Edge struct {
	From      string
	To        string
	Kind      string
	Language  string
	Locations []relationship.Location
}

// Classification is the fixture classification applied to one edge. Unset
// dimensions become the contract's unknown values, so structure-only metrics
// can be exercised without inventing ordinals.
type Classification struct {
	Strength   relationship.Strength
	Distance   relationship.Distance
	Volatility relationship.Volatility
	Severity   relationship.Severity
}

// Index maps an edge key to its fixture classification.
type Index map[string]Classification

// Fixture holds a relationship fixture's participants and relationships before
// classification is applied.
type Fixture struct {
	Nodes []Node
	Edges []Edge
}

// NewFixture collects fixture nodes and edges.
func NewFixture(nodes []Node, edges []Edge) Fixture { return Fixture{Nodes: nodes, Edges: edges} }

// Classify applies an index to a fixture and returns the relationship set.
func Classify(f Fixture, idx Index) relationship.Set {
	return BuildRelationships(f.Nodes, f.Edges, idx)
}

// BuildRelationships assembles a relationship.Set fixture from nodes, edges,
// and an optional classification index. Missing classifications become unknown
// relationship facts.
func BuildRelationships(nodes []Node, edges []Edge, idx Index) relationship.Set {
	set := relationship.Set{
		Nodes: make([]relationship.Node, 0, len(nodes)),
		Edges: make([]relationship.Edge, 0, len(edges)),
	}
	for _, n := range nodes {
		id := n.ID()
		set.Nodes = append(set.Nodes, relationship.Node{
			ID: id, Path: n.Path, Kind: n.Kind, Language: n.Language,
			Module: relationship.ModuleKey(id), FirstParty: n.Kind != NodeKindExternal,
		})
	}
	for _, e := range edges {
		cl := Classification{Strength: relationship.StrengthUnknown, Distance: relationship.DistanceUnknown, Volatility: relationship.VolatilityUnknown}
		if found, ok := idx[ImportKeyForKind(e.From, e.To, e.Kind)]; ok {
			cl = found
		}
		set.Edges = append(set.Edges, relationship.Edge{
			FromID: e.From, ToID: e.To,
			FromPath: relationship.NodePath(e.From), ToPath: relationship.NodePath(e.To),
			FromModule: relationship.ModuleKey(e.From), ToModule: relationship.ModuleKey(e.To),
			Kind: e.Kind, Language: e.Language,
			Strength: cl.Strength, Distance: cl.Distance,
			Volatility: cl.Volatility, Severity: cl.Severity,
			Locations: e.Locations,
		})
	}
	return set
}

// ImportKey returns the classification index key for an imports edge.
func ImportKey(from, to string) string {
	return ImportKeyForKind(from, to, EdgeKindImports)
}

// ImportKeyForKind returns the classification index key for an edge kind.
func ImportKeyForKind(from, to, kind string) string {
	return from + "\x00" + to + "\x00" + kind
}

// ApproxEqual reports whether two float64 values are within 1e-9 of each other.
func ApproxEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
