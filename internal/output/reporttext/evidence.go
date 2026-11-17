package reporttext

import (
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v4/internal/model/report"
)

// writeEvidence prints only fixed vocabulary and counts, never untrusted paths.
func writeEvidence(b *strings.Builder, e report.ReportEvidence) {
	b.WriteString("evidence:\n  snapshot: ")
	switch e.Snapshot.Stability {
	case "stable_by_pre_post_check":
		b.WriteString("stable by pre/post check")
	case "changed":
		b.WriteString("changed")
	default:
		b.WriteString("unknown")
	}
	b.WriteString("\n  policy binding: ")
	switch e.Snapshot.PolicyBinding {
	case "match":
		b.WriteString("match")
	case "mismatch":
		b.WriteString("mismatch")
	default:
		b.WriteString("unknown")
	}
	for _, gap := range e.Snapshot.Unknowns {
		if gap.Fact == "report_context" {
			b.WriteString("\n  relationships: unknown\n  interface inventory: unknown\n  snapshot gaps: " + strconv.Itoa(len(e.Snapshot.Unknowns)) + "\n")
			return
		}
	}
	b.WriteString("\n  seam-eligible edges: " + strconv.Itoa(e.PairPopulation.TotalEdges) + ", all retained edges: " + strconv.Itoa(e.Connascence.TotalEdges) + "\n")
	counts := map[string]int{}
	for _, c := range e.Components {
		counts[c.Volatility.Origin]++
	}
	b.WriteString("  volatility origins: asserted " + strconv.Itoa(counts["asserted"]) + ", assessed " + strconv.Itoa(counts["assessed"]) + ", assumed " + strconv.Itoa(counts["assumed"]) + ", unknown " + strconv.Itoa(counts["unknown"]) + "\n")
	b.WriteString("  interface inventory: unknown\n")
	if n := len(e.Snapshot.Unknowns); n > 0 {
		b.WriteString("  snapshot gaps: " + strconv.Itoa(n) + "\n")
	}
}
