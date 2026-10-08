package console

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/output/brief"
	reportports "github.com/alexei-led/archfit/v3/internal/report/ports"
)

// Renderer writes a report document's architecture state as terminal text.
type Renderer struct{}

var _ reportports.Renderer = (*Renderer)(nil)

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "console".
func (r *Renderer) Format() string { return "console" }

// Render writes the document's architecture state as terminal text.
func (r *Renderer) Render(d report.Document, w io.Writer) error {
	view := brief.Build(brief.Input{State: d.State, CoverageGaps: d.CoverageGaps})
	return writeState(d.State, view, w)
}

// RenderState writes the architecture state as terminal-native plain text: the
// headline, the blockers with their location, goal, and check command, the
// next steps, the nine dimension envelopes, what could not be measured and the
// step that closes it, the coupling seam ledger, the diagnostics, and the
// comparison.
//
// There is no repository score and no "why the score is low" section, because
// there is no repository score: the decision is the verdict, and what a reader
// acts on is a named blocker, a flagged dimension, or an unmeasured one. No
// Markdown, no wide tables, no color — scannable in a terminal and safe to pipe
// (timing and progress live on stderr, not here).
func RenderState(s report.ArchitectureState, w io.Writer) error {
	return writeState(s, brief.Build(brief.Input{State: s}), w)
}

func writeState(s report.ArchitectureState, view brief.View, w io.Writer) error {
	var b strings.Builder

	b.WriteString("ARCHITECTURE STATE\n\n")
	writeHeadline(&b, s, view.VerdictReason)
	writeBlockers(&b, view.Blockers)
	writeNextSteps(&b, view.NextSteps)
	writeDimensions(&b, s.Dimensions)
	writeUnknowns(&b, s.Dimensions, view)
	writeSeams(&b, s.Seams)
	writeDiagnostics(&b, view.Diagnostics)
	writeComparison(&b, s.Comparison)
	writeGateReference(&b, s.GateReference)
	writeFindingIndex(&b, s.Findings)

	_, err := io.WriteString(w, b.String())
	return err
}

// headlineKeyWidth aligns the headline key column (VERDICT, BLOCKING, …).
const headlineKeyWidth = 10

func writeGateReference(b *strings.Builder, c *report.StateComparison) {
	if c == nil {
		return
	}
	fmt.Fprintf(b, "\nGATE REFERENCE\n\n  status: %s  ·  reference: %s\n", c.Status, c.BaseRef)
	for _, reason := range c.Reasons {
		fmt.Fprintf(b, "    %s\n", condense(reason, 140))
	}
}

func writeHeadline(b *strings.Builder, s report.ArchitectureState, reason string) {
	blockers, diagnostics := findingPopulations(s.Dimensions)
	verdict := verdictLabel(s.Verdict)
	if reason != "" {
		verdict += " — " + reason
	}
	kv(b, "VERDICT", verdict)
	kv(b, "BLOCKING", fmt.Sprintf("%d active  ·  hard gates: %s", s.Decision.ActiveBlockers, s.Decision.HardGates))
	kv(b, "ATTENTION", fmt.Sprintf("%d dimension(s) flagged  ·  %d diagnostic(s)", s.Decision.AttentionDimensions, diagnostics))
	kv(b, "COVERAGE", fmt.Sprintf("%d measured · %d partial · %d unmeasured  (of %d)",
		s.Coverage.Measured, s.Coverage.Partial, s.Coverage.Unmeasured, report.DimensionCount))
	// The reassurance is keyed on the VERDICT, never on the finding count alone.
	// A required-tool policy failure blocks without producing a finding, so
	// "no blockers … not to stop development" over a BLOCKED run tells the reader to ignore the exit code the same run
	// just returned.
	switch {
	case s.Verdict == report.StateBlocked && blockers == 0:
		b.WriteString("\nBlocked by a hard gate that produces no finding — a required\nanalyzer. See the dimension(s) below reporting gate: fail.\n")
	case s.Decision.HardGates == report.HardGateUnmeasured:
		b.WriteString("\nRequired architecture checks could not be completed. Supply the\nmissing evidence before treating this run as a passed gate.\n")
	case blockers == 0:
		b.WriteString("\nNo blockers. Use this run for architecture-improvement planning,\nnot to stop development.\n")
	}
	for _, rule := range s.Decision.UnevaluatedRequiredRules {
		fmt.Fprintf(b, "  not evaluated: %s — %s\n", rule.RuleID, rule.Reason)
	}
}

