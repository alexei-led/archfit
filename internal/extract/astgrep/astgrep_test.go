package astgrep_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	patternUnsafe  = "unsafe-pointer-cast"
	patternReflect = "reflect-use"
	toolAstGrep    = "ast-grep"
	dupFile        = "pkg/dup/dup.go"
)

// fixtureBytes reads testdata/astgrep/sg_output.json relative to the repo root.
func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "astgrep", "sg_output.json")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	b, err := os.ReadFile(abs) //nolint:gosec // test fixture path is repo-internal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// sgEntry builds a minimal sg JSON match entry for inline fixture construction.
// Column is fixed at 0; only file, line, and pattern identity matter for these tests.
func sgEntry(text, file, ruleID string, line int) map[string]any {
	return map[string]any{
		"text": text,
		"range": map[string]any{
			"start": map[string]any{"line": line, "column": 0},
			"end":   map[string]any{"line": line, "column": len(text)},
		},
		"file": file,
		"rule": map[string]any{"id": ruleID},
	}
}

// marshalEntries serialises a slice of sgEntry results to JSON bytes.
func marshalEntries(t *testing.T, entries []map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entries: %v", err)
	}
	return b
}

// presentRunner returns a RunnerMock where Detect reports "sg" as present and
// Run always returns the given bytes.
func presentRunner(output []byte) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: "sg", Path: "/usr/bin/sg"}, true
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: output}, nil
		},
	}
}

// absentRunner returns a RunnerMock where Detect always reports "sg" as missing.
func absentRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
}

var testScope = scope.Scope{Root: "/repo", Mode: scope.ModeFull}

var singlePatternCfg = config.PatternConfig{
	{ID: patternUnsafe, Lang: "go", Rule: "unsafe.Pointer($X)"},
}

func TestFind_ParsesFixtureJSON(t *testing.T) {
	runner := presentRunner(fixtureBytes(t))
	a := astgrep.New(runner)

	matches, cov, err := a.Find(context.Background(), testScope, singlePatternCfg)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want %q", cov.Status, "ok")
	}
	if cov.Tool != toolAstGrep {
		t.Errorf("cov.Tool = %q, want %q", cov.Tool, toolAstGrep)
	}
	// Fixture has two matches in two distinct files.
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}

	// Results must be sorted by (file, line); bar < foo alphabetically.
	m0 := matches[0]
	if m0.File != "internal/bar/bar.go" {
		t.Errorf("matches[0].File = %q, want internal/bar/bar.go", m0.File)
	}
	if m0.Line != 26 { // fixture line=25 (0-based) → 26 (1-based)
		t.Errorf("matches[0].Line = %d, want 26", m0.Line)
	}
	if m0.Column != 8 {
		t.Errorf("matches[0].Column = %d, want 8", m0.Column)
	}
	if m0.Pattern != patternUnsafe {
		t.Errorf("matches[0].Pattern = %q, want %q", m0.Pattern, patternUnsafe)
	}
	if m0.Text != "unsafe.Pointer(&y)" {
		t.Errorf("matches[0].Text = %q", m0.Text)
	}

	m1 := matches[1]
	if m1.File != "internal/foo/foo.go" {
		t.Errorf("matches[1].File = %q, want internal/foo/foo.go", m1.File)
	}
	if m1.Line != 11 { // fixture line=10 (0-based) → 11 (1-based)
		t.Errorf("matches[1].Line = %d, want 11", m1.Line)
	}
}

func TestFind_AbsentTool_ReturnsEmptyAndAbsentStatus(t *testing.T) {
	a := astgrep.New(absentRunner())

	matches, cov, err := a.Find(context.Background(), testScope, singlePatternCfg)
	if err != nil {
		t.Fatalf("Find with absent tool must not error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for absent tool, got %d", len(matches))
	}
	if cov.Status != "absent" {
		t.Errorf("cov.Status = %q, want absent", cov.Status)
	}
	if cov.Tool != toolAstGrep {
		t.Errorf("cov.Tool = %q, want %q", cov.Tool, toolAstGrep)
	}
}

func TestFind_MultiPattern_MergesResults(t *testing.T) {
	out1 := marshalEntries(t, []map[string]any{
		sgEntry("reflect.TypeOf(x)", "pkg/a/a.go", patternReflect, 5),
	})
	out2 := marshalEntries(t, []map[string]any{
		sgEntry("unsafe.Pointer(&z)", "pkg/b/b.go", patternUnsafe, 2),
	})

	callCount := 0
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: "sg"}, true
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			callCount++
			if callCount == 1 {
				return toolrun.Output{Stdout: out1}, nil
			}
			return toolrun.Output{Stdout: out2}, nil
		},
	}

	cfg := config.PatternConfig{
		{ID: patternReflect, Lang: "go", Rule: "reflect.TypeOf($X)"},
		{ID: patternUnsafe, Lang: "go", Rule: "unsafe.Pointer($X)"},
	}
	a := astgrep.New(runner)
	matches, cov, err := a.Find(context.Background(), testScope, cfg)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	// Sorted: pkg/a/a.go < pkg/b/b.go.
	if matches[0].File != "pkg/a/a.go" {
		t.Errorf("matches[0].File = %q, want pkg/a/a.go", matches[0].File)
	}
	if matches[1].File != "pkg/b/b.go" {
		t.Errorf("matches[1].File = %q, want pkg/b/b.go", matches[1].File)
	}
	if callCount != 2 {
		t.Errorf("Run called %d times, want 2", callCount)
	}
}

func TestFind_Deduplication(t *testing.T) {
	// Two identical (file, line, pattern) entries in a single Run response.
	dupOutput := marshalEntries(t, []map[string]any{
		sgEntry("unsafe.Pointer(&a)", dupFile, patternUnsafe, 9),
		sgEntry("unsafe.Pointer(&a)", dupFile, patternUnsafe, 9),
	})

	runner := presentRunner(dupOutput)
	a := astgrep.New(runner)

	matches, _, err := a.Find(context.Background(), testScope, singlePatternCfg)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("dedup: len(matches) = %d, want 1", len(matches))
	}
	if matches[0].File != dupFile {
		t.Errorf("matches[0].File = %q, want %q", matches[0].File, dupFile)
	}
	if matches[0].Line != 10 { // 0-based 9 → 1-based 10
		t.Errorf("matches[0].Line = %d, want 10", matches[0].Line)
	}
}

func TestFind_EmptyOutput_ReturnsNoMatches(t *testing.T) {
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: "sg"}, true
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: nil}, nil
		},
	}
	a := astgrep.New(runner)
	matches, cov, err := a.Find(context.Background(), testScope, singlePatternCfg)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty stdout, got %d", len(matches))
	}
	if cov.Status != "ok" {
		t.Errorf("cov.Status = %q, want ok", cov.Status)
	}
}

func TestFind_FilesSeen_CountsDistinctFiles(t *testing.T) {
	runner := presentRunner(fixtureBytes(t))
	a := astgrep.New(runner)

	_, cov, err := a.Find(context.Background(), testScope, singlePatternCfg)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	// Fixture has matches in two distinct files.
	if cov.FilesSeen != 2 {
		t.Errorf("FilesSeen = %d, want 2", cov.FilesSeen)
	}
}

func TestAdapter_Name(t *testing.T) {
	a := astgrep.New(absentRunner())
	if a.Name() != toolAstGrep {
		t.Errorf("Name() = %q, want %q", a.Name(), toolAstGrep)
	}
}

// Compile-time interface check.
var _ engine.PatternProvider = (*astgrep.Adapter)(nil)
