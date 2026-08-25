package scip

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	reportmodel "github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// readerJSONSuccess is a minimal valid scip_reader.py output containing one
// symbol and one cross-module reference edge.
const readerJSONSuccess = `{
	"edges": [],
	"symbols": [
		{"symbol": "go 1.0 example.com/foo internal/a/a.go/A#", "path": "internal/a/a.go", "module": "internal/a", "fan_in": 3},
		{"symbol": "go 1.0 example.com/foo internal/b/b.go/B#", "path": "internal/b/b.go", "module": "internal/b", "fan_in": 1}
	],
	"symbol_refs": [
		{"from_symbol": "go 1.0 example.com/foo internal/b/b.go/B#", "to_symbol": "go 1.0 example.com/foo internal/a/a.go/A#"}
	],
	"intra_refs": [
		{"from_symbol": "go 1.0 example.com/foo internal/a/a.go/A#", "to_symbol": "go 1.0 example.com/foo internal/a/a2.go/A2#"}
	]
}`

// readerJSONEmpty is a valid output with no symbols or refs.
const readerJSONEmpty = `{"edges": [], "symbols": [], "symbol_refs": []}`

// symAFixture is symbol A from readerJSONSuccess (definition, cross-module ref
// target, and intra-module ref source).
const symAFixture = "go 1.0 example.com/foo internal/a/a.go/A#"

// makeGoRoot creates a temporary directory with a go.mod so detectIndexer picks
// up scip-go as the indexer.
func makeGoRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gomod := "module example.com/foo\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// indexCreatingRunner returns a RunnerMock that:
//   - detects scip-go and uv as present
//   - for the indexer run: creates the --output file so os.Stat passes
//   - for the uv run: returns the provided reader JSON
func indexCreatingRunner(readerJSON string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerGo || tool == "uv" {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == "uv" {
				return toolrun.Output{Stdout: []byte(readerJSON), ExitCode: 0}, nil
			}
			// Indexer: create the --output file so os.Stat succeeds.
			for i, arg := range cmd.Args {
				if arg == "--output" && i+1 < len(cmd.Args) {
					_ = os.WriteFile(cmd.Args[i+1], []byte("placeholder"), 0o600)
				}
			}
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

func TestParseReaderSymbols(t *testing.T) {
	tests := []struct {
		name       string
		stdout     string
		wantErr    bool
		wantSymbol string
		wantModule string
		wantPath   string
		wantFanIn  int
		wantRef    string // from_symbol that should have at least one ref
		wantIntra  string // from_symbol that should have at least one intra-module ref
	}{
		{
			name:       "success: symbols and refs populated",
			stdout:     readerJSONSuccess,
			wantSymbol: symAFixture,
			wantModule: "internal/a",
			wantPath:   "internal/a/a.go",
			wantFanIn:  3,
			wantRef:    "go 1.0 example.com/foo internal/b/b.go/B#",
			wantIntra:  symAFixture,
		},
		{
			name:   "empty arrays yield empty graph",
			stdout: readerJSONEmpty,
		},
		{
			name:    "helper error fails parse",
			stdout:  `{"error": "scip bindings: boom", "edges": [], "symbols": [], "symbol_refs": []}`,
			wantErr: true,
		},
		{
			name:    "malformed json fails parse",
			stdout:  `not json`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, err := parseReaderSymbols([]byte(tc.stdout))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantSymbol != "" {
				if got := graph.Module[tc.wantSymbol]; got != tc.wantModule {
					t.Errorf("Module[%q] = %q, want %q", tc.wantSymbol, got, tc.wantModule)
				}
				if got := graph.Path[tc.wantSymbol]; got != tc.wantPath {
					t.Errorf("Path[%q] = %q, want %q", tc.wantSymbol, got, tc.wantPath)
				}
				if got := graph.FanIn[tc.wantSymbol]; got != tc.wantFanIn {
					t.Errorf("FanIn[%q] = %d, want %d", tc.wantSymbol, got, tc.wantFanIn)
				}
			}
			if tc.wantRef != "" {
				if len(graph.Refs[tc.wantRef]) == 0 {
					t.Errorf("Refs[%q] is empty, want at least one entry", tc.wantRef)
				}
			}
			if tc.wantIntra != "" {
				if len(graph.IntraRefs[tc.wantIntra]) == 0 {
					t.Errorf("IntraRefs[%q] is empty, want at least one entry", tc.wantIntra)
				}
			}
		})
	}
}

func TestAdapter_Symbols_AbsentTool(t *testing.T) {
	a := New(&toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}, 0)
	graph, cov, err := a.Symbols(context.Background(), scope.Scope{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusAbsent)
	}
	if len(graph.Module) != 0 || len(graph.FanIn) != 0 || len(graph.Refs) != 0 {
		t.Error("expected empty graph for absent tool")
	}
}

