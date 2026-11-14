// Behavior tests for the nine architecture-state envelopes. They pin what each
// dimension measures, what it refuses to measure, that every measured envelope
// carries a denominator and provenance, and that findings reach the dimension
// that owns their subject.
package evaluation_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	// toolDepCruiser is the TypeScript primary analyzer's coverage name.
	toolDepCruiser      = "dependency-cruiser"
	metricEncapsulation = "encapsulation"
	bandNA              = "n/a"
	assessUnitSvc       = "svc"
	assessUnitAPI       = "api"
	assessTeamA         = "team-a"
	// toolCargo/toolSyntaxAggregate/globInternalAll are repeated across the
	// dimension tables below.
	toolCargo           = "cargo"
	toolSyntaxAggregate = "ast-grep/syntax"
	globInternalAll     = "internal/**"
	toolDeployDiscovery = "deploy-unit"
)

// dimensionsFixture is a run with something to measure in every dimension that
// v1 can measure at all.
func dimensionsFixture() (*result.Result, evaluation.StateInput) {
	const primaryTool = "go/packages"
	diag := &result.Result{
		Findings: []finding.Finding{},
		Metrics: []result.MetricResult{
			{Name: metricCycle, Value: 0, Band: "strong", Confidence: confHigh, Version: "cycle.v1", Mode: assessMetricUnitCount},
			{Name: "blast_radius", Value: 2, Band: "info", Confidence: volLow, Version: "blast_radius.v2", Mode: assessMetricUnitCount},
			{Name: metricEncapsulation, Value: 0, Band: bandNA, Confidence: volLow, Version: "encapsulation.v3", Mode: assessMetricUnitRatio},
			{Name: "coverage", Value: 1, Band: "strong", Confidence: confHigh, Version: "coverage.v1", Mode: assessMetricUnitRatio},
		},
		ModuleGraphComplexity: &result.ModuleGraphComplexity{Modules: 2, MaxDependencyChain: 1, FanInP90: 1, FanOutP90: 1},
		ClassifiedEdges: &result.ClassifiedEdgeSummary{
			Total: 20, Scored: 10, Abstained: 2, SameModule: 3, External: 5, ConnectedModules: 2,
			DependencyEdges: 20, InternalDependencies: 15, ClassifiedInternalDependencies: 15,
			SameModuleDependencies: 3, LibraryDependencies: 3, UnmappedFirstPartyDependencies: 2,
			DependencyModules: 2, FirstPartyNodes: 2, AttributedFirstPartyNodes: 2,
			TailRisk: &result.CouplingTailRiskSummary{CriticalEdges: 1, HighOrWorseEdges: 3, DistributedMonolithEdges: 0},
		},
		ToolCoverage: []modevidence.Coverage{
			{Tool: primaryTool, Status: modevidence.StatusOK},
			{Tool: "scip", Status: modevidence.StatusAbsent},
			{Tool: toolDeployDiscovery, Status: modevidence.StatusOK},
		},
		PrimaryExtractorTools: []string{primaryTool},
	}
	modules := map[string]policy.ModuleDef{
		assessModA: {Paths: []string{assessPathsA}, Public: []string{"a/api"}, Owner: assessTeamA, DeployUnit: assessUnitSvc},
		assessModB: {Paths: []string{assessPathsB}, Owner: assessTeamA},
	}
	topology := policy.TopologyView{Modules: modules, Layers: []string{assessCore, "adapter"}, ModuleMap: policy.BuildModuleMap(modules)}
	in := evaluation.StateInput{
		Policy: policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil),
		Facts: evaluation.Observations{
			FileLOC:             map[string]int{assessFileA: 100, assessFileTestA: 40, assessFileB: 250},
			FileClassIndex:      map[string]fileclass.FileClass{assessFileA: fileclass.Production, assessFileTestA: fileclass.Test, assessFileB: fileclass.Production},
			DeclaredDeployUnits: map[string]string{assessModA: assessUnitSvc},
			CorroboratedDeployUnits: map[string]modevidence.CorroboratedDeployUnit{
				assessModA: {Path: "a", Unit: assessUnitSvc, Source: modevidence.TopologySourceDockerfile},
				assessModB: {Path: "b", Unit: "worker", Source: modevidence.TopologySourceDockerfile},
			},
			OwnerProvenance: map[string]modevidence.OwnerProvenance{
				assessModA: {Module: assessModA, Owner: assessTeamA, Source: modevidence.TopologySourceDeclared},
				assessModB: {Module: assessModB, Owner: assessTeamA, Source: modevidence.TopologySourceDeclared},
			},
		},
	}
	return diag, in
}

