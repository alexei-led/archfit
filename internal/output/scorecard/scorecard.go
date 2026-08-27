// Package scorecard renders the nine-dimensional architecture-state scorecard:
// one block per dimension with its measurement status, gate posture,
// confidence, denominator, metrics, and what it could not observe.
//
// It is deliberately not an architecture score. There is no 0-100 value and no
// band, because averaging nine differently-measured dimensions into one number
// is exactly the claim this contract exists to stop making.
package scorecard

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
	reportports "github.com/alexei-led/archfit/internal/report/ports"
)

// Renderer formats a report document's architecture state as a scorecard.
type Renderer struct{}

var _ reportports.Renderer = (*Renderer)(nil)

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "scorecard".
func (r *Renderer) Format() string { return "scorecard" }

// Render writes the architecture-state scorecard.
func (r *Renderer) Render(d report.Document, w io.Writer) error {
	s := d.State
	var b strings.Builder

	b.WriteString("# archfit architecture state\n\n")
	fmt.Fprintf(&b, "**Verdict:** %s\n", verdictLabel(s.Verdict))
	fmt.Fprintf(&b, "**Hard gates:** %s — %d active blocker(s)\n", s.Decision.HardGates, s.Decision.ActiveBlockers)
	fmt.Fprintf(&b, "**Attention:** %d dimension(s) flagged\n", s.Decision.AttentionDimensions)
	fmt.Fprintf(&b, "**Coverage:** %d measured / %d partial / %d unmeasured (of %d)\n",
		s.Coverage.Measured, s.Coverage.Partial, s.Coverage.Unmeasured, report.DimensionCount)
	fmt.Fprintf(&b, "**Rubric version:** %s\n", s.Comparison.RubricVersion)
	if s.Comparison.ConfigHash != "" {
		fmt.Fprintf(&b, "**Config hash:** `%s`\n", s.Comparison.ConfigHash)
	}

	b.WriteString("\n## Dimensions\n")
	for _, dim := range s.Dimensions.All() {
		writeDimension(&b, dim)
	}

	writeComparison(&b, s.Comparison)
	writeDelta(&b, d.Delta)
	writeRequiredToolsMissing(&b, d.CoverageGaps)
	writeFindingIndex(&b, s.Findings)

	_, err := io.WriteString(w, b.String())
	return err
}

// writeFindingIndex appends every finding in the document's canonical order
// with its ID, lifecycle status, and rule. The scorecard is a per-dimension
// view and names no finding otherwise, so without this appendix it is the one
// format a reader cannot reconcile against the others.
func writeFindingIndex(b *strings.Builder, findings []report.Finding) {
	fmt.Fprintf(b, "\n## Finding index (%d)\n\n", len(findings))
	if len(findings) == 0 {
		b.WriteString("- none\n")
		return
	}
	for _, f := range findings {
		fmt.Fprintf(b, "- `%s` %s %s\n", f.ID, f.Status, f.RuleID)
	}
}

// verdictLabel renders the verdict for humans. JSON stores lower case.
func verdictLabel(v report.StateVerdict) string {
	switch v {
	case report.StateBlocked:
		return "BLOCKED"
	case report.StateHealthy:
		return "HEALTHY"
	case report.StateNeedsAttention:
		return "NEEDS ATTENTION"
	default:
		return strings.ToUpper(strings.ReplaceAll(string(v), "_", " "))
	}
}

func writeDimension(b *strings.Builder, dim report.DimensionState) {
	fmt.Fprintf(b, "\n### %s — %s · gate: %s · confidence: %s\n",
		dim.Name, dim.Status, dim.Gate, dim.Confidence)
	fmt.Fprintf(b, "owner: %s\n", dim.Owner)
	if dim.Coverage.Basis == "" {
		b.WriteString("denominator: none — this dimension measured nothing\n")
	} else {
		fmt.Fprintf(b, "denominator: %s %d/%d\n", dim.Coverage.Basis, dim.Coverage.Observed, dim.Coverage.Total)
	}
	for _, m := range dim.Metrics {
		fmt.Fprintf(b, "- %s: %s %s%s\n", m.Name, formatValue(m.Value), m.Unit, denominatorSuffix(m.Denominator))
	}
	for _, u := range dim.Unknown {
		fmt.Fprintf(b, "- not measured — %s: %s\n", u.Fact, strings.TrimSpace(u.Reason))
	}
	if dim.Delta != nil {
		fmt.Fprintf(b, "- delta: %s\n", dim.Delta.Status)
		for _, reason := range dim.Delta.Reasons {
			fmt.Fprintf(b, "  - %s\n", strings.TrimSpace(reason))
		}
	}
}

// formatValue prints a metric without trailing zeros, so a count reads as a
// count and a ratio keeps its precision.
func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func denominatorSuffix(d *report.MetricDenominator) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf(" (%d/%d)", d.Observed, d.Total)
}

func writeComparison(b *strings.Builder, c report.StateComparison) {
	b.WriteString("\n## Comparison\n\n")
	target := c.BaseRef
	if target == "" {
		target = "none"
	}
	fmt.Fprintf(b, "- status: %s\n- reference: %s\n", c.Status, target)
	for _, reason := range c.Reasons {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(reason))
	}
}

// writeDelta appends the finding-lifecycle bucket counts for a delta run, so a
// reader sees how many findings this change introduced, resolved, or merely
// touched versus pre-existing debt. Counts only — the per-finding lists live in
// the markdown and JSON output. Omitted outside delta mode.
func writeDelta(b *strings.Builder, delta *report.DeltaReport) {
	if delta == nil {
		return
	}
	b.WriteString("\n## Delta\n\n")
	for _, row := range []struct {
		label string
		ids   []string
	}{
		{"new", delta.New},
		{"severity changed", delta.SeverityChanged},
		{"touched by this change", delta.TouchedByDelta},
		{"pre-existing", delta.Existing},
		{"resolved", delta.Resolved},
	} {
		fmt.Fprintf(b, "- %s: %d\n", row.label, len(row.ids))
	}
}

// writeRequiredToolsMissing appends the coverage-gap block so a reader sees why
// dimensions are partial rather than mistaking absent evidence for a healthy
// result. One line per missing analyzer with the dimensions it unlocks and an
// install hint. Omitted when no tool is missing.
func writeRequiredToolsMissing(b *strings.Builder, gaps []report.CoverageGap) {
	if len(gaps) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Required tools missing (%d)\n\n", len(gaps))
	b.WriteString("These analyzers did not run; what they feed is unmeasured, not healthy.\n\n")
	for _, g := range gaps {
		fmt.Fprintf(b, "- **%s** [gate: %s] — affects %s; install: `%s`\n",
			g.Tool, g.Gate, strings.Join(g.AffectedMetrics, ", "), g.InstallCmd)
	}
}
