package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
)

// This file hosts the shared clone-pair test fixtures (fileFromA / fileToB —
// the canonical cross-module test node IDs — plus twoModuleConfig and
// modABClonePair) and the tests that first used them. Other classify_test
// files reuse these package-level constants directly (same package).
const (
	fileFromA      = "file:services/a/a.go"
	fileToB        = "file:services/b/b.go"
	hintFunctional = "functional"
	hintIntrusive  = "intrusive"
	modNameA       = "modA"
	modNameB       = "modB"
)

// twoModuleConfig returns a ClassifyConfig with two modules (modA: services/a/**,
// modB: services/b/**) and no public/internal globs unless provided.
func twoModuleConfig(publicGlobs []string, clonePairs map[string]struct{}) classify.Config {
	return classify.Config{
		Modules: map[string]policy.ModuleDef{
			modNameA: {Paths: []string{pathsA}},
			modNameB: {Paths: []string{pathsB}, Public: publicGlobs},
		},
		CrossModuleClonePairs: clonePairs,
	}
}

// modABClonePair is the canonical clone-pair key set for the modA/modB test pair.
// "modA" < "modB" lexicographically so the key is always modA\x00modB.
var modABClonePair = map[string]struct{}{modABKey: {}}

// A clone pair never changes an import edge. It is its own symmetric clone fact
// (ClonePairs), so the edge keeps the strength its own evidence gave it.
func TestClonePairLeavesEdgeStrengthAlone(t *testing.T) {
	t.Parallel()

	edge := graph.Edge{From: fileFromA, To: fileToB, Kind: graph.EdgeKindImports, StrengthHint: hintFunctional}
	g := makeGraph([]graph.Edge{edge})
	key := edgeKey(edge)

	clWith := classify.Run(g, twoModuleConfig(nil, modABClonePair))[key]
	clWithout := classify.Run(g, twoModuleConfig(nil, nil))[key]

	if clWith.Strength != coupling.StrengthFunctional || clWithout.Strength != coupling.StrengthFunctional {
		t.Errorf("strengths = %q with clone, %q without, want functional both", clWith.Strength, clWithout.Strength)
	}
	if clWith.Score.Value != clWithout.Score.Value {
		t.Errorf("Score.Value = %d with clone, %d without, want equal", clWith.Score.Value, clWithout.Score.Value)
	}
	if len(clWith.CloneLocations) != 0 {
		t.Errorf("edge CloneLocations = %v, want none: the locations belong to the clone fact", clWith.CloneLocations)
	}
}

// A flat-named edge across a bare module boundary scores at D=9: functional
// coupling into a core module is critical whatever the key spelling.
func TestFlatNameEdgeScoresAtTheBoundaryRung(t *testing.T) {
	t.Parallel()

	cfg := classify.Config{Modules: map[string]policy.ModuleDef{
		"core": {Paths: []string{"core/**"}, Subdomain: subdomainCore},
		"api":  {Paths: []string{"api/**"}},
	}}
	edge := graph.Edge{From: "file:core/x.go", To: "file:api/y.go", Kind: graph.EdgeKindImports, StrengthHint: hintFunctional}
	cl := classify.Run(makeGraph([]graph.Edge{edge}), cfg)[edgeKey(edge)]

	if cl.Distance != coupling.DistanceCrossModule {
		t.Errorf("Distance = %q, want cross_module", cl.Distance)
	}
	// V = worse(core high, api undeclared) = high: S=8, D=9, V=10 → 2.
	if cl.Score.Balance != 2 || cl.Score.Band != coupling.SeverityCritical {
		t.Errorf("score = %d %q, want 2 critical", cl.Score.Balance, cl.Score.Band)
	}
}
