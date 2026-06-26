package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// GlobalStateDensityMetric reports the total count of global mutable state sites
// (static mut, Atomic* singletons, OnceLock) extracted by ast-grep, grouped by
// module. Report-only — Band: BandInformational, never gates. n/a when no
// global_state syntax facts are present (sg absent or no Rust).
//
// Note: like unsafe_density, this metric does NOT exclude test files.
// Global-state sites in test helpers are still part of the safety surface
// and should be measured.
type GlobalStateDensityMetric struct{}

// Name returns "global_state_density".
func (m GlobalStateDensityMetric) Name() string { return "global_state_density" }

// Version returns "global_state_density.v1".
func (m GlobalStateDensityMetric) Version() string { return "global_state_density.v1" }

type globalStateModule struct {
	key   string
	count int
}

// Calculate counts global_state facts per module and returns the total count with
// a human-readable summary of the top modules. n/a when no such facts exist.
func (m GlobalStateDensityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "global mutable state sites (static mut, Atomic* singletons, OnceLock) — report-only"
	modCounts := make(map[string]int)
	total := 0
	for _, f := range in.SyntaxFacts {
		if f.Kind != "global_state" {
			continue
		}
		key := f.Module
		if key == "" {
			key = f.File
		}
		modCounts[key]++
		total++
	}
	if total == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}

	mods := make([]globalStateModule, 0, len(modCounts))
	for k, v := range modCounts {
		mods = append(mods, globalStateModule{key: k, count: v})
	}
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].count != mods[j].count {
			return mods[i].count > mods[j].count
		}
		return mods[i].key < mods[j].key
	})

	confidence := result.ConfidenceHigh
	if len(mods) < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(total),
		Display:    globalStateDisplay(mods, total),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

func globalStateDisplay(mods []globalStateModule, total int) string {
	if len(mods) == 0 {
		return "0 global state site(s)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d global state site(s) across %d module(s): ", total, len(mods))
	for i, mo := range mods {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(mods)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%d)", mo.key, mo.count)
	}
	return b.String()
}
