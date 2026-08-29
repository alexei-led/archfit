// Package relationship defines the classified relationship result contract shared
// by the relationship stage and downstream assessment consumers.
package relationship

import (
	"cmp"
	"slices"
	"strings"
)

// Dependency edge kind literals considered by dependency metrics and cycle
// detection. These mirror the extractor graph's dependency edges without
// importing the raw graph package.
const (
	edgeKindImports      = "imports"
	edgeKindDependsOn    = "depends_on"
	edgeKindUsesInternal = "uses_internal"
)

// Strength classifies how a dependency is expressed at an API boundary.
type Strength string

// Strength constants name the relationship integration-strength facts.
const (
	StrengthContract   Strength = "contract"
	StrengthIntrusive  Strength = "intrusive"
	StrengthModel      Strength = "model"
	StrengthFunctional Strength = "functional"
	StrengthSymmetric  Strength = "symmetric"
	StrengthUnknown    Strength = "unknown"
)

// Distance measures how far apart two modules are in the ownership hierarchy.
type Distance string

// Distance constants name the measured ownership/deployment gap facts.
const (
	DistanceSameModule           Distance = "same_module"
	DistanceCrossModuleSameOwner Distance = "cross_module_same_owner"
	DistanceCrossModuleDiffOwner Distance = "cross_module_different_owner"
	DistanceCrossDeployUnit      Distance = "cross_deploy_unit"
	DistanceExternal             Distance = "declared_external"
	DistanceUnknown              Distance = "unknown"
)

// Volatility classifies how likely a module's API is to change.
type Volatility string

// Volatility constants name the resolved target volatility facts.
const (
	VolatilityFrozen     Volatility = "frozen"
	VolatilityLow        Volatility = "low"
	VolatilityMedium     Volatility = "medium"
	VolatilityHigh       Volatility = "high"
	VolatilityUndeclared Volatility = "undeclared"
	VolatilityUnknown    Volatility = "unknown"
)

// VolatilityProvenance counts modules by the source of their volatility label.
// It is relationship evidence and is projected into assessment/report values by
// the application layer.
type VolatilityProvenance struct {
	Declared   int `json:"declared"`
	Inherited  int `json:"inherited"`
	Cascade    int `json:"cascade"`
	Undeclared int `json:"undeclared"`
}

// Explicitness records whether a relationship uses a declared contract. The
// levels themselves are named by the classifier that assigns them; this
// contract only carries the assigned value.
type Explicitness string

// Severity expresses relationship risk. Empty means no relationship finding.
type Severity string

