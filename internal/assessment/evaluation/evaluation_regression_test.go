// Regression tests for assessment decisions whose only observable effect is in
// the rendered report: advisory visibility, the warnings count, endpoint module
// labels, and the paths a rolled-up advisory reports.
package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	moduleA   = "a"
	moduleB   = "b"
	ruleWarn  = "warn_rule"
	fpWarnAdv = "11111111111111111111111111111111"
	fpGate    = "22222222222222222222222222222222"
)

// warnRuleFinding is what gatedRule emits for a `gate: warn` rule: a rule
// finding stamped KindAdvisory.
func warnRuleFinding(id string) finding.Finding {
	return finding.Finding{
		ID: id, Kind: finding.KindAdvisory, RuleID: ruleWarn, Status: finding.StatusNew,
		Severity: finding.SeverityMedium,
		Edge:     finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Path: pathB}, Kind: kindImports},
	}
}

// TestEvaluateHidesWarnRuleAdvisoriesWithoutAdvisories pins the --no-advisories
// contract for `gate: warn` rules. They are advisories, not gate findings: they
// must not appear in findings[] when advisories are off, or `archfit baseline
// --no-advisories` persists an entry the flag says it excludes.
func TestEvaluateHidesWarnRuleAdvisoriesWithoutAdvisories(t *testing.T) {
	in := func(include bool) evaluation.Input {
		return evaluation.Input{
			Rules:    evaluation.RulesetOf(stubRule{id: ruleWarn, findings: []finding.Finding{warnRuleFinding(fpWarnAdv)}}),
			Accepted: acceptedSet{}, IncludeAdvisories: include, Now: evaluatedAt,
		}
	}
	off := evaluation.Evaluate(in(false))
	if ids := findingIDs(off.Findings, ruleWarn); len(ids) != 0 {
		t.Errorf("findings with --no-advisories = %v, want the warn-rule advisory suppressed", ids)
	}
	if off.Warnings != 0 {
		t.Errorf("warnings with --no-advisories = %d, want 0", off.Warnings)
	}
	if off.Verdict != result.VerdictWarn {
		t.Errorf("verdict = %q, want warn: an active warn-rule finding warns even when hidden", off.Verdict)
	}

	on := evaluation.Evaluate(in(true))
	if ids := findingIDs(on.Findings, ruleWarn); len(ids) != 1 {
		t.Errorf("findings with advisories = %v, want the warn-rule advisory visible", ids)
	}
	if on.Warnings != 1 {
		t.Errorf("warnings = %d, want the warn-rule advisory counted", on.Warnings)
	}
	// Pinned on BOTH sides of the flag. Asserting the verdict only when
	// advisories are hidden lets a regression that drops the warn verdict while
	// keeping the count correct pass unnoticed.
	if on.Verdict != result.VerdictWarn {
		t.Errorf("verdict with advisories = %q, want warn", on.Verdict)
	}
}

// TestEvaluateCountsRuleAndCouplingAdvisoriesInWarnings pins that
// summary.warnings covers BOTH advisory sources. Counting only the coupling
// half let the report list advisories while claiming zero warnings.
func TestEvaluateCountsRuleAndCouplingAdvisoriesInWarnings(t *testing.T) {
	got := evaluation.Evaluate(evaluation.Input{
		Rules: evaluation.RulesetOf(stubRule{id: ruleWarn, findings: []finding.Finding{warnRuleFinding(fpWarnAdv)}}),
		AdvisoryCandidates: []relationship.AdvisoryCandidate{{
			ID: "adv-1", RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: pathA, To: pathB, FromModule: moduleA, ToModule: moduleB, EdgeKind: kindImports,
		}},
		Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt,
	})
	if got.Warnings != 2 {
		t.Errorf("warnings = %d, want both the coupling and the warn-rule advisory", got.Warnings)
	}
}

