package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	launcherBunx     = "bunx"
	launcherBunxPath = "/usr/bin/bunx"
	tsconfigName     = "tsconfig.json"
	modeFull         = "full"
)

// fixtureDir is the testdata/ts directory with package.json and JSON fixtures.
var fixtureDir = filepath.Join("..", "..", "..", "testdata", "ts")

// loadFixture reads a JSON fixture from testdata/ts.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return data
}

// mockRunner builds a RunnerMock that:
//   - DetectFunc: returns (ToolInfo{Name: tool}, true) for bunx; false for anything else.
//   - RunFunc: when args contain "--version" returns "14.0.0"; otherwise returns fixtureData.
func mockRunner(fixtureData []byte) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == launcherBunx {
				return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("14.0.0\n")}, nil
			}
			return toolrun.Output{Stdout: fixtureData}, nil
		},
	}
}

func TestExtract_Parse(t *testing.T) {
	data := loadFixture(t, "depcruise_output.json")
	runner := mockRunner(data)

	cfg := config.ExtractConfig{
		Mode:     config.ModeAuto,
		Internal: []string{"src/b/internal/**"},
	}
	extractor := ts.New(runner, cfg)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := extractor.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Coverage checks.
	if cov.FilesSeen != 2 {
		t.Errorf("FilesSeen = %d, want 2", cov.FilesSeen)
	}
	if cov.Unresolved != 0 {
		t.Errorf("Unresolved = %d, want 0", cov.Unresolved)
	}
	if cov.Status != "ok" {
		t.Errorf("Status = %q, want %q", cov.Status, "ok")
	}
	if cov.Version != "14.0.0" {
		t.Errorf("Version = %q, want %q", cov.Version, "14.0.0")
	}

	// Edge assertions.
	findEdge := func(from, to string, kind graph.EdgeKind) *graph.Edge {
		for i := range facts.Edges {
			e := &facts.Edges[i]
			if e.From == from && e.To == to && e.Kind == kind {
				return e
			}
		}
		return nil
	}

	// a.ts → b/index.ts: imports
	e1 := findEdge("file:src/a.ts", "file:src/b/index.ts", graph.EdgeKindImports)
	if e1 == nil {
		t.Error("expected edge file:src/a.ts → file:src/b/index.ts (imports), not found")
	}

	// a.ts → b/internal/impl.ts: uses_internal (matches internal glob)
	e2 := findEdge("file:src/a.ts", "file:src/b/internal/impl.ts", graph.EdgeKindUsesInternal)
	if e2 == nil {
		t.Error("expected edge file:src/a.ts → file:src/b/internal/impl.ts (uses_internal), not found")
	}

	// fs (coreModule=true) must NOT appear as an edge.
	for _, e := range facts.Edges {
		if e.To == "file:fs" {
			t.Errorf("core module edge should not be emitted: %+v", e)
		}
	}
}

func TestExtract_CouldNotResolve(t *testing.T) {
	data := loadFixture(t, "depcruise_unresolved.json")
	runner := mockRunner(data)

	cfg := config.ExtractConfig{Mode: config.ModeAuto}
	extractor := ts.New(runner, cfg)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := extractor.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if cov.Unresolved != 1 {
		t.Errorf("Unresolved = %d, want 1", cov.Unresolved)
	}
	if cov.Status != "partial" {
		t.Errorf("Status = %q, want %q", cov.Status, "partial")
	}
	if facts.Unresolved != 1 {
		t.Errorf("facts.Unresolved = %d, want 1", facts.Unresolved)
	}

	// The unresolved edge must still be emitted with low confidence, pointing at
	// an external node (not a first-party file).
	if len(facts.Edges) == 0 {
		t.Fatal("expected at least one edge for couldNotResolve case")
	}
	edge := facts.Edges[0]
	if edge.Confidence != "low" {
		t.Errorf("edge.Confidence = %q, want %q", edge.Confidence, "low")
	}
	if edge.To != "external:src/missing.ts" {
		t.Errorf("edge.To = %q, want %q (unresolved target marked external)", edge.To, "external:src/missing.ts")
	}
}

