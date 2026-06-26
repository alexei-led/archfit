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
// that import each other directly (A imports B AND B imports A). This is a
// file-granularity signal that the module-level cycle metric misses when both files
// sit inside the same module — module-level SCC detection cannot see intra-module
// file cycles.
//
// Detection uses actual bidirectional edges: for each file→file dependency edge A→B
// (EdgeKindImports, EdgeKindDependsOn, EdgeKindUsesInternal — mirrors g.Cycles()),
// the metric checks whether B→A also exists. Only genuinely bidirectional pairs
// {A,B} are counted. A 3-node cycle A→B→C→A has zero bidirectional pairs because
// no two nodes have edges in both directions.
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

// Calculate finds file→file mutual-import pairs by checking for genuinely
// bidirectional edges: for each dependency edge A→B where both A and B are file-kind
// nodes (EdgeKindImports, EdgeKindDependsOn, EdgeKindUsesInternal), it checks whether
// B→A also exists. Each unordered {A,B} pair is counted once. A pure cycle
// A→B→C→A has zero bidirectional pairs — only actual A↔B double-edges qualify.
// n/a when the graph is nil or has no file nodes.
func (m FileMutualImportMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "file-level mutual imports: pairs of source files that import each other (file→file bidirectional edges; TypeScript only — Go/Python have no file→file edges)"

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

	// Build a set of all file→file dependency edges for O(1) reverse-edge lookup.
	// Mirror the same edge-kind set used by g.Cycles() so TS uses_internal edges
	// (and any future depends_on file→file edges) are not missed.
	fileEdges := make(map[[2]string]struct{})
	for _, e := range g.Edges() {
		switch e.Kind {
		case graph.EdgeKindImports, graph.EdgeKindDependsOn, graph.EdgeKindUsesInternal:
		default:
			continue
		}
		if _, ok := fileNodes[e.From]; !ok {
			continue
		}
		if _, ok := fileNodes[e.To]; !ok {
			continue
		}
		fileEdges[[2]string{e.From, e.To}] = struct{}{}
	}

	// For each A→B file edge, check B→A; record each unordered {pathA, pathB} once.
	pairs := make(map[[2]string]struct{})
	for e := range fileEdges {
		from, to := e[0], e[1]
		if from == to {
			continue // skip self-loops
		}
		if _, rev := fileEdges[[2]string{to, from}]; !rev {
			continue
		}
		// Canonical key: sorted so A↔B and B↔A produce the same entry.
		pathFrom, pathTo := graph.NodePath(from), graph.NodePath(to)
		if pathFrom > pathTo {
			pathFrom, pathTo = pathTo, pathFrom
		}
		pairs[[2]string{pathFrom, pathTo}] = struct{}{}
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
