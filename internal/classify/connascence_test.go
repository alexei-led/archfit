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
)

// twoModuleConfig returns a ClassifyConfig with two modules (modA: services/a/**,
// modB: services/b/**) and no public/internal globs unless provided.
func twoModuleConfig(publicB []string, clonePairs map[string]struct{}) config.ClassifyConfig {
	return config.ClassifyConfig{
		Modules: map[string]config.ModuleDef{
			"modA": {Paths: []string{"services/a/**"}},
			"modB": {Paths: []string{"services/b/**"}, Public: publicB},
		},
		CrossModuleClonePairs: clonePairs,
	}
}

// clonePairSet builds the canonical clone-pair key set for (a, b).
func clonePairSet(a, b string) map[string]struct{} {
	if a > b {
		a, b = b, a
	}
	return map[string]struct{}{a + "\x00" + b: {}}
}

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
			cfg:          twoModuleConfig([]string{"services/b/api/**"}, nil),
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
			cfg:          twoModuleConfig(nil, clonePairSet("modA", "modB")),
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
			cfg:          twoModuleConfig(nil, clonePairSet("modA", "modB")),
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

// TestConnascenceReportOnly verifies that connascence tags do NOT influence score
// or severity — they are purely descriptive metadata.
func TestConnascenceReportOnly(t *testing.T) {
	t.Parallel()

	// Two identical edges differing only in whether a clone pair is present.
	edge := graph.Edge{
		From:         fileFromA,
		To:           fileToB,
		Kind:         graph.EdgeKindImports,
		StrengthHint: hintFunctional,
	}

	cfgWithClone := twoModuleConfig(nil, clonePairSet("modA", "modB"))
	cfgWithout := twoModuleConfig(nil, nil)

	g := makeGraph([]graph.Edge{edge})
	key := edgeKey(edge)

	clWith := classify.Run(g, cfgWithClone)[key]
	clWithout := classify.Run(g, cfgWithout)[key]

	// CoA tag is set when clone pair present
	if clWith.Connascence != coupling.ConnascenceAlgorithm {
		t.Errorf("with clone: Connascence = %q, want %q", clWith.Connascence, coupling.ConnascenceAlgorithm)
	}
	if clWithout.Connascence != coupling.ConnascenceNone {
		t.Errorf("without clone: Connascence = %q, want %q", clWithout.Connascence, coupling.ConnascenceNone)
	}

	// Severity and Score must be IDENTICAL regardless of connascence tag.
	if clWith.Severity != clWithout.Severity {
		t.Errorf("Severity differs: with=%q without=%q", clWith.Severity, clWithout.Severity)
	}
	if clWith.Score.Value != clWithout.Score.Value {
		t.Errorf("Score.Value differs: with=%d without=%d", clWith.Score.Value, clWithout.Score.Value)
	}
	if clWith.Score.Band != clWithout.Score.Band {
		t.Errorf("Score.Band differs: with=%q without=%q", clWith.Score.Band, clWithout.Score.Band)
	}
}
