package gitnexus

import (
	"context"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// gitnexusSuccessJSON is a canned `gitnexus cypher` envelope with two files.
const gitnexusSuccessJSON = `{
	"markdown": "| file | dependants |\n| --- |\n| src/app/store.py | 42 |\n| src/app/util.py | 3 |",
	"row_count": 2
}`

// gitnexusEmptyJSON is a valid envelope with no data rows.
const gitnexusEmptyJSON = `{"markdown": "| file | dependants |\n| --- |", "row_count": 0}`

// makeImpactRunner returns a RunnerMock that detects gitnexus and returns the
// given JSON on stdout with exit code 0.
func makeImpactRunner(jsonOutput string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0, Stdout: []byte(jsonOutput)}, nil
		},
	}
}

// absentRunner returns a RunnerMock where no tool is detected.
func absentRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
}

// failRunner detects gitnexus but Run returns exit code 1 (e.g. repo not indexed).
func failRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 1}, nil
		},
	}
}

// malformedRunner detects gitnexus and returns non-JSON stdout.
func malformedRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0, Stdout: []byte("not valid json at all")}, nil
		},
	}
}

func TestRun_EnabledPresent(t *testing.T) {
	runner := makeImpactRunner(gitnexusSuccessJSON)
	impact, cov, err := Run(context.Background(), runner, t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusOK)
	}
	if len(impact) != 2 {
		t.Fatalf("impact len = %d, want 2", len(impact))
	}
	if impact["src/app/store.py"] != 42 {
		t.Errorf("store.py dependants = %d, want 42", impact["src/app/store.py"])
	}
	if impact["src/app/util.py"] != 3 {
		t.Errorf("util.py dependants = %d, want 3", impact["src/app/util.py"])
	}

	// The adapter must call the real CLI interface: cypher -r <root> <query>.
	calls := runner.RunCalls()
	if len(calls) != 1 {
		t.Fatalf("Run called %d times, want 1", len(calls))
	}
	args := calls[0].Cmd.Args
	if len(args) < 4 || args[0] != "cypher" || args[1] != "-r" {
		t.Errorf("args = %v, want cypher -r <root> <query>", args)
	}
	if !strings.Contains(args[3], "MATCH") {
		t.Errorf("query arg does not look like Cypher: %q", args[3])
	}
}

func TestRun_Disabled(t *testing.T) {
	impact, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusAbsent)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map for disabled tool, got %d entries", len(impact))
	}
}

func TestRun_AbsentTool(t *testing.T) {
	impact, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusAbsent)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map when tool absent, got %d entries", len(impact))
	}
}

func TestRun_ToolFailure(t *testing.T) {
	impact, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusPartial)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map on tool failure, got %d entries", len(impact))
	}
}

func TestRun_MalformedOutput(t *testing.T) {
	impact, cov, err := Run(context.Background(), malformedRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusPartial)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map for malformed output, got %d entries", len(impact))
	}
}

func TestRun_EmptyResult(t *testing.T) {
	impact, cov, err := Run(context.Background(), makeImpactRunner(gitnexusEmptyJSON), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusOK)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map for empty result, got %d entries", len(impact))
	}
}

func TestParseDependants(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    map[string]int
		wantErr bool
	}{
		{
			name: "success table",
			data: gitnexusSuccessJSON,
			want: map[string]int{"src/app/store.py": 42, "src/app/util.py": 3},
		},
		{
			name: "header and separator skipped",
			data: `{"markdown": "| file | dependants |\n| --- |\n| a.go | 7 |", "row_count": 1}`,
			want: map[string]int{"a.go": 7},
		},
		{
			name: "non-numeric count skipped",
			data: `{"markdown": "| a.go | seven |\n| b.go | 2 |", "row_count": 2}`,
			want: map[string]int{"b.go": 2},
		},
		{
			name:    "cypher error envelope",
			data:    `{"error": "Prepare failed: Binder exception"}`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			data:    "not json",
			wantErr: true,
		},
		{
			name: "empty markdown",
			data: gitnexusEmptyJSON,
			want: map[string]int{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseDependants([]byte(tc.data))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m) != len(tc.want) {
				t.Fatalf("got %d entries %v, want %d %v", len(m), m, len(tc.want), tc.want)
			}
			for k, v := range tc.want {
				if m[k] != v {
					t.Errorf("m[%q] = %d, want %d", k, m[k], v)
				}
			}
		})
	}
}
