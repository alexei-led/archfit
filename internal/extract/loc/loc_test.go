package loc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/syntax"
)

// writeFile creates a file with the given content under dir.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRun_CountsSourceFiles(t *testing.T) {
	root := t.TempDir()

	// Source files that should be counted.
	writeFile(t, root, "pkg/a/a.go", "package a\n\nfunc A() {}\n")           // 3 lines
	writeFile(t, root, "pkg/b/b.py", "def b():\n    pass\n")                 // 2 lines
	writeFile(t, root, "frontend/app.ts", "export const x = 1\n")            // 1 line
	writeFile(t, root, "frontend/comp.tsx", "export default () => null\n\n") // 2 lines

	out, _, cov, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	wantFiles := map[string]int{
		"pkg/a/a.go":        3,
		"pkg/b/b.py":        2,
		"frontend/app.ts":   1,
		"frontend/comp.tsx": 2,
	}
	for rel, wantLines := range wantFiles {
		if got, ok := out[rel]; !ok {
			t.Errorf("missing %q in output", rel)
		} else if got != wantLines {
			t.Errorf("%q: got %d lines, want %d", rel, got, wantLines)
		}
	}

	// Coverage fields.
	if cov.Tool != toolName {
		t.Errorf("coverage tool = %q, want %q", cov.Tool, toolName)
	}
	if cov.FilesSeen != len(out) {
		t.Errorf("FilesSeen = %d, want %d (map len)", cov.FilesSeen, len(out))
	}
	if cov.FilesApplicable != len(out) {
		t.Errorf("FilesApplicable = %d, want %d", cov.FilesApplicable, len(out))
	}
	if cov.Status != diagnostic.StatusOK {
		t.Errorf("Status = %q, want %q", cov.Status, diagnostic.StatusOK)
	}
}

func TestRun_ExcludesTestFiles(t *testing.T) {
	root := t.TempDir()

	// These should NOT be counted.
	writeFile(t, root, "pkg/a/a_test.go", "package a\n")
	writeFile(t, root, "pkg/b/mock_client.go", "package b\n")
	writeFile(t, root, "pkg/c/test_helpers.py", "# helpers\n")
	writeFile(t, root, "pkg/d/utils_test.py", "# test\n")
	writeFile(t, root, "frontend/app.spec.ts", "describe('x', () => {})\n")
	writeFile(t, root, "frontend/types.d.ts", "export type T = string\n")

	// This should be counted.
	writeFile(t, root, "pkg/a/a.go", "package a\n\nfunc A() {}\n")

	out, _, cov, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if _, ok := out["pkg/a/a_test.go"]; ok {
		t.Error("_test.go files must be excluded")
	}
	if _, ok := out["pkg/b/mock_client.go"]; ok {
		t.Error("mock_ prefix files must be excluded")
	}
	if _, ok := out["pkg/c/test_helpers.py"]; ok {
		t.Error("test_ prefix .py files must be excluded")
	}
	if _, ok := out["pkg/d/utils_test.py"]; ok {
		t.Error("_test.py suffix files must be excluded")
	}
	if _, ok := out["frontend/app.spec.ts"]; ok {
		t.Error(".spec. files must be excluded")
	}
	if _, ok := out["frontend/types.d.ts"]; ok {
		t.Error(".d.ts files must be excluded")
	}
	if len(out) != 1 {
		t.Errorf("expected exactly 1 counted file, got %d: %v", len(out), out)
	}
	if cov.FilesSeen != 1 {
		t.Errorf("FilesSeen = %d, want 1", cov.FilesSeen)
	}
}

func TestRun_SkipsSkipDirsAndDotDirs(t *testing.T) {
	root := t.TempDir()

	// Files inside skipped dirs — none should appear.
	skipDirCases := []string{
		"node_modules/lib/index.js",
		"vendor/pkg/x.go",
		"testdata/fixture.go",
		".git/COMMIT_EDITMSG",
		".venv/lib/a.py",
	}
	for _, rel := range skipDirCases {
		writeFile(t, root, rel, "content\n")
	}
	// One real source file.
	writeFile(t, root, "src/main.go", "package main\n")

	out, _, _, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	for _, rel := range skipDirCases {
		// Normalise separator.
		normed := filepath.ToSlash(rel)
		if _, ok := out[normed]; ok {
			t.Errorf("file in skipped dir must not appear: %s", normed)
		}
	}
	if len(out) != 1 {
		t.Errorf("expected exactly 1 file, got %d: %v", len(out), out)
	}
}

