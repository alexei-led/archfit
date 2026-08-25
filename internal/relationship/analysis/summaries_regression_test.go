// Regression tests for the classified-edge rollups. Each one pins a counter
// that a consumer reads directly: tail risk feeds the coupling_balance evidence
// line, and the LLM counters feed its confidence band.
package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

const fileA2 = "a/other.go"

// mixedGraph carries one cross-module a→b intrusive edge (book balance 4, the
// "high" band) and one same-module contract edge (balance 2, "critical"). The
// same-module edge is deliberately the WORSE of the two: if it leaks into the
// tail, the high-or-worse count exceeds the scored denominator.
func mixedGraph() *graph.Graph {
	return graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileA2, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileB, Language: graph.LangGo},
		},
		Edges: []graph.Edge{
			{From: nodeA, To: nodeB, Kind: graph.EdgeKindImports, Language: graph.LangGo,
				StrengthHint: string(relationship.StrengthIntrusive)},
			{From: nodeA, To: "file:" + fileA2, Kind: graph.EdgeKindImports, Language: graph.LangGo,
				StrengthHint: string(relationship.StrengthContract)},
		},
	}})
}

// TestTailRiskCountsCrossBoundaryEdgesOnly pins the tail-risk denominator
// contract: the share is reported against Scored, which counts cross-boundary
// edges only, so a same-module edge must not reach the numerator. Admitting it
// produced shares above 100%.
func TestTailRiskCountsCrossBoundaryEdgesOnly(t *testing.T) {
	got := analysis.Analyze(analysis.Input{Graph: mixedGraph(), Policy: relationshipPolicy(twoModules())})
	s := got.Evidence.ClassifiedEdges
	if s == nil || s.TailRisk == nil {
		t.Fatal("TailRisk = nil, want the tail summary")
	}
	if s.Total != 2 || s.SameModule != 1 || s.Scored != 1 {
		t.Fatalf("summary total/same-module/scored = %d/%d/%d, want 2/1/1", s.Total, s.SameModule, s.Scored)
	}
	if s.TailRisk.HighOrWorseEdges > s.Scored {
		t.Errorf("high-or-worse edges = %d over %d scored: the same-module edge leaked into the tail",
			s.TailRisk.HighOrWorseEdges, s.Scored)
	}
	if s.TailRisk.HighOrWorseSharePct > 100 {
		t.Errorf("high-or-worse share = %d%%, want a share the denominator can carry", s.TailRisk.HighOrWorseSharePct)
	}
}

// TestTailRiskKeepsWellBalancedEdges pins that a band-"none" cross-boundary edge
// (balance 9-10) still enters the balance distribution: it is what makes
// WorstBalance and LowerDecileBalance honest, and dropping it silently shifted
// both.
func TestTailRiskKeepsWellBalancedEdges(t *testing.T) {
	// Book ordinals: contract S=1 over a cross-deploy-unit boundary D=9 gives
	// |S-D| = 8, and low volatility gives 10-V = 7, so balance = 9 — the "none"
	// band, and the most balanced corner of the scale.
	modules := map[string]policy.ModuleDef{
		moduleA: {Paths: []string{globA}, Owner: teamA, DeployUnit: moduleA, Volatility: volLow},
		moduleB: {Paths: []string{globB}, Owner: teamB, DeployUnit: moduleB, Volatility: volLow},
	}
	got := analysis.Analyze(analysis.Input{
		Graph:  graphWith(string(relationship.StrengthContract)),
		Policy: relationshipPolicy(modules),
	})
	s := got.Evidence.ClassifiedEdges
	if s == nil || s.Scored != 1 {
		t.Fatalf("scored = %+v, want the single cross-boundary edge scored", s)
	}
	if s.BySeverity[string(relationship.SeverityNone)] != 1 {
		t.Fatalf("severity buckets = %v, want the edge in the none band", s.BySeverity)
	}
	if s.TailRisk == nil {
		t.Fatal("TailRisk = nil, want a well-balanced edge to still anchor the distribution")
	}
	if s.TailRisk.WorstBalance == 0 {
		t.Error("WorstBalance = 0, want the band-none edge counted in the balance distribution")
	}
}

// TestClassifiedSummaryCountsLLMLabelProvenance pins the two counters the
// coupling_balance confidence rule reads. LLMLowConfidenceEdges losing its
// producer silently disabled the documented "llm labels below high confidence
// lower the band" behavior.
func TestClassifiedSummaryCountsLLMLabelProvenance(t *testing.T) {
	// An llm-provenance label only fills a cell every static source left
	// unknown, so the fixture edge carries no extractor strength hint.
	lbls := []labels.Label{{
		From: moduleA, To: moduleB, Strength: string(relationship.StrengthIntrusive),
		Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceMedium,
	}}
	got := analysis.Analyze(analysis.Input{
		Graph:  graphWith(""),
		Policy: relationshipPolicy(twoModules()),
		Labels: lbls,
	})
	edge := onlyEdge(t, got)
	if edge.Strength != relationship.StrengthIntrusive {
		t.Fatalf("strength = %q, want the approved label applied", edge.Strength)
	}
	s := got.Evidence.ClassifiedEdges
	if s.LabeledLLM != 1 {
		t.Errorf("LabeledLLM = %d, want the llm-labeled edge counted", s.LabeledLLM)
	}
	if s.LLMLowConfidenceEdges != 1 {
		t.Errorf("LLMLowConfidenceEdges = %d, want the below-high-confidence label counted", s.LLMLowConfidenceEdges)
	}
}

// A same-module edge carries no cross-boundary coupling, so its label
// provenance must not reach the counters either.
func TestClassifiedSummarySkipsLLMProvenanceOnSameModuleEdges(t *testing.T) {
	got := analysis.Analyze(analysis.Input{Graph: mixedGraph(), Policy: relationshipPolicy(twoModules())})
	if s := got.Evidence.ClassifiedEdges; s.LabeledLLM != 0 || s.LLMLowConfidenceEdges != 0 {
		t.Errorf("llm counters = %d/%d, want zero without approved llm labels", s.LabeledLLM, s.LLMLowConfidenceEdges)
	}
}
