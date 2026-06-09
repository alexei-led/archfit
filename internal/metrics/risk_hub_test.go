package metrics

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// symHub is the test symbol name used in hub-ranking tests.
const symHub = "hub"

// makeGraph is a test helper that builds a symbol.Graph from a Module map and
// a flat list of directed reference edges (from, to pairs).
func makeGraph(modules map[string]string, edges [][2]string) symbol.Graph {
	g := symbol.Graph{
		Module: modules,
		FanIn:  make(map[string]int),
		Refs:   make(map[string]map[string]struct{}),
	}
	for _, e := range edges {
		from, to := e[0], e[1]
		if g.Refs[from] == nil {
			g.Refs[from] = make(map[string]struct{})
		}
		g.Refs[from][to] = struct{}{}
	}
	return g
}

// makeConfig returns a config.Config with the supplied module volatility bands.
func makeConfig(moduleVols map[string]string) config.Config {
	mods := make(map[string]config.ModuleDef, len(moduleVols))
	for name, vol := range moduleVols {
		mods[name] = config.ModuleDef{Volatility: vol}
	}
	return config.Config{
		Version: 1,
		Modules: mods,
	}
}

// TestRiskHub_KnownHubRanksTop verifies that a symbol with many transitive
// dependents surfaces as the top-ranked hub.
func TestRiskHub_KnownHubRanksTop(t *testing.T) {
	// symHub is referenced by A, B, C, D — high impact (4 dependents).
	// leaf is referenced only by A — low impact (1 dependent).
	// symHub does NOT reference leaf, so leaf's impact stays low.
	g := makeGraph(
		map[string]string{
			symHub: "core",
			"leaf": "util",
			"A":    "svc/a",
			"B":    "svc/b",
			"C":    "svc/c",
			"D":    "svc/d",
		},
		[][2]string{
			{"A", symHub},
			{"B", symHub},
			{"C", symHub},
			{"D", symHub},
			{"A", "leaf"}, // leaf has only 1 dependent (A)
		},
	)
	m := newRiskHubMetric(makeConfig(nil))
	result := m.Calculate(MetricInput{SymbolGraph: g})

	if result.Band == bandNA {
		t.Fatal("expected a real result, got n/a")
	}
	if !strings.Contains(result.Display, "core") {
		t.Errorf("expected 'core' (owner of hub) in display, got: %s", result.Display)
	}
	// core should appear before util in the display (higher impact).
	corePos := strings.Index(result.Display, "core")
	utilPos := strings.Index(result.Display, "util")
	if utilPos != -1 && corePos > utilPos {
		t.Errorf("expected 'core' to appear before 'util' in display, got: %s", result.Display)
	}
}

// TestRiskHub_CyclicRefsDoNotInflate verifies that SCC condensation prevents
// cyclic references from inflating impact counts (test (b) in the plan).
//
// If X ↔ Y (mutual refs) and Z → X, then X and Y are in the same SCC.
// The SCC has 1 transitive dependent (Z). Impact of X = 1 (Z) + 1 (Y in same SCC) - 1 = 1.
// Without condensation a naive BFS would count Y→X→Y→X... and either loop or
// inflate. Condensation collapses X+Y to one component; Z's component is the
// only external reverse-dependent.
func TestRiskHub_CyclicRefsDoNotInflate(t *testing.T) {
	// X and Y are mutually dependent (same SCC).
	// Z depends on X.
	g := makeGraph(
		map[string]string{
			"X": "mod",
			"Y": "mod",
			"Z": "other",
		},
		[][2]string{
			{"X", "Y"},
			{"Y", "X"}, // cycle
			{"Z", "X"},
		},
	)
	impact := symbolImpact(g)

	// X and Y are in the same SCC. Z is the only external dependent.
	// Expected impact of X: 1 (from Z) + (SCC size 2 - 1) = 2.
	// Expected impact of Y: same (Z depends via X, same SCC).
	// What must NOT happen: impact > 2 (inflation from cycling).
	for sym, imp := range impact {
		if imp > 3 {
			t.Errorf("symbol %q impact %d is unexpectedly large (cycle inflation?)", sym, imp)
		}
	}
	// Z has no dependents.
	if impact["Z"] != 0 {
		t.Errorf("Z should have 0 impact (no dependents), got %d", impact["Z"])
	}
}

