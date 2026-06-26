package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/syntax"
)

// PanicDensityMetric reports the total count of production panic/unwrap/expect
// call sites extracted by ast-grep, grouped by module. Test files (Go: *_test.go;
// Rust: files under /tests/) are excluded from the count — this is a production-
// code signal only. Report-only — Band: BandInformational, never gates.
// n/a when no panic_op syntax facts survive test-file exclusion.
type PanicDensityMetric struct{}

// Name returns "panic_density".
func (m PanicDensityMetric) Name() string { return "panic_density" }

// Version returns "panic_density.v1".
func (m PanicDensityMetric) Version() string { return "panic_density.v1" }

type panicModule struct {
	key   string
	count int
}

// Calculate counts panic_op facts per module (excluding test files) and returns
// the total count with a human-readable summary of the top modules. n/a when no
// such facts remain after test-file exclusion.
func (m PanicDensityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "production panic/unwrap/expect call sites (Rust: unwrap/expect/panic!, Go: panic) — excludes test files, report-only"
	modCounts := make(map[string]int)
	total := 0
	for _, f := range in.SyntaxFacts {
		if f.Kind != "panic_op" {
			continue
		}
		if syntax.IsTestFile(f.Language, f.File) {
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

	mods := make([]panicModule, 0, len(modCounts))
	for k, v := range modCounts {
		mods = append(mods, panicModule{key: k, count: v})
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
		Display:    panicDisplay(mods, total),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

func panicDisplay(mods []panicModule, total int) string {
	if len(mods) == 0 {
		return "0 panic/unwrap site(s)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d panic/unwrap site(s) across %d module(s): ", total, len(mods))
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
