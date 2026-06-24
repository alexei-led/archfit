package astgrep_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// TestSyntaxIntegration_JSONShape runs the real sg binary against a small Go
// fixture and asserts that the JSON shape archfit depends on is present:
//   - ruleId   — rule identifier string
//   - range    — object with start.line and end.line (0-based integers)
//   - metaVariables — object with a "single" sub-object holding captured names
//
// The test skips cleanly when sg is absent from PATH or when the resolved
// binary is not ast-grep (util-linux ships an unrelated /usr/bin/sg).
func TestSyntaxIntegration_JSONShape(t *testing.T) {
	t.Helper()

	// --- skip guard: sg must be on PATH ---
	sgPath, err := exec.LookPath("sg")
	if err != nil {
		t.Skip("sg not found on PATH — skipping integration test")
	}

	// --- skip guard: must be ast-grep, not util-linux sg ---
	out, err := exec.Command(sgPath, "--version").Output() //nolint:gosec // path from LookPath
	if err != nil || !strings.Contains(string(out), "ast-grep") {
		t.Skipf("sg at %s is not ast-grep (version output: %q) — skipping", sgPath, string(out))
	}

	// --- locate fixture directory relative to this test file ---
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixtureDir := filepath.Join(filepath.Dir(thisFile), "testdata", "integration")

	// --- run Syntax() via the real ToolRunner ---
	runner := toolrun.New()
	a := astgrep.New(runner)

	s := scope.Scope{Root: fixtureDir}
	facts, cov, err := a.Syntax(context.Background(), s, []string{"go"})
	if err != nil {
		t.Fatalf("Syntax() returned unexpected error: %v", err)
	}
	if cov.Status == statusAbsentStr {
		t.Skip("sg reported absent from within Syntax() — environment inconsistency")
	}

	// --- assert JSON shape produced at least one fact ---
	if len(facts) == 0 {
		t.Fatal("Syntax() returned 0 facts for fixture with exported Go declarations")
	}

	// --- validate the shape archfit depends on per fact ---
	for _, f := range facts {
		// ruleId → Kind must be non-empty (parser consumed ruleId)
		if f.Kind == "" {
			t.Errorf("fact %+v: Kind is empty — ruleId was not parsed", f)
		}
		// range: StartLine must be positive (0-based sg line + 1 in adapter)
		if f.StartLine <= 0 {
			t.Errorf("fact %+v: StartLine = %d, want > 0 (range.start.line missing/zero)", f, f.StartLine)
		}
		// range: EndLine must be >= StartLine
		if f.EndLine < f.StartLine {
			t.Errorf("fact %+v: EndLine %d < StartLine %d (range.end.line missing)", f, f.EndLine, f.StartLine)
		}
		// metaVariables.single must have been consumed — Name is non-empty
		if f.Name == "" {
			t.Errorf("fact %+v: Name is empty — metaVariables.single.$NAME not captured", f)
		}
		// File must be non-empty and relative
		if f.File == "" {
			t.Errorf("fact %+v: File is empty", f)
		}
	}

	// Spot-check: fixture defines exactly one exported function "Hello".
	// This validates that our embedded Go rules fire on real source.
	found := false
	for _, f := range facts {
		if f.Name == "Hello" && f.Kind == kindFunctionStr {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a fact with Name=%q Kind=%q; got facts: %+v", "Hello", "function", facts)
	}
}
