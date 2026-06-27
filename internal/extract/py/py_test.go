package py_test

import (
	"context"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	fixtureJSON   = `{"edges":[{"importer":"myapp.a","imported":"myapp.b","line":5},{"importer":"myapp.a","imported":"myapp.b._internal.impl","line":6}],"unresolved":0}`
	testPkgName   = "myapp"
	testScopeMode = "full"
	testRoot      = "../../../testdata/py"

	modMyappA        = "module:myapp.a"
	modMyappB        = "module:myapp.b"
	modMyappBPrivate = "module:myapp.b._internal.impl"
	modPub           = "module:pub"
	hintIntrusive    = "intrusive"
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

	// Edge myapp.a → myapp.b._internal.impl should be "uses_internal" and carry
	// an "intrusive" strength hint (PEP 8-private segment "_internal").
	k2 := edgeKey{modMyappA, modMyappBPrivate}
	if got := edges[k2].Kind; got != graph.EdgeKindUsesInternal {
		t.Errorf("edge %v: kind = %q, want %q", k2, got, graph.EdgeKindUsesInternal)
	}
	if got := edges[k2].StrengthHint; got != hintIntrusive {
		t.Errorf("edge %v: StrengthHint = %q, want %q", k2, got, hintIntrusive)
	}
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
			return toolrun.Output{
				Stdout:   []byte(`{"edges":[],"unresolved":2}`),
				ExitCode: 0,
			}, nil
		},
	}

	cfg := config.ExtractConfig{
		PyPackage: testPkgName,
		Mode:      config.ModeAuto,
	}
	e := py.New(mock, cfg)

	_, cov, err := e.Extract(context.Background(), scope.Scope{Root: testRoot, Mode: testScopeMode})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Unresolved != 2 {
		t.Errorf("cov.Unresolved = %d, want 2", cov.Unresolved)
	}
}

func TestExtract_SymbolLevelStrength(t *testing.T) {
	// Fixture: three edges, all module names are public (no module-level private check
	// applies). Only the line_contents drives the strength hint.
	const fixture = `{"edges":[` +
		`{"importer":"pub","imported":"priv","line":1,"line_contents":"from priv import _sym"},` +
		`{"importer":"pub","imported":"other","line":2,"line_contents":"from other import thing"},` +
		`{"importer":"pub","imported":"multi","line":3,"line_contents":"from multi import a, _b, c"},` +
		`{"importer":"pub","imported":"alias","line":4,"line_contents":"from alias import _hidden as h"}` +
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
