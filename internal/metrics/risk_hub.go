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
	case "high":
		return 1.00
	case "medium":
		return 0.66
	case "low":
		return 0.33
	default:
		return 1.0
	}
}

// symbolImpact computes, for each symbol in the graph, the count of symbols
// that transitively depend on it (i.e. transitive reverse-reachability over
// g.Refs). SCCs are condensed so cycles do not inflate the count — mirroring
// blastRadius/tarjanSCC from modularity.go.
//
// g.Refs[from] = {to, ...} means "from references to", so the reverse edge
// "to ← from" means "from depends on to". symbolImpact returns, for each "to",
// how many symbols transitively reach it via those reverse edges.
func symbolImpact(g symbol.Graph) map[string]int {
	if g.Empty() {
		return nil
	}

	// Build adjacency from Refs (forward: from → to).
	// We also need the full symbol universe (all symbols that appear in Module).
	adj := make(map[string]map[string]struct{}, len(g.Module))
	for sym := range g.Module {
		adj[sym] = make(map[string]struct{})
	}
	for from, tos := range g.Refs {
		if _, known := adj[from]; !known {
			adj[from] = make(map[string]struct{})
		}
		for to := range tos {
			if from == to {
				continue
			}
			if _, known := adj[to]; !known {
				adj[to] = make(map[string]struct{})
			}
			adj[from][to] = struct{}{}
		}
	}

	// Condense SCCs so cycles don't inflate impact counts.
	comp := tarjanSCC(adj) // symbol → component id
	compSize := make(map[int]int)
	for _, c := range comp {
		compSize[c]++
	}

	// Build reverse condensed adjacency: for each forward edge from→to
	// (different SCCs), add crev[comp[to]][comp[from]] so we can do
	// reverse-reachability from a target component.
	crev := make(map[int]map[int]struct{})
	for from, tos := range adj {
		for to := range tos {
			cf, ct := comp[from], comp[to]
			if cf == ct {
				continue
			}
			if crev[ct] == nil {
				crev[ct] = make(map[int]struct{})
			}
			crev[ct][cf] = struct{}{}
		}
	}

	impact := make(map[string]int, len(adj))
	for sym := range adj {
		// Transitive reverse-reach in the condensed DAG, counting member symbols.
		start := comp[sym]
		seen := map[int]struct{}{start: {}}
		stack := []int{start}
		members := 0
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for pc := range crev[c] {
				if _, ok := seen[pc]; !ok {
					seen[pc] = struct{}{}
					members += compSize[pc]
					stack = append(stack, pc)
				}
			}
		}
		// Symbols in the same SCC (other than sym itself) also depend on sym.
		impact[sym] = members + (compSize[start] - 1)
	}
	return impact
}

// RiskHubMetric surfaces symbol-level risk hubs: symbols whose transitive
// dependents (impact) are multiplied by the owning module's explicit config
// volatility. This is intentionally distinct from change_amplification, which
// uses churn-derived volatility at module granularity. risk_hub uses only
// hand-authored subdomain/volatility config — never churn — to avoid
// double-counting the same signal.
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
	module     string
	risk       float64
	topSymbol  string
	topImpact  int
	multiplier float64
}

// Calculate ranks modules by max(symbol_impact × volatility_multiplier) and
// reports the top hubs. Returns n/a when SymbolGraph is empty (SCIP off or
// indexer absent) — never a false zero.
func (m RiskHubMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	def := "symbol-level impact hubs: transitive dependents × explicit config volatility " +
		"(churn-independent; never gates)"
	if in.SymbolGraph.Empty() {
		return naCount(m.Name(), m.Version(), def)
	}

	impact := symbolImpact(in.SymbolGraph)
	if len(impact) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}

	// Aggregate symbols to their owning module: take the max risk score per module.
	type symRisk struct {
		sym    string
		impact int
		risk   float64
	}
	modBest := make(map[string]symRisk)
	for sym, imp := range impact {
		owningModule := in.SymbolGraph.Module[sym]
		multiplier := m.volatilityMultiplier(owningModule)
		risk := float64(imp) * multiplier
		if prev, ok := modBest[owningModule]; !ok || risk > prev.risk {
			modBest[owningModule] = symRisk{sym: sym, impact: imp, risk: risk}
		}
	}

	// Sort modules by descending risk score.
	hubs := make([]riskHubInfo, 0, len(modBest))
	for mod, best := range modBest {
		mult := m.volatilityMultiplier(mod)
		hubs = append(hubs, riskHubInfo{
			module:     mod,
			risk:       best.risk,
			topSymbol:  best.sym,
			topImpact:  best.impact,
			multiplier: mult,
		})
	}
	sort.Slice(hubs, func(i, j int) bool {
		if hubs[i].risk != hubs[j].risk {
			return hubs[i].risk > hubs[j].risk
		}
		return hubs[i].module < hubs[j].module
	})

	// Confidence: high when there are enough symbols; low for tiny graphs.
	confidence := confidenceHigh
	if len(impact) < modularitySmallN {
		confidence = confidenceLow
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(hubs)),
		Display:    riskHubDisplay(hubs, len(impact)),
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
func riskHubDisplay(hubs []riskHubInfo, totalSymbols int) string {
	if len(hubs) == 0 {
		return fmt.Sprintf("0 risk hubs (%d symbols)", totalSymbols)
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
		sym := shortSymbol(h.topSymbol)
		fmt.Fprintf(&b, "%s [%s, impact %d, ×%.2f→%.2f]",
			shortModule(h.module), sym, h.topImpact, h.multiplier, h.risk)
	}
	return b.String()
}

// shortSymbol trims a fully-qualified symbol to its last identifier for compact display.
func shortSymbol(sym string) string {
	// SCIP symbols look like "go package path/TypeName#MethodName()." or similar;
	// take the last non-empty segment after splitting on common delimiters.
	sym = strings.TrimRight(sym, ".")
	for _, sep := range []string{"#", ".", "/"} {
		if i := strings.LastIndex(sym, sep); i >= 0 {
			candidate := sym[i+1:]
			if candidate != "" {
				sym = candidate
				break
			}
		}
	}
	return sym
}
