package scip

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		{
			name: "Rust project, rust-analyzer absent",
			setup: func(t *testing.T) (string, toolrun.Runner) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return dir, &toolrun.RunnerMock{DetectFunc: noTools}
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

// TestDetectIndexer_Rust asserts a Cargo project with rust-analyzer installed
// resolves to the rust-analyzer indexer, the crate name, and lang "rust".
func TestDetectIndexer_Rust(t *testing.T) {
	dir := t.TempDir()
	manifest := "[package]\nname = \"demo-crate\"\nversion = \"0.1.0\"\n"
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerRust {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
	}
	indexer, pkg, lang, ok := New(runner).detectIndexer(context.Background(), dir)
	if !ok {
		t.Fatal("detectIndexer: ok = false, want true")
	}
	if indexer != indexerRust {
		t.Errorf("indexer = %q, want %q", indexer, indexerRust)
	}
	if pkg != "demo-crate" {
		t.Errorf("pkg = %q, want demo-crate", pkg)
	}
	if lang != langRust {
		t.Errorf("lang = %q, want rust", lang)
	}
}

// TestDetectIndexer_VirtualWorkspace asserts that a Cargo virtual workspace
// (no [package] in root Cargo.toml) still triggers rust-analyzer when cargo
// metadata enumerates members — pkg is the comma-joined crate names.
func TestDetectIndexer_VirtualWorkspace(t *testing.T) {
	dir := t.TempDir()
	manifest := "[workspace]\nmembers = [\"crate-a\", \"crate-b\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	// Canned cargo metadata JSON with two workspace members.
	metaJSON := `{"packages":[{"name":"crate-a"},{"name":"crate-b"}],"workspace_members":["crate-a 0.1.0 (path+file:///tmp/a)","crate-b 0.1.0 (path+file:///tmp/b)"]}`
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerRust || tool == toolCargo {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == "cargo" {
				return toolrun.Output{Stdout: []byte(metaJSON), ExitCode: 0}, nil
			}
			return toolrun.Output{}, nil
		},
	}
	indexer, pkg, lang, ok := New(runner).detectIndexer(context.Background(), dir)
	if !ok {
		t.Fatal("detectIndexer: ok = false, want true for virtual workspace")
	}
	if indexer != indexerRust {
		t.Errorf("indexer = %q, want %q", indexer, indexerRust)
	}
	if lang != langRust {
		t.Errorf("lang = %q, want %q", lang, langRust)
	}
	// pkg must contain both crate names (comma-joined).
	for _, name := range []string{"crate-a", "crate-b"} {
		found := false
		for _, p := range strings.Split(pkg, ",") {
			if strings.TrimSpace(p) == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("pkg %q does not contain %q", pkg, name)
		}
	}
}

func TestCargoPackageName(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     string
	}{
		{"package table", "[package]\nname = \"ripgrep\"\nversion = \"14.0.0\"\n", "ripgrep"},
		{"name after other keys", "[package]\nedition = \"2021\"\nname = \"just\"\n", "just"},
		{"single quotes", "[package]\nname = 'crate'\n", "crate"},
		{"virtual workspace, no package", "[workspace]\nmembers = [\"crates/*\"]\n", ""},
		{"name key only under dependencies ignored", "[dependencies]\nname = \"1.0\"\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(tc.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := cargoPackageName(dir); got != tc.want {
				t.Errorf("cargoPackageName = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIndexArgs_Rust pins the rust-analyzer command shape: scip needs the project
// path positional ("." — WorkDir is the root) or it exits 0 and writes nothing.
func TestIndexArgs_Rust(t *testing.T) {
	got := indexArgs(indexerRust, "demo", "/repo", "/tmp/index.scip")
	want := []string{"scip", ".", flagOutput, "/tmp/index.scip"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
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
