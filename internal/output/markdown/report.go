package markdown

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// RenderState writes the architecture state as the Markdown report's headline:
// the decision, the nine dimension envelopes, the coverage summary, the coupling
// seam ledger, the comparison and its comparability reasons, and what could not
// be measured. It leads the detailed audit that Render produces.
//
// The same facts appear here as in --format json; only the layout differs. There
// is no repository score, because there is no repository score.
func RenderState(s report.ArchitectureState, w io.Writer) error {
	var b strings.Builder

	b.WriteString("# archfit — architecture state\n\n")
	writeStateHeadline(&b, s)
	writeDimensionTable(&b, s.Dimensions)
	writeCoverageTable(&b, s.Coverage)
	writeSeamLedger(&b, s.Seams)
	writeActionableFindings(&b, s)
	writeStateComparison(&b, s.Comparison)
	writeStateUnknowns(&b, s.Dimensions)

	_, err := io.WriteString(w, b.String())
	return err
}

func writeStateHeadline(b *strings.Builder, s report.ArchitectureState) {
	_, diagnostics := statePopulations(s.Dimensions)
	fmt.Fprintf(b, "- **Verdict:** %s\n", stateVerdictPhrase(s.Verdict))
	fmt.Fprintf(b, "- **Blocking:** %d active — hard gates: %s\n", s.Decision.ActiveBlockers, s.Decision.HardGates)
	fmt.Fprintf(b, "- **Attention:** %d dimension(s) flagged — %d diagnostic(s)\n", s.Decision.AttentionDimensions, diagnostics)
	fmt.Fprintf(b, "- **Coverage:** %d measured / %d partial / %d unmeasured (of %d)\n",
		s.Coverage.Measured, s.Coverage.Partial, s.Coverage.Unmeasured, report.DimensionCount)
}

// statePopulations counts the two active populations from the dimension
// envelopes, which reference only active findings. Counting the published refs
// keeps this renderer from re-deriving the lifecycle predicate and reaching a
// different answer than the run did.
func statePopulations(dims report.Dimensions) (blockers, diagnostics int) {
	seen := map[string]struct{}{}
	for _, dim := range dims.All() {
		for _, ref := range dim.Findings {
			if _, dup := seen[ref.ID]; dup {
				continue
			}
			seen[ref.ID] = struct{}{}
			if ref.Kind == report.FindingKindGate {
				blockers++
				continue
			}
			diagnostics++
		}
	}
	return blockers, diagnostics
}

func stateVerdictPhrase(v report.StateVerdict) string {
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

func writeDimensionTable(b *strings.Builder, dims report.Dimensions) {
	b.WriteString("\n## Dimensions\n\n")
	b.WriteString("| Dimension | Status | Gate | Confidence | Denominator | Findings |\n")
	b.WriteString("| --- | --- | --- | --- | --- | ---: |\n")
	for _, dim := range dims.All() {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %d |\n",
			dim.Name, dim.Status, dim.Gate, dim.Confidence, stateCoverageCell(dim.Coverage), len(dim.Findings))
	}
}

// stateCoverageCell renders a dimension's denominator. An unmeasured envelope
// has no basis, and printing "0/0" would read as measured-and-empty.
func stateCoverageCell(c report.DimensionCoverage) string {
	if c.Basis == "" {
		return "_no denominator_"
	}
	return fmt.Sprintf("%s %d/%d", c.Basis, c.Observed, c.Total)
}

func writeCoverageTable(b *strings.Builder, c report.StateCoverage) {
	if len(c.Tools) == 0 {
		return
	}
	b.WriteString("\n## Evidence coverage\n\n")
	b.WriteString("| Tool | Status | Reason |\n| --- | --- | --- |\n")
	for _, tool := range c.Tools {
		reason := tool.Reason
		if reason == "" {
			reason = "—"
		}
		fmt.Fprintf(b, "| %s | %s | %s |\n", tool.Tool, tool.Status, reason)
	}
}

// seamLedgerCap bounds the rendered ledger; the full list is in --format json.
const seamLedgerCap = 20

