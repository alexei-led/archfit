package metrics

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// Test constants for repeated string literals (goconst requirement).
const (
	testModStateStore = "stateStore"
	testModSingleHub  = "singleHub"
	testSymHubFn      = "hub_fn"
	testSymHub        = "hub"
	testModMod        = "mod"
	testModAlpha      = "alpha"
	testModBeta       = "beta"
	testSymDepA       = "depA"
	testSymDepB       = "depB"
	testSymA          = "symA"
	testSymB          = "symB"
)

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

// TestRiskHub_BroadSurfaceModuleRanksTop verifies that a module with many
// externally-referenced symbols ranks above one with fewer, even when the
// latter has a single heavily-referenced symbol (the key divergence from
// blast_radius). This tests the "broad-surface module outranks single-symbol hub"
// property that makes risk_hub complementary to blast_radius.
//
// stateStore has 4 symbols referenced from other modules (breadth=4).
// singleHub has 1 symbol referenced from 4 other modules (breadth=1).
// stateStore should rank higher.
func TestRiskHub_BroadSurfaceModuleRanksTop(t *testing.T) {
	g := makeGraph(
		map[string]string{
			// stateStore has 4 symbols each referenced by one external module
			"ss_field_a": testModStateStore,
			"ss_field_b": testModStateStore,
			"ss_field_c": testModStateStore,
			"ss_field_d": testModStateStore,
			// singleHub has 1 symbol referenced by 4 external modules
			testSymHubFn: testModSingleHub,
			// consumers in different modules
			"ca": "svc/a",
			"cb": "svc/b",
			"cc": "svc/c",
			"cd": "svc/d",
		},
		[][2]string{
			// 4 different symbols of stateStore each referenced by one consumer
			{"ca", "ss_field_a"},
			{"cb", "ss_field_b"},
			{"cc", "ss_field_c"},
			{"cd", "ss_field_d"},
			// singleHub's one symbol referenced by all 4 consumers
			{"ca", testSymHubFn},
			{"cb", testSymHubFn},
			{"cc", testSymHubFn},
			{"cd", testSymHubFn},
		},
	)
	m := newRiskHubMetric(makeConfig(nil))
	result := m.Calculate(MetricInput{SymbolGraph: g})

	if result.Band == bandNA {
		t.Fatal("expected a real result, got n/a")
	}
	// stateStore (breadth=4) should appear before singleHub (breadth=1).
	statePos := strings.Index(result.Display, "stateStore")
	hubPos := strings.Index(result.Display, "singleHub")
	if statePos == -1 || hubPos == -1 {
		t.Fatalf("expected both 'stateStore' and 'singleHub' in display, got: %s", result.Display)
	}
	if statePos > hubPos {
		t.Errorf("expected 'stateStore' (breadth 4) before 'singleHub' (breadth 1), got: %s", result.Display)
	}
}

