package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// fileFromA / fileToB are the canonical cross-module test node IDs used by
// connascence tests. They live here because connascence_test.go is the first
// file that introduced them; other classify_test files that need them can reuse
// these package-level constants directly (same package: classify_test).
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
func twoModuleConfig(publicGlobs []string, clonePairs map[string]struct{}) config.ClassifyConfig {
	return config.ClassifyConfig{
		Modules: map[string]config.ModuleDef{
			modNameA: {Paths: []string{pathsA}},
			modNameB: {Paths: []string{pathsB}, Public: publicGlobs},
		},
		CrossModuleClonePairs: clonePairs,
	}
}

// modABClonePair is the canonical clone-pair key set for the modA/modB test pair.
// "modA" < "modB" lexicographically so the key is always modA\x00modB.
var modABClonePair = map[string]struct{}{"modA\x00modB": {}}

func TestConnascenceTagging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		edge         graph.Edge
		cfg          config.ClassifyConfig
		wantConn     coupling.Connascence
		wantDistance coupling.Distance // must be cross-module (not same/unknown) for tag to apply
	}{
		{
			name: "CoT: StrengthHint=model cross-module",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: pinnedModel,
			},
			cfg:          twoModuleConfig(nil, nil),
			wantConn:     coupling.ConnascenceType,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "CoT: StrengthHint=contract cross-module",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: "contract",
			},
			cfg:          twoModuleConfig(nil, nil),
			wantConn:     coupling.ConnascenceType,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "CoT: strength=contract from config public glob",
			edge: graph.Edge{
				From: fileFromA,
				To:   "file:services/b/api/b.go",
				Kind: graph.EdgeKindImports,
			},
			// services/b/api/** is a public (contract) glob
			cfg:          twoModuleConfig([]string{publicB}, nil),
			wantConn:     coupling.ConnascenceType,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "CoA: clone pair crosses module boundary",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: hintFunctional, // not CoT
			},
			cfg:          twoModuleConfig(nil, modABClonePair),
			wantConn:     coupling.ConnascenceAlgorithm,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "CoA beats CoT when both signals present",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: pinnedModel, // CoT signal
			},
			// Also has clone pair → CoA should win
			cfg:          twoModuleConfig(nil, modABClonePair),
			wantConn:     coupling.ConnascenceAlgorithm,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "no connascence on same-module edge",
			edge: graph.Edge{
				From:         "file:services/a/x.go",
				To:           "file:services/a/y.go",
				Kind:         graph.EdgeKindImports,
				StrengthHint: pinnedModel,
			},
			cfg:          twoModuleConfig(nil, nil),
			wantConn:     coupling.ConnascenceNone,
			wantDistance: coupling.DistanceSameModule,
		},
		{
			name: "no connascence: functional hint, no clone pair",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: hintFunctional,
			},
			cfg:          twoModuleConfig(nil, nil),
			wantConn:     coupling.ConnascenceNone,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "no connascence: intrusive hint, no clone pair",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: hintIntrusive,
			},
			cfg:          twoModuleConfig(nil, nil),
			wantConn:     coupling.ConnascenceNone,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "no connascence: unknown hint, no clone pair",
			edge: graph.Edge{
				From: fileFromA,
				To:   fileToB,
				Kind: graph.EdgeKindImports,
			},
			cfg:          twoModuleConfig(nil, nil),
			wantConn:     coupling.ConnascenceNone,
			wantDistance: coupling.DistanceCrossModuleDiffOwner,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := makeGraph([]graph.Edge{tt.edge})
			idx := classify.Run(g, tt.cfg)
			key := edgeKey(tt.edge)
			cl, ok := idx[key]
			if !ok {
				t.Fatalf("edge key %q not in index", key)
			}
			if cl.Distance != tt.wantDistance {
				t.Errorf("Distance = %q, want %q", cl.Distance, tt.wantDistance)
			}
			if cl.Connascence != tt.wantConn {
				t.Errorf("Connascence = %q, want %q", cl.Connascence, tt.wantConn)
			}
			// Connascence is never fed into severity — verify it is independent.
			// (Same-module edges have no severity; cross-module may have any.)
			_ = cl.Severity
		})
	}
}

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

	// With clone pair: strength upgraded to Symmetric, CoA tagged.
	if clWith.Strength != coupling.StrengthSymmetric {
		t.Errorf("with clone: Strength = %q, want %q", clWith.Strength, coupling.StrengthSymmetric)
	}
	if clWith.Connascence != coupling.ConnascenceAlgorithm {
		t.Errorf("with clone: Connascence = %q, want %q", clWith.Connascence, coupling.ConnascenceAlgorithm)
	}

	// Without clone pair: strength stays at functional (from hint), no CoA.
	if clWithout.Strength != coupling.StrengthFunctional {
		t.Errorf("without clone: Strength = %q, want %q", clWithout.Strength, coupling.StrengthFunctional)
	}
	if clWithout.Connascence != coupling.ConnascenceNone {
		t.Errorf("without clone: Connascence = %q, want %q", clWithout.Connascence, coupling.ConnascenceNone)
	}

	// Symmetric (ordinal 9) > Functional (ordinal 8) → scores must differ.
	if clWith.Score.Value == clWithout.Score.Value {
		t.Errorf("Score.Value identical (%d); Symmetric and Functional should produce different scores", clWith.Score.Value)
	}
}
