package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

type stateInput struct {
	Policy policy.PolicySnapshot
	Facts  Observations
}

func classifyFindings(findings []finding.Finding) map[string][]state.FindingRef {
	out := map[string][]state.FindingRef{}
	for _, f := range findings {
		if f.Status != finding.StatusNew || f.Kind != finding.KindAdvisory {
			continue
		}
		dim := dimensionForRule(f.RuleID)
		out[dim] = append(out[dim], state.FindingRef{
			ID: f.ID, RuleID: f.RuleID, Kind: f.Kind, Severity: string(f.Severity), Status: string(f.Status),
		})
	}
	return out
}

func buildState(diag *result.Result, in stateInput) state.Architecture {
	st := state.New()
	split := classifyFindings(diag.Findings)
	st.Dimensions = buildDimensions(diag, in, split)
	return st
}