func writeSeamLedger(b *strings.Builder, seams []report.Seam) {
	if len(seams) == 0 {
		return
	}
	ranked := append([]report.Seam(nil), seams...)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, c := ranked[i], ranked[j]
		if a.DistributedMonolith != c.DistributedMonolith {
			return a.DistributedMonolith
		}
		if a.CriticalEdges != c.CriticalEdges {
			return a.CriticalEdges > c.CriticalEdges
		}
		return a.ID < c.ID
	})

	fmt.Fprintf(b, "\n## Coupling seams (%d)\n\n", len(seams))
	b.WriteString("| Seam | Strength | Distance | Volatility | Scored | Critical | Median | Quadrant | Try |\n")
	b.WriteString("| --- | --- | --- | --- | ---: | ---: | ---: | --- | --- |\n")
	for i, s := range ranked {
		if i == seamLedgerCap {
			fmt.Fprintf(b, "\n_… +%d more seams (see `--format json`)_\n", len(ranked)-seamLedgerCap)
			break
		}
		name := s.FromModule + " → " + s.ToModule
		if s.DistributedMonolith {
			name += " ⚠"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %d | %d | %d | %s | %s |\n",
			name, s.Strength, s.Distance, s.Volatility, s.ScoredEdges, s.CriticalEdges,
			s.Scores.Median, dash(s.Quadrant), dash(s.Hypothesis))
	}
}

func dash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

// findingListCap bounds each rendered population; the full list is in
// --format json.
const findingListCap = 20

func writeActionableFindings(b *strings.Builder, s report.ArchitectureState) {
	blocking, advisory := splitActionable(s)
	if len(blocking) == 0 && len(advisory) == 0 {
		return
	}
	b.WriteString("\n## Top actionable findings\n")
	writeFindingList(b, "Blocking", blocking)
	writeFindingList(b, "Diagnostic", advisory)
}

// splitActionable selects the findings the dimension envelopes reference, in
// the document's own finding order. Only active findings are referenced, so a
// baselined or waived one cannot reappear here as work to do.
func splitActionable(s report.ArchitectureState) (blocking, advisory []report.Finding) {
	active := map[string]string{}
	for _, dim := range s.Dimensions.All() {
		for _, ref := range dim.Findings {
			active[ref.ID] = ref.Kind
		}
	}
	for _, f := range s.Findings {
		kind, ok := active[f.ID]
		if !ok {
			continue
		}
		if kind == report.FindingKindGate {
			blocking = append(blocking, f)
			continue
		}
		advisory = append(advisory, f)
	}
	return blocking, advisory
}

func writeFindingList(b *strings.Builder, label string, findings []report.Finding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s (%d)\n\n", label, len(findings))
	for i, f := range findings {
		if i == findingListCap {
			fmt.Fprintf(b, "\n_… +%d more (see `--format json`)_\n", len(findings)-findingListCap)
			break
		}
		fmt.Fprintf(b, "- **%s** [%s] — %s\n", f.RuleID, f.Severity, strings.TrimSpace(strings.ReplaceAll(f.Why, "\n", " ")))
	}
}

func writeStateComparison(b *strings.Builder, c report.StateComparison) {
	b.WriteString("\n## Comparison\n\n")
	target := c.BaseRef
	if target == "" {
		target = "none"
	}
	fmt.Fprintf(b, "- **Status:** %s\n- **Reference:** %s\n", c.Status, target)
	for _, reason := range c.Reasons {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(reason))
	}
}

func writeStateUnknowns(b *strings.Builder, dims report.Dimensions) {
	type row struct {
		dimension string
		fact      report.UnknownFact
	}
	var rows []row
	for _, dim := range dims.All() {
		for _, u := range dim.Unknown {
			rows = append(rows, row{dim.Name, u})
		}
	}
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Not measured (%d)\n\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(b, "- **%s — %s** (owner: %s): %s\n", r.dimension, r.fact.Fact, r.fact.Owner, strings.TrimSpace(r.fact.Reason))
	}
}
