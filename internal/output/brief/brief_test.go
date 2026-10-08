package brief

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

var dimensionNames = []string{
	state.DimensionIntent, state.DimensionStructure, state.DimensionModularity,
	state.DimensionCoupling, state.DimensionChangeLocality, state.DimensionComplexity,
	state.DimensionTestability, state.DimensionOperations, state.DimensionDrift,
}

// TestFactStepsCoverTheRequiredFactContract pins the step table to the
// assessment's fact contract: every fact a dimension can report unknown has a
// step, and exactly the out-of-claim facts carry the no-action mark.
func TestFactStepsCoverTheRequiredFactContract(t *testing.T) {
	contract := map[string]bool{}
	for _, dim := range dimensionNames {
		for _, fact := range state.RequiredFacts(dim) {
			contract[fact.Name] = fact.InClaim
			fs, ok := factSteps[fact.Name]
			switch {
			case !ok:
				t.Errorf("fact %q (%s) has no step", fact.Name, dim)
			case fact.InClaim && (fs.category == categoryOutOfClaim || fs.step == ""):
				t.Errorf("in-claim fact %q has no step", fact.Name)
			case !fact.InClaim && fs.category != categoryOutOfClaim:
				t.Errorf("out-of-claim fact %q is given a step", fact.Name)
			}
		}
	}
	for fact := range factSteps {
		if _, ok := contract[fact]; !ok {
			t.Errorf("step table names %q, which no dimension reports", fact)
		}
	}
	if got := Step(state.DimensionDrift); !strings.HasPrefix(got, "→ ") {
		t.Errorf("Step for a dimension-level default = %q, want a step", got)
	}
}

func TestStep(t *testing.T) {
	for _, tc := range []struct{ fact, want string }{
		{state.FactSuppliedCoverageUnits, "→ supply a test coverage report for the current tree in the coverage: section"},
		{state.FactCognitiveComplexity, OutOfClaim},
		{"a fact from a newer engine", "→ " + fallbackStep},
	} {
		if got := Step(tc.fact); got != tc.want {
			t.Errorf("Step(%q) = %q, want %q", tc.fact, got, tc.want)
		}
	}
}

const (
	idBlocker      = "b228b5d0aaaaaaaaaaaaaaaaaaaaaaaa"
	idRuleAdv      = "c0ffee00aaaaaaaaaaaaaaaaaaaaaaaa"
	idBCAdv        = "bc000000aaaaaaaaaaaaaaaaaaaaaaaa"
	idOld          = "01d00000aaaaaaaaaaaaaaaaaaaaaaaa"
	checkCmd       = "archfit check -c .archfit.yaml"
	modAPI         = "api"
	refBaseline    = "baseline"
	stepFixBlocker = "Fix blocker b228b5d0 (layers-point-inward)."
)

func ref(f report.Finding) report.FindingRef {
	return report.FindingRef{ID: f.ID, RuleID: f.RuleID, Kind: f.Kind, Status: f.Status}
}

func dims(status report.MeasurementStatus, refs ...report.FindingRef) report.Dimensions {
	d := report.Dimensions{}
	for _, p := range []*report.DimensionState{&d.Intent, &d.Structure, &d.Modularity, &d.Coupling, &d.ChangeLocality,
		&d.Complexity, &d.Testability, &d.Operations, &d.Drift} {
		p.Status = status
	}
	d.Intent.Name, d.Structure.Name, d.Coupling.Name = "intent", "structure", "coupling"
	d.Testability.Name, d.Operations.Name = "testability", "operations"
	d.Structure.Findings = refs
	return d
}

