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
	ruleNoAToB = "no_a_to_b"
	ruleDep    = "dep"
)

// dimensionsFixture is a run with something to measure in every dimension that
// v1 can measure at all.
func dimensionsFixture() (*result.Result, evaluation.StateInput) {
	diag := &result.Result{
		Findings: []finding.Finding{},
		Metrics: []result.MetricResult{
			{Name: metricCycle, Value: 0, Band: "strong", Confidence: confHigh, Version: "cycle.v1", Mode: "count"},
			{Name: "blast_radius", Value: 2, Band: "info", Confidence: volLow, Version: "blast_radius.v1", Mode: "count"},
			{Name: "encapsulation", Value: 0, Band: "n/a", Confidence: volLow, Version: "encapsulation.v3", Mode: "ratio"},
			{Name: "coverage", Value: 1, Band: "strong", Confidence: confHigh, Version: "coverage.v1", Mode: "ratio"},
		},
		ClassifiedEdges: &result.ClassifiedEdgeSummary{
			Total: 20, Scored: 10, Abstained: 2, SameModule: 3, External: 5, ConnectedModules: 2,
			TailRisk: &result.CouplingTailRiskSummary{CriticalEdges: 1, HighOrWorseEdges: 3, DistributedMonolithEdges: 0},
		},
		VolatilityCorroboration: &modevidence.VolatilityCorroboration{
			Source: assessHistory, Status: modevidence.StatusOK, CommitsScanned: 120, ModulesTouched: 4,
		},
		ToolCoverage: []modevidence.Coverage{
			{Tool: "go/packages", Status: modevidence.StatusOK},
			{Tool: "scip", Status: modevidence.StatusAbsent},
		},
	}
	modules := map[string]policy.ModuleDef{
		assessModA: {Paths: []string{assessPathsA}, Public: []string{"a/api"}, Owner: "team-a", DeployUnit: "svc"},
		assessModB: {Paths: []string{assessPathsB}, Owner: "team-a"},
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
			FileLOC:        map[string]int{"a/a.go": 100, "a/a_test.go": 40, "b/b.go": 250},
			FileClassIndex: map[string]fileclass.FileClass{"a/a.go": fileclass.Production, "a/a_test.go": fileclass.Test, "b/b.go": fileclass.Production},
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
				state.DimensionComplexity:     state.Partial,
				state.DimensionTestability:    state.Partial,
				state.DimensionOperations:     state.Partial,
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
		"newdep": "new_cross_module_dependency", "pub": "public_api_only", "max": "public_api_max",
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
	for _, m := range metrics {
		if m.Name == name {
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
		name         string
		anchor       evaluation.BaselineAnchor
		wantStatus   state.MeasurementStatus
		wantDelta    state.ComparisonStatus
		wantMetrics  map[string]float64
		wantReasonIn string
	}{
		{
			name:         "no stored reference",
			anchor:       evaluation.BaselineAnchor{},
			wantStatus:   state.Unmeasured,
			wantDelta:    state.ComparisonNonComparable,
			wantReasonIn: "no comparable architecture-state reference",
		},
		{
			name: "drifted fingerprints name the input that moved",
			anchor: evaluation.BaselineAnchor{
				NonComparableReason: "the stored baseline was written under different inputs",
				SnapshotMismatches:  []string{"config_hash differs between the two runs"},
			},
			wantStatus:   state.Unmeasured,
			wantDelta:    state.ComparisonNonComparable,
			wantReasonIn: "config_hash",
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
			if tc.wantReasonIn != "" {
				joined := strings.Join(dim.Delta.Reasons, " ")
				if !strings.Contains(joined, tc.wantReasonIn) {
					t.Errorf("reasons %q do not name %q", joined, tc.wantReasonIn)
				}
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
