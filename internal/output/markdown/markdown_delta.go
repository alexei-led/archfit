package markdown

import (
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// writeAgentTasks prints the structured repair-task block: one entry per
// active gate finding, with goal, involved files, constraints, and the exact
// validation command. Omitted when there are no active gate findings.
func writeAgentTasks(b *strings.Builder, tasks []report.AgentTask) {
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
func writeAdvisoryTasks(b *strings.Builder, tasks []report.AdvisoryTask) {
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
		if task.Hypothesis != "" {
			fmt.Fprintf(b, "  - hypothesis: %s\n", task.Hypothesis)
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
