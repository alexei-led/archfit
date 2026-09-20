package console

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

func TestUnmeasuredRequiredRulesDoNotReassure(t *testing.T) {
	s := report.NewArchitectureState()
	s.Verdict = report.StateNeedsAttention
	s.Decision.HardGates = report.HardGateUnmeasured
	s.Decision.UnevaluatedRequiredRules = []report.UnevaluatedRule{{RuleID: "domain_boundary", Reason: "Go dependencies were not loaded"}}
	out := render(t, s)
	if strings.Contains(out, "No blockers") {
		t.Fatalf("incomplete gate reassures reader: %s", out)
	}
	for _, want := range []string{"could not be completed", "domain_boundary", "Go dependencies were not loaded"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestGateReferenceKeepsBaselineDistinctFromGitComparison(t *testing.T) {
	s := report.NewArchitectureState()
	s.Comparison = report.StateComparison{Status: report.ComparisonComparable, BaseRef: "main"}
	s.GateReference = &report.StateComparison{Status: report.ComparisonNonComparable, BaseRef: ".archfit-baseline.json", Reasons: []string{"baseline predates measurement profile"}}
	out := render(t, s)
	for _, want := range []string{"reference: main", "GATE REFERENCE", ".archfit-baseline.json", "baseline predates measurement profile"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}
