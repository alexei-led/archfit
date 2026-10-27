package agentout

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	reportports "github.com/alexei-led/archfit/v3/internal/report/ports"
)

// TextFormatName is the --format value this renderer serves for check.
const TextFormatName = "text"

// Prefixes agenttask puts on the constraint strings a rule's declared fields
// become; the text record splits them back into why and instead.
const (
	rationalePrefix   = "rationale: "
	alternativePrefix = "allowed alternative: "
)

// Footer words, one per change status.
var footerWord = map[ChangeStatus]string{ChangePass: "PASS", ChangeFail: "FAIL", ChangeUnknown: "UNKNOWN"}

// TextRenderer writes the linter-shaped text of `check`: one record per
// blocker, then a footer. It reads the same Result the agent JSON and the hooks
// read, so the three cannot disagree. The scorecard, dimensions and seam ledger
// live in `analyze`.
type TextRenderer struct{}

var _ reportports.Renderer = (*TextRenderer)(nil)

// NewText returns the check text renderer.
func NewText() *TextRenderer { return &TextRenderer{} }

// Format returns "text".
func (r *TextRenderer) Format() string { return TextFormatName }

// Render writes the records and the footer.
func (r *TextRenderer) Render(d report.Document, w io.Writer) error {
	res := bound(Build(d), textRunes, 0)
	var b strings.Builder
	for _, rep := range res.Change.Blockers {
		writeRecord(&b, rep)
	}
	for _, rule := range res.Change.UnevaluatedRules {
		fmt.Fprintf(&b, "%s: unknown: %s\n", rule.RuleID, rule.Reason)
	}
	for _, gap := range res.Change.EvidenceGaps {
		fmt.Fprintf(&b, "%s: unknown: required analyzer did not run (gate %s)", gap.Tool, gap.Gate)
		if gap.InstallCmd != "" {
			fmt.Fprintf(&b, "; install: %s", gap.InstallCmd)
		}
		b.WriteString("\n")
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s  %d new, %d unknown, %d accepted\n", footerWord[res.Change.Status],
		len(res.Change.Blockers), len(res.Change.UnevaluatedRules)+len(res.Change.EvidenceGaps), accepted(d.State.Findings))
	fmt.Fprintf(&b, "%s\n", res.Identity.Line())
	if len(res.Repository.Attention) > 0 {
		b.WriteString("\nrepository attention (not this change):\n")
		for _, item := range res.Repository.Attention {
			count := ""
			if item.Count > 0 {
				count = " (" + strconv.Itoa(item.Count) + ")"
			}
			fmt.Fprintf(&b, "  %s%s: %s\n", item.Kind, count, item.Reason)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeRecord writes one blocker: its first location and rule IDs on the
// header line, then why, instead, and status, indented.
func writeRecord(b *strings.Builder, rep Repair) {
	at := "-"
	if len(rep.At) > 0 {
		at = rep.At[0].File + ":" + strconv.Itoa(rep.At[0].Line)
	} else if rep.Edge != nil {
		at = rep.Edge.From
	}
	fmt.Fprintf(b, "%s: %s: %s\n", at, strings.Join(rep.RuleIDs, ", "), rep.Goal)
	if len(rep.At) > 1 {
		sites := make([]string, 0, len(rep.At)-1)
		for _, loc := range rep.At[1:] {
			sites = append(sites, loc.File+":"+strconv.Itoa(loc.Line))
		}
		fmt.Fprintf(b, "  also: %s\n", strings.Join(sites, ", "))
	}
	var rationale, rule, instead []string
	for _, c := range rep.Constraints {
		switch {
		case strings.HasPrefix(c, rationalePrefix):
			rationale = append(rationale, strings.TrimPrefix(c, rationalePrefix))
		case strings.HasPrefix(c, alternativePrefix):
			instead = append(instead, strings.TrimPrefix(c, alternativePrefix))
		default:
			rule = append(rule, c)
		}
	}
	// A rule without a rationale still says what it requires.
	for _, line := range slices.Concat(rationale, rule) {
		fmt.Fprintf(b, "  why: %s\n", line)
	}
	for _, line := range instead {
		fmt.Fprintf(b, "  instead: %s\n", line)
	}
	origin := rep.Origin
	if origin == "" {
		origin = report.FindingStatusNew
	}
	fmt.Fprintf(b, "  status: %s, %s\n", origin, rep.RepairKind)
}

// accepted counts the findings a baseline or a waiver already accepts.
func accepted(findings []report.Finding) int {
	n := 0
	for _, f := range findings {
		if f.Status == report.FindingStatusBaseline || f.Status == report.FindingStatusWaived {
			n++
		}
	}
	return n
}
