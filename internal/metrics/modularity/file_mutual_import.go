package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// FileMutualImportMetric detects file-level mutual imports: pairs of source files
// that import each other directly (A imports B and B imports A). This is a
// file-granularity signal that the module-level cycle metric misses when both files
// sit inside the same module — module-level SCC detection cannot see intra-module
// file cycles.
//
// Scope: languages that emit file→file import edges (TypeScript). Go emits
// file→package edges and Python emits module→module edges — neither produces
// file→file cycles, so this metric naturally emits 0 / n/a for those graphs.
//
// Report-only — Band: BandInformational, never gates.
// n/a when the graph has no file-kind nodes.
type FileMutualImportMetric struct{}

// Name returns "file_mutual_import".
func (m FileMutualImportMetric) Name() string { return "file_mutual_import" }

// Version returns "file_mutual_import.v1".
func (m FileMutualImportMetric) Version() string { return "file_mutual_import.v1" }

type mutualFilePair struct {
	a, b string // node paths (not IDs), sorted a < b for dedup
}

// Calculate finds file→file mutual-import cycles using g.Cycles() (Tarjan SCC).
// It keeps only SCCs where every node has NodeKindFile. Each such SCC of size ≥ 2
// is decomposed into all mutual-import pairs within it (the SCC may be larger than 2
// when three or more files form a cycle together). The returned count is the number
// of distinct mutual-import pairs found. n/a when the graph is nil or has no file nodes.
func (m FileMutualImportMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "file-level mutual imports: pairs of source files that import each other (file→file cycles; TypeScript only — Go/Python have no file→file edges)"

	g := in.Graph
	if g == nil {
		return result.NACount(m.Name(), m.Version(), def)
	}

	// Build an index of node IDs that are file-kind for fast lookup.
	fileNodes := make(map[string]struct{})
	for _, n := range g.Nodes() {
		if n.Kind == graph.NodeKindFile {
			fileNodes[n.ID()] = struct{}{}
		}
	}
	if len(fileNodes) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}

	// g.Cycles() returns SCCs of size > 1 across all dependency edges.
	// We filter for SCCs where every member is a file node.
	sccs := g.Cycles()
	pairs := make(map[[2]string]struct{})
	for _, scc := range sccs {
		if !allFileNodes(scc, fileNodes) {
			continue
		}
		// Decompose the SCC into all ordered pairs — every node in the SCC is
		// mutually reachable from every other, so each unordered pair is a mutual import.
		for i := 0; i < len(scc); i++ {
			for j := i + 1; j < len(scc); j++ {
				a, b := graph.NodePath(scc[i]), graph.NodePath(scc[j])
				if a > b {
					a, b = b, a
				}
				pairs[[2]string{a, b}] = struct{}{}
			}
		}
	}

	if len(pairs) == 0 {
		return diagnostic.MetricResult{
			Name:       m.Name(),
			Value:      0,
			Display:    "0 file mutual import pair(s)",
			Band:       result.BandInformational,
			Confidence: result.ConfidenceHigh,
			Version:    m.Version(),
			Mode:       result.ModeCount,
			Definition: def,
		}
	}

	sorted := make([]mutualFilePair, 0, len(pairs))
	for k := range pairs {
		sorted = append(sorted, mutualFilePair{a: k[0], b: k[1]})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].a != sorted[j].a {
			return sorted[i].a < sorted[j].a
		}
		return sorted[i].b < sorted[j].b
	})

	confidence := result.ConfidenceHigh
	if len(fileNodes) < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(pairs)),
		Display:    fileMutualImportDisplay(sorted),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

// allFileNodes reports whether every node ID in scc is in the fileNodes set.
func allFileNodes(scc []string, fileNodes map[string]struct{}) bool {
	for _, id := range scc {
		if _, ok := fileNodes[id]; !ok {
			return false
		}
	}
	return true
}

func fileMutualImportDisplay(pairs []mutualFilePair) string {
	if len(pairs) == 0 {
		return "0 file mutual import pair(s)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d file mutual import pair(s): ", len(pairs))
	for i, p := range pairs {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(pairs)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s ↔ %s", p.a, p.b)
	}
	return b.String()
}
