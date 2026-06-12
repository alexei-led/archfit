package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// riskHubTopN is the maximum number of module hubs displayed in the output.
const riskHubTopN = 5

// volatilityBandMultiplier maps an explicit config volatility band to a risk
// multiplier. Only bands authored in the config file are recognised; anything
// else (including the empty string for unset modules) returns 1.0 (neutral).
// This is intentionally separate from config.DeriveVolatility — risk_hub must
// NEVER use churn-derived volatility, which would double-count the same signal
// as change_amplification.
//
//	high   → 1.00 (most volatile)
//	medium → 0.66
//	low    → 0.33
//	unset  → 1.00 (neutral — no explicit opinion)
func volatilityBandMultiplier(band string) float64 {
	switch band {
	case volBandHigh:
		return 1.00
	case volBandMedium:
		return 0.66
	case volBandLow:
		return 0.33
	default:
		return 1.0
	}
}

// Volatility band values as authored in config (ModuleDef.Volatility).
const (
	volBandHigh   = "high"
	volBandMedium = "medium"
	volBandLow    = "low"
)

// moduleSurfaceBreadth computes, for each module, the count of that module's
// own symbols that are referenced by at least one symbol from a different
// module (cross-module external surface breadth).
//
// This is distinct from blast_radius (which measures module-level transitive
// reverse-reachability) because it counts *how many of the module's own
// symbols* are externally coupled — not how many modules transitively depend
// on it. A state store with 73 externally-referenced fields ranks higher than
// a utility with 1 widely-called function, even if the utility's module-level
// blast radius is larger. This difference is the unique signal risk_hub adds.
//
// g.Refs[from] = {to, ...} means "from references to". For each "to" in
// module M that has at least one cross-module "from", increment M's breadth
// count by 1.
func moduleSurfaceBreadth(g symbol.Graph) map[string]int {
	if g.Empty() {
		return nil
	}

	// Build a set of symbols that have at least one cross-module referencing symbol.
	externallyReferenced := make(map[string]struct{})
	for from, tos := range g.Refs {
		fromMod := g.Module[from]
		for to := range tos {
			if from == to {
				continue
			}
			toMod := g.Module[to]
			if fromMod != "" && toMod != "" && fromMod != toMod {
				externallyReferenced[to] = struct{}{}
			}
		}
	}

	if len(externallyReferenced) == 0 {
		return nil
	}

	// Aggregate: count of each module's symbols that are externally referenced.
	breadth := make(map[string]int, len(g.Module))
	for sym := range externallyReferenced {
		mod := g.Module[sym]
		if mod != "" {
			breadth[mod]++
		}
	}
	return breadth
}

// RiskHubMetric surfaces symbol-level risk hubs: modules whose cross-module
// external surface breadth (count of their own symbols referenced from other
// modules) multiplied by the owning module's explicit config volatility
// identifies the highest-risk hubs.
//
// This is intentionally distinct from blast_radius (module-level transitive
// reachability) and change_amplification (churn-derived volatility at module
// granularity). risk_hub uses only hand-authored subdomain/volatility config —
// never churn — to avoid double-counting; and it counts symbol-surface breadth
// rather than module-level fan-out, so a data store with many externally-used
// fields ranks higher than a shallow utility with one heavily-called function.
//
// Volatility multipliers are captured at construction time (before
// config.ApplyVolatility can inject churn-derived values), ensuring that
// only explicit config volatility affects the score.
//
// The result is report-only (band: info) — it never gates.
type RiskHubMetric struct {
	// moduleVolatility maps module name → explicit-config volatility multiplier.
	// Built in New() before ApplyVolatility runs; never updated afterward.
	// Modules absent from this map get a neutral multiplier of 1.0.
	moduleVolatility map[string]float64
}

// Name returns "risk_hub".
func (m RiskHubMetric) Name() string { return "risk_hub" }

// Version returns "risk_hub.v1".
func (m RiskHubMetric) Version() string { return "risk_hub.v1" }

// riskHubInfo holds per-module aggregated risk for display.
type riskHubInfo struct {
	module          string
	breadth         int     // count of externally-referenced symbols
	risk            float64 // breadth × volatility multiplier [× gitnexus factor when present]
	multiplier      float64
	gitnexusFactor  float64 // 1.0 when gitnexus absent; bounded [1.0, 2.0] when present
	gitnexusPresent bool    // true when GitnexusImpact was non-empty
}

// gitnexusImpactFactor converts a raw gitnexus impact count for a module into a
// bounded multiplicative factor in [1.0, 2.0]. This keeps gitnexus as a refining
// enrichment rather than allowing it to dominate the surface-breadth ranking.
//
// Formula: 1.0 + clamp(impact / maxImpact, 0, 1) where maxImpact is the largest
// impact value seen across all modules in this run (normalised per-run).
// A module absent from the impact map receives 1.0 (neutral — no penalty).
func gitnexusImpactFactor(mod string, impactMap map[string]int, maxImpact int) float64 {
	if len(impactMap) == 0 || maxImpact <= 0 {
		return 1.0
	}
	v, ok := impactMap[mod]
	if !ok {
		return 1.0
	}
	normalised := float64(v) / float64(maxImpact) // [0, 1]
	return 1.0 + normalised                       // [1.0, 2.0]
}

