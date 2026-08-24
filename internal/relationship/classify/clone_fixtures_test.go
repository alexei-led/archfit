package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/view"
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
func twoModuleConfig(publicGlobs []string, clonePairs map[string]struct{}) view.ClassifyConfig {
	return view.ClassifyConfig{
		Modules: map[string]module.ModuleDef{
			modNameA: {Paths: []string{pathsA}},
			modNameB: {Paths: []string{pathsB}, Public: publicGlobs},
		},
		CrossModuleClonePairs: clonePairs,
	}
}

// modABClonePair is the canonical clone-pair key set for the modA/modB test pair.
// "modA" < "modB" lexicographically so the key is always modA\x00modB.
var modABClonePair = map[string]struct{}{modABKey: {}}

// TestClonePairUpgradesStrengthToSymmetric verifies that a cross-module clone pair
// upgrades strength from functional (or unknown) to Symmetric (book ordinal 9).
// This changes severity and score — clone pairs are NOT purely descriptive.
func TestClonePairUpgradesStrengthToSymmetric(t *testing.T) {
	t.Parallel()

	// Two identical edges differing only in whether a clone pair is present.
	// StrengthHint=functional → without clone: Functional; with clone: Symmetric.
	edge := graph.Edge{
		From:         fileFromA,
		To:           fileToB,
		Kind:         graph.EdgeKindImports,
		StrengthHint: hintFunctional,
	}

	cfgWithClone := twoModuleConfig(nil, modABClonePair)
	cfgWithout := twoModuleConfig(nil, nil)

	g := makeGraph([]graph.Edge{edge})
	key := edgeKey(edge)

	clWith := classify.Run(g, cfgWithClone)[key]
	clWithout := classify.Run(g, cfgWithout)[key]

	// With clone pair: strength upgraded to Symmetric.
	if clWith.Strength != coupling.StrengthSymmetric {
		t.Errorf("with clone: Strength = %q, want %q", clWith.Strength, coupling.StrengthSymmetric)
	}

	// Without clone pair: strength stays at functional (from hint).
	if clWithout.Strength != coupling.StrengthFunctional {
		t.Errorf("without clone: Strength = %q, want %q", clWithout.Strength, coupling.StrengthFunctional)
	}

	// Symmetric (ordinal 9) > Functional (ordinal 8) → scores must differ.
	if clWith.Score.Value == clWithout.Score.Value {
		t.Errorf("Score.Value identical (%d); Symmetric and Functional should produce different scores", clWith.Score.Value)
	}
}

// TestFlatNameEdgeScoresSameOwnerAfterP1Fix verifies that a flat-named
// (single-segment) edge in a degenerate-owner repo classifies as
// cross_module_same_owner (not diff_owner) and scores Medium under the book
// formula — the P1 fix for false tight-coupling on single-team flat-named repos.
//
// Case: StrengthSymmetric (S=9, via clone pair), Distance=SameOwner (D=4),
// Volatility=high (V=10, via subdomain:core).
// balance = max(|S-D|, 10-V)+1 = max(|9-4|, 10-10)+1 = max(5,0)+1 = 6.
// ScoreBand(6) = Medium.
//
// Pre-fix: flat names → DiffOwner (D=7) → balance = max(|9-7|, 0)+1 = 3 → High.
func TestFlatNameEdgeScoresSameOwnerAfterP1Fix(t *testing.T) {
	t.Parallel()

	const (
		flatModA = "core"
		flatModB = "api"
	)
	flatFileA := "file:" + flatModA + "/x.go"
	flatFileB := "file:" + flatModB + "/y.go"
	// "api" < "core" lexicographically → sorted key is "api\x00core".
	var clonePairKey string
	if flatModB < flatModA {
		clonePairKey = flatModB + "\x00" + flatModA
	} else {
		clonePairKey = flatModA + "\x00" + flatModB
	}

	cfg := view.ClassifyConfig{
		Modules: map[string]module.ModuleDef{
			flatModA: {
				Paths:     []string{flatModA + "/**"},
				Subdomain: "core", // → high volatility
			},
			flatModB: {
				Paths: []string{flatModB + "/**"},
			},
		},
		CrossModuleClonePairs: map[string]struct{}{clonePairKey: {}},
	}

	edge := graph.Edge{
		From:         flatFileA,
		To:           flatFileB,
		Kind:         graph.EdgeKindImports,
		StrengthHint: hintFunctional, // upgraded to Symmetric by clone pair
	}

	g := makeGraph([]graph.Edge{edge})
	idx := classify.Run(g, cfg)
	key := edgeKey(edge)
	cl, ok := idx[key]
	if !ok {
		t.Fatal("edge not in classification index")
	}

	// After P1 fix: flat names in degenerate-owner repo → SameOwner.
	if cl.Distance != coupling.DistanceCrossModuleSameOwner {
		t.Errorf("Distance = %q, want cross_module_same_owner (P1 fix)", cl.Distance)
	}
	// Clone pair upgrades strength to Symmetric.
	if cl.Strength != coupling.StrengthSymmetric {
		t.Errorf("Strength = %q, want symmetric (clone pair upgrade)", cl.Strength)
	}
	// balance = max(|9-4|, 10-10)+1 = 6 → Medium.
	const wantBalance = 6
	if cl.Score.Balance != wantBalance {
		t.Errorf("Score.Balance = %d, want %d (S=9, D=4, V=10)", cl.Score.Balance, wantBalance)
	}
	if cl.Score.Band != coupling.SeverityMedium {
		t.Errorf("Score.Band = %q, want medium", cl.Score.Band)
	}
}
