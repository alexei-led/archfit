// Package markdown renders a Diagnostic as clean, LLM-friendly Markdown:
// `##` sections and `-` lists, no box-drawing tables (which do not align in raw
// text). Reads well both as raw text and rendered Markdown.
package markdown

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// Renderer formats a Diagnostic as Markdown. Satisfies engine.Renderer.
type Renderer struct{}

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "markdown".
func (r *Renderer) Format() string { return "markdown" }

// Render writes the Markdown report for d to w.
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	var b strings.Builder
	verdict, exitCode := verdictLabel(d.Verdict)

	b.WriteString("# archfit report\n\n")
	fmt.Fprintf(&b, "**Verdict:** %s (exit %d)\n", verdict, exitCode)

	b.WriteString("\n## Summary\n\n")
	fmt.Fprintf(&b, "- gate findings: %d\n", d.Summary.GateFindings)
	fmt.Fprintf(&b, "- warnings: %d\n", d.Summary.Warnings)
	fmt.Fprintf(&b, "- exceptions used: %d\n", d.Summary.ExceptionsUsed)

	if len(d.Metrics) > 0 {
		b.WriteString("\n## Metrics\n\n")
		for _, m := range d.Metrics {
			band := m.Band
			if m.Confidence != "" && m.Confidence != "high" {
				band = fmt.Sprintf("%s (%s confidence)", band, m.Confidence)
			}
			fmt.Fprintf(&b, "- **%s**: %s — %s\n", m.Name, m.Display, band)
		}
	}

	writeFileFacts(&b, d.FileFacts)

	gate, advisories := splitFindings(d.Findings)
	if len(gate) > 0 {
		fmt.Fprintf(&b, "\n## Gate findings (%d)\n\n", len(gate))
		for _, f := range gate {
			writeFinding(&b, f)
		}
	}

	writeAgentTasks(&b, d.AgentTasks)
	if len(advisories) > 0 {
		fmt.Fprintf(&b, "\n## Advisories (%d, top by severity)\n\n", len(advisories))
		for i, f := range advisories {
			if i == 25 {
				fmt.Fprintf(&b, "- ... +%d more (use `--format json`)\n", len(advisories)-25)
				break
			}
			writeFinding(&b, f)
		}
	}

	if len(d.ToolCoverage) > 0 {
		b.WriteString("\n## Coverage\n\n")
		for _, c := range d.ToolCoverage {
			extra := ""
			if c.FilesSeen > 0 {
				extra = fmt.Sprintf(" (%d files)", c.FilesSeen)
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", c.Tool, c.Status, extra)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// writeAgentTasks prints the structured repair-task block: one entry per
// active gate finding, with goal, involved files, constraints, and the exact
// validation command. Omitted when there are no active gate findings.
func writeAgentTasks(b *strings.Builder, tasks []diagnostic.AgentTask) {
	if len(tasks) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Agent tasks (%d)\n\n", len(tasks))
	for _, task := range tasks {
		fmt.Fprintf(b, "- **%s** [`%s`] %s\n", task.RuleID, task.FindingID[:min(8, len(task.FindingID))], task.Goal)
		if len(task.Files) > 0 {
			fmt.Fprintf(b, "  - files: %s\n", strings.Join(task.Files, ", "))
		}
		for _, c := range task.Constraints {
			fmt.Fprintf(b, "  - constraint: %s\n", c)
		}
		for _, v := range task.Validation {
			fmt.Fprintf(b, "  - validate: `%s`\n", v)
		}
	}
}

// fileFactsTopN is the number of modules listed per axis in the structural-facts section.
const fileFactsTopN = 5

// writeFileFacts prints the neutral structural-facts block: the top modules by
// each axis (inbound module fan-in, outbound destinations, LOC). Numbers only —
// no risk labels or ranking verdicts; the full per-module list is in the JSON
// output. Omitted entirely when no facts were collected (SCIP off/absent).
func writeFileFacts(b *strings.Builder, facts []diagnostic.FileFact) {
	if len(facts) == 0 {
		return
	}
	b.WriteString("\n## Structural facts (neutral evidence)\n\n")
	fmt.Fprintf(b, "%d modules; top %d per axis (full list in `--format json`):\n\n", len(facts), fileFactsTopN)

	axes := []struct {
		label string
		value func(diagnostic.FileFact) int
	}{
		{"inbound module fan-in", func(f diagnostic.FileFact) int { return f.InboundModuleFanIn }},
		{"outbound destinations", func(f diagnostic.FileFact) int { return f.OutboundDestinations }},
		{"LOC", func(f diagnostic.FileFact) int { return f.LOC }},
	}
	for _, axis := range axes {
		ranked := make([]diagnostic.FileFact, len(facts))
		copy(ranked, facts)
		sort.SliceStable(ranked, func(i, j int) bool {
			if v1, v2 := axis.value(ranked[i]), axis.value(ranked[j]); v1 != v2 {
				return v1 > v2
			}
			return ranked[i].Module < ranked[j].Module
		})
		fmt.Fprintf(b, "- %s:", axis.label)
		for i, f := range ranked {
			if i == fileFactsTopN || axis.value(f) == 0 {
				break
			}
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(b, " %s (%d)", f.Module, axis.value(f))
		}
		b.WriteString("\n")
	}
}

// writeFinding prints one finding as a single Markdown list item.
func writeFinding(b *strings.Builder, f finding.Finding) {
	edge := ""
	if f.Edge.From.Path != "" || f.Edge.To.Path != "" {
		edge = fmt.Sprintf(" — %s → %s", f.Edge.From.Path, f.Edge.To.Path)
	}
	why := strings.TrimSpace(f.Why)
	if len(why) > 140 {
		why = why[:137] + "..."
	}
	if why != "" {
		why = ": " + why
	}
	fmt.Fprintf(b, "- **%s** [%s] %s%s%s\n", f.RuleID, f.Severity, f.Status, edge, why)
}

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
