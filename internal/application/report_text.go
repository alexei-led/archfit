package application

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// Report text bounds. A strict state consumer (the archfit App) rejects any
// string holding a control character or a line/paragraph separator, caps free
// text at 500 runes and an agent task's goal and constraints at 4096, and a
// single rejected string invalidates the whole report. Tool stderr, error
// chains, and config globs reach these fields verbatim, so every free-text
// field is bounded here, once, below those caps.
const (
	maxReportTextRunes     = 400
	maxReportTaskTextRunes = 3600
)

// reportEllipsis marks text the projection shortened; the full text went to stderr.
const reportEllipsis = "…"

// reportText returns s as one line of at most limit runes. Text that already
// satisfies both rules is returned unchanged, so a valid report stays byte for
// byte what it was. Otherwise ANSI colour sequences are dropped, every run of
// whitespace and control characters becomes one space, and text over limit
// keeps its leading runes — the tool name, exit code, and first error line —
// and ends in an ellipsis.
func reportText(s string, limit int) string {
	if !strings.ContainsFunc(s, unsafeReportRune) && utf8.RuneCountInString(s) <= limit {
		return s
	}
	var b strings.Builder
	pendingSpace := false
	for i := 0; i < len(s); {
		if n := ansiSequenceLen(s[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) <= limit {
		return out
	}
	runes := []rune(out)[:limit-utf8.RuneCountInString(reportEllipsis)]
	return strings.TrimRight(string(runes), " ") + reportEllipsis
}

// unsafeReportRune is the consumer's single-line rule: no control character and
// no Unicode line or paragraph separator.
func unsafeReportRune(r rune) bool {
	return unicode.IsControl(r) || r == ' ' || r == ' '
}

// ansiSequenceLen returns the byte length of the ANSI CSI sequence (ESC '['
// parameters, intermediates, final byte) at the start of s, or 0.
func ansiSequenceLen(s string) int {
	if len(s) < 2 || s[0] != '\x1b' || s[1] != '[' {
		return 0
	}
	for i := 2; i < len(s); i++ {
		if c := s[i]; c >= 0x40 && c <= 0x7e {
			return i + 1
		}
	}
	return len(s)
}

// reportTexts bounds every element into a new slice, so the projection never
// rewrites the assessment slices it was handed. nil stays nil and an empty
// list stays empty: the two serialise differently.
func reportTexts(in []string, limit int) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = reportText(s, limit)
	}
	return out
}

// boundReportText applies the report text bounds to every free-text field of
// the projected document: the state contract and the legacy blocks the
// Markdown and text renderers read. Identity material — finding and rule IDs,
// hashes, paths, tool versions, validation commands — is never rewritten: a
// shortened ID or command would be a different one.
func boundReportText(doc *report.Document) {
	for i := range doc.Findings {
		f := &doc.Findings[i]
		f.Why = reportText(f.Why, maxReportTextRunes)
		f.Constraint = reportText(f.Constraint, maxReportTextRunes)
		f.Alternatives = reportTexts(f.Alternatives, maxReportTextRunes)
	}
	for i := range doc.AgentTasks {
		t := &doc.AgentTasks[i]
		t.Goal = reportText(t.Goal, maxReportTaskTextRunes)
		t.Constraints = reportTexts(t.Constraints, maxReportTaskTextRunes)
	}
	for i := range doc.AdvisoryTasks {
		t := &doc.AdvisoryTasks[i]
		t.Goal = reportText(t.Goal, maxReportTaskTextRunes)
		t.Hypothesis = reportText(t.Hypothesis, maxReportTaskTextRunes)
		t.Constraints = reportTexts(t.Constraints, maxReportTaskTextRunes)
	}
	for i := range doc.ToolCoverage {
		doc.ToolCoverage[i].Reason = reportText(doc.ToolCoverage[i].Reason, maxReportTextRunes)
	}
	doc.ConfigWarnings = reportTexts(doc.ConfigWarnings, maxReportTextRunes)

	st := &doc.State
	st.Findings, st.AgentTasks = doc.Findings, doc.AgentTasks
	for i := range st.Decision.UnevaluatedRequiredRules {
		rule := &st.Decision.UnevaluatedRequiredRules[i]
		rule.Reason = reportText(rule.Reason, maxReportTextRunes)
	}
	boundComparisonText(&st.Comparison)
	if st.GateReference != nil {
		boundComparisonText(st.GateReference)
	}
	for _, dim := range []*report.DimensionState{
		&st.Dimensions.Intent, &st.Dimensions.Structure, &st.Dimensions.Modularity, &st.Dimensions.Coupling,
		&st.Dimensions.ChangeLocality, &st.Dimensions.Complexity, &st.Dimensions.Testability,
		&st.Dimensions.Operations, &st.Dimensions.Drift,
	} {
		dim.Coverage.Basis = reportText(dim.Coverage.Basis, maxReportTextRunes)
		for i := range dim.Unknown {
			dim.Unknown[i].Fact = reportText(dim.Unknown[i].Fact, maxReportTextRunes)
			dim.Unknown[i].Reason = reportText(dim.Unknown[i].Reason, maxReportTextRunes)
		}
		if dim.Delta != nil {
			delta := *dim.Delta
			delta.Reasons = reportTexts(delta.Reasons, maxReportTextRunes)
			dim.Delta = &delta
		}
	}
	for i := range st.Coverage.Tools {
		st.Coverage.Tools[i].Reason = reportText(st.Coverage.Tools[i].Reason, maxReportTextRunes)
	}
}

func boundComparisonText(c *report.StateComparison) {
	c.Reasons = reportTexts(c.Reasons, maxReportTextRunes)
	c.OriginReasons = reportTexts(c.OriginReasons, maxReportTextRunes)
}

// discloseRawCoverageReasons writes, on stderr, the full reason of every
// coverage row the report will shorten or flatten. The report keeps one bounded
// line; the analyzer's own output stays readable where an operator looks for it.
func discloseRawCoverageReasons(w io.Writer, label string, rows []evidence.Coverage) {
	for _, row := range rows {
		if row.Reason == "" || reportText(row.Reason, maxReportTextRunes) == row.Reason {
			continue
		}
		_, _ = fmt.Fprintf(w, "warning: %s%s is %s; full analyzer output (the report keeps one shortened line):\n%s\n",
			label, row.Tool, row.Status, strings.TrimRight(row.Reason, "\n"))
	}
}