// TestDimensionStatuses is the measured/partial/unmeasured table for a run that
// has evidence, and for one that has none. Nothing may report measured on an
// empty run, and nothing may report unmeasured when its evidence is present.
func TestDimensionStatuses(t *testing.T) {
	t.Parallel()
	populated, in := dimensionsFixture()
	tests := []struct {
		name  string
		diag  *result.Result
		input evaluation.StateInput
		want  map[string]state.MeasurementStatus
	}{
		{
			name: "populated run", diag: populated, input: in,
			want: map[string]state.MeasurementStatus{
				state.DimensionIntent:         state.Measured,
				state.DimensionStructure:      state.Measured,
				state.DimensionModularity:     state.Measured,
				state.DimensionCoupling:       state.Partial,
				state.DimensionChangeLocality: state.Unmeasured,
				state.DimensionComplexity:     state.Measured,
				state.DimensionTestability:    state.Partial,
				state.DimensionOperations:     state.Measured,
				state.DimensionDrift:          state.Unmeasured,
			},
		},
		{
			name: "empty run", diag: &result.Result{}, input: evaluation.StateInput{},
			want: map[string]state.MeasurementStatus{
				state.DimensionIntent:         state.Unmeasured,
				state.DimensionStructure:      state.Unmeasured,
				state.DimensionModularity:     state.Unmeasured,
				state.DimensionCoupling:       state.Unmeasured,
				state.DimensionChangeLocality: state.Unmeasured,
				state.DimensionComplexity:     state.Unmeasured,
				state.DimensionTestability:    state.Unmeasured,
				state.DimensionOperations:     state.Unmeasured,
				state.DimensionDrift:          state.Unmeasured,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dims := evaluation.BuildDimensions(tc.diag, tc.input, nil)
			for _, dim := range dims.All() {
				if dim.Status != tc.want[dim.Name] {
					t.Errorf("%s status = %q, want %q", dim.Name, dim.Status, tc.want[dim.Name])
				}
			}
			measured, partial, unmeasured := dims.CountStatuses()
			if measured+partial+unmeasured != state.DimensionCount {
				t.Errorf("coverage counts sum to %d, want %d", measured+partial+unmeasured, state.DimensionCount)
			}
		})
	}
}

// TestDimensionCoverageRequired is the executable fixture behind the
// dimension_coverage_required rule. It is not expressible as a dependency rule,
// so it is enforced here: the nine envelopes always account for themselves
// exactly once, a dimension that claims a measurement says what it counted and
// who observed every number it reports, and a dimension that measured nothing
// says why.
func TestDimensionCoverageRequired(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	dims := evaluation.BuildDimensions(diag, in, nil)
	if measured, partial, unmeasured := dims.CountStatuses(); measured+partial+unmeasured != state.DimensionCount {
		t.Fatalf("coverage counts (%d, %d, %d) do not sum to %d", measured, partial, unmeasured, state.DimensionCount)
	}
	for _, dim := range dims.All() {
		if dim.Name == "" || dim.Owner == "" {
			t.Errorf("envelope %+v ships without a name or an evidence owner", dim)
		}
		if dim.Status == state.Unmeasured {
			if len(dim.Unknown) == 0 {
				t.Errorf("%s is unmeasured with no stated reason", dim.Name)
			}
			continue
		}
		if dim.Coverage.Basis == "" {
			t.Errorf("%s is %q with no coverage basis", dim.Name, dim.Status)
		}
		if dim.Coverage.Observed > dim.Coverage.Total {
			t.Errorf("%s coverage = %d/%d, observed exceeds total", dim.Name, dim.Coverage.Observed, dim.Coverage.Total)
		}
		if len(dim.Metrics) == 0 {
			t.Errorf("%s is %q with no metric", dim.Name, dim.Status)
		}
		for _, m := range dim.Metrics {
			if len(m.Provenance) == 0 {
				t.Errorf("%s metric %q has no provenance", dim.Name, m.Name)
			}
			if m.Unit == "" {
				t.Errorf("%s metric %q has no unit", dim.Name, m.Name)
			}
		}
		if dim.Confidence == state.ConfidenceUnrated {
			t.Errorf("%s is %q but rates its own confidence unrated", dim.Name, dim.Status)
		}
	}
}

