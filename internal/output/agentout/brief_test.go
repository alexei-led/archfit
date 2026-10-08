package agentout

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestBriefNamesTheRepair(t *testing.T) {
	t.Parallel()
	d := document(report.StateBlocked, gateTask("a1", "no_x"))
	d.State.Decision.UnevaluatedRequiredRules = []report.UnevaluatedRule{{RuleID: "r2", Reason: "go/packages evidence is partial"}}
	got := Brief(Build(d))
	for _, want := range []string{
		"archfit: next_action repair",
		"repair 1 (code_change, in scope): no_x [a1]",
		"  at: " + sourceFile + ":5",
		"  goal: Remove the forbidden dependency",
		"  constraint: constraint of no_x",
		"  edit: " + sourceFile,
		"unevaluated rule: r2: go/packages evidence is partial",
		"validate: " + validation + " --format agent",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("brief is missing %q:\n%s", want, got)
		}
	}
	if Brief(Build(d)) != got {
		t.Error("two briefs of one result differ")
	}
}
