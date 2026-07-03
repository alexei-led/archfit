package rust_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// fixtureDir is the testdata/rust directory; it holds the Cargo.toml marker and
// the cargo-metadata JSON fixtures.
var fixtureDir = filepath.Join("..", "..", "..", "testdata", "rust")

const (
	cargoVersion = "cargo 1.78.0 (54d8815d0 2024-03-26)"
	testTool     = "cargo"
	nodeGrep     = "package:grep"
	nodeRip      = "package:ripgrep"
	nodeIgnore   = "package:ignore"
	statusAbsent = "absent"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return data
}

// mockRunner returns a RunnerMock that reports cargo as present, answers
// `cargo --version`, and returns fixtureData for the metadata call.
func mockRunner(fixtureData []byte) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == testTool {
				return toolrun.ToolInfo{Name: testTool, Path: "/usr/bin/cargo"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte(cargoVersion + "\n")}, nil
			}
			return toolrun.Output{Stdout: fixtureData}, nil
		},
	}
}

func nodeSet(facts graph.Facts) map[string]struct{} {
	set := make(map[string]struct{}, len(facts.Nodes))
	for _, n := range facts.Nodes {
		set[n.ID()] = struct{}{}
	}
	return set
}

func hasEdge(facts graph.Facts, from, to string) *graph.Edge {
	for i := range facts.Edges {
		e := &facts.Edges[i]
		if e.From == from && e.To == to && e.Kind == graph.EdgeKindDependsOn {
			return e
		}
	}
	return nil
}