// TestPartialDimensionsNameWhatIsMissing pins that every dimension v1 cannot
// fully measure says which fact is absent and who would have to observe it.
func TestPartialDimensionsNameWhatIsMissing(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	for _, dim := range evaluation.BuildDimensions(diag, in, nil).All() {
		if dim.Status == state.Measured {
			continue
		}
		if len(dim.Unknown) == 0 {
			t.Errorf("%s is %q but names nothing it could not observe", dim.Name, dim.Status)
		}
		for _, unknown := range dim.Unknown {
			if unknown.Fact == "" || unknown.Reason == "" || unknown.Owner == "" {
				t.Errorf("%s unknown fact %+v is incomplete", dim.Name, unknown)
			}
		}
	}
}

// TestDimensionMetricsSkipUnmeasuredMetrics pins that an n/a metric is left out
// rather than copied in as a zero — an abstention is not a measurement.
func TestDimensionMetricsSkipUnmeasuredMetrics(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	dims := evaluation.BuildDimensions(diag, in, nil)
	for _, m := range dims.Modularity.Metrics {
		if m.Name == "encapsulation" {
			t.Fatalf("modularity reports the n/a encapsulation metric as a value: %+v", m)
		}
	}
	if !hasMetric(dims.Modularity.Metrics, "blast_radius") {
		t.Error("modularity dropped the measured blast_radius metric")
	}
	if !hasMetric(dims.Structure.Metrics, "cycle") {
		t.Error("structure dropped the measured cycle metric")
	}
}

// TestDimensionConfidenceFollowsTheWeakestEvidence pins that a dimension cannot
// be more confident than the metric it reports: blast_radius is low-confidence
// on a small module set, so modularity is too.
func TestDimensionConfidenceFollowsTheWeakestEvidence(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	dims := evaluation.BuildDimensions(diag, in, nil)
	if dims.Modularity.Confidence != state.ConfidenceLow {
		t.Errorf("modularity confidence = %q, want %q from a low-confidence blast_radius",
			dims.Modularity.Confidence, state.ConfidenceLow)
	}
	if dims.Structure.Confidence != state.ConfidenceHigh {
		t.Errorf("structure confidence = %q, want %q from a measured, high-confidence cycle count",
			dims.Structure.Confidence, state.ConfidenceHigh)
	}
}

// TestFindingsRouteToTheOwningDimension pins the routing table, including the
// documented fallback: a rule nobody can classify lands in intent rather than
// disappearing from every dimension.
func TestFindingsRouteToTheOwningDimension(t *testing.T) {
	t.Parallel()
	tests := []struct{ ruleID, want string }{
		{finding.RuleIDBCImbalanced, state.DimensionCoupling},
		{finding.RuleIDDuplicatedKnowledge, state.DimensionCoupling},
		{"staleness", state.DimensionIntent},
	}
	for _, tc := range tests {
		t.Run(tc.ruleID, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{Findings: []finding.Finding{
				{ID: tc.ruleID, RuleID: tc.ruleID, Kind: finding.KindAdvisory, Status: finding.StatusNew},
			}}
			st := evaluation.BuildState(diag, evaluation.StateInput{})
			for _, dim := range st.Dimensions.All() {
				routed := len(dim.Findings) == 1
				if routed != (dim.Name == tc.want) {
					t.Errorf("%s routed=%t, want it on %s only", dim.Name, routed, tc.want)
				}
			}
		})
	}
}

