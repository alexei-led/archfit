package scip

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// TestStrengths_AbsentReason asserts that absent semantic-strength coverage
// carries an actionable reason + enable step, not a silent absent — in
// particular that a TS project missing node_modules names the deps fix.
func TestStrengths_AbsentReason(t *testing.T) {
	writePkgJSON := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	noTools := func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
		return toolrun.ToolInfo{}, false
	}

	tests := []struct {
		name       string
		setup      func(t *testing.T) (root string, runner toolrun.Runner)
		wantReason string
	}{
		{
			name: "TS project, no node_modules, no tools installed",
			setup: func(t *testing.T) (string, toolrun.Runner) {
				dir := t.TempDir()
				writePkgJSON(t, dir)
				return dir, &toolrun.RunnerMock{DetectFunc: noTools}
			},
			wantReason: reasonTSNoNodeModules,
		},
		{
			name: "TS project, indexer+uv present but node_modules missing",
			setup: func(t *testing.T) (string, toolrun.Runner) {
				dir := t.TempDir()
				writePkgJSON(t, dir)
				return dir, &toolrun.RunnerMock{
					DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
						if tool == indexerTS || tool == "uv" {
							return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
						}
						return toolrun.ToolInfo{}, false
					},
				}
			},
			wantReason: reasonTSNoNodeModules,
		},
		{
			name: "non-TS project, no indexer",
			setup: func(t *testing.T) (string, toolrun.Runner) {
				return t.TempDir(), &toolrun.RunnerMock{DetectFunc: noTools}
			},
			wantReason: reasonScipNoIndexer,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, runner := tc.setup(t)
			a := New(runner)
			_, cov, err := a.Strengths(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cov.Status != diagnostic.StatusAbsent {
				t.Errorf("status = %q, want %q", cov.Status, diagnostic.StatusAbsent)
			}
			if cov.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", cov.Reason, tc.wantReason)
			}
			if cov.Tool != toolName {
				t.Errorf("tool = %q, want %q", cov.Tool, toolName)
			}
		})
	}
}

func TestParseReaderEdges(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		wantKey string
		wantVal string
		wantLen int
		wantErr bool
	}{
		{
			name:    "edges parsed and keyed by from\\x00to",
			stdout:  `{"edges":[{"from":"ccgram.handlers.hook_events","to":"ccgram.telegram_client","strength":"contract"},{"from":"ccgram.bootstrap","to":"ccgram.hook","strength":"intrusive"}]}`,
			wantKey: "ccgram.handlers.hook_events\x00ccgram.telegram_client",
			wantVal: "contract",
			wantLen: 2,
		},
		{
			name:    "helper error fails parse",
			stdout:  `{"error":"scip bindings: boom","edges":[]}`,
			wantErr: true,
		},
		{
			name:    "malformed json fails parse",
			stdout:  `not json`,
			wantErr: true,
		},
		{
			name:    "empty edge list yields empty map",
			stdout:  `{"edges":[]}`,
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseReaderEdges([]byte(tc.stdout))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(m), tc.wantLen)
			}
			if tc.wantKey != "" && m[tc.wantKey] != tc.wantVal {
				t.Errorf("m[%q] = %q, want %q", tc.wantKey, m[tc.wantKey], tc.wantVal)
			}
		})
	}
}

func TestDetectPyPackage(t *testing.T) {
	dir := t.TempDir()
	// flat layout: <dir>/mypkg/__init__.py
	pkgDir := filepath.Join(dir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, pyInitFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := detectPyPackage(dir); got != "mypkg" {
		t.Errorf("detectPyPackage = %q, want mypkg", got)
	}
}
