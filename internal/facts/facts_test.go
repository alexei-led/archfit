package facts_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/facts"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// Repeated fixture strings extracted as constants to satisfy goconst.
const (
	symHub1      = "hub.Hub1"
	symHub2      = "hub.Hub2"
	symSprawl1   = "sprawl.S1"
	symLeaf1     = "leaf.L1"
	modHub       = "hub"
	modSprawl    = "sprawl"
	fileHubPy    = "src/hub/state.py"
	fileHubExtra = "src/hub/extra.py"
	fileSprawlPy = "src/sprawl/s.py"
	fileCorePy   = "src/core/core.py"
)

// threeModuleGraph builds a symbol.Graph with dotted Python-style module keys
// and slash-path defining files (matching what scip_reader.py emits):
//
//	"hub"    — referenced by many other modules (high inbound fan-in)
//	"sprawl" — references many other modules (high outbound destinations)
//	"leaf"   — references one module
func threeModuleGraph() symbol.Graph {
	return symbol.Graph{
		Module: map[string]string{
			symHub1:      modHub,
			symHub2:      modHub,
			symSprawl1:   modSprawl,
			symLeaf1:     "leaf",
			"alpha.A1":   "alpha",
			"beta.B1":    "beta",
			"gamma.G1":   "gamma",
			"delta.D1":   "delta",
			"epsilon.E1": "epsilon",
			"t1.T1":      "t1",
			"t2.T2":      "t2",
			"t3.T3":      "t3",
		},
		Path: map[string]string{
			symHub1:    fileHubPy,
			symHub2:    fileHubExtra,
			symSprawl1: fileSprawlPy,
			symLeaf1:   "src/leaf/leaf.py",
		},
		Refs: map[string]map[string]struct{}{
			// Six distinct modules reference hub: alpha..epsilon + leaf → inbound = 6.
			"alpha.A1":   {symHub1: {}},
			"beta.B1":    {symHub1: {}},
			"gamma.G1":   {symHub2: {}},
			"delta.D1":   {symHub1: {}},
			"epsilon.E1": {symHub2: {}},
			// sprawl references three distinct modules → outbound = 3.
			symSprawl1: {
				"t1.T1": {},
				"t2.T2": {},
				"t3.T3": {},
			},
			symLeaf1: {symHub1: {}},
		},
		FanIn: map[string]int{
			symHub1: 4,
			symHub2: 2,
		},
	}
}

// TestBuild_HighInboundFanIn verifies that a module referenced by many distinct
// other modules accumulates the correct InboundModuleFanIn count.
func TestBuild_HighInboundFanIn(t *testing.T) {
	got := facts.Build(threeModuleGraph(), nil, nil, nil)

	hub := findFact(t, got, modHub)
	if hub.InboundModuleFanIn != 6 {
		t.Errorf("hub InboundModuleFanIn = %d, want 6", hub.InboundModuleFanIn)
	}
	if hub.OutboundDestinations != 0 {
		t.Errorf("hub OutboundDestinations = %d, want 0", hub.OutboundDestinations)
	}
}

// TestBuild_HighOutboundDestinations verifies that a module referencing many
// distinct destination modules accumulates the correct OutboundDestinations count.
func TestBuild_HighOutboundDestinations(t *testing.T) {
	got := facts.Build(threeModuleGraph(), nil, nil, nil)

	sprawl := findFact(t, got, modSprawl)
	if sprawl.OutboundDestinations != 3 {
		t.Errorf("sprawl OutboundDestinations = %d, want 3", sprawl.OutboundDestinations)
	}
	if sprawl.InboundModuleFanIn != 0 {
		t.Errorf("sprawl InboundModuleFanIn = %d, want 0", sprawl.InboundModuleFanIn)
	}
}

// TestBuild_FilesAndLOC verifies the exact path join: Files come from
// symbol.Graph.Path, LOC sums fileLOC over exactly those files — dotted module
// keys never prefix-match against file paths.
func TestBuild_FilesAndLOC(t *testing.T) {
	g := threeModuleGraph()
	fileLOC := map[string]int{
		fileHubPy:           200,
		fileHubExtra:        50,
		"src/hub/orphan.py": 999, // defines no symbol — not attributed
		fileSprawlPy:        120,
	}
	got := facts.Build(g, fileLOC, nil, nil)

	hub := findFact(t, got, modHub)
	wantFiles := []string{fileHubExtra, fileHubPy}
	if !reflect.DeepEqual(hub.Files, wantFiles) {
		t.Errorf("hub Files = %v, want %v", hub.Files, wantFiles)
	}
	if hub.LOC != 250 {
		t.Errorf("hub LOC = %d, want 250 (state.py + extra.py)", hub.LOC)
	}

	sprawl := findFact(t, got, modSprawl)
	if sprawl.LOC != 120 {
		t.Errorf("sprawl LOC = %d, want 120", sprawl.LOC)
	}

	// A module with no Path data keeps empty Files and zero LOC — no fabrication.
	alpha := findFact(t, got, "alpha")
	if len(alpha.Files) != 0 || alpha.LOC != 0 {
		t.Errorf("alpha Files=%v LOC=%d, want empty/0 (no path data)", alpha.Files, alpha.LOC)
	}
}

