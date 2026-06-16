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

// ChangeAmplificationMetric reports modules whose change is expensive AND likely:
// blast radius (transitive reverse-deps) weighted by volatility (recent churn). A
// stable hub scores low (fine); a volatile hub scores high (the real risk). This is
// archfit's on-mission signal — expected change cost — reported as a ranked finding.
type ChangeAmplificationMetric struct{}

// Name returns "change_amplification".
func (m ChangeAmplificationMetric) Name() string { return "change_amplification" }

// Version returns "change_amplification.v1".
func (m ChangeAmplificationMetric) Version() string { return "change_amplification.v1" }

type ampHub struct {
	module string
	amp    float64
	blast  int
	churn  int
}

// Calculate ranks modules by change amplification. n/a without a graph or churn.
func (m ChangeAmplificationMetric) Calculate(in signal.MetricInput) diagnostic.MetricResult {
	if in.Graph == nil || len(in.FileChurn) == 0 {
		return result.NACount(m.Name(), m.Version(), "blast radius weighted by recent churn (expected change cost)")
	}
	blast, n := modgraph.BlastRadius(in.Graph)
	if n < 2 {
		return result.NACount(m.Name(), m.Version(), "blast radius weighted by recent churn (expected change cost)")
	}
	mc := modgraph.ModuleChurn(in.FileChurn, modgraph.DominantLanguage(in.Graph))
	maxChurn := 0
	for _, c := range mc {
		if c > maxChurn {
			maxChurn = c
		}
	}
	if maxChurn == 0 {
		return result.NACount(m.Name(), m.Version(), "blast radius weighted by recent churn (expected change cost)")
	}
	denom := float64(n - 1)

	var hubs []ampHub
	for mod, b := range blast {
		amp := (float64(b) / denom) * (float64(mc[mod]) / float64(maxChurn))
		if amp >= ampHubThreshold {
			hubs = append(hubs, ampHub{module: mod, amp: amp, blast: b, churn: mc[mod]})
		}
	}
	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].amp != hubs[j].amp {
			return hubs[i].amp > hubs[j].amp
		}
		return hubs[i].module < hubs[j].module
	})

	confidence := result.ConfidenceHigh
	if n < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(len(hubs)), Display: ampDisplay(hubs),
		Band: result.BandInformational, Confidence: confidence, Version: m.Version(), Mode: result.ModeCount,
		Definition: "modules ranked by blast radius × recent churn (volatile change-impact hubs)",
	}
}

func ampDisplay(hubs []ampHub) string {
	if len(hubs) == 0 {
		return "0 volatile change-impact hubs"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d volatile hub(s): ", len(hubs))
	for i, h := range hubs {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(hubs)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (amp %.2f: %d deps × %d commits)", result.ShortModule(h.module), h.amp, h.blast, h.churn)
	}
	return b.String()
}
