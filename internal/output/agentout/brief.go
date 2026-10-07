package agentout

import (
	"fmt"
	"strconv"
	"strings"
)

// Brief renders the result as short plain text for a hook's stderr or system
// message: the next action, then each listed repair with its IDs, locations,
// goal, constraints, and files, then the other blockers and the validation
// command. Lists and free text are bounded as in an over-budget result.
func Brief(r Result) string {
	r = bound(r, tightRunes, tightListLen)
	var b strings.Builder
	fmt.Fprintf(&b, "archfit: next_action %s (%s)\n", r.NextAction, r.Summary)
	for i, rep := range r.Repairs {
		scope := "in scope"
		if !rep.InScope {
			scope = "outside scope"
		}
		fmt.Fprintf(&b, "repair %d (%s, %s): %s [%s]\n", i+1, rep.RepairKind, scope, strings.Join(rep.RuleIDs, ", "), strings.Join(rep.FindingIDs, ", "))
		if len(rep.At) > 0 {
			sites := make([]string, 0, len(rep.At))
			for _, loc := range rep.At {
				sites = append(sites, loc.File+":"+strconv.Itoa(loc.Line))
			}
			fmt.Fprintf(&b, "  at: %s%s\n", strings.Join(sites, ", "), more(rep.AtOmitted))
		}
		fmt.Fprintf(&b, "  goal: %s\n", rep.Goal)
		for _, c := range rep.Constraints {
			fmt.Fprintf(&b, "  constraint: %s\n", c)
		}
		if len(rep.Edit) > 0 {
			fmt.Fprintf(&b, "  edit: %s%s\n", strings.Join(rep.Edit, ", "), more(rep.EditOmitted))
		}
	}
	for _, rule := range r.UnevaluatedRules {
		fmt.Fprintf(&b, "unevaluated rule: %s: %s\n", rule.RuleID, rule.Reason)
	}
	for _, gap := range r.EvidenceGaps {
		fmt.Fprintf(&b, "evidence gap: %s (gate %s)\n", gap.Tool, gap.Gate)
	}
	if r.Validate != "" {
		fmt.Fprintf(&b, "validate: %s\n", r.Validate)
	}
	return b.String()
}

func more(omitted int) string {
	if omitted == 0 {
		return ""
	}
	return fmt.Sprintf(" (+%d more)", omitted)
}
