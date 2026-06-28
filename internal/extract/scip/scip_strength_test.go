package scip

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	strengthContract   = "contract"
	strengthFunctional = "functional"
	strengthModel      = "model"
	strengthIntrusive  = "intrusive"
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
			a := New(runner, 0)
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
			wantVal: strengthContract,
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
	indexer, pkg, lang, ok, err := New(runner, 0).detectIndexer(context.Background(), dir)
	if err != nil {
		t.Fatalf("detectIndexer: unexpected error: %v", err)
	}
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
	indexer, pkg, lang, ok, err := New(runner, 0).detectIndexer(context.Background(), dir)
	if err != nil {
		t.Fatalf("detectIndexer: unexpected error: %v", err)
	}
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

// TestStrengths_Timeout asserts that the per-analyzer watchdog fires and returns
// StatusTimedOut coverage with nil error and no deadlock.
func TestStrengths_Timeout(t *testing.T) {
	// Minimal Go project: go.mod lets detectIndexer pick scip-go.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/timeout-test\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Runner detects scip-go and uv but blocks forever on Run.
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerGo || tool == "uv" {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(ctx context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			<-ctx.Done()
			return toolrun.Output{}, ctx.Err()
		},
	}

	a := New(runner, 10*time.Millisecond)
	_, cov, err := a.Strengths(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusTimedOut {
		t.Errorf("status = %q, want %q", cov.Status, diagnostic.StatusTimedOut)
	}
	if cov.Reason != reasonTimedOut {
		t.Errorf("reason = %q, want %q", cov.Reason, reasonTimedOut)
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

// TestParsePythonStrengthFixture verifies that parseReaderEdges correctly maps each
// Python symbol kind to the expected Balanced-Coupling strength.
// The fixture is a canned JSON representing what scip_reader.py emits for Python:
//   - function/method target → "functional"
//   - concrete class target → "model"
//   - Protocol/ABC subclass target → "contract"
//   - _-prefixed (private) target → "intrusive"
//
// TypedDict and @dataclass are concrete classes (type suffix "#") and therefore
// map to "model" — the "contract/model" grouping in the plan covers both.
// scip_reader.py derives these mappings from SCIP symbol descriptor suffixes and
// is_implementation relationships; this test verifies the Go parsing of its output.
func TestParsePythonStrengthFixture(t *testing.T) {
	// Canned JSON: what scip_reader.py would emit for a Python project with
	// cross-module references to symbols of each BC-relevant kind.
	const fixture = `{"edges":[` +
		`{"from":"myapp.a","to":"myapp.services","strength":"functional"},` +
		`{"from":"myapp.a","to":"myapp.models","strength":"model"},` +
		`{"from":"myapp.a","to":"myapp.interfaces","strength":"contract"},` +
		`{"from":"myapp.a","to":"myapp._internal","strength":"intrusive"}` +
		`]}`

	m, err := parseReaderEdges([]byte(fixture))
	if err != nil {
		t.Fatalf("parseReaderEdges: %v", err)
	}
	if len(m) != 4 {
		t.Errorf("strength map len = %d, want 4", len(m))
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		// function/method target (e.g. "myapp/services/get_data()." → suffix "method"):
		{"function target → functional", "myapp.a\x00myapp.services", strengthFunctional},
		// concrete class target (e.g. "myapp/models/User#" → suffix "type"):
		{"class target → model", "myapp.a\x00myapp.models", strengthModel},
		// Protocol/ABC subclass (is_implementation → ABC/Protocol → contract set):
		{"Protocol/ABC target → contract", "myapp.a\x00myapp.interfaces", strengthContract},
		// private module (_-prefixed segment → _is_private → intrusive):
		{"private target → intrusive", "myapp.a\x00myapp._internal", strengthIntrusive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m[tc.key]; got != tc.want {
				t.Errorf("strength[%q] = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// TestStrengths_PythonCannedFixture tests the full Strengths() pipeline for a
// Python project using a mocked runner — no live scip-python or uv required.
// The mock: detects scip-python and uv, creates the index file on demand, and
// returns a canned JSON (what scip_reader.py would emit). Verifies that
// Strengths() returns OK coverage and the expected module→strength map.
func TestStrengths_PythonCannedFixture(t *testing.T) {
	dir := t.TempDir()
	// Minimal Python project: pyproject.toml + flat-layout package.
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"myapp\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(dir, "myapp")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, pyInitFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Canned scip_reader.py JSON output: one edge per Python strength kind.
	const cannedJSON = `{"edges":[` +
		`{"from":"myapp.a","to":"myapp.services","strength":"functional"},` +
		`{"from":"myapp.a","to":"myapp.models","strength":"model"},` +
		`{"from":"myapp.a","to":"myapp.interfaces","strength":"contract"},` +
		`{"from":"myapp.a","to":"myapp._internal","strength":"intrusive"}` +
		`],"symbols":[],"symbol_refs":[],"intra_refs":[]}`

	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerPython || tool == "uv" {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch cmd.Name {
			case indexerPython:
				// Create the index file at the path supplied via --output so
				// os.Stat succeeds and the pipeline proceeds to the reader step.
				for i, arg := range cmd.Args {
					if arg == flagOutput && i+1 < len(cmd.Args) {
						if err := os.WriteFile(cmd.Args[i+1], []byte("scip-fake"), 0o600); err != nil {
							return toolrun.Output{}, err
						}
					}
				}
				return toolrun.Output{ExitCode: 0}, nil
			case "uv":
				return toolrun.Output{Stdout: []byte(cannedJSON), ExitCode: 0}, nil
			}
			return toolrun.Output{}, nil
		},
	}

	a := New(runner, 0)
	m, cov, err := a.Strengths(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("Strengths: %v", err)
	}
	if cov.Status != diagnostic.StatusOK {
		t.Errorf("cov.Status = %q, want %q", cov.Status, diagnostic.StatusOK)
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"function → functional", "myapp.a\x00myapp.services", strengthFunctional},
		{"class → model", "myapp.a\x00myapp.models", strengthModel},
		{"Protocol/ABC → contract", "myapp.a\x00myapp.interfaces", strengthContract},
		{"private → intrusive", "myapp.a\x00myapp._internal", strengthIntrusive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m[tc.key]; got != tc.want {
				t.Errorf("strength[%q] = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// TestStrengths_PythonAbsent verifies that when scip-python is not available,
// Strengths() returns an absent coverage and an empty map — so Python grimp edges
// retain their StrengthHint="" (abstaining) set by the extractor. This upholds
// the abstain-not-fake invariant: no SCIP evidence → no strength assigned.
func TestStrengths_PythonAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"myapp\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false // no SCIP indexer, no uv
		},
	}
	a := New(runner, 0)
	m, cov, err := a.Strengths(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("Strengths: unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusAbsent {
		t.Errorf("cov.Status = %q, want %q", cov.Status, diagnostic.StatusAbsent)
	}
	if len(m) != 0 {
		t.Errorf("strength map non-empty when SCIP absent: %v", m)
	}
}

// TestStrengths_EmptyIndex verifies that when the SCIP pipeline succeeds but the
// reader emits zero edges, Strengths() returns StatusPartial (not StatusOK) with
// an actionable reason. This prevents a silent ok when the index was built but
// contained no cross-module occurrences (path-case mismatch, wrong indexer, etc.).
func TestStrengths_EmptyIndex(t *testing.T) {
	dir := t.TempDir()
	// Minimal Python project so detectIndexer picks scip-python.
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"myapp\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(dir, "myapp")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, pyInitFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Reader returns an empty edges array — simulates a built but vacuous index.
	const emptyJSON = `{"edges":[],"symbols":[],"symbol_refs":[],"intra_refs":[]}`

	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerPython || tool == "uv" {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch cmd.Name {
			case indexerPython:
				// Create the index file so os.Stat succeeds.
				for i, arg := range cmd.Args {
					if arg == flagOutput && i+1 < len(cmd.Args) {
						if err := os.WriteFile(cmd.Args[i+1], []byte("scip-fake"), 0o600); err != nil {
							return toolrun.Output{}, err
						}
					}
				}
				return toolrun.Output{ExitCode: 0}, nil
			case "uv":
				return toolrun.Output{Stdout: []byte(emptyJSON), ExitCode: 0}, nil
			}
			return toolrun.Output{}, nil
		},
	}

	a := New(runner, 0)
	m, cov, err := a.Strengths(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("Strengths: unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusPartial {
		t.Errorf("cov.Status = %q, want %q (empty index must be partial, not ok)", cov.Status, diagnostic.StatusPartial)
	}
	if !strings.Contains(cov.Reason, "empty index") {
		t.Errorf("cov.Reason = %q, want it to mention \"empty index\"", cov.Reason)
	}
	if len(m) != 0 {
		t.Errorf("strength map non-empty for empty index: %v", m)
	}
}

// TestCargoWorkspaceMembers_TimeoutMapsToStatusTimedOut verifies that when cargo
// metadata returns context.DeadlineExceeded (inner cap fired before outer watchdog),
// Strengths returns StatusTimedOut — not StatusAbsent — so operators know to raise
// tools.scip.timeout rather than thinking Rust SCIP is simply unsupported.
func TestCargoWorkspaceMembers_TimeoutMapsToStatusTimedOut(t *testing.T) {
	dir := t.TempDir()
	// Virtual workspace (no [package]) so detectIndexer calls cargoWorkspaceMembers.
	manifest := "[workspace]\nmembers = [\"crate-a\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == indexerRust || tool == toolCargo {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == toolCargo {
				// Simulate the inner 60 s cap firing before the outer watchdog.
				return toolrun.Output{}, context.DeadlineExceeded
			}
			return toolrun.Output{}, nil
		},
	}
	// adapter timeout=0 → outer watchdog uses defaultTimeout (20 min), stays inert.
	// Only the inner cargo cap fires (returned directly by the mock).
	a := New(runner, 0)
	_, cov, err := a.Strengths(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusTimedOut {
		t.Errorf("status = %q, want %q (cargo timeout must not be absorbed as absent)", cov.Status, diagnostic.StatusTimedOut)
	}
}
