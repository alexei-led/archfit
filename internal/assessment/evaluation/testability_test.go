package evaluation_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	testabilityFormat     = "go-coverprofile"
	testabilityVersion    = "coverage-parser.go-coverprofile.v1"
	testabilityStatements = "statements"
)

func TestTestabilityDisabledCoveragePreservesLegacyEnvelope(t *testing.T) {
	t.Parallel()
	facts := evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
		assessFileA:     fileclass.Production,
		assessFileTestA: fileclass.Test,
	}}
	got := testabilityDimensionForTest(map[string]policy.ModuleDef{
		"a": {Paths: []string{"a/**"}},
	}, facts)
	want := state.NewDimension(state.DimensionTestability, state.OwnerTestability)
	want.Status = state.Partial
	want.Confidence = state.ConfidenceMedium
	want.Gate = state.GatePass
	want.Coverage = state.Coverage{Basis: "classified source files", Observed: 2, Total: 2}
	want.Metrics = []state.MetricValue{
		{Name: "test_files", Value: 1, Unit: assessMetricUnitCount, Provenance: []string{assessProvFileClass}},
		{Name: "production_files", Value: 1, Unit: assessMetricUnitCount, Provenance: []string{assessProvFileClass}},
		{Name: "test_to_production_files", Value: 1, Unit: assessMetricUnitRatio, Denominator: &state.MetricDenominator{Observed: 1, Total: 1}, Provenance: []string{assessProvFileClass}},
	}
	want.Unknown = []state.UnknownFact{{
		Fact: "executed test coverage", Reason: "v1 does not run a target repository's test suite; supplied coverage is not yet an input", Owner: state.OwnerTestability,
	}, {
		Fact: "boundary test coverage", Reason: "which module boundaries a test actually exercises needs test-to-production import resolution, which v1 does not collect", Owner: state.OwnerTestability,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("absent CoverageIngest changed the legacy testability envelope:\ngot  = %#v\nwant = %#v", got, want)
	}
	for _, metric := range got.Metrics {
		for _, provenance := range metric.Provenance {
			if strings.Contains(provenance, "coverage") {
				t.Fatalf("disabled coverage leaked provenance %q into %+v", provenance, got)
			}
		}
	}
}

func TestTestabilityEnabledCoverageWithEmptyFileIndexNamesEveryMissingFact(t *testing.T) {
	t.Parallel()
	facts := evaluation.Observations{
		FileClassIndex: map[string]fileclass.FileClass{},
		SuppliedCoverage: []modevidence.CoverageIngest{{
			Freshness: modevidence.FreshnessMatched,
		}},
	}
	dim := testabilityDimensionForTest(testabilityModules("a"), facts)
	if dim.Status != state.Unmeasured {
		t.Fatalf("empty file index status = %q, want unmeasured", dim.Status)
	}

	got := make([]string, 0, len(dim.Unknown))
	for _, unknown := range dim.Unknown {
		got = append(got, unknown.Fact)
	}
	want := make([]string, 0)
	for _, fact := range state.RequiredFacts(state.DimensionTestability) {
		if fact.InClaim {
			want = append(want, fact.Name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("empty file index unknown facts = %q, want declared in-claim facts %q", got, want)
	}
}

func TestTestabilityFreshCoverageMeasuresDeclaredModules(t *testing.T) {
	t.Parallel()
	modules := testabilityModules("a", "b")
	facts := completeTestabilityFacts(map[string][2]int{
		assessFileA: {4, 10},
		assessFileB: {0, 5},
	})
	dim := testabilityDimensionForTest(modules, facts)
	if dim.Status != state.Measured || dim.Confidence != state.ConfidenceHigh || dim.Gate != state.GatePass {
		t.Fatalf("fresh complete coverage = status %q confidence %q gate %q, want measured/high/pass; unknown=%+v", dim.Status, dim.Confidence, dim.Gate, dim.Unknown)
	}
	if dim.Coverage.Observed != 2 || dim.Coverage.Total != 2 {
		t.Fatalf("module coverage = %d/%d, want 2/2", dim.Coverage.Observed, dim.Coverage.Total)
	}
	assertTestabilityMetric(t, dim, "covered_units", 4, 4, 15)
	assertTestabilityMetric(t, dim, "total_units", 15, 4, 15)
	assertTestabilityMetric(t, dim, "coverage_ratio", 4.0/15.0, 4, 15)
	assertTestabilityMetric(t, dim, "modules_with_coverage", 2, 2, 2)
	assertTestabilityMetric(t, dim, "unresolved_coverage_paths", 0, -1, -1)
	assertTestabilityMetric(t, dim, "test_files", 0, -1, -1)
	assertTestabilityMetric(t, dim, "production_files", 2, -1, -1)
	for _, name := range []string{"covered_units", "total_units", "coverage_ratio", "modules_with_coverage", "unresolved_coverage_paths"} {
		metric := requireTestabilityMetric(t, dim, name)
		if (name == "covered_units" || name == "total_units") && metric.Unit != testabilityStatements {
			t.Errorf("%s unit = %q, want statements", name, metric.Unit)
		}
		joined := strings.Join(metric.Provenance, " ")
		for _, want := range []string{testabilityFormat, testabilityVersion, string(modevidence.FreshnessMatched)} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s provenance %q does not carry %q", name, joined, want)
			}
		}
	}
	for _, unknown := range dim.Unknown {
		if unknown.Fact != state.FactAssertionQuality && unknown.Fact != state.FactBoundaryTestSemantics {
			t.Errorf("measured testability carries in-claim unknown %+v", unknown)
		}
	}
}

