package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

func TestRequiredRuleEvidenceControlsHardGateCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  string
		gate    string
		from    string
		blocker bool
		want    state.HardGateState
		missing int
	}{
		{name: "failed required producer", status: modevidence.StatusPartial, gate: string(policy.GateFail), want: state.HardGateUnmeasured, missing: 1},
		{name: "absent required producer", status: modevidence.StatusAbsent, gate: string(policy.GateFail), want: state.HardGateUnmeasured, missing: 1},
		{name: "optional missing producer", status: modevidence.StatusPartial, gate: "warn", want: state.HardGatePass},
		{name: "off rule", status: modevidence.StatusPartial, gate: "off", want: state.HardGatePass},
		{name: "known failure dominates", status: modevidence.StatusPartial, gate: string(policy.GateFail), blocker: true, want: state.HardGateFail, missing: 1},
		{name: "completed required producer", status: modevidence.StatusOK, gate: string(policy.GateFail), want: state.HardGatePass},
		{name: "explicitly empty scope", status: modevidence.StatusPartial, gate: string(policy.GateFail), from: "missing/**/*.go", want: state.HardGatePass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := dimensionsFixture()
			diag.ToolCoverage[0].Status = tc.status
			in.Policy.Gates.Rules.Rules = []policy.RuleDef{{ID: "required", Type: ruleForbidden, Gate: tc.gate, From: tc.from}}
			if tc.blocker {
				diag.Findings = append(diag.Findings, finding.Finding{ID: "known", Kind: finding.KindGate, Status: finding.StatusNew})
			}
			st := evaluation.BuildState(diag, in)
			if st.Decision.HardGates != tc.want {
				t.Fatalf("hard gates = %s, want %s", st.Decision.HardGates, tc.want)
			}
			if len(st.Decision.UnevaluatedRequiredRules) != tc.missing {
				t.Fatalf("unevaluated rules = %+v", st.Decision.UnevaluatedRequiredRules)
			}
			if tc.missing > 0 && (st.Decision.UnevaluatedRequiredRules[0].RuleID != "required" || st.Decision.UnevaluatedRequiredRules[0].Reason == "") {
				t.Fatalf("missing typed evidence: %+v", st.Decision)
			}
		})
	}
}

func TestRequiredRuleEvidenceSharesIntentPrerequisites(t *testing.T) {
	diag, in := dimensionsFixture()
	in.Policy.Gates.Rules.Rules = []policy.RuleDef{
		{ID: "dependency", Type: ruleForbidden, Gate: string(policy.GateFail)},
		{ID: "syntax", Type: "public_api_max", Gate: string(policy.GateFail)},
	}
	st := evaluation.BuildState(diag, in)
	if st.Decision.HardGates != state.HardGateUnmeasured || len(st.Decision.UnevaluatedRequiredRules) != 1 || st.Decision.UnevaluatedRequiredRules[0].RuleID != "syntax" {
		t.Fatalf("decision = %+v", st.Decision)
	}
	if st.Dimensions.Intent.Coverage.Observed != 1 {
		t.Fatalf("intent = %+v", st.Dimensions.Intent)
	}
	if st.Verdict != state.NeedsAttention {
		t.Fatalf("verdict = %s", st.Verdict)
	}
}

func TestRequiredRuleEvidenceIgnoresUnrelatedLanguageProducer(t *testing.T) {
	diag, in := dimensionsFixture()
	const pythonProducer = "grimp"
	diag.PrimaryExtractorTools = []string{diag.PrimaryExtractorTools[0], toolDepCruiser, pythonProducer, toolCargo}
	diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{Tool: pythonProducer, Status: modevidence.StatusAbsent})
	in.Facts.FileLOC["a/helper.py"] = 10
	in.Policy.Gates.Rules.Rules = []policy.RuleDef{{
		ID: "go_dependency", Type: ruleForbidden, Gate: string(policy.GateFail), From: "a/**", To: "b/**",
	}}
	st := evaluation.BuildState(diag, in)
	if st.Decision.HardGates != state.HardGatePass || len(st.Decision.UnevaluatedRequiredRules) != 0 {
		t.Fatalf("unrelated producer tainted required Go rule: %+v", st.Decision)
	}
}

func TestRequiredRuleEvidenceHonorsDefaultGates(t *testing.T) {
	for _, tc := range []struct {
		ruleType string
		want     state.HardGateState
	}{
		{ruleForbidden, state.HardGateUnmeasured},
		{"cycle", state.HardGateUnmeasured},
		{"public_api_max", state.HardGateUnmeasured},
		{"public_api_change", state.HardGatePass},
		{"public_api_type_leak", state.HardGatePass},
	} {
		t.Run(tc.ruleType, func(t *testing.T) {
			diag, in := dimensionsFixture()
			diag.ToolCoverage[0].Status = modevidence.StatusPartial
			in.Policy.Gates.Rules.Rules = []policy.RuleDef{{ID: "implicit", Type: tc.ruleType}}
			st := evaluation.BuildState(diag, in)
			if st.Decision.HardGates != tc.want {
				t.Fatalf("default gate evidence = %+v, want %s", st.Decision, tc.want)
			}
		})
	}
}