// findingPopulations counts the two active populations from the dimension
// envelopes. The envelopes reference only active findings, so the counts cannot
// disagree with the decision: the renderer reads the published refs instead of
// re-deriving the lifecycle predicate over the whole finding list.
func findingPopulations(dims report.Dimensions) (blockers, diagnostics int) {
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

func kv(b *strings.Builder, k, v string) {
	fmt.Fprintf(b, "%-*s %s\n", headlineKeyWidth, k, v)
}

// verdictLabel renders the verdict for humans. JSON stores lower case; the
// terminal displays upper case.
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

// dimensionNameWidth aligns the dimension column.
const dimensionNameWidth = 15

func writeDimensions(b *strings.Builder, dims report.Dimensions) {
	b.WriteString("\nDIMENSIONS\n\n")
	for _, dim := range dims.All() {
		fmt.Fprintf(b, "  %-*s %-11s gate: %-15s confidence: %-8s %s\n",
			dimensionNameWidth, dim.Name, dim.Status, dim.Gate, dim.Confidence, coverageLabel(dim.Coverage))
		for _, metric := range dim.Metrics {
			fmt.Fprintf(b, "    %s: %s %s%s\n", metric.Name, formatMetricValue(metric.Value), metric.Unit, metricDenominator(metric.Denominator))
		}
	}
}

func formatMetricValue(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func metricDenominator(d *report.MetricDenominator) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf(" (%d/%d)", d.Observed, d.Total)
}

// coverageLabel renders a dimension's denominator. An unmeasured envelope has
// no basis, and printing "0/0" there would read as a measured-and-empty result.
func coverageLabel(c report.DimensionCoverage) string {
	if c.Basis == "" {
		return "no denominator"
	}
	return fmt.Sprintf("%s %d/%d", c.Basis, c.Observed, c.Total)
}

func writeUnknowns(b *strings.Builder, dims report.Dimensions, view brief.View) {
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
	fmt.Fprintf(b, "\nNOT MEASURED (%d)\n\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(b, "  %s — %s\n    %s\n    %s\n", r.dimension, r.fact.Fact, condense(r.fact.Reason, 140), view.StepFor(r.fact.Fact))
	}
}

// seamCap bounds how many seams the terminal lists. The full ledger is always
// in --format json.
const seamCap = 8

func writeSeams(b *strings.Builder, seams []report.Seam) {
	if len(seams) == 0 {
		return
	}
	ranked := rankSeams(seams)
	fmt.Fprintf(b, "\nCOUPLING SEAMS (%d)\n\n", len(seams))
	for i, s := range ranked {
		if i == seamCap {
			fmt.Fprintf(b, "  … +%d more (see --format json)\n", len(ranked)-seamCap)
			break
		}
		marker := ""
		if s.DistributedMonolith {
			marker = "  [distributed monolith]"
		}
		fmt.Fprintf(b, "  %s -> %s%s\n", s.FromModule, s.ToModule, marker)
		fmt.Fprintf(b, "    %s × %s × %s volatility · %d critical of %d scored · median balance %s\n",
			s.Strength, s.Distance, s.Volatility, s.CriticalEdges, s.ScoredEdges, seamMedian(s.Scores))
		if s.Hypothesis != "" {
			fmt.Fprintf(b, "    try: %s\n", s.Hypothesis)
		}
	}
}

// seamMedian renders the balance median. A seam whose every edge abstained has
// no distribution at all, and printing the zero value as "0" would publish a
// balance below the book's 1..10 range as if it had been measured.
func seamMedian(d report.SeamScoreDistribution) string {
	if d.N == 0 {
		return "n/a"
	}
	return strconv.Itoa(d.Median)
}

// rankSeams orders the ledger worst-first for display: qualifying seams, then
// critical edge count, then the stable seam ID. It never reorders the published
// ledger — the caller's slice is left alone.
func rankSeams(seams []report.Seam) []report.Seam {
	out := append([]report.Seam(nil), seams...)
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		if a.DistributedMonolith != c.DistributedMonolith {
			return a.DistributedMonolith
		}
		if a.CriticalEdges != c.CriticalEdges {
			return a.CriticalEdges > c.CriticalEdges
		}
		return a.ID < c.ID
	})
	return out
}

// diagnosticCap bounds how many diagnostics the terminal lists; blockers are
// never capped.
const diagnosticCap = 8

