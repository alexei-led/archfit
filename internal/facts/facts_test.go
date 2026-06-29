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
	symHub1        = "hub.Hub1"
	symHub2        = "hub.Hub2"
	symSprawl1     = "sprawl.S1"
	symLeaf1       = "leaf.L1"
	symSpotinfo    = "spotinfo.S1"
	modHub         = "hub"
	modSprawl      = "sprawl"
	modSpotinfo    = "cmd/spotinfo"
	fileHubPy      = "src/hub/state.py"
	fileHubExtra   = "src/hub/extra.py"
	fileSprawlPy   = "src/sprawl/s.py"
	fileSpotinfoGo = "cmd/spotinfo/main.go"
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
	got := facts.Build(threeModuleGraph(), nil)

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
	got := facts.Build(threeModuleGraph(), nil)

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
	got := facts.Build(g, fileLOC)

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

// TestBuild_EmptyGraph confirms an empty symbol graph returns an empty (non-nil)
// slice without panicking.
func TestBuild_EmptyGraph(t *testing.T) {
	got := facts.Build(symbol.Graph{}, nil)
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

	first := facts.Build(g, fileLOC)
	second := facts.Build(g, fileLOC)

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
		"OutboundDestinations": {}, "LOC": {},
	}
	for field := range reflect.TypeFor[diagnostic.FileFact]().Fields() {
		if _, ok := allowed[field.Name]; !ok {
			t.Errorf("unexpected FileFact field %q — facts must stay neutral", field.Name)
		}
	}

	for _, ff := range facts.Build(threeModuleGraph(), nil) {
		if ff.Files == nil {
			t.Errorf("module %q: Files must be empty slice, not nil", ff.Module)
		}
	}
}

// TestBuild_InboundFanIn_ExcludesTestModules verifies that a referencing module
// whose name ends in ".test" (scip-go test-binary convention) is NOT counted in
// inbound_module_fanin, reproducing the spotinfo 3→2 eval finding.
func TestBuild_InboundFanIn_ExcludesTestModules(t *testing.T) {
	// "cmd/spotinfo" is referenced by two real modules and one ".test" module.
	// fan-in must be 2, not 3.
	g := symbol.Graph{
		Module: map[string]string{
			symSpotinfo:        modSpotinfo,
			"caller1.C1":       "pkg/caller1",
			"caller2.C2":       "pkg/caller2",
			"spotinfo.test.T1": modSpotinfo + ".test",
		},
		Path: map[string]string{
			symSpotinfo: fileSpotinfoGo,
		},
		Refs: map[string]map[string]struct{}{
			"caller1.C1":       {symSpotinfo: {}},
			"caller2.C2":       {symSpotinfo: {}},
			"spotinfo.test.T1": {symSpotinfo: {}}, // test binary — must be excluded
		},
	}

	got := facts.Build(g, nil)

	spot := findFact(t, got, modSpotinfo)
	if spot.InboundModuleFanIn != 2 {
		t.Errorf("InboundModuleFanIn = %d, want 2 (test module excluded)", spot.InboundModuleFanIn)
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
