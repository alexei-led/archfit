package classify_test

import (
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
)

// modABKey is the sorted null-delimited key for the modA→modB pair used in label maps.
const modABKey = modNameA + "\x00" + modNameB

// emptyGraph is a graph with no import edge between any module pair.
var emptyGraph = graph.Build(nil)

// TestCloneNeverUpgradesEdgeStrength: whatever else a clone pair does, it leaves
// the strength of an import edge to the edge's own evidence, labels included.
func TestCloneNeverUpgradesEdgeStrength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		edge         graph.Edge
		cfg          classify.Config
		wantStrength coupling.Strength
	}{
		{
			name:         "functional hint stays functional",
			edge:         graph.Edge{From: fileFromA, To: fileToB, Kind: graph.EdgeKindImports, StrengthHint: hintFunctional},
			cfg:          twoModuleConfig(nil, modABClonePair),
			wantStrength: coupling.StrengthFunctional,
		},
		{
			name:         "unknown strength stays unknown",
			edge:         graph.Edge{From: fileFromA, To: fileToB, Kind: graph.EdgeKindImports},
			cfg:          twoModuleConfig(nil, modABClonePair),
			wantStrength: coupling.StrengthUnknown,
		},
		{
			name:         "contract glob stays contract",
			edge:         graph.Edge{From: fileFromA, To: "file:services/b/api/b.go", Kind: graph.EdgeKindImports},
			cfg:          twoModuleConfig([]string{publicB}, modABClonePair),
			wantStrength: coupling.StrengthContract,
		},
		{
			name: "pinned label stays pinned",
			edge: graph.Edge{From: fileFromA, To: fileToB, Kind: graph.EdgeKindImports},
			cfg: classify.Config{
				Modules:               map[string]policy.ModuleDef{modNameA: {Paths: []string{pathsA}}, modNameB: {Paths: []string{pathsB}}},
				ApprovedLabels:        map[string]string{modABKey: string(coupling.StrengthFunctional)},
				CrossModuleClonePairs: modABClonePair,
			},
			wantStrength: coupling.StrengthFunctional,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cl, ok := classify.Run(makeGraph([]graph.Edge{tt.edge}), tt.cfg)[edgeKey(tt.edge)]
			if !ok {
				t.Fatal("edge not in index")
			}
			if cl.Strength != tt.wantStrength {
				t.Errorf("Strength = %q, want %q", cl.Strength, tt.wantStrength)
			}
		})
	}
}

// TestFunctionalEdgeVolatilityIsWorseOfBothSides: functional and symmetric
// coupling ties both sides (Ch7), so the worse volatility of source and target
// drives the edge. Contract, model and intrusive edges keep the target's.
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

// TestClonePairIsASymmetricFact: a clone pair between two deploy units is its
// own symmetric fact at D=9 and V=10: balance 1, the worst coupling score. When
// the modules also share an import edge the pair is marked Connected.
func TestClonePairIsASymmetricFact(t *testing.T) {
	t.Parallel()

	cfg := classify.Config{
		Modules: map[string]policy.ModuleDef{
			"svc-a": {Paths: []string{"services/a/**"}, DeployUnit: "svc-a", Subdomain: subdomainCore},
			"svc-b": {Paths: []string{"services/b/**"}, DeployUnit: "svc-b", Subdomain: subdomainCore},
		},
		CrossModuleClonePairs: map[string]struct{}{"svc-a\x00svc-b": {}},
	}
	edge := graph.Edge{From: "file:services/a/a.go", To: "file:services/b/b.go", Kind: graph.EdgeKindImports, StrengthHint: hintFunctional}

	tests := []struct {
		name          string
		g             *graph.Graph
		wantConnected bool
	}{
		{"import edge present", makeGraph([]graph.Edge{edge}), true},
		{"no import edge", emptyGraph, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pairs := classify.ClonePairs(tt.g, cfg)
			if len(pairs) != 1 {
				t.Fatalf("pairs = %d, want 1", len(pairs))
			}
			p := pairs[0]
			if p.Connected != tt.wantConnected {
				t.Errorf("Connected = %t, want %t", p.Connected, tt.wantConnected)
			}
			cl := p.Classification
			if cl.Strength != coupling.StrengthSymmetric || cl.Distance != coupling.DistanceCrossDeployUnit || cl.Score.Balance != 1 || cl.Score.Band != coupling.SeverityCritical {
				t.Errorf("fact = %s %s balance %d %s, want symmetric cross_deploy_unit 1 critical", cl.Strength, cl.Distance, cl.Score.Balance, cl.Score.Band)
			}
		})
	}
}

// TestClonePairCarriesEvidenceLocations: the real duplicated-code locations (both
// sides) travel with the clone fact, so the finding cites jscpd's file:line. A
// pair with no evidence synthesizes none.
func TestClonePairCarriesEvidenceLocations(t *testing.T) {
	t.Parallel()

	wantLocs := []graph.Location{{File: "crate_a/src/lib.rs", Line: 12}, {File: "crate_b/src/lib.rs", Line: 40}}
	wantCoupling := []coupling.Location{{File: "crate_a/src/lib.rs", Line: 12}, {File: "crate_b/src/lib.rs", Line: 40}}
	modules := map[string]policy.ModuleDef{modNameA: {Paths: []string{pathsA}}, modNameB: {Paths: []string{pathsB}}}

	with := classify.ClonePairs(emptyGraph, classify.Config{
		Modules: modules, CrossModuleClonePairs: modABClonePair,
		CloneEvidence: map[string][]graph.Location{modABKey: wantLocs},
	})
	if len(with) != 1 || !slices.Equal(with[0].Classification.CloneLocations, wantCoupling) {
		t.Fatalf("pairs = %+v, want one pair carrying %v", with, wantCoupling)
	}
	without := classify.ClonePairs(emptyGraph, classify.Config{Modules: modules, CrossModuleClonePairs: modABClonePair})
	if len(without) != 1 || len(without[0].Classification.CloneLocations) != 0 {
		t.Errorf("pairs = %+v, want one pair with no locations", without)
	}
}
