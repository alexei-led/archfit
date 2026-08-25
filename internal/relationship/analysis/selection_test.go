package analysis_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

const (
	modA = "a"
	modB = "b"
	modC = "c"

	pathA1 = "pkg/a/a.go"
	pathA2 = "pkg/a/a2.go"
	pathB1 = "pkg/b/b.go"
	pathB2 = "pkg/b/b2.go"
	pathC1 = "pkg/c/c.go"
	pathC2 = "pkg/c/c2.go"
	pathX  = "vendor/x/x.go"
)

// selEdge declares one fixture relationship in module terms.
type selEdge struct {
	fromPath, toPath string
	fromMod, toMod   string
	strength         relationship.Strength
	distance         relationship.Distance
	file             string
	line             int
}

func selSet(edges ...selEdge) relationship.Set {
	set := relationship.Set{Edges: make([]relationship.Edge, 0, len(edges))}
	for _, e := range edges {
		edge := relationship.Edge{
			FromID: "file:" + e.fromPath, ToID: "file:" + e.toPath,
			FromPath: e.fromPath, ToPath: e.toPath,
			FromModule: e.fromMod, ToModule: e.toMod,
			Kind: kindImports, Strength: e.strength, Distance: e.distance,
		}
		if e.file != "" {
			edge.Locations = []relationship.Location{{File: e.file, Line: e.line}}
		}
		set.Edges = append(set.Edges, edge)
	}
	return set
}

const crossOwner = relationship.DistanceCrossModuleDiffOwner

// refinableSet has one refinable a→b pair (two functional edges) plus edges
// that must be excluded: a contract-strength a→c edge, and a c→b edge the
// caller can mark approved.
func refinableSet() relationship.Set {
	return selSet(
		selEdge{pathA1, pathB1, modA, modB, relationship.StrengthFunctional, crossOwner, "", 0},
		selEdge{pathA2, pathB2, modA, modB, relationship.StrengthFunctional, crossOwner, "", 0},
		selEdge{pathA1, pathC1, modA, modC, relationship.StrengthContract, crossOwner, "", 0},
		selEdge{pathC1, pathB1, modC, modB, relationship.StrengthFunctional, crossOwner, "", 0},
	)
}

// abstainedSet has one abstained a→b pair (two unknown-strength edges, the
// first carrying a location) plus edges that must be excluded: known strength,
// same-module distance, external (unknown) distance, and an approvable pair.
func abstainedSet() relationship.Set {
	return selSet(
		selEdge{pathA1, pathB1, modA, modB, relationship.StrengthUnknown, crossOwner, pathA1, 3},
		selEdge{pathA2, pathB2, modA, modB, relationship.StrengthUnknown, crossOwner, "", 0},
		selEdge{pathA1, pathC1, modA, modC, relationship.StrengthFunctional, crossOwner, "", 0},
		selEdge{pathC1, pathB1, modC, modB, relationship.StrengthUnknown, crossOwner, "", 0},
		selEdge{pathC1, pathC2, modC, modC, relationship.StrengthUnknown, relationship.DistanceSameModule, "", 0},
		selEdge{pathC2, pathX, modC, "", relationship.StrengthUnknown, relationship.DistanceUnknown, "", 0},
	)
}

func approve(pairs ...[2]string) map[string]struct{} {
	out := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		out[labels.Key(p[0], p[1])] = struct{}{}
	}
	return out
}

// TestRefinablePairs pins which edges reach semantic review: heuristic-strength
// cross-module pairs only, never a config-authoritative contract edge and never
// an already-approved pair.
func TestRefinablePairs(t *testing.T) {
	t.Parallel()
	pairs := analysis.RefinablePairs(refinableSet(), approve([2]string{modC, modB}))
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want exactly a->b", pairs)
	}
	p := pairs[0]
	if p.From != modA || p.To != modB {
		t.Errorf("pair = %s->%s, want a->b", p.From, p.To)
	}
	if p.EdgeCount != 2 {
		t.Errorf("edge count = %d, want 2", p.EdgeCount)
	}
	if p.Strength != string(relationship.StrengthFunctional) {
		t.Errorf("strength = %q, want functional", p.Strength)
	}
	if len(p.SamplePaths) != 2 || !strings.Contains(p.SamplePaths[0], "pkg/a/") {
		t.Errorf("samples = %v", p.SamplePaths)
	}
}

// TestRefinablePairs_ExcludedShapes is the negative table: each row is a single
// edge that must never be offered for review.
func TestRefinablePairs_ExcludedShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edge selEdge
	}{
		{"contract strength is config-authoritative", selEdge{pathA1, pathB1, modA, modB, relationship.StrengthContract, crossOwner, "", 0}},
		{"intrusive strength is config-authoritative", selEdge{pathA1, pathB1, modA, modB, relationship.StrengthIntrusive, crossOwner, "", 0}},
		{"same-module edges are not a boundary", selEdge{pathA1, pathA2, modA, modA, relationship.StrengthFunctional, relationship.DistanceSameModule, "", 0}},
		{"unknown distance means missing facts", selEdge{pathA1, pathX, modA, "", relationship.StrengthFunctional, relationship.DistanceUnknown, "", 0}},
		{"unresolved target module", selEdge{pathA1, pathX, modA, "", relationship.StrengthUnknown, crossOwner, "", 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := analysis.RefinablePairs(selSet(test.edge), nil); len(got) != 0 {
				t.Errorf("pairs = %+v, want none", got)
			}
		})
	}
}