// TestExtract_ExternalNodes mirrors the codegraph case: a CLI importing the
// uninstalled npm package "commander" (couldNotResolve) and the node builtin
// "fs" (coreModule), with no node_modules. dependency-cruiser lists both as
// their own module.source entries (not just as dependencies), so the fixture
// includes those source entries. The unresolved package must become an external
// node — never a first-party file: node that pollutes martin metrics — and the
// core module must be dropped entirely, whether it appears as a dependency or as
// a source.
func TestExtract_ExternalNodes(t *testing.T) {
	data := loadFixture(t, "depcruise_external.json")
	runner := mockRunner(data)

	cfg := config.ExtractConfig{Mode: config.ModeAuto}
	extractor := ts.New(runner, cfg)

	facts, cov, err := extractor.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// commander appears both as a module entry and as a dependency edge; it must
	// be counted once, not twice (no double-count of the same unresolved package).
	if cov.Unresolved != 1 {
		t.Errorf("cov.Unresolved = %d, want 1 (commander counted once)", cov.Unresolved)
	}
	// FilesSeen counts first-party source files only: src/cli.ts and
	// src/commands/index.ts — not the unresolved (commander) or core (fs) entries.
	if cov.FilesSeen != 2 {
		t.Errorf("cov.FilesSeen = %d, want 2 (first-party files only, excludes core/unresolved)", cov.FilesSeen)
	}
	if facts.Unresolved != 1 {
		t.Errorf("facts.Unresolved = %d, want 1 (commander counted once)", facts.Unresolved)
	}

	hasNode := func(id string) bool {
		for _, n := range facts.Nodes {
			if n.ID() == id {
				return true
			}
		}
		return false
	}

	// commander is unresolved → external node, NOT a first-party file: node.
	if hasNode("file:commander") {
		t.Error("unresolved npm package must not appear as a first-party file: node")
	}
	if !hasNode("external:commander") {
		t.Error("expected external:commander node for the unresolved npm package")
	}
	// fs is a core module → dropped entirely (no node, no edge).
	if hasNode("file:fs") || hasNode("external:fs") {
		t.Error("core module fs must not appear as a node")
	}
	// First-party files keep their file: nodes.
	if !hasNode("file:src/cli.ts") || !hasNode("file:src/commands/index.ts") {
		t.Error("first-party source files must keep file: nodes")
	}
	// Exactly the two first-party files plus external:commander — the builtin/
	// unresolved source entries must not add any first-party module nodes.
	if len(facts.Nodes) != 3 {
		t.Errorf("node count = %d, want 3 (2 first-party files + external:commander); nodes=%v", len(facts.Nodes), facts.Nodes)
	}

	// The edge to commander is kept (fan-out stays complete) and points external.
	var found bool
	for _, e := range facts.Edges {
		if e.To == "external:commander" {
			found = true
			if e.From != "file:src/cli.ts" {
				t.Errorf("commander edge From = %q, want file:src/cli.ts", e.From)
			}
			if e.Confidence != "low" {
				t.Errorf("commander edge Confidence = %q, want low", e.Confidence)
			}
		}
		if e.To == "file:fs" || e.To == "external:fs" {
			t.Errorf("core module fs must not produce an edge: %+v", e)
		}
	}
	if !found {
		t.Error("expected edge to external:commander")
	}
}

// TestExtract_EdgeTypes asserts dependency-cruiser dependencyTypes drive the
// Balanced Coupling integration-strength hint: a type-only (`import type`) edge
// shares only the type shape and vanishes at runtime → Contract (weakest); a
// value/runtime import and a dynamic `import()` both bind to exported
// names/signatures → Functional.
func TestExtract_EdgeTypes(t *testing.T) {
	data := loadFixture(t, "depcruise_edgetypes.json")
	runner := mockRunner(data)

	extractor := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto})

	facts, _, err := extractor.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	hintFor := func(to string) string {
		for _, e := range facts.Edges {
			if e.From == "file:src/a.ts" && e.To == to {
				return e.StrengthHint
			}
		}
		t.Fatalf("edge file:src/a.ts → %s not found", to)
		return ""
	}

	cases := []struct {
		name string
		to   string
		want string
	}{
		{"type-only import → contract", "file:src/types.ts", "contract"},
		{"value import → functional", "file:src/b/index.ts", "functional"},
		{"dynamic import → functional", "file:src/lazy.ts", "functional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hintFor(tc.to); got != tc.want {
				t.Errorf("StrengthHint(%s) = %q, want %q", tc.to, got, tc.want)
			}
		})
	}
}

func TestExtract_ToolAbsentAuto(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		// RunFunc must not be called; leave nil to panic if it is.
	}

	cfg := config.ExtractConfig{Mode: config.ModeAuto}
	extractor := ts.New(runner, cfg)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := extractor.Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: expected nil error for absent tool in auto mode, got %v", err)
	}
	if len(facts.Edges) != 0 {
		t.Errorf("expected empty facts, got %d edges", len(facts.Edges))
	}
	if cov.Status != "absent" {
		t.Errorf("Coverage.Status = %q, want %q", cov.Status, "absent")
	}
	if cov.Tool != "dependency-cruiser" {
		t.Errorf("Coverage.Tool = %q, want %q", cov.Tool, "dependency-cruiser")
	}
}

