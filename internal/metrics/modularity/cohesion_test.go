package modularity

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

const (
	modX  = "internal/x"
	modY  = "internal/y"
	pkgA  = "pkg.a"
	pkgB  = "pkg.b"
	docXA = "internal/x/a.go"
	docXB = "internal/x/b.go"
	docXC = "internal/x/c.go"
	docXD = "internal/x/d.go"
)

// symGraph is a small builder for cohesion tests. modules maps symbol → module;
// paths maps symbol → defining document; intra is the same-module edge list.
func symGraph(modules, paths map[string]string, intra [][2]string) symbol.Graph {
	refs := make(map[string]map[string]struct{})
	for _, e := range intra {
		if refs[e[0]] == nil {
			refs[e[0]] = make(map[string]struct{})
		}
		refs[e[0]][e[1]] = struct{}{}
	}
	fanIn := make(map[string]int, len(modules))
	for s := range modules {
		fanIn[s] = 0
	}
	return symbol.Graph{Module: modules, Path: paths, FanIn: fanIn, IntraRefs: refs}
}

func calcCohesion(g symbol.Graph) (band string, value float64, display string) {
	r := CohesionMetric{}.Calculate(signal.SymbolInput{Symbol: signal.SymbolSignals{Graph: g}})
	return r.Band, r.Value, r.Display
}

// TestCohesion_NAWhenNoSymbolGraph: SCIP off → n/a, not a false zero.
func TestCohesion_NAWhenNoSymbolGraph(t *testing.T) {
	if band, _, _ := calcCohesion(symbol.Graph{}); band != result.BandNA {
		t.Fatalf("empty graph band = %q, want %q", band, result.BandNA)
	}
}

// TestCohesion_NAWhenNoIntraEdges: a graph with symbols but no same-module edges
// (only cross-module Refs populated) is n/a — the proxy has no internal structure.
func TestCohesion_NAWhenNoIntraEdges(t *testing.T) {
	g := symbol.Graph{
		Module: map[string]string{"a": modX, "b": modX},
		Path:   map[string]string{"a": docXA, "b": docXB},
		FanIn:  map[string]int{"a": 0, "b": 0},
		Refs:   map[string]map[string]struct{}{"a": {"x": {}}}, // cross-module only
	}
	if band, _, _ := calcCohesion(g); band != result.BandNA {
		t.Fatalf("no-intra-edge band = %q, want %q", band, result.BandNA)
	}
}

// TestCohesion_SingleDocModulesUnmeasurable: when every module is a single
// document (the Python/TS shape), no module is measurable → n/a, NOT a verdict.
// This is the document-scoped enclosing_range caveat.
func TestCohesion_SingleDocModulesUnmeasurable(t *testing.T) {
	modules := map[string]string{
		"a1": pkgA, "a2": pkgA, "a3": pkgA, "a4": pkgA,
		"b1": pkgB, "b2": pkgB, "b3": pkgB, "b4": pkgB,
	}
	const fileA, fileB = "pkg/a.py", "pkg/b.py"
	paths := map[string]string{
		"a1": fileA, "a2": fileA, "a3": fileA, "a4": fileA,
		"b1": fileB, "b2": fileB, "b3": fileB, "b4": fileB,
	}
	// Even with an intra edge, single-document modules must be excluded.
	g := symGraph(modules, paths, [][2]string{{"a1", "a2"}})
	if band, _, _ := calcCohesion(g); band != result.BandNA {
		t.Fatalf("single-doc modules band = %q, want %q (unmeasurable)", band, result.BandNA)
	}
}