// TestEvaluateLabelsFindingEndpointsWithoutAMatchingEdge pins that module
// labels are a path lookup, not an edge-match side effect. The cycle rule
// reports two sorted SCC members that need not be directly connected; gating
// the label on an edge match left those findings with empty modules and dropped
// the "public surface of module" constraint from their repair tasks.
func TestEvaluateLabelsFindingEndpointsWithoutAMatchingEdge(t *testing.T) {
	set := relationship.Set{Edges: []relationship.Edge{
		{FromID: "file:" + pathA, ToID: "file:" + pathB, FromPath: pathA, ToPath: pathB,
			FromModule: moduleA, ToModule: moduleB, Kind: kindImports,
			Strength: relationship.StrengthFunctional, Distance: relationship.DistanceCrossModuleDiffOwner},
	}}
	// The finding names the same two endpoints but a kind no edge carries, so
	// FindByFindingEdge cannot match it.
	f := gateFinding(fpGate, pathA, pathB, finding.SeverityHigh)
	f.Edge.Kind = metricCycle

	got := evaluation.Evaluate(evaluation.Input{
		Relationships: set,
		Rules:         evaluation.RulesetOf(stubRule{id: ruleForbidden, findings: []finding.Finding{f}}),
		Accepted:      acceptedSet{}, Now: evaluatedAt,
	})
	out := findByID(t, got.Findings, fpGate)
	if out.Edge.From.Module != moduleA || out.Edge.To.Module != moduleB {
		t.Errorf("endpoint modules = %q -> %q, want %q -> %q",
			out.Edge.From.Module, out.Edge.To.Module, moduleA, moduleB)
	}
}

// A path no edge touches still resolves through the configured module map.
func TestEvaluateLabelsFindingEndpointsFromConfiguredModules(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		moduleA: {Paths: []string{assessPathsA}},
		moduleB: {Paths: []string{assessPathsB}},
	}
	got := evaluation.Evaluate(evaluation.Input{
		Rules:    evaluation.RulesetOf(stubRule{id: ruleForbidden, findings: []finding.Finding{gateFinding(fpGate, pathA, pathB, finding.SeverityHigh)}}),
		Policy:   policy.AssessmentPolicy{Topology: policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}},
		Accepted: acceptedSet{}, Now: evaluatedAt,
	})
	out := findByID(t, got.Findings, fpGate)
	if out.Edge.From.Module != moduleA || out.Edge.To.Module != moduleB {
		t.Errorf("endpoint modules = %q -> %q, want the configured modules", out.Edge.From.Module, out.Edge.To.Module)
	}
}

// TestEvaluateRolledUpAdvisoryReportsTheFirstLocationsOwner pins that a
// rolled-up advisory's from/to paths name the member owning locations[0], so
// the reported paths and the first reported location agree. Last-match wins
// made the pair point at a different member's file.
func TestEvaluateRolledUpAdvisoryReportsTheFirstLocationsOwner(t *testing.T) {
	shared := relationship.Location{File: "a/first.go", Line: 1}
	member := func(id, from, to string, locs ...relationship.Location) relationship.AdvisoryCandidate {
		return relationship.AdvisoryCandidate{
			ID: id, RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: from, To: to, FromModule: moduleA, ToModule: moduleB, EdgeKind: kindImports,
			Locations: locs,
			MatchedBy: map[string]string{keyStrength: strFunctional, keyDistance: distSameOwner, keyVolatility: volLow},
		}
	}
	// Both members carry the shared first-sorting location; only the earlier ID
	// is the representative, and its paths must win.
	got := evaluation.Evaluate(evaluation.Input{
		AdvisoryCandidates: []relationship.AdvisoryCandidate{
			member("adv-1", "a/first.go", "b/first.go", shared),
			member("adv-2", "a/second.go", "b/second.go", shared, relationship.Location{File: "a/zzz.go", Line: 9}),
		},
		Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt,
	})
	rep := findByID(t, got.Findings, "adv-1")
	if len(rep.Locations) == 0 || rep.Locations[0] != shared {
		t.Fatalf("rolled-up locations = %v, want %v first", rep.Locations, shared)
	}
	if rep.Edge.From.Path != "a/first.go" || rep.Edge.To.Path != "b/first.go" {
		t.Errorf("rolled-up paths = %q -> %q, want the member owning locations[0]",
			rep.Edge.From.Path, rep.Edge.To.Path)
	}
}
