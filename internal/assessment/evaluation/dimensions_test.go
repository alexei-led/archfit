// Behavior tests for the nine architecture-state envelopes. They pin what each
// dimension measures, what it refuses to measure, that every measured envelope
// carries a denominator and provenance, and that findings reach the dimension
// that owns their subject.
package evaluation_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	ruleNoAToB       = "no_a_to_b"
	ruleDep          = "dep"
	rulePublicAPIMax = "public_api_max"
	gateWarnPosture  = "warn"
	// toolDepCruiser is the TypeScript primary analyzer's coverage name.
	toolDepCruiser      = "dependency-cruiser"
	metricEncapsulation = "encapsulation"
	bandNA              = "n/a"
	assessUnitSvc       = "svc"
	assessUnitAPI       = "api"
	assessTeamA         = "team-a"
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
			SameModuleDependencies: 3, DependencyModules: 2, FirstPartyNodes: 2, AttributedFirstPartyNodes: 2,
			TailRisk: &result.CouplingTailRiskSummary{CriticalEdges: 1, HighOrWorseEdges: 3, DistributedMonolithEdges: 0},
		},
		VolatilityCorroboration: &modevidence.VolatilityCorroboration{
			Source: assessHistory, Status: modevidence.StatusOK, CommitsScanned: 120, ModulesTouched: 4,
		},
		ToolCoverage: []modevidence.Coverage{
			{Tool: primaryTool, Status: modevidence.StatusOK},
			{Tool: "scip", Status: modevidence.StatusAbsent},
		},
		PrimaryExtractorTools: []string{primaryTool},
	}
	modules := map[string]policy.ModuleDef{
		assessModA: {Paths: []string{assessPathsA}, Public: []string{"a/api"}, Owner: assessTeamA, DeployUnit: assessUnitSvc},
		assessModB: {Paths: []string{assessPathsB}, Owner: assessTeamA},
	}
	topology := policy.TopologyView{Modules: modules, Layers: []string{assessCore, "adapter"}, ModuleMap: policy.BuildModuleMap(modules)}
	gates := policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{
		{ID: ruleNoAToB, Type: ruleForbidden, Gate: "fail"},
		{ID: "off_rule", Type: "cycle", Gate: "off"},
	}}}
	in := evaluation.StateInput{
		Policy:    policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, gates, nil, nil),
		RuleTypes: map[string]string{ruleNoAToB: ruleForbidden, "off_rule": metricCycle},
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
				state.DimensionChangeLocality: state.Measured,
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

