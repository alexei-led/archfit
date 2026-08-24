package markdown

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// writeBCAdvisories renders coupling advisories in BC lint-message format.
// BC advisories (bc/imbalanced_coupling) get the structured lint-message block.
// All other advisories (staleness, map/*, labels/*) render as plain list items.
func writeBCAdvisories(b *strings.Builder, advisories []report.Finding) {
	var bcFindings, otherFindings []report.Finding
	for _, f := range advisories {
		if f.RuleID == "bc/imbalanced_coupling" {
			bcFindings = append(bcFindings, f)
		} else {
			otherFindings = append(otherFindings, f)
		}
	}

	if len(bcFindings) > 0 {
		edges := 0
		for _, f := range bcFindings {
			edges += rollupCount(f)
		}
		fmt.Fprintf(b, "\n## Balanced Coupling advisories (%d rollups, %d edges)\n\n", len(bcFindings), edges)
		b.WriteString("Same-shape edges between a module pair are grouped into one rollup.\n")
		b.WriteString("Integration strength × distance × volatility lint messages.\n")
		b.WriteString("Severity: `none` · `low` · `medium` · `high` · `critical`.\n\n")
		for i, f := range bcFindings {
			if i == 25 {
				fmt.Fprintf(b, "- ... +%d more rollups (use `--format json`)\n", len(bcFindings)-25)
				break
			}
			writeBCLintMessage(b, f)
		}
	}

	if len(otherFindings) > 0 {
		fmt.Fprintf(b, "\n## Advisories (%d)\n\n", len(otherFindings))
		for i, f := range otherFindings {
			if i == 25 {
				fmt.Fprintf(b, "- ... +%d more (use `--format json`)\n", len(otherFindings)-25)
				break
			}
			writeGateFinding(b, f)
		}
	}
}

// writeBCLintMessage renders one bc/imbalanced_coupling finding as a BC lint message:
//
//	ARCHFIT[BC-UNBALANCED <SEV>] from -> to  [<id8>]
//	  integration strength: <s>   distance: <d>   volatility: <v>
//	  score: <value>/10 (<band>) [<scorer>]
//	  why: <why>
//	  cheapest move: <move>
func writeBCLintMessage(b *strings.Builder, f report.Finding) {
	from := f.Edge.From.Path
	to := f.Edge.To.Path
	if from == "" {
		from = f.Edge.From.Module
	}
	if to == "" {
		to = f.Edge.To.Module
	}

	sev := strings.ToUpper(f.Severity)
	idShort := f.ID
	if len(idShort) > 8 {
		idShort = idShort[:8]
	}
	fmt.Fprintf(b, "```\nARCHFIT[BC-UNBALANCED %s] %s -> %s  [%s]\n", sev, from, to, idShort)

	// Strength / distance / volatility from MatchedBy.
	strength := f.MatchedBy["strength"]
	distance := f.MatchedBy["distance"]
	volatility := f.MatchedBy["volatility"]
	scorer := f.MatchedBy["score"] // scorer name (e.g. "multiplicative")
	scoreValue := f.MatchedBy["score_value"]
	scoreBand := f.MatchedBy["score_band"]
	cheapestMove := f.MatchedBy["cheapest_move"]
	why := strings.TrimSpace(f.Why)

	if strength != "" || distance != "" || volatility != "" {
		fmt.Fprintf(b, "  integration strength: %-12s  distance: %-30s  volatility: %s\n",
			orUnknown(strength), orUnknown(distance), orUnknown(volatility))
	}
	switch {
	case scoreValue != "":
		// Numeric maintenance-effort score: <value>/10 (<band>) [<scorer>].
		fmt.Fprintf(b, "  score: %s/10 (%s) [%s]\n", scoreValue, orUnknown(scoreBand), orUnknown(scorer))
	case scorer != "":
		fmt.Fprintf(b, "  score: %s\n", scorer)
	}
	if why != "" {
		if len(why) > 200 {
			why = why[:197] + "..."
		}
		fmt.Fprintf(b, "  why: %s\n", why)
	}
	if cheapestMove != "" {
		fmt.Fprintf(b, "  cheapest move: %s\n", cheapestMove)
	}
	if n := rollupCount(f); n > 1 {
		members := f.MatchedBy["group_members"]
		if members != "" {
			fmt.Fprintf(b, "  rollup: %d same-shape edges (e.g. %s)\n", n, members)
		} else {
			fmt.Fprintf(b, "  rollup: %d same-shape edges\n", n)
		}
	}
	b.WriteString("```\n\n")
}

// rollupCount returns the number of edges a BC advisory represents: the value of
// MatchedBy["group_count"] when the advisory is a rollup, else 1.
func rollupCount(f report.Finding) int {
	if n, err := strconv.Atoi(f.MatchedBy["group_count"]); err == nil && n > 0 {
		return n
	}
	return 1
}

// writeGateFinding prints one gate or non-BC advisory finding as a Markdown list item.
func writeGateFinding(b *strings.Builder, f report.Finding) {
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