func TestOperationsDeclaredTopologyCompleteness(t *testing.T) {
	t.Parallel()

	t.Run("Dockerfiles and CODEOWNERS provenance measure the declared topology", func(t *testing.T) {
		t.Parallel()
		modules := map[string]policy.ModuleDef{
			assessModA: {Paths: []string{assessPathsA}, Owner: "@team-a", DeployUnit: assessUnitAPI},
			assessModB: {Paths: []string{assessPathsB}, Owner: "@team-b", DeployUnit: "declared-worker"},
		}
		facts := evaluation.Observations{
			DeclaredDeployUnits: map[string]string{assessModA: assessUnitAPI, assessModB: "declared-worker"},
			CorroboratedDeployUnits: map[string]modevidence.CorroboratedDeployUnit{
				assessModA: {Path: "a", Unit: assessUnitAPI, Source: modevidence.TopologySourceDockerfile},
				assessModB: {Path: "b", Unit: "detected-worker", Source: modevidence.TopologySourceDockerfile},
			},
			OwnerProvenance: map[string]modevidence.OwnerProvenance{
				assessModA: {Module: assessModA, Owner: "@team-a", Source: modevidence.TopologySourceCodeowners},
				assessModB: {Module: assessModB, Owner: "@team-b", Source: modevidence.TopologySourceCodeowners},
			},
		}
		dim := operationsDimensionForTest(modules, facts)
		if dim.Status != state.Measured {
			t.Fatalf("operations status = %q, want measured; unknown=%+v", dim.Status, dim.Unknown)
		}
		for name, want := range map[string]float64{
			"owners_from_codeowners":                2,
			"owners_from_git_author_fallback":       0,
			"declared_deploy_units":                 2,
			"corroborated_deploy_units":             2,
			"modules_with_corroborated_deploy_unit": 2,
			"matching_declared_deploy_units":        1,
			"mismatched_declared_deploy_units":      1,
		} {
			if got, ok := dimensionMetricValue(dim.Metrics, name); !ok || got != want {
				t.Errorf("metric %q = %v (found=%t), want %v", name, got, ok, want)
			}
		}
		for _, fact := range []string{state.FactRuntimeTopology, state.FactSupplyChainInventory} {
			if !hasUnknownFact(dim.Unknown, fact) {
				t.Errorf("measured operations unknowns = %+v, want out-of-claim %q", dim.Unknown, fact)
			}
		}
	})

	t.Run("missing corroboration and ownership statements stay partial", func(t *testing.T) {
		t.Parallel()
		modules := map[string]policy.ModuleDef{assessModA: {Paths: []string{assessPathsA}}}
		dim := operationsDimensionForTest(modules, evaluation.Observations{
			DeclaredDeployUnits: map[string]string{}, CorroboratedDeployUnits: map[string]modevidence.CorroboratedDeployUnit{},
			OwnerProvenance: map[string]modevidence.OwnerProvenance{},
		})
		if dim.Status != state.Partial {
			t.Fatalf("operations status = %q, want partial", dim.Status)
		}
		if !hasUnknownFact(dim.Unknown, state.FactCorroboratedDeployUnit) || !hasUnknownFact(dim.Unknown, state.FactOwnerProvenance) {
			t.Errorf("operations unknowns = %+v, want corroboration and owner provenance", dim.Unknown)
		}
		for _, unknown := range dim.Unknown {
			if unknown.Fact == state.FactCorroboratedDeployUnit && !strings.Contains(unknown.Reason, "corroborating deploy manifest") {
				t.Errorf("corroboration reason %q does not name the missing manifest", unknown.Reason)
			}
		}
	})

	t.Run("a declaration is never its own corroboration", func(t *testing.T) {
		t.Parallel()
		modules := map[string]policy.ModuleDef{assessModA: {
			Paths: []string{assessPathsA}, Owner: assessTeamA, DeployUnit: assessUnitAPI,
		}}
		dim := operationsDimensionForTest(modules, evaluation.Observations{
			DeclaredDeployUnits:     map[string]string{assessModA: assessUnitAPI},
			CorroboratedDeployUnits: map[string]modevidence.CorroboratedDeployUnit{},
			OwnerProvenance: map[string]modevidence.OwnerProvenance{
				assessModA: {Module: assessModA, Owner: assessTeamA, Source: modevidence.TopologySourceDeclared},
			},
		})
		if dim.Status != state.Partial {
			t.Fatalf("operations status = %q, want partial without independent corroboration", dim.Status)
		}
		for name, want := range map[string]float64{"declared_deploy_units": 1, "corroborated_deploy_units": 0} {
			if got, ok := dimensionMetricValue(dim.Metrics, name); !ok || got != want {
				t.Errorf("metric %q = %v (found=%t), want %v", name, got, ok, want)
			}
		}
	})

	t.Run("git-author provenance is surfaced but does not qualify", func(t *testing.T) {
		t.Parallel()
		modules := map[string]policy.ModuleDef{assessModA: {
			Paths: []string{assessPathsA}, Owner: "dev@example.com", DeployUnit: assessUnitAPI,
		}}
		dim := operationsDimensionForTest(modules, evaluation.Observations{
			DeclaredDeployUnits: map[string]string{assessModA: assessUnitAPI},
			CorroboratedDeployUnits: map[string]modevidence.CorroboratedDeployUnit{
				assessModA: {Path: "a", Unit: assessUnitAPI, Source: modevidence.TopologySourceDockerfile},
			},
			OwnerProvenance: map[string]modevidence.OwnerProvenance{
				assessModA: {Module: assessModA, Owner: "dev@example.com", Source: modevidence.TopologySourceGitAuthor},
			},
		})
		if dim.Status != state.Partial || !hasUnknownFact(dim.Unknown, state.FactOwnerProvenance) {
			t.Fatalf("operations = status %q unknown %+v, want partial owner-provenance gap", dim.Status, dim.Unknown)
		}
		if got, ok := dimensionMetricValue(dim.Metrics, "owners_from_git_author_fallback"); !ok || got != 1 {
			t.Errorf("owners_from_git_author_fallback = %v (found=%t), want 1", got, ok)
		}
	})
}