// moduleImpactFromFiles aggregates the file-keyed gitnexus dependant counts to
// module granularity via the symbol graph's defining-file paths. A module's
// impact is the MAX of its files' counts (its most-depended-on file) — summing
// would double-count multi-file modules against single-file ones.
func moduleImpactFromFiles(g symbol.Graph, fileImpact map[string]int) map[string]int {
	if len(fileImpact) == 0 {
		return nil
	}
	out := make(map[string]int)
	for sym, mod := range g.Module {
		if mod == "" {
			continue
		}
		if v, ok := fileImpact[g.Path[sym]]; ok && v > out[mod] {
			out[mod] = v
		}
	}
	return out
}

// Calculate ranks modules by (symbol-surface breadth × volatility_multiplier)
// and reports the top hubs. Returns n/a when SymbolGraph is empty (SCIP off or
// indexer absent) — never a false zero.
//
// When MetricInput.GitnexusImpact is non-empty (tools.gitnexus.enabled: on and
// the CLI was present), each module's score is further multiplied by a bounded
// gitnexus factor in [1.0, 2.0] derived from its historical change-impact count
// (normalised to the per-run maximum). When GitnexusImpact is nil/empty (the
// default), this factor is 1.0 for every module and the result is exactly the
// same as plain surface-breadth × volatility.
func (m RiskHubMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	hasGitnexus := len(in.GitnexusImpact) > 0
	def := "cross-module surface breadth × explicit config volatility: count of a module's " +
		"symbols referenced from other modules (churn-independent; never gates)"
	if hasGitnexus {
		def = "cross-module surface breadth × explicit config volatility × gitnexus historical impact: " +
			"count of a module's symbols referenced from other modules, refined by historical change impact " +
			"(churn-independent; never gates)"
	}
	if in.SymbolGraph.Empty() {
		return naCount(m.Name(), m.Version(), def)
	}

	breadth := moduleSurfaceBreadth(in.SymbolGraph)
	if len(breadth) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}

	// GitnexusImpact is keyed by file path; aggregate to module granularity
	// through the symbol graph, then normalise to the per-run maximum.
	moduleImpact := moduleImpactFromFiles(in.SymbolGraph, in.GitnexusImpact)
	maxImpact := 0
	for _, v := range moduleImpact {
		if v > maxImpact {
			maxImpact = v
		}
	}

	// Build ranked list of module hubs.
	hubs := make([]riskHubInfo, 0, len(breadth))
	for mod, b := range breadth {
		mult := m.volatilityMultiplier(mod)
		gf := gitnexusImpactFactor(mod, moduleImpact, maxImpact)
		hubs = append(hubs, riskHubInfo{
			module:          mod,
			breadth:         b,
			risk:            float64(b) * mult * gf,
			multiplier:      mult,
			gitnexusFactor:  gf,
			gitnexusPresent: hasGitnexus,
		})
	}
	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].risk != hubs[j].risk {
			return hubs[i].risk > hubs[j].risk
		}
		return hubs[i].module < hubs[j].module
	})

	// Confidence: high when there are enough externally-referenced symbols;
	// low for tiny graphs.
	confidence := confidenceHigh
	totalExternalSymbols := 0
	for _, b := range breadth {
		totalExternalSymbols += b
	}
	if totalExternalSymbols < modularitySmallN {
		confidence = confidenceLow
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(hubs)),
		Display:    riskHubDisplay(hubs, totalExternalSymbols),
		Band:       bandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: def,
	}
}

// volatilityMultiplier returns the pre-captured explicit-config multiplier for
// a module. Returns 1.0 (neutral) for modules not in the map.
func (m RiskHubMetric) volatilityMultiplier(module string) float64 {
	if v, ok := m.moduleVolatility[module]; ok {
		return v
	}
	return 1.0
}

// riskHubDisplay renders the top-N hubs compactly for human/LLM output.
func riskHubDisplay(hubs []riskHubInfo, totalExternalSymbols int) string {
	if len(hubs) == 0 {
		return fmt.Sprintf("0 risk hubs (%d external symbols)", totalExternalSymbols)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d risk hub(s): ", len(hubs))
	for i, h := range hubs {
		if i == riskHubTopN {
			fmt.Fprintf(&b, "+%d more", len(hubs)-riskHubTopN)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		if h.gitnexusPresent {
			fmt.Fprintf(&b, "%s [breadth %d, ×%.2f, gn×%.2f→%.2f]",
				shortModule(h.module), h.breadth, h.multiplier, h.gitnexusFactor, h.risk)
		} else {
			fmt.Fprintf(&b, "%s [breadth %d, ×%.2f→%.2f]",
				shortModule(h.module), h.breadth, h.multiplier, h.risk)
		}
	}
	return b.String()
}