func TestExtract_Workspace(t *testing.T) {
	runner := mockRunner(loadFixture(t, "cargo_workspace.json"))
	e := rust.New(runner, config.ExtractConfig{Mode: config.ModeAuto})

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Coverage: three workspace members, ok status, version forwarded.
	if cov.Tool != testTool {
		t.Errorf("cov.Tool = %q, want cargo", cov.Tool)
	}
	if cov.FilesSeen != 3 || cov.FilesApplicable != 3 {
		t.Errorf("FilesSeen/FilesApplicable = %d/%d, want 3/3", cov.FilesSeen, cov.FilesApplicable)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if cov.Version != cargoVersion {
		t.Errorf("cov.Version = %q, want %q", cov.Version, cargoVersion)
	}
	if facts.Language != "rust" {
		t.Errorf("facts.Language = %q, want rust", facts.Language)
	}

	// LastCrateRoots mirrors facts.CrateRoots — agenttask's PathResolver reads
	// this (not facts) to resolve a "crate::mod" module key to a real src dir.
	if !slices.Equal(e.LastCrateRoots(), facts.CrateRoots) {
		t.Errorf("LastCrateRoots() = %+v, want facts.CrateRoots %+v", e.LastCrateRoots(), facts.CrateRoots)
	}

	// Nodes: members as package:, registry deps as external:. tempfile is a
	// dev-dep and must NOT appear (IncludeDevDeps defaults to false).
	nodes := nodeSet(facts)
	// pcre2 is an optional (feature-gated) dependency. It is emitted because
	// extraction reads manifest-declared deps, not the feature-resolved graph
	// (cargo metadata --no-deps) — see doc.go. This pins that declared contract.
	wantPresent := []string{
		nodeRip, nodeGrep, nodeIgnore,
		"external:serde", "external:regex", "external:cc", "external:pcre2",
	}
	for _, id := range wantPresent {
		if _, ok := nodes[id]; !ok {
			t.Errorf("missing node %q; nodes=%v", id, facts.Nodes)
		}
	}
	if _, ok := nodes["external:tempfile"]; ok {
		t.Error("dev-dependency tempfile must not appear as a node when IncludeDevDeps is false")
	}

	// Intra-workspace edges resolve to package:; build-dep cc kept; dev-dep dropped.
	wantEdges := [][2]string{
		{nodeRip, nodeGrep},
		{nodeRip, nodeIgnore},
		{nodeRip, "external:serde"},
		{nodeRip, "external:pcre2"},
		{nodeGrep, nodeIgnore},
		{nodeGrep, "external:regex"},
		{nodeGrep, "external:cc"},
	}
	for _, we := range wantEdges {
		edge := hasEdge(facts, we[0], we[1])
		if edge == nil {
			t.Errorf("missing depends_on edge %s -> %s", we[0], we[1])
			continue
		}
		if edge.Language != "rust" {
			t.Errorf("edge %s -> %s: Language = %q, want rust", we[0], we[1], edge.Language)
		}
		if edge.Confidence != "high" {
			t.Errorf("edge %s -> %s: Confidence = %q, want high", we[0], we[1], edge.Confidence)
		}
		if len(edge.Locations) != 1 || edge.Locations[0].File != "Cargo.toml" {
			t.Errorf("edge %s -> %s: Locations = %v, want [{Cargo.toml 0}]", we[0], we[1], edge.Locations)
		}
	}
	if len(facts.Edges) != len(wantEdges) {
		t.Errorf("edge count = %d, want %d; edges=%v", len(facts.Edges), len(wantEdges), facts.Edges)
	}
	if facts.Unresolved != 0 {
		t.Errorf("facts.Unresolved = %d, want 0", facts.Unresolved)
	}

	// The metadata command shape is the contract for downstream cargo invocation.
	gotArgs := lastMetadataArgs(runner)
	for _, want := range []string{"metadata", "--format-version", "1", "--no-deps"} {
		if !slices.Contains(gotArgs, want) {
			t.Errorf("cargo args %v missing %q", gotArgs, want)
		}
	}
}

func TestExtract_IncludeDevDeps(t *testing.T) {
	runner := mockRunner(loadFixture(t, "cargo_workspace.json"))
	e := rust.New(runner, config.ExtractConfig{Mode: config.ModeAuto, IncludeDevDeps: true})

	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if _, ok := nodeSet(facts)["external:tempfile"]; !ok {
		t.Error("dev-dependency tempfile must appear as external node when IncludeDevDeps is true")
	}
	if hasEdge(facts, nodeRip, "external:tempfile") == nil {
		t.Error("expected depends_on edge package:ripgrep -> external:tempfile when IncludeDevDeps is true")
	}
}

func TestExtract_SingleCrate(t *testing.T) {
	runner := mockRunner(loadFixture(t, "cargo_single.json"))
	e := rust.New(runner, config.ExtractConfig{Mode: config.ModeAuto})

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: expected no error for single-crate repo, got %v", err)
	}
	if cov.FilesSeen != 1 {
		t.Errorf("FilesSeen = %d, want 1", cov.FilesSeen)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(facts.Nodes) != 1 || facts.Nodes[0].ID() != "package:just" {
		t.Errorf("nodes = %v, want exactly [package:just]", facts.Nodes)
	}
	if len(facts.Edges) != 0 {
		t.Errorf("edges = %v, want none for a depless single crate", facts.Edges)
	}
}

func TestExtract_FeaturesAndManifest(t *testing.T) {
	runner := mockRunner(loadFixture(t, "cargo_single.json"))
	cfg := config.ExtractConfig{
		Mode:          config.ModeAuto,
		CargoFeatures: []string{"foo", "bar"},
		CargoManifest: "sub/Cargo.toml",
	}
	e := rust.New(runner, cfg)

	if _, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	args := lastMetadataArgs(runner)
	assertArgPair(t, args, "--features", "foo,bar")
	assertArgPair(t, args, "--manifest-path", "sub/Cargo.toml")
}

func TestExtract_ManifestMarkerNoRoot(t *testing.T) {
	// rust_manifest points at a sub-crate manifest while the project root has NO
	// Cargo.toml. The configured manifest must serve as the applicability marker
	// so the extractor still runs (regression: rust_manifest was projected into
	// the cargo args but ignored by the root-Cargo.toml presence check).
	root := t.TempDir()
	sub := filepath.Join(root, "crates", "core")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Cargo.toml"), []byte("[package]\nname = \"core\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := mockRunner(loadFixture(t, "cargo_single.json"))
	cfg := config.ExtractConfig{Mode: config.ModeAuto, CargoManifest: "crates/core/Cargo.toml"}
	if _, _, err := rust.New(runner, cfg).Extract(context.Background(), scope.Scope{Root: root}); err != nil {
		t.Fatalf("Extract: expected applicable via rust_manifest, got %v", err)
	}
	if lastMetadataArgs(runner) == nil {
		t.Error("expected cargo metadata to run when rust_manifest marks an existing sub-manifest")
	}

	// On mode with a configured manifest that does not exist → error, not silent absent.
	missing := config.ExtractConfig{Mode: config.ModeOn, CargoManifest: "crates/missing/Cargo.toml"}
	if _, _, err := rust.New(runner, missing).Extract(context.Background(), scope.Scope{Root: root}); err == nil {
		t.Error("expected error in on mode when configured rust_manifest is absent")
	}
}

func TestExtract_ModeOff(t *testing.T) {
	// RunFunc left nil: it must never be invoked in off mode.
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
	e := rust.New(runner, config.ExtractConfig{Mode: config.ModeOff})

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
		t.Errorf("expected empty facts in off mode, got %d nodes / %d edges", len(facts.Nodes), len(facts.Edges))
	}
	if cov.Status != statusAbsent {
		t.Errorf("cov.Status = %q, want absent", cov.Status)
	}
}