// TestAbstainedPairs pins abstained selection: only unknown-strength
// cross-module edges, grouped per ordered pair, carrying the first location.
func TestAbstainedPairs(t *testing.T) {
	t.Parallel()
	pairs, total := analysis.AbstainedPairs(abstainedSet(), approve([2]string{modC, modB}), 100, 5)
	if total != 2 {
		t.Fatalf("total = %d, want 2 abstained edges", total)
	}
	if len(pairs) != 1 || pairs[0].From != modA || pairs[0].To != modB {
		t.Fatalf("pairs = %+v, want exactly a->b", pairs)
	}
	if pairs[0].EdgeCount != 2 {
		t.Errorf("edge count = %d, want 2", pairs[0].EdgeCount)
	}
	if len(pairs[0].Samples) != 2 {
		t.Fatalf("samples = %+v, want one per edge", pairs[0].Samples)
	}
	if pairs[0].Samples[0].File != pathA1 || pairs[0].Samples[0].Line != 3 {
		t.Errorf("first sample = %+v, want the edge location", pairs[0].Samples[0])
	}
}

// TestAbstainedPairs_CapsAreDisclosedNotHidden pins that edgeCap bounds the
// returned samples while total keeps reporting every abstained edge, so a
// truncated prompt never reads as "there was nothing else".
func TestAbstainedPairs_CapsAreDisclosedNotHidden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		edgeCap, sampleCap int
		wantEdgeCount      int
		wantSamples        int
	}{
		{name: "uncapped", edgeCap: 100, sampleCap: 5, wantEdgeCount: 2, wantSamples: 2},
		{name: "edge cap truncates the group", edgeCap: 1, sampleCap: 5, wantEdgeCount: 1, wantSamples: 1},
		{name: "sample cap truncates the locations", edgeCap: 100, sampleCap: 1, wantEdgeCount: 2, wantSamples: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pairs, total := analysis.AbstainedPairs(abstainedSet(), approve([2]string{modC, modB}), test.edgeCap, test.sampleCap)
			if total != 2 {
				t.Errorf("total = %d, want every abstained edge counted regardless of caps", total)
			}
			if len(pairs) != 1 {
				t.Fatalf("pairs = %+v, want a->b", pairs)
			}
			if pairs[0].EdgeCount != test.wantEdgeCount {
				t.Errorf("edge count = %d, want %d", pairs[0].EdgeCount, test.wantEdgeCount)
			}
			if len(pairs[0].Samples) != test.wantSamples {
				t.Errorf("samples = %d, want %d", len(pairs[0].Samples), test.wantSamples)
			}
		})
	}
}

// TestPairEvidence pins the label evidence hash: it covers only wanted pairs,
// and it changes when the dependency surface of a pair changes — which is what
// makes an approved label go stale.
func TestPairEvidence(t *testing.T) {
	t.Parallel()
	wanted := map[string]struct{}{labels.Key(modA, modB): {}}
	base := analysis.PairEvidence(refinableSet(), wanted)
	if len(base) != 1 {
		t.Fatalf("evidence = %v, want only the wanted pair", base)
	}
	if got := analysis.PairEvidence(refinableSet(), nil); got != nil {
		t.Errorf("evidence for no wanted pairs = %v, want nil", got)
	}
	changed := analysis.PairEvidence(selSet(
		selEdge{pathA1, pathB1, modA, modB, relationship.StrengthFunctional, crossOwner, "", 0},
	), wanted)
	if changed[labels.Key(modA, modB)] == base[labels.Key(modA, modB)] {
		t.Error("dropping an edge from the pair must change its evidence hash")
	}
}

// TestAugmentConfig_RegistersSyntheticRustModules pins that auto-discovered
// "<crate>::<mod>" nodes become modules: without them every intra-crate edge
// classifies as distance-unknown and never reaches review.
func TestAugmentConfig_RegistersSyntheticRustModules(t *testing.T) {
	t.Parallel()
	const from, to = "krate::alpha", "krate::beta"
	fromNode := graph.Node{Kind: graph.NodeKindModule, Path: from, Language: graph.LangRust}
	toNode := graph.Node{Kind: graph.NodeKindModule, Path: to, Language: graph.LangRust}
	g := graph.Build([]graph.Facts{{
		Language: graph.LangRust,
		Nodes:    []graph.Node{fromNode, toNode},
		Edges:    []graph.Edge{{From: fromNode.ID(), To: toNode.ID(), Kind: graph.EdgeKindImports, Language: graph.LangRust}},
	}})

	bare := policy.BuildModuleMap(nil)
	if _, ok := bare.ModuleFor(from); ok {
		t.Fatalf("an unaugmented module map must not resolve %q", from)
	}
	augmented := analysis.AugmentConfig(g, classify.ConfigFrom(policy.RelationshipPolicy{})).ModuleMap
	for _, path := range []string{from, to} {
		if _, ok := augmented.ModuleFor(path); !ok {
			t.Errorf("augmented module map does not resolve %q", path)
		}
	}
}