// TestExtract_TSConfigAutoDetect verifies dependency-cruiser is passed --ts-config
// so tsconfig path aliases resolve: an explicit config wins, otherwise a tsconfig
// at the root is auto-detected, and nothing is passed when none is configured or
// present. Without it, aliased imports come back unresolved and are dropped from
// the coupling metrics.
// TestExtract_IncludeOnly verifies that --include-only ^<SubtreePrefix> is added
// to depcruise args when ScanRoot is a subtree (SubtreePrefix != ""), and is
// omitted entirely when SubtreePrefix is empty (byte-identical no-subtree path).
func TestExtract_IncludeOnly(t *testing.T) {
	tests := []struct {
		name            string
		subtreePrefix   string
		wantIncludeOnly bool
		wantPattern     string
	}{
		{
			name:            "subtree: --include-only added",
			subtreePrefix:   "services/api",
			wantIncludeOnly: true,
			wantPattern:     "^services/api(?:/|$)",
		},
		{
			name:            "root: --include-only omitted (byte-identical)",
			subtreePrefix:   "",
			wantIncludeOnly: false,
		},
		{
			name:            "subtree with metachar: dot is escaped",
			subtreePrefix:   "packages/my.app",
			wantIncludeOnly: true,
			wantPattern:     "^packages/my\\.app(?:/|$)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"t"}`), 0o600); err != nil {
				t.Fatal(err)
			}

			var gotArgs []string
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
					if tool == launcherBunx {
						return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
					}
					return toolrun.ToolInfo{}, false
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if slices.Contains(cmd.Args, "--version") {
						return toolrun.Output{Stdout: []byte("14.0.0\n")}, nil
					}
					gotArgs = cmd.Args
					return toolrun.Output{Stdout: []byte(`{"modules":[]}`)}, nil
				},
			}

			extractor := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto})
			s := scope.Scope{
				Root:          root,
				GitRoot:       filepath.Dir(root),
				SubtreePrefix: tt.subtreePrefix,
				Mode:          modeFull,
			}
			if _, _, err := extractor.Extract(context.Background(), s); err != nil {
				t.Fatalf("Extract: %v", err)
			}

			idx := slices.Index(gotArgs, "--include-only")
			if !tt.wantIncludeOnly {
				if idx != -1 {
					t.Fatalf("expected no --include-only flag, got args %v", gotArgs)
				}
				return
			}
			if idx == -1 || idx+1 >= len(gotArgs) {
				t.Fatalf("expected --include-only flag, got args %v", gotArgs)
			}
			got := gotArgs[idx+1]
			if got != tt.wantPattern {
				t.Errorf("--include-only = %q, want %q", got, tt.wantPattern)
			}
			// Verify the boundary: the compiled pattern must match the prefix dir
			// itself and a file inside it, but NOT a sibling with a longer name.
			if tt.subtreePrefix != "" {
				re, err := regexp.Compile(got)
				if err != nil {
					t.Fatalf("--include-only pattern %q is not a valid regexp: %v", got, err)
				}
				prefix := filepath.ToSlash(tt.subtreePrefix)
				if !re.MatchString(prefix + "/src/index.ts") {
					t.Errorf("pattern %q should match %q but does not", got, prefix+"/src/index.ts")
				}
				if re.MatchString(prefix + "-client/src/index.ts") {
					t.Errorf("pattern %q should NOT match sibling %q but does", got, prefix+"-client/src/index.ts")
				}
			}
			// --exclude ^node_modules must always be present.
			exIdx := slices.Index(gotArgs, "--exclude")
			if exIdx == -1 || exIdx+1 >= len(gotArgs) || gotArgs[exIdx+1] != "^node_modules" {
				t.Errorf("expected --exclude ^node_modules in args %v", gotArgs)
			}
		})
	}
}

