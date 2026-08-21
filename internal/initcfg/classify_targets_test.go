package initcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mkFile creates a regular file at path (with all parent dirs).
func mkFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkFile MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("mkFile WriteFile: %v", err)
	}
}

// mkDir creates a directory at path.
func mkDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkDir: %v", err)
	}
}

func TestBuildClassifyTargets_GoSubdirOnly(t *testing.T) {
	// A directory that exists but contains no regular files — Files must be empty,
	// no crash.
	root := t.TempDir()
	mkDir(t, filepath.Join(root, "internal", "classify"))

	mods := []ModuleDef{
		{Name: testClassify, Paths: []string{testClassifyPath}},
	}

	targets := BuildClassifyTargets(root, mods)

	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	ct := targets[0]
	if ct.Name != testClassify {
		t.Errorf("Name = %q, want %q", ct.Name, testClassify)
	}
	if len(ct.Files) != 0 {
		t.Errorf("Files = %v, want empty (directory has no files)", ct.Files)
	}
}

func TestBuildClassifyTargets_TSGlobWithFiles(t *testing.T) {
	// TS-style glob dir with files; expect basenames resolved, sorted.
	root := t.TempDir()
	dir := filepath.Join(root, "src", "gateway")
	mkFile(t, filepath.Join(dir, "index.ts"))
	mkFile(t, filepath.Join(dir, "auth.ts"))
	mkFile(t, filepath.Join(dir, "users.ts"))
	mkDir(t, filepath.Join(dir, "subpkg")) // directory — must be skipped

	mods := []ModuleDef{
		{Name: "gateway", Paths: []string{"src/gateway/**"}},
	}

	targets := BuildClassifyTargets(root, mods)

	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	ct := targets[0]
	want := []string{"auth.ts", "index.ts", "users.ts"}
	if !equalSlice(ct.Files, want) {
		t.Errorf("Files = %v, want %v", ct.Files, want)
	}
}

func TestBuildClassifyTargets_TSGlobSkipsDotUnderscore(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src", "transport")
	mkFile(t, filepath.Join(dir, "routes.ts"))
	mkFile(t, filepath.Join(dir, ".hidden"))
	mkFile(t, filepath.Join(dir, "_internal.ts"))

	mods := []ModuleDef{
		{Name: "transport", Paths: []string{"src/transport/**"}},
	}

	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	want := []string{"routes.ts"}
	if !equalSlice(ct.Files, want) {
		t.Errorf("Files = %v, want %v", ct.Files, want)
	}
}

func TestBuildClassifyTargets_PythonDottedAbsent(t *testing.T) {
	// Python dotted path where the directory does not exist → Files empty.
	root := t.TempDir()

	mods := []ModuleDef{
		{Name: "ingestion", Paths: []string{"ccgram.ingestion", "ccgram.ingestion.*"}},
	}

	targets := BuildClassifyTargets(root, mods)

	ct := targets[0]
	if len(ct.Files) != 0 {
		t.Errorf("Files = %v, want empty (dir absent)", ct.Files)
	}
}

func TestBuildClassifyTargets_PythonDottedPresent(t *testing.T) {
	// Python dotted path where the directory exists with files.
	root := t.TempDir()
	dir := filepath.Join(root, "ccgram", "ingestion")
	mkFile(t, filepath.Join(dir, "base.py"))
	mkFile(t, filepath.Join(dir, "auth.py"))

	mods := []ModuleDef{
		{Name: "ingestion", Paths: []string{"ccgram.ingestion", "ccgram.ingestion.*"}},
	}

	targets := BuildClassifyTargets(root, mods)

	ct := targets[0]
	want := []string{"auth.py", "base.py"}
	if !equalSlice(ct.Files, want) {
		t.Errorf("Files = %v, want %v", ct.Files, want)
	}
}

func TestBuildClassifyTargets_CapAt20(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src", "bigmod")
	for i := range 25 {
		mkFile(t, filepath.Join(dir, fmt.Sprintf("file%02d.ts", i)))
	}

	mods := []ModuleDef{
		{Name: "bigmod", Paths: []string{"src/bigmod/**"}},
	}

	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if len(ct.Files) != maxClassifyFiles {
		t.Errorf("Files len = %d, want %d (cap)", len(ct.Files), maxClassifyFiles)
	}
	// Verify they are sorted.
	for i := 1; i < len(ct.Files); i++ {
		if ct.Files[i] < ct.Files[i-1] {
			t.Errorf("Files not sorted at index %d: %v", i, ct.Files)
			break
		}
	}
}