// writeBlockers lists every active blocker: its short ID, rule, and subject;
// the first location; the full why; the repair goal; and the commands that
// check the fix.
func writeBlockers(b *strings.Builder, blockers []brief.Blocker) {
	if len(blockers) == 0 {
		return
	}
	fmt.Fprintf(b, "\nBLOCKERS (%d)\n", len(blockers))
	for _, bl := range blockers {
		fmt.Fprintf(b, "\n  %s %s  %s\n", bl.ShortID, bl.RuleID, bl.Subject)
		if bl.Location != "" {
			more := ""
			if bl.MoreLocations > 0 {
				more = fmt.Sprintf("  (+%d more)", bl.MoreLocations)
			}
			fmt.Fprintf(b, "    %s%s\n", bl.Location, more)
		}
		if bl.Origin != "" {
			fmt.Fprintf(b, "    origin: %s\n", bl.Origin)
		}
		fmt.Fprintf(b, "    why:   %s\n", oneLine(bl.Why))
		if bl.Goal != "" {
			fmt.Fprintf(b, "    goal:  %s\n", oneLine(bl.Goal))
		}
		for _, check := range bl.Checks {
			fmt.Fprintf(b, "    check: %s\n", check)
		}
	}
}

func writeNextSteps(b *strings.Builder, steps []string) {
	if len(steps) == 0 {
		return
	}
	b.WriteString("\nNEXT STEPS\n\n")
	for i, step := range steps {
		fmt.Fprintf(b, "  %d. %s\n", i+1, step)
	}
}

func writeDiagnostics(b *strings.Builder, findings []report.Finding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(b, "\nDIAGNOSTICS (%d)\n\n", len(findings))
	for i, f := range findings {
		if i == diagnosticCap {
			fmt.Fprintf(b, "  … +%d more (see --format json)\n", len(findings)-diagnosticCap)
			break
		}
		fmt.Fprintf(b, "  · %s [%s] — %s\n", f.RuleID, f.Severity, condense(f.Why, 100))
	}
}

// oneLine folds a multi-line text onto one line without cutting it.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func writeComparison(b *strings.Builder, c report.StateComparison) {
	b.WriteString("\nCOMPARISON\n\n")
	target := c.BaseRef
	if target == "" {
		target = "none"
	}
	fmt.Fprintf(b, "  status: %s  ·  reference: %s\n", c.Status, target)
	for _, reason := range c.Reasons {
		fmt.Fprintf(b, "    %s\n", condense(reason, 140))
	}
	if c.OriginStatus == "" {
		return
	}
	fmt.Fprintf(b, "  origin: %s  ·  introduced: %d  ·  resolved: %d\n",
		c.OriginStatus, idCount(c.IntroducedFindingIDs), idCount(c.ResolvedFindingIDs))
	for _, reason := range c.OriginReasons {
		fmt.Fprintf(b, "    %s\n", condense(reason, 140))
	}
}

// idCount counts an optional ID list; nil means the list was not requested.
func idCount(ids *[]string) int {
	if ids == nil {
		return 0
	}
	return len(*ids)
}

// writeFindingIndex appends every finding in the document's canonical order
// with its ID, lifecycle status, and rule. The actionable section above is
// deliberately abbreviated for a terminal reader; this appendix is what makes
// the abbreviation safe, because a truncated list is indistinguishable from a
// shorter run otherwise. Every format carries the same sequence, including
// accepted and waived findings, so no reader can pick the format that omits
// the finding they would rather not see.
func writeFindingIndex(b *strings.Builder, findings []report.Finding) {
	fmt.Fprintf(b, "\nFINDING INDEX (%d)\n\n", len(findings))
	if len(findings) == 0 {
		b.WriteString("  none\n")
		return
	}
	for _, f := range findings {
		fmt.Fprintf(b, "  %s  %s  %s\n", f.ID, f.Status, f.RuleID)
	}
}

// condense trims whitespace and caps s to maxLen bytes with an ellipsis.
func condense(s string, maxLen int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return cutAtRuneBoundary(s, maxLen)
	}
	return cutAtRuneBoundary(s, maxLen-1) + "…"
}

// cutAtRuneBoundary returns s truncated to at most n bytes, backing the cut off
// to the nearest rune start. Slicing a UTF-8 string at an arbitrary byte index
// splits a multi-byte rune and emits invalid UTF-8, which is not merely ugly: a
// consumer that decodes the document strictly fails on the WHOLE document, so
// one truncated "×" in one advisory loses the entire report.
func cutAtRuneBoundary(s string, n int) string {
	if n >= len(s) {
		return s
	}
	if n < 0 {
		return ""
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
