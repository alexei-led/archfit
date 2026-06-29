package markdown

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/decision"
)

// RenderReport writes a concise, decision-led Markdown summary for the
// `analyze --markdown` artifact: decision band, gate/advisory counts, score,
// categorized recommendations, per-dimension "why low / what moves it", and an
// optional delta. It is meant to lead the detailed audit that Render produces;
// callers write this first, then the full Render(diag) detail.
func RenderReport(r decision.Report, w io.Writer) error {
	var b strings.Builder

	b.WriteString("# archfit — decision\n\n")
	fmt.Fprintf(&b, "- **Decision:** %s\n", decisionPhrase(r.Band))
	gate := "PASS"
	if r.Blocking > 0 {
		gate = "FAIL"
	}
	fmt.Fprintf(&b, "- **Gate:** %s — %d blocking\n", gate, r.Blocking)
	fmt.Fprintf(&b, "- **Warnings:** %d advisory\n", r.Advisory)
	fmt.Fprintf(&b, "- **Score:** %d / 100 (%s)\n", r.Overall, r.OverallBand)
	if r.Headline != "" {
		fmt.Fprintf(&b, "\n%s\n", r.Headline)
	}

	writeRecs(&b, r.Recommendations)
	writeLowDims(&b, r.Dimensions)
	writeReportDelta(&b, r.Delta)

	_, err := io.WriteString(w, b.String())
	return err
}

func decisionPhrase(band decision.Band) string {
	switch band {
	case decision.BandFail:
		return "FAIL"
	case decision.BandNeedsAttention:
		return "NEEDS ATTENTION"
	case decision.BandHealthy:
		return "HEALTHY"
	default:
		return "ACCEPTABLE WITH WATCH ITEMS"
	}
}

func writeRecs(b *strings.Builder, recs decision.Recommendations) {
	b.WriteString("\n## Recommendations\n")
	writeRecGroup(b, "Must fix", recs.MustFix)
	writeRecGroup(b, "Should fix", recs.ShouldFix)
	writeRecGroup(b, "Watch", recs.Watch)
	if len(recs.Calibrate) > 0 {
		writeRecGroup(b, "Calibrate", recs.Calibrate)
	}
	if len(recs.Ignore) > 0 {
		writeRecGroup(b, "Ignore", recs.Ignore)
	}
}

func writeRecGroup(b *strings.Builder, label string, recs []decision.Rec) {
	fmt.Fprintf(b, "\n### %s\n", label)
	if len(recs) == 0 {
		b.WriteString("- none\n")
		return
	}
	for _, rec := range recs {
		title := rec.RuleID
		if title == "" {
			title = rec.Title
		}
		if d := strings.TrimSpace(rec.Detail); d != "" {
			fmt.Fprintf(b, "- **%s** — %s\n", title, d)
		} else {
			fmt.Fprintf(b, "- **%s**\n", title)
		}
	}
}

func writeLowDims(b *strings.Builder, dims []decision.DimReport) {
	var low []decision.DimReport
	for _, d := range dims {
		if !d.Meta && d.Value <= 60 {
			low = append(low, d)
		}
	}
	if len(low) == 0 {
		return
	}
	b.WriteString("\n## Why the score is low\n")
	for _, d := range low {
		fmt.Fprintf(b, "\n- **%s** (%d/100, %s)", d.Name, d.Value, d.Band)
		if d.Why != "" {
			fmt.Fprintf(b, ": %s", strings.TrimSpace(d.Why))
		}
		b.WriteString("\n")
		if d.WhatMoves != "" {
			fmt.Fprintf(b, "  - _What moves it:_ %s\n", d.WhatMoves)
		}
	}
}

func writeReportDelta(b *strings.Builder, d *decision.Delta) {
	if d == nil {
		return
	}
	b.WriteString("\n## Change vs base\n\n")
	fmt.Fprintf(b, "- **overall:** %s\n", signedReportDelta(d.Overall))
	for _, dim := range d.Dimensions {
		if dim.Change == 0 {
			continue
		}
		fmt.Fprintf(b, "- **%s:** %d → %d (%s)\n", dim.Name, dim.Before, dim.After, signedReportDelta(dim.Change))
	}
}

func signedReportDelta(n int) string {
	switch {
	case n > 0:
		return fmt.Sprintf("+%d", n)
	case n == 0:
		return "no change"
	default:
		return strconv.Itoa(n)
	}
}
