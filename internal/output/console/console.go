// Package console renders a Diagnostic as clean, LLM-friendly plain text:
// labelled sections of aligned key/value lines, no box-drawing tables.
package console

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// Renderer prints a structured, human- and LLM-readable Diagnostic summary.
type Renderer struct{}

// New returns a new Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "text".
func (r *Renderer) Format() string { return "text" }

// Render writes the diagnostic as labelled sections (verdict, summary, metrics,
// gate findings, advisories, coverage). Lines are aligned with spaces so the
// output stays readable and machine-parseable without table formatting.
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	var b strings.Builder
	verdict, exitCode := verdictLabel(d.Verdict)

	fmt.Fprintf(&b, "archfit  verdict: %s  (exit %d)\n", verdict, exitCode)

	b.WriteString("\nsummary:\n")
	writeKV(&b, "gate findings", strconv.Itoa(d.Summary.GateFindings))
	writeKV(&b, "warnings", strconv.Itoa(d.Summary.Warnings))
	writeKV(&b, "exceptions used", strconv.Itoa(d.Summary.ExceptionsUsed))

	if len(d.Metrics) > 0 {
		b.WriteString("\nmetrics:\n")
		nameW := maxLen(metricNames(d.Metrics))
		for _, m := range d.Metrics {
			band := m.Band
			if m.Confidence != "" && m.Confidence != "high" {
				band = fmt.Sprintf("%s, %s confidence", band, m.Confidence)
			}
			fmt.Fprintf(&b, "  %-*s  %s  [%s]\n", nameW, m.Name, m.Display, band)
		}
	}

	gate, advisories := splitFindings(d.Findings)
	if len(gate) > 0 {
		fmt.Fprintf(&b, "\ngate findings (%d):\n", len(gate))
		for _, f := range gate {
			writeFinding(&b, f)
		}
	}
	if len(advisories) > 0 {
		fmt.Fprintf(&b, "\nadvisories (%d, top by severity):\n", len(advisories))
		for i, f := range advisories {
			if i == 15 {
				fmt.Fprintf(&b, "  ... +%d more (see --format json)\n", len(advisories)-15)
				break
			}
			writeFinding(&b, f)
		}
	}

	if len(d.ToolCoverage) > 0 {
		b.WriteString("\ncoverage:\n")
		toolW := maxLen(toolNames(d.ToolCoverage))
		for _, c := range d.ToolCoverage {
			extra := ""
			if c.FilesSeen > 0 {
				extra = fmt.Sprintf("  %d files", c.FilesSeen)
			}
			fmt.Fprintf(&b, "  %-*s  %s%s\n", toolW, c.Tool, c.Status, extra)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// writeFinding prints one finding as a single readable line.
func writeFinding(b *strings.Builder, f finding.Finding) {
	edge := ""
	if f.Edge.From.Path != "" || f.Edge.To.Path != "" {
		edge = fmt.Sprintf("  %s -> %s", f.Edge.From.Path, f.Edge.To.Path)
	}
	why := strings.TrimSpace(f.Why)
	if len(why) > 100 {
		why = why[:97] + "..."
	}
	if why != "" {
		why = "  " + why
	}
	fmt.Fprintf(b, "  - %s/%s  %s%s%s\n", f.RuleID, f.Severity, f.Status, edge, why)
}

func writeKV(b *strings.Builder, k, v string) {
	fmt.Fprintf(b, "  %s: %s\n", k, v)
}

// splitFindings separates active gate findings from advisories, advisories sorted
// by severity (most severe first) for a useful head.
func splitFindings(fs []finding.Finding) (gate, advisories []finding.Finding) {
	for _, f := range fs {
		if f.Kind == "gate" {
			gate = append(gate, f)
		} else {
			advisories = append(advisories, f)
		}
	}
	sort.SliceStable(advisories, func(i, j int) bool {
		return severityRank(advisories[i].Severity) > severityRank(advisories[j].Severity)
	})
	return gate, advisories
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

func metricNames(ms []diagnostic.MetricResult) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

func toolNames(cs []diagnostic.Coverage) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Tool
	}
	return out
}

func maxLen(ss []string) int {
	n := 0
	for _, s := range ss {
		if len(s) > n {
			n = len(s)
		}
	}
	return n
}

// verdictLabel returns the verdict word (upper-case) and exit code.
func verdictLabel(v diagnostic.Verdict) (string, int) {
	switch v {
	case diagnostic.VerdictFail:
		return "FAIL", 1
	case diagnostic.VerdictWarn:
		return "WARN", 2
	default:
		return "PASS", 0
	}
}
