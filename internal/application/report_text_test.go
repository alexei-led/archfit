package application

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	reporttest "github.com/alexei-led/archfit/v3/internal/testutil/report"
)

const (
	textToolDepcruise       = "dependency-cruiser"
	textToolGrimp           = "grimp"
	textStatusNonComparable = "non_comparable"
)

// toolStderr is two kilobytes of the multi-line stderr a failing analyzer
// writes: a dependency-cruiser parse failure, as on storybook's Svelte 5 code.
func toolStderr() string {
	var b strings.Builder
	b.WriteString("ERROR: Extracting dependencies ran afoul of...\n\n  Cannot use `await` in a non-async context\r\n")
	for b.Len() < 2048 {
		b.WriteString("\thttps://svelte.dev/e/experimental_async\n... in code/renderers/svelte/template/stories/AsyncComponent.svelte\n")
	}
	return b.String()
}

// stageWithCoverage acquires nothing but the given coverage rows.
type stageWithCoverage struct {
	*lifecycleStage
	rows []evidence.Coverage
}

func (s stageWithCoverage) Acquire(ctx context.Context, req AnalysisRequest) (Acquired, error) {
	acquired, err := s.lifecycleStage.Acquire(ctx, req)
	acquired.Context.MarkedCoverage = s.rows
	return acquired, err
}

// TestCoverageReasonsReachTheReportAsOneBoundedLine pushes each analyzer's
// failure shape through the stage executor and the projection. The published
// reason is one line within the report bound and keeps the leading text; the
// raw tool output reaches stderr intact.
func TestCoverageReasonsReachTheReportAsOneBoundedLine(t *testing.T) {
	stderrText := toolStderr()
	cases := []struct {
		name, tool, reason, wantPrefix string
	}{
		{"dependency-cruiser exit", textToolDepcruise, "dependency-cruiser exited 1: " + stderrText, "dependency-cruiser exited 1: ERROR: Extracting dependencies ran afoul of... Cannot use `await`"},
		{"grimp helper stderr", textToolGrimp, "helper exited 2: " + stderrText, "helper exited 2: ERROR:"},
		{"grimp helper error JSON", textToolGrimp, strings.Repeat("ModuleNotFoundError: no module named x; ", 60), "ModuleNotFoundError: no module named x;"},
		{"cargo metadata exit", "cargo", "cargo metadata exited 101: error: failed to parse manifest\r\n\r\nCaused by:\n  " + stderrText, "cargo metadata exited 101: error: failed to parse manifest Caused by: ERROR:"},
		{"ast-grep rejected rule file", "ast-grep/syntax", "sg rejected rule file for \"go\" (exit 1): \x1b[31merror\x1b[0m: bad YAML\n" + stderrText, "sg rejected rule file for \"go\" (exit 1): error: bad YAML ERROR:"},
		{"extractor error through Collect", "go/packages", "extract/golang: load /repo: exit status 1\n" + stderrText, "extract/golang: load /repo: exit status 1 ERROR:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			rows := []evidence.Coverage{{Tool: tc.tool, Status: evidence.StatusPartial, Reason: tc.reason}}
			exec := StageExecutor{Preparer: newLifecycleStage(), Evidence: stageWithCoverage{newLifecycleStage(), rows}, Stderr: &stderr}
			out, err := exec.Execute(t.Context(), AnalysisRequest{})
			if err != nil {
				t.Fatal(err)
			}
			got := ProjectReport(out.Diagnostic, out.Score).State.Coverage.Tools[0].Reason
			if strings.ContainsAny(got, "\n\r\t\x1b") {
				t.Errorf("reason keeps a control character: %q", got)
			}
			if n := utf8.RuneCountInString(got); n > maxReportTextRunes {
				t.Errorf("reason is %d runes, want at most %d", n, maxReportTextRunes)
			}
			if !strings.HasPrefix(got, tc.wantPrefix) || !strings.HasSuffix(got, "…") {
				t.Errorf("reason = %q, want the leading text %q cut with an ellipsis", got, tc.wantPrefix)
			}
			if !strings.Contains(stderr.String(), tc.reason) {
				t.Errorf("stderr does not carry the raw %s output:\n%s", tc.tool, stderr.String())
			}
		})
	}
}

