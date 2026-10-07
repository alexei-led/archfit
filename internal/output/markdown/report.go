package markdown

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/output/brief"
)

// RenderState writes the architecture state as the Markdown report's headline:
// the decision, the blockers and next steps, the nine dimension envelopes, the coverage summary, the coupling
// seam ledger, the comparison and its comparability reasons, and what could not
// be measured. It leads the detailed audit that Render produces.
//
// The same facts appear here as in --format json; only the layout differs. There
// is no repository score, because there is no repository score.
//
// The state alone carries no metric deltas, so it cannot name a tripped
// metric ratchet; Render, which holds the whole document, can.
func RenderState(s report.ArchitectureState, w io.Writer) error {
	return writeState(s, brief.Build(brief.Input{State: s}), w)
}

func writeState(s report.ArchitectureState, view brief.View, w io.Writer) error {
	var b strings.Builder

	b.WriteString("# archfit — architecture state\n\n")
	writeStateHeadline(&b, s, view.VerdictReason)
	writeBlockers(&b, view.Blockers)
	writeNextSteps(&b, view.NextSteps)
	writeDimensionTable(&b, s.Dimensions)
	writeDimensionMetrics(&b, s.Dimensions)
	writeCoverageTable(&b, s.Coverage)
	writeSeamLedger(&b, s.Seams)
	writeDiagnostics(&b, view.Diagnostics)
	writeStateComparison(&b, s.Comparison)
	writeGateReference(&b, s.GateReference)
	writeStateUnknowns(&b, s.Dimensions, view)

	_, err := io.WriteString(w, b.String())
	return err
}

func writeGateReference(b *strings.Builder, c *report.StateComparison) {
	if c == nil {
		return
	}
	fmt.Fprintf(b, "\n## Gate reference\n\n- **Status:** %s\n- **Reference:** `%s`\n", c.Status, c.BaseRef)
	for _, reason := range c.Reasons {
		fmt.Fprintf(b, "- %s\n", reason)
	}
}

func writeStateHeadline(b *strings.Builder, s report.ArchitectureState, reason string) {
	_, diagnostics := statePopulations(s.Dimensions)
	verdict := stateVerdictPhrase(s.Verdict)
	if reason != "" {
		verdict += " — " + reason
	}
	fmt.Fprintf(b, "- **Verdict:** %s\n", verdict)
	fmt.Fprintf(b, "- **Blocking:** %d active — hard gates: %s\n", s.Decision.ActiveBlockers, s.Decision.HardGates)
	fmt.Fprintf(b, "- **Attention:** %d dimension(s) flagged — %d diagnostic(s)\n", s.Decision.AttentionDimensions, diagnostics)
	fmt.Fprintf(b, "- **Coverage:** %d measured / %d partial / %d unmeasured (of %d)\n",
		s.Coverage.Measured, s.Coverage.Partial, s.Coverage.Unmeasured, report.DimensionCount)
	for _, rule := range s.Decision.UnevaluatedRequiredRules {
		fmt.Fprintf(b, "- **Required rule not evaluated:** `%s` — %s\n", rule.RuleID, rule.Reason)
	}
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
			mdTableCell(dim.Name), dim.Status, dim.Gate, dim.Confidence,
			mdTableCell(stateCoverageCell(dim.Coverage)), len(dim.Findings))
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
		fmt.Fprintf(b, "| %s | %s | %s |\n", mdTableCell(tool.Tool), tool.Status, mdTableCell(reason))
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
		fmt.Fprintf(b, "| %s | %s | %s | %s | %d | %d | %s | %s | %s |\n",
			mdTableCell(name), s.Strength, s.Distance, s.Volatility,
			s.ScoredEdges, s.CriticalEdges, seamMedianCell(s.Scores),
			dash(s.Quadrant), mdTableCell(dash(s.Hypothesis)))
	}
}

