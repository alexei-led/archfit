package acquisition

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	evidencecontract "github.com/alexei-led/archfit/v3/internal/evidence"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/syntax"
)

func TestDeclaredOutOfScopeHonorsExclusionsAndSwitchedOffLanguages(t *testing.T) {
	const (
		langTypeScript = "typescript"
		langPython     = "python"
		toolScript     = "tools/gen.mjs"
		pyHelper       = "scripts/helper.py"
		tsSource       = "web/app.ts"
		modeOff        = "off"
	)
	f := evidencecontract.Facts{
		FileLOC: map[string]int{"internal/a.go": 10, toolScript: 3, pyHelper: 4, tsSource: 5},
		FileClassIndex: map[string]fileclass.FileClass{
			"internal/a_test.go": fileclass.Test, toolScript: fileclass.Production,
		},
	}
	for _, tc := range []struct {
		name       string
		exclusions []string
		cov        CoverageOptions
		want       map[string]struct{}
	}{
		{name: "nothing declared out of scope"},
		{
			name:       "exclude glob",
			exclusions: []string{"tools/**", "**/testdata/**"},
			want:       map[string]struct{}{toolScript: {}},
		},
		{
			name: "switched-off language",
			cov:  CoverageOptions{Modes: map[string]string{langPython: modeOff}},
			want: map[string]struct{}{pyHelper: {}},
		},
		{
			// An explicit gate on a switched-off language asks to be told the
			// producer did not run, so its sources stay in rule scope.
			name: "switched-off language with an explicit gate",
			cov: CoverageOptions{
				Modes: map[string]string{langTypeScript: modeOff},
				Gates: map[string]string{langTypeScript: gateWarn},
			},
		},
		{
			name:       "both declarations",
			exclusions: []string{"tools/**"},
			cov:        CoverageOptions{Modes: map[string]string{langPython: modeOff, langTypeScript: modeOff}},
			want:       map[string]struct{}{toolScript: {}, pyHelper: {}, tsSource: {}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := declaredOutOfScope(f, tc.exclusions, tc.cov)
			if !maps.Equal(got, tc.want) {
				t.Fatalf("declaredOutOfScope = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUnanalysedFilesFollowTheExtractorsOwnApplicability pins the rule-scope
// files no dependency producer analyses. A language counts as absent from the
// tree exactly when its primary coverage row is gapless absent: the
// extractor's own probe finds no project under root and no explicit gate asked
// to be told. A Rust file outside every cargo workspace member is never
// analysed once cargo metadata named the members.
func TestUnanalysedFilesFollowTheExtractorsOwnApplicability(t *testing.T) {
	const (
		root    = "/repo"
		goFile  = "cmd/main.go"
		tsFile  = "web/ui/src/app.ts"
		pyFile  = "scripts/helper.py"
		crateRs = "crates/engine/src/lib.rs"
		fuzzRs  = "fuzz/fuzz_targets/parse.rs"
		langTS  = "typescript"
	)
	probes := func(absent ...string) map[string]func(string) bool {
		out := map[string]func(string) bool{}
		for _, tool := range []string{registry.ToolGoPackages, registry.ToolDepCruiser, registry.ToolGrimp, registry.ToolCargo} {
			out[tool] = func(string) bool { return true }
		}
		for _, tool := range absent {
			out[tool] = func(string) bool { return false }
		}
		return out
	}
	members := graph.Build([]graph.Facts{{Language: graph.LangRust, CrateRoots: []graph.CrateRoot{{Dir: "crates/engine", Name: "engine"}}}})
	for _, tc := range []struct {
		name  string
		root  string
		graph *graph.Graph
		cov   CoverageOptions
		out   map[string]struct{}
		want  map[string]struct{}
	}{
		{name: "every language present", root: root, cov: CoverageOptions{ProjectPresent: probes()}},
		{name: "typescript project absent", root: root, cov: CoverageOptions{ProjectPresent: probes(registry.ToolDepCruiser)},
			want: map[string]struct{}{tsFile: {}}},
		{name: "explicit gate demands the absent language", root: root,
			cov: CoverageOptions{ProjectPresent: probes(registry.ToolDepCruiser), Gates: map[string]string{langTS: gateWarn}}},
		{name: "gate off demands nothing", root: root,
			cov:  CoverageOptions{ProjectPresent: probes(registry.ToolDepCruiser), Gates: map[string]string{langTS: gateOff}},
			want: map[string]struct{}{tsFile: {}}},
		{name: "go without a module and python without a project", root: root,
			cov:  CoverageOptions{ProjectPresent: probes(registry.ToolGoPackages, registry.ToolGrimp)},
			want: map[string]struct{}{goFile: {}, pyFile: {}}},
		{name: "already declared out of scope", root: root, cov: CoverageOptions{ProjectPresent: probes(registry.ToolDepCruiser)},
			out: map[string]struct{}{tsFile: {}}},
		{name: "unprobeable root", cov: CoverageOptions{ProjectPresent: probes(registry.ToolDepCruiser)}},
		{name: "rust file outside every workspace member", root: root, graph: members,
			cov: CoverageOptions{ProjectPresent: probes()}, want: map[string]struct{}{fuzzRs: {}}},
		{name: "rust members unknown without cargo metadata", root: root, cov: CoverageOptions{ProjectPresent: probes()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := evidencecontract.Facts{
				FileLOC:        map[string]int{goFile: 1, tsFile: 1, crateRs: 1, fuzzRs: 1},
				FileClassIndex: map[string]fileclass.FileClass{pyFile: fileclass.Production},
				Graph:          tc.graph,
			}
			if got := unanalysedFiles(tc.root, f, tc.out, tc.cov, goMemberSplit{}); !maps.Equal(got, tc.want) {
				t.Fatalf("unanalysedFiles = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUnanalysedFilesSkipGoMembersTheModuleFilterRemoves pins Go applicability
// under languages.go.modules: the extractor never loads a member the filter
// removes, so its files are unanalysed, while a kept member nested inside a
// removed one, and a file in no member, are not.
func TestUnanalysedFilesSkipGoMembersTheModuleFilterRemoves(t *testing.T) {
	split := goMemberSplit{kept: []string{"a", "b/keep"}, removed: []string{".", "b"}}
	f := evidencecontract.Facts{FileLOC: map[string]int{
		"a/x.go": 1, "b/y.go": 1, "b/keep/z.go": 1, "root.go": 1, "web/app.ts": 1,
	}}
	cov := CoverageOptions{ProjectPresent: map[string]func(string) bool{
		registry.ToolGoPackages: func(string) bool { return true },
		registry.ToolDepCruiser: func(string) bool { return true },
	}}
	want := map[string]struct{}{"b/y.go": {}, "root.go": {}}
	if got := unanalysedFiles("/repo", f, nil, cov, split); !maps.Equal(got, want) {
		t.Fatalf("unanalysedFiles = %v, want %v", got, want)
	}
	if got := unanalysedFiles("/repo", f, nil, cov, goMemberSplit{}); len(got) != 0 {
		t.Fatalf("unanalysedFiles without a filter = %v, want none", got)
	}
}

// TestUnwalkedSourceProductionClassifiesSkippedEdgeSources pins the class of
// the dependency-edge source files the LOC walk never visited (it skips dot
// directories and names such as target/ that an analyzer still loads): a
// path-only FileClass with the configured globs, and never production when
// the configuration declared the file out of scope.
func TestUnwalkedSourceProductionClassifiesSkippedEdgeSources(t *testing.T) {
	const (
		walked     = "code/src/a.ts"
		storybook  = "code/.storybook/preview.tsx"
		targetGo   = "internal/target/x.go"
		targetTest = "internal/target/x_test.go"
		excluded   = "legacy/old.go"
	)
	g := graph.Build([]graph.Facts{{Edges: []graph.Edge{
		{From: "file:" + walked, To: "file:code/src/b.ts", Kind: graph.EdgeKindImports},
		{From: "file:" + storybook, To: "file:code/src/b.ts", Kind: graph.EdgeKindImports},
		{From: "file:" + targetGo, To: "package:internal/a", Kind: graph.EdgeKindImports,
			Locations: []graph.Location{{File: targetGo, Line: 3}, {File: targetTest, Line: 4}}},
		{From: "file:" + excluded, To: "package:internal/a", Kind: graph.EdgeKindImports},
		{From: "package:core", To: "package:util", Kind: graph.EdgeKindDependsOn, Locations: []graph.Location{{File: "Cargo.toml"}}},
	}}})
	f := evidencecontract.Facts{Graph: g, FileClassIndex: map[string]fileclass.FileClass{walked: fileclass.Production}}
	for _, tc := range []struct {
		name  string
		globs []string
		want  map[string]bool
	}{
		{name: "default classes", want: map[string]bool{storybook: true, targetGo: true, targetTest: false, excluded: false}},
		{name: "test_globs classify dev tooling", globs: []string{"**/.storybook/**"},
			want: map[string]bool{storybook: false, targetGo: true, targetTest: false, excluded: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := unwalkedSourceProduction(f, []string{"legacy/**"}, CoverageOptions{}, syntax.FileClassConfig{TestGlobs: tc.globs})
			if !maps.Equal(got, tc.want) {
				t.Fatalf("unwalkedSourceProduction = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRustModuleNodesListTheModuleGraph pins the crate::mod node IDs rule
// scope judges Rust module selectors against: cargo-modules nodes only, never
// a crate node or another language's node.
func TestRustModuleNodesListTheModuleGraph(t *testing.T) {
	g := graph.Build([]graph.Facts{
		{Language: graph.LangRust, Nodes: []graph.Node{
			{Kind: graph.NodeKindPackage, Path: "engine", Language: graph.LangRust},
			{Kind: graph.NodeKindPackage, Path: "engine::types::narrow", Language: graph.LangRust},
			{Kind: graph.NodeKindPackage, Path: "engine::place", Language: graph.LangRust},
			{Kind: graph.NodeKindExternal, Path: "serde", Language: graph.LangRust},
		}},
		{Language: "python", Nodes: []graph.Node{{Kind: graph.NodeKindModule, Path: "app.core", Language: "python"}}},
	})
	got := rustModuleNodes(evidencecontract.Facts{Graph: g})
	if want := []string{"engine::place", "engine::types::narrow"}; !slices.Equal(got, want) {
		t.Fatalf("rustModuleNodes = %v, want %v", got, want)
	}
	if got := rustModuleNodes(evidencecontract.Facts{}); got != nil {
		t.Fatalf("rustModuleNodes without a graph = %v, want nil", got)
	}
}

// TestRustCratesNameBothSpellings pins the loaded crate identities rule scope
// judges crate::mod selectors against: the crate identifier (a binary target's
// own name) and the library spelling of the package name.
func TestRustCratesNameBothSpellings(t *testing.T) {
	g := graph.Build([]graph.Facts{{Language: graph.LangRust,
		CrateRoots: []graph.CrateRoot{{Dir: crateFM, Name: crateFM, Crate: crateYazi}}}})
	got := rustCrates(evidencecontract.Facts{Graph: g})
	if want := []string{crateYazi, "yazi_fm"}; !slices.Equal(got, want) {
		t.Fatalf("rustCrates = %v, want %v", got, want)
	}
	if got := rustCrates(evidencecontract.Facts{}); got != nil {
		t.Fatalf("rustCrates without a graph = %v, want nil", got)
	}
}

// TestAssessmentObservationsCarryRustCrateIdentities pins that the Rust crate
// identities rule scope reads survive the narrowing to assessment observations.
func TestAssessmentObservationsCarryRustCrateIdentities(t *testing.T) {
	inventory := evaluation.Observations{RustCrates: []string{crateYazi}, RustModuleGraphCrates: []string{crateYazi}}
	got := assessmentObservationsOf(evidencecontract.Facts{}, inventory, nil, nil, nil, nil)
	if !slices.Equal(got.RustCrates, inventory.RustCrates) || !slices.Equal(got.RustModuleGraphCrates, inventory.RustModuleGraphCrates) {
		t.Fatalf("RustCrates = %v, RustModuleGraphCrates = %v, want both %v", got.RustCrates, got.RustModuleGraphCrates, []string{crateYazi})
	}
}

const (
	crateTool    = "tool"
	crateToolCLI = "tool_cli"
)

// TestRustCratesNameEveryLoadedTarget pins that a binary target of a lib+bin
// package (package tool, lib tool, bin tool-cli) is a loaded crate: crate::mod
// selectors under tool_cli are undecidable without its module graph, never
// definitely absent. A lib-only package keeps its existing identities.
func TestRustCratesNameEveryLoadedTarget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		targets []string
		want    []string
	}{
		{name: "lib and bin", targets: []string{crateTool, crateToolCLI}, want: []string{crateTool, crateToolCLI}},
		{name: "lib only", targets: []string{crateTool}, want: []string{crateTool}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := graph.Build([]graph.Facts{{Language: graph.LangRust,
				CrateRoots: []graph.CrateRoot{{Name: crateTool, Crate: crateTool}}}})
			f := evidencecontract.Facts{Graph: g, RustTargetCrates: tc.targets}
			got := ruleScopeObservations(context.Background(), nil, nil, t.TempDir(), f, RunOptions{}).RustCrates
			if !slices.Equal(got, tc.want) {
				t.Fatalf("RustCrates = %v, want %v", got, tc.want)
			}
		})
	}
}
