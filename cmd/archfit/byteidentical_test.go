package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Fixture dirs relative to this source file (cmd/archfit/).
const (
	fixtureSingleModule       = "../../internal/extract/golang/testdata/single-module"
	fixtureOneMemberWorkspace = "../../internal/extract/golang/testdata/one-member-workspace"
)

// TestByteIdentical_SingleModule pins the committed JSON baseline for a
// minimal single-module Go fixture: any diff is a regression. It is the
// detector for path-plumbing changes that leak into rendered output — a
// wrongly derived scan root, for instance, shows up as a spurious `--root` in
// every agent_tasks[].validate string and nowhere else.
func TestByteIdentical_SingleModule(t *testing.T) {
	t.Parallel()
	runByteIdenticalTest(t, fixtureSingleModule)
}

// TestByteIdentical_OneMemberWorkspace pins the committed JSON baseline for a
// go.work fixture with one real member plus one excluded (testdata/extra)
// member: only the surviving member's packages are loaded.
func TestByteIdentical_OneMemberWorkspace(t *testing.T) {
	t.Parallel()
	runByteIdenticalTest(t, fixtureOneMemberWorkspace)
}

// runByteIdenticalTest materialises the fixture into a fresh temp git repo,
// runs archfit analyze --format json in-process, normalises volatile
// fields (absolute paths), and diffs the result against baseline.json beside
// the fixture.
//
// First run (no baseline.json): the file is written so the developer can
// review and commit it. Subsequent runs compare and fail on any diff — never
// delete a committed baseline.json to get green; that makes this test pass
// vacuously by re-recording whatever the code now emits.
func runByteIdenticalTest(t *testing.T, fixtureRelPath string) {
	t.Helper()

	absFixture, root := materializeFixtureRepo(t, fixtureRelPath)
	got := runAnalyzeNormalized(t, root)

	// Read or bootstrap the committed baseline.
	baselinePath := filepath.Join(absFixture, "baseline.json")
	want, readErr := os.ReadFile(baselinePath) //nolint:gosec // fixture path constructed from runtime.Caller
	if os.IsNotExist(readErr) {
		// First run: write the baseline for review and commit.
		if writeErr := os.WriteFile(baselinePath, got, 0o600); writeErr != nil {
			t.Fatalf("write baseline %s: %v", baselinePath, writeErr)
		}
		t.Logf("baseline written to %s — review and commit", baselinePath)
		return
	}
	if readErr != nil {
		t.Fatalf("read baseline %s: %v", baselinePath, readErr)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs from baseline:\n%s", firstDiffLine(string(want), string(got)))
	}
}

// TestByteIdentical_ColdWarmNoCache pins the fact-cache correctness gate: on
// the SAME materialized tree, a second (warm, cache-populated) run and a
// --refresh run must produce output byte-identical to the first (cold) run.
// A cache hit may never change what archfit reports.
func TestByteIdentical_ColdWarmNoCache(t *testing.T) {
	t.Parallel()
	_, root := materializeFixtureRepo(t, fixtureSingleModule)

	cold := runAnalyzeNormalized(t, root)
	warm := runAnalyzeNormalized(t, root)
	noCache := runAnalyzeNormalized(t, root, "--refresh")

	if !bytes.Equal(warm, cold) {
		t.Errorf("warm run differs from cold run:\n%s", firstDiffLine(string(cold), string(warm)))
	}
	if !bytes.Equal(noCache, cold) {
		t.Errorf("--refresh run differs from cold run:\n%s", firstDiffLine(string(cold), string(noCache)))
	}
}

