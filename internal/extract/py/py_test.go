package py_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	fixtureJSON           = `{"edges":[{"importer":"myapp.a","imported":"myapp.b","line":5},{"importer":"myapp.a","imported":"myapp.b._internal.impl","line":6}],"unresolved":0}`
	unresolvedFixtureJSON = `{"edges":[],"unresolved":2,"unresolved_imports":[{"importer":"myapp.a","imported":"httpx","line":3,"line_contents":"import httpx"},{"importer":"myapp.a","imported":"boto3.session","line":4,"line_contents":"from boto3.session import Session"}]}`
	testPkgName           = "myapp"
	testScopeMode         = "full"
	testRoot              = "../../../testdata/py"

	modMyappA        = "module:myapp.a"
	modMyappB        = "module:myapp.b"
	modMyappBPrivate = "module:myapp.b._internal.impl"
	extHTTPX         = "external:httpx"
	extBoto3Session  = "external:boto3.session"
	modPub           = "module:pub"
	hintIntrusive    = "intrusive"
	sourceGrimp      = "grimp"
	confidenceLow    = "low"
	confidenceHigh   = "high"
)

func TestExtract_Parse(t *testing.T) {
	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			// Return fixture JSON for every call (version check + actual run).
			return toolrun.Output{Stdout: []byte(fixtureJSON), ExitCode: 0}, nil
		},
	}

	cfg := config.ExtractConfig{
		PyPackage: testPkgName,
		// Dotted module glob (same form as paths: and classifyStrength), not slash.
		Internal: []string{testPkgName + ".b._internal.*"},
		Mode:     config.ModeAuto,
	}
	e := py.New(mock, cfg)

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Verify unresolved count.
	if cov.Unresolved != 0 {
		t.Errorf("cov.Unresolved = %d, want 0", cov.Unresolved)
	}

	// Build a lookup of edges by (from, to).
	type edgeKey struct{ from, to string }
	edges := make(map[edgeKey]graph.Edge)
	for _, edge := range facts.Edges {
		edges[edgeKey{edge.From, edge.To}] = edge
	}

	// Edge myapp.a → myapp.b should be "imports" with no strength hint
	// (public module name, no config public glob).
	k1 := edgeKey{modMyappA, modMyappB}
	if got := edges[k1].Kind; got != graph.EdgeKindImports {
		t.Errorf("edge %v: kind = %q, want %q", k1, got, graph.EdgeKindImports)
	}
	if got := edges[k1].StrengthHint; got != "" {
		t.Errorf("edge %v: StrengthHint = %q, want empty", k1, got)
	}
	if !hasPyConnascence(edges[k1], graph.ConnascenceName) {
		t.Errorf("edge %v: connascence = %+v, want name", k1, edges[k1].ConnascenceHints)
	}
	if hasPyConnascence(edges[k1], graph.ConnascenceType) {
		t.Errorf("edge %v: connascence = %+v, must not invent type", k1, edges[k1].ConnascenceHints)
	}

	// Edge myapp.a → myapp.b._internal.impl should be "uses_internal" and carry
	// an "intrusive" strength hint (PEP 8-private segment "_internal").
	k2 := edgeKey{modMyappA, modMyappBPrivate}
	if got := edges[k2].Kind; got != graph.EdgeKindUsesInternal {
		t.Errorf("edge %v: kind = %q, want %q", k2, got, graph.EdgeKindUsesInternal)
	}
	if got := edges[k2].StrengthHint; got != hintIntrusive {
		t.Errorf("edge %v: StrengthHint = %q, want %q", k2, got, hintIntrusive)
	}
	if !hasPyConnascence(edges[k2], graph.ConnascenceName) {
		t.Errorf("edge %v: connascence = %+v, want private-name evidence", k2, edges[k2].ConnascenceHints)
	}
}

func hasPyConnascence(e graph.Edge, kind string) bool {
	for _, h := range e.ConnascenceHints {
		if h.Kind == kind && h.Source == sourceGrimp {
			return true
		}
	}
	return false
}

func TestExtract_WithUnresolved(t *testing.T) {
	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(unresolvedFixtureJSON), ExitCode: 0}, nil
		},
	}

	cfg := config.ExtractConfig{
		PyPackage: testPkgName,
		Mode:      config.ModeAuto,
	}
	e := py.New(mock, cfg)

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Unresolved != 2 {
		t.Errorf("cov.Unresolved = %d, want 2", cov.Unresolved)
	}

	type edgeKey struct{ from, to string }
	byKey := make(map[edgeKey]graph.Edge)
	for _, edge := range facts.Edges {
		byKey[edgeKey{edge.From, edge.To}] = edge
	}
	for _, want := range []struct {
		key  edgeKey
		line int
	}{
		{key: edgeKey{modMyappA, extHTTPX}, line: 3},
		{key: edgeKey{modMyappA, extBoto3Session}, line: 4},
	} {
		edge, ok := byKey[want.key]
		if !ok {
			t.Fatalf("edge %v not found in facts", want.key)
		}
		if edge.Kind != graph.EdgeKindImports {
			t.Fatalf("edge %v: kind = %q, want %q", want.key, edge.Kind, graph.EdgeKindImports)
		}
		if edge.Confidence != confidenceLow {
			t.Fatalf("edge %v: confidence = %q, want %q", want.key, edge.Confidence, confidenceLow)
		}
		if len(edge.Locations) != 1 || edge.Locations[0].File != "myapp/a" || edge.Locations[0].Line != want.line {
			t.Fatalf("edge %v: locations = %+v, want myapp/a:%d", want.key, edge.Locations, want.line)
		}
	}
}

