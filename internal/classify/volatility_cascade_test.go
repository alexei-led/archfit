package classify_test

// Tests for the inferred-volatility cascade (book Ch9).
//
// The cascade raises a module's effective volatility to high when it is
// strongly coupled (strength ≥ functional) to a high-volatility module.
// Weak coupling (contract/model) does not propagate. The pass is single-hop
// and config-gated (VolatilityCascadeEnabled).

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	cascadeFilePayment       = "payment/svc.go"
	cascadeFileNotifications = "notifications/svc.go"
	cascadeFileGateway       = "gateway/svc.go"
)

// buildGraph constructs a Graph from a slice of (from, to, strengthHint) triples.
// Node paths use the "file" kind so pathFromID strips the prefix correctly.
func buildGraph(edges [][3]string) *graph.Graph {
	seen := make(map[string]bool)
	var nodes []graph.Node
	gedges := make([]graph.Edge, 0, len(edges))
	addNode := func(path string) {
		id := "file:" + path
		if !seen[id] {
			seen[id] = true
			nodes = append(nodes, graph.Node{Kind: graph.NodeKindFile, Path: path})
		}
	}
	for _, e := range edges {
		from, to, hint := e[0], e[1], e[2]
		addNode(from)
		addNode(to)
		gedges = append(gedges, graph.Edge{
			From:         "file:" + from,
			To:           "file:" + to,
			Kind:         graph.EdgeKindImports,
			StrengthHint: hint,
		})
	}
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: gedges}})
}

// probeKey returns the coupling.Index key for a probe edge.
func probeKey(from, to string) string {
	return "file:" + from + "\x00" + "file:" + to + "\x00" + string(graph.EdgeKindImports)
}

// cascadeModules is the module map used across TestVolatilityCascade cases.
// subdomainCore/Supporting/Generic constants are declared in classify_test.go (same package).
var cascadeModules = map[string]config.ModuleDef{
	"payment":       {Paths: []string{"payment/**"}, Subdomain: subdomainCore},             // high
	"notifications": {Paths: []string{"notifications/**"}, Subdomain: subdomainSupporting}, // medium
	"gateway":       {Paths: []string{"gateway/**"}, Subdomain: subdomainGeneric},          // low
	"unknown":       {Paths: []string{"unknown/**"}},                                       // undeclared
}

