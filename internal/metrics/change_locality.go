package metrics

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
)

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

// Version returns "change_locality.v1".
func (m ChangeLocalityMetric) Version() string { return "change_locality.v1" }

// Calculate counts cross-module edges from changed files and the forward
// reach (distinct files reachable from the changed set).
func (m ChangeLocalityMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	def := "per-change drift: cross-module edges originating in changed files + forward " +
		"graph reach from changed nodes (delta mode only; report-only, never gates)"
	if len(in.ChangedFiles) == 0 || in.Graph == nil {
		return naCount(m.Name(), m.Version(), def)
	}

	changed := make(map[string]struct{}, len(in.ChangedFiles))
	for _, f := range in.ChangedFiles {
		changed[f] = struct{}{}
	}

	// Cross-module edges from changed files, judged by the classification's
	// distance dimension (same_module and unknown do not count).
	crossEdges := 0
	adjacency := make(map[string][]string)
	for _, e := range in.Graph.Edges() {
		adjacency[e.From] = append(adjacency[e.From], e.To)

		fromPath := graph.NodePath(e.From)
		if _, isChanged := changed[fromPath]; !isChanged {
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

	reach := forwardReach(adjacency, in.ChangedFiles)

	confidence := confidenceHigh
	if len(in.Classifications) == 0 {
		confidence = confidenceLow
	}

	return diagnostic.MetricResult{
		Name:  m.Name(),
		Value: float64(crossEdges),
		Display: fmt.Sprintf("%d cross-module edge(s) from %d changed file(s); forward reach %d file(s)",
			crossEdges, len(in.ChangedFiles), reach),
		Band:       bandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: def,
	}
}

// forwardReach BFS-walks the adjacency from every changed file node and
// returns the count of distinct reached nodes outside the changed set.
func forwardReach(adjacency map[string][]string, changedFiles []string) int {
	start := make([]string, 0, len(changedFiles))
	visited := make(map[string]struct{}, len(changedFiles))
	for _, f := range changedFiles {
		id := "file:" + f
		start = append(start, id)
		visited[id] = struct{}{}
	}

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