func briefState() report.ArchitectureState {
	blocker := report.Finding{
		ID: idBlocker, Kind: report.FindingKindGate, RuleID: "layers-point-inward", Status: report.FindingStatusNew,
		Edge:      report.FindingEdge{From: report.FindingEndpoint{Path: "internal/domain/billing"}, To: report.FindingEndpoint{Path: "internal/adapter/stripe"}},
		Locations: []report.Location{{File: "internal/domain/billing/charge.go", Line: 3}, {File: "internal/domain/billing/refund.go", Line: 9}},
		Why:       "domain module billing imports adapter module stripe",
	}
	bcAdv := report.Finding{ID: idBCAdv, Kind: report.FindingKindAdvisory, RuleID: "bc/imbalanced_coupling", Status: report.FindingStatusNew}
	ruleAdv := report.Finding{ID: idRuleAdv, Kind: report.FindingKindAdvisory, RuleID: "no_http_in_domain", Status: report.FindingStatusNew}
	accepted := report.Finding{ID: idOld, Kind: report.FindingKindGate, RuleID: "old", Status: report.FindingStatusBaseline}
	return report.ArchitectureState{
		Verdict:    report.StateBlocked,
		Findings:   []report.Finding{blocker, bcAdv, ruleAdv, accepted},
		Dimensions: dims(report.MeasurementMeasured, ref(blocker), ref(bcAdv), ref(ruleAdv)),
		AgentTasks: []report.AgentTask{{FindingID: idBlocker, Goal: "remove the import", Validation: []string{checkCmd}}},
	}
}

func TestBuildListsBlockersWithLocationGoalAndCheck(t *testing.T) {
	v := Build(Input{State: briefState()})
	if len(v.Blockers) != 1 {
		t.Fatalf("blockers = %+v, want only the active gate finding", v.Blockers)
	}
	got := v.Blockers[0]
	want := Blocker{
		ShortID: "b228b5d0", RuleID: "layers-point-inward",
		Subject:  "internal/domain/billing -> internal/adapter/stripe",
		Location: "internal/domain/billing/charge.go:3", MoreLocations: 1,
		Why: "domain module billing imports adapter module stripe", Goal: "remove the import", Checks: []string{checkCmd},
	}
	if got.ShortID != want.ShortID || got.Subject != want.Subject || got.Location != want.Location ||
		got.MoreLocations != want.MoreLocations || got.Why != want.Why || got.Goal != want.Goal || !slices.Equal(got.Checks, want.Checks) {
		t.Errorf("blocker = %+v, want %+v", got, want)
	}
	if ids := []string{v.Diagnostics[0].ID, v.Diagnostics[1].ID}; !slices.Equal(ids, []string{idRuleAdv, idBCAdv}) {
		t.Errorf("diagnostics = %v, want the warn-gated rule finding before the coupling advisory", ids)
	}
}

func TestBlockerSubjectAndLocationVariants(t *testing.T) {
	for _, tc := range []struct {
		name         string
		f            report.Finding
		subject, loc string
	}{
		{name: "module pair", f: report.Finding{Edge: report.FindingEdge{
			From: report.FindingEndpoint{Module: "a"}, To: report.FindingEndpoint{Module: "b"}}}, subject: "a -> b"},
		{name: "one module on both sides", f: report.Finding{Edge: report.FindingEdge{
			From: report.FindingEndpoint{Path: modAPI}, To: report.FindingEndpoint{Path: modAPI}}}, subject: modAPI},
		{name: "map finding names its directory", f: report.Finding{
			MatchedBy: map[string]string{"subject": "tools/gen"}, Locations: []report.Location{{File: "tools/gen/main.go"}}},
			subject: "tools/gen", loc: "tools/gen/main.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := blockerOf(tc.f, report.AgentTask{})
			if b.Subject != tc.subject || b.Location != tc.loc || b.Goal != "" || b.Checks != nil {
				t.Errorf("blocker = %+v, want subject %q location %q and no invented goal", b, tc.subject, tc.loc)
			}
		})
	}
}

func TestVerdictReasonNamesIncompleteDimensions(t *testing.T) {
	s := report.ArchitectureState{Verdict: report.StateNeedsAttention, Dimensions: dims(report.MeasurementMeasured)}
	s.Dimensions.Testability.Status = report.MeasurementPartial
	s.Dimensions.Operations.Status = report.MeasurementUnmeasured
	if got := Build(Input{State: s}).VerdictReason; got != "evidence incomplete: testability, operations" {
		t.Errorf("VerdictReason = %q", got)
	}
	if got := Build(Input{State: briefState()}).VerdictReason; got != "" {
		t.Errorf("VerdictReason with findings = %q, want empty", got)
	}
}

