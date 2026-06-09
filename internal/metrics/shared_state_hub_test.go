package metrics

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/symbol"
)

// makeGraphWithFanIn builds a symbol.Graph with explicit FanIn values.
// modules: symbol → dotted module path.
// fanIn:   symbol → fan-in count.
// Refs is left empty — shared_state_hub only reads Module and FanIn.
func makeGraphWithFanIn(modules map[string]string, fanIn map[string]int) symbol.Graph {
	g := symbol.Graph{
		Module: modules,
		FanIn:  make(map[string]int),
		Refs:   make(map[string]map[string]struct{}),
	}
	for sym, fi := range fanIn {
		g.FanIn[sym] = fi
	}
	return g
}

// -----------------------------------------------------------------------
// Test constants for module/symbol names.
// -----------------------------------------------------------------------

const (
	// deep-fan-in file (narrow surface, high depth — the "polling_state" stand-in)
	sshModDeep    = "app.handlers.polling.polling_state"
	sshSymDeepHub = "polling_state_cls" // the one heavily-shared symbol
	sshSymDeepB   = "internal_helper"   // low fan-in sibling

	// wide-surface file (many symbols, each with shallow fan-in)
	sshModWide  = "app.handlers.misc.wide_surface"
	sshSymWideA = "fn_alpha"
	sshSymWideB = "fn_beta"
	sshSymWideC = "fn_gamma"
	sshSymWideD = "fn_delta"
)

// TestSharedStateHub_NarrowDeepRanksAboveWideSurface verifies that a file with a
// single symbol of very high fan-in ranks above a file with many symbols each of
// low fan-in (the fundamental signal risk_hub misses).
func TestSharedStateHub_NarrowDeepRanksAboveWideSurface(t *testing.T) {
	g := makeGraphWithFanIn(
		map[string]string{
			// deep file: one hot symbol + one cold sibling
			sshSymDeepHub: sshModDeep,
			sshSymDeepB:   sshModDeep,
			// wide file: four symbols, each with fan-in 3
			sshSymWideA: sshModWide,
			sshSymWideB: sshModWide,
			sshSymWideC: sshModWide,
			sshSymWideD: sshModWide,
		},
		map[string]int{
			sshSymDeepHub: 22, // deep fan-in — 22 sibling files reference this symbol
			sshSymDeepB:   1,
			sshSymWideA:   3,
			sshSymWideB:   3,
			sshSymWideC:   3,
			sshSymWideD:   3,
		},
	)
	in := MetricInput{SymbolGraph: g}
	result := SharedStateHubMetric{}.Calculate(in)

	if result.Band == bandNA {
		t.Fatalf("expected real result, got n/a; display: %s", result.Display)
	}
	// Deep file must appear in the display.
	if !strings.Contains(result.Display, "polling_state") {
		t.Errorf("expected deep file 'polling_state' in display, got: %s", result.Display)
	}
	// Deep file (max fan-in=22) must rank before wide file (max fan-in=3).
	deepPos := strings.Index(result.Display, "polling_state")
	widePos := strings.Index(result.Display, "wide_surface")
	if deepPos == -1 || widePos == -1 {
		t.Fatalf("expected 'polling_state' and 'wide_surface' in display, got: %s", result.Display)
	}
	if deepPos > widePos {
		t.Errorf("deep file (max fan-in 22) should appear before wide file (max fan-in 3), got: %s", result.Display)
	}
	// Fan-in must be visible in display.
	if !strings.Contains(result.Display, "fan-in=22") {
		t.Errorf("expected 'fan-in=22' in display, got: %s", result.Display)
	}
}

// TestSharedStateHub_DefiningDocExcluded verifies that the metric does not
// self-inflate: because the SCIP reader already excludes the defining document
// when computing FanIn, a symbol's own file does not count as a referencing doc.
// We simulate this by giving a file's symbol a fan-in that already excludes self
// (the reader's contract) and confirm the reported value matches.
func TestSharedStateHub_DefiningDocExcluded(t *testing.T) {
	// Symbol has fan-in=5: exactly 5 *other* documents reference it.
	// If self-count were included it would be 6. We confirm the metric
	// reports 5, trusting the reader's contract.
	g := makeGraphWithFanIn(
		map[string]string{sshSymDeepHub: sshModDeep},
		map[string]int{sshSymDeepHub: 5},
	)
	in := MetricInput{SymbolGraph: g}
	result := SharedStateHubMetric{}.Calculate(in)

	if result.Band == bandNA {
		t.Fatalf("expected real result, got n/a; display: %s", result.Display)
	}
	if !strings.Contains(result.Display, "fan-in=5") {
		t.Errorf("expected fan-in=5 (self excluded by reader), got: %s", result.Display)
	}
}

// TestSharedStateHub_NAWhenGraphEmpty verifies that an empty SymbolGraph produces n/a.
func TestSharedStateHub_NAWhenGraphEmpty(t *testing.T) {
	result := SharedStateHubMetric{}.Calculate(MetricInput{SymbolGraph: symbol.Graph{}})

	if result.Band != bandNA {
		t.Errorf("expected band %q for empty graph, got %q display=%q", bandNA, result.Band, result.Display)
	}
	if result.Display != bandNA {
		t.Errorf("expected display %q for empty graph, got %q", bandNA, result.Display)
	}
}

