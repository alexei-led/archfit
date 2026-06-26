package modularity

import (
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// DeprecatedDepCountMetric reports the count of locally-declared deprecation
// and retraction markers found in manifest files (go.mod retract directives
// and package.json "deprecated" fields). Report-only — Band: BandInformational,
// never gates. Zero when no markers are present (returns value 0, not n/a).
//
// Ceiling: cargo yanked and live-version EOL require external registry queries
// and are not detected here — they are routed to the LLM review path.
type DeprecatedDepCountMetric struct{}

// Name returns "deprecated_dep_count".
func (m DeprecatedDepCountMetric) Name() string { return "deprecated_dep_count" }

// Version returns "deprecated_dep_count.v1".
func (m DeprecatedDepCountMetric) Version() string { return "deprecated_dep_count.v1" }

const deprecatedDepDef = "locally-declared deprecation/retraction markers in manifest files " +
	"(go.mod retract, package.json deprecated) — report-only; " +
	"cargo yanked and live EOL require external registry queries (use archfit review/enrich)"

// Calculate counts DeprecatedDep entries and returns a BandInformational result.
// Returns value 0 (not n/a) when no markers are present so the metric is always
// present in output and never shifts analysisConfidence's n/a share.
func (m DeprecatedDepCountMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	deps := in.DeprecatedDeps
	n := len(deps)
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(n),
		Display:    deprecatedDepDisplay(deps),
		Band:       result.BandInformational,
		Confidence: result.ConfidenceHigh,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: deprecatedDepDef,
	}
}

func deprecatedDepDisplay(deps []diagnostic.DeprecatedDep) string {
	n := len(deps)
	if n == 0 {
		return "0 declared deprecation markers"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d declared deprecation marker(s): ", n)
	for i, d := range deps {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", n-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%s)", d.Subject, d.Kind)
	}
	return b.String()
}