func operationsDimensionForTest(modules map[string]policy.ModuleDef, facts evaluation.Observations) state.Dimension {
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	input := evaluation.StateInput{Policy: policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil), Facts: facts}
	diag := &result.Result{ToolCoverage: []modevidence.Coverage{{Tool: toolDeployDiscovery, Status: modevidence.StatusOK}}}
	return evaluation.BuildDimensions(diag, input, nil).Operations
}

// TestEvidenceDependentDimensionsRequireCompletedProducers pins the Task 2
// status corrections: a partial primary graph cannot support structure or
// module-graph completeness, and it blocks only rules whose language scope
// actually depends on that producer.
func TestEvidenceDependentDimensionsRequireCompletedProducers(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	diag.PrimaryExtractorTools = append(diag.PrimaryExtractorTools, toolDepCruiser)
	diag.ToolCoverage = append(diag.ToolCoverage, modevidence.Coverage{
		Tool: toolDepCruiser, Status: modevidence.StatusPartial, Unresolved: 1, SpecifiersSeen: 20,
	})
	dims := evaluation.BuildDimensions(diag, in, nil)
	if dims.Intent.Status != state.Measured {
		t.Errorf("intent status = %q, want measured because its Go-scoped rule does not depend on the partial TypeScript producer", dims.Intent.Status)
	}
	for _, dim := range []state.Dimension{dims.Structure, dims.Modularity} {
		if dim.Status != state.Partial {
			t.Errorf("%s status = %q, want partial with a partial primary extractor", dim.Name, dim.Status)
		}
	}
}

// TestPartialDotNetEvaluationKeepsModularityPartial pins the offline .NET shape
// to the TypeScript precedent above: projects that fail evaluation make the
// dotnet-msbuild primary partial, so modularity is partial and names the
// module graph shape it could not observe. blast_radius is still computed and
// reported, as it is over a partial TypeScript graph; the dimension status,
// not the metric, carries the incompleteness.
func TestPartialDotNetEvaluationKeepsModularityPartial(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	diag.PrimaryExtractorTools = append(diag.PrimaryExtractorTools, "dotnet-msbuild")
	diag.ToolCoverage = append(diag.ToolCoverage,
		modevidence.Coverage{Tool: "dotnet-msbuild", Status: modevidence.StatusPartial},
		modevidence.Coverage{Tool: "roslyn", Status: modevidence.StatusPartial, Unresolved: 3, UnresolvedInputsMissing: 4},
	)
	dim := evaluation.BuildDimensions(diag, in, nil).Modularity
	if dim.Status != state.Partial || !hasUnknownFact(dim.Unknown, state.FactModuleGraphShape) {
		t.Errorf("modularity = status %q unknown %+v, want partial naming the module graph shape", dim.Status, dim.Unknown)
	}
	if !hasMetric(dim.Metrics, "blast_radius") {
		t.Errorf("modularity metrics = %+v, want blast_radius still reported", dim.Metrics)
	}
}

// toolAstGrep is the coverage name of the ast-grep pattern pass.

// TestCompletedEmptyDependencyInventoryIsMeasured pins STR-1: a producer that
// completed and found zero dependencies observed a zero; it did not abstain.
func TestCompletedEmptyDependencyInventoryIsMeasured(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	diag.ClassifiedEdges = &result.ClassifiedEdgeSummary{}
	diag.Metrics = []result.MetricResult{
		{Name: metricCycle, Band: bandNA, Confidence: volLow, Version: "cycle.v1"},
		{Name: "blast_radius", Band: bandNA, Confidence: volLow, Version: "blast_radius.v2"},
		{Name: metricEncapsulation, Band: bandNA, Confidence: volLow, Version: "encapsulation.v3"},
	}
	dims := evaluation.BuildDimensions(diag, in, nil)
	if dims.Structure.Status != state.Measured {
		t.Errorf("structure status = %q, want measured for a completed empty inventory", dims.Structure.Status)
	}
	if dims.Modularity.Status != state.Measured {
		t.Errorf("modularity status = %q, want measured for producer-proved empty graph facts", dims.Modularity.Status)
	}
}

