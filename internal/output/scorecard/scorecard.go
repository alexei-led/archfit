// Package scorecard renders a Diagnostic as the architect skill's seven-dimension
// banded scorecard: an overall 0-100 value plus one block per dimension with its
// value, band, confidence, evidence references, and a one-line summary. The
package scorecard

import (
	"fmt"
	"io"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/model/scan"
)

// Renderer formats a Diagnostic as a banded scorecard. Satisfies engine.Renderer.
type Renderer struct{}

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "scorecard".
func (r *Renderer) Format() string { return "scorecard" }

// Render writes the supplied scorecard and diagnostic details.
func (r *Renderer) Render(d scan.Diagnostic, sc report.Scorecard, w io.Writer) error {
	var b strings.Builder
	b.WriteString("# archfit scorecard\n\n")
	fmt.Fprintf(&b, "**Rubric version:** %d\n", sc.RubricVersion)
	if sc.OverallBand.Unmeasured() {
		fmt.Fprintf(&b, "**Overall:** n/a — coupling unmeasured (no scored cross-boundary edges)\n")
	} else {
		fmt.Fprintf(&b, "**Overall:** %d/100 (%s)\n", sc.Overall, sc.OverallBand)
	}
	if d.ConfigHash != "" {
		fmt.Fprintf(&b, "**Config hash:** `%s`\n", d.ConfigHash)
	}

	b.WriteString("\n## Dimensions\n")
	for _, dim := range sc.Dimensions {
		meta := ""
		if dim.Meta {
			meta = " · meta (scores the review, not the architecture)"
		}
		if dim.Band.Unmeasured() {
			fmt.Fprintf(&b, "\n### %s — n/a (unmeasured) · confidence: %s%s\n",
				dim.Name, dim.Confidence, meta)
		} else {
			fmt.Fprintf(&b, "\n### %s — %d/100 (%s) · confidence: %s%s\n",
				dim.Name, dim.Value, dim.Band, dim.Confidence, meta)
		}
		if dim.Summary != "" {
			fmt.Fprintf(&b, "%s\n", dim.Summary)
		}
		for _, e := range dim.Evidence {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}

	writeDelta(&b, d.Delta)

	writeRequiredToolsMissing(&b, d.CoverageGaps)

	_, err := io.WriteString(w, b.String())
	return err
}

// writeDelta appends a compact delta-bucket count summary for a delta run, so a
// scorecard reader sees how many findings this change introduced, resolved, or
// merely touched versus pre-existing debt. Counts only — the per-finding lists
// live in the markdown/json output. Omitted outside delta mode (delta nil).
func writeDelta(b *strings.Builder, delta *report.DeltaReport) {
	if delta == nil {
		return
	}
	b.WriteString("\n## Delta\n\n")
	rows := []struct {
		label string
		ids   []string
	}{
		{"new", delta.New},
		{"severity changed", delta.SeverityChanged},
		{"touched by this change", delta.TouchedByDelta},
		{"pre-existing", delta.Existing},
		{"resolved", delta.Resolved},
	}
	for _, r := range rows {
		fmt.Fprintf(b, "- %s: %d\n", r.label, len(r.ids))
	}
}

// writeRequiredToolsMissing appends the coverage-gap block to the scorecard so a
// reader sees why dimensions are n/a rather than mistaking absent evidence for a
// strong result. One line per missing analyzer with the dimensions it unlocks
// and an install hint. Omitted when no tool is missing.
func writeRequiredToolsMissing(b *strings.Builder, gaps []evidence.CoverageGap) {
	if len(gaps) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Required tools missing (%d)\n\n", len(gaps))
	b.WriteString("These analyzers did not run; the metrics they feed are n/a, not strong.\n\n")
	for _, g := range gaps {
		fmt.Fprintf(b, "- **%s** [gate: %s] — affects %s; install: `%s`\n",
			g.Tool, g.Gate, strings.Join(g.AffectedMetrics, ", "), g.InstallCmd)
	}
}
