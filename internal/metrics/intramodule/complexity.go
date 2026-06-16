// Package intramodule holds the intra-module quality metrics — signals about the
// internals of a module (function complexity, architecture-enforcement presence)
// rather than its coupling to other modules.
package intramodule

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// complexityThreshold is the cyclomatic complexity above which a function is a
// hotspot (lizard's default warning level). Tunable; the metric is report-only.
const complexityThreshold = 15

// ComplexityMetric reports the most cyclomatically-complex functions — the
// intra-module signal that size (structural_weight) cannot see (a single giant
// branchy function). Language-agnostic: archfit reuses an external complexity tool
// (lizard) rather than reimplementing per-language parsing. Opt-in; report-only.
type ComplexityMetric struct{}

// Name returns "complexity".
func (m ComplexityMetric) Name() string { return "complexity" }

// Version returns "complexity.v1".
func (m ComplexityMetric) Version() string { return "complexity.v1" }

// Calculate reports functions over the complexity threshold, ranked. n/a when no
// complexity data is available (the tool is opt-in / not installed).
func (m ComplexityMetric) Calculate(in signal.ComplexityInput) diagnostic.MetricResult {
	def := "functions whose cyclomatic complexity exceeds " +
		strconv.Itoa(complexityThreshold) + " (external tool: lizard)"
	if len(in.Complexity.Funcs) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}
	hot := make([]signal.ComplexityFunc, 0)
	for _, f := range in.Complexity.Funcs {
		if f.CCN > complexityThreshold {
			hot = append(hot, f)
		}
	}
	sort.Slice(hot, func(i, j int) bool { return hot[i].CCN > hot[j].CCN })

	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(len(hot)), Display: complexityDisplay(hot),
		Band: result.BandInformational, Confidence: result.ConfidenceHigh, Version: m.Version(),
		Mode: result.ModeCount, Definition: def,
	}
}

func complexityDisplay(hot []signal.ComplexityFunc) string {
	if len(hot) == 0 {
		return fmt.Sprintf("0 functions over CCN %d", complexityThreshold)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d complex function(s) (CCN>%d): ", len(hot), complexityThreshold)
	for i, f := range hot {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(hot)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		name := f.Name
		if name == "" {
			name = "(anon)"
		}
		fmt.Fprintf(&b, "%s CCN %d (%s:%d)", name, f.CCN, shortFile(f.File), f.Line)
	}
	return b.String()
}

// shortFile trims a file path to its last two segments for compact display.
func shortFile(file string) string {
	parts := strings.Split(file, "/")
	if len(parts) <= 2 {
		return file
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}