func TestBuildClassifyTargets_DeterministicOrdering(t *testing.T) {
	// Two modules; order in output must match mods slice order across repeated calls.
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "internal", "alpha", "a.go"))
	mkFile(t, filepath.Join(root, "internal", "sigma", "s.go"))

	mods := []ModuleDef{
		{Name: "alpha", Paths: []string{"internal/alpha/**"}},
		{Name: "sigma", Paths: []string{"internal/sigma/**"}},
	}

	for range 5 {
		targets := BuildClassifyTargets(root, mods)
		if len(targets) != 2 {
			t.Fatalf("want 2 targets, got %d", len(targets))
		}
		if targets[0].Name != "alpha" || targets[1].Name != "sigma" {
			t.Errorf("order unstable: got %q, %q", targets[0].Name, targets[1].Name)
		}
		if !equalSlice(targets[0].Files, []string{"a.go"}) {
			t.Errorf("alpha Files = %v", targets[0].Files)
		}
		if !equalSlice(targets[1].Files, []string{"s.go"}) {
			t.Errorf("sigma Files = %v", targets[1].Files)
		}
	}
}

func TestBuildClassifyTargets_MissingDir(t *testing.T) {
	// Glob pointing to a directory that does not exist: no crash, Files empty.
	root := t.TempDir()

	mods := []ModuleDef{
		{Name: "absent", Paths: []string{"internal/absent/**"}},
	}

	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if len(ct.Files) != 0 {
		t.Errorf("Files = %v, want empty (dir missing)", ct.Files)
	}
}

func TestBuildClassifyTargets_PathsPassedThrough(t *testing.T) {
	// Paths on the target must equal ModuleDef.Paths verbatim.
	root := t.TempDir()

	mods := []ModuleDef{
		{Name: "foo", Paths: []string{"internal/foo/**", "internal/foo_extra/**"}},
	}

	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if !equalSlice(ct.Paths, mods[0].Paths) {
		t.Errorf("Paths = %v, want %v", ct.Paths, mods[0].Paths)
	}
}

// testPkg is a package name repeated across the src-layout resolver tests.
const testPkg = "mypkg"

// TestResolveDir_SrcLayoutDottedPath verifies that a Python src-layout dotted
// path (e.g. "mypkg.handlers") is resolved under <root>/src when the directory
// does not exist directly under root.
func TestResolveDir_SrcLayoutDottedPath(t *testing.T) {
	root := t.TempDir()
	// Place the package under src/ (src-layout).
	dir := filepath.Join(root, "src", testPkg, "handlers")
	mkFile(t, filepath.Join(dir, "base.py"))
	// __init__.py starts with '_' so resolveFiles skips it — not added here.

	mods := []ModuleDef{
		{Name: "handlers", Paths: []string{"mypkg.handlers", "mypkg.handlers.*"}},
	}
	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if !equalSlice(ct.Files, []string{"base.py"}) {
		t.Errorf("Files = %v, want [base.py] (src-layout dotted path)", ct.Files)
	}
}

// TestResolveDir_SrcLayoutSingleSegment verifies that a bare single-segment
// package name is resolved under <root>/src when it does not exist under root.
func TestResolveDir_SrcLayoutSingleSegment(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src", testPkg)
	mkFile(t, filepath.Join(dir, "main.py"))

	mods := []ModuleDef{
		{Name: testPkg, Paths: []string{testPkg}},
	}
	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if !equalSlice(ct.Files, []string{"main.py"}) {
		t.Errorf("Files = %v, want [main.py] (src-layout single-segment)", ct.Files)
	}
}

// TestResolveDir_SingleSegmentTopLevel verifies that a bare single-segment
// package name under root (flat layout) is resolved.
func TestResolveDir_SingleSegmentTopLevel(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, testPkg)
	mkFile(t, filepath.Join(dir, "core.py"))

	mods := []ModuleDef{
		{Name: testPkg, Paths: []string{testPkg}},
	}
	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if !equalSlice(ct.Files, []string{"core.py"}) {
		t.Errorf("Files = %v, want [core.py] (top-level single-segment)", ct.Files)
	}
}

// TestResolveDir_NonexistentSingleSegment verifies that a bare single-segment
// name that exists under neither root nor root/src degrades to empty Files.
func TestResolveDir_NonexistentSingleSegment(t *testing.T) {
	root := t.TempDir()
	mods := []ModuleDef{
		{Name: testGhostModule, Paths: []string{testGhostModule}},
	}
	targets := BuildClassifyTargets(root, mods)
	ct := targets[0]
	if len(ct.Files) != 0 {
		t.Errorf("Files = %v, want empty (nonexistent path)", ct.Files)
	}
}

// equalSlice reports whether two string slices are equal element by element.
func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	// Iterate over indices of b to avoid a gosec G602 false-positive when
	// indexing the second slice inside a range-over-first loop.
	for i, v := range b {
		if a[i] != v {
			return false
		}
	}
	return true
}
