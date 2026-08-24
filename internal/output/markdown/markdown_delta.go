package markdown

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/scan"
)

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

const advisoryTaskMarkdownLimit = 25

// writeAdvisoryTasks prints report-only grouped advisory work items. These are
// separate from agent_tasks[] so advisory noise never masquerades as a gate repair.
func writeAdvisoryTasks(b *strings.Builder, tasks []diagnostic.AdvisoryTask) {
	if len(tasks) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Advisory tasks (%d)\n\n", len(tasks))
	b.WriteString("Report-only rollups from grouped advisories; these do not affect verdict or gate status.\n")
	shown := min(len(tasks), advisoryTaskMarkdownLimit)
	for _, task := range tasks[:shown] {
		fmt.Fprintf(b, "- **%s** [`%s`] %s\n", task.RuleID, task.FindingID[:min(8, len(task.FindingID))], task.Goal)
		fmt.Fprintf(b, "  - severity: %s; status: %s; group_count: %d\n", task.Severity, task.Status, task.GroupCount)
		if len(task.GroupMembers) > 0 {
			fmt.Fprintf(b, "  - group members: %s\n", strings.Join(task.GroupMembers, ", "))
		}
		if task.CheapestMove != "" {
			fmt.Fprintf(b, "  - cheapest move: %s\n", task.CheapestMove)
		}
		if task.ScoreValue > 0 {
			fmt.Fprintf(b, "  - score: %d/10\n", task.ScoreValue)
		}
		if len(task.TopFiles) > 0 {
			fmt.Fprintf(b, "  - top files: %s\n", strings.Join(task.TopFiles, ", "))
		}
		for _, c := range task.Constraints {
			fmt.Fprintf(b, "  - constraint: %s\n", c)
		}
		for _, v := range task.Validation {
			fmt.Fprintf(b, "  - validate: `%s`\n", v)
		}
	}
	if hidden := len(tasks) - shown; hidden > 0 {
		fmt.Fprintf(b, "\n_…and %d more advisory tasks (see --json for the full list)._\n", hidden)
	}
}

// writeDelta renders the delta-bucket section for a delta run: findings grouped
// by how they relate to the baseline (new / severity changed / touched by this
// change / pre-existing / resolved), so a reviewer can tell what the change
// introduced from what was already there. Omitted outside delta mode (d.Delta
// nil). Each bucket holds finding IDs that join back to d.Findings.
func writeDelta(b *strings.Builder, d scan.Diagnostic) {
	if d.Delta == nil {
		return
	}
	byID := make(map[string]finding.Finding, len(d.Findings))
	for _, f := range d.Findings {
		byID[f.ID] = f
	}

	b.WriteString("\n## Delta\n\n")
	b.WriteString("Findings grouped against the baseline so this change's impact is legible.\n")

	sections := []struct {
		title string
		ids   []string
	}{
		{"New", d.Delta.New},
		{"Severity changed", d.Delta.SeverityChanged},
		{"Touched by this change", d.Delta.TouchedByDelta},
		{"Pre-existing", d.Delta.Existing},
		{"Resolved", d.Delta.Resolved},
	}
	for _, s := range sections {
		if len(s.ids) == 0 {
			continue
		}
		fmt.Fprintf(b, "\n### %s (%d)\n\n", s.title, len(s.ids))
		fs := make([]finding.Finding, 0, len(s.ids))
		for _, id := range s.ids {
			if f, ok := byID[id]; ok {
				fs = append(fs, f)
			}
		}
		sort.SliceStable(fs, func(i, j int) bool { return findingLess(fs[i], fs[j]) })
		for _, f := range fs {
			writeDeltaFinding(b, f)
		}
	}
}

// writeDeltaFinding renders one finding as a compact delta-bucket line. The
// bucket already conveys lifecycle status, so the status is omitted here; the
// severity and edge are shown when present (fixed findings carry neither).
func writeDeltaFinding(b *strings.Builder, f finding.Finding) {
	sev := ""
	if f.Severity != "" {
		sev = fmt.Sprintf(" [%s]", f.Severity)
	}
	edge := ""
	if f.Edge.From.Path != "" || f.Edge.To.Path != "" {
		edge = fmt.Sprintf(" — %s → %s", f.Edge.From.Path, f.Edge.To.Path)
	}
	fmt.Fprintf(b, "- **%s**%s%s\n", f.RuleID, sev, edge)
}