func TestNextStepsOrderAndBound(t *testing.T) {
	s := briefState()
	s.Decision.UnevaluatedRequiredRules = []report.UnevaluatedRule{
		{RuleID: "dead", Reason: "selector matches nothing: from ghost/**"},
		{RuleID: "partial", Reason: "go/packages evidence is partial"},
	}
	s.Dimensions.Testability.Unknown = []report.UnknownFact{{Fact: state.FactSuppliedCoverageUnits}, {Fact: state.FactAssertionQuality}}
	s.Dimensions.Operations.Unknown = []report.UnknownFact{{Fact: state.FactOwnerProvenance}}
	got := Build(Input{State: s, CoverageGaps: []report.CoverageGap{{Tool: "dependency-cruiser"}}}).NextSteps
	want := []string{
		stepFixBlocker,
		"Ask the owner to fix rule dead: its selector matches nothing (archfit config lint).",
		"Restore the evidence rule partial needs: archfit doctor --fix.",
		"Install or fix the missing analyzers (dependency-cruiser): archfit doctor --fix.",
		"Declare owner for each module or add CODEOWNERS.",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("next steps =\n%q\nwant\n%q", got, want)
	}

	s = briefState()
	s.Dimensions.Testability.Unknown = []report.UnknownFact{{Fact: state.FactSuppliedCoverageUnits}}
	s.Dimensions.Operations.Unknown = []report.UnknownFact{{Fact: state.FactOwnerProvenance}}
	got = Build(Input{State: s}).NextSteps
	want = []string{
		stepFixBlocker,
		"Declare owner for each module or add CODEOWNERS.",
		"Supply a test coverage report for the current tree in the coverage: section.",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("next steps =\n%q\nwant module decisions before evidence\n%q", got, want)
	}
}

func TestNextStepsCollapseManyBlockers(t *testing.T) {
	s := briefState()
	refs := make([]report.FindingRef, 0, 7)
	s.Findings = nil
	for i := range 7 {
		f := report.Finding{ID: strings.Repeat(string(rune('a'+i)), 32), Kind: report.FindingKindGate, RuleID: "r"}
		s.Findings = append(s.Findings, f)
		refs = append(refs, ref(f))
	}
	s.Dimensions.Structure.Findings = refs
	got := Build(Input{State: s}).NextSteps
	if len(got) != MaxNextSteps || got[MaxNextSteps-1] != "Fix the other 3 blockers listed under BLOCKERS." {
		t.Errorf("next steps = %q, want four blockers and one line for the rest", got)
	}
}