func TestVolatilityCascade(t *testing.T) {
	tests := []struct {
		name           string
		cascadeEnabled bool
		// primary edge: from→to with given strength hint
		from string
		to   string
		hint string
		// probeTarget: we add a probe→probeTarget edge and read probeTarget's effective vol.
		// Normally probeTarget == from so we observe the from-module's post-cascade vol.
		probeTarget string
		wantVol     coupling.Volatility
	}{
		{
			name:           "disabled: supporting stays medium",
			cascadeEnabled: false,
			from:           cascadeFileNotifications,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthFunctional),
			probeTarget:    cascadeFileNotifications,
			wantVol:        coupling.VolatilityMedium,
		},
		{
			name:           "enabled functional: supporting raised to high",
			cascadeEnabled: true,
			from:           cascadeFileNotifications,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthFunctional),
			probeTarget:    cascadeFileNotifications,
			wantVol:        coupling.VolatilityHigh,
		},
		{
			name:           "enabled symmetric: supporting raised to high",
			cascadeEnabled: true,
			from:           cascadeFileNotifications,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthSymmetric),
			probeTarget:    cascadeFileNotifications,
			wantVol:        coupling.VolatilityHigh,
		},
		{
			name:           "enabled intrusive: supporting raised to high",
			cascadeEnabled: true,
			from:           cascadeFileNotifications,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthIntrusive),
			probeTarget:    cascadeFileNotifications,
			wantVol:        coupling.VolatilityHigh,
		},
		{
			name:           "enabled contract: does NOT propagate",
			cascadeEnabled: true,
			from:           cascadeFileNotifications,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthContract),
			probeTarget:    cascadeFileNotifications,
			wantVol:        coupling.VolatilityMedium,
		},
		{
			name:           "enabled model: does NOT propagate",
			cascadeEnabled: true,
			from:           cascadeFileNotifications,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthModel),
			probeTarget:    cascadeFileNotifications,
			wantVol:        coupling.VolatilityMedium,
		},
		{
			// payment→gateway: gateway(low) is NOT high, so payment is not raised.
			// Probe payment: should remain high (core — never lowered).
			name:           "cascade does not lower: core stays high when coupled to low",
			cascadeEnabled: true,
			from:           cascadeFilePayment,
			to:             cascadeFileGateway,
			hint:           string(coupling.StrengthFunctional),
			probeTarget:    cascadeFilePayment,
			wantVol:        coupling.VolatilityHigh,
		},
		{
			name:           "generic raised to high via functional to core",
			cascadeEnabled: true,
			from:           cascadeFileGateway,
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthFunctional),
			probeTarget:    cascadeFileGateway,
			wantVol:        coupling.VolatilityHigh,
		},
		{
			name:           "undeclared raised to high via functional to core",
			cascadeEnabled: true,
			from:           "unknown/svc.go",
			to:             cascadeFilePayment,
			hint:           string(coupling.StrengthFunctional),
			probeTarget:    "unknown/svc.go",
			wantVol:        coupling.VolatilityHigh,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := "probe/x.go"
			g := buildGraph([][3]string{
				{tc.from, tc.to, tc.hint},
				{probe, tc.probeTarget, ""},
			})
			c := config.ClassifyConfig{
				Modules:                  cascadeModules,
				VolatilityCascadeEnabled: tc.cascadeEnabled,
			}
			idx := classify.Run(g, c)
			key := probeKey(probe, tc.probeTarget)
			cl, ok := idx[key]
			if !ok {
				t.Fatalf("probe edge %q not in index", key)
			}
			if cl.Volatility != tc.wantVol {
				t.Errorf("volatility = %q, want %q", cl.Volatility, tc.wantVol)
			}
		})
	}
}

// TestVolatilityCascade_SingleHop verifies the cascade is a single read-base-only pass.
//
// Setup: A→B (functional, B=core/high), B→C (functional, C=supporting/medium).
// A should be raised (A→B, base[B]=high).
// C should NOT be raised: edge B→C has base[C]=medium — not high — so no raise.
// The cascade never chains through effective values.
func TestVolatilityCascade_SingleHop(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"modA": {Paths: []string{"a/**"}, Subdomain: subdomainSupporting}, // medium base
		"modB": {Paths: []string{"b/**"}, Subdomain: subdomainCore},       // high base
		"modC": {Paths: []string{"c/**"}, Subdomain: subdomainSupporting}, // medium base
	}
	probe := "probe/x.go"
	g := buildGraph([][3]string{
		{"a/x.go", "b/x.go", string(coupling.StrengthFunctional)}, // A→B: A raised
		{"b/x.go", "c/x.go", string(coupling.StrengthFunctional)}, // B→C: C not raised (base[C]=medium)
		{probe, "a/x.go", ""},
		{probe, "c/x.go", ""},
	})
	c := config.ClassifyConfig{
		Modules:                  modules,
		VolatilityCascadeEnabled: true,
	}
	idx := classify.Run(g, c)

	clA, okA := idx[probeKey(probe, "a/x.go")]
	clC, okC := idx[probeKey(probe, "c/x.go")]
	if !okA || !okC {
		t.Fatalf("probe edges missing from index")
	}
	// A strongly coupled to B (high base) → raised.
	if clA.Volatility != coupling.VolatilityHigh {
		t.Errorf("modA: got %q, want high (raised by cascade)", clA.Volatility)
	}
	// C not raised: base[C]=medium, so B→C does not trigger propagation.
	if clC.Volatility != coupling.VolatilityMedium {
		t.Errorf("modC: got %q, want medium (single-hop: no chaining)", clC.Volatility)
	}
}
