package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// UnsafeDensityMetric reports the total count of unsafe Rust operation sites
// (unsafe{} blocks, UnsafeCell, raw pointer casts, transmute) extracted by
// ast-grep, grouped by module. Report-only — Band: BandInformational, never
// gates. n/a when no unsafe_op syntax facts are present (sg absent or no Rust).
type UnsafeDensityMetric struct{}

// Name returns "unsafe_density".
func (m UnsafeDensityMetric) Name() string { return "unsafe_density" }

// Version returns "unsafe_density.v1".
func (m UnsafeDensityMetric) Version() string { return "unsafe_density.v1" }

type unsafeModule struct {
	key   string
	count int
}

// Calculate counts unsafe_op facts per module and returns the total count with
// a human-readable summary of the top modules. n/a when no such facts exist.
func (m UnsafeDensityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "unsafe Rust operation sites (unsafe{} blocks, UnsafeCell, raw casts, transmute) — report-only"
	modCounts := make(map[string]int)
	total := 0
	for _, f := range in.SyntaxFacts {
		if f.Kind != "unsafe_op" {
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

	mods := make([]unsafeModule, 0, len(modCounts))
	for k, v := range modCounts {
		mods = append(mods, unsafeModule{key: k, count: v})
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
		Display:    unsafeDisplay(mods, total),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

func unsafeDisplay(mods []unsafeModule, total int) string {
	if len(mods) == 0 {
		return "0 unsafe site(s)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d unsafe site(s) across %d module(s): ", total, len(mods))
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
