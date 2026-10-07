package evaluation

import (
	"sort"
	"strings"

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
	// Ratchets is the stored reference as the metric ratchets see it. A ratchet
	// decides only against a comparable reference; against any other it is
	// unmeasured, never passed and never blocked.
	Ratchets RatchetReference
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
		if finding.IsMetricRatchet(f.RuleID) {
			// The owning dimension is the one that publishes the metric, which
			// only the built dimensions know; buildDimensions places it.
			dim = f.RuleID
		}
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
	unevaluated := unevaluatedRequiredRules(diag, in.Policy, in.Facts)
	if u, ok := in.Ratchets.unevaluated(in.Policy.Gates.Metrics); ok {
		unevaluated = append(unevaluated, u)
	}
	hardGates := state.HardGatePass
	if in.RequiredToolFailure || len(st.Blockers) > 0 {
		hardGates = state.HardGateFail
	}
	st.Verdict, st.Decision = state.Decide(state.DecisionInput{
		HardGates:                hardGates,
		UnevaluatedRequiredRules: unevaluated,
		ActiveBlockers:           len(st.Blockers),
		ActiveDiagnostics:        len(st.Diagnostics),
		Dimensions:               st.Dimensions.Signals(),
	})
	return st
}

func unevaluatedRequiredRules(diag *result.Result, p policy.PolicySnapshot, f Observations) []state.UnevaluatedRule {
	var missing []state.UnevaluatedRule
	for _, rule := range p.Gates.Rules.Rules {
		if !rule.Blocks() {
			continue
		}
		if reason := ruleUnevaluatedReason(diag, rule, p, f); reason != "" {
			missing = append(missing, state.UnevaluatedRule{RuleID: rule.ID, Reason: reason})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].RuleID < missing[j].RuleID })
	return missing
}

func ruleUnevaluatedReason(diag *result.Result, rule policy.RuleDef, p policy.PolicySnapshot, f Observations) string {
	scope := ruleProducerScope(rule, p, f)
	if scope.status == ruleScopeNotApplicable {
		return ""
	}
	if scope.status != ruleScopeApplicable {
		if scope.reason != "" {
			return scope.reason
		}
		return "rule scope cannot be established from the supported source inventory"
	}
	var reasons []string
	if ruleNeedsDependencies(rule.Type) && !primaryEvidenceComplete(diag, scope.languages) {
		for language := range scope.languages {
			tool, ok := primaryToolForLanguage(diag, language)
			if !ok {
				reasons = append(reasons, language+" dependency producer is not registered")
				continue
			}
			if reason := producerIncompleteReason(diag, tool); reason != "" {
				reasons = append(reasons, reason)
			}
		}
	}
	if ruleNeedsSyntax(rule.Type) && !syntaxEvidenceComplete(diag, scope.languages) {
		reasons = append(reasons, "syntax evidence is incomplete for the rule scope")
	}
	if ruleNeedsPatterns(rule.Type) {
		if reason := producerIncompleteReason(diag, patternCoverageTool); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	sort.Strings(reasons)
	return strings.Join(reasons, "; ")
}

func producerIncompleteReason(diag *result.Result, tool string) string {
	rows := coverageRows(diag.ToolCoverage, tool)
	if len(rows) != 1 {
		return tool + " has no unique completed coverage record"
	}
	row := rows[0]
	if row.Status == "ok" {
		return ""
	}
	reason := tool + " evidence is " + row.Status
	if row.Reason != "" {
		reason += ": " + row.Reason
	}
	return reason
}