// TestStructureUsesTheDependencyOnlyDenominator pins STR-3: containment edges,
// clone-only pairs, and declared external systems stay out of the structure
// claim even though coupling's existing summary still carries them. Library
// edges stay out of the denominator too — the bitwarden shape (19,749 internal
// + 8 unmapped first-party + 11,653 library) read as 19,749/31,410, a ratio no
// library-using repository can pass — while first-party edges no module owns
// stay in, because those are the gap the module map leaves.
func TestStructureUsesTheDependencyOnlyDenominator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		internal     int
		unmapped     int
		library      int
		wantObserved int
		wantTotal    int
	}{
		{name: "mixed", internal: 15, unmapped: 2, library: 3, wantObserved: 15, wantTotal: 17},
		{name: "library-heavy", internal: 15, unmapped: 2, library: 11653, wantObserved: 15, wantTotal: 17},
		{name: "no library edges", internal: 15, unmapped: 2, library: 0, wantObserved: 15, wantTotal: 17},
		{name: "every first-party edge unmapped", internal: 0, unmapped: 5, library: 0, wantObserved: 0, wantTotal: 5},
		{name: "only library edges", internal: 0, unmapped: 0, library: 20, wantObserved: 0, wantTotal: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag, in := dimensionsFixture()
			diag.ClassifiedEdges.InternalDependencies = tc.internal
			diag.ClassifiedEdges.UnmappedFirstPartyDependencies = tc.unmapped
			diag.ClassifiedEdges.LibraryDependencies = tc.library
			diag.ClassifiedEdges.DependencyEdges = tc.internal + tc.unmapped + tc.library
			dim := evaluation.BuildDimensions(diag, in, nil).Structure
			if dim.Coverage.Observed != tc.wantObserved || dim.Coverage.Total != tc.wantTotal {
				t.Errorf("structure coverage = %d/%d, want first-party %d/%d",
					dim.Coverage.Observed, dim.Coverage.Total, tc.wantObserved, tc.wantTotal)
			}
		})
	}

	diag, in := dimensionsFixture()
	dim := evaluation.BuildDimensions(diag, in, nil).Structure
	for name, want := range map[string]float64{
		"internal_edges": 15, "library_edges": 3, "unmapped_first_party_edges": 2,
		"same_module_edges": 3, "connected_modules": 2,
	} {
		if got, ok := dimensionMetricValue(dim.Metrics, name); !ok || got != want {
			t.Errorf("metric %q = %v (found=%t), want %v", name, got, ok, want)
		}
	}
}

// TestMeasuredRuntimeDimensionsOnlyDiscloseOutOfClaimUnknowns applies the fixed
// fact registry to real collector output, not only to synthetic Promote calls.
func TestMeasuredRuntimeDimensionsOnlyDiscloseOutOfClaimUnknowns(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	for _, dim := range evaluation.BuildDimensions(diag, in, nil).All() {
		if dim.Status != state.Measured {
			continue
		}
		facts := state.RequiredFacts(dim.Name)
		for _, unknown := range dim.Unknown {
			foundOutOfClaim := false
			for _, fact := range facts {
				if fact.Name == unknown.Fact && !fact.InClaim {
					foundOutOfClaim = true
					break
				}
			}
			if !foundOutOfClaim {
				t.Errorf("measured %s carries undeclared or in-claim unknown %+v", dim.Name, unknown)
			}
		}
	}
}

// TestBuildDimensionsIsDeterministic pins that two identical runs produce
// byte-identical envelopes. Several collectors read maps, so a stray map
// iteration would show up here and nowhere else.
func TestBuildDimensionsIsDeterministic(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	first := evaluation.BuildDimensions(diag, in, nil)
	for range 5 {
		if next := evaluation.BuildDimensions(diag, in, nil); !reflect.DeepEqual(first, next) {
			t.Fatalf("repeated runs differ:\nfirst = %+v\nnext  = %+v", first, next)
		}
	}
}

func hasMetric(metrics []state.MetricValue, name string) bool {
	_, ok := dimensionMetricValue(metrics, name)
	return ok
}

func dimensionMetricValue(metrics []state.MetricValue, name string) (float64, bool) {
	for _, m := range metrics {
		if m.Name == name {
			return m.Value, true
		}
	}
	return 0, false
}

func hasUnknownFact(unknown []state.UnknownFact, name string) bool {
	for _, fact := range unknown {
		if fact.Fact == name {
			return true
		}
	}
	return false
}

func TestDriftDimensionHasNoStoredComparison(t *testing.T) {
	t.Parallel()
	diag := &result.Result{Seams: []result.Seam{{ID: "seam", DistributedMonolith: true}}}
	dim := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).Drift
	if dim.Status != state.Unmeasured || len(dim.Metrics) != 0 {
		t.Fatalf("drift = %+v, want unmeasured without fabricated deltas", dim)
	}
}