// TestExtract_SubtreePathStrip verifies that parseAndNormalize strips the
// SubtreePrefix from node IDs and edge paths in subtree mode, so downstream
// classify and matchesInternal work against ScanRoot-relative config globs.
// The byte-identical invariant (no-op when SubtreePrefix=="") is also checked.
func TestExtract_SubtreePathStrip(t *testing.T) {
	// depcruise JSON with git-root-relative paths (subtree prefix "packages/api").
	const prefix = "packages/api"
	depcruiseJSON := `{"modules":[
		{"source":"packages/api/src/index.ts","dependencies":[
			{"resolved":"packages/api/src/util.ts","module":"./util","dependencyTypes":["local"]},
			{"resolved":"react","module":"react","dependencyTypes":["npm"],"couldNotResolve":true}
		]},
		{"source":"packages/api/src/util.ts","dependencies":[]}
	]}`

	tests := []struct {
		name          string
		subtreePrefix string
		wantFromPath  string // expected node path for the first file
		wantToPath    string // expected node path for the internal dep
		wantEdgeKind  graph.EdgeKind
	}{
		{
			name:          "subtree mode: prefix stripped",
			subtreePrefix: prefix,
			wantFromPath:  "src/index.ts",
			wantToPath:    "src/util.ts",
			wantEdgeKind:  graph.EdgeKindUsesInternal,
		},
		{
			name:          "non-subtree mode: paths unchanged (byte-identical)",
			subtreePrefix: "",
			wantFromPath:  "packages/api/src/index.ts",
			wantToPath:    "packages/api/src/util.ts",
			wantEdgeKind:  graph.EdgeKindImports, // matchesInternal won't match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configure internal globs as ScanRoot-relative: "src/**"
			cfg := config.ExtractConfig{
				Mode:     config.ModeAuto,
				Internal: []string{"src/**"},
			}
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
					if tool == launcherBunx {
						return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
					}
					return toolrun.ToolInfo{}, false
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if slices.Contains(cmd.Args, "--version") {
						return toolrun.Output{Stdout: []byte("14.0.0\n")}, nil
					}
					return toolrun.Output{Stdout: []byte(depcruiseJSON)}, nil
				},
			}

			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"t"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			extractor := ts.New(runner, cfg)
			s := scope.Scope{
				Root:          root,
				GitRoot:       filepath.Dir(root),
				SubtreePrefix: tt.subtreePrefix,
				Mode:          modeFull,
			}
			facts, _, err := extractor.Extract(context.Background(), s)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}

			// Find the file node for index.ts.
			var foundFrom bool
			for _, n := range facts.Nodes {
				if n.Path == tt.wantFromPath {
					foundFrom = true
					break
				}
			}
			if !foundFrom {
				var paths []string
				for _, n := range facts.Nodes {
					paths = append(paths, string(n.Kind)+":"+n.Path)
				}
				t.Errorf("node %q not found; got nodes: %v", tt.wantFromPath, paths)
			}

			// Find the internal edge (index.ts → util.ts).
			var foundEdge bool
			for _, e := range facts.Edges {
				if e.To == "file:"+tt.wantToPath {
					foundEdge = true
					if e.Kind != tt.wantEdgeKind {
						t.Errorf("edge kind = %v, want %v", e.Kind, tt.wantEdgeKind)
					}
					break
				}
			}
			if !foundEdge {
				var tos []string
				for _, e := range facts.Edges {
					tos = append(tos, string(e.Kind)+":"+e.To)
				}
				t.Errorf("edge to %q not found; got edges: %v", "file:"+tt.wantToPath, tos)
			}
		})
	}
}

// TestExtract_TSConfigSubdir is a regression test confirming that resolveTSConfig
// finds the package-local tsconfig when ScanRoot is a subdirectory of the git root
// (Task 4 made s.Root the ScanRoot). The path returned must be relative to the
// depcruise workDir (gitRoot for the subtree case) so dependency-cruiser resolves
// it correctly.
func TestExtract_TSConfigSubdir(t *testing.T) {
	gitRoot := t.TempDir()
	pkg := filepath.Join(gitRoot, "packages", "pkg-a")
	if err := os.MkdirAll(pkg, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"pkg-a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, tsconfigName), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "bunx" {
				return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("14.0.0\n")}, nil
			}
			gotArgs = cmd.Args
			return toolrun.Output{Stdout: []byte(`{"modules":[]}`)}, nil
		},
	}

	extractor := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto})
	s := scope.Scope{
		Root:          pkg,
		GitRoot:       gitRoot,
		SubtreePrefix: "packages/pkg-a",
		Mode:          modeFull,
	}
	if _, _, err := extractor.Extract(context.Background(), s); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	idx := slices.Index(gotArgs, "--ts-config")
	if idx == -1 || idx+1 >= len(gotArgs) {
		t.Fatalf("expected --ts-config in args %v", gotArgs)
	}
	// Path must be relative to gitRoot (workDir), not to pkg.
	want := "packages/pkg-a/tsconfig.json"
	if got := gotArgs[idx+1]; got != want {
		t.Errorf("--ts-config = %q, want %q", got, want)
	}
}