// TestCohesion_CohesiveMultiDocModule: a multi-file module whose files reference
// each other forms ONE component → not fragmented → 0 fragmented modules (info).
func TestCohesion_CohesiveMultiDocModule(t *testing.T) {
	modules := map[string]string{"a": modX, "b": modX, "c": modX, "d": modX}
	paths := map[string]string{"a": docXA, "b": docXB, "c": docXC, "d": docXD}
	// a→b, b→c, c→d chains all four symbols across four docs into one component.
	g := symGraph(modules, paths, [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}})
	band, value, display := calcCohesion(g)
	if band != result.BandInformational {
		t.Fatalf("band = %q, want %q", band, result.BandInformational)
	}
	if value != 0 {
		t.Fatalf("fragmented count = %v, want 0 (cohesive)", value)
	}
	if !strings.Contains(display, "1 measurable") {
		t.Errorf("display = %q, want it to report 1 measurable module", display)
	}
}

// TestCohesion_FragmentedMultiDocModule: a multi-file module whose files never
// reference each other splits into multiple components → flagged as fragmented.
func TestCohesion_FragmentedMultiDocModule(t *testing.T) {
	modules := map[string]string{"a": modX, "b": modX, "c": modX, "d": modX}
	// Two files; symbols pair up within each file but the files never link.
	paths := map[string]string{"a": docXA, "b": docXA, "c": docXC, "d": docXC}
	g := symGraph(modules, paths, [][2]string{{"a", "b"}, {"c", "d"}})
	band, value, display := calcCohesion(g)
	if band != result.BandInformational {
		t.Fatalf("band = %q, want %q", band, result.BandInformational)
	}
	if value != 1 {
		t.Fatalf("fragmented count = %v, want 1", value)
	}
	if !strings.Contains(display, "2 parts") {
		t.Errorf("display = %q, want it to report 2 parts", display)
	}
}

// TestCohesion_SmallModuleExcluded: a measurable-by-docs module below the symbol
// floor is not counted (too small for a lack-of-cohesion verdict).
func TestCohesion_SmallModuleExcluded(t *testing.T) {
	modules := map[string]string{"a": modX, "b": modX}
	paths := map[string]string{"a": docXA, "b": docXB}
	g := symGraph(modules, paths, nil) // 2 symbols < cohesionMinSymbols, 2 docs
	if band, _, _ := calcCohesion(g); band != result.BandNA {
		t.Fatalf("below-floor module band = %q, want %q (no measurable modules)", band, result.BandNA)
	}
}

// TestCohesion_Deterministic: byte-identical Display across repeated calls (no
// map-iteration-order leakage into ranking).
func TestCohesion_Deterministic(t *testing.T) {
	modules := map[string]string{
		"a": modX, "b": modX, "c": modX, "d": modX,
		"e": modY, "f": modY, "g": modY, "h": modY,
	}
	const docYE, docYG = "internal/y/e.go", "internal/y/g.go"
	paths := map[string]string{
		"a": docXA, "b": docXA, "c": docXC, "d": docXC,
		"e": docYE, "f": docYE, "g": docYG, "h": docYG,
	}
	g := symGraph(modules, paths, [][2]string{{"a", "b"}, {"c", "d"}, {"e", "f"}, {"g", "h"}})
	_, _, first := calcCohesion(g)
	for range 20 {
		if _, _, got := calcCohesion(g); got != first {
			t.Fatalf("non-deterministic display: %q != %q", got, first)
		}
	}
}

// TestLCOMComponents covers the union-find directly: isolated symbols each form
// their own component; an edge merges two.
func TestLCOMComponents(t *testing.T) {
	syms := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	if got := lcomComponents(syms, nil); got != 3 {
		t.Errorf("no edges: components = %d, want 3", got)
	}
	intra := map[string]map[string]struct{}{"a": {"b": {}}}
	if got := lcomComponents(syms, intra); got != 2 {
		t.Errorf("one edge: components = %d, want 2", got)
	}
	// Edge to a symbol outside the module set is ignored.
	intraOut := map[string]map[string]struct{}{"a": {"z": {}}}
	if got := lcomComponents(syms, intraOut); got != 3 {
		t.Errorf("out-of-module edge: components = %d, want 3", got)
	}
}