func TestExtract_SymbolLevelStrength(t *testing.T) {
	// Fixture: three edges, all module names are public (no module-level private check
	// applies). Only the line_contents drives the strength hint.
	const fixture = `{"edges":[` +
		`{"importer":"pub","imported":"priv","line":1,"line_contents":"from priv import _sym"},` +
		`{"importer":"pub","imported":"other","line":2,"line_contents":"from other import thing"},` +
		`{"importer":"pub","imported":"multi","line":3,"line_contents":"from multi import a, _b, c"},` +
		`{"importer":"pub","imported":"alias","line":4,"line_contents":"from alias import _hidden as h"},` +
		`{"importer":"pub","imported":"consts","line":5,"line_contents":"from consts import MAX_SIZE"},` +
		`{"importer":"pub","imported":"dtos","line":6,"line_contents":"from dtos import UserDTO"}` +
		`],"unresolved":0}`

	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(fixture), ExitCode: 0}, nil
		},
	}

	cfg := config.ExtractConfig{PyPackage: testPkgName, Mode: config.ModeAuto}
	e := py.New(mock, cfg)

	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	type edgeKey struct{ from, to string }
	byKey := make(map[edgeKey]graph.Edge)
	for _, edge := range facts.Edges {
		byKey[edgeKey{edge.From, edge.To}] = edge
	}

	tests := []struct {
		name string
		key  edgeKey
		want string // expected StrengthHint; "" means abstained
	}{
		{"private symbol → intrusive", edgeKey{modPub, "module:priv"}, hintIntrusive},
		{"public symbol → abstained", edgeKey{modPub, "module:other"}, ""},
		{"multi-import one private → intrusive", edgeKey{modPub, "module:multi"}, hintIntrusive},
		{"aliased private symbol → intrusive", edgeKey{modPub, "module:alias"}, hintIntrusive},
		// grimp/line-parse has no object kinds: a pure-data const import (book Ch7
		// → model in Go/Rust) is indistinguishable from a function import here, so
		// Python abstains rather than fabricating a strength (Wave 7 LLM labels are
		// the designed refinement path).
		{"const import → abstained (no object kinds)", edgeKey{modPub, "module:consts"}, ""},
		// Same blindness for DTOs: Go upgrades a pure-data struct to the "dto"
		// hint (method set + field visibility from type info), but a Python class
		// import carries no shape at grimp granularity — abstain, never a
		// fabricated contract upgrade (Wave 7 LLM labels are the designed path).
		{"DTO-shaped class import → abstained (class shape invisible)", edgeKey{modPub, "module:dtos"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := byKey[tt.key]
			if !ok {
				t.Fatalf("edge %v not found in facts", tt.key)
			}
			if e.StrengthHint != tt.want {
				t.Errorf("StrengthHint = %q, want %q", e.StrengthHint, tt.want)
			}
		})
	}
}

// TestExtract_ModuleLevelIntrusiveStillDetected verifies the pre-existing module-level
// private detection (underscore segment in the imported module name) is unaffected by
// the symbol-level extension. It feeds edges WITHOUT line_contents so the symbol parser
// sees empty strings and must not interfere.
func TestExtract_ModuleLevelIntrusiveStillDetected(t *testing.T) {
	// Edges have no line_contents — the module name itself carries the private segment.
	const fixture = `{"edges":[` +
		`{"importer":"myapp.a","imported":"myapp.b._internal.impl","line":1},` +
		`{"importer":"myapp.a","imported":"myapp.b","line":2}` +
		`],"unresolved":0}`

	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(fixture), ExitCode: 0}, nil
		},
	}

	cfg := config.ExtractConfig{PyPackage: testPkgName, Mode: config.ModeAuto}
	e := py.New(mock, cfg)

	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	type edgeKey struct{ from, to string }
	byKey := make(map[edgeKey]graph.Edge)
	for _, edge := range facts.Edges {
		byKey[edgeKey{edge.From, edge.To}] = edge
	}

	// Module with private segment → intrusive (module-level rule).
	k1 := edgeKey{modMyappA, modMyappBPrivate}
	if got := byKey[k1].StrengthHint; got != hintIntrusive {
		t.Errorf("module-level private: StrengthHint = %q, want %q", got, hintIntrusive)
	}
	// Public module → abstained.
	k2 := edgeKey{modMyappA, modMyappB}
	if got := byKey[k2].StrengthHint; got != "" {
		t.Errorf("public module: StrengthHint = %q, want empty", got)
	}
}

