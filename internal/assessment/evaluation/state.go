package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/assessment/state"
)

// classifyFindings splits the diagnostic's findings into the two populations the
// architecture decision reads: hard-gate blockers and active diagnostics.
//
// The split is computed here, in the capability that owns finding identity, so
// the state aggregator never has to look at a finding's kind-vs-status rules —
// or at anything a score touched. "Active" is the existing lifecycle predicate
// (new or expired_waiver): a baseline-accepted or waived finding was already
// decided and must not re-open the verdict.
//
// Findings are referenced, never copied into a second list, so the diagnostic
// stays the single owner of identity, status, and order.
func classifyFindings(findings []finding.Finding) (blockers, diagnostics []state.FindingRef) {
	blockers, diagnostics = []state.FindingRef{}, []state.FindingRef{}
	for _, f := range findings {
		if !score.IsActiveGateFinding(f) {
			continue
		}
		ref := state.FindingRef{
			ID: f.ID, RuleID: f.RuleID, Kind: f.Kind,
			Severity: string(f.Severity), Status: string(f.Status),
		}
		if f.Kind == finding.KindGate {
			blockers = append(blockers, ref)
			continue
		}
		diagnostics = append(diagnostics, ref)
	}
	return blockers, diagnostics
}

// buildState assembles the assessment-owned architecture state.
//
// It runs after finalize, never before: the coupling gate promotes advisories to
// gate findings and can append the synthetic trip finding, so a split taken
// earlier would undercount blockers.
//
// The nine dimension envelopes stay at their honest unmeasured default here;
// their collectors and the verdict aggregator land in the following slices.
func buildState(diag *result.Result, requiredToolFailure bool) state.Architecture {
	st := state.New()
	st.Blockers, st.Diagnostics = classifyFindings(diag.Findings)
	st.RequiredToolFailure = requiredToolFailure
	st.Decision.ActiveBlockers = len(st.Blockers)
	// A required-tool policy failure blocks without producing a finding, so the
	// hard-gate result is not simply "are there blocker findings". Rules and
	// gates did run, so the absence of a failure is a pass, not an abstention.
	if requiredToolFailure || len(st.Blockers) > 0 {
		st.Decision.HardGates = state.HardGateFail
	} else {
		st.Decision.HardGates = state.HardGatePass
	}
	_, partial, unmeasured := st.Dimensions.CountStatuses()
	st.Decision.UnknownDimensions = partial + unmeasured
	return st
}