func TestTestabilityPartialCoverageKeepsDeclaredModuleDenominator(t *testing.T) {
	t.Parallel()
	modules := testabilityModules("a", "b", "c", "d", "e")
	facts := completeTestabilityFacts(map[string][2]int{
		assessFileA: {1, 1},
		assessFileB: {1, 1},
		"c/c.go":    {1, 1},
	})
	for _, module := range []string{"d", "e"} {
		facts.FileClassIndex[module+"/"+module+".go"] = fileclass.Production
	}
	dim := testabilityDimensionForTest(modules, facts)
	if dim.Status != state.Partial {
		t.Fatalf("3/5 module coverage status = %q, want partial", dim.Status)
	}
	metric := requireTestabilityMetric(t, dim, "modules_with_coverage")
	if metric.Denominator == nil || metric.Denominator.Observed != 3 || metric.Denominator.Total != 5 {
		t.Fatalf("modules_with_coverage denominator = %+v, want 3/5", metric.Denominator)
	}
	unknown := requireTestabilityUnknown(t, dim, state.FactCoverageModuleAttribution)
	if !strings.Contains(unknown.Reason, "d") || !strings.Contains(unknown.Reason, "e") {
		t.Fatalf("module attribution reason %q does not name both uncovered modules", unknown.Reason)
	}
}

func TestTestabilityUnattributedCoveragePathStaysPartial(t *testing.T) {
	t.Parallel()
	facts := completeTestabilityFacts(map[string][2]int{
		assessFileA:    {2, 4},
		"outside/x.go": {1, 2},
	})
	dim := testabilityDimensionForTest(testabilityModules("a"), facts)
	if dim.Status != state.Partial {
		t.Fatalf("unattributed coverage status = %q, want partial", dim.Status)
	}
	unknown := requireTestabilityUnknown(t, dim, state.FactCoverageModuleAttribution)
	if !strings.Contains(unknown.Reason, "outside/x.go") {
		t.Fatalf("module attribution reason %q does not name the unattributed path", unknown.Reason)
	}
	assertTestabilityMetric(t, dim, "coverage_ratio", 0.5, 2, 4)
}

func TestTestabilityUnresolvedCoveragePathAlwaysStaysPartial(t *testing.T) {
	t.Parallel()
	facts := completeTestabilityFacts(map[string][2]int{assessFileA: {10, 10}})
	facts.SuppliedCoverage[0].UnresolvedPaths = 1
	facts.SuppliedCoverage[0].Reason = "unresolved_coverage_paths"
	dim := testabilityDimensionForTest(testabilityModules("a"), facts)
	if dim.Status != state.Partial {
		t.Fatalf("unresolved high-ratio coverage status = %q, want partial", dim.Status)
	}
	assertTestabilityMetric(t, dim, "coverage_ratio", 1, 10, 10)
	assertTestabilityMetric(t, dim, "unresolved_coverage_paths", 1, -1, -1)
	requireTestabilityUnknown(t, dim, state.FactCoveragePathResolution)
}

func TestTestabilityStaleAndUnverifiedCoveragePublishMarkedRatios(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		freshness modevidence.CoverageFreshness
		reason    string
	}{
		{name: "stale", freshness: modevidence.FreshnessStale, reason: "worktree_differs_from_ref"},
		{name: "unverified", freshness: modevidence.FreshnessUnverified, reason: "freshness_unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts := completeTestabilityFacts(map[string][2]int{assessFileA: {2, 10}})
			facts.SuppliedCoverage[0].Freshness = tc.freshness
			facts.SuppliedCoverage[0].Reason = tc.reason
			dim := testabilityDimensionForTest(testabilityModules("a"), facts)
			if dim.Status != state.Partial {
				t.Fatalf("%s coverage status = %q, want partial", tc.name, dim.Status)
			}
			metric := requireTestabilityMetric(t, dim, "coverage_ratio")
			if math.Abs(metric.Value-0.2) > 1e-12 || !strings.Contains(strings.Join(metric.Provenance, " "), string(tc.freshness)) {
				t.Fatalf("%s coverage ratio = %+v, want 0.2 with marked provenance", tc.name, metric)
			}
			unknown := requireTestabilityUnknown(t, dim, state.FactCoverageFreshness)
			if !strings.Contains(unknown.Reason, tc.reason) {
				t.Fatalf("freshness reason = %q, want %q", unknown.Reason, tc.reason)
			}
		})
	}
}