func TestRun_CoverageFieldsMatchMapLen(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/a.go", "package a\n")
	writeFile(t, root, "b/b.go", "package b\n")

	out, _, cov, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if cov.FilesSeen != len(out) {
		t.Errorf("FilesSeen %d != map len %d", cov.FilesSeen, len(out))
	}
	if cov.FilesApplicable != len(out) {
		t.Errorf("FilesApplicable %d != map len %d", cov.FilesApplicable, len(out))
	}
	if cov.Unresolved != 0 {
		t.Errorf("Unresolved = %d, want 0", cov.Unresolved)
	}
}

func TestRun_EmptyRoot(t *testing.T) {
	root := t.TempDir()
	out, _, cov, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map for empty root, got %v", out)
	}
	if cov.Status != diagnostic.StatusOK {
		t.Errorf("Status = %q, want ok for empty root", cov.Status)
	}
	if cov.FilesSeen != 0 {
		t.Errorf("FilesSeen = %d, want 0", cov.FilesSeen)
	}
}

// TestRun_SkipsGoModuleCache is the bug-1 regression: a Go module cache
// (<root>/pkg/mod) inside a repo must not be counted as first-party source (its
// 18k-LOC stdlib files otherwise dominate file_structural_weight), while
// similarly-named sibling dirs (pkg/api, pkg/models) must still be counted.
func TestRun_SkipsGoModuleCache(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/mod/golang.org/x/tools@v0.42.0/manifest.go", "package stdlib\n\nvar M = 1\n")
	writeFile(t, root, "pkg/api/api.go", "package api\n\nfunc A() {}\n")
	writeFile(t, root, "pkg/models/user.go", "package models\n\ntype User struct{}\n")

	out, _, _, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	for rel := range out {
		if strings.HasPrefix(rel, "pkg/mod/") {
			t.Errorf("module cache file must not be counted: %s", rel)
		}
	}
	if _, ok := out["pkg/api/api.go"]; !ok {
		t.Error("real source pkg/api/api.go must be counted")
	}
	if _, ok := out["pkg/models/user.go"]; !ok {
		t.Error("pkg/models/user.go must not be over-excluded by the pkg/mod skip")
	}
}

// TestRun_FileClassIndex verifies the FileClassIndex parallel map:
// - production files appear as Production
// - test/generated files appear in the index with the right class
// - LOC map only contains production files
func TestRun_FileClassIndex(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/core/core.go", "package core\n\nfunc F() {}\n")
	writeFile(t, root, "pkg/core/core_test.go", "package core_test\n")
	writeFile(t, root, "pkg/api/api.pb.go", "package api\n")   // generated filename
	writeFile(t, root, "pkg/gen/wire_gen.go", "package gen\n") // _gen suffix

	locMap, classes, _, err := Run(root)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// LOC map must contain only the production file.
	if _, ok := locMap["pkg/core/core.go"]; !ok {
		t.Error("production file must be in LOC map")
	}
	if _, ok := locMap["pkg/core/core_test.go"]; ok {
		t.Error("test file must NOT be in LOC map")
	}
	if _, ok := locMap["pkg/api/api.pb.go"]; ok {
		t.Error("generated file must NOT be in LOC map")
	}

	// FileClassIndex must classify all visited files.
	if got := classes["pkg/core/core.go"]; got != fileclass.Production {
		t.Errorf("core.go class = %q, want production", got)
	}
	if got := classes["pkg/core/core_test.go"]; got != fileclass.Test {
		t.Errorf("core_test.go class = %q, want test", got)
	}
	if got := classes["pkg/api/api.pb.go"]; got != fileclass.Generated {
		t.Errorf("api.pb.go class = %q, want generated", got)
	}
	if got := classes["pkg/gen/wire_gen.go"]; got != fileclass.Generated {
		t.Errorf("wire_gen.go class = %q, want generated", got)
	}
}

