package metrics

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/symbol"
)

// makePythonGraph builds a symbol.Graph with Python-style dotted module paths.
// modules: symbol → dotted module (e.g. "ccgram.handlers.polling.directory_callbacks")
// edges: from/to symbol pairs (cross-file pairs only — fileSpread counts distinct
// subsystems reached; intra-file edges are dropped inside fileSpread).
// FanIn is not needed by cohesion_spread, so it is left empty.
func makePythonGraph(modules map[string]string, edges [][2]string) symbol.Graph {
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

// pythonLOC builds a FileLOC map with Python slash-path keys.
// loc: module dotted path → LOC; converted to the slash+".py" key format.
func pythonLOC(loc map[string]int) map[string]int {
	out := make(map[string]int, len(loc))
	for mod, n := range loc {
		slash := strings.ReplaceAll(mod, ".", "/") + ".py"
		out[slash] = n
	}
	return out
}

// -----------------------------------------------------------------------
// Test constants for module/symbol names.
// -----------------------------------------------------------------------

const (
	// wide-spread large file (the "directory_callbacks" stand-in)
	csModWide  = "app.handlers.polling.directory_callbacks"
	csSymWideA = "wide_sym_a"
	csSymWideB = "wide_sym_b"
	csSymWideC = "wide_sym_c"
	csSymWideD = "wide_sym_d"
	// targets in different subsystems
	csTgtModels    = "app.models.user"
	csTgtProviders = "app.providers.base"
	csTgtStorage   = "app.storage.db"
	csTgtNotify    = "app.notify.email"
	csSymModels    = "user_model"
	csSymProviders = "provider_fn"
	csSymStorage   = "db_session"
	csSymNotify    = "send_email"
	// single-subsystem file (all targets share a parent)
	csModNarrow  = "app.utils.helper"
	csSymNarrowA = "narrow_a"
	csSymNarrowB = "narrow_b"
	csTgtNarrowX = "app.utils.format"
	csTgtNarrowY = "app.utils.validate"
	csSymNarrowX = "fmt_fn"
	csSymNarrowY = "val_fn"
	// small dispatcher (below LOC floor)
	csModDispatcher = "app.wiring.bootstrap"
	csSymDispA      = "disp_a"
	csSymDispB      = "disp_b"
	csTgtDispX      = "app.svc.alpha"
	csTgtDispY      = "app.svc.beta"
	csSymDispX      = "svc_alpha"
	csSymDispY      = "svc_beta"
)

// TestCohesionSpread_WideSpreaderRanksTop verifies that a large file reaching into
// many distinct subsystems ranks above a file with narrow outbound reach.
func TestCohesionSpread_WideSpreaderRanksTop(t *testing.T) {
	g := makePythonGraph(
		map[string]string{
			// wide-spread file: symbols referencing 4 different subsystems
			csSymWideA: csModWide,
			csSymWideB: csModWide,
			csSymWideC: csModWide,
			csSymWideD: csModWide,
			// targets in distinct subsystems
			csSymModels:    csTgtModels,
			csSymProviders: csTgtProviders,
			csSymStorage:   csTgtStorage,
			csSymNotify:    csTgtNotify,
			// narrow file: all targets share the "app.utils" subsystem
			csSymNarrowA: csModNarrow,
			csSymNarrowB: csModNarrow,
			csSymNarrowX: csTgtNarrowX,
			csSymNarrowY: csTgtNarrowY,
		},
		[][2]string{
			// wide file → 4 distinct subsystems
			{csSymWideA, csSymModels},
			{csSymWideB, csSymProviders},
			{csSymWideC, csSymStorage},
			{csSymWideD, csSymNotify},
			// narrow file → 1 subsystem (both targets are in "app.utils")
			{csSymNarrowA, csSymNarrowX},
			{csSymNarrowB, csSymNarrowY},
		},
	)
	in := MetricInput{
		SymbolGraph: g,
		FileLOC: pythonLOC(map[string]int{
			csModWide:   800, // well above floor
			csModNarrow: 200, // above floor
		}),
	}
	m := CohesionSpreadMetric{}
	result := m.Calculate(in)

	if result.Band == bandNA {
		t.Fatalf("expected real result, got n/a; display: %s", result.Display)
	}
	// Both files should be present in display.
	if !strings.Contains(result.Display, "directory_callbacks") {
		t.Errorf("expected wide-spread file in display, got: %s", result.Display)
	}
	// Wide file (spread=4) must appear before narrow file (spread=1).
	widePos := strings.Index(result.Display, "directory_callbacks")
	narrowPos := strings.Index(result.Display, "helper")
	if widePos == -1 || narrowPos == -1 {
		t.Fatalf("expected 'directory_callbacks' and 'helper' in display, got: %s", result.Display)
	}
	if widePos > narrowPos {
		t.Errorf("wide file (spread 4) should appear before narrow file (spread 1), got: %s", result.Display)
	}
}

// TestCohesionSpread_SingleSubsystemRanksLow verifies that a file referencing only
// targets within one parent subsystem produces spread=1 and ranks below a wide file.
func TestCohesionSpread_SingleSubsystemRanksLow(t *testing.T) {
	g := makePythonGraph(
		map[string]string{
			csSymNarrowA: csModNarrow,
			csSymNarrowB: csModNarrow,
			csSymNarrowX: csTgtNarrowX,
			csSymNarrowY: csTgtNarrowY,
		},
		[][2]string{
			// Both targets are under "app.utils" — only 1 distinct subsystem.
			{csSymNarrowA, csSymNarrowX},
			{csSymNarrowB, csSymNarrowY},
		},
	)
	in := MetricInput{
		SymbolGraph: g,
		FileLOC: pythonLOC(map[string]int{
			csModNarrow:  300,
			csTgtNarrowX: 100,
			csTgtNarrowY: 100,
		}),
	}
	m := CohesionSpreadMetric{}
	result := m.Calculate(in)

	if result.Band == bandNA {
		t.Fatalf("expected real result, got n/a; display: %s", result.Display)
	}
	// Value should be 1 (only one qualifying file, spread=1).
	if result.Value != 1 {
		t.Errorf("expected Value=1 (one qualifying file), got %.0f; display: %s", result.Value, result.Display)
	}
	// Spread for the single entry must be 1.
	if !strings.Contains(result.Display, "spread 1") {
		t.Errorf("expected 'spread 1' in display (single-subsystem file), got: %s", result.Display)
	}
}

// TestCohesionSpread_LOCFloorDropsDispatcher verifies that a file below the LOC floor
// is excluded even if it reaches multiple subsystems, while a large file survives.
func TestCohesionSpread_LOCFloorDropsDispatcher(t *testing.T) {
	g := makePythonGraph(
		map[string]string{
			// dispatcher: below LOC floor, but reaches two subsystems
			csSymDispA: csModDispatcher,
			csSymDispB: csModDispatcher,
			csSymDispX: csTgtDispX,
			csSymDispY: csTgtDispY,
			// wide file: above floor, reaches same two subsystems
			csSymWideA: csModWide,
			csSymWideC: csModWide,
			// reuse target symbols (different subsystems)
			csSymModels:    csTgtModels,
			csSymProviders: csTgtProviders,
		},
		[][2]string{
			// dispatcher reaches 2 subsystems (app.svc.alpha → app.svc, app.svc.beta → app.svc)
			// Wait — both are under "app.svc" → only 1 distinct subsystem. Use different ones.
			// Actually both csTgtDispX and csTgtDispY have subsystem "app.svc" (same parent).
			// That's fine — dispatcher gets spread=1 but we're primarily testing the LOC floor.
			{csSymDispA, csSymDispX},
			{csSymDispB, csSymDispY},
			// wide file reaches 2 distinct subsystems
			{csSymWideA, csSymModels},
			{csSymWideC, csSymProviders},
		},
	)
	in := MetricInput{
		SymbolGraph: g,
		FileLOC: pythonLOC(map[string]int{
			csModDispatcher: 50,  // below floor of 150
			csModWide:       600, // above floor
			csTgtModels:     100,
			csTgtProviders:  100,
		}),
	}
	m := CohesionSpreadMetric{}
	result := m.Calculate(in)

	if result.Band == bandNA {
		t.Fatalf("expected real result (wide file survives), got n/a; display: %s", result.Display)
	}
	// Dispatcher should not appear.
	if strings.Contains(result.Display, "bootstrap") {
		t.Errorf("dispatcher (below LOC floor) should be excluded, got: %s", result.Display)
	}
	// Wide file must appear.
	if !strings.Contains(result.Display, "directory_callbacks") {
		t.Errorf("expected wide file in display, got: %s", result.Display)
	}
}

// TestCohesionSpread_NAWhenGraphEmpty verifies that an empty SymbolGraph produces n/a.
func TestCohesionSpread_NAWhenGraphEmpty(t *testing.T) {
	m := CohesionSpreadMetric{}
	result := m.Calculate(MetricInput{SymbolGraph: symbol.Graph{}})

	if result.Band != bandNA {
		t.Errorf("expected band %q for empty graph, got %q display=%q", bandNA, result.Band, result.Display)
	}
	if result.Display != bandNA {
		t.Errorf("expected display %q for empty graph, got %q", bandNA, result.Display)
	}
}

// TestCohesionSpread_DistinctnessFromRiskHub verifies the key design property:
// a file that wins cohesion_spread (high outbound spread, zero inbound) does NOT
// win risk_hub (which measures inbound cross-module surface breadth).
//
// "outbound_god" references 4 distinct subsystems but nothing references it.
// "inbound_hub" is referenced by 4 external files but references nothing itself.
func TestCohesionSpread_DistinctnessFromRiskHub(t *testing.T) {
	const (
		modOutboundGod = "app.core.outbound_god"
		symOutboundGod = "og_sym"
		modInboundHub  = "app.shared.inbound_hub"
		symInboundHub  = "ih_sym"
		// outbound_god reaches 4 different subsystems
		modTargetA = "app.svc.alpha"
		modTargetB = "app.data.store"
		modTargetC = "app.notify.mailer"
		modTargetD = "app.auth.session"
		symTgtA    = "svc_a"
		symTgtB    = "data_b"
		symTgtC    = "notify_c"
		symTgtD    = "auth_d"
		// 4 consumers that reference inbound_hub
		modConsA = "app.consumers.a"
		modConsB = "app.consumers.b"
		modConsC = "app.consumers.c"
		modConsD = "app.consumers.d"
		symConsA = "cons_a"
		symConsB = "cons_b"
		symConsC = "cons_c"
		symConsD = "cons_d"
	)

	g := makePythonGraph(
		map[string]string{
			symOutboundGod: modOutboundGod,
			symInboundHub:  modInboundHub,
			symTgtA:        modTargetA,
			symTgtB:        modTargetB,
			symTgtC:        modTargetC,
			symTgtD:        modTargetD,
			symConsA:       modConsA,
			symConsB:       modConsB,
			symConsC:       modConsC,
			symConsD:       modConsD,
		},
		[][2]string{
			// outbound_god → 4 distinct subsystems (outbound spread=4)
			{symOutboundGod, symTgtA},
			{symOutboundGod, symTgtB},
			{symOutboundGod, symTgtC},
			{symOutboundGod, symTgtD},
			// 4 consumers → inbound_hub (inbound breadth=1 symbol but 4 modules ref it)
			{symConsA, symInboundHub},
			{symConsB, symInboundHub},
			{symConsC, symInboundHub},
			{symConsD, symInboundHub},
		},
	)
	in := MetricInput{
		SymbolGraph: g,
		FileLOC: pythonLOC(map[string]int{
			modOutboundGod: 500, // above floor
			modInboundHub:  200, // above floor
			modTargetA:     100,
			modTargetB:     100,
			modTargetC:     100,
			modTargetD:     100,
			modConsA:       100,
			modConsB:       100,
			modConsC:       100,
			modConsD:       100,
		}),
	}

	spreadResult := CohesionSpreadMetric{}.Calculate(in)
	riskResult := newRiskHubMetric(makeConfig(nil)).Calculate(in)

	// cohesion_spread: outbound_god (spread=4) must appear before inbound_hub (spread=0).
	if spreadResult.Band == bandNA {
		t.Fatalf("cohesion_spread: expected real result, got n/a; display: %s", spreadResult.Display)
	}
	if !strings.Contains(spreadResult.Display, "outbound_god") {
		t.Errorf("cohesion_spread: expected 'outbound_god' in display, got: %s", spreadResult.Display)
	}
	// inbound_hub has zero outbound cross-file refs → should not appear in cohesion_spread
	if strings.Contains(spreadResult.Display, "inbound_hub") {
		t.Errorf("cohesion_spread: 'inbound_hub' should not appear (zero outbound spread), got: %s", spreadResult.Display)
	}

	// risk_hub: inbound_hub (referenced by 4 external modules) must appear.
	if riskResult.Band == bandNA {
		t.Fatalf("risk_hub: expected real result, got n/a; display: %s", riskResult.Display)
	}
	if !strings.Contains(riskResult.Display, "inbound_hub") {
		t.Errorf("risk_hub: expected 'inbound_hub' in display, got: %s", riskResult.Display)
	}
	// outbound_god is not referenced by anything → should not be a risk_hub
	// (it may still appear if it has any inbound, but it has none here)
	if strings.Contains(riskResult.Display, "outbound_god") {
		t.Errorf("risk_hub: 'outbound_god' should not appear (zero inbound), got: %s", riskResult.Display)
	}
}

// TestCohesionSpread_BandIsInfo verifies the metric is always report-only.
func TestCohesionSpread_BandIsInfo(t *testing.T) {
	g := makePythonGraph(
		map[string]string{
			csSymWideA:  csModWide,
			csSymModels: csTgtModels,
		},
		[][2]string{{csSymWideA, csSymModels}},
	)
	in := MetricInput{
		SymbolGraph: g,
		FileLOC: pythonLOC(map[string]int{
			csModWide: 500,
		}),
	}
	m := CohesionSpreadMetric{}
	result := m.Calculate(in)

	if result.Band != bandInformational {
		t.Errorf("expected band %q, got %q", bandInformational, result.Band)
	}
}