// TestBuild_CoChangePartners verifies partner resolution through the file-path
// join, ordering (count desc, path asc tie-break), the cap at 5, and the
// exclusion of own-module files.
func TestBuild_CoChangePartners(t *testing.T) {
	g := symbol.Graph{
		Module: map[string]string{"core.C1": "core", "core.C2": "core"},
		Path: map[string]string{
			"core.C1": fileCorePy,
			"core.C2": "src/core/util.py",
		},
	}
	coChange := map[[2]string]int{
		{fileCorePy, "src/other/a.py"}:       10,
		{fileCorePy, "src/other/b.py"}:       7,
		{fileCorePy, "src/other/c.py"}:       5,
		{"src/other/d.py", fileCorePy}:       5, // same count as c — alpha tie-break
		{fileCorePy, "src/other/e.py"}:       3,
		{fileCorePy, "src/other/f.py"}:       1, // 6th — dropped by the cap
		{fileCorePy, "src/core/util.py"}:     9, // own-module partner — excluded
		{"src/other/g.py", "src/other/h.py"}: 8, // unrelated — must not appear
	}
	got := facts.Build(g, nil, coChange, nil)

	core := findFact(t, got, "core")
	want := []string{
		"src/other/a.py",
		"src/other/b.py",
		"src/other/c.py",
		"src/other/d.py",
		"src/other/e.py",
	}
	if !reflect.DeepEqual(core.CoChangePartners, want) {
		t.Errorf("partners = %v, want %v", core.CoChangePartners, want)
	}
}

// TestBuild_SymbolDependants verifies enrichment when the dependant map is present
// and nil SymbolDependants when absent or uncovered.
func TestBuild_SymbolDependants(t *testing.T) {
	g := threeModuleGraph()

	t.Run("absent map leaves all nil", func(t *testing.T) {
		for _, ff := range facts.Build(g, nil, nil, nil) {
			if ff.SymbolDependants != nil {
				t.Errorf("module %q SymbolDependants = %d, want nil", ff.Module, *ff.SymbolDependants)
			}
		}
	})

	t.Run("present map enriches covered modules only", func(t *testing.T) {
		// File-keyed counts; hub has two files — module count is the MAX.
		impact := map[string]int{fileHubPy: 41, fileHubExtra: 7, fileSprawlPy: 13}
		got := facts.Build(g, nil, nil, impact)

		hub := findFact(t, got, modHub)
		if hub.SymbolDependants == nil || *hub.SymbolDependants != 41 {
			t.Errorf("hub SymbolDependants = %v, want 41 (max over files)", hub.SymbolDependants)
		}
		sprawl := findFact(t, got, modSprawl)
		if sprawl.SymbolDependants == nil || *sprawl.SymbolDependants != 13 {
			t.Errorf("sprawl SymbolDependants = %v, want 13", sprawl.SymbolDependants)
		}
		leaf := findFact(t, got, "leaf")
		if leaf.SymbolDependants != nil {
			t.Errorf("leaf SymbolDependants = %d, want nil (not covered)", *leaf.SymbolDependants)
		}
	})
}

// TestBuild_EmptyGraph confirms an empty symbol graph returns an empty (non-nil)
// slice without panicking.
func TestBuild_EmptyGraph(t *testing.T) {
	got := facts.Build(symbol.Graph{}, nil, nil, nil)
	if got == nil {
		t.Fatal("expected non-nil slice for empty graph, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 facts for empty graph, got %d", len(got))
	}
}

// TestBuild_Determinism confirms that two calls on the same input produce
// deeply-equal slices in stable sorted order.
func TestBuild_Determinism(t *testing.T) {
	g := threeModuleGraph()
	fileLOC := map[string]int{
		fileHubPy:          100,
		fileSprawlPy:       200,
		"src/leaf/leaf.py": 50,
	}
	coChange := map[[2]string]int{
		{fileHubPy, fileSprawlPy}: 3,
	}
	impact := map[string]int{modHub: 7}

	first := facts.Build(g, fileLOC, coChange, impact)
	second := facts.Build(g, fileLOC, coChange, impact)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("two calls produced different results:\nfirst:  %+v\nsecond: %+v", first, second)
	}

	for i := 1; i < len(first); i++ {
		if first[i].Module <= first[i-1].Module {
			t.Errorf("slice not sorted: facts[%d].Module=%q <= facts[%d].Module=%q",
				i, first[i].Module, i-1, first[i-1].Module)
		}
	}
}

// TestBuild_NeutralNoLabels confirms the facts stay neutral: only the known
// evidence fields exist on FileFact — no risk label, score, band, or rank.
func TestBuild_NeutralNoLabels(t *testing.T) {
	allowed := map[string]struct{}{
		"Module": {}, "Files": {}, "InboundModuleFanIn": {},
		"OutboundDestinations": {}, "LOC": {}, "CoChangePartners": {},
		"SymbolDependants": {},
	}
	for field := range reflect.TypeFor[diagnostic.FileFact]().Fields() {
		if _, ok := allowed[field.Name]; !ok {
			t.Errorf("unexpected FileFact field %q — facts must stay neutral", field.Name)
		}
	}

	for _, ff := range facts.Build(threeModuleGraph(), nil, nil, nil) {
		if ff.CoChangePartners == nil || ff.Files == nil {
			t.Errorf("module %q: nested slices must be empty, not nil", ff.Module)
		}
	}
}

// findFact returns the FileFact with the given module key, or fails the test.
func findFact(t *testing.T, all []diagnostic.FileFact, module string) diagnostic.FileFact {
	t.Helper()
	for _, f := range all {
		if f.Module == module {
			return f
		}
	}
	t.Fatalf("no FileFact found for module=%q; got %v", module, moduleNames(all))
	return diagnostic.FileFact{}
}

// moduleNames extracts the Module field from each FileFact for error messages.
func moduleNames(fs []diagnostic.FileFact) []string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = f.Module
	}
	return names
}
