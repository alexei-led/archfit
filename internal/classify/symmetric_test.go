package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// TestSymmetricStrengthUpgrade covers the clone-pair → Symmetric upgrade path
// and all cases where the upgrade must NOT fire (authoritative labels).
func TestSymmetricStrengthUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		edge            graph.Edge
		cfg             config.ClassifyConfig
		wantStrength    coupling.Strength
		wantConnascence coupling.Connascence
	}{
		{
			name: "clone pair + functional hint → symmetric",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: hintFunctional,
			},
			cfg:             twoModuleConfig(nil, modABClonePair),
			wantStrength:    coupling.StrengthSymmetric,
			wantConnascence: coupling.ConnascenceAlgorithm,
		},
		{
			name: "clone pair + unknown strength → symmetric",
			edge: graph.Edge{
				From: fileFromA,
				To:   fileToB,
				Kind: graph.EdgeKindImports,
				// no StrengthHint → unknown
			},
			cfg:             twoModuleConfig(nil, modABClonePair),
			wantStrength:    coupling.StrengthSymmetric,
			wantConnascence: coupling.ConnascenceAlgorithm,
		},
		{
			name: "clone pair + contract glob → contract (not overridden)",
			edge: graph.Edge{
				From: fileFromA,
				To:   "file:services/b/api/b.go",
				Kind: graph.EdgeKindImports,
				// no StrengthHint — glob match makes it contract
			},
			// services/b/api/** is a public (contract) glob; clone pair also present
			cfg:             twoModuleConfig([]string{publicB}, modABClonePair),
			wantStrength:    coupling.StrengthContract,
			wantConnascence: coupling.ConnascenceAlgorithm,
		},
		{
			name: "clone pair + pinned model label → model (not overridden)",
			edge: graph.Edge{
				From: fileFromA,
				To:   fileToB,
				Kind: graph.EdgeKindImports,
				// no StrengthHint — pinned label provides model
			},
			cfg: config.ClassifyConfig{
				Modules: map[string]config.ModuleDef{
					"modA": {Paths: []string{pathsA}},
					"modB": {Paths: []string{pathsB}},
				},
				ApprovedLabels: map[string]string{
					// key must be sorted: modA < modB
					"modA\x00modB": string(coupling.StrengthModel),
				},
				CrossModuleClonePairs: modABClonePair,
			},
			// model is not functional/unknown → upgrade must not fire
			wantStrength:    coupling.StrengthModel,
			wantConnascence: coupling.ConnascenceAlgorithm,
		},
		{
			name: "no clone pair + functional → functional",
			edge: graph.Edge{
				From:         fileFromA,
				To:           fileToB,
				Kind:         graph.EdgeKindImports,
				StrengthHint: hintFunctional,
			},
			cfg:             twoModuleConfig(nil, nil),
			wantStrength:    coupling.StrengthFunctional,
			wantConnascence: coupling.ConnascenceNone,
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
			if cl.Strength != tt.wantStrength {
				t.Errorf("Strength = %q, want %q", cl.Strength, tt.wantStrength)
			}
			if cl.Connascence != tt.wantConnascence {
				t.Errorf("Connascence = %q, want %q", cl.Connascence, tt.wantConnascence)
			}
		})
	}
}

// TestSymmetricDistributedMonolithDetected is an integration test proving that
// the book's distributed-monolith pattern is detected end-to-end:
// two modules in different deploy units with a clone pair and high volatility
// produce balance=1 (SeverityCritical) — the worst coupling score.
//
// Book formula: S=9 (Symmetric), D=9 (CrossDeployUnit), V=10 (high/core)
// modularity=|9-9|=0, volRescue=10-10=0, balance=max(0,0)+1=1 → critical.
func TestSymmetricDistributedMonolithDetected(t *testing.T) {
	t.Parallel()

	edge := graph.Edge{
		From:         "file:services/a/a.go",
		To:           "file:services/b/b.go",
		Kind:         graph.EdgeKindImports,
		StrengthHint: hintFunctional,
	}

	cfg := config.ClassifyConfig{
		Modules: map[string]config.ModuleDef{
			"svc-a": {
				Paths:      []string{"services/a/**"},
				DeployUnit: "svc-a",
				Subdomain:  subdomainCore, // volatility=high (10)
			},
			"svc-b": {
				Paths:      []string{"services/b/**"},
				DeployUnit: "svc-b",
				Subdomain:  subdomainCore, // volatility=high (10)
			},
		},
		// Clone pair between the two deploy units.
		CrossModuleClonePairs: map[string]struct{}{
			"svc-a\x00svc-b": {},
		},
	}

	g := makeGraph([]graph.Edge{edge})
	idx := classify.Run(g, cfg)
	key := edgeKey(edge)
	cl, ok := idx[key]
	if !ok {
		t.Fatalf("edge key %q not in index", key)
	}

	if cl.Strength != coupling.StrengthSymmetric {
		t.Errorf("Strength = %q, want %q", cl.Strength, coupling.StrengthSymmetric)
	}
	if cl.Distance != coupling.DistanceCrossDeployUnit {
		t.Errorf("Distance = %q, want %q", cl.Distance, coupling.DistanceCrossDeployUnit)
	}
	if cl.Volatility != coupling.VolatilityHigh {
		t.Errorf("Volatility = %q, want %q", cl.Volatility, coupling.VolatilityHigh)
	}
	if cl.Score.Balance != 1 {
		t.Errorf("Score.Balance = %d, want 1 (distributed monolith)", cl.Score.Balance)
	}
	if cl.Score.Band != coupling.SeverityCritical {
		t.Errorf("Score.Band = %q, want %q", cl.Score.Band, coupling.SeverityCritical)
	}
}
