package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/syntax"
)

// PanicDensityMetric reports the total count of production panic/unwrap/expect
// call sites extracted by ast-grep, grouped by module. Test files and generated
// files (mock_*.go, *.pb.go, files with the standard generated header, etc.) are
// excluded from the count — this is a production-code signal only. The excluded
// count is surfaced in the evidence string so nothing is hidden.
// Report-only — Band: BandInformational, never gates.
// n/a when no panic_op syntax facts exist at all.
type PanicDensityMetric struct{}

// Name returns "panic_density".
func (m PanicDensityMetric) Name() string { return "panic_density" }

// Version returns "panic_density.v1".
func (m PanicDensityMetric) Version() string { return "panic_density.v1" }

type panicModule struct {
	key   string
	count int
}

// Calculate counts panic_op facts per module (excluding test and generated files)
// and returns the total production count. The count of excluded facts is appended
// to the evidence string. n/a when no panic_op facts exist at all.
//
// FileClassIndex keys and SyntaxFact.File values must both be repo-relative slash
// paths for the index lookup to hit. When the index is nil (loc walk did not run),
// LookupFileClass falls back to built-in filename/path patterns only.
func (m PanicDensityMetric) Calculate(in signal.SizeInput) diagnostic.MetricResult {
	const def = "production panic/unwrap/expect call sites (Rust: unwrap/expect/panic!, Go: panic) — excludes test/generated/vendor files, report-only"
	modCounts := make(map[string]int)
	total := 0
	excluded := 0
	// FileClassConfig{} is intentional: index files already incorporate the
	// user's config patterns (applied at loc-walk time); the skipDir fallback
	// path loses custom patterns, but built-in filename heuristics cover the
	// common cases (mock_*.go, *.pb.go, generated header, etc.).
	cfg := syntax.FileClassConfig{}
	for _, f := range in.SyntaxFacts {
		if f.Kind != "panic_op" {
			continue
		}
		fc := syntax.LookupFileClass(f.File, in.Size.FileClassIndex, f.Language, cfg)
		if !fileclass.IsProduction(fc) {
			excluded++
			continue
		}
		key := f.Module
		if key == "" {
			key = f.File
		}
		modCounts[key]++
		total++
	}
	if total == 0 && excluded == 0 {
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
		Display:    panicDisplay(mods, total, excluded),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

func panicDisplay(mods []panicModule, total, excluded int) string {
	var b strings.Builder
	if len(mods) == 0 {
		fmt.Fprintf(&b, "0 production panic/unwrap site(s)")
	} else {
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
	}
	if excluded > 0 {
		fmt.Fprintf(&b, " (%d in test/generated/vendor excluded)", excluded)
	}
	return b.String()
}
