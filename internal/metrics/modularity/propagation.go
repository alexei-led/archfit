package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/modgraph"
	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// ComputePropagation returns, for each first-party module, the count of other
// first-party modules that are transitively reachable from it in the forward
// direction ("how many modules can this one cascade into?"). It also returns
// the total number of first-party modules.
//
// System-level PC = sum(reach[m]) / (n*(n-1)).
// Per-module fraction = reach[m] / (n-1).
//
// Only first-party modules (those present as graph nodes) are counted.
// Self-loops and duplicate edges are collapsed before the BFS.
func ComputePropagation(g *graph.Graph) (reach map[string]int, n int) {
	// Build first-party set from graph nodes.
	firstParty := make(map[string]struct{})
	for _, nd := range g.Nodes() {
		firstParty[modgraph.ModuleKey(nd.ID())] = struct{}{}
	}
	n = len(firstParty)

	// Collapse edges to per-module adjacency (dedup multi-edges).
	adj := make(map[string]map[string]struct{}, n)
	for m := range firstParty {
		adj[m] = make(map[string]struct{})
	}
	for _, e := range g.Edges() {
		from, to := modgraph.ModuleKey(e.From), modgraph.ModuleKey(e.To)
		if from == to {
			continue
		}
		if _, ok := firstParty[to]; !ok {
			continue // skip edges into external dependencies
		}
		adj[from][to] = struct{}{}
	}

	// BFS from each module to count transitively reachable modules.
	reach = make(map[string]int, n)
	for start := range firstParty {
		visited := make(map[string]struct{}, n)
		visited[start] = struct{}{}
		queue := []string{start}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for nb := range adj[cur] {
				if _, seen := visited[nb]; !seen {
					visited[nb] = struct{}{}
					queue = append(queue, nb)
				}
			}
		}
		reach[start] = len(visited) - 1 // exclude start itself
	}
	return reach, n
}

// ---------------------------------------------------------------------------
// PropagationCostMetric
// ---------------------------------------------------------------------------

// PropagationCostMetric reports Propagation Cost (PC): the fraction of all
// ordered module pairs (i,j) where j is transitively reachable from i.
//
//	PC = reachable_pairs / (n*(n-1))
//
// where reachable_pairs = sum of reach[m] for all first-party modules m, and
// n = number of first-party modules.
//
// A system PC of 1.0 means every module cascades into every other (fully
// connected transitive closure). A value near 0 means a sparse graph.
//
// Per-module PC = reach[m]/(n-1): the fraction of the system reachable from m.
// Modules exceeding 50% are surfaced in the display as wide-blast contributors.
//
// Report-only (band "info"); never gates. Beyond Balanced Coupling — a standard
// cascade metric (CodeScene, LCOM literature).
type PropagationCostMetric struct{}

// Name returns "propagation_cost".
func (m PropagationCostMetric) Name() string { return "propagation_cost" }

// Version returns "propagation_cost.v1".
func (m PropagationCostMetric) Version() string { return "propagation_cost.v1" }

const propagationCostDef = "propagation cost PC=reachable_pairs/(n*(n-1)): fraction of ordered module pairs " +
	"(i,j) where j is transitively reachable from i; system + per-module; beyond Balanced Coupling, report-only"

// Calculate computes system-level and per-module propagation cost.
func (m PropagationCostMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.naResult()
	}

	reach, n := ComputePropagation(in.Graph)
	if n < 2 {
		return m.naResult()
	}

	denom := n * (n - 1)
	var totalReach int
	for _, r := range reach {
		totalReach += r
	}
	systemPC := float64(totalReach) / float64(denom)

	// Collect per-module wide-blast contributors (per-module PC > 50%).
	type entry struct {
		module string
		pc     float64
	}
	const perModThreshold = 0.5
	var wide []entry
	for mod, r := range reach {
		pc := float64(r) / float64(n-1)
		if pc > perModThreshold {
			wide = append(wide, entry{mod, pc})
		}
	}
	sort.Slice(wide, func(a, b int) bool {
		if wide[a].pc != wide[b].pc {
			return wide[a].pc > wide[b].pc
		}
		return wide[a].module < wide[b].module
	})

	confidence := result.ConfidenceHigh
	if n < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}

	var display string
	if len(wide) == 0 {
		display = fmt.Sprintf("PC=%.3f (N=%d); 0 modules reach >50%% of system", systemPC, n)
	} else {
		var sb strings.Builder
		fmt.Fprintf(&sb, "PC=%.3f (N=%d); %d modules reach >50%%: ", systemPC, n, len(wide))
		for i, e := range wide {
			if i == 5 {
				fmt.Fprintf(&sb, "+%d more", len(wide)-5)
				break
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s (%.0f%%)", result.ShortModule(e.module), e.pc*100)
		}
		display = sb.String()
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      systemPC,
		Display:    display,
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: propagationCostDef,
	}
}

func (m PropagationCostMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: m.Name(), Value: 0, Display: result.BandNA, Band: result.BandNA,
		Confidence: result.ConfidenceLow, Version: m.Version(), Mode: result.ModeRatio,
		Definition: propagationCostDef,
	}
}
