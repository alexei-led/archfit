package complexity

import (
	"context"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/toolrun"
)

const gocycloPath = "/usr/local/bin/gocyclo"

// gocycloRunner builds a mock that detects gocyclo and returns canned output.
func gocycloRunnerWithOutput(output string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == gocycloTool {
				return toolrun.ToolInfo{Name: tool, Path: gocycloPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0, Stdout: []byte(output)}, nil
		},
	}
}

// ---------------------------------------------------------------------------
// parseGocycloLine
// ---------------------------------------------------------------------------

func TestParseGocycloLine_TableDriven(t *testing.T) {
	root := testRoot
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantCCN  int
		wantFunc string
		wantFile string
		wantLine int
	}{
		{
			name:     "simple function",
			line:     "5 mypkg MyFunc /repo/pkg/foo.go:10:1",
			wantOK:   true,
			wantCCN:  5,
			wantFunc: "MyFunc",
			wantFile: "pkg/foo.go",
			wantLine: 10,
		},
		{
			name:     "method with receiver",
			line:     "12 mypkg (*Adapter).Run /repo/pkg/adapter.go:42:1",
			wantOK:   true,
			wantCCN:  12,
			wantFunc: "(*Adapter).Run",
			wantFile: "pkg/adapter.go",
			wantLine: 42,
		},
		{
			name:   "too few fields",
			line:   "5 mypkg",
			wantOK: false,
		},
		{
			name:   "non-numeric CCN",
			line:   "x mypkg Foo /repo/pkg/foo.go:1:1",
			wantOK: false,
		},
		{
			name:   "position missing colon",
			line:   "3 mypkg Bar /repo/pkg/bar.go",
			wantOK: false,
		},
		{
			name:   "position only one colon",
			line:   "3 mypkg Bar /repo/pkg/bar.go:10",
			wantOK: false,
		},
		{
			name:   "non-numeric line",
			line:   "3 mypkg Bar /repo/pkg/bar.go:abc:1",
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := parseGocycloLine(tc.line, root)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if f.CCN != tc.wantCCN {
				t.Errorf("CCN = %d, want %d", f.CCN, tc.wantCCN)
			}
			if f.Name != tc.wantFunc {
				t.Errorf("Name = %q, want %q", f.Name, tc.wantFunc)
			}
			if f.File != tc.wantFile {
				t.Errorf("File = %q, want %q", f.File, tc.wantFile)
			}
			if f.Line != tc.wantLine {
				t.Errorf("Line = %d, want %d", f.Line, tc.wantLine)
			}
			if f.NLOC != 0 {
				t.Errorf("NLOC = %d, want 0 (gocyclo has no NLOC)", f.NLOC)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseGocycloOutput
// ---------------------------------------------------------------------------

func TestParseGocycloOutput_MultiLine(t *testing.T) {
	root := testRoot
	data := []byte(
		"10 pkg (*T).A /repo/a.go:1:1\n" +
			"bad line\n" +
			"3 pkg B /repo/b.go:5:1\n",
	)
	funcs := parseGocycloOutput(data, root)
	if len(funcs) != 2 {
		t.Fatalf("got %d funcs, want 2", len(funcs))
	}
	if funcs[0].CCN != 10 || funcs[0].Name != "(*T).A" {
		t.Errorf("first func = %+v", funcs[0])
	}
	if funcs[1].CCN != 3 || funcs[1].Name != "B" {
		t.Errorf("second func = %+v", funcs[1])
	}
}

func TestParseGocycloOutput_Empty(t *testing.T) {
	funcs := parseGocycloOutput([]byte(""), "/repo")
	if len(funcs) != 0 {
		t.Errorf("expected empty, got %d", len(funcs))
	}
}

func TestParseGocycloOutput_AllMalformed(t *testing.T) {
	data := []byte("bad\nalso bad\n")
	funcs := parseGocycloOutput(data, "/repo")
	if len(funcs) != 0 {
		t.Errorf("expected empty for all-malformed output, got %d", len(funcs))
	}
}

func TestParseGocycloOutput_PathNormalised(t *testing.T) {
	root := "/my/repo"
	data := []byte("7 pkg Fn /my/repo/sub/file.go:3:1\n")
	funcs := parseGocycloOutput(data, root)
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].File != "sub/file.go" {
		t.Errorf("File = %q, want %q", funcs[0].File, "sub/file.go")
	}
}

// ---------------------------------------------------------------------------
// runGocyclo
// ---------------------------------------------------------------------------

func TestRunGocyclo_Absent(t *testing.T) {
	funcs, ok, err := runGocyclo(context.Background(), absentRunner(), t.TempDir(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("ok = true, want false when gocyclo absent")
	}
	if len(funcs) != 0 {
		t.Errorf("expected no funcs when absent, got %d", len(funcs))
	}
}

func TestRunGocyclo_Present(t *testing.T) {
	output := "16 pkg HotFunc /tmp/repo/svc.go:10:1\n3 pkg CoolFunc /tmp/repo/svc.go:50:1\n"
	funcs, ok, err := runGocyclo(context.Background(), gocycloRunnerWithOutput(output), "/tmp/repo", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("ok = false, want true when gocyclo present")
	}
	if len(funcs) != 2 {
		t.Fatalf("got %d funcs, want 2", len(funcs))
	}
	if funcs[0].Name != "HotFunc" || funcs[0].CCN != 16 {
		t.Errorf("unexpected first func: %+v", funcs[0])
	}
}

func TestRunGocyclo_NonZeroExitStillReturnsData(t *testing.T) {
	// gocyclo exits with count of over-threshold functions — non-zero is normal.
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == gocycloTool {
				return toolrun.ToolInfo{Name: tool, Path: gocycloPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 2, Stdout: []byte("20 pkg Big /tmp/r/x.go:1:1\n")}, nil
		},
	}
	funcs, ok, err := runGocyclo(context.Background(), runner, "/tmp/r", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("ok should be true even on non-zero exit")
	}
	if len(funcs) != 1 || funcs[0].CCN != 20 {
		t.Errorf("expected one func with CCN=20, got %+v", funcs)
	}
}

// TestRunGocyclo_TimeoutParam asserts that runGocyclo threads the configured
// timeout into ToolCmd.Timeout, and falls back to gocycloTimeout when zero
// (the byte-identical no-config path).
func TestRunGocyclo_TimeoutParam(t *testing.T) {
	var capturedTimeout time.Duration
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == gocycloTool {
				return toolrun.ToolInfo{Name: tool, Path: gocycloPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			capturedTimeout = cmd.Timeout
			return toolrun.Output{ExitCode: 0, Stdout: []byte{}}, nil
		},
	}
	// Zero → built-in constant (byte-identical guard).
	if _, _, err := runGocyclo(context.Background(), runner, t.TempDir(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTimeout != gocycloTimeout {
		t.Errorf("timeout(0) = %v, want %v (gocycloTimeout)", capturedTimeout, gocycloTimeout)
	}
	// Configured value → used directly.
	configured := 5 * time.Minute
	if _, _, err := runGocyclo(context.Background(), runner, t.TempDir(), configured); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTimeout != configured {
		t.Errorf("timeout(configured) = %v, want %v", capturedTimeout, configured)
	}
}
