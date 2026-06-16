package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/modgraph"
	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// HiddenCouplingMetric reports module pairs that change together (git co-change)
// but do NOT import each other — coupling invisible to the dependency graph (e.g.
// deflected by boundary tests or mediated by a shared data model). The canonical
// case is a module with low import fan-in that co-changes with many others.
type HiddenCouplingMetric struct{}

// Name returns "hidden_coupling".
func (m HiddenCouplingMetric) Name() string { return "hidden_coupling" }

// Version returns "hidden_coupling.v1".
func (m HiddenCouplingMetric) Version() string { return "hidden_coupling.v1" }

// Calculate counts hidden-coupling module pairs (co-change without a static edge).
func (m HiddenCouplingMetric) Calculate(in signal.MetricInput) diagnostic.MetricResult {
	def := "module pairs that co-change without importing each other (coupling unseen by the graph)"
	if in.Graph == nil || len(in.CoChange) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}
	lang := modgraph.DominantLanguage(in.Graph)
	mc := modgraph.ModuleChurn(in.FileChurn, lang)

	// First-party module nodes: restrict co-change to real graph modules so docs
	// (CHANGELOG.md), config files, and pre-rename historical paths are excluded.
	nodeSet := make(map[string]struct{})
	for _, n := range in.Graph.Nodes() {
		nodeSet[modgraph.ModuleKey(n.ID())] = struct{}{}
	}
	// Static undirected module edges (importing relationship).
	edge := make(map[[2]string]struct{})
	for _, e := range in.Graph.Edges() {
		a, b := modgraph.ModuleKey(e.From), modgraph.ModuleKey(e.To)
		if a == b {
			continue
		}
		edge[modgraph.OrderedPair(a, b)] = struct{}{}
	}
	// Aggregate file-pair co-change onto module pairs (graph modules only).
	modPair := make(map[[2]string]int)
	for fp, c := range in.CoChange {
		a, b := modgraph.FileToModuleKey(fp[0], lang), modgraph.FileToModuleKey(fp[1], lang)
		if a == "" || b == "" || a == b {
			continue
		}
		if _, ok := nodeSet[a]; !ok {
			continue
		}
		if _, ok := nodeSet[b]; !ok {
			continue
		}
		modPair[modgraph.OrderedPair(a, b)] += c
	}

	hiddenPartners := map[string]int{}
	pairs := 0
	for mp, nab := range modPair {
		if nab < hiddenMinSupport {
			continue
		}
		if _, imports := edge[mp]; imports {
			continue // expected coupling — they depend on each other
		}
		minChurn := min(mc[mp[0]], mc[mp[1]])
		if minChurn == 0 {
			continue
		}
		if float64(nab)/float64(minChurn) >= hiddenLCThreshold {
			hiddenPartners[mp[0]]++
			hiddenPartners[mp[1]]++
			pairs++
		}
	}

	type hp struct {
		module string
		count  int
	}
	var ranked []hp
	for mod, c := range hiddenPartners {
		ranked = append(ranked, hp{mod, c})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].module < ranked[j].module
	})

	n := len(mc)
	confidence := result.ConfidenceHigh
	if n < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	var disp strings.Builder
	fmt.Fprintf(&disp, "%d hidden-coupling pair(s)", pairs)
	if len(ranked) > 0 {
		disp.WriteString("; top: ")
		for i, r := range ranked {
			if i == 4 {
				break
			}
			if i > 0 {
				disp.WriteString(", ")
			}
			fmt.Fprintf(&disp, "%s (%d)", result.ShortModule(r.module), r.count)
		}
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(pairs), Display: disp.String(),
		Band: result.BandInformational, Confidence: confidence, Version: m.Version(), Mode: result.ModeCount,
		Definition: def,
	}
}
