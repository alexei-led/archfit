package boundary

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// ---------------------------------------------------------------------------
// ChangeLocalityMetric (change_locality.v2)
// ---------------------------------------------------------------------------

// ChangeLocalityMetric is the per-change drift signal (spec §10.4): in delta
// mode (--base <ref>) it reports how far THIS change reaches beyond its own
// modules — the count of cross-module edges originating in changed files,
// plus the forward graph reach from the changed nodes.
//
// Report-only (band: info): the new_cross_module_dependency RULE is the gate
// for unreviewed cross-module edges; this metric quantifies the change's
// blast surface for the agent feedback loop. n/a in full mode (no diff to
// localize) — never a false zero.
type ChangeLocalityMetric struct{}

// Name returns "change_locality".
func (m ChangeLocalityMetric) Name() string { return "change_locality" }

// Version returns "change_locality.v2".
func (m ChangeLocalityMetric) Version() string { return "change_locality.v2" }

// Calculate counts cross-module edges from changed files and the forward
// reach (distinct files reachable from the changed set).
func (m ChangeLocalityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	def := "per-change drift: cross-module edges originating in changed files + forward " +
		"graph reach from changed nodes (delta mode only; report-only, never gates)"
	if len(in.ChangedFiles) == 0 || in.Graph == nil {
		return result.NACount(m.Name(), m.Version(), def)
	}

	changed := make(map[string]struct{}, len(in.ChangedFiles))
	for _, f := range in.ChangedFiles {
		changed[f] = struct{}{}
	}

	// Map changed files to graph node IDs across every extractor's node-ID
	// scheme (Go/TS "file:", Python "module:") so cross-edge and reach counts
	// work for all languages. Matching the raw "file:"+path form here produced a
	// false zero for Python, whose IDs are dotted "module:" names.
	changedNodes := changedNodeIDs(in.Graph, changed)

	// Cross-module edges from changed files, judged by the classification's
	// distance dimension (same_module and unknown do not count).
	crossEdges := 0
	adjacency := make(map[string][]string)
	for _, e := range in.Graph.Edges() {
		adjacency[e.From] = append(adjacency[e.From], e.To)

		if _, isChanged := changedNodes[e.From]; !isChanged {
			continue
		}
		cl, ok := in.Classifications[e.From+"\x00"+e.To+"\x00"+string(e.Kind)]
		if !ok {
			continue
		}
		if cl.Distance == coupling.DistanceSameModule ||
			cl.Distance == coupling.DistanceUnknown ||
			cl.Distance == "" {
			continue
		}
		crossEdges++
	}

	reach := forwardReach(adjacency, changedNodes)

	confidence := result.ConfidenceHigh
	if len(in.Classifications) == 0 {
		confidence = result.ConfidenceLow
	}

	return diagnostic.MetricResult{
		Name:  m.Name(),
		Value: float64(crossEdges),
		Display: fmt.Sprintf("%d cross-module edge(s) from %d changed file(s); forward reach %d file(s)",
			crossEdges, len(in.ChangedFiles), reach),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

// changedNodeIDs returns the set of dependency-graph node IDs whose source file
// is in the changed set. It bridges every extractor's node-ID scheme to the
// repo-relative file paths git reports:
//   - file nodes (Go, TypeScript): the node path IS the file path.
//   - module nodes (Python): the dotted module path maps to file candidates
//     "<a/b/c>.py", "<a/b/c>.pyi", and "<a/b/c>/__init__.py".
//
// Go package nodes and repo/external nodes do not map to a single changed source
// file and are skipped — Go changes enter through their file nodes.
//
// src-layout: a Python module's dotted name drops its source-root prefix
// ("ccgram.bootstrap" → "ccgram/bootstrap.py") and will not equal the
// repo-relative changed path ("src/ccgram/bootstrap.py"). We additionally match
// each changed file with its single leading source-root segment stripped
// ("src/ccgram/bootstrap.py" → "ccgram/bootstrap.py"), kept only while it stays
// multi-segment so a stripped path can never collapse to a bare basename and
// over-match an unrelated top-level module.
//
// Ceiling: only a single-level source root is stripped; a two-level root
// (packages/x/src/...) still will not match. Acceptable for a report-only
// signal. The stripped alias applies to module nodes only — file nodes (Go, TS)
// already carry the full repo-relative path and match exactly.
func changedNodeIDs(g *graph.Graph, changed map[string]struct{}) map[string]struct{} {
	rootStripped := make(map[string]struct{})
	for f := range changed {
		if _, rest, found := strings.Cut(f, "/"); found && strings.Contains(rest, "/") {
			rootStripped[rest] = struct{}{}
		}
	}

	out := make(map[string]struct{})
	for _, n := range g.Nodes() {
		for _, cand := range nodeFileCandidates(n) {
			if _, ok := changed[cand]; ok {
				out[n.ID()] = struct{}{}
				break
			}
			if n.Kind == graph.NodeKindModule {
				if _, ok := rootStripped[cand]; ok {
					out[n.ID()] = struct{}{}
					break
				}
			}
		}
	}
	return out
}

// nodeFileCandidates returns the repo-relative source-file path(s) a node could
// have been built from, per its kind. Returns nil for nodes that do not map to a
// single changed source file (Go package, repo, external).
func nodeFileCandidates(n graph.Node) []string {
	switch n.Kind {
	case graph.NodeKindFile:
		return []string{n.Path}
	case graph.NodeKindModule:
		// Python: dotted module → slash path; the file is "<path>.py", a typed
		// stub "<path>.pyi", or a package's "<path>/__init__.py".
		slashed := strings.ReplaceAll(n.Path, ".", "/")
		return []string{slashed + ".py", slashed + ".pyi", slashed + "/__init__.py"}
	default:
		return nil
	}
}

// forwardReach BFS-walks the adjacency from every changed node and returns the
// count of distinct reached nodes outside the changed set. Start node IDs are
// the graph nodes that map to changed files (see changedNodeIDs), so the walk
// works regardless of the extractor's node-ID scheme.
func forwardReach(adjacency map[string][]string, changedNodes map[string]struct{}) int {
	start := make([]string, 0, len(changedNodes))
	visited := make(map[string]struct{}, len(changedNodes))
	for id := range changedNodes {
		start = append(start, id)
		visited[id] = struct{}{}
	}
	// Sort the seed set so the BFS queue is built in a deterministic order
	// (the reach count is order-independent; this guards against any future
	// order-sensitive change).
	slices.Sort(start)

	reach := 0
	queue := start
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[node] {
			if _, seen := visited[next]; seen {
				continue
			}
			visited[next] = struct{}{}
			reach++
			queue = append(queue, next)
		}
	}
	return reach
}