func TestTestabilityDuplicateFactsUseLowerBoundWithoutInventingAUnion(t *testing.T) {
	t.Parallel()
	facts := completeTestabilityFacts(map[string][2]int{assessFileA: {6, 10}})
	facts.SuppliedCoverage[0].Facts = append(facts.SuppliedCoverage[0].Facts, modevidence.CoverageFact{
		File: assessFileA, CoveredUnits: 7, TotalUnits: 10, Unit: testabilityStatements, Format: testabilityFormat,
	})
	dim := testabilityDimensionForTest(testabilityModules("a"), facts)
	if dim.Status != state.Measured {
		t.Fatalf("compatible duplicate status = %q, want measured; unknown=%+v", dim.Status, dim.Unknown)
	}
	assertTestabilityMetric(t, dim, "covered_units", 7, 7, 10)
	assertTestabilityMetric(t, dim, "total_units", 10, 7, 10)
	assertTestabilityMetric(t, dim, "merged_coverage_facts", 1, -1, -1)

	facts.SuppliedCoverage[0].Facts[1].TotalUnits = 11
	conflict := testabilityDimensionForTest(testabilityModules("a"), facts)
	if conflict.Status != state.Partial {
		t.Fatalf("conflicting duplicate status = %q, want partial", conflict.Status)
	}
	if _, found := testabilityMetric(conflict, "coverage_ratio"); found {
		t.Fatal("conflicting denominators published a fabricated coverage_ratio")
	}
	requireTestabilityUnknown(t, conflict, state.FactSuppliedCoverageUnits)
}

func TestTestabilityZeroTestsAndZeroCoverageIsStillMeasured(t *testing.T) {
	t.Parallel()
	facts := completeTestabilityFacts(map[string][2]int{assessFileA: {0, 10}})
	dim := testabilityDimensionForTest(testabilityModules("a"), facts)
	if dim.Status != state.Measured {
		t.Fatalf("measured zero coverage status = %q, want measured", dim.Status)
	}
	assertTestabilityMetric(t, dim, "test_files", 0, -1, -1)
	assertTestabilityMetric(t, dim, "coverage_ratio", 0, 0, 10)
	if len(dim.Findings) != 0 || dim.Gate != state.GatePass {
		t.Fatalf("zero coverage created a finding or moved a gate: findings=%+v gate=%q", dim.Findings, dim.Gate)
	}
}

func testabilityDimensionForTest(modules map[string]policy.ModuleDef, facts evaluation.Observations) state.Dimension {
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	input := evaluation.StateInput{Policy: policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil), Facts: facts}
	return evaluation.BuildDimensions(&result.Result{}, input, nil).Testability
}

func testabilityModules(names ...string) map[string]policy.ModuleDef {
	modules := make(map[string]policy.ModuleDef, len(names))
	for _, name := range names {
		modules[name] = policy.ModuleDef{Paths: []string{name + "/**"}}
	}
	return modules
}

func completeTestabilityFacts(files map[string][2]int) evaluation.Observations {
	facts := evaluation.Observations{
		FileClassIndex: make(map[string]fileclass.FileClass, len(files)),
		SuppliedCoverage: []modevidence.CoverageIngest{{
			Freshness: modevidence.FreshnessMatched,
			Format:    testabilityFormat, ToolVersion: testabilityVersion,
		}},
	}
	for file, counts := range files {
		facts.FileClassIndex[file] = fileclass.Production
		facts.SuppliedCoverage[0].Facts = append(facts.SuppliedCoverage[0].Facts, modevidence.CoverageFact{
			File: file, CoveredUnits: counts[0], TotalUnits: counts[1], Unit: testabilityStatements, Format: testabilityFormat,
		})
	}
	return facts
}

func assertTestabilityMetric(t *testing.T, dim state.Dimension, name string, want float64, observed, total int) {
	t.Helper()
	metric := requireTestabilityMetric(t, dim, name)
	if math.Abs(metric.Value-want) > 1e-12 {
		t.Fatalf("%s = %v, want %v", name, metric.Value, want)
	}
	if observed >= 0 {
		if metric.Denominator == nil || metric.Denominator.Observed != observed || metric.Denominator.Total != total {
			t.Fatalf("%s denominator = %+v, want %d/%d", name, metric.Denominator, observed, total)
		}
	}
}

func requireTestabilityMetric(t *testing.T, dim state.Dimension, name string) state.MetricValue {
	t.Helper()
	metric, found := testabilityMetric(dim, name)
	if !found {
		t.Fatalf("testability metric %q missing from %+v", name, dim.Metrics)
	}
	return metric
}

func testabilityMetric(dim state.Dimension, name string) (state.MetricValue, bool) {
	for _, metric := range dim.Metrics {
		if metric.Name == name {
			return metric, true
		}
	}
	return state.MetricValue{}, false
}

func requireTestabilityUnknown(t *testing.T, dim state.Dimension, fact string) state.UnknownFact {
	t.Helper()
	for _, unknown := range dim.Unknown {
		if unknown.Fact == fact {
			return unknown
		}
	}
	t.Fatalf("testability unknown %q missing from %+v", fact, dim.Unknown)
	return state.UnknownFact{}
}