func TestAdapter_Symbols_Success(t *testing.T) {
	root := makeGoRoot(t)
	a := New(indexCreatingRunner(readerJSONSuccess), 0)

	graph, cov, err := a.Symbols(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusOK)
	}
	if len(graph.Module) != 2 {
		t.Errorf("Module len = %d, want 2", len(graph.Module))
	}
	if graph.Module["go 1.0 example.com/foo internal/a/a.go/A#"] != "internal/a" {
		t.Error("symbol A not mapped to internal/a")
	}
	if graph.FanIn["go 1.0 example.com/foo internal/a/a.go/A#"] != 3 {
		t.Errorf("FanIn for A = %d, want 3", graph.FanIn["go 1.0 example.com/foo internal/a/a.go/A#"])
	}
	toSym := "go 1.0 example.com/foo internal/a/a.go/A#"
	fromSym := "go 1.0 example.com/foo internal/b/b.go/B#"
	if _, ok := graph.Refs[fromSym][toSym]; !ok {
		t.Error("expected cross-module ref from B to A")
	}
}

// TestAdapter_Symbols_EmptyIndex verifies that when the SCIP pipeline succeeds
// but the reader emits zero symbols, Symbols() returns StatusPartial (not
// StatusOK) with an actionable reason. This fires when scip-python runs against
// a Python version it does not yet support (e.g. 3.14), producing a tiny index
// with no definitions.
func TestAdapter_Symbols_EmptyIndex(t *testing.T) {
	root := makeGoRoot(t)
	a := New(indexCreatingRunner(readerJSONEmpty), 0)

	graph, cov, err := a.Symbols(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusPartial {
		t.Errorf("cov.Status = %q, want %q (empty index must be partial, not ok)", cov.Status, reportmodel.StatusPartial)
	}
	if cov.Reason == "" {
		t.Error("cov.Reason is empty, want an actionable message")
	}
	if len(graph.Module) != 0 {
		t.Error("expected empty graph for empty index")
	}
}

func TestAdapter_Symbols_MalformedOutput(t *testing.T) {
	root := makeGoRoot(t)
	a := New(indexCreatingRunner("not json at all"), 0)

	graph, cov, err := a.Symbols(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusPartial)
	}
	if len(graph.Module) != 0 {
		t.Error("expected empty graph for malformed output")
	}
}

// TestAdapter_SinglePass verifies that Strengths and Symbols share one memoized
// index+read pipeline per root: the SCIP indexer must run exactly once even
// when both methods execute (the engine calls both every run).
func TestAdapter_SinglePass(t *testing.T) {
	root := makeGoRoot(t)
	runner := indexCreatingRunner(readerJSONSuccess)
	a := New(runner, 0)

	if _, _, err := a.Strengths(context.Background(), scope.Scope{Root: root}); err != nil {
		t.Fatalf("Strengths: %v", err)
	}
	g, cov, err := a.Symbols(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Symbols: %v", err)
	}
	if cov.Status != reportmodel.StatusOK || cov.Tool != "scip-symbols" {
		t.Errorf("Symbols coverage = %+v, want ok/scip-symbols", cov)
	}
	if len(g.Module) != 2 {
		t.Errorf("Module len = %d, want 2 (cached pipeline must yield full output)", len(g.Module))
	}

	indexerRuns := 0
	for _, call := range runner.RunCalls() {
		if call.Cmd.Name == indexerGo {
			indexerRuns++
		}
	}
	if indexerRuns != 1 {
		t.Errorf("indexer ran %d times, want 1 (single-pass memoization)", indexerRuns)
	}
}
