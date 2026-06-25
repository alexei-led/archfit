package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	bandNAStr      = "n/a"
	kindFunctionSF = "function" // syntax fact Kind for functions; used by unsafe_density and struct_field_density tests
	kindStructStr  = "struct"   // syntax fact Kind for structs; used by unsafe_density and panic_density tests
	// Shared file/module names used across unsafe_density and panic_density tests.
	fileARs          = "a.rs"
	fileMainRs       = "main.rs"
	fileMainGo       = "main.go"
	fileFooTestGo    = "foo_test.go"
	fileTestsIntegRs = "tests/integration.rs"
	modMyMod         = "mymod"
	modMyApp         = "myapp"
	langRust         = "rust"
	langGo           = "go"
	// TypeScript file paths reused across martin and file_mutual_import tests.
	tsFileA = "src/a.ts"
	tsFileB = "src/b.ts"
)

// crateHerdr is the single-crate fixture name reused across the Rust Cal-4 cases.
const crateHerdr = "herdr"

// Chain A -> B -> C (A imports B, B imports C). Reverse-deps: C has {A,B}=2,
// B has {A}=1, A has 0.
func TestBlastRadius_TransitiveReverseDeps(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.a"}
	b := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.b"}
	c := graph.Node{Kind: graph.NodeKindModule, Path: "pkg.c"}
	edges := []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports},
	}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, edges)

	res := modularity.BlastRadiusMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Name != "blast_radius" {
		t.Fatalf("name = %q", res.Name)
	}
	// 3 modules, threshold 30%: C reaches 2/2=100% -> a hub; B 1/2=50% -> a hub; A 0.
	if res.Value != 2 {
		t.Errorf("expected 2 hubs (C,B) got %v; display=%q", res.Value, res.Display)
	}
	// small N -> low confidence (not a quality verdict), never gating.
	if res.Confidence != confidenceLow {
		t.Errorf("expected low confidence for small N, got %q", res.Confidence)
	}
}

func TestStructuralWeight_GodModuleBySize(t *testing.T) {
	// Go graph (sets language) + per-file LOC: one module far over 4x the median
	// and the absolute floor is a god-module; a large-but-cohesive core just under
	// the multiple is not.
	a := graph.Node{Kind: graph.NodeKindPackage, Path: "internal/big"}
	b := graph.Node{Kind: graph.NodeKindPackage, Path: "internal/small"}
	g := metricstest.BuildGraph([]graph.Node{a, b}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: "go"},
	})
	fileLOC := map[string]int{
		"internal/big/a.go":   1600, // 1600 LOC -> god (>=4x median 200, >=400 floor)
		"internal/small/a.go": 200,
		"internal/small/b.go": 150,
		"internal/mid/a.go":   250,
	}

	res := modularity.StructuralWeightMetric{}.Calculate(signal.SizeInput{
		CommonInput: signal.CommonInput{Graph: g},
		Size:        signal.SizeSignals{FileLOC: fileLOC},
	})
	if res.Value != 1 {
		t.Errorf("expected 1 god-module got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, "internal/big") {
		t.Errorf("expected internal/big flagged; display=%q", res.Display)
	}
	if strings.Contains(res.Display, "internal/small") || strings.Contains(res.Display, "internal/mid") {
		t.Errorf("small/mid modules must not be god-modules; display=%q", res.Display)
	}
}

