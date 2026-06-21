package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const bandNAStr = "n/a"

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
