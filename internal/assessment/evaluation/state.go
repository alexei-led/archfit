package evaluation

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/policy"
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
		if _, fired := firedRules(diag.Findings)[u.RuleID]; !fired {
			unevaluated = append(unevaluated, u)
		}
	}
	if in.Policy.Gates.UncoveredChange == policy.GateFail {
		// Recomputed from the evidence, not read from the findings: a waiver, the
		// baseline, or --no-advisories must not turn unread code into a pass.
		uncovered := uncoveredChange(RuleEvidence{
			FileClasses: inScopeFileClasses(in.Facts), UnanalysedFiles: in.Facts.UnanalysedFiles,
			SourceSelectors: in.Facts.SourceSelectors, CrateOwners: in.Facts.CrateOwners,
			AddedFiles: in.Facts.AddedFiles, UncoveredCauses: in.Facts.UncoveredCauses,
			AddedClasses: in.Facts.AddedClasses, AddedUnread: in.Facts.AddedUnread,
		}, in.Policy.Assessment)
		if u, ok := uncoveredChangeUnevaluated(uncovered); ok {
			unevaluated = append(unevaluated, u)
		}
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

// firedRules is the set of rules that produced a finding. Such a rule has a
// result, so a report must not also list it as unevaluated: only a rule whose
// scope could not be established may carry both, because a finding from a
// scope the configuration did not define is not the declared rule's result.
func firedRules(findings []finding.Finding) map[string]struct{} {
	fired := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		fired[f.RuleID] = struct{}{}
	}
	return fired
}

func unevaluatedRequiredRules(diag *result.Result, p policy.PolicySnapshot, f Observations) []state.UnevaluatedRule {
	var missing []state.UnevaluatedRule
	fired := firedRules(diag.Findings)
	for _, rule := range p.Gates.Rules.Rules {
		if !rule.Blocks() {
			continue
		}
		if reason := ruleUnevaluatedReason(diag, rule, p, f, fired); reason != "" {
			missing = append(missing, state.UnevaluatedRule{RuleID: rule.ID, Reason: reason})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].RuleID < missing[j].RuleID })
	return missing
}

// ruleUnevaluatedReason names why a required rule has no trustworthy result,
// and is empty once the rule is evaluated. A scope the configuration does not
// establish is reported even when the rule produced a finding: the finding
// comes from a file set the declaration did not define, so it is not that
// rule's result. Past the scope, a finding is a result, and a rule that fired
// is not unevaluated.
func ruleUnevaluatedReason(diag *result.Result, rule policy.RuleDef, p policy.PolicySnapshot, f Observations, fired map[string]struct{}) string {
	scope := ruleProducerScope(diag, rule, p, f)
	if scope.status == ruleScopeNotApplicable {
		return ""
	}
	if scope.status != ruleScopeApplicable {
		if scope.reason != "" {
			return scope.reason
		}
		return "rule scope cannot be established from the supported source inventory"
	}
	if _, ok := fired[rule.ID]; ok {
		return ""
	}
	var reasons []string
	rowLanguages := scope.languages
	if ruleNeedsDependencies(rule.Type) && len(f.ScopedDependencyEvidence) > 0 {
		rowLanguages = make(map[string]struct{}, len(scope.languages))
		for language := range scope.languages {
			ev, scoped := f.ScopedDependencyEvidence[language]
			if !scoped {
				rowLanguages[language] = struct{}{}
				continue
			}
			if reason := scopedEvidenceReason(rule, p, f, ev); reason != "" {
				reasons = append(reasons, reason)
			}
		}
	}
	if ruleNeedsDependencies(rule.Type) && len(rowLanguages) > 0 && !primaryEvidenceComplete(diag, rowLanguages) {
		for language := range rowLanguages {
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
	if ruleNeedsSyntax(rule.Type) && !syntaxEvidenceComplete(diag, scope.languages, f) {
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
