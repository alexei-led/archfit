package evaluation

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

func TestComplexityModuleGraphDistribution(t *testing.T) {
	modules := complexityModules("a", "b", "c", "d", "e")
	graph := relationship.Set{Edges: []relationship.Edge{
		complexityEdge("a", "b"), complexityEdge("a", "c"), complexityEdge("a", "d"), complexityEdge("a", "e"),
		complexityEdge("b", "c"), complexityEdge("b", "d"), complexityEdge("b", "e"),
		complexityEdge("a", "b"), // duplicate source edges are one module neighbour
	}}
	got := moduleGraphComplexity(modules, graph)
	if got == nil {
		t.Fatal("moduleGraphComplexity returned nil")
	}
	want := result.ModuleGraphComplexity{Modules: 5, MaxDependencyChain: 2, FanInP90: 2, FanOutP90: 4}
	if *got != want {
		t.Fatalf("module graph complexity = %+v, want %+v", *got, want)
	}
}

func TestComplexityDependencyDepthUsesSCCCondensation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		modules map[string]policy.ModuleDef
		edges   []relationship.Edge
		want    int
	}{
		{name: "single edgeless module", modules: complexityModules("a"), want: 0},
		{name: "cycle internal edges add no depth", modules: complexityModules("a", "b"), edges: []relationship.Edge{
			complexityEdge("a", "b"), complexityEdge("b", "a"),
		}, want: 0},
		{name: "an edge leaving a cycle adds one", modules: complexityModules("a", "b", "c"), edges: []relationship.Edge{
			complexityEdge("a", "b"), complexityEdge("b", "a"), complexityEdge("b", "c"),
		}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := moduleGraphComplexity(tc.modules, relationship.Set{Edges: tc.edges})
			if got == nil || got.MaxDependencyChain != tc.want {
				t.Fatalf("module graph complexity = %+v, want depth %d", got, tc.want)
			}
		})
	}
}

func TestComplexityFunctionLengthDistribution(t *testing.T) {
	facts := []modevidence.SyntaxFact{
		{Kind: syntaxKindFunction, StartLine: 1, EndLine: 10},
		{Kind: syntaxKindMethod, StartLine: 11, EndLine: 30},
		{Kind: syntaxKindFunction, StartLine: 31, EndLine: 60},
		{Kind: syntaxKindFunction, StartLine: 61, EndLine: 160},
		{Kind: syntaxKindFunction, StartLine: 200, EndLine: 0},
		{Kind: "class", StartLine: 1, EndLine: 500},
	}
	got := functionLengthDistribution(facts, policy.DefaultFunctionLOCThreshold)
	want := functionLengthStats{Total: 5, Observed: 4, P50: 20, P90: 100, Max: 100, OverThreshold: 1}
	if got != want {
		t.Fatalf("function length distribution = %+v, want %+v", got, want)
	}
	if custom := functionLengthDistribution(facts, 25); custom.OverThreshold != 2 {
		t.Fatalf("functions over custom threshold = %d, want 2", custom.OverThreshold)
	}
}