// TestRunWithConfig_PathGlobGenerated verifies that config-supplied
// GeneratedGlobs with path patterns (e.g. "gen/**", "**/generated/*.go") match
// via the repo-relative path passed through the loc walk.
// This is a regression test for the abs-path bug: ClassifyFile was previously
// called with the absolute path, so doublestar.Match("gen/**", "/abs/root/gen/foo.go")
// returned false and the pattern silently never matched.
func TestRunWithConfig_PathGlobGenerated(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "gen/client.go", "package gen\n\nfunc C() {}\n")
	writeFile(t, root, "pkg/generated/foo.go", "package generated\n\nfunc F() {}\n")
	writeFile(t, root, "pkg/core/core.go", "package core\n\nfunc A() {}\n")

	cfg := syntax.FileClassConfig{
		GeneratedGlobs: []string{"gen/**", "**/generated/*.go"},
	}
	locMap, classes, _, err := RunWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("RunWithConfig error: %v", err)
	}

	// Both path-glob matched files must be Generated.
	if got := classes["gen/client.go"]; got != fileclass.Generated {
		t.Errorf("gen/client.go class = %q, want generated (gen/** pattern failed to match repo-relative path)", got)
	}
	if got := classes["pkg/generated/foo.go"]; got != fileclass.Generated {
		t.Errorf("pkg/generated/foo.go class = %q, want generated (**/generated/*.go pattern failed to match repo-relative path)", got)
	}
	// Neither must appear in LOC map.
	if _, ok := locMap["gen/client.go"]; ok {
		t.Error("gen/client.go must not be in LOC map (generated)")
	}
	if _, ok := locMap["pkg/generated/foo.go"]; ok {
		t.Error("pkg/generated/foo.go must not be in LOC map (generated)")
	}
	// Production file unaffected.
	if got := classes["pkg/core/core.go"]; got != fileclass.Production {
		t.Errorf("core.go class = %q, want production", got)
	}
}

// TestRunWithConfig_PathGlobTest verifies that config-supplied TestGlobs with
// path patterns match via the repo-relative path.
func TestRunWithConfig_PathGlobTest(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "testutil/helpers.go", "package testutil\n\nfunc H() {}\n")
	writeFile(t, root, "pkg/core/core.go", "package core\n\nfunc A() {}\n")

	cfg := syntax.FileClassConfig{
		TestGlobs: []string{"testutil/**"},
	}
	locMap, classes, _, err := RunWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("RunWithConfig error: %v", err)
	}

	if got := classes["testutil/helpers.go"]; got != fileclass.Test {
		t.Errorf("testutil/helpers.go class = %q, want test (testutil/** pattern failed to match repo-relative path)", got)
	}
	if _, ok := locMap["testutil/helpers.go"]; ok {
		t.Error("testutil/helpers.go must not be in LOC map (test file)")
	}
	if got := classes["pkg/core/core.go"]; got != fileclass.Production {
		t.Errorf("core.go class = %q, want production", got)
	}
}

// TestRunWithConfig_CustomMockPattern verifies that a config-supplied mock
// pattern reclassifies matching files as Generated, reproducing the pumba
// fixture where mocks/ files inflate panic_density.
func TestRunWithConfig_CustomMockPattern(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/core/core.go", "package core\n\nfunc F() {}\n")
	// fake_ prefix not in built-in patterns; config must extend detection.
	writeFile(t, root, "pkg/fakes/fake_client.go", "package fakes\n")

	cfg := syntax.FileClassConfig{
		MockFrameworks: []string{"fake_"},
	}
	locMap, classes, _, err := RunWithConfig(root, cfg)
	if err != nil {
		t.Fatalf("RunWithConfig error: %v", err)
	}

	// Custom mock must be Generated, not Production.
	if got := classes["pkg/fakes/fake_client.go"]; got != fileclass.Generated {
		t.Errorf("fake_client.go class = %q, want generated", got)
	}
	// Must NOT appear in LOC map.
	if _, ok := locMap["pkg/fakes/fake_client.go"]; ok {
		t.Error("custom mock file must not be in LOC map")
	}
	// Production file unaffected.
	if got := classes["pkg/core/core.go"]; got != fileclass.Production {
		t.Errorf("core.go class = %q, want production", got)
	}
}