func TestExtract_ToolAbsent(t *testing.T) {
	absent := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}

	t.Run("auto returns absent coverage", func(t *testing.T) {
		e := rust.New(absent, config.ExtractConfig{Mode: config.ModeAuto})
		facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
		if err != nil {
			t.Fatalf("Extract: expected nil error for absent cargo in auto mode, got %v", err)
		}
		if len(facts.Edges) != 0 {
			t.Errorf("expected empty facts, got %d edges", len(facts.Edges))
		}
		if cov.Status != statusAbsent || cov.Tool != testTool {
			t.Errorf("coverage = %+v, want absent/cargo", cov)
		}
	})

	t.Run("on returns error", func(t *testing.T) {
		e := rust.New(absent, config.ExtractConfig{Mode: config.ModeOn})
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir}); err == nil {
			t.Error("expected error for absent cargo in on mode")
		}
	})
}

func TestExtract_NoMarker(t *testing.T) {
	empty := t.TempDir() // no Cargo.toml
	runner := mockRunner(loadFixture(t, "cargo_single.json"))

	t.Run("auto returns absent coverage", func(t *testing.T) {
		e := rust.New(runner, config.ExtractConfig{Mode: config.ModeAuto})
		facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: empty})
		if err != nil {
			t.Fatalf("Extract: expected nil error without Cargo.toml in auto mode, got %v", err)
		}
		if len(facts.Nodes) != 0 {
			t.Errorf("expected empty facts, got %d nodes", len(facts.Nodes))
		}
		if cov.Status != statusAbsent {
			t.Errorf("cov.Status = %q, want absent", cov.Status)
		}
	})

	t.Run("on degrades to absent without a default Cargo.toml", func(t *testing.T) {
		// A missing DEFAULT root Cargo.toml means "not a Rust project here": even in
		// on mode it must degrade to n/a coverage, not exit 3 (E3). An explicitly
		// configured-but-missing rust_manifest still errors (covered elsewhere).
		e := rust.New(runner, config.ExtractConfig{Mode: config.ModeOn})
		facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: empty})
		if err != nil {
			t.Fatalf("expected n/a (not error) without a default Cargo.toml in on mode, got %v", err)
		}
		if len(facts.Nodes) != 0 {
			t.Errorf("expected empty facts, got %d nodes", len(facts.Nodes))
		}
		if cov.Status != statusAbsent {
			t.Errorf("cov.Status = %q, want absent", cov.Status)
		}
	})
}

// TestExtract_NonZeroExit: when cargo metadata exits non-zero (e.g. a malformed
// manifest), auto mode degrades to a partial coverage gap instead of failing the
// whole run; ModeOn still hard-errors so a required analyzer surfaces the problem.
func TestExtract_NonZeroExit(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == testTool
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte(cargoVersion)}, nil
			}
			return toolrun.Output{ExitCode: 101, Stderr: []byte("error: failed to load manifest")}, nil
		},
	}

	t.Run("auto degrades to partial coverage", func(t *testing.T) {
		e := rust.New(runner, config.ExtractConfig{Mode: config.ModeAuto})
		facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
		if err != nil {
			t.Fatalf("auto mode must not error on cargo metadata non-zero exit; got %v", err)
		}
		if cov.Status != "partial" {
			t.Errorf("Status = %q, want %q", cov.Status, "partial")
		}
		if len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
			t.Errorf("expected no nodes/edges on degraded run; got %d nodes, %d edges", len(facts.Nodes), len(facts.Edges))
		}
	})

	t.Run("on hard-errors", func(t *testing.T) {
		e := rust.New(runner, config.ExtractConfig{Mode: config.ModeOn})
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir}); err == nil {
			t.Error("ModeOn must hard-error on cargo metadata non-zero exit")
		}
	})
}

// lastMetadataArgs returns the args of the most recent non-version cargo call.
func lastMetadataArgs(runner *toolrun.RunnerMock) []string {
	for _, call := range runner.RunCalls() {
		if !slices.Contains(call.Cmd.Args, "--version") {
			return call.Cmd.Args
		}
	}
	return nil
}

func assertArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, a := range args {
		if a == flag {
			if i+1 >= len(args) || args[i+1] != value {
				t.Errorf("flag %q: got value %q, want %q (args=%v)", flag, argAt(args, i+1), value, args)
			}
			return
		}
	}
	t.Errorf("flag %q not found in args %v", flag, args)
}

func argAt(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}
