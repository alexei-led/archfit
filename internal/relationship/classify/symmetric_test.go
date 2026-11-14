package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
)

func TestFunctionalEdgeVolatilityIsWorseOfBothSides(t *testing.T) {
	t.Parallel()

	const (
		high, low, med, frozen = "high", "low", "medium", "frozen"
	)
	tests := []struct {
		name     string
		strength coupling.Strength
		src, dst string // declared volatility ("" = undeclared)
		want     coupling.Volatility
	}{
		{"functional: high source, low target", coupling.StrengthFunctional, high, low, coupling.VolatilityHigh},
		{"functional: low source, high target", coupling.StrengthFunctional, low, high, coupling.VolatilityHigh},
		{"functional: both low", coupling.StrengthFunctional, low, low, coupling.VolatilityLow},
		{"functional: medium and low", coupling.StrengthFunctional, med, low, coupling.VolatilityMedium},
		{"functional: frozen and low", coupling.StrengthFunctional, frozen, low, coupling.VolatilityLow},
		{"functional: undeclared source, low target is unrated", coupling.StrengthFunctional, "", low, coupling.VolatilityUndeclared},
		{"functional: declared high beats undeclared", coupling.StrengthFunctional, high, "", coupling.VolatilityHigh},
		{"symmetric: high source, low target", coupling.StrengthSymmetric, high, low, coupling.VolatilityHigh},
		{"model keeps the target volatility", coupling.StrengthModel, high, low, coupling.VolatilityLow},
		{"contract keeps the target volatility", coupling.StrengthContract, high, low, coupling.VolatilityLow},
		{"intrusive keeps the target volatility", coupling.StrengthIntrusive, high, low, coupling.VolatilityLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := classify.Config{Modules: map[string]policy.ModuleDef{
				modNameA: {Paths: []string{pathsA}, Volatility: tt.src},
				modNameB: {Paths: []string{pathsB}, Volatility: tt.dst},
			}}
			edge := graph.Edge{From: fileFromA, To: fileToB, Kind: graph.EdgeKindImports, StrengthHint: string(tt.strength)}
			cl := classify.Run(makeGraph([]graph.Edge{edge}), cfg)[edgeKey(edge)]
			if cl.Strength != tt.strength {
				t.Fatalf("Strength = %q, want %q", cl.Strength, tt.strength)
			}
			if cl.Volatility != tt.want {
				t.Errorf("Volatility = %q, want %q", cl.Volatility, tt.want)
			}
		})
	}
}
