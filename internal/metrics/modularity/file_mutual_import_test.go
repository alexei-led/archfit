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
	pkgA := graph.Node{Kind: graph.NodeKindPackage, Path: ccModA}
	pkgB := graph.Node{Kind: graph.NodeKindPackage, Path: ccModB}
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
	c := graph.Node{Kind: graph.NodeKindFile, Path: tsFileC}
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

// TestFileMutualImport_ThreeFileCycle: A→B→C→A (pure 3-node cycle) → 0 pairs.
// A pure cycle has no genuinely bidirectional edge (A↔B), so mutual-import count
// is zero. Contrast with TestFileMutualImport_ThreeFileMutualPairs below.
func TestFileMutualImport_ThreeFileCycle(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindFile, Path: tsFileA}
	b := graph.Node{Kind: graph.NodeKindFile, Path: tsFileB}
	c := graph.Node{Kind: graph.NodeKindFile, Path: tsFileC}
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
	// Pure cycle A→B→C→A: no bidirectional pair → 0 mutual imports.
	if res.Value != 0 {
		t.Errorf("expected 0 pairs for pure 3-cycle, got %v; display=%q", res.Value, res.Display)
	}
}

// TestFileMutualImport_ThreeFileMutualPairs: A↔B, B↔C (all three mutually linked)
// with edges in both directions → 2 mutual pairs {A,B} and {B,C}.
func TestFileMutualImport_ThreeFileMutualPairs(t *testing.T) {
	a := graph.Node{Kind: graph.NodeKindFile, Path: tsFileA}
	b := graph.Node{Kind: graph.NodeKindFile, Path: tsFileB}
	c := graph.Node{Kind: graph.NodeKindFile, Path: tsFileC}
	edges := []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: b.ID(), To: a.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: b.ID(), To: c.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
		{From: c.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: graph.LangTypeScript},
	}
	g := metricstest.BuildGraph([]graph.Node{a, b, c}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != result.BandInformational {
		t.Errorf("expected info band, got %q", res.Band)
	}
	// {A,B} and {B,C} are bidirectional → 2 mutual pairs.
	if res.Value != 2 {
		t.Errorf("expected 2 mutual pairs, got %v; display=%q", res.Value, res.Display)
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

// TestFileMutualImport_IsolatedFileNode_ZeroPairs: a file node exists but has no
// edges at all → 0 pairs, Band=Informational (not n/a). The n/a boundary is the
// absence of file-kind nodes, not the absence of edges.
func TestFileMutualImport_IsolatedFileNode_ZeroPairs(t *testing.T) {
	isolated := graph.Node{Kind: graph.NodeKindFile, Path: tsFileA}
	g := metricstest.BuildGraph([]graph.Node{isolated}, nil)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != result.BandInformational {
		t.Errorf("expected info band for isolated file node, got %q", res.Band)
	}
	if res.Value != 0 {
		t.Errorf("expected 0 mutual pairs for isolated node, got %v; display=%q", res.Value, res.Display)
	}
}

// TestFileMutualImport_SelfLoop: A self-loop A→A must NOT count as a mutual pair.
func TestFileMutualImport_SelfLoop(t *testing.T) {
	fileA := graph.Node{Kind: graph.NodeKindFile, Path: "pkg/a/a.ts"}
	pkgA := graph.Node{Kind: graph.NodeKindPackage, Path: "pkg/a"}
	edges := []graph.Edge{
		{From: fileA.ID(), To: fileA.ID(), Kind: graph.EdgeKindImports, Language: "typescript", Confidence: "high"},
	}
	g := metricstest.BuildGraph([]graph.Node{fileA, pkgA}, edges)
	m := modularity.FileMutualImportMetric{}
	res := m.Calculate(signal.CommonInput{Graph: g})
	if res.Value != 0 {
		t.Errorf("self-loop counted as mutual pair: got value=%.0f, want 0", res.Value)
	}
}

// TestFileMutualImport_UsesInternal_MutualPair: EdgeKindUsesInternal file→file mutual
// edges (TS internal-glob rewriting) are detected — not just EdgeKindImports.
func TestFileMutualImport_UsesInternal_MutualPair(t *testing.T) {
	fileA := graph.Node{Kind: graph.NodeKindFile, Path: "src/a/index.ts"}
	fileB := graph.Node{Kind: graph.NodeKindFile, Path: "src/b/index.ts"}
	edges := []graph.Edge{
		{From: fileA.ID(), To: fileB.ID(), Kind: graph.EdgeKindUsesInternal, Language: graph.LangTypeScript},
		{From: fileB.ID(), To: fileA.ID(), Kind: graph.EdgeKindUsesInternal, Language: graph.LangTypeScript},
	}
	g := metricstest.BuildGraph([]graph.Node{fileA, fileB}, edges)
	res := modularity.FileMutualImportMetric{}.Calculate(signal.CommonInput{Graph: g})
	if res.Band != result.BandInformational {
		t.Errorf("expected info band, got %q", res.Band)
	}
	if res.Value != 1 {
		t.Errorf("expected 1 mutual pair for uses_internal bidirectional edges, got %v; display=%q", res.Value, res.Display)
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