// TestCouplingDimension_UnresolvedTypeScriptImportsLowerTheEnvelope pins the
// coupling envelope against the honesty defect the corpus sweep found on
// storybook: dependency-cruiser left 55% of the import specifiers unresolved,
// coupling_balance disclosed "high confidence disallowed", and the PRIMARY
// contract still reported `measured` with `high` confidence and no unknown
// fact. Two answers about the same evidence, and the wrong one was primary.
func TestCouplingDimension_UnresolvedTypeScriptImportsLowerTheEnvelope(t *testing.T) {
	t.Parallel()

	// Every edge that survived resolution is scored, so the scored fraction is
	// 100% and cannot express what dependency-cruiser dropped.
	edges := &result.ClassifiedEdgeSummary{Total: 10, Scored: 10, ConnectedModules: 2}

	for _, tc := range []struct {
		name           string
		coverage       modevidence.Coverage
		wantStatus     state.MeasurementStatus
		wantConfidence state.Confidence
		wantUnknown    bool
	}{
		{
			name:           "a clean TypeScript extraction stays measured",
			coverage:       modevidence.Coverage{Tool: toolDepCruiser, Status: modevidence.StatusOK},
			wantStatus:     state.Measured,
			wantConfidence: state.ConfidenceHigh,
		},
		{
			// Exactly at the ceiling: the cap is strictly greater-than, so this
			// side of the boundary must not move the envelope.
			name: "unresolved specifiers at the ceiling stay measured",
			coverage: modevidence.Coverage{
				Tool: toolDepCruiser, Status: modevidence.StatusPartial,
				Unresolved: 10, SpecifiersSeen: 100,
			},
			wantStatus:     state.Measured,
			wantConfidence: state.ConfidenceHigh,
		},
		{
			name: "unresolved specifiers above the ceiling make it partial",
			coverage: modevidence.Coverage{
				Tool: toolDepCruiser, Status: modevidence.StatusPartial,
				Unresolved: 6288, SpecifiersSeen: 11337,
			},
			wantStatus:     state.Partial,
			wantConfidence: state.ConfidenceMedium,
			wantUnknown:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{
				Findings:        []finding.Finding{},
				ClassifiedEdges: edges,
				ToolCoverage:    []modevidence.Coverage{tc.coverage},
			}
			dim := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).Coupling
			if dim.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", dim.Status, tc.wantStatus)
			}
			if dim.Confidence != tc.wantConfidence {
				t.Errorf("confidence = %q, want %q", dim.Confidence, tc.wantConfidence)
			}
			named := false
			for _, u := range dim.Unknown {
				if u.Fact == state.FactExtractorResolutionWithinCeiling {
					named = true
				}
			}
			if named != tc.wantUnknown {
				t.Errorf("names the unresolved imports = %v, want %v (unknown: %+v)", named, tc.wantUnknown, dim.Unknown)
			}
		})
	}
}

// TestCouplingDimension_UnresolvedRoslynSymbolsKeepItPartial pins the .NET
// promotion rule: every resolved edge is scored and rated, yet one unresolved
// Roslyn reference may hide a cross-boundary edge, so the envelope stays
// partial and names why. There is no ratio ceiling as for TypeScript.
func TestCouplingDimension_UnresolvedRoslynSymbolsKeepItPartial(t *testing.T) {
	t.Parallel()
	const roslyn = "roslyn"
	edges := &result.ClassifiedEdgeSummary{Total: 10, Scored: 10, ConnectedModules: 2}
	for _, tc := range []struct {
		name       string
		coverage   modevidence.Coverage
		wantStatus state.MeasurementStatus
	}{
		{"a complete Roslyn run stays measured", modevidence.Coverage{Tool: roslyn, Status: modevidence.StatusOK}, state.Measured},
		{"one unresolved reference makes it partial", modevidence.Coverage{Tool: roslyn, Status: modevidence.StatusPartial, Unresolved: 1, UnresolvedInputsMissing: 1}, state.Partial},
		{"a timed-out Roslyn run makes it partial", modevidence.Coverage{Tool: roslyn, Status: modevidence.StatusTimedOut}, state.Partial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{
				Findings:        []finding.Finding{},
				ClassifiedEdges: edges,
				ToolCoverage:    []modevidence.Coverage{{Tool: "dotnet-msbuild", Status: modevidence.StatusOK}, tc.coverage},
			}
			dim := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).Coupling
			if dim.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (unknown: %+v)", dim.Status, tc.wantStatus, dim.Unknown)
			}
			var reason string
			for _, u := range dim.Unknown {
				if u.Fact == state.FactExtractorResolutionWithinCeiling {
					reason = u.Reason
				}
			}
			if blocked := tc.wantStatus == state.Partial; blocked != strings.Contains(reason, roslyn) {
				t.Errorf("resolution unknown reason = %q, want one naming roslyn: %v", reason, blocked)
			}
		})
	}
}

