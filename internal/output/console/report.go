package console

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
	reportports "github.com/alexei-led/archfit/internal/report/ports"
)

// labelFail is the gate/verdict fail label, shared with the legacy verdict
// renderer so the literal is defined once.
const labelFail = "FAIL"

// Renderer writes a report document's projected decision as terminal text.
type Renderer struct{}

var _ reportports.Renderer = (*Renderer)(nil)

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "console".
func (r *Renderer) Format() string { return "console" }

// Render writes the document's projected decision as terminal text.
func (r *Renderer) Render(d report.Document, w io.Writer) error {
	return RenderReport(d.Decision, w)
}

// RenderReport writes a decision.Report as terminal-native plain text: a
// decision-led summary block, categorized recommendations, and per-dimension
// "why low / what moves it" sections. No Markdown, no wide tables, no color —
// scannable in a terminal and safe to pipe (timing/progress live on stderr, not
// here). This is the default human output for `archfit analyze`.
func RenderReport(r report.Report, w io.Writer) error {
	var b strings.Builder

	b.WriteString("ARCHFIT RESULT\n\n")
	writeResultHeader(&b, r)

	if r.Headline != "" {
		fmt.Fprintf(&b, "\n%s\n", r.Headline)
	}
	if r.Blocking == 0 {
		b.WriteString("\nNo blockers. Use this run for architecture-improvement planning,\nnot to stop development.\n")
	}

	writeRecommendations(&b, r.Recommendations)
	writeLowDimensions(&b, r.Dimensions)
	if r.Delta != nil {
		writeDelta(&b, *r.Delta)
	}
	writeTargets(&b, r)

	_, err := io.WriteString(w, b.String())
	return err
}

// resultKeyWidth aligns the header key column ("Decision", "Warnings", …).
const resultKeyWidth = 10

func writeResultHeader(b *strings.Builder, r report.Report) {
	gate := "PASS"
	if r.Blocking > 0 {
		gate = labelFail
	}
	rkv(b, "Decision", decisionLabel(r.Band))
	rkv(b, "Gate", fmt.Sprintf("%s  ·  %d blocking", gate, r.Blocking))
	rkv(b, "Warnings", fmt.Sprintf("%d advisory", r.Advisory))
	if r.OverallBand.Unmeasured() {
		rkv(b, "Score", "n/a  ·  coupling unmeasured (no scored cross-boundary edges)")
	} else {
		rkv(b, "Score", fmt.Sprintf("%d / 100  %s", r.Overall, r.OverallBand))
	}
}

// rkv writes one aligned "Key   value" header line.
func rkv(b *strings.Builder, k, v string) {
	fmt.Fprintf(b, "%-*s %s\n", resultKeyWidth, k, v)
}

// decisionLabel renders the decision band as a human-readable phrase.
func decisionLabel(band report.DecisionBand) string {
	switch band {
	case report.DecisionBandFail:
		return labelFail
	case report.DecisionBandNeedsAttention:
		return "NEEDS ATTENTION"
	case report.DecisionBandHealthy:
		return "HEALTHY"
	default: // BandAcceptable
		return "ACCEPTABLE WITH WATCH ITEMS"
	}
}

// recCap bounds how many items are listed per recommendation category before a
// "+N more" pointer; the full set is always in --json.
const recCap = 8

func writeRecommendations(b *strings.Builder, recs report.Recommendations) {
	b.WriteString("\nRECOMMENDATIONS\n")
	writeRecCategory(b, "MUST FIX", recs.MustFix)
	writeRecCategory(b, "SHOULD FIX", recs.ShouldFix)
	writeRecCategory(b, "WATCH", recs.Watch)
	// CALIBRATE / IGNORE are populated only by the LLM layer; show when present.
	if len(recs.Calibrate) > 0 {
		writeRecCategory(b, "CALIBRATE", recs.Calibrate)
	}
	if len(recs.Ignore) > 0 {
		writeRecCategory(b, "IGNORE", recs.Ignore)
	}
}

