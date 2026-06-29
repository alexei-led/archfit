// Package markdown renders a Diagnostic as Balanced-Coupling-aligned Markdown:
// lint-message advisory format, BC vocabulary throughout, clearly-labeled
// "Supporting structural metrics (beyond Balanced Coupling)" and
// "Distance confidence" sections. Reads well both as raw text and rendered
// Markdown; parseable by AI agents without NLP.
package markdown

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// confidenceHigh is the confidence value that needs no qualification in output.
const confidenceHigh = "high"

// beyondBCMetrics is the set of metric names that belong in the
// "Supporting structural metrics (beyond Balanced Coupling)" section.
// These are report-only and never gate. Everything else appears in the
// primary Metrics section (coupling_balance, encapsulation, etc.).
var beyondBCMetrics = map[string]bool{
	"cycle":                true,
	"blast_radius":         true,
	"propagation_cost":     true,
	"instability":          true,
	"abstractness":         true,
	"martin_distance":      true,
	"change_coupling":      true,
	"change_amplification": true,
	"hidden_coupling":      true,
	"structural_weight":    true,
	"complexity":           true,
	"risk_hub":             true,
	"coverage":             true,
}

// Renderer formats a Diagnostic as BC-aligned Markdown. Satisfies engine.Renderer.
type Renderer struct{}

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "markdown".
func (r *Renderer) Format() string { return "markdown" }

// Render writes the BC-aligned Markdown report for d to w.
// Sections follow design §8:
//  1. Verdict + config_hash + tool/coverage
//  2. Gate violations (rules)
//  3. Balanced Coupling advisories — lint-message format
//  4. Supporting structural metrics (beyond Balanced Coupling)
//  5. Distance confidence
//  6. Agent tasks
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	var b strings.Builder
	verdict, exitCode := verdictLabel(d.Verdict)

	b.WriteString("# archfit report\n\n")
	fmt.Fprintf(&b, "**Verdict:** %s (exit %d)\n", verdict, exitCode)
	if d.ConfigHash != "" {
		fmt.Fprintf(&b, "**Config hash:** `%s`\n", d.ConfigHash)
	}

	b.WriteString("\n## Summary\n\n")
	fmt.Fprintf(&b, "- gate findings: %d\n", d.Summary.GateFindings)
	fmt.Fprintf(&b, "- warnings: %d\n", d.Summary.Warnings)
	fmt.Fprintf(&b, "- exceptions used: %d\n", d.Summary.ExceptionsUsed)

	writeDelta(&b, d)

	// Split metrics: BC-primary vs beyond-BC.
	var primaryMetrics, beyondMetrics []diagnostic.MetricResult
	for _, m := range d.Metrics {
		if beyondBCMetrics[m.Name] {
			beyondMetrics = append(beyondMetrics, m)
		} else {
			primaryMetrics = append(primaryMetrics, m)
		}
	}

	if len(primaryMetrics) > 0 {
		b.WriteString("\n## Metrics\n\n")
		for _, m := range primaryMetrics {
			band := m.Band
			if m.Confidence != "" && m.Confidence != confidenceHigh {
				band = fmt.Sprintf("%s (%s confidence)", band, m.Confidence)
			}
			fmt.Fprintf(&b, "- **%s**: %s — %s\n", m.Name, m.Display, band)
		}
	}

	writeFileFacts(&b, d.FileFacts)

	writeSyntaxSurface(&b, d.SyntaxFacts)

	writeDynamicImports(&b, d.DynamicImports)

	writeDeprecatedDeps(&b, d.DeprecatedDeps)

	writeCoverageGaps(&b, d.CoverageGaps)

	writeConfigWarnings(&b, d.ConfigWarnings)

	gate, advisories := splitFindings(d.Findings)
	if len(gate) > 0 {
		fmt.Fprintf(&b, "\n## Gate findings (%d)\n\n", len(gate))
		for _, f := range gate {
			writeGateFinding(&b, f)
		}
	}

	writeAgentTasks(&b, d.AgentTasks)

	if len(advisories) > 0 {
		writeBCAdvisories(&b, advisories)
	}

	if len(beyondMetrics) > 0 {
		writeBeyondBCMetrics(&b, beyondMetrics)
	}

	writeDistanceConfidence(&b, d)

	if len(d.ToolCoverage) > 0 {
		b.WriteString("\n## Coverage\n\n")
		for _, c := range d.ToolCoverage {
			extra := ""
			if c.FilesSeen > 0 {
				extra = fmt.Sprintf(" (%d files)", c.FilesSeen)
			}
			reason := ""
			if c.Reason != "" {
				reason = " — " + c.Reason
			}
			fmt.Fprintf(&b, "- %s: %s%s%s\n", c.Tool, c.Status, extra, reason)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func splitFindings(fs []finding.Finding) (gate, advisories []finding.Finding) {
	for _, f := range fs {
		if f.Kind == finding.KindGate {
			gate = append(gate, f)
		} else {
			advisories = append(advisories, f)
		}
	}
	// Both lists get a total deterministic order so output never depends on the
	// incoming slice order (which originates from map iteration upstream).
	sort.SliceStable(gate, func(i, j int) bool { return findingLess(gate[i], gate[j]) })
	sort.SliceStable(advisories, func(i, j int) bool { return findingLess(advisories[i], advisories[j]) })
	return gate, advisories
}

// findingLess orders findings deterministically: severity descending, then a
// stable tie-break chain (rule id, status, from path, to path, edge kind, id).
// id is a unique fingerprint, so the order is total — equal-severity findings
// no longer depend on input order.
func findingLess(a, b finding.Finding) bool {
	if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
		return ra > rb
	}
	if a.RuleID != b.RuleID {
		return a.RuleID < b.RuleID
	}
	if a.Status != b.Status {
		return a.Status < b.Status
	}
	if a.Edge.From.Path != b.Edge.From.Path {
		return a.Edge.From.Path < b.Edge.From.Path
	}
	if a.Edge.To.Path != b.Edge.To.Path {
		return a.Edge.To.Path < b.Edge.To.Path
	}
	if a.Edge.Kind != b.Edge.Kind {
		return a.Edge.Kind < b.Edge.Kind
	}
	return a.ID < b.ID
}

func severityRank(s finding.Severity) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func verdictLabel(v diagnostic.Verdict) (string, int) {
	switch v {
	case diagnostic.VerdictFail:
		return "fail", 1
	case diagnostic.VerdictWarn:
		return "warn", 2
	default:
		return "pass", 0
	}
}

// orUnknown returns s if non-empty, else "unknown".
func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