// Severity constants name relationship risk levels.
const (
	SeverityNone     Severity = ""
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Location is a source-code position attached to a relationship edge.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Provenance records compact report-only sources that explain how a classified
// relationship fact was derived. It carries facts, not assessment findings or
// metric state.
type Provenance struct {
	ClassificationKey       string
	DistanceBasis           string
	StrengthFromLLM         bool
	StrengthFromNonHighLLM  bool
	StrengthFromConnascence bool
	ConnascenceKinds        []string
	CloneLocationCount      int
}

// Score is the relationship-owned score value produced by classification.
type Score struct {
	Scored       bool
	Balance      int
	Value        int
	Band         Severity
	Reason       string
	CheapestMove string
	Breakdown    ScoreBreakdown
}

// ScoreBreakdown records the score inputs needed by report projections.
type ScoreBreakdown struct {
	StrengthValue   int
	DistanceValue   int
	VolatilityValue int
	Modularity      int
	VolDiscount     int
}

// ConnascenceEvidence is a compact report-only static signal on an edge.
type ConnascenceEvidence struct {
	Kind   string
	Source string
	Detail string
}

// Classification is a relationship-owned value object. It replaces the
// classifier index at the relationship boundary without exposing that index.
type Classification struct {
	Explicitness        Explicitness
	ContractRecommended bool
	Score               Score
	DistanceBasis       string
	CloneLocations      []Location
	Connascence         []ConnascenceEvidence
}

// Node is a relationship participant. Module is the resolved declared-module
// identity used by assessment metrics; it is empty when the node is explicitly
// outside the module map. BoundaryClassified distinguishes that explicit
// outside-map result from a node a projection failed to classify. FirstParty is
// false for declared external graph nodes.
type Node struct {
	ID                 string
	Path               string
	Kind               string
	Language           string
	Module             string
	FirstParty         bool
	BoundaryClassified bool
}

// Edge is one classified directed relationship. It contains only the facts the
// assessment seam needs: endpoint identity, configured module labels,
// classification, risk, and provenance.
type Edge struct {
	FromID              string
	ToID                string
	FromPath            string
	ToPath              string
	FromModule          string
	ToModule            string
	FromLayer           string
	ToLayer             string
	StructureClassified bool
	Kind                string
	Language            string
	Strength            Strength
	Distance            Distance
	Volatility          Volatility
	Severity            Severity
	Locations           []Location
	Provenance          Provenance
	Classified          Classification
}

// CloneOnlyPair is relationship-owned duplicated-knowledge provenance.
type CloneOnlyPair struct {
	FromModule string
	ToModule   string
	FromPath   string
	ToPath     string
	Strength   Strength
	Distance   Distance
	Volatility Volatility
	Severity   Severity
	Locations  []Location
	Classified Classification
}

// Set is the relationship-owned output of classification. It is deliberately
// narrower than the extractor graph: nodes and classified edges only.
type Set struct {
	Nodes []Node
	Edges []Edge
}

// Empty reports whether no relationship evidence was provided.
func (s Set) Empty() bool { return len(s.Nodes) == 0 && len(s.Edges) == 0 }

// DependencyEdges returns the dependency-like edges considered by assessment
// metrics. The returned slice is a copy to preserve Set immutability by convention.
func (s Set) DependencyEdges() []Edge {
	out := make([]Edge, 0, len(s.Edges))
	for _, e := range s.Edges {
		if e.IsDependency() {
			out = append(out, e)
		}
	}
	return out
}

// IsDependency reports whether the edge kind participates in dependency metrics.
func (e Edge) IsDependency() bool {
	switch e.Kind {
	case edgeKindImports, edgeKindDependsOn, edgeKindUsesInternal:
		return true
	default:
		return false
	}
}

// BoundaryClassified reports whether Strength takes a stance on boundary respect.
func (e Edge) BoundaryClassified() bool {
	switch e.Strength {
	case StrengthContract, StrengthIntrusive:
		return true
	default:
		return false
	}
}

// CrossBoundary reports whether the relationship crosses a measured first-party
// module boundary and is therefore relevant to encapsulation.
func (e Edge) CrossBoundary() bool {
	return e.Distance != DistanceSameModule && e.Distance != DistanceUnknown && e.Distance != DistanceExternal
}

// VolatilityResolved reports whether v is a concrete level assessment can act on.
func VolatilityResolved(v Volatility) bool {
	return v == VolatilityFrozen || v == VolatilityLow || v == VolatilityMedium || v == VolatilityHigh
}

// NodePath extracts the path component from a node ID of the form "kind:path".
func NodePath(id string) string {
	_, after, ok := strings.Cut(id, ":")
	if ok {
		return after
	}
	return id
}

// ModuleKey collapses a graph node id to the structural module unit used by the
// blast-radius metric. It matches the pre-contract graph collapse semantics.
func ModuleKey(nodeID string) string {
	path := NodePath(nodeID)
	if strings.HasSuffix(path, ".go") {
		if j := strings.LastIndexByte(path, '/'); j >= 0 {
			return path[:j]
		}
	}
	return path
}

// FindByFindingEdge returns the classified relationship matching a finding edge
// (which carries stripped endpoint paths).
func (s Set) FindByFindingEdge(fromPath, toPath, kind string) (Edge, bool) {
	for _, e := range s.Edges {
		if e.FromPath == fromPath && e.ToPath == toPath && e.Kind == kind {
			return e, true
		}
	}
	return Edge{}, false
}

// Cycles returns all strongly-connected components of size > 1 found in the
// dependency edges (imports, depends_on, uses_internal; excluding belongs_to).
// Each SCC is returned as a sorted slice of node IDs. The outer slice is sorted
// by the first element of each SCC for determinism.
//
// This is the relationship-owned cycle detection primitive consumed by both the
// cycle metric and the cycle rule, mirroring the pre-contract graph.Cycles
// traversal over the narrower relationship contract.
func (s Set) Cycles() [][]string {
	// Build adjacency list from dependency edges only.
	adj := make(map[string][]string)
	nodeSet := make(map[string]struct{})
	for _, e := range s.Edges {
		if !e.IsDependency() {
			continue
		}
		adj[e.FromID] = append(adj[e.FromID], e.ToID)
		nodeSet[e.FromID] = struct{}{}
		nodeSet[e.ToID] = struct{}{}
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
