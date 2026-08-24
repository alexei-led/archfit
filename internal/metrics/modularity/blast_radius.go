package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/modgraph"
	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// BlastRadiusMetric reports change-impact concentration: how many modules are
// "hubs" whose change forces a wide transitive rebuild, and which ones. It is a
// factual finding (report-only), not a quality verdict — a stable hub is fine.
// Volatility is the book's coupling_balance concern, not this metric's.
type BlastRadiusMetric struct{}

// Name returns "blast_radius".
func (m BlastRadiusMetric) Name() string { return "blast_radius" }

// Version returns "blast_radius.v1".
func (m BlastRadiusMetric) Version() string { return "blast_radius.v1" }

// hubInfo is one high-blast module for display.
type hubInfo struct {
	module string
	blast  int
	rel    float64
}

// Calculate computes per-module blast radius and reports the hubs above the
// threshold. Indeterminate (n/a) for graphs too small to have meaningful
// concentration.
func (m BlastRadiusMetric) Calculate(in signal.CommonInput) report.MetricResult {
	if in.Graph == nil {
		return m.naResult()
	}
	blast, n := modgraph.BlastRadius(in.Graph)
	if n < 2 {
		return m.naResult()
	}
	denom := float64(n - 1)

	hubs := make([]hubInfo, 0)
	for mod, b := range blast {
		rel := float64(b) / denom
		if rel >= hubBlastThreshold {
			hubs = append(hubs, hubInfo{module: mod, blast: b, rel: rel})
		}
	}
	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].blast != hubs[j].blast {
			return hubs[i].blast > hubs[j].blast
		}
		return hubs[i].module < hubs[j].module
	})

	confidence := result.ConfidenceHigh
	if n < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	return report.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(hubs)),
		Display:    blastDisplay(hubs, n),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: "modules whose transitive reverse-dependencies exceed " +
			fmt.Sprintf("%.0f%%", hubBlastThreshold*100) + " of the codebase (change-impact hubs)",
		Direction: report.DirectionHigherIsWorse,
	}
}

func (m BlastRadiusMetric) naResult() report.MetricResult {
	return report.MetricResult{
		Name: m.Name(), Value: 0, Display: result.BandNA, Band: result.BandNA,
		Confidence: result.ConfidenceLow, Version: m.Version(), Mode: result.ModeCount,
		Definition: "modules whose transitive reverse-dependencies are a large fraction of the codebase",
		Direction:  report.DirectionHigherIsWorse,
	}
}

// blastDisplay renders the top hubs compactly for human/LLM output.
func blastDisplay(hubs []hubInfo, n int) string {
	if len(hubs) == 0 {
		return fmt.Sprintf("0 change-impact hubs (%d modules)", n)
	}
	var b strings.Builder
	// Report the count as a fraction of all modules so dilution is self-evident:
	// "85 of 100 modules" reads as a structural property, not 85 distinct problems
	// (a fixed 30% threshold flags many modules on large hierarchical graphs, F11).
	fmt.Fprintf(&b, "%d of %d modules are change-impact hubs: ", len(hubs), n)
	for i, h := range hubs {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(hubs)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%.0f%%, %d deps)", result.ShortModule(h.module), h.rel*100, h.blast)
	}
	return b.String()
}
