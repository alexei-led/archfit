package agentout

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func renderText(t *testing.T, d report.Document) string {
	t.Helper()
	var buf bytes.Buffer
	if err := NewText().Render(d, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestTextRecordShape(t *testing.T) {
	t.Parallel()
	task := gateTask("a1", "no_x")
	d := document(report.StateBlocked, task)
	d.State.AgentTasks[0].Constraints = []string{"keep the edge out", "rationale: callers must not see storage", "allowed alternative: use internal/b/api"}
	d.State.Findings = append(d.State.Findings, report.Finding{ID: "old", Status: report.FindingStatusBaseline})

	got := renderText(t, d)
	for _, want := range []string{
		sourceFile + ":5: no_x: Remove the forbidden dependency",
		"  why: callers must not see storage\n",
		"  why: keep the edge out\n",
		"  instead: use internal/b/api\n",
		"  status: new, code_change\n",
		"FAIL  1 new, 0 unknown, 1 accepted\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("text is missing %q:\n%s", want, got)
		}
	}
}

// TestTextHasNoScorecard pins the demotion of analyze content: check text
// carries no dimension, seam, or score section.
func TestTextHasNoScorecard(t *testing.T) {
	t.Parallel()
	d := document(report.StateBlocked, gateTask("a1", "no_x"))
	got := renderText(t, d)
	for _, banned := range []string{"ARCHITECTURE STATE", "DIMENSIONS", "SEAM", "SCORE", "Structure", "Coupling"} {
		if strings.Contains(got, banned) {
			t.Errorf("check text contains %q:\n%s", banned, got)
		}
	}
}

func TestTextFooterWord(t *testing.T) {
	t.Parallel()
	for _, tc := range changeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := map[ChangeStatus]string{ChangePass: "PASS", ChangeFail: "FAIL", ChangeUnknown: "UNKNOWN"}[tc.wantStatus]
			if got := renderText(t, tc.doc); !strings.Contains(got, "\n"+want+"  ") && !strings.HasPrefix(got, want+"  ") {
				t.Errorf("footer is not %s:\n%s", want, got)
			}
		})
	}
}

func TestTextReportsGateReasonsAsAttention(t *testing.T) {
	t.Parallel()
	d := document(report.StateNeedsAttention)
	d.State.GateReasons = []string{"5 distributed-monolith seam(s) present; no comparable reference"}
	got := renderText(t, d)
	if !strings.Contains(got, "repository attention (not this change):\n  coupling_gate: 5 distributed-monolith seam(s) present") {
		t.Errorf("gate reason is not repository attention:\n%s", got)
	}
}