func TestComplexityDimensionPromotionAndDiagnostics(t *testing.T) {
	diag, in := completeComplexityFixture()
	dim := complexityDimension(diag, in.Policy, in.Facts)
	if dim.Status != state.Measured {
		t.Fatalf("complexity status = %q, want measured; unknown=%+v", dim.Status, dim.Unknown)
	}
	for name, want := range map[string]float64{
		"max_dependency_chain": 1, "module_fan_in_p90": 1, "module_fan_out_p90": 1,
		"function_loc_p50": 10, "function_loc_p90": 100, "function_loc_max": 100, "functions_over_threshold": 1,
	} {
		if got, ok := dimensionMetric(dim.Metrics, name); !ok || got.Value != want {
			t.Errorf("metric %q = %+v (found=%t), want %v", name, got, ok, want)
		} else if got.Denominator == nil || len(got.Provenance) == 0 {
			t.Errorf("metric %q lacks denominator or provenance: %+v", name, got)
		}
	}

	t.Run("syntax diagnostics do not promote", func(t *testing.T) {
		withoutSyntax := in
		withoutSyntax.Facts.SyntaxFacts = nil
		got := complexityDimension(diag, withoutSyntax.Policy, withoutSyntax.Facts)
		if got.Status != state.Measured {
			t.Fatalf("status without syntax = %q, want measured", got.Status)
		}
		if _, found := dimensionMetric(got.Metrics, "function_loc_p50"); found {
			t.Error("missing syntax facts were published as a zero function distribution")
		}
		if !complexityUnknown(got, state.FactFunctionLengthDistribution) {
			t.Errorf("unknowns = %+v, want out-of-claim function distribution", got.Unknown)
		}
	})

	t.Run("an incomplete module graph blocks promotion", func(t *testing.T) {
		incomplete := *diag
		incomplete.ToolCoverage = []modevidence.Coverage{{Tool: toolGoPackages, Status: modevidence.StatusPartial}}
		got := complexityDimension(&incomplete, in.Policy, in.Facts)
		if got.Status != state.Partial {
			t.Fatalf("status = %q, want partial; unknown=%+v", got.Status, got.Unknown)
		}
		if !complexityUnknown(got, state.FactDependencyChainDepth) || !complexityUnknown(got, state.FactModuleFanInDistribution) || !complexityUnknown(got, state.FactModuleFanOutDistribution) {
			t.Errorf("unknowns = %+v, want all incomplete module-graph distributions", got.Unknown)
		}
		if _, found := dimensionMetric(got.Metrics, "max_dependency_chain"); found {
			t.Error("partial graph published a complete dependency depth")
		}
	})

	first := complexityDimension(diag, in.Policy, in.Facts)
	second := complexityDimension(diag, in.Policy, in.Facts)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("identical complexity runs differ:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestComplexityDimensionRequiresApplicableDependencyProducer(t *testing.T) {
	diag, in := completeComplexityFixture()
	diag.ModuleGraphComplexity = &result.ModuleGraphComplexity{Modules: 2}
	diag.ClassifiedEdges = &result.ClassifiedEdgeSummary{}

	diag.ToolCoverage = []modevidence.Coverage{{Tool: toolGoPackages, Status: modevidence.StatusAbsent}}
	withoutProducer := complexityDimension(diag, in.Policy, in.Facts)
	if withoutProducer.Status != state.Partial {
		t.Fatalf("complexity without an applicable producer = %q, want partial; unknown=%+v", withoutProducer.Status, withoutProducer.Unknown)
	}
	if _, found := dimensionMetric(withoutProducer.Metrics, "max_dependency_chain"); found {
		t.Fatal("complexity without an applicable producer published graph metrics")
	}

	diag.ToolCoverage = []modevidence.Coverage{{Tool: toolGoPackages, Status: modevidence.StatusOK}}
	withProducer := complexityDimension(diag, in.Policy, in.Facts)
	if withProducer.Status != state.Measured {
		t.Fatalf("complexity with a completed empty graph = %q, want measured; unknown=%+v", withProducer.Status, withProducer.Unknown)
	}
}

func completeComplexityFixture() (*result.Result, stateInput) {
	modules := complexityModules("a", "b")
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	p := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{FunctionLOCThreshold: 60}, policy.GatePolicy{}, nil, nil)
	// Bound to locals: gofmt before Go 1.27 indents multi-line composite
	// literals in a multi-value return differently, so the inline form cannot
	// satisfy both toolchains at once.
	res := &result.Result{
		ModuleGraphComplexity: &result.ModuleGraphComplexity{
			Modules: 2, MaxDependencyChain: 1, FanInP90: 1, FanOutP90: 1,
		},
		PrimaryExtractorTools: []string{toolGoPackages},
		ToolCoverage:          []modevidence.Coverage{{Tool: toolGoPackages, Status: modevidence.StatusOK}},
		ClassifiedEdges: &result.ClassifiedEdgeSummary{
			InternalDependencies: 1, ClassifiedInternalDependencies: 1, FirstPartyNodes: 2, AttributedFirstPartyNodes: 2,
		},
	}
	in := stateInput{Policy: p, Facts: Observations{
		FileLOC:        map[string]int{"a/a.go": 10, "b/b.go": 100},
		FileClassIndex: map[string]fileclass.FileClass{"a/a.go": fileclass.Production, "b/b.go": fileclass.Production},
		SyntaxFacts: []modevidence.SyntaxFact{
			{Kind: syntaxKindFunction, StartLine: 1, EndLine: 10},
			{Kind: syntaxKindMethod, StartLine: 1, EndLine: 100},
		},
	}}

	return res, in
}

func complexityModules(names ...string) map[string]policy.ModuleDef {
	modules := make(map[string]policy.ModuleDef, len(names))
	for _, name := range names {
		modules[name] = policy.ModuleDef{Paths: []string{name + "/**"}}
	}
	return modules
}

func complexityEdge(from, to string) relationship.Edge {
	return relationship.Edge{FromModule: from, ToModule: to, Kind: "imports"}
}

func dimensionMetric(metrics []state.MetricValue, name string) (state.MetricValue, bool) {
	for _, metric := range metrics {
		if metric.Name == name {
			return metric, true
		}
	}
	return state.MetricValue{}, false
}

func complexityUnknown(dim state.Dimension, fact string) bool {
	for _, unknown := range dim.Unknown {
		if unknown.Fact == fact {
			return true
		}
	}
	return false
}