// TestRiskHub_KnownHubRanksTop verifies that a module with externally-referenced
// symbols surfaces as the top-ranked hub.
func TestRiskHub_KnownHubRanksTop(t *testing.T) {
	// "core" module has a symbol referenced by 4 external modules.
	// "util" module has a symbol referenced by 1 external module.
	g := makeGraph(
		map[string]string{
			testSymHub: "core",
			"leaf":     "util",
			"A":        "svc/a",
			"B":        "svc/b",
			"C":        "svc/c",
			"D":        "svc/d",
		},
		[][2]string{
			{"A", testSymHub},
			{"B", testSymHub},
			{"C", testSymHub},
			{"D", testSymHub},
			{"A", "leaf"}, // leaf has only 1 external referencing module
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
	// core should appear before util in the display (higher breadth).
	corePos := strings.Index(result.Display, "core")
	utilPos := strings.Index(result.Display, "util")
	if utilPos != -1 && corePos > utilPos {
		t.Errorf("expected 'core' to appear before 'util' in display, got: %s", result.Display)
	}
}

// TestRiskHub_IntraModuleRefsIgnored verifies that references between symbols
// in the same module do not contribute to surface breadth.
func TestRiskHub_IntraModuleRefsIgnored(t *testing.T) {
	// All refs are within "mod" — no cross-module refs → breadth=0 → n/a.
	g := makeGraph(
		map[string]string{
			"X": testModMod,
			"Y": testModMod,
			"Z": testModMod,
		},
		[][2]string{
			{"X", "Y"},
			{"Y", "Z"},
			{"Z", "X"},
		},
	)
	m := newRiskHubMetric(makeConfig(nil))
	result := m.Calculate(MetricInput{SymbolGraph: g})

	if result.Band != bandNA {
		t.Errorf("expected n/a when only intra-module refs exist, got band=%q display=%q",
			result.Band, result.Display)
	}
}

// TestRiskHub_ChurnDoesNotAffectScore verifies that two MetricInput values that
// differ ONLY in FileChurn produce identical risk_hub output.
//
// This test is structural: because risk_hub reads volatility from the
// pre-captured moduleVolatility map (built in New before ApplyVolatility runs),
// FileChurn cannot reach the metric at all.
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
// never a false zero.
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
// raw breadth.
func TestRiskHub_HighVolatilityRanksAboveNeutral(t *testing.T) {
	g := makeGraph(
		map[string]string{
			testSymA:    testModAlpha,
			testSymB:    testModBeta,
			testSymDepA: "consumer",
			testSymDepB: "consumer2",
		},
		[][2]string{
			{testSymDepA, testSymA}, // alpha breadth=1
			{testSymDepB, testSymB}, // beta breadth=1 (equal breadth)
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
	// "alpha" should appear before "beta" because its score is higher
	// (1×1.0=1.0 vs 1×0.33=0.33).
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

// TestRiskHub_GitnexusImpactRefinesRanking verifies two properties:
//
//  1. When GitnexusImpact is non-empty, the module with higher historical impact
//     ranks above one with identical surface-breadth but lower impact.
//  2. When GitnexusImpact is nil, the result is byte-identical to today's
//     surface-breadth × volatility output (the gitnexus path is a no-op).
func TestRiskHub_GitnexusImpactRefinesRanking(t *testing.T) {
	// Two modules with equal breadth=1 so only gitnexus factor breaks the tie.
	g := makeGraph(
		map[string]string{
			testSymA:    testModAlpha,
			testSymB:    testModBeta,
			testSymDepA: "consumer",
			testSymDepB: "consumer2",
		},
		[][2]string{
			{testSymDepA, testSymA}, // alpha breadth=1
			{testSymDepB, testSymB}, // beta  breadth=1 (equal)
		},
	)
	m := newRiskHubMetric(makeConfig(nil))

	// Without gitnexus: both modules have equal score; alpha sorts first (alphabetical tiebreak).
	withoutGN := m.Calculate(MetricInput{SymbolGraph: g})
	if withoutGN.Band == bandNA {
		t.Fatal("expected real result without gitnexus, got n/a")
	}

	// With gitnexus: beta has much higher impact → should rank above alpha.
	impact := map[string]int{"beta": 100, "alpha": 1}
	withGN := m.Calculate(MetricInput{SymbolGraph: g, GitnexusImpact: impact})
	if withGN.Band == bandNA {
		t.Fatal("expected real result with gitnexus, got n/a")
	}

	// Verify ranking is affected: "beta" should appear before "alpha" in display.
	betaPos := strings.Index(withGN.Display, "beta")
	alphaPos := strings.Index(withGN.Display, "alpha")
	if betaPos == -1 || alphaPos == -1 {
		t.Fatalf("expected both 'beta' and 'alpha' in display, got: %s", withGN.Display)
	}
	if betaPos > alphaPos {
		t.Errorf("expected 'beta' (higher gitnexus impact) before 'alpha', got: %s", withGN.Display)
	}

	// Verify the gitnexus factor is surfaced in the display string.
	if !strings.Contains(withGN.Display, "gn×") {
		t.Errorf("expected gitnexus factor marker 'gn×' in display, got: %s", withGN.Display)
	}

	// Verify that nil GitnexusImpact produces the same output as an empty map
	// (both are the no-gitnexus path — behavior must be identical).
	withEmpty := m.Calculate(MetricInput{SymbolGraph: g, GitnexusImpact: map[string]int{}})
	if withoutGN.Display != withEmpty.Display {
		t.Errorf("nil vs empty GitnexusImpact differ:\n  nil:   %s\n  empty: %s",
			withoutGN.Display, withEmpty.Display)
	}
	if withoutGN.Value != withEmpty.Value {
		t.Errorf("nil vs empty GitnexusImpact Value differ: %v vs %v", withoutGN.Value, withEmpty.Value)
	}
}

// TestModuleSurfaceBreadth_EmptyGraph verifies that moduleSurfaceBreadth returns
// nil for an empty graph.
func TestModuleSurfaceBreadth_EmptyGraph(t *testing.T) {
	breadth := moduleSurfaceBreadth(symbol.Graph{})
	if breadth != nil {
		t.Errorf("expected nil for empty graph, got %v", breadth)
	}
}

// TestModuleSurfaceBreadth_CrossModuleOnly verifies that only cross-module refs
// contribute to breadth.
func TestModuleSurfaceBreadth_CrossModuleOnly(t *testing.T) {
	// X→Y are both in "mod" (intra-module); Z→X is cross-module.
	g := makeGraph(
		map[string]string{
			"X": "mod",
			"Y": "mod",
			"Z": "other",
		},
		[][2]string{
			{"X", "Y"}, // intra-module — should NOT count
			{"Z", "X"}, // cross-module — X in "mod" gets breadth 1
		},
	)
	breadth := moduleSurfaceBreadth(g)
	if breadth["mod"] != 1 {
		t.Errorf("mod should have breadth 1 (only X is cross-module referenced), got %d", breadth["mod"])
	}
	if breadth["other"] != 0 {
		t.Errorf("other should have breadth 0 (Z is a referencing module, not a target), got %d", breadth["other"])
	}
}

// TestModuleSurfaceBreadth_NoCrossModuleRefs verifies nil result when all refs
// are intra-module.
func TestModuleSurfaceBreadth_NoCrossModuleRefs(t *testing.T) {
	g := makeGraph(
		map[string]string{"A": "m", "B": "m"},
		[][2]string{{"A", "B"}},
	)
	breadth := moduleSurfaceBreadth(g)
	if breadth != nil {
		t.Errorf("expected nil when no cross-module refs, got %v", breadth)
	}
}
