package gitnexus

import (
	"context"
	"os"
	"path/filepath"
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

// indexedRoot returns a temp dir containing a .gitnexus index directory so
// hasIndex(root) is true (the "index present on disk" condition).
func indexedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".gitnexus"), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRun_EnabledPresent(t *testing.T) {
	runner := makeImpactRunner(gitnexusSuccessJSON)
	impact, cov, err := Run(context.Background(), runner, t.TempDir(), true, false)
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

// TestRun_AutoDetectUsesPresentIndex is the core of the auto-detect fix: with
// gitnexus neither forced on nor explicitly off, a present .gitnexus index is
// queried automatically (the prior blind spot silently ignored it). The OK
// coverage carries the auto-detect reason so the run is self-documenting.
func TestRun_AutoDetectUsesPresentIndex(t *testing.T) {
	runner := makeImpactRunner(gitnexusSuccessJSON)
	impact, cov, err := Run(context.Background(), runner, indexedRoot(t), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Fatalf("status = %q, want %q (present index must be auto-used)", cov.Status, statusOK)
	}
	if len(impact) != 2 {
		t.Fatalf("impact len = %d, want 2 (index not actually queried)", len(impact))
	}
	if cov.Reason != reasonAutoDetected {
		t.Errorf("reason = %q, want %q", cov.Reason, reasonAutoDetected)
	}
	if n := len(runner.RunCalls()); n != 1 {
		t.Errorf("CLI Run called %d times, want 1 (auto-detect must query the index)", n)
	}
}

// TestRun_AutoDetectNoIndex: auto-detect mode with no index on disk → absent,
// with the actionable "build an index" reason (not silent).
func TestRun_AutoDetectNoIndex(t *testing.T) {
	_, cov, err := Run(context.Background(), makeImpactRunner(gitnexusSuccessJSON), t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusAbsent {
		t.Errorf("status = %q, want %q", cov.Status, statusAbsent)
	}
	if cov.Reason != reasonOptInNoIndex {
		t.Errorf("reason = %q, want %q", cov.Reason, reasonOptInNoIndex)
	}
}

// TestRun_AutoDetectIndexPresentCLIAbsent: an index we cannot read because the
// CLI is missing reports the install step, not a generic "not installed".
func TestRun_AutoDetectIndexPresentCLIAbsent(t *testing.T) {
	_, cov, _ := Run(context.Background(), absentRunner(), indexedRoot(t), false, false)
	if cov.Status != statusAbsent {
		t.Errorf("status = %q, want %q", cov.Status, statusAbsent)
	}
	if cov.Reason != reasonHasIndexNoCLI {
		t.Errorf("reason = %q, want %q", cov.Reason, reasonHasIndexNoCLI)
	}
}

// TestRun_DisabledReasons asserts the explicit-off path distinguishes a present
// index ("flip the flag") from no index, and that the forced-on path's missing
// CLI and unindexed repo each carry their own actionable reason.
func TestRun_DisabledReasons(t *testing.T) {
	t.Run("explicitly off with index present (warning path)", func(t *testing.T) {
		// Even with a queryable CLI, an explicit off is respected — the present
		// index is reported, never auto-used.
		runner := makeImpactRunner(gitnexusSuccessJSON)
		impact, cov, _ := Run(context.Background(), runner, indexedRoot(t), false, true)
		if cov.Status != statusAbsent {
			t.Errorf("status = %q, want %q", cov.Status, statusAbsent)
		}
		if cov.Reason != reasonDisabledHasIndex {
			t.Errorf("reason = %q, want %q", cov.Reason, reasonDisabledHasIndex)
		}
		if len(impact) != 0 {
			t.Errorf("explicit off must not query the index, got %d entries", len(impact))
		}
		if n := len(runner.RunCalls()); n != 0 {
			t.Errorf("CLI Run called %d times, want 0 (explicit off must not query)", n)
		}
	})

	t.Run("explicitly off with no index", func(t *testing.T) {
		_, cov, _ := Run(context.Background(), absentRunner(), t.TempDir(), false, true)
		if cov.Reason != reasonDisabledNoIndex {
			t.Errorf("reason = %q, want %q", cov.Reason, reasonDisabledNoIndex)
		}
	})

	t.Run("on but CLI absent", func(t *testing.T) {
		_, cov, _ := Run(context.Background(), absentRunner(), t.TempDir(), true, false)
		if cov.Reason != reasonNotInstalled {
			t.Errorf("reason = %q, want %q", cov.Reason, reasonNotInstalled)
		}
	})

	t.Run("on with index present but CLI absent", func(t *testing.T) {
		_, cov, _ := Run(context.Background(), absentRunner(), indexedRoot(t), true, false)
		if cov.Reason != reasonHasIndexNoCLI {
			t.Errorf("reason = %q, want %q", cov.Reason, reasonHasIndexNoCLI)
		}
	})

	t.Run("on, repo not indexed (run fails)", func(t *testing.T) {
		_, cov, _ := Run(context.Background(), failRunner(), t.TempDir(), true, false)
		if cov.Status != statusPartial {
			t.Errorf("status = %q, want %q", cov.Status, statusPartial)
		}
		if cov.Reason != reasonNotIndexed {
			t.Errorf("reason = %q, want %q", cov.Reason, reasonNotIndexed)
		}
	})
}

func TestRun_AbsentTool(t *testing.T) {
	impact, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), true, false)
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
	impact, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true, false)
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
	impact, cov, err := Run(context.Background(), malformedRunner(), t.TempDir(), true, false)
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
	impact, cov, err := Run(context.Background(), makeImpactRunner(gitnexusEmptyJSON), t.TempDir(), true, false)
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

// TestHasIndex covers both recognised index directories and the absent case.
func TestHasIndex(t *testing.T) {
	for _, dir := range indexDirs {
		t.Run("detects "+dir, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
				t.Fatal(err)
			}
			if !hasIndex(root) {
				t.Errorf("hasIndex(%s) = false, want true", dir)
			}
		})
	}
	t.Run("absent", func(t *testing.T) {
		if hasIndex(t.TempDir()) {
			t.Error("hasIndex on empty dir = true, want false")
		}
	})
	t.Run("file not dir is not an index", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".gitnexus"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if hasIndex(root) {
			t.Error("a .gitnexus file (not dir) must not count as an index")
		}
	})
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