// seamMedianCell renders the balance median. A seam whose every edge abstained
// has no distribution at all, and printing the zero value as "0" would publish a
// balance score below the book's 1..10 range as if it had been measured.
func seamMedianCell(d report.SeamScoreDistribution) string {
	if d.N == 0 {
		return "—"
	}
	return strconv.Itoa(d.Median)
}

func dash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

// diagnosticListCap bounds the rendered diagnostics; blockers are never
// capped, and the full list is in --format json.
const diagnosticListCap = 20

// writeBlockers lists every active blocker with its short ID, rule, subject,
// first location, full why, repair goal, and check commands.
func writeBlockers(b *strings.Builder, blockers []brief.Blocker) {
	if len(blockers) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Blockers (%d)\n\n", len(blockers))
	for _, bl := range blockers {
		fmt.Fprintf(b, "- **`%s` %s** — %s\n", bl.ShortID, bl.RuleID, bl.Subject)
		if bl.Location != "" {
			more := ""
			if bl.MoreLocations > 0 {
				more = fmt.Sprintf(" (+%d more)", bl.MoreLocations)
			}
			fmt.Fprintf(b, "  - location: `%s`%s\n", bl.Location, more)
		}
		fmt.Fprintf(b, "  - why: %s\n", oneLine(bl.Why))
		if bl.Goal != "" {
			fmt.Fprintf(b, "  - goal: %s\n", oneLine(bl.Goal))
		}
		for _, check := range bl.Checks {
			fmt.Fprintf(b, "  - check: `%s`\n", check)
		}
	}
}

func writeNextSteps(b *strings.Builder, steps []string) {
	if len(steps) == 0 {
		return
	}
	b.WriteString("\n## Next steps\n\n")
	for i, step := range steps {
		fmt.Fprintf(b, "%d. %s\n", i+1, step)
	}
}

func writeDiagnostics(b *strings.Builder, findings []report.Finding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Diagnostics (%d)\n\n", len(findings))
	for i, f := range findings {
		if i == diagnosticListCap {
			fmt.Fprintf(b, "\n_… +%d more (see `--format json`)_\n", len(findings)-diagnosticListCap)
			break
		}
		fmt.Fprintf(b, "- **%s** [%s] — %s\n", f.RuleID, f.Severity, oneLine(f.Why))
	}
}

// oneLine folds a multi-line text onto one line without cutting it.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// writeFindingIndex appends every finding in the document's canonical order
// with its ID, lifecycle status, and rule. The actionable list above is capped
// for readability; without this appendix a capped list and a shorter run render
// identically, and the finding sequence would differ between formats.
func writeFindingIndex(b *strings.Builder, findings []report.Finding) {
	fmt.Fprintf(b, "\n## Finding index (%d)\n\n", len(findings))
	if len(findings) == 0 {
		b.WriteString("_none_\n")
		return
	}
	b.WriteString("| Finding | Status | Rule |\n| --- | --- | --- |\n")
	for _, f := range findings {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", f.ID, f.Status, f.RuleID)
	}
}

func writeStateComparison(b *strings.Builder, c report.StateComparison) {
	b.WriteString("\n## Comparison\n\n")
	target := c.BaseRef
	if target == "" {
		target = "none"
	}
	fmt.Fprintf(b, "- **Status:** %s\n- **Reference:** %s\n", c.Status, target)
	if c.ConfigHash != "" {
		fmt.Fprintf(b, "- **Config hash:** `%s`\n", c.ConfigHash)
	}
	for _, reason := range c.Reasons {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(reason))
	}
}

func writeStateUnknowns(b *strings.Builder, dims report.Dimensions, view brief.View) {
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
		fmt.Fprintf(b, "- **%s — %s** (owner: %s): %s %s\n", r.dimension, r.fact.Fact, r.fact.Owner, strings.TrimSpace(r.fact.Reason), view.StepFor(r.fact.Fact))
	}
}