func TestReferenceStep(t *testing.T) {
	missing := &report.StateComparison{Status: report.ComparisonNonComparable, BaseRef: refBaseline, Reasons: []string{"no baseline file was loaded"}}
	stored := func(reason string) *report.StateComparison {
		return &report.StateComparison{Status: report.ComparisonNonComparable, BaseRef: refBaseline, Reasons: []string{reason}}
	}
	for _, tc := range []struct {
		name     string
		ref      *report.StateComparison
		blockers int
		want     string
	}{
		{name: "comparable", ref: &report.StateComparison{Status: report.ComparisonComparable}, want: ""},
		{name: "missing, no blocker", ref: missing, want: stepRecordReference},
		{name: "missing with a blocker fixes the blockers first", ref: missing, blockers: 1, want: stepBlockersFirst},
		{name: "classification drift asks for review", ref: stored("classification_hash differs between the two runs (a vs b): a policy change is not a code change"), blockers: 1, want: stepReviewReference},
		{name: "profile drift asks for review", ref: stored("measurement_profile is missing from reference"), want: stepReviewReference},
		{name: "incomplete seam snapshot asks for review", ref: stored("stored baseline qualifying_seam_ids snapshot is missing or null"), want: stepReviewReference},
		{name: "missing state snapshot asks for review", ref: stored("stored baseline has no architecture-state snapshot"), want: stepReviewReference},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := referenceStep(tc.ref, tc.blockers); got != tc.want {
				t.Errorf("referenceStep = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReferenceStepAgreesAcrossSections pins NOT MEASURED to NEXT STEPS: with
// an active blocker neither offers a blanket baseline, and with a drifted
// reference both ask for a review.
func TestReferenceStepAgreesAcrossSections(t *testing.T) {
	s := briefState()
	s.GateReference = &report.StateComparison{Status: report.ComparisonNonComparable, BaseRef: refBaseline, Reasons: []string{"no baseline file was loaded"}}
	s.Dimensions.Drift.Unknown = []report.UnknownFact{{Fact: state.FactAdmissiblePersistedReference}}
	v := Build(Input{State: s})
	if got := v.StepFor(state.FactAdmissiblePersistedReference); got != "→ "+stepBlockersFirst {
		t.Errorf("NOT MEASURED step with a blocker = %q", got)
	}
	for _, step := range v.NextSteps {
		if strings.Contains(step, "archfit baseline") {
			t.Errorf("NEXT STEPS offers a baseline with an active blocker: %q", v.NextSteps)
		}
	}
	s.GateReference.Reasons = []string{"model_hash differs between the two runs (a vs b): a policy change is not a code change"}
	v = Build(Input{State: s})
	if got := v.StepFor(state.FactAdmissiblePersistedReference); got != "→ "+stepReviewReference {
		t.Errorf("NOT MEASURED step with a drifted reference = %q", got)
	}
	if !slices.Contains(v.NextSteps, capitalize(stepReviewReference)+".") {
		t.Errorf("NEXT STEPS = %q, want the review step", v.NextSteps)
	}
}

// TestNextStepsFollowTheAgentDecisionOrder pins step 1 to agentout.decide: a
// code repair before an owner decision, and a dead selector (ask_owner)
// before missing evidence (restore_evidence), whatever the input order.
func TestNextStepsFollowTheAgentDecisionOrder(t *testing.T) {
	s := briefState()
	owner := report.Finding{ID: "0wner000aaaaaaaaaaaaaaaaaaaaaaaa", Kind: report.FindingKindGate, RuleID: "map/uncovered_path", Status: report.FindingStatusNew}
	s.Findings = append([]report.Finding{owner}, s.Findings...)
	s.Dimensions.Intent.Findings = []report.FindingRef{ref(owner)}
	s.AgentTasks = append(s.AgentTasks, report.AgentTask{FindingID: owner.ID, RepairKind: repairNeedsOwnerDecision})
	got := Build(Input{State: s}).NextSteps
	if len(got) < 2 || got[0] != stepFixBlocker || got[1] != "Ask the owner to decide blocker 0wner000 (map/uncovered_path)." {
		t.Fatalf("next steps = %q, want the code repair before the owner decision", got)
	}

	s = briefState()
	s.Findings, s.Dimensions.Structure.Findings, s.Verdict = nil, nil, report.StateNeedsAttention
	s.Decision.UnevaluatedRequiredRules = []report.UnevaluatedRule{
		{RuleID: "partial", Reason: "go/packages evidence is partial"},
		{RuleID: "dead", Reason: "selector matches nothing: from ghost/**"},
	}
	if got := Build(Input{State: s}).NextSteps; len(got) == 0 || !strings.HasPrefix(got[0], "Ask the owner to fix rule dead") {
		t.Fatalf("next steps = %q, want the dead selector (ask_owner) first", got)
	}
}

func TestShortIDKeepsSyntheticPrefixes(t *testing.T) {
	for id, want := range map[string]string{
		"ba3803eca947bf3c1b7efa8f37854d5e":               "ba3803ec",
		"coupling-gate/1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d": "coupling-gate/1a2b3c4d",
		"short": "short",
	} {
		if got := shortID(id); got != want {
			t.Errorf("shortID(%q) = %q, want %q", id, got, want)
		}
	}
}