// TestSharedStateHub_DistinctnessFromRiskHub verifies the key design property:
// a file that wins shared_state_hub (deep single-symbol fan-in, narrow surface)
// does NOT win risk_hub (which measures cross-module surface breadth).
//
// "deep_singleton" owns one symbol with fan-in=20 but is never referenced by any
// cross-module Refs edge, so risk_hub sees it as surface breadth=0.
// "inbound_hub" has many consumers referencing it via Refs, giving risk_hub a
// real breadth signal — but its max fan-in (=1 per symbol) is low.
func TestSharedStateHub_DistinctnessFromRiskHub(t *testing.T) {
	const (
		modDeepSingleton = "app.polling.polling_state"
		symDeepSingleton = "deep_singleton_sym"
		// inbound_hub: referenced by 4 external modules via Refs
		modInboundHub = "app.shared.inbound_hub"
		symInboundHub = "ih_fn"
		modConsA      = "app.consumers.a"
		modConsB      = "app.consumers.b"
		modConsC      = "app.consumers.c"
		modConsD      = "app.consumers.d"
		symConsA      = "ca"
		symConsB      = "cb"
		symConsC      = "cc"
		symConsD      = "cd"
	)

	// Build a graph with both FanIn data and Refs data.
	g := symbol.Graph{
		Module: map[string]string{
			symDeepSingleton: modDeepSingleton,
			symInboundHub:    modInboundHub,
			symConsA:         modConsA,
			symConsB:         modConsB,
			symConsC:         modConsC,
			symConsD:         modConsD,
		},
		FanIn: map[string]int{
			symDeepSingleton: 20, // 20 referencing docs — deep but narrow
			symInboundHub:    4,  // 4 referencing docs via Refs below
			symConsA:         0,
			symConsB:         0,
			symConsC:         0,
			symConsD:         0,
		},
		Refs: map[string]map[string]struct{}{
			// 4 consumers → inbound_hub: gives risk_hub breadth signal
			symConsA: {symInboundHub: {}},
			symConsB: {symInboundHub: {}},
			symConsC: {symInboundHub: {}},
			symConsD: {symInboundHub: {}},
			// deep_singleton has NO outbound Refs and NO inbound Refs edges,
			// so risk_hub sees surface breadth=0 for it.
		},
	}

	in := MetricInput{SymbolGraph: g}

	shResult := SharedStateHubMetric{}.Calculate(in)
	rhResult := newRiskHubMetric(makeConfig(nil)).Calculate(in)

	// shared_state_hub: deep_singleton (fan-in=20) must top the list.
	if shResult.Band == bandNA {
		t.Fatalf("shared_state_hub: expected real result, got n/a; display: %s", shResult.Display)
	}
	if !strings.Contains(shResult.Display, "polling_state") {
		t.Errorf("shared_state_hub: expected 'polling_state' in display, got: %s", shResult.Display)
	}
	// Verify deep_singleton outranks inbound_hub in shared_state_hub.
	deepPos := strings.Index(shResult.Display, "polling_state")
	inboundPos := strings.Index(shResult.Display, "inbound_hub")
	if inboundPos != -1 && deepPos > inboundPos {
		t.Errorf("shared_state_hub: deep_singleton should rank above inbound_hub, got: %s", shResult.Display)
	}

	// risk_hub: inbound_hub (4 external modules reference it) must appear.
	if rhResult.Band == bandNA {
		t.Fatalf("risk_hub: expected real result, got n/a; display: %s", rhResult.Display)
	}
	if !strings.Contains(rhResult.Display, "inbound_hub") {
		t.Errorf("risk_hub: expected 'inbound_hub' in display, got: %s", rhResult.Display)
	}
	// deep_singleton has no cross-module inbound Refs → must NOT appear in risk_hub.
	if strings.Contains(rhResult.Display, "polling_state") {
		t.Errorf("risk_hub: 'polling_state' should not appear (zero cross-module inbound), got: %s", rhResult.Display)
	}
}

// TestSharedStateHub_HotCountSecondarySortKey verifies that when two files share the
// same max fan-in, the file with more hot symbols (fan-in >= threshold) ranks first.
func TestSharedStateHub_HotCountSecondarySortKey(t *testing.T) {
	const (
		modA  = "app.mod.file_a"
		symA1 = "a_hot1"
		symA2 = "a_hot2"
		modB  = "app.mod.file_b"
		symB1 = "b_hot1"
		symB2 = "b_cold"
	)
	// Both files have max fan-in=10.
	// file_a has 2 hot symbols (fan-in >= 8); file_b has 1 hot symbol.
	g := makeGraphWithFanIn(
		map[string]string{
			symA1: modA, symA2: modA,
			symB1: modB, symB2: modB,
		},
		map[string]int{
			symA1: 10, symA2: 9, // both hot
			symB1: 10, symB2: 2, // only one hot
		},
	)
	in := MetricInput{SymbolGraph: g}
	result := SharedStateHubMetric{}.Calculate(in)

	if result.Band == bandNA {
		t.Fatalf("expected real result, got n/a; display: %s", result.Display)
	}
	aPos := strings.Index(result.Display, "file_a")
	bPos := strings.Index(result.Display, "file_b")
	if aPos == -1 || bPos == -1 {
		t.Fatalf("expected both 'file_a' and 'file_b' in display, got: %s", result.Display)
	}
	if aPos > bPos {
		t.Errorf("file_a (2 hot symbols) should rank before file_b (1 hot symbol), got: %s", result.Display)
	}
}

// TestSharedStateHub_BandIsInfo verifies the metric is always report-only.
func TestSharedStateHub_BandIsInfo(t *testing.T) {
	g := makeGraphWithFanIn(
		map[string]string{sshSymDeepHub: sshModDeep},
		map[string]int{sshSymDeepHub: 5},
	)
	in := MetricInput{SymbolGraph: g}
	result := SharedStateHubMetric{}.Calculate(in)

	if result.Band != bandInformational {
		t.Errorf("expected band %q, got %q", bandInformational, result.Band)
	}
}
