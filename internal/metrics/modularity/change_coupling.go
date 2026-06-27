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

// changeCouplingThreshold is CodeScene's canonical flag level: when two modules
// co-change in ≥65% of commits touching either, they are change-coupled regardless
// of the static dependency graph. Report-only until calibrated on more repos.
const changeCouplingThreshold = 0.65

// changeCouplingMinSupport is the minimum co-change commits before a pair is
// considered (small samples are noise). Same policy as hidden_coupling.
const changeCouplingMinSupport = 4

// ChangeCouplingMetric sharpens hidden_coupling with CodeScene's exact formula:
//
//	CC(A,B) = C_AB / min(C_A, C_B)
//
// where C_A and C_B are the number of commits that touched module A and B
// respectively, and C_AB is the number of commits that touched BOTH.
// Pairs with CC ≥ 65% are flagged as change-coupled.
//
// This metric covers ALL module pairs (not just those without a static import
// edge), so it complements hidden_coupling (which filters out importing pairs).
// The 65% threshold is CodeScene's published calibration value for "temporal
// coupling that is likely accidental or fragile". Report-only, band "info",
// never gates. Beyond Balanced Coupling standard metric.
type ChangeCouplingMetric struct{}

// Name returns "change_coupling".
func (m ChangeCouplingMetric) Name() string { return "change_coupling" }

// Version returns "change_coupling.v1".
func (m ChangeCouplingMetric) Version() string { return "change_coupling.v1" }

const changeCouplingDef = "change-coupled module pairs (CC≥65%%): " +
	"CC≈max-file-pair-co-change/min(module commits), an approximation of CodeScene's " +
	"C_AB/min(C_A,C_B) clamped to 100%% (true module C_AB needs commit→module data); " +
	"beyond Balanced Coupling, report-only"

// Calculate counts module pairs with CC ≥ 65%.
func (m ChangeCouplingMetric) Calculate(in signal.HistoryInput) diagnostic.MetricResult {
	if in.Graph == nil || len(in.History.CoChange) == 0 {
		return result.NACount(m.Name(), m.Version(), changeCouplingDef)
	}
	// A window with a single commit always produces 0 co-change pairs regardless of
	// the threshold — the result would be vacuously strong. Abstain instead.
	if in.History.CommitCount > 0 && in.History.CommitCount < 2 {
		return result.NACountMsg(m.Name(), m.Version(), changeCouplingDef, result.NANoHistory)
	}
	resolve := modgraph.ModuleKeyResolver(in.Graph)
	mc := modgraph.ModuleChurn(in.Graph, in.History.FileChurn)
	if len(mc) == 0 {
		return result.NACount(m.Name(), m.Version(), changeCouplingDef)
	}

	// Restrict to first-party module nodes present in the graph.
	nodeSet := make(map[string]struct{})
	for _, n := range in.Graph.Nodes() {
		nodeSet[modgraph.ModuleKey(n.ID())] = struct{}{}
	}

	// Aggregate file-pair co-change onto module pairs (graph modules only).
	//
	// Module-level C_AB (commits touching both modules) cannot be reconstructed
	// exactly from file-pair co-change counts: summing them over-counts a single
	// commit that touches many files of each module (it inflates one commit into
	// up to |A|×|B| file-pair hits, yielding CC>100%). We approximate C_AB by the
	// MAX single file-pair co-change between the two modules — a bounded lower
	// estimate of how often the modules co-change — and clamp the ratio to 1.0.
	// Upgrade trigger: when the history extractor emits commit→module co-occurrence,
	// replace this with the exact commit count.
	modPair := make(map[[2]string]int)
	for fp, c := range in.History.CoChange {
		a := resolve(fp[0])
		b := resolve(fp[1])
		if a == "" || b == "" || a == b {
			continue
		}
		if _, ok := nodeSet[a]; !ok {
			continue
		}
		if _, ok := nodeSet[b]; !ok {
			continue
		}
		key := modgraph.OrderedPair(a, b)
		if c > modPair[key] {
			modPair[key] = c
		}
	}

	type pair struct {
		a, b string
		cc   float64
	}
	var flagged []pair
	for mp, nab := range modPair {
		if nab < changeCouplingMinSupport {
			continue
		}
		minChurn := min(mc[mp[0]], mc[mp[1]])
		if minChurn == 0 {
			continue
		}
		cc := float64(nab) / float64(minChurn)
		if cc > 1.0 { // CC is a ratio; the max-file-pair proxy can still exceed minChurn
			cc = 1.0
		}
		if cc >= changeCouplingThreshold {
			flagged = append(flagged, pair{mp[0], mp[1], cc})
		}
	}

	sort.Slice(flagged, func(i, j int) bool {
		if flagged[i].cc != flagged[j].cc {
			return flagged[i].cc > flagged[j].cc
		}
		if flagged[i].a != flagged[j].a {
			return flagged[i].a < flagged[j].a
		}
		return flagged[i].b < flagged[j].b
	})

	n := len(mc)
	confidence := result.ConfidenceHigh
	if n < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}

	var disp strings.Builder
	fmt.Fprintf(&disp, "%d change-coupled pair(s) (CC≥65%%)", len(flagged))
	if len(flagged) > 0 {
		disp.WriteString("; top: ")
		for i, p := range flagged {
			if i == 4 {
				break
			}
			if i > 0 {
				disp.WriteString(", ")
			}
			fmt.Fprintf(&disp, "%s↔%s (CC=%.0f%%)",
				result.ShortModule(p.a), result.ShortModule(p.b), p.cc*100)
		}
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(flagged)),
		Display:    disp.String(),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: changeCouplingDef,
	}
}