// TestStructuralWeight_RustModuleGranularity is the Cal-4 regression: a single-crate
// Rust repo must expose god *modules/files* by size, not collapse every .rs file to
// the one crate (which left structural_weight n/a, missing god files). With crate
// roots carried on the graph, per-file LOC maps to module keys ("<crate>::<mod>");
// without them the crate-only convention sees a single bucket and reports n/a.
func TestStructuralWeight_RustModuleGranularity(t *testing.T) {
	fileLOC := map[string]int{
		"src/server/headless.rs": 7843, // god file
		"src/integration/mod.rs": 6365, // god file
		"src/lib.rs":             50,
		"src/util.rs":            120,
		"src/config.rs":          100,
	}

	// With crate roots → module granularity → god modules measured.
	withRoots := graph.Build([]graph.Facts{{
		Nodes:      []graph.Node{{Kind: graph.NodeKindPackage, Path: crateHerdr}},
		Language:   graph.LangRust,
		CrateRoots: []graph.CrateRoot{{Dir: "", Name: crateHerdr}},
	}})
	res := modularity.StructuralWeightMetric{}.Calculate(signal.SizeInput{
		CommonInput: signal.CommonInput{Graph: withRoots},
		Size:        signal.SizeSignals{FileLOC: fileLOC},
	})
	if res.Value != 2 {
		t.Errorf("expected 2 god modules (server, integration) got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, "headless") || !strings.Contains(res.Display, "integration") {
		t.Errorf("expected server/integration god files flagged; display=%q", res.Display)
	}

	// Without crate roots, the crate-only convention collapses src/ files to "" → n/a
	// (the pre-Cal-4 blind spot this change fixes).
	noRoots := graph.Build([]graph.Facts{{
		Nodes:    []graph.Node{{Kind: graph.NodeKindPackage, Path: crateHerdr}, {Kind: graph.NodeKindExternal, Path: "serde"}},
		Edges:    []graph.Edge{{From: "package:" + crateHerdr, To: "external:serde", Kind: graph.EdgeKindDependsOn, Language: graph.LangRust}},
		Language: graph.LangRust,
	}})
	naRes := modularity.StructuralWeightMetric{}.Calculate(signal.SizeInput{
		CommonInput: signal.CommonInput{Graph: noRoots},
		Size:        signal.SizeSignals{FileLOC: fileLOC},
	})
	if naRes.Band != bandNAStr {
		t.Errorf("expected n/a without crate roots (crate-only collapse), got band=%q value=%v", naRes.Band, naRes.Value)
	}
}

func TestStructuralWeight_NoLOCIsNA(t *testing.T) {
	g := metricstest.BuildGraph([]graph.Node{{Kind: graph.NodeKindPackage, Path: "x"}}, nil)
	res := modularity.StructuralWeightMetric{}.Calculate(signal.SizeInput{
		CommonInput: signal.CommonInput{Graph: g},
	})
	if res.Band != bandNAStr {
		t.Errorf("expected n/a without LOC data, got %q", res.Band)
	}
}

func TestFileStructuralWeight_GodFileBySize(t *testing.T) {
	// One file far over 4x the median and floor → flagged.
	// Three small files (median ~150) → not flagged.
	fileLOC := map[string]int{
		"internal/score/score.go": 900, // ~6x median → god-file
		"internal/small/a.go":     150,
		"internal/small/b.go":     140,
		"internal/small/c.go":     160,
	}
	res := modularity.FileStructuralWeightMetric{}.Calculate(signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: fileLOC},
	})
	if res.Value != 1 {
		t.Errorf("expected 1 god-file, got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, "score.go") {
		t.Errorf("expected score.go flagged; display=%q", res.Display)
	}
	if strings.Contains(res.Display, "small") {
		t.Errorf("small files must not be god-files; display=%q", res.Display)
	}
}

func TestFileStructuralWeight_NoLOCIsNA(t *testing.T) {
	res := modularity.FileStructuralWeightMetric{}.Calculate(signal.SizeInput{})
	if res.Band != bandNAStr {
		t.Errorf("expected n/a without LOC data, got %q", res.Band)
	}
}

func TestFileStructuralWeight_NoGodsClean(t *testing.T) {
	// All files similar size → no god-files (value 0).
	fileLOC := map[string]int{
		fcPathA:      100,
		"pkg/b/b.go": 110,
		"pkg/c/c.go": 95,
		"pkg/d/d.go": 105,
		"pkg/e/e.go": 108,
	}
	res := modularity.FileStructuralWeightMetric{}.Calculate(signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: fileLOC},
	})
	if res.Value != 0 {
		t.Errorf("expected 0 god-files for uniform sizes, got %v; display=%q", res.Value, res.Display)
	}
	if res.Band != result.BandInformational {
		t.Errorf("expected info band, got %q", res.Band)
	}
}

func TestFileStructuralWeight_MedianThreshold(t *testing.T) {
	// Verify threshold math: median=200, 4x=800, floor=400, threshold=800.
	// File at 799 → NOT flagged. File at 800 → flagged.
	base := map[string]int{
		"a.go": 200, "b.go": 200, "c.go": 200, "d.go": 200, "e.go": 200,
	}
	notGod := make(map[string]int)
	for k, v := range base {
		notGod[k] = v
	}
	notGod["big.go"] = 799
	res := modularity.FileStructuralWeightMetric{}.Calculate(signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: notGod},
	})
	if res.Value != 0 {
		t.Errorf("799 LOC (just under threshold 800) must not be flagged; got %v display=%q", res.Value, res.Display)
	}

	isGod := make(map[string]int)
	for k, v := range base {
		isGod[k] = v
	}
	isGod["big.go"] = 800
	res2 := modularity.FileStructuralWeightMetric{}.Calculate(signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: isGod},
	})
	if res2.Value != 1 {
		t.Errorf("800 LOC (at threshold) must be flagged; got %v display=%q", res2.Value, res2.Display)
	}
}

func TestFileStructuralWeight_DeterministicSort(t *testing.T) {
	// Two files with the same LOC → order must be by path asc (deterministic).
	fileLOC := map[string]int{
		"z/file.go":  900, // same LOC
		"a/file.go":  900, // same LOC
		"x/small.go": 100,
		"b/small.go": 110,
		"c/small.go": 95,
	}
	res := modularity.FileStructuralWeightMetric{}.Calculate(signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: fileLOC},
	})
	// a/file.go must come before z/file.go in display
	aIdx := strings.Index(res.Display, "a/file")
	zIdx := strings.Index(res.Display, "z/file")
	if aIdx < 0 || zIdx < 0 {
		t.Fatalf("expected both a/file and z/file in display; got %q", res.Display)
	}
	if aIdx > zIdx {
		t.Errorf("a/file.go must appear before z/file.go (path tiebreaker); display=%q", res.Display)
	}
}
