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
		Internal:  []string{testPkgName + ".b._internal.**"},
		Mode:      config.ModeAuto,
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
	edgeKinds := make(map[edgeKey]graph.EdgeKind)
	for _, edge := range facts.Edges {
		edgeKinds[edgeKey{edge.From, edge.To}] = edge.Kind
	}

	// Edge myapp.a → myapp.b should be "imports".
	k1 := edgeKey{"module:myapp.a", "module:myapp.b"}
	if got := edgeKinds[k1]; got != graph.EdgeKindImports {
		t.Errorf("edge %v: kind = %q, want %q", k1, got, graph.EdgeKindImports)
	}

	// Edge myapp.a → myapp.b._internal.impl should be "uses_internal".
	k2 := edgeKey{"module:myapp.a", "module:myapp.b._internal.impl"}
	if got := edgeKinds[k2]; got != graph.EdgeKindUsesInternal {
		t.Errorf("edge %v: kind = %q, want %q", k2, got, graph.EdgeKindUsesInternal)
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
