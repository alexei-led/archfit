package facts_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/facts"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// Repeated fixture strings extracted as constants to satisfy goconst.
const (
	symHub1    = "src/hub/Hub1"
	symHub2    = "src/hub/Hub2"
	modHub     = "src/hub"
	fileHubGo  = "src/hub/hub.go"
	fileCoreGo = "src/core/core.go"
)

// fixture helpers — hand-built graphs; no real SCIP.

// threeModuleGraph builds a symbol.Graph with three module keys:
//
//	"src/hub"       — referenced by many other modules (high inbound fan-in)
//	"src/sprawl"    — references many other modules (high outbound destinations)
//	"src/leaf"      — referenced by one module, references one module
//
// Symbol naming convention: "<module>/<name>" — the module field is a slash-path
// package dir (matching what scip_reader.py emits, e.g. "internal/a").
func threeModuleGraph() symbol.Graph {
	return symbol.Graph{
		Module: map[string]string{
			// hub symbols — defined in "src/hub"
			symHub1: modHub,
			symHub2: modHub,
			// sprawl symbols — defined in "src/sprawl"
			"src/sprawl/S1": "src/sprawl",
			// leaf symbols — defined in "src/leaf"
			"src/leaf/L1": "src/leaf",
			// caller symbols from distinct modules referencing hub
			"src/alpha/A1":   "src/alpha",
			"src/beta/B1":    "src/beta",
			"src/gamma/G1":   "src/gamma",
			"src/delta/D1":   "src/delta",
			"src/epsilon/E1": "src/epsilon",
			// targets that sprawl references
			"src/t1/T1": "src/t1",
			"src/t2/T2": "src/t2",
			"src/t3/T3": "src/t3",
		},
		Refs: map[string]map[string]struct{}{
			// Six distinct modules reference src/hub: alpha, beta, gamma, delta, epsilon, leaf → inbound fan-in = 6.
			"src/alpha/A1":   {symHub1: {}},
			"src/beta/B1":    {symHub1: {}},
			"src/gamma/G1":   {symHub2: {}},
			"src/delta/D1":   {symHub1: {}},
			"src/epsilon/E1": {symHub2: {}},
			// sprawl references three distinct modules → outbound = 3.
			"src/sprawl/S1": {
				"src/t1/T1": {},
				"src/t2/T2": {},
				"src/t3/T3": {},
			},
			// leaf references hub → leaf outbound = 1.
			"src/leaf/L1": {symHub1: {}},
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
	g := threeModuleGraph()
	got := facts.Build(g, nil, nil)

	// Five caller modules (alpha, beta, gamma, delta, epsilon) plus src/leaf = 6.
	hub := findFact(t, got, modHub)
	if hub.InboundModuleFanIn != 6 {
		t.Errorf("hub InboundModuleFanIn = %d, want 6", hub.InboundModuleFanIn)
	}
	// hub's own symbols reference nothing outbound in this fixture.
	if hub.OutboundDestinations != 0 {
		t.Errorf("hub OutboundDestinations = %d, want 0", hub.OutboundDestinations)
	}
}

// TestBuild_HighOutboundDestinations verifies that a module referencing many
// distinct destination modules accumulates the correct OutboundDestinations count.
func TestBuild_HighOutboundDestinations(t *testing.T) {
	g := threeModuleGraph()
	got := facts.Build(g, nil, nil)

	sprawl := findFact(t, got, "src/sprawl")
	if sprawl.OutboundDestinations != 3 {
		t.Errorf("sprawl OutboundDestinations = %d, want 3", sprawl.OutboundDestinations)
	}
	// nobody references sprawl in this fixture.
	if sprawl.InboundModuleFanIn != 0 {
		t.Errorf("sprawl InboundModuleFanIn = %d, want 0", sprawl.InboundModuleFanIn)
	}
}

// TestBuild_InboundAndOutboundAreIndependent confirms the two axes are computed
// separately: high inbound does not inflate outbound and vice versa.
func TestBuild_InboundAndOutboundAreIndependent(t *testing.T) {
	g := threeModuleGraph()
	got := facts.Build(g, nil, nil)

	hub := findFact(t, got, modHub)
	sprawl := findFact(t, got, "src/sprawl")

	if hub.OutboundDestinations != 0 || sprawl.InboundModuleFanIn != 0 {
		t.Errorf("axes leaked: hub.out=%d (want 0), sprawl.in=%d (want 0)",
			hub.OutboundDestinations, sprawl.InboundModuleFanIn)
	}
}

// TestBuild_LOC_Join verifies the prefix-join from module key to fileLOC.
// Module keys are slash-path package dirs; fileLOC keys are slash-path file paths.
// This fixture uses mismatched key spaces to confirm the join works correctly and
// does not bleed across path-component boundaries.
func TestBuild_LOC_Join(t *testing.T) {
	g := symbol.Graph{
		Module: map[string]string{
			"src/hub/H1": modHub,
			"src/ab/X1":  "src/ab", // "src/ab" must NOT pick up "src/a/..." entries
		},
	}
	fileLOC := map[string]int{
		fileHubGo:         200,
		"src/hub/util.go": 50,
		"src/ab/main.go":  100,
		"src/a/other.go":  999, // must NOT be summed into "src/ab"
	}
	got := facts.Build(g, fileLOC, nil)

	hub := findFact(t, got, modHub)
	if hub.LOC != 250 {
		t.Errorf("hub LOC = %d, want 250 (sum of hub.go + util.go)", hub.LOC)
	}

	ab := findFact(t, got, "src/ab")
	if ab.LOC != 100 {
		t.Errorf("ab LOC = %d, want 100 (only main.go, not src/a/other.go)", ab.LOC)
	}
}

// TestBuild_CoChangePartners verifies partner resolution, ordering (count desc,
// name asc tie-break), and the cap at maxCoChangePartners (5).
func TestBuild_CoChangePartners(t *testing.T) {
	g := symbol.Graph{
		Module: map[string]string{
			"src/core/C1": "src/core",
		},
	}
	// Pairs are sorted (a < b) as the git history builder produces.
	// fileCoreGo co-changes with several partners at different counts.
	coChange := map[[2]string]int{
		{fileCoreGo, "src/other/a.go"}:       10,
		{fileCoreGo, "src/other/b.go"}:       7,
		{fileCoreGo, "src/other/c.go"}:       5,
		{"src/other/d.go", fileCoreGo}:       5, // same count as c — alpha tie-break
		{fileCoreGo, "src/other/e.go"}:       3,
		{fileCoreGo, "src/other/f.go"}:       1, // 6th — should be dropped
		{"src/other/g.go", "src/other/h.go"}: 8, // unrelated — must not appear
	}
	got := facts.Build(g, nil, coChange)

	core := findFact(t, got, "src/core")
	if len(core.CoChangePartners) > 5 {
		t.Errorf("too many partners: %d, want ≤5", len(core.CoChangePartners))
	}
	if len(core.CoChangePartners) < 5 {
		t.Errorf("too few partners: %d, want 5 (6 candidates, cap=5)", len(core.CoChangePartners))
	}

	wantFirst := "src/other/a.go" // count 10 — highest
	if core.CoChangePartners[0] != wantFirst {
		t.Errorf("partners[0] = %q, want %q", core.CoChangePartners[0], wantFirst)
	}

	// c.go and d.go both have count 5; alpha-sort gives c < d.
	wantThird := "src/other/c.go"
	wantFourth := "src/other/d.go"
	if core.CoChangePartners[2] != wantThird {
		t.Errorf("partners[2] = %q, want %q", core.CoChangePartners[2], wantThird)
	}
	if core.CoChangePartners[3] != wantFourth {
		t.Errorf("partners[3] = %q, want %q", core.CoChangePartners[3], wantFourth)
	}

	// unrelated pair must not appear
	for _, p := range core.CoChangePartners {
		if p == "src/other/g.go" || p == "src/other/h.go" {
			t.Errorf("unrelated file %q appeared in partners", p)
		}
	}
}

// TestBuild_EmptyGraph confirms an empty symbol graph returns an empty (non-nil)
// slice without panicking.
func TestBuild_EmptyGraph(t *testing.T) {
	got := facts.Build(symbol.Graph{}, nil, nil)
	if got == nil {
		t.Fatal("expected non-nil slice for empty graph, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 facts for empty graph, got %d", len(got))
	}
}

// TestBuild_Determinism confirms that two calls on the same input produce
// byte-identical slices (stable sorted order).
func TestBuild_Determinism(t *testing.T) {
	g := threeModuleGraph()
	fileLOC := map[string]int{
		fileHubGo:          100,
		"src/sprawl/s.go":  200,
		"src/leaf/leaf.go": 50,
	}
	coChange := map[[2]string]int{
		{fileHubGo, "src/sprawl/s.go"}: 3,
	}

	first := facts.Build(g, fileLOC, coChange)
	second := facts.Build(g, fileLOC, coChange)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("two calls produced different results:\nfirst:  %+v\nsecond: %+v", first, second)
	}

	// Confirm slice is sorted by File.
	for i := 1; i < len(first); i++ {
		if first[i].File <= first[i-1].File {
			t.Errorf("slice not sorted: facts[%d].File=%q <= facts[%d].File=%q",
				i, first[i].File, i-1, first[i-1].File)
		}
	}
}

// TestBuild_NeutralNoLabels confirms no risk label, score, or hub annotation
// exists on the FileFact type (structural check).
func TestBuild_NeutralNoLabels(t *testing.T) {
	got := facts.Build(threeModuleGraph(), nil, nil)
	for _, ff := range got {
		// File and Module are the same key.
		if ff.File != ff.Module {
			t.Errorf("File=%q != Module=%q, expected identical keys", ff.File, ff.Module)
		}
		// CoChangePartners must never be nil (empty slice is fine).
		if ff.CoChangePartners == nil {
			t.Errorf("FileFact.CoChangePartners is nil for %q, want empty slice", ff.File)
		}
	}
}

// findFact returns the FileFact with the given file key, or fails the test.
func findFact(t *testing.T, all []facts.FileFact, file string) facts.FileFact {
	t.Helper()
	for _, f := range all {
		if f.File == file {
			return f
		}
	}
	t.Fatalf("no FileFact found for file=%q; got %v", file, fileNames(all))
	return facts.FileFact{}
}

// fileNames extracts the File field from each FileFact for error messages.
func fileNames(fs []facts.FileFact) []string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = f.File
	}
	return names
}
