// Behavior tests for the assessment evaluation seam. They pin what Evaluate
// decides — findings, waivers, statuses, advisory filtering and rollup, metric
// measurement, and verdict — so the capability migration can move
// ownership without moving semantics.
package evaluation_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/metrics"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	signal "github.com/alexei-led/archfit/v3/internal/assessment/signals"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const (
	ruleModuleCycle = "no_module_cycles"
	ruleForbidden   = "forbidden_dependency"
	ruleTypePattern = "forbidden_pattern"
	selCatalog      = "internal/catalog/**"
	ruleBC          = "bc/imbalanced_coupling"

	metricName    = "cycle_count"
	pathA         = "a/a.go"
	pathB         = "b/b.go"
	kindImports   = "imports"
	keyStrength   = "strength"
	keyDistance   = "distance"
	keyVolatility = "volatility"
	strFunctional = "functional"
	distSameOwner = "cross_module"
	volLow        = "low"
)

var evaluatedAt = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

// stubMetric supplies a measurement without invoking a real calculator.
type stubMetric struct{ res result.MetricResult }

func (m stubMetric) Name() string    { return m.res.Name }
func (m stubMetric) Version() string { return "v1" }
func (m stubMetric) Calculate(signal.CollectedSignals) result.MetricResult {
	return m.res
}

var _ metrics.Metric = stubMetric{}

func TestEvaluateMetricsAreMeasurements(t *testing.T) {
	for _, value := range []float64{0, 2, 1000} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				Metrics: evaluation.MetricsetOf(stubMetric{res: result.MetricResult{
					Name: metricName, Value: value, Direction: result.DirectionHigherIsWorse,
				}}),
				Now: evaluatedAt,
			})
			if len(got.Metrics) != 1 || got.Metrics[0].Name != metricName || got.Metrics[0].Value != value {
				t.Fatalf("metric measurement = %+v, want %s=%v", got.Metrics, metricName, value)
			}
			if len(got.Findings) != 0 || got.Warnings != 0 {
				t.Fatalf("measurement changed the verdict: %+v", got)
			}
		})
	}
}

// findingIDs returns the IDs of every finding carrying ruleID.
func findingIDs(fs []finding.Finding, ruleID string) []string {
	out := []string{}
	for _, f := range fs {
		if ruleID == "" || f.RuleID == ruleID {
			out = append(out, f.ID)
		}
	}
	return out
}

func findByID(t *testing.T, fs []finding.Finding, id string) finding.Finding {
	t.Helper()
	for _, f := range fs {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("finding %q not found in %v", id, findingIDs(fs, ""))
	return finding.Finding{}
}

func TestEvaluateAdvisoryVisibilityAndWarnVerdict(t *testing.T) {
	candidate := relationship.AdvisoryCandidate{
		ID: "adv1", RuleID: ruleBC, Kind: finding.KindAdvisory, Severity: relationship.SeverityMedium,
		From: pathA, To: pathB, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
		Why: "unbalanced", MatchedBy: map[string]string{keyStrength: strFunctional},
	}
	tests := []struct {
		name              string
		includeAdvisories bool
		wantVisible       int
		wantWarnings      int
	}{
		{name: "advisories excluded", includeAdvisories: false, wantVisible: 0, wantWarnings: 0},
		{name: "advisories included, counted as a warning, verdict stays pass", includeAdvisories: true, wantVisible: 1, wantWarnings: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				AdvisoryCandidates: []relationship.AdvisoryCandidate{candidate},
				IncludeAdvisories:  test.includeAdvisories,
				Now:                evaluatedAt,
			})
			if len(got.Findings) != test.wantVisible {
				t.Errorf("visible findings = %d, want %d", len(got.Findings), test.wantVisible)
			}
			if got.Warnings != test.wantWarnings {
				t.Errorf("Warnings = %d, want %d", got.Warnings, test.wantWarnings)
			}
		})
	}
}

