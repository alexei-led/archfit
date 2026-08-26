package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/assessment/state"
	"github.com/alexei-led/archfit/internal/policy"
)

// stateInput carries the already-resolved values the architecture-state
// collectors read. It is a projection of ScoreInput, not a second source: the
// state never reaches for a fact the scored run did not already have.
type stateInput struct {
	Policy policy.PolicySnapshot
	Facts  Observations
	// RuleTypes maps a declared rule ID to its type, so a finding can be routed
	// to the dimension that owns its subject.
	RuleTypes map[string]string
	// RequiredToolFailure is the required-analyzer gate result. It blocks
	// without producing a finding, so it cannot be inferred from the findings.
	RequiredToolFailure bool
	// Drift is the stored architecture-state reference and its comparability.
	// It is the same anchor the seam gate reads, so the drift dimension and the
	// gate can never disagree about whether a comparison was admissible.
	Drift BaselineAnchor
}

// classified is the one pass over the run's findings that the architecture
// state is decided from: the two decision populations plus the per-dimension
// routing, all in the diagnostic's own finding order.
type classified struct {
	blockers    []state.FindingRef
	diagnostics []state.FindingRef
	byDimension map[string][]state.FindingRef
}

// classifyFindings splits the diagnostic's findings into the two populations the
// architecture decision reads — hard-gate blockers and active diagnostics — and
// routes each to the dimension that owns its subject.
//
// The split is computed here, in the capability that owns finding identity, so
// the state aggregator never has to look at a finding's kind-vs-status rules —
// or at anything a score touched. "Active" is the existing lifecycle predicate
// (new or expired_waiver): a baseline-accepted or waived finding was already
// decided and must not re-open the verdict or warn a dimension.
//
// Findings are referenced, never copied into a second list, so the diagnostic
// stays the single owner of identity, status, and order.
func classifyFindings(findings []finding.Finding, ruleTypes map[string]string) classified {
	out := classified{
		blockers:    []state.FindingRef{},
		diagnostics: []state.FindingRef{},
		byDimension: map[string][]state.FindingRef{},
	}
	for _, f := range findings {
		if !score.IsActiveGateFinding(f) {
			continue
		}
		ref := state.FindingRef{
			ID: f.ID, RuleID: f.RuleID, Kind: f.Kind,
			Severity: string(f.Severity), Status: string(f.Status),
		}
		if f.Kind == finding.KindGate {
			out.blockers = append(out.blockers, ref)
		} else {
			out.diagnostics = append(out.diagnostics, ref)
		}
		dim := dimensionForRule(f.RuleID, ruleTypes)
		out.byDimension[dim] = append(out.byDimension[dim], ref)
	}
	return out
}

// buildState assembles the assessment-owned architecture state.
//
// It runs after finalize, never before: the coupling gate promotes advisories to
// gate findings and can append the synthetic trip finding, so a split taken
// earlier would undercount blockers.
//
// The verdict itself is not decided here — the metric-blind aggregator owns it,
// and this function deliberately hands it only statuses, gate results, and
// classifications.
func buildState(diag *result.Result, in stateInput) state.Architecture {
	st := state.New()
	split := classifyFindings(diag.Findings, in.RuleTypes)
	st.Blockers, st.Diagnostics = split.blockers, split.diagnostics
	st.RequiredToolFailure = in.RequiredToolFailure
	st.Dimensions = buildDimensions(diag, in, split.byDimension)
	// A required-tool policy failure blocks without producing a finding, so the
	// hard-gate result is not simply "are there blocker findings". Rules and
	// gates did run, so the absence of a failure is a pass, not an abstention.
	hardGates := state.HardGatePass
	if in.RequiredToolFailure || len(st.Blockers) > 0 {
		hardGates = state.HardGateFail
	}
	st.Verdict, st.Decision = state.Decide(state.DecisionInput{
		HardGates:         hardGates,
		ActiveBlockers:    len(st.Blockers),
		ActiveDiagnostics: len(st.Diagnostics),
		Dimensions:        st.Dimensions.Signals(),
	})
	return st
}
