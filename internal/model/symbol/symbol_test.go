package symbol_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/symbol"
)

// symbol/file/module constants for test fixtures.
const (
	symA = "symA"
	symB = "symB"
	symC = "symC"
	symD = "symD"
	symE = "symE"
	symF = "symF"
	symG = "symG"
	symH = "symH"
	symI = "symI"
	symJ = "symJ"
	symK = "symK"

	fileA = "src/a.go"
	fileB = "src/b.go"
	fileC = "src/c.go"
	fileE = "src/e.go"
	fileF = "src/f.go"
	fileG = "src/g.go"
	fileH = "src/h.go"
	fileJ = "src/j.go"
	fileK = "src/k.go"

	pkgA = "pkgA"
	pkgB = "pkgB"
	pkgC = "pkgC"
	pkgE = "pkgE"
	pkgF = "pkgF"
	pkgG = "pkgG"
	pkgH = "pkgH"
	pkgI = "pkgI"
	pkgJ = "pkgJ"
	pkgK = "pkgK"
)

// TestDependantsFromSymbolGraph covers the core algorithm and edge cases.
func TestDependantsFromSymbolGraph(t *testing.T) {
	t.Parallel()

	t.Run("empty graph returns nil", func(t *testing.T) {
		t.Parallel()
		if got := symbol.DependantsFromSymbolGraph(symbol.Graph{}); got != nil {
			t.Errorf("empty graph: want nil, got %v", got)
		}
	})

	t.Run("graph with refs but all same file returns nil", func(t *testing.T) {
		t.Parallel()
		// from and to both map to the same file: self-references excluded
		g := symbol.Graph{
			Module: map[string]string{symA: "modA", symB: "modA"},
			Path:   map[string]string{symA: fileA, symB: fileA},
			FanIn:  map[string]int{symB: 1},
			Refs:   map[string]map[string]struct{}{symA: {symB: {}}},
		}
		if got := symbol.DependantsFromSymbolGraph(g); got != nil {
			t.Errorf("same-file refs: want nil, got %v", got)
		}
	})

	t.Run("single cross-file edge", func(t *testing.T) {
		t.Parallel()
		// symA in fileA references symB in fileB → fileB has 1 distinct dependant (fileA)
		g := symbol.Graph{
			Module: map[string]string{symA: pkgA, symB: pkgB},
			Path:   map[string]string{symA: fileA, symB: fileB},
			FanIn:  map[string]int{symB: 1},
			Refs:   map[string]map[string]struct{}{symA: {symB: {}}},
		}
		got := symbol.DependantsFromSymbolGraph(g)
		if got == nil {
			t.Fatal("want non-nil map, got nil")
		}
		if got[fileB] != 1 {
			t.Errorf("fileB dependants = %d, want 1", got[fileB])
		}
		if v, ok := got[fileA]; ok {
			t.Errorf("fileA should not appear as a target, got %d", v)
		}
	})

	t.Run("multiple referrers from same file deduped", func(t *testing.T) {
		t.Parallel()
		// symC and symD both in fileC reference symE in fileE
		// → fileE should have 1 distinct dependant (fileC), not 2
		g := symbol.Graph{
			Module: map[string]string{symC: pkgC, symD: pkgC, symE: pkgE},
			Path:   map[string]string{symC: fileC, symD: fileC, symE: fileE},
			FanIn:  map[string]int{symE: 2},
			Refs: map[string]map[string]struct{}{
				symC: {symE: {}},
				symD: {symE: {}},
			},
		}
		got := symbol.DependantsFromSymbolGraph(g)
		if got == nil {
			t.Fatal("want non-nil map, got nil")
		}
		if got[fileE] != 1 {
			t.Errorf("fileE dependants = %d, want 1 (fileC deduplicated)", got[fileE])
		}
	})

	t.Run("multiple distinct referrers counted", func(t *testing.T) {
		t.Parallel()
		// symF in fileF and symG in fileG both reference symH in fileH
		// → fileH should have 2 distinct dependants
		g := symbol.Graph{
			Module: map[string]string{symF: pkgF, symG: pkgG, symH: pkgH},
			Path:   map[string]string{symF: fileF, symG: fileG, symH: fileH},
			FanIn:  map[string]int{symH: 2},
			Refs: map[string]map[string]struct{}{
				symF: {symH: {}},
				symG: {symH: {}},
			},
		}
		got := symbol.DependantsFromSymbolGraph(g)
		if got == nil {
			t.Fatal("want non-nil map, got nil")
		}
		if got[fileH] != 2 {
			t.Errorf("fileH dependants = %d, want 2", got[fileH])
		}
	})

	t.Run("missing from-path entries skipped", func(t *testing.T) {
		t.Parallel()
		// symI has no Path entry (empty string): must be skipped
		g := symbol.Graph{
			Module: map[string]string{symI: pkgI, symJ: pkgJ},
			Path:   map[string]string{symJ: fileJ}, // symI has no path
			FanIn:  map[string]int{symJ: 1},
			Refs:   map[string]map[string]struct{}{symI: {symJ: {}}},
		}
		if got := symbol.DependantsFromSymbolGraph(g); got != nil {
			t.Errorf("missing from-path: want nil, got %v", got)
		}
	})

	t.Run("graph with only FanIn but no Refs returns nil", func(t *testing.T) {
		t.Parallel()
		g := symbol.Graph{
			Module: map[string]string{symK: pkgK},
			FanIn:  map[string]int{symK: 5},
			Path:   map[string]string{symK: fileK},
		}
		if got := symbol.DependantsFromSymbolGraph(g); got != nil {
			t.Errorf("no Refs: want nil, got %v", got)
		}
	})
}