// bc/imbalanced_coupling advisories on the same module pair with the same
// classification collapse to one representative carrying the member roll-up.
func TestEvaluateRollsUpBCAdvisoriesByModulePairAndClassification(t *testing.T) {
	same := func(id, from, to string) relationship.AdvisoryCandidate {
		return relationship.AdvisoryCandidate{
			ID: id, RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: from, To: to, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
			Locations: []relationship.Location{{File: from, Line: 1}},
			MatchedBy: map[string]string{keyStrength: strFunctional, keyDistance: distSameOwner, keyVolatility: volLow},
		}
	}
	differing := same("adv-other", "a/c.go", "b/c.go")
	differing.MatchedBy = map[string]string{keyStrength: "intrusive", keyDistance: distSameOwner, keyVolatility: volLow}
	other := relationship.AdvisoryCandidate{
		ID: "adv-dup", RuleID: "bc/duplicated_knowledge", Severity: relationship.SeverityLow,
		From: pathA, To: pathB, FromModule: "a", ToModule: "b", EdgeKind: "clone",
	}

	got := evaluation.Evaluate(evaluation.Input{
		AdvisoryCandidates: []relationship.AdvisoryCandidate{
			same("adv-2", "a/x.go", "b/x.go"), same("adv-1", pathA, pathB), differing, other,
		},
		IncludeAdvisories: true,
		Now:               evaluatedAt,
	})

	bcIDs := findingIDs(got.Findings, ruleBC)
	if len(bcIDs) != 2 {
		t.Fatalf("bc advisories = %v, want 2 (one rolled-up group + one distinct classification)", bcIDs)
	}
	rep := findByID(t, got.Findings, "adv-1")
	if rep.MatchedBy["group_count"] != "2" {
		t.Errorf("group_count = %q, want 2", rep.MatchedBy["group_count"])
	}
	if members := rep.MatchedBy["group_members"]; !strings.Contains(members, "adv-1") || !strings.Contains(members, "adv-2") {
		t.Errorf("group_members = %q, want both member IDs", members)
	}
	if len(rep.Locations) != 2 {
		t.Errorf("rolled-up locations = %d, want 2 merged locations", len(rep.Locations))
	}
	if ids := findingIDs(got.Findings, "bc/duplicated_knowledge"); len(ids) != 1 {
		t.Errorf("non-bc advisories = %v, want the duplicated-knowledge finding passed through untouched", ids)
	}
}

// A rollup keeps every member ID for complete finding identity, past the cap on the
// readable group_members list.
func TestEvaluateRollupKeepsEveryMemberPastTheDisplayCap(t *testing.T) {
	const n = 10
	candidates := make([]relationship.AdvisoryCandidate, 0, n)
	want := make([]string, 0, n)
	for i := range n {
		id := fmt.Sprintf("adv-%02d", i)
		want = append(want, id)
		candidates = append(candidates, relationship.AdvisoryCandidate{
			ID: id, RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: fmt.Sprintf("a/%02d.go", i), To: pathB, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
			MatchedBy: map[string]string{keyStrength: strFunctional, keyDistance: distSameOwner, keyVolatility: volLow},
		})
	}

	got := evaluation.Evaluate(evaluation.Input{
		AdvisoryCandidates: candidates, IncludeAdvisories: true, Now: evaluatedAt,
	})

	rep := findByID(t, got.Findings, "adv-00")
	if !slices.Equal(rep.Members, want) {
		t.Errorf("Members = %v, want all %d member IDs", rep.Members, n)
	}
	if shown := strings.Split(rep.MatchedBy["group_members"], ","); len(shown) != 8 {
		t.Errorf("group_members lists %d IDs, want the display cap of 8", len(shown))
	}
}

func TestEvaluateEmptyInputIsPassWithNoFindings(t *testing.T) {
	got := evaluation.Evaluate(evaluation.Input{IncludeAdvisories: true, Now: evaluatedAt})
	if len(got.Findings) != 0 || len(got.Metrics) != 0 {
		t.Fatalf("Result = %+v, want no findings and no metrics", got)
	}
	if got.Warnings != 0 {
		t.Fatalf("counts = %+v, want all zero", got)
	}
}

func TestEvaluateEmitsStalenessAdvisoriesFromPolicy(t *testing.T) {
	set := relationship.Set{Nodes: []relationship.Node{
		{ID: "file:unmapped/x.go", Path: "unmapped/x.go", Kind: "file", Language: "go", FirstParty: true},
	}}
	modules := map[string]policy.ModuleDef{"ghost": {Paths: []string{"ghost/**"}}}
	pol := policy.AssessmentPolicy{
		Topology:  policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)},
		Staleness: policy.StalenessPolicy{Enabled: true, Threshold: 24 * time.Hour},
	}

	off := evaluation.Evaluate(evaluation.Input{
		Relationships: set, IncludeAdvisories: true, Now: evaluatedAt,
	})
	on := evaluation.Evaluate(evaluation.Input{
		Relationships: set, Policy: pol, IncludeAdvisories: true, Now: evaluatedAt,
	})
	if len(on.Findings) <= len(off.Findings) {
		t.Fatalf("staleness findings on = %d, off = %d; want enabling the policy to add advisories",
			len(on.Findings), len(off.Findings))
	}
	for _, f := range on.Findings {
		if f.Kind != finding.KindAdvisory {
			t.Errorf("staleness finding %s kind = %q, want advisory", f.ID, f.Kind)
		}
	}
}
