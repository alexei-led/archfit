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
	s := got.Assessment.ClassifiedEdges
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
	s := got.Assessment.ClassifiedEdges
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
	s := got.Assessment.ClassifiedEdges
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
	if s := got.Assessment.ClassifiedEdges; s.LabeledLLM != 0 || s.LLMLowConfidenceEdges != 0 {
		t.Errorf("llm counters = %d/%d, want zero without approved llm labels", s.LabeledLLM, s.LLMLowConfidenceEdges)
	}
}

// TestDistanceCompressionCountsWellBalancedEdges pins the structural-span
// evidence contract: code_structure_boundary_counts / _ancestor_depths describe
// the SHAPE of the module tree the scored edges cross, not the risk they carry.
// Filtering them by severity band erases every well-balanced edge — and a
// balance of 9 or 10 lands in the "none" band, which is also the zero value —
// so a healthy repo silently reported no structural spans at all.
func TestDistanceCompressionCountsWellBalancedEdges(t *testing.T) {
	// One owner, one deploy unit, frozen volatility: ownership is degenerate, so
	// distance stays on the code_structure basis, and 10-V dominates the balance
	// into the "none" band.
	modules := map[string]policy.ModuleDef{
		moduleA: {Paths: []string{globA}, Owner: teamA, DeployUnit: teamA, Volatility: string(relationship.VolatilityFrozen)},
		moduleB: {Paths: []string{globB}, Owner: teamA, DeployUnit: teamA, Volatility: string(relationship.VolatilityFrozen)},
	}
	got := analysis.Analyze(analysis.Input{
		Graph:  graphWith(string(relationship.StrengthContract)),
		Policy: relationshipPolicy(modules),
	})
	s := got.Assessment.ClassifiedEdges
	if s == nil || s.Scored != 1 {
		t.Fatalf("scored = %+v, want the single cross-boundary edge scored", s)
	}
	edge := onlyEdge(t, got)
	if edge.Classified.DistanceBasis != "code_structure" {
		t.Fatalf("distance basis = %q, want code_structure: the fixture must exercise the structural ladder", edge.Classified.DistanceBasis)
	}
	if edge.Classified.Score.Band != relationship.SeverityNone {
		t.Fatalf("band = %q, want the none band: the fixture must exercise a well-balanced edge", edge.Classified.Score.Band)
	}
	if s.DistanceCompression == nil {
		t.Fatal("DistanceCompression = nil, want the structural-span evidence")
	}
	if len(s.DistanceCompression.CodeStructureBoundaryCounts) == 0 {
		t.Error("CodeStructureBoundaryCounts is empty: a well-balanced code_structure edge still crosses module boundaries")
	}
	if len(s.DistanceCompression.CodeStructureAncestorDepths) == 0 {
		t.Error("CodeStructureAncestorDepths is empty: a well-balanced code_structure edge still has a shared-ancestor depth")
	}
}

// TestDriverHistogramsCountCrossBoundaryEdgesOnly pins the driver denominators.
// Every histogram in the coupling_balance evidence line is read against Scored,
// which counts cross-boundary edges only. Counting the same-module rung here
// made by_balance_driver sum to Scored+SameModule and printed a critical-driver
// total larger than the critical-band count on the very same line.
func TestDriverHistogramsCountCrossBoundaryEdgesOnly(t *testing.T) {
	got := analysis.Analyze(analysis.Input{Graph: mixedGraph(), Policy: relationshipPolicy(twoModules())})
	s := got.Assessment.ClassifiedEdges
	if s == nil {
		t.Fatal("ClassifiedEdges = nil, want the summary")
	}
	if s.SameModule != 1 || s.Scored != 1 {
		t.Fatalf("same-module/scored = %d/%d, want 1/1", s.SameModule, s.Scored)
	}
	if sum := sumCounts(s.ByBalanceDriver); sum != s.Scored {
		t.Errorf("by_balance_driver sums to %d over %d scored", sum, s.Scored)
	}
	if sum := sumCounts(s.ByModulePair); sum != s.Scored {
		t.Errorf("by_module_pair sums to %d over %d scored", sum, s.Scored)
	}
	if sum := sumCounts(s.ByCriticalDriver); sum > s.BySeverity[string(relationship.SeverityCritical)] {
		t.Errorf("by_critical_driver sums to %d over %d critical-band edges",
			sum, s.BySeverity[string(relationship.SeverityCritical)])
	}
	if sum := sumCounts(s.ByBalanceDriver); sum != sumCounts(s.ByStrength) {
		t.Errorf("by_balance_driver (%d) and by_strength (%d) report different denominators",
			sum, sumCounts(s.ByStrength))
	}
}

func sumCounts(m map[string]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}

// TestStaticExternalCandidatesDedupEvidenceSites pins that one candidate group
// never repeats an evidence site. Two edges in one group can carry byte-equal
// sites: the Rust extractor stamps every crate dependency with the package-level
// "Cargo.toml", so a workspace whose members map to one configured module emits
// the same site once per member. Count reports how many edges were seen;
// evidence_sites is meant to be the distinct places a reviewer can look.
func TestStaticExternalCandidatesDedupEvidenceSites(t *testing.T) {
	const manifest = "a/Cargo.toml"
	site := graph.Location{File: manifest, Line: 1}
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangRust},
			{Kind: graph.NodeKindFile, Path: fileA2, Language: graph.LangRust},
		},
		// Both edges leave module a for the same undeclared target and report
		// the identical manifest location.
		Edges: []graph.Edge{
			{From: nodeA, To: "external:serde", Kind: graph.EdgeKindImports, Language: graph.LangRust, Locations: []graph.Location{site}},
			{From: "file:" + fileA2, To: "external:serde", Kind: graph.EdgeKindImports, Language: graph.LangRust, Locations: []graph.Location{site}},
		},
	}})
	got := analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(twoModules())})
	var found bool
	for _, c := range got.Evidence.DistanceConfigCandidates {
		if c.Target == "" || c.Module != moduleA {
			continue
		}
		found = true
		if c.Count != 2 {
			t.Errorf("candidate %q count = %d, want 2 (both edges counted)", c.Target, c.Count)
		}
		if len(c.EvidenceSites) != 1 {
			t.Errorf("candidate %q evidence_sites = %d, want 1 distinct site; got %+v",
				c.Target, len(c.EvidenceSites), c.EvidenceSites)
		}
	}
	if !found {
		t.Fatal("no external distance candidate for module a; fixture no longer exercises the dedup")
	}
}