func TestExtract_TSConfigAutoDetect(t *testing.T) {
	tests := []struct {
		name      string
		tsconfig  string // file created at root ("" = none)
		cfgTS     string // explicit config.TSConfig ("" = unset)
		wantTSArg string // expected --ts-config value ("" = flag absent)
	}{
		{name: "auto-detect tsconfig.json", tsconfig: tsconfigName, wantTSArg: tsconfigName},
		{name: "auto-detect tsconfig.base.json", tsconfig: "tsconfig.base.json", wantTSArg: "tsconfig.base.json"},
		{name: "no tsconfig present", tsconfig: "", wantTSArg: ""},
		{name: "explicit config wins", tsconfig: tsconfigName, cfgTS: "custom/tsconfig.json", wantTSArg: "custom/tsconfig.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"t"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if tt.tsconfig != "" {
				if err := os.WriteFile(filepath.Join(root, tt.tsconfig), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			var gotArgs []string
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
					if tool == launcherBunx {
						return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
					}
					return toolrun.ToolInfo{}, false
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if slices.Contains(cmd.Args, "--version") {
						return toolrun.Output{Stdout: []byte("14.0.0\n")}, nil
					}
					gotArgs = cmd.Args
					return toolrun.Output{Stdout: []byte(`{"modules":[]}`)}, nil
				},
			}

			extractor := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto, TSConfig: tt.cfgTS})
			if _, _, err := extractor.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
				t.Fatalf("Extract: %v", err)
			}

			idx := slices.Index(gotArgs, "--ts-config")
			if tt.wantTSArg == "" {
				if idx != -1 {
					t.Fatalf("expected no --ts-config flag, got args %v", gotArgs)
				}
				return
			}
			if idx == -1 || idx+1 >= len(gotArgs) {
				t.Fatalf("expected --ts-config %q, got args %v", tt.wantTSArg, gotArgs)
			}
			if got := gotArgs[idx+1]; got != tt.wantTSArg {
				t.Errorf("--ts-config = %q, want %q", got, tt.wantTSArg)
			}
		})
	}
}

// TestExtract_SourceDirMissing is the bug-3 regression: when dependency-cruiser
// exits non-zero because the source dir does not exist ("Can't open 'src' for
// reading", as on an npm-workspaces monorepo with no top-level src/), auto mode
// degrades to a partial coverage gap instead of failing the whole run; ModeOn
// still hard-errors so a required analyzer surfaces the problem.
func TestExtract_SourceDirMissing(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == launcherBunx {
				return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("16.0.0\n")}, nil
			}
			return toolrun.Output{ExitCode: 1, Stderr: []byte("ERROR: Can't open 'src' for reading. Does it exist?")}, nil
		},
	}
	s := scope.Scope{Root: fixtureDir}

	t.Run("auto degrades to partial coverage", func(t *testing.T) {
		facts, cov, err := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto}).Extract(context.Background(), s)
		if err != nil {
			t.Fatalf("auto mode must not error on depcruise non-zero exit; got %v", err)
		}
		if cov.Status != "partial" {
			t.Errorf("Status = %q, want %q", cov.Status, "partial")
		}
		if len(facts.Edges) != 0 {
			t.Errorf("expected no edges on degraded run; got %d", len(facts.Edges))
		}
	})

	t.Run("on hard-errors", func(t *testing.T) {
		_, _, err := ts.New(runner, config.ExtractConfig{Mode: config.ModeOn}).Extract(context.Background(), s)
		if err == nil {
			t.Error("ModeOn must hard-error on depcruise non-zero exit")
		}
	})
}

// TestExtract_SrcDotIsLiteral locks Config.ForExtract's Src="." default (the
// analysis root itself) to depcruise's actual scan argument, instead of being
// silently rewritten to "src" — a repo whose TS sources live directly under
// the root with no src/ subdir (e.g. storybookjs/storybook's "code/" layout)
// would otherwise scan a directory that does not exist.
func TestExtract_SrcDotIsLiteral(t *testing.T) {
	var gotArgs []string
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == launcherBunx {
				return toolrun.ToolInfo{Name: launcherBunx, Path: launcherBunxPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("16.0.0\n")}, nil
			}
			gotArgs = cmd.Args
			return toolrun.Output{Stdout: []byte(`{"modules":[]}`)}, nil
		},
	}
	s := scope.Scope{Root: fixtureDir}
	_, _, err := ts.New(runner, config.ExtractConfig{Mode: config.ModeAuto, Src: "."}).Extract(context.Background(), s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if slices.Contains(gotArgs, "src") {
		t.Errorf("Src=%q was rewritten to \"src\"; depcruise args = %v", ".", gotArgs)
	}
	if !slices.Contains(gotArgs, ".") {
		t.Errorf("Src=%q was not passed through literally; depcruise args = %v", ".", gotArgs)
	}
}