func TestCouplingDimension_TypeScriptAndRoslynBothNamed(t *testing.T) {
	t.Parallel()
	diag := &result.Result{
		Findings:        []finding.Finding{},
		ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 10, Scored: 10, ConnectedModules: 2},
		ToolCoverage: []modevidence.Coverage{
			{Tool: toolDepCruiser, Status: modevidence.StatusPartial, Unresolved: 5, SpecifiersSeen: 10},
			{Tool: "roslyn", Status: modevidence.StatusPartial, Unresolved: 1, UnresolvedInputsMissing: 1},
		},
	}
	dim := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).Coupling
	var reason string
	for _, u := range dim.Unknown {
		if u.Fact == state.FactExtractorResolutionWithinCeiling {
			reason = u.Reason
		}
	}
	tsAt, roslynAt := strings.Index(reason, "dependency-cruiser"), strings.Index(reason, "roslyn")
	if dim.Status != state.Partial || tsAt < 0 || roslynAt < tsAt || !strings.Contains(reason, "; roslyn") {
		t.Errorf("status = %q, reason = %q, want partial naming dependency-cruiser then roslyn", dim.Status, reason)
	}
}

func TestCouplingDimension_UnratedVolatilityMakesItPartial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		edges      int
		modules    []string
		wantStatus state.MeasurementStatus
		wantReason string
	}{
		{name: "every end rated", wantStatus: state.Measured},
		{name: "one module named", edges: 3, modules: []string{"a"}, wantStatus: state.Partial,
			wantReason: "volatility is undeclared on 3 scored cross-boundary edge(s); declare `volatility:` or `subdomain:` on: a"},
		{name: "five modules all named", edges: 9, modules: []string{"a", "b", "c", "d", "e"}, wantStatus: state.Partial,
			wantReason: "volatility is undeclared on 9 scored cross-boundary edge(s); declare `volatility:` or `subdomain:` on: a, b, c, d, e"},
		{name: "more than five are counted", edges: 9, modules: []string{"a", "b", "c", "d", "e", "f", "g"}, wantStatus: state.Partial,
			wantReason: "volatility is undeclared on 9 scored cross-boundary edge(s); declare `volatility:` or `subdomain:` on: a, b, c, d, e and 2 more"},
		{name: "no module list", edges: 1, wantStatus: state.Partial,
			wantReason: "volatility is undeclared on 1 scored cross-boundary edge(s); declare `volatility:` or `subdomain:` on the modules involved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{
				Findings: []finding.Finding{},
				ClassifiedEdges: &result.ClassifiedEdgeSummary{
					Total: 10, Scored: 10, ConnectedModules: 2,
					UnratedVolatilityEdges: tc.edges, UnratedVolatilityModules: tc.modules,
				},
			}
			dim := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).Coupling
			if dim.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (unknown %+v)", dim.Status, tc.wantStatus, dim.Unknown)
			}
			var got string
			for _, u := range dim.Unknown {
				if u.Fact == state.FactCouplingVolatility {
					got = u.Reason
				}
			}
			if got != tc.wantReason {
				t.Errorf("reason = %q, want %q", got, tc.wantReason)
			}
		})
	}
}

func TestCouplingDimension_QualifyingEdgesMetric(t *testing.T) {
	t.Parallel()
	diag := &result.Result{
		Findings: []finding.Finding{},
		ClassifiedEdges: &result.ClassifiedEdgeSummary{
			Total: 4, Scored: 4, ConnectedModules: 2,
			TailRisk: &result.CouplingTailRiskSummary{DistributedMonolithEdges: 2},
		},
	}
	dim := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).Coupling
	found := false
	for _, m := range dim.Metrics {
		if m.Name == "critical_high_distance_edges" {
			t.Errorf("old metric name still emitted")
		}
		if m.Name == "qualifying_edges" {
			found = true
			if m.Value != 2 {
				t.Errorf("qualifying_edges = %v, want 2", m.Value)
			}
		}
	}
	if !found {
		t.Error("qualifying_edges metric missing")
	}
}