func writeRecCategory(b *strings.Builder, label string, recs []report.Rec) {
	fmt.Fprintf(b, "\n  %s\n", label)
	if len(recs) == 0 {
		b.WriteString("    none\n")
		return
	}
	for i, rec := range recs {
		if i == recCap {
			fmt.Fprintf(b, "    … +%d more (see --json)\n", len(recs)-recCap)
			break
		}
		title := rec.Title
		if rec.RuleID != "" {
			title = rec.RuleID
		}
		detail := condense(rec.Detail, 90)
		if detail != "" {
			fmt.Fprintf(b, "    · %s — %s\n", title, detail)
		} else {
			fmt.Fprintf(b, "    · %s\n", title)
		}
	}
}

// lowBandCeiling: dimensions at or below this value (mixed or worse) get the
// "why low / what moves it" treatment. Serviceable (61+) is healthy enough.
const lowBandCeiling = 60

// lowDimensions returns the non-meta dimensions at or below the low-band ceiling,
// sorted by value ascending (worst first).
func lowDimensions(dims []report.DimReport) []report.DimReport {
	var out []report.DimReport
	for _, d := range dims {
		if d.Meta || d.Value > lowBandCeiling {
			continue
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

func writeLowDimensions(b *strings.Builder, dims []report.DimReport) {
	low := lowDimensions(dims)
	if len(low) == 0 {
		return
	}

	b.WriteString("\nWHY THE SCORE IS LOW\n")
	for _, d := range low {
		fmt.Fprintf(b, "\n  %s  %d/100  [%s]\n", d.Name, d.Value, d.Band)
		if d.Why != "" {
			fmt.Fprintf(b, "    %s\n", condense(d.Why, 160))
		}
	}

	b.WriteString("\nWHAT WOULD IMPROVE THE SCORE\n")
	for _, d := range low {
		if d.WhatMoves == "" {
			continue
		}
		fmt.Fprintf(b, "\n  %s\n    %s\n", d.Name, d.WhatMoves)
	}
}

func writeDelta(b *strings.Builder, d report.Delta) {
	fmt.Fprintf(b, "\nCHANGE VS BASE\n\n")
	fmt.Fprintf(b, "  overall  %s\n", signedChange(d.Overall))
	for _, dim := range d.Dimensions {
		if dim.Change == 0 {
			continue
		}
		fmt.Fprintf(b, "  %s  %d → %d  (%s)\n", dim.Name, dim.Before, dim.After, signed(dim.Change))
	}
}

func writeTargets(b *strings.Builder, r report.Report) {
	b.WriteString("\nTARGETS\n")
	rkvIndent(b, "Current", fmt.Sprintf("%d  %s", r.Overall, r.OverallBand))
	if label, rng := nextBand(r.OverallBand); label != "" {
		rkvIndent(b, "Near-term", fmt.Sprintf("%s  %s", rng, label))
	}
	rkvIndent(b, "Main goal", "keep blocking findings at 0")
}

const targetKeyWidth = 11

func rkvIndent(b *strings.Builder, k, v string) {
	fmt.Fprintf(b, "  %-*s %s\n", targetKeyWidth, k, v)
}

// nextBand returns the next-healthier band label and its 0-100 range, or "" when
// already at the top band.
func nextBand(b report.ScoreBand) (label, rng string) {
	switch b {
	case report.ScoreBandCritical:
		return string(report.ScoreBandPoor), "21-40"
	case report.ScoreBandPoor:
		return string(report.ScoreBandMixed), "41-60"
	case report.ScoreBandMixed:
		return string(report.ScoreBandServiceable), "61-80"
	case report.ScoreBandServiceable:
		return string(report.ScoreBandStrong), "81-100"
	default: // strong or unknown
		return "", ""
	}
}

// condense trims whitespace and caps s to maxLen runes with an ellipsis.
func condense(s string, maxLen int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}

// signed renders a signed integer with an explicit + for non-negative values.
func signed(n int) string {
	if n >= 0 {
		return "+" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// signedChange renders a 0 as "no change" and non-zero as a signed delta.
func signedChange(n int) string {
	if n == 0 {
		return "no change"
	}
	return signed(n)
}
