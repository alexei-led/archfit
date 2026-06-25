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

// TestFileMutualImport_NilGraph: n/a when no graph provided.
func TestFileMutualImport_NilGraph(t *testing.T) {
	m := modularity.FileMutualImportMetric{}
	res := m.Calculate(signal.CommonInput{})
	if res.Band != "n/a" {
		t.Errorf("expected n/a for nil graph, got %q", res.Band)
	}
}

// TestFileMutualImport_NoFileNodes: n/a when graph has only package/module nodes (e.g. Go graph).
func TestFileMutualImport_NoFileNodes(t *testing.T) {
	// Go-style: file→package (no file→file edges, no NodeKindFile nodes in this test).
	pkgA := graph.Node{Kind: graph.NodeKindPackage, Path: "pkg/a"}
	pkgB := graph.Node{Kind: graph.NodeKindPackage, Path: "pkg/b"}
	edges := []graph.Edge{
		{From: pkgA.ID(), To: pkgB.ID(), Kind: graph.EdgeKindImports, Language: graph.LangGo},
		{From: pkgB.ID(), To: pkgA.ID(), Kind: graph.EdgeKindImports, Language: graph.LangGo},
	}
	g := metricstest.BuildGraph([]graph.Node{pkgA, pkgB}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != "n/a" {
		t.Errorf("expected n/a for graph with no file nodes, got %q (value=%v)", res.Band, res.Value)
	}
}

// TestFileMutualImport_MutualPair: two TS files that import each other → 1 pair detected.
// Mirrors the codegraph `extraction ↔ resolution` case.
func TestFileMutualImport_MutualPair(t *testing.T) {
	// TypeScript file→file mutual import: extraction.ts <-> resolution.ts
	extraction := graph.Node{Kind: graph.NodeKindFile, Path: "src/extraction/index.ts"}
	resolution := graph.Node{Kind: graph.NodeKindFile, Path: "src/resolution/index.ts"}
	edges := []graph.Edge{
		{From: extraction.ID(), To: resolution.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: resolution.ID(), To: extraction.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}
	g := metricstest.BuildGraph([]graph.Node{extraction, resolution}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != result.BandInformational {
		t.Errorf("expected info band, got %q", res.Band)
	}
	if res.Value != 1 {
		t.Errorf("expected 1 mutual pair, got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, "extraction") || !strings.Contains(res.Display, "resolution") {
		t.Errorf("display must name both files; got %q", res.Display)
	}
}

// TestFileMutualImport_NoPair: A→B→C (DAG, no mutual import) → 0 pairs.
func TestFileMutualImport_NoPair(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindFile, Path: tsFileA}
	b := graph.Node{Kind: graph.NodeKindFile, Path: tsFileB}
	c := graph.Node{Kind: graph.NodeKindFile, Path: "src/c.ts"}
	edges := []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != result.BandInformational {
		t.Errorf("expected info band for zero pairs, got %q", res.Band)
	}
	if res.Value != 0 {
		t.Errorf("expected 0 mutual pairs for DAG, got %v", res.Value)
	}
}

// TestFileMutualImport_ThreeFileCycle: A↔B↔C↔A (3-node SCC) → 3 pairs (all combinations).
func TestFileMutualImport_ThreeFileCycle(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindFile, Path: tsFileA}
	b := graph.Node{Kind: graph.NodeKindFile, Path: tsFileB}
	c := graph.Node{Kind: graph.NodeKindFile, Path: "src/c.ts"}
	edges := []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: c.ID(), To: a.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != result.BandInformational {
		t.Errorf("expected info band, got %q", res.Band)
	}
	// 3-node SCC → C(3,2)=3 pairs
	if res.Value != 3 {
		t.Errorf("expected 3 pairs from 3-node SCC, got %v; display=%q", res.Value, res.Display)
	}
}

// TestFileMutualImport_GoStyleFileToPackage: Go-style file→package edges cannot form
// file↔file cycles → no false positives.
func TestFileMutualImport_GoStyleFileToPackage(t *testing.T) {
	// Go: file nodes import package nodes (different kinds).
	// Even if package A and package B mutually import, file nodes never form a cycle.
	fileA := graph.Node{Kind: graph.NodeKindFile, Path: fcPathA}
	pkgB := graph.Node{Kind: graph.NodeKindPackage, Path: "pkg/b"}
	fileB := graph.Node{Kind: graph.NodeKindFile, Path: "pkg/b/b.go"}
	pkgA := graph.Node{Kind: graph.NodeKindPackage, Path: "pkg/a"}
	edges := []graph.Edge{
		{From: fileA.ID(), To: pkgB.ID(), Kind: graph.EdgeKindImports, Language: graph.LangGo},
		{From: fileB.ID(), To: pkgA.ID(), Kind: graph.EdgeKindImports, Language: graph.LangGo},
	}
	g := metricstest.BuildGraph([]graph.Node{fileA, pkgB, fileB, pkgA}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Value != 0 {
		t.Errorf("Go file→package edges must not produce false positives; got %v pair(s) display=%q", res.Value, res.Display)
	}
}

// TestFileMutualImport_MixedGraph: mixed file+package graph; only file-kind mutual
// imports are counted.
func TestFileMutualImport_MixedGraph(t *testing.T) {
	// Two TS files with mutual import + two Go packages with mutual import.
	// Only the TS pair should be counted.
	tsA := graph.Node{Kind: graph.NodeKindFile, Path: tsFileA}
	tsB := graph.Node{Kind: graph.NodeKindFile, Path: tsFileB}
	goPkgA := graph.Node{Kind: graph.NodeKindPackage, Path: "internal/a"}
	goPkgB := graph.Node{Kind: graph.NodeKindPackage, Path: "internal/b"}
	edges := []graph.Edge{
		{From: tsA.ID(), To: tsB.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: tsB.ID(), To: tsA.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: goPkgA.ID(), To: goPkgB.ID(), Kind: graph.EdgeKindImports, Language: graph.LangGo},
		{From: goPkgB.ID(), To: goPkgA.ID(), Kind: graph.EdgeKindImports, Language: graph.LangGo},
	}
	g := metricstest.BuildGraph([]graph.Node{tsA, tsB, goPkgA, goPkgB}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Value != 1 {
		t.Errorf("expected 1 file mutual pair (TS only), got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, tsFileA) || !strings.Contains(res.Display, tsFileB) {
		t.Errorf("display must name the TS mutual pair; got %q", res.Display)
	}
}

// TestFileMutualImport_Deterministic: multiple runs return the same display order.
func TestFileMutualImport_Deterministic(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindFile, Path: "z/z.ts"}
	b := graph.Node{Kind: graph.NodeKindFile, Path: "a/a.ts"}
	edges := []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: b.ID(), To: a.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}
	g := metricstest.BuildGraph([]graph.Node{a, b}, edges)
	res1 := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	res2 := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res1.Display != res2.Display {
		t.Errorf("non-deterministic display: %q vs %q", res1.Display, res2.Display)
	}
	// Sorted ascending: a/a.ts should appear before z/z.ts
	aIdx := strings.Index(res1.Display, "a/a.ts")
	zIdx := strings.Index(res1.Display, "z/z.ts")
	if aIdx < 0 || zIdx < 0 {
		t.Fatalf("expected both files in display; got %q", res1.Display)
	}
	if aIdx > zIdx {
		t.Errorf("a/a.ts must sort before z/z.ts; got %q", res1.Display)
	}
}
