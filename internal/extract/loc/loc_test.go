package loc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
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

	out, cov, err := Run(root)
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

	out, cov, err := Run(root)
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

	out, _, err := Run(root)
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

	out, cov, err := Run(root)
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
	out, cov, err := Run(root)
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

	out, _, err := Run(root)
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