func TestExtract_ToolAbsentAuto(t *testing.T) {
	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			t.Fatal("RunFunc should not be called when tool is absent")
			return toolrun.Output{}, nil
		},
	}

	cfg := config.ExtractConfig{
		PyPackage: testPkgName,
		Mode:      config.ModeAuto,
	}
	e := py.New(mock, cfg)

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
	if err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}
	if len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
		t.Errorf("expected empty facts, got nodes=%d edges=%d", len(facts.Nodes), len(facts.Edges))
	}
	if !strings.EqualFold(cov.Status, "absent") {
		t.Errorf("cov.Status = %q, want \"absent\"", cov.Status)
	}
}

// TestExtract_NonZeroExit: when the grimp helper exits non-zero (e.g. an import
// error inside the target project), auto mode degrades to a partial coverage gap
// instead of failing the whole run; ModeOn still hard-errors so a required
// analyzer surfaces the problem.
func TestExtract_NonZeroExit(t *testing.T) {
	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("uv 0.4.0"), ExitCode: 0}, nil
			}
			return toolrun.Output{ExitCode: 1, Stderr: []byte("ModuleNotFoundError: no module named 'myapp'")}, nil
		},
	}

	t.Run("auto degrades to partial coverage", func(t *testing.T) {
		cfg := config.ExtractConfig{PyPackage: testPkgName, Mode: config.ModeAuto}
		e := py.New(mock, cfg)
		facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
		if err != nil {
			t.Fatalf("auto mode must not error on grimp helper non-zero exit; got %v", err)
		}
		if cov.Status != "partial" {
			t.Errorf("Status = %q, want %q", cov.Status, "partial")
		}
		if len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
			t.Errorf("expected no nodes/edges on degraded run; got %d nodes, %d edges", len(facts.Nodes), len(facts.Edges))
		}
	})

	t.Run("on hard-errors", func(t *testing.T) {
		cfg := config.ExtractConfig{PyPackage: testPkgName, Mode: config.ModeOn}
		e := py.New(mock, cfg)
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode}); err == nil {
			t.Error("ModeOn must hard-error on grimp helper non-zero exit")
		}
	})
}

// TestExtract_MultiPackageArgs verifies that when multiple top-level Python packages
// are discovered under ScanRoot, all names are passed to the grimp helper via
// --packages pkg1 pkg2 … (grimp.build_graph is variadic).
func TestFirstPartyPackages_IncludesDiscoveredAndConfiguredRoots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, dir := range []string{filepath.Join(root, "src", "prefect"), filepath.Join(root, "tests")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "__init__.py"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]"), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("uv 0.4.0"), ExitCode: 0}, nil
			}
			gotArgs = cmd.Args
			return toolrun.Output{Stdout: []byte(`{"edges":[],"unresolved":0}`), ExitCode: 0}, nil
		},
	}

	e := py.New(mock, config.ExtractConfig{PyPackage: "tests", Paths: []string{"prefect.blocks.**", "tests.**"}, Mode: config.ModeAuto})
	if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root, Mode: testScopeMode}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	idx := slices.Index(gotArgs, "--first-party-packages")
	if idx == -1 {
		t.Fatalf("expected --first-party-packages in args %v", gotArgs)
	}
	firstParty := gotArgs[idx+1:]
	for _, want := range []string{"prefect", "tests"} {
		if !slices.Contains(firstParty, want) {
			t.Fatalf("first-party package %q not found in args %v", want, gotArgs)
		}
	}
}

func TestExtract_MultiPackageArgs(t *testing.T) {
	root := t.TempDir()

	// Python project marker.
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Two top-level packages discovered by __init__.py.
	for _, pkg := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, pkg), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pkg, "__init__.py"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var gotArgs []string
	mock := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "uv" {
				return toolrun.ToolInfo{Name: "uv"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("uv 0.4.0"), ExitCode: 0}, nil
			}
			gotArgs = cmd.Args
			return toolrun.Output{Stdout: []byte(`{"edges":[],"unresolved":0}`), ExitCode: 0}, nil
		},
	}

	e := py.New(mock, config.ExtractConfig{Mode: config.ModeAuto})
	if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root, Mode: testScopeMode}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// --packages flag must be present.
	idx := slices.Index(gotArgs, "--packages")
	if idx == -1 {
		t.Fatalf("expected --packages flag in args %v", gotArgs)
	}
	// Both discovered packages (sorted) must follow --packages.
	remaining := gotArgs[idx+1:]
	for _, want := range []string{"alpha", "beta"} {
		if !slices.Contains(remaining, want) {
			t.Errorf("package %q not found after --packages in args %v", want, gotArgs)
		}
	}
}
