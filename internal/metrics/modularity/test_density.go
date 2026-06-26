package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// TestDensityMetric reports the total count of test functions per module from
// SyntaxFacts with Kind="test_fn". Unlike panic_density, test files are NOT
// excluded — test functions live in test files by design.
//
// Proxy for test coverage: counts test functions declared in test files
// (Go `*_test.go`, Rust `tests/`, Python `test_*`). Real coverage needs a
// coverage-tool run and stays LLM/human territory.
//
// Report-only — Band: BandInformational, never gates.
// Distinction:
//   - SyntaxFacts nil/empty (syntax layer did not run): n/a.
//   - SyntaxFacts present but zero test_fn (syntax ran; repo has no tests): Value=0, BandInformational.
//
// This matters for absence-signal coverage probes: Value=0 / non-n/a band is the
// "no tests found" signal; n/a means "tooling absent / not analysed".
type TestDensityMetric struct{}

// Name returns "test_density".
func (m TestDensityMetric) Name() string { return "test_density" }

// Version returns "test_density.v1".
func (m TestDensityMetric) Version() string { return "test_density.v1" }

type testModule struct {
	key   string
	count int
}

// Calculate counts test_fn facts per module and returns the total count with a
// human-readable summary of the top modules.
// n/a only when SyntaxFacts is nil or empty (syntax layer did not run).
// Value=0 / BandInformational when syntax ran but found no test_fn (no tests in repo).
func (m TestDensityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "test functions per module (Go: func TestXxx, Rust: fn test_xxx, Python: def test_xxx) — proxy for test coverage, report-only"

	// n/a only when the syntax layer produced no facts at all — tooling absent or unsupported.
	if len(in.SyntaxFacts) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}

	modCounts := make(map[string]int)
	total := 0
	for _, f := range in.SyntaxFacts {
		if f.Kind != "test_fn" {
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
		// Syntax ran and found other facts, but zero test functions — absence IS the signal.
		return diagnostic.MetricResult{
			Name:       m.Name(),
			Value:      0,
			Display:    testDisplay(nil, 0),
			Band:       result.BandInformational,
			Confidence: result.ConfidenceLow,
			Version:    m.Version(),
			Mode:       result.ModeCount,
			Definition: def,
		}
	}

	mods := make([]testModule, 0, len(modCounts))
	for k, v := range modCounts {
		mods = append(mods, testModule{key: k, count: v})
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
		Display:    testDisplay(mods, total),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

func testDisplay(mods []testModule, total int) string {
	if len(mods) == 0 {
		return "0 test function(s)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d test function(s) across %d module(s): ", total, len(mods))
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