func TestReportText(t *testing.T) {
	atLimit := strings.Repeat("é", 10)
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"valid text is byte-identical", "a  b", "a  b"},
		{"exactly the limit", atLimit, atLimit},
		{"one rune over keeps limit-1 runes and an ellipsis", atLimit + "x", strings.Repeat("é", 9) + "…"},
		{"newlines, tabs and CRLF collapse", "a\n\n\tb\r\nc", "a b c"},
		{"leading and trailing whitespace drop", "\n  a b\n", "a b"},
		{"line and paragraph separators collapse", "a b c", "a b c"},
		{"ANSI colour sequences drop", "\x1b[1;31merror\x1b[0m: x\n", "error: x"},
		{"a lone escape becomes a space", "a\x1bb\n", "a b"},
		{"cut never ends on a space", "aaaaaaaa bbbb\n", "aaaaaaaa…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportText(tc.in, 10); got != tc.want {
				t.Errorf("reportText(%q, 10) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestReportSafeTextIsUntouched pins byte identity for every report that was
// already valid: a single-line reason within the bound is published as is and
// adds nothing to stderr.
func TestReportSafeTextIsUntouched(t *testing.T) {
	const reason = "dependency-cruiser: 3 of 120 specifiers unresolved  (double space kept)"
	var stderr bytes.Buffer
	rows := []evidence.Coverage{{Tool: textToolDepcruise, Status: evidence.StatusPartial, Reason: reason}}
	exec := StageExecutor{Preparer: newLifecycleStage(), Evidence: stageWithCoverage{newLifecycleStage(), rows}, Stderr: &stderr}
	out, err := exec.Execute(t.Context(), AnalysisRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ProjectReport(out.Diagnostic, out.Score).State.Coverage.Tools[0].Reason; got != reason {
		t.Errorf("reason = %q, want it unchanged", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("a report-safe reason was disclosed on stderr: %q", stderr.String())
	}
}

// TestProjectedStatePassesTheConsumerTextRules fills every free-text field the
// state document carries — tool output, config-derived text, and text built by
// joining them — with multi-line, oversized values, and checks the published
// JSON against the consumer's string rules.
func TestProjectedStatePassesTheConsumerTextRules(t *testing.T) {
	long := toolStderr()
	r := result.New()
	r.ToolCoverage = []evidence.Coverage{{Tool: textToolDepcruise, Status: evidence.StatusPartial, Reason: long}}
	r.ConfigWarnings = []string{long}
	alternatives := []string{long}
	r.Findings = []finding.Finding{{
		ID: "f1", Kind: "gate", RuleID: "r1", Status: finding.StatusNew, Severity: finding.SeverityHigh,
		Why: long, Constraint: long, Alternatives: alternatives,
	}}
	taskConstraints := []string{strings.Repeat(long, 3)}
	r.AgentTasks = []result.AgentTask{{FindingID: "f1", RuleID: "r1", RepairKind: "code_change", Goal: strings.Repeat(long, 3), Constraints: taskConstraints, Files: []string{}, Validation: []string{"archfit check"}}}
	r.State.Decision.UnevaluatedRequiredRules = []state.UnevaluatedRule{{RuleID: "r1", Reason: "dependency-cruiser evidence is partial: " + long}}
	r.State.Dimensions.Intent.Coverage.Basis = long
	r.State.Dimensions.Intent.Unknown = []state.UnknownFact{{Fact: long, Reason: long, Owner: "intent"}}
	r.State.Dimensions.Drift.Delta = &state.Delta{Status: textStatusNonComparable, Reasons: []string{long}}
	r.Comparison = &result.StateComparison{Status: textStatusNonComparable, BaseRef: "main", Reasons: []string{long}, OriginStatus: decision.OriginUnknown, OriginReasons: []string{long}}
	r.GateReference = &result.StateComparison{Status: textStatusNonComparable, Reasons: []string{long}}

	doc := ProjectReport(r, score.Scorecard{})
	data, err := json.Marshal(doc.State)
	if err != nil {
		t.Fatal(err)
	}
	violations, err := reporttest.AppTextViolations(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("published state breaks the consumer text rules:\n%s", strings.Join(violations, "\n"))
	}
	if doc.ConfigWarnings[0] == long || doc.ToolCoverage[0].Reason == long {
		t.Error("legacy document blocks (markdown config warnings, tool coverage) keep the raw multi-line text")
	}
	if alternatives[0] != long || taskConstraints[0] != strings.Repeat(long, 3) {
		t.Error("the projection rewrote the assessment result it projects")
	}
	if !slices.Equal(doc.State.Findings[0].Alternatives, doc.Findings[0].Alternatives) {
		t.Error("state findings and document findings disagree after bounding")
	}
}