// materializeFixtureRepo copies the fixture into an isolated temp dir
// (renaming .go.txt → .go and gowork.txt → go.work per repo convention) and
// git-inits the copy — scope.Resolve requires a git repo root. Returns the
// absolute fixture dir (where baseline.json lives) and the temp repo root.
func materializeFixtureRepo(t *testing.T, fixtureRelPath string) (absFixture, root string) {
	t.Helper()

	// Resolve the fixture path relative to this source file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	absFixture, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), fixtureRelPath))
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}

	// Copy fixture files into an isolated temp dir. Fixtures must not carry a
	// .git directory, so we copy and then git-init the copy.
	root = t.TempDir()
	if err := copyFixtureIntoDir(absFixture, root); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	// Rename gowork.txt → go.work (go.work* is gitignored at the repo level,
	// so workspace fixtures are committed under the .txt name).
	goworkTxt := filepath.Join(root, "gowork.txt")
	if _, statErr := os.Stat(goworkTxt); statErr == nil {
		if err := os.Rename(goworkTxt, filepath.Join(root, "go.work")); err != nil {
			t.Fatalf("rename gowork.txt → go.work: %v", err)
		}
	}

	gitInitFixtureRepo(t, root)
	return absFixture, root
}

// runAnalyzeNormalized runs archfit analyze --format json in-process
// (plus extraArgs) against the materialized repo at root and returns the
// normalized JSON output. Exit 0 = pass, 1 = gate violation; both are valid
// analysis results. 2/3 indicate a config or runtime error.
func runAnalyzeNormalized(t *testing.T, root string, extraArgs ...string) []byte {
	t.Helper()

	cfgPath := filepath.Join(root, ".archfit.yaml")
	args := append([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, extraArgs...)
	var buf bytes.Buffer
	code := Run(args, &buf)
	if code != 0 && code != 1 {
		t.Fatalf("archfit exited %d (want 0 or 1):\n%s", code, buf.String())
	}

	// Normalise volatile fields: replace the temp root path with <ROOT> so the
	// output is stable across runs, then re-marshal through interface{} for
	// canonical (alphabetical) key ordering.
	got, err := normalizeArchfitJSON(buf.Bytes(), root)
	if err != nil {
		t.Fatalf("normalise output: %v", err)
	}
	return got
}

// normalizeArchfitJSON replaces all occurrences of root (the temp scan root)
// with <ROOT>, then unmarshals and re-marshals the JSON for canonical key
// ordering. Both the baseline write and the comparison run go through this
// function, so ordering is consistent.
func normalizeArchfitJSON(data []byte, root string) ([]byte, error) {
	// filepath.ToSlash is a no-op on Unix; handles Windows temp paths.
	rootFwd := filepath.ToSlash(root)
	normalized := strings.ReplaceAll(string(data), root, "<ROOT>")
	if rootFwd != root {
		normalized = strings.ReplaceAll(normalized, rootFwd, "<ROOT>")
	}

	var m interface{}
	if err := json.Unmarshal([]byte(normalized), &m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return append(out, '\n'), nil
}

// copyFixtureIntoDir recursively copies the fixture directory into dst.
// Files with a .go.txt extension are renamed to .go so the copied tree is a
// valid Go module (the .go.txt extension is a repo convention that prevents
// golangci-lint from typechecking fixture packages as part of the main module).
func copyFixtureIntoDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Rename .go.txt → .go in the destination.
		destRel := rel
		if strings.HasSuffix(rel, ".go.txt") {
			destRel = strings.TrimSuffix(rel, ".txt")
		}
		dstPath := filepath.Join(dst, destRel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o750)
		}
		data, err := os.ReadFile(path) //nolint:gosec // path is from filepath.WalkDir within fixture dir
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o600) //nolint:gosec // dstPath is within t.TempDir()
	})
}

// firstDiffLine returns a short description of the first differing line
// between want and got (both as multi-line strings).
func firstDiffLine(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) < n {
		n = len(gl)
	}
	for i := 0; i < n; i++ {
		if wl[i] != gl[i] {
			return fmt.Sprintf("line %d:\n  want: %s\n   got: %s", i+1, wl[i], gl[i])
		}
	}
	if len(wl) != len(gl) {
		return fmt.Sprintf("line count: want %d, got %d", len(wl), len(gl))
	}
	return "(no diff found — byte comparison may be trailing-whitespace)"
}