// TestChangeLocalityRefusesAnEmptyHistory is R7: zero observed commits is
// unmeasured, never a quiet zero that reads as a stable architecture.
func TestChangeLocalityRefusesAnEmptyHistory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		history *modevidence.VolatilityCorroboration
		want    state.MeasurementStatus
	}{
		{"no history at all", nil, state.Unmeasured},
		{"scan returned no commit", &modevidence.VolatilityCorroboration{Status: modevidence.StatusOK}, state.Unmeasured},
		{"timed-out scan is partial", &modevidence.VolatilityCorroboration{Status: "timeout", CommitsScanned: 10, ModulesTouched: 2}, state.Partial},
		{"complete scan is measured", &modevidence.VolatilityCorroboration{Status: modevidence.StatusOK, CommitsScanned: 10, ModulesTouched: 2}, state.Measured},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{VolatilityCorroboration: tc.history}
			got := evaluation.BuildDimensions(diag, evaluation.StateInput{}, nil).ChangeLocality
			if got.Status != tc.want {
				t.Errorf("change locality status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}

// TestFindingsRouteToTheOwningDimension pins the routing table, including the
// documented fallback: a rule nobody can classify lands in intent rather than
// disappearing from every dimension.
func TestFindingsRouteToTheOwningDimension(t *testing.T) {
	t.Parallel()
	ruleTypes := map[string]string{
		ruleDep: ruleForbidden, "layer": "forbidden_layer_direction", "cyc": metricCycle,
		"newdep": "new_cross_module_dependency", "pub": "public_api_only", "max": rulePublicAPIMax,
		"leak": "public_api_type_leak", "internal": "internal_api_access", "waiver": "waiver_expiry",
	}
	tests := []struct {
		ruleID string
		want   string
	}{
		{ruleDep, state.DimensionStructure},
		{"layer", state.DimensionStructure},
		{"cyc", state.DimensionStructure},
		{"newdep", state.DimensionStructure},
		{"pub", state.DimensionModularity},
		{"max", state.DimensionModularity},
		{"leak", state.DimensionModularity},
		{"internal", state.DimensionModularity},
		{finding.RuleIDBCImbalanced, state.DimensionCoupling},
		{finding.RuleIDDuplicatedKnowledge, state.DimensionCoupling},
		{finding.RuleIDCouplingGate, state.DimensionCoupling},
		{"labels/stale", state.DimensionCoupling},
		{"waiver", state.DimensionIntent},
		{"nobody_declared_this", state.DimensionIntent},
	}
	for _, tc := range tests {
		t.Run(tc.ruleID, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{Findings: []finding.Finding{
				{ID: tc.ruleID, RuleID: tc.ruleID, Kind: finding.KindGate, Status: finding.StatusNew},
			}}
			st := evaluation.BuildState(diag, evaluation.StateInput{RuleTypes: ruleTypes})
			for _, dim := range st.Dimensions.All() {
				routed := len(dim.Findings) == 1
				if routed != (dim.Name == tc.want) {
					t.Errorf("%s routed=%t, want it on %s only", dim.Name, routed, tc.want)
				}
			}
		})
	}
}

// TestDimensionGatePosture pins that a dimension fails only on a routed hard
// gate, warns on an active diagnostic, and passes when it measured something
// with nothing active. An unmeasured dimension gates nothing.
func TestDimensionGatePosture(t *testing.T) {
	t.Parallel()
	ruleTypes := map[string]string{ruleDep: ruleForbidden}
	tests := []struct {
		name    string
		finding *finding.Finding
		want    state.GateState
	}{
		{"nothing active passes", nil, state.GatePass},
		{"active advisory warns", &finding.Finding{ID: "a", RuleID: ruleDep, Kind: finding.KindAdvisory, Status: finding.StatusNew}, state.GateWarn},
		{"active gate finding fails", &finding.Finding{ID: "g", RuleID: ruleDep, Kind: finding.KindGate, Status: finding.StatusNew}, state.GateFail},
		{"accepted gate finding passes", &finding.Finding{ID: "b", RuleID: ruleDep, Kind: finding.KindGate, Status: finding.StatusBaseline}, state.GatePass},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag, in := dimensionsFixture()
			in.RuleTypes = ruleTypes
			if tc.finding != nil {
				diag.Findings = []finding.Finding{*tc.finding}
			}
			got := evaluation.BuildState(diag, in).Dimensions.Structure.Gate
			if got != tc.want {
				t.Errorf("structure gate = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOperationsFailsOnARequiredToolPolicyFailure pins the one gate that has no
// finding behind it: a required analyzer that did not run fails operations even
// though nothing violated a rule.
func TestOperationsFailsOnARequiredToolPolicyFailure(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	in.RequiredToolFailure = true
	if got := evaluation.BuildDimensions(diag, in, nil).Operations.Gate; got != state.GateFail {
		t.Errorf("operations gate = %q, want %q", got, state.GateFail)
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
	return evaluation.BuildDimensions(&result.Result{}, input, nil).Operations
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

	in.Policy.Gates.Rules.Rules = []policy.RuleDef{{
		ID: "typescript_dependency", Type: ruleForbidden, Gate: gateWarnPosture,
		From: "src/core/**/*.ts", To: "src/ui/**/*.ts",
	}}
	in.Facts.FileClassIndex["src/core/index.ts"] = fileclass.Production
	in.Facts.FileClassIndex["src/ui/index.ts"] = fileclass.Production
	if got := evaluation.BuildDimensions(diag, in, nil).Intent.Status; got != state.Partial {
		t.Errorf("TypeScript-scoped intent status = %q, want partial with its partial producer", got)
	}
}

// TestIntentRequiresSyntaxEvidenceForSyntaxRules prevents a rule invocation over
// an empty syntax fact set from being mistaken for a successful conformance
// result.
func TestIntentRequiresSyntaxEvidenceForSyntaxRules(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	maxPublicAPIs := 5
	in.Policy.Gates.Rules.Rules = append(in.Policy.Gates.Rules.Rules, policy.RuleDef{
		ID: "public_api_limit", Type: rulePublicAPIMax, Gate: gateWarnPosture, Max: &maxPublicAPIs,
	})
	dim := evaluation.BuildDimensions(diag, in, nil).Intent
	if dim.Status != state.Partial {
		t.Fatalf("intent status = %q, want partial without ast-grep/syntax completion", dim.Status)
	}
	if !hasUnknownFact(dim.Unknown, state.FactActiveRuleConformance) {
		t.Errorf("intent unknowns = %+v, want %q", dim.Unknown, state.FactActiveRuleConformance)
	}
}

// TestIntentDoesNotConformUnsupportedLanguageRules prevents an empty supported-
// language graph or syntax result from acting as evidence about a rule whose
// declared scope is Java. Unsupported scope is unknown, not producer-proved n/a.
func TestIntentDoesNotConformUnsupportedLanguageRules(t *testing.T) {
	t.Parallel()
	maxPublicAPIs := 5
	primaryTools := []string{"go/packages", "dependency-cruiser", "grimp", "cargo"}
	absentPrimaries := []modevidence.Coverage{
		{Tool: primaryTools[0], Status: modevidence.StatusAbsent},
		{Tool: primaryTools[1], Status: modevidence.StatusAbsent},
		{Tool: primaryTools[2], Status: modevidence.StatusAbsent},
		{Tool: primaryTools[3], Status: modevidence.StatusAbsent},
	}
	modules := map[string]policy.ModuleDef{
		"java-service": {Paths: []string{"src/**/*.java"}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	tests := []struct {
		name     string
		rule     policy.RuleDef
		coverage []modevidence.Coverage
	}{
		{
			name: "dependency rule",
			rule: policy.RuleDef{ID: "java_dependency", Type: ruleForbidden, Gate: gateWarnPosture,
				From: "src/domain/**/*.java", To: "src/adapter/**/*.java"},
			coverage: absentPrimaries,
		},
		{
			name: "dependency target scope",
			rule: policy.RuleDef{ID: "go_to_java_dependency", Type: ruleForbidden, Gate: gateWarnPosture,
				From: "src/domain/**/*.go", To: "src/adapter/**/*.java"},
			coverage: []modevidence.Coverage{
				{Tool: primaryTools[0], Status: modevidence.StatusOK},
				{Tool: primaryTools[1], Status: modevidence.StatusAbsent},
				{Tool: primaryTools[2], Status: modevidence.StatusAbsent},
				{Tool: primaryTools[3], Status: modevidence.StatusAbsent},
			},
		},
		{
			name: "syntax rule despite aggregate syntax success",
			rule: policy.RuleDef{ID: "java_api_limit", Type: rulePublicAPIMax, Gate: gateWarnPosture, Max: &maxPublicAPIs},
			coverage: append(append([]modevidence.Coverage(nil), absentPrimaries...),
				modevidence.Coverage{Tool: "ast-grep/syntax", Status: modevidence.StatusOK}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gates := policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{tc.rule}}}
			in := evaluation.StateInput{
				Policy: policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, gates, nil, nil),
				Facts: evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
					"src/domain/service.go":   fileclass.Production,
					"src/domain/Service.java": fileclass.Production,
				}},
			}
			diag := &result.Result{PrimaryExtractorTools: primaryTools, ToolCoverage: tc.coverage}
			dim := evaluation.BuildDimensions(diag, in, nil).Intent
			if dim.Status != state.Partial {
				t.Fatalf("intent status = %q, want partial for unsupported-language rule scope", dim.Status)
			}
			if dim.Coverage.Observed != 0 || dim.Coverage.Total != 1 {
				t.Errorf("intent rule coverage = %d/%d, want 0/1", dim.Coverage.Observed, dim.Coverage.Total)
			}
			if !hasUnknownFact(dim.Unknown, state.FactActiveRuleConformance) {
				t.Errorf("intent unknowns = %+v, want %q", dim.Unknown, state.FactActiveRuleConformance)
			}
		})
	}
}

func TestIntentOnlyAcceptsExplicitlyEmptySupportedRuleScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		from       string
		wantStatus state.MeasurementStatus
		wantSeen   int
	}{
		{name: "supported language scope", from: "missing/**/*.go", wantStatus: state.Measured, wantSeen: 1},
		{name: "language-ambiguous scope", from: "missing/**", wantStatus: state.Partial, wantSeen: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag, in := dimensionsFixture()
			in.Policy.Gates.Rules.Rules = []policy.RuleDef{{
				ID: "empty_scope", Type: ruleForbidden, Gate: gateWarnPosture,
				From: tc.from, To: "a/**/*.go",
			}}
			dim := evaluation.BuildDimensions(diag, in, nil).Intent
			if dim.Status != tc.wantStatus {
				t.Fatalf("intent status = %q, want %q", dim.Status, tc.wantStatus)
			}
			if dim.Coverage.Observed != tc.wantSeen || dim.Coverage.Total != 1 {
				t.Errorf("intent rule coverage = %d/%d, want %d/1", dim.Coverage.Observed, dim.Coverage.Total, tc.wantSeen)
			}
		})
	}
}

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
// claim even though coupling's existing summary still carries them.
func TestStructureUsesTheDependencyOnlyDenominator(t *testing.T) {
	t.Parallel()
	diag, in := dimensionsFixture()
	dim := evaluation.BuildDimensions(diag, in, nil).Structure
	if dim.Coverage.Observed != 15 || dim.Coverage.Total != 20 {
		t.Errorf("structure coverage = %d/%d, want dependency-only 15/20", dim.Coverage.Observed, dim.Coverage.Total)
	}
	for name, want := range map[string]float64{
		"internal_edges": 15, "external_edges": 5, "same_module_edges": 3, "connected_modules": 2,
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

// TestDriftDimensionRequiresAComparableReference is the erosion contract: drift
// is measurable only against a reference written under the same config, module
// map, labels, and rubric. Everything else stays unmeasured with a named cause,
// because a reference that records no seams is not evidence that there were
// none.
func TestDriftDimensionRequiresAComparableReference(t *testing.T) {
	t.Parallel()
	diag := &result.Result{Seams: []result.Seam{
		{ID: "seam-kept", DistributedMonolith: true},
		{ID: "seam-new", DistributedMonolith: true},
		{ID: "seam-ignored"},
	}}

	tests := []struct {
		name          string
		anchor        evaluation.BaselineAnchor
		wantStatus    state.MeasurementStatus
		wantDelta     state.ComparisonStatus
		wantMetrics   map[string]float64
		wantReasonIn  string
		wantReasonOut string
	}{
		{
			name:          "comparison not requested without a persisted baseline",
			anchor:        evaluation.BaselineAnchor{},
			wantStatus:    state.Unmeasured,
			wantDelta:     state.ComparisonNonComparable,
			wantReasonIn:  "no comparable architecture-state reference",
			wantReasonOut: "stored baseline was written under different inputs",
		},
		{
			name: "persisted baseline with drifted fingerprints",
			anchor: evaluation.BaselineAnchor{
				NonComparableReason: "the stored baseline was written under different inputs",
				SnapshotMismatches:  []string{"config_hash differs between the two runs"},
			},
			wantStatus:    state.Unmeasured,
			wantDelta:     state.ComparisonNonComparable,
			wantReasonIn:  "config_hash",
			wantReasonOut: "no comparable architecture-state reference is stored",
		},
		{
			name: "comparable reference measures the seam delta",
			anchor: evaluation.BaselineAnchor{
				SeamsComparable:   true,
				QualifyingSeamIDs: []string{"seam-kept", "seam-gone"},
			},
			wantStatus:  state.Measured,
			wantDelta:   state.ComparisonComparable,
			wantMetrics: map[string]float64{"new_seams": 1, "resolved_seams": 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dim := evaluation.BuildDimensions(diag, evaluation.StateInput{Drift: tc.anchor}, nil).Drift
			if dim.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", dim.Status, tc.wantStatus)
			}
			if dim.Delta == nil {
				t.Fatal("drift envelope carries no delta")
			}
			if dim.Delta.Status != tc.wantDelta {
				t.Errorf("delta status = %q, want %q", dim.Delta.Status, tc.wantDelta)
			}
			joinedReasons := strings.Join(dim.Delta.Reasons, " ")
			if tc.wantReasonIn != "" && !strings.Contains(joinedReasons, tc.wantReasonIn) {
				t.Errorf("reasons %q do not name %q", joinedReasons, tc.wantReasonIn)
			}
			if tc.wantReasonOut != "" && strings.Contains(joinedReasons, tc.wantReasonOut) {
				t.Errorf("reasons %q unexpectedly contain %q", joinedReasons, tc.wantReasonOut)
			}
			for name, want := range tc.wantMetrics {
				got, found := 0.0, false
				for _, m := range dim.Metrics {
					if m.Name == name {
						got, found = m.Value, true
					}
				}
				if !found {
					t.Errorf("metric %q missing from a measured drift envelope", name)
					continue
				}
				if got != want {
					t.Errorf("metric %q = %v, want %v", name, got, want)
				}
			}
		})
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

// TestSyntaxEvidenceRequiresPrimaryOKNotMerelyNonDisabled is a regression test
// for the bug where syntaxEvidenceComplete checked == StatusDisabled instead of
// != StatusOK. A primary row of StatusAbsent (gapless: the extractor ran but
// found no applicable files for a language that is present in the topology)
// incorrectly passed the disabled check, marking a syntax rule as evaluated
// despite zero actual syntax evidence for that language.
func TestSyntaxEvidenceRequiresPrimaryOKNotMerelyNonDisabled(t *testing.T) {
	t.Parallel()
	const toolGoPkgs = "go/packages" // local to keep the goconst literal count low
	maxPublicAPIs := 5
	// A Go module exists in topology and a Go file is observed, so
	// moduleRuleScope returns {go} as applicable. The syntax aggregate tool
	// ran successfully but the Go primary row is StatusAbsent (the extractor
	// was excluded via config). Old code: StatusAbsent != StatusDisabled →
	// syntaxEvidenceComplete returned true → rule counted as evaluated (bug).
	// New code: StatusAbsent != StatusOK → returns false → partial (correct).
	modules := map[string]policy.ModuleDef{
		"svc": {Paths: []string{"internal/**/*.go"}, Public: []string{"internal/api"}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	gates := policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{
		{ID: "api_surface_limit", Type: rulePublicAPIMax, Gate: gateWarnPosture, Max: &maxPublicAPIs},
	}}}
	in := evaluation.StateInput{
		Policy: policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, gates, nil, nil),
		Facts: evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{
			"internal/handler/handler.go": fileclass.Production,
		}},
	}
	diag := &result.Result{
		PrimaryExtractorTools: []string{toolGoPkgs},
		ToolCoverage: []modevidence.Coverage{
			// Syntax aggregate: OK (ran for the project).
			{Tool: "ast-grep/syntax", Status: modevidence.StatusOK},
			// Go primary: Absent — extractor was excluded, no applicable files.
			// This must NOT be treated as "not disabled" and therefore passing.
			{Tool: toolGoPkgs, Status: modevidence.StatusAbsent},
		},
	}
	dim := evaluation.BuildDimensions(diag, in, nil).Intent
	if dim.Status != state.Partial {
		t.Fatalf("intent status = %q, want partial: syntax aggregate OK but Go primary absent should block rule evaluation", dim.Status)
	}
	if !hasUnknownFact(dim.Unknown, state.FactActiveRuleConformance) {
		t.Errorf("intent unknowns = %+v, want %q", dim.Unknown, state.FactActiveRuleConformance)
	}
}