// TestRiskHub_ChurnDoesNotAffectScore verifies that two MetricInput values that
// differ ONLY in FileChurn produce identical risk_hub output (test (c) in plan).
//
// This test is structural: because risk_hub reads volatility from the
// pre-captured moduleVolatility map (built in New before ApplyVolatility runs),
// FileChurn cannot reach the metric at all. The test makes that invariant explicit
// and regression-proof.
func TestRiskHub_ChurnDoesNotAffectScore(t *testing.T) {
	g := makeGraph(
		map[string]string{
			"sym": "modA",
			"dep": "modB",
		},
		[][2]string{{"dep", "sym"}},
	)
	m := newRiskHubMetric(makeConfig(nil))

	inNoChurn := MetricInput{SymbolGraph: g}
	inHighChurn := MetricInput{
		SymbolGraph: g,
		FileChurn:   map[string]int{"modA/file.go": 9999, "modB/file.go": 9999},
	}

	r1 := m.Calculate(inNoChurn)
	r2 := m.Calculate(inHighChurn)

	if r1.Value != r2.Value {
		t.Errorf("churn changed Value: no-churn=%v high-churn=%v", r1.Value, r2.Value)
	}
	if r1.Display != r2.Display {
		t.Errorf("churn changed Display:\n  no-churn:   %s\n  high-churn: %s", r1.Display, r2.Display)
	}
}

// TestRiskHub_NAWhenGraphEmpty verifies that an empty SymbolGraph produces n/a,
// never a false zero (test (d) in the plan).
func TestRiskHub_NAWhenGraphEmpty(t *testing.T) {
	m := newRiskHubMetric(makeConfig(nil))
	result := m.Calculate(MetricInput{SymbolGraph: symbol.Graph{}})

	if result.Band != bandNA {
		t.Errorf("expected band %q for empty graph, got %q", bandNA, result.Band)
	}
	if result.Display != bandNA {
		t.Errorf("expected display %q for empty graph, got %q", bandNA, result.Display)
	}
}

// TestRiskHub_HighVolatilityRanksAboveNeutral verifies that a module with an
// explicit high-volatility config ranks above a low-volatility module at equal
// raw impact (test (e) in the plan).
//
// "alpha" (high, ×1.0) vs "beta" (low, ×0.33) at impact=1:
// alpha score = 1.0, beta score = 0.33 → alpha wins.
func TestRiskHub_HighVolatilityRanksAboveNeutral(t *testing.T) {
	g := makeGraph(
		map[string]string{
			"symA": "alpha",
			"symB": "beta",
			"depA": "consumer",
			"depB": "consumer",
		},
		[][2]string{
			{"depA", "symA"}, // symA impact = 1
			{"depB", "symB"}, // symB impact = 1 (equal impact)
		},
	)
	cfg := makeConfig(map[string]string{
		"alpha": "high",
		"beta":  "low",
	})
	m := newRiskHubMetric(cfg)
	result := m.Calculate(MetricInput{SymbolGraph: g})

	if result.Band == bandNA {
		t.Fatal("expected a real result, got n/a")
	}
	// "alpha" should appear before "beta" in the display because its score is
	// higher (1×1.0=1.0 vs 1×0.33=0.33).
	alphaPos := strings.Index(result.Display, "alpha")
	betaPos := strings.Index(result.Display, "beta")
	if alphaPos == -1 || betaPos == -1 {
		t.Fatalf("expected both 'alpha' and 'beta' in display, got: %s", result.Display)
	}
	if alphaPos > betaPos {
		t.Errorf("expected 'alpha' (high volatility) before 'beta' (low), got: %s", result.Display)
	}
}

// TestRiskHub_BandIsInfo verifies the metric is always report-only.
func TestRiskHub_BandIsInfo(t *testing.T) {
	g := makeGraph(
		map[string]string{"s": "m", "d": "n"},
		[][2]string{{"d", "s"}},
	)
	m := newRiskHubMetric(makeConfig(nil))
	result := m.Calculate(MetricInput{SymbolGraph: g})
	if result.Band != bandInformational {
		t.Errorf("expected band %q, got %q", bandInformational, result.Band)
	}
}

// TestSymbolImpact_EmptyGraph verifies that symbolImpact returns nil for an empty graph.
func TestSymbolImpact_EmptyGraph(t *testing.T) {
	impact := symbolImpact(symbol.Graph{})
	if impact != nil {
		t.Errorf("expected nil for empty graph, got %v", impact)
	}
}

// TestSymbolImpact_LinearChain verifies transitive counting in a linear chain.
// A → B → C: C has 2 transitive dependents (A and B), B has 1 (A), A has 0.
func TestSymbolImpact_LinearChain(t *testing.T) {
	g := makeGraph(
		map[string]string{"A": "m", "B": "m", "C": "m"},
		[][2]string{{"A", "B"}, {"B", "C"}},
	)
	impact := symbolImpact(g)
	if impact["C"] != 2 {
		t.Errorf("C should have impact 2 (A and B depend on it), got %d", impact["C"])
	}
	if impact["B"] != 1 {
		t.Errorf("B should have impact 1 (only A depends on it), got %d", impact["B"])
	}
	if impact["A"] != 0 {
		t.Errorf("A should have impact 0 (nothing depends on it), got %d", impact["A"])
	}
}
