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

// sgSkipGuard skips t if sg is absent from PATH or is not ast-grep
// (util-linux ships an unrelated /usr/bin/sg).
func sgSkipGuard(t *testing.T) {
	t.Helper()
	sgPath, err := exec.LookPath("sg")
	if err != nil {
		t.Skip("sg not found on PATH — skipping integration test")
	}
	out, err := exec.Command(sgPath, "--version").Output() //nolint:gosec // path from LookPath
	if err != nil || !strings.Contains(string(out), "ast-grep") {
		t.Skipf("sg at %s is not ast-grep (version output: %q) — skipping", sgPath, string(out))
	}
}

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
	sgSkipGuard(t)

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

// TestSyntaxIntegration_AllRuleFiles verifies that every embedded language rule
// file is valid YAML that sg accepts AND produces at least one fact against its
// per-language fixture. This is the systemic guard: a malformed rule file
// (e.g. an unquoted YAML-special character like '#' in a pattern value) will
// cause sg to exit non-zero with empty stdout, which this test catches.
//
// Per-kind assertions: each language has a set of expected kinds that its
// fixture must produce. This catches "rule parses but never matches" (e.g. the
// rs-attribute bug where #[$NAME($$$)] inside has: never matched any real
// attribute_item). Any expected kind with 0 facts fails the sub-test.
//
// Each sub-test is independent; adding a new language rule file requires only a
// new fixture and a new table row.
func TestSyntaxIntegration_AllRuleFiles(t *testing.T) {
	sgSkipGuard(t)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixtureDir := filepath.Join(filepath.Dir(thisFile), "testdata", "integration")

	// spotName is a fact Name that must appear for each language so we know the
	// rule file fired on real source, not just parsed without matching anything.
	// requiredKinds lists every Kind that the fixture contains at least one of —
	// the test fails if any of these yields 0 facts (rule parsed but never matched).
	tests := []struct {
		lang          string
		spotName      string
		spotKind      string
		requiredKinds []string
	}{
		// Go: fixture.go exports func Hello + BigStruct (struct_field) + PanicFixture (panic_op);
		// fixture_test_import.go imports testify/mock;
		// fixture_type_leak.go (//go:build ignore) exercises go-type-leak (type_leak kind).
		// Fixture covers: function, struct, interface, test_import, struct_field, panic_op, type_leak (see fixture*.go).
		{
			lang:          "go",
			spotName:      "Hello",
			spotKind:      kindFunctionStr,
			requiredKinds: []string{kindFunctionStr, kindStructStr, kindInterfaceStr, kindTestImportStr, kindStructFieldStr, kindPanicOpStr, kindTypeLeakStr},
		},
		// TypeScript: fixture.ts exports func greet + @Controller decorator + express route + jest import.
		// Fixture covers: function, class, interface, annotation, route, test_import (see fixture.ts).
		{
			lang:          "typescript",
			spotName:      "greet",
			spotKind:      kindFunctionStr,
			requiredKinds: []string{kindFunctionStr, kindClassStr, kindInterfaceStr, kindAnnotStr, kindRouteStr, kindTestImportStr},
		},
		// Python: fixture.py declares func process + @staticmethod decorator + fastapi route + pytest import +
		// lazy_loader (import os inside function) + lazy_from_loader (from pathlib import Path inside function).
		// Fixture covers: function, class, annotation, route, test_import, lazy_import (see fixture.py).
		{
			lang:          "python",
			spotName:      "process",
			spotKind:      kindFunctionStr,
			requiredKinds: []string{kindFunctionStr, kindClassStr, kindAnnotStr, kindRouteStr, kindTestImportStr, kindLazyImportStr},
		},
		// Rust: fixture.rs exports func create_widget + #[derive(Debug, Clone)] attribute + mockall import +
		// unsafe block, UnsafeCell, raw cast (safety surface fixture) + MultiField struct (struct_field) +
		// fixture_panics with unwrap/expect/panic! (panic_op) +
		// global-state fixture: static mut GLOBAL_COUNTER, AtomicU32 ID_GENERATOR, OnceLock REGISTRY.
		// Fixture covers: function, struct, enum, interface, method, annotation, test_import, unsafe_op,
		//                 struct_field, panic_op, global_state.
		// annotation (rs-attribute) was the previously-broken kind — it must now yield ≥1 match.
		{
			lang:          "rust",
			spotName:      "create_widget",
			spotKind:      kindFunctionStr,
			requiredKinds: []string{kindFunctionStr, kindStructStr, kindEnumStr, kindInterfaceStr, kindMethodStr, kindAnnotStr, kindTestImportStr, kindUnsafeOpStr, kindStructFieldStr, kindPanicOpStr, kindGlobalStateStr},
		},
	}

	runner := toolrun.New()
	a := astgrep.New(runner)
	s := scope.Scope{Root: fixtureDir}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.lang, func(t *testing.T) {
			facts, cov, err := a.Syntax(context.Background(), s, []string{tc.lang})
			if err != nil {
				t.Fatalf("Syntax(%q) returned unexpected error: %v", tc.lang, err)
			}
			if cov.Status == statusAbsentStr {
				t.Skip("sg reported absent from within Syntax() — environment inconsistency")
			}
			// Non-ok status means the rule file was rejected by sg (YAML error,
			// exit non-zero with empty stdout). This is the key regression check.
			if cov.Status != "ok" {
				t.Fatalf("Syntax(%q) returned non-ok coverage status=%q reason=%q — rule file likely rejected by sg", tc.lang, cov.Status, cov.Reason)
			}
			if len(facts) == 0 {
				t.Fatalf("Syntax(%q) returned 0 facts for fixture with exported declarations", tc.lang)
			}
			// Spot-check: the expected declaration must appear.
			found := false
			for _, f := range facts {
				if f.Name == tc.spotName && f.Kind == tc.spotKind {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Syntax(%q): expected fact Name=%q Kind=%q; got %d facts: %+v", tc.lang, tc.spotName, tc.spotKind, len(facts), facts)
			}
			// Per-kind assertion: every expected kind must yield at least one fact.
			// This catches rules that parse correctly but never match real code.
			kindCount := make(map[string]int, len(facts))
			for _, f := range facts {
				kindCount[f.Kind]++
			}
			for _, wantKind := range tc.requiredKinds {
				if kindCount[wantKind] == 0 {
					t.Errorf("Syntax(%q): expected ≥1 fact of Kind=%q but got 0 — rule for this kind matches nothing in the fixture", tc.lang, wantKind)
				}
			}
			// FilesSeen must be > 0 when facts were produced.
			if cov.FilesSeen == 0 {
				t.Errorf("Syntax(%q): FilesSeen=0 but %d facts were produced — coverage counter not set", tc.lang, len(facts))
			}
		})
	}
}
