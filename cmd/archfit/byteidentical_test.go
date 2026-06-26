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

// xfail: Task 9 sets this to false once the workspace refactor (Tasks 2-8) is
// complete and both fixtures produce verified, byte-identical output.
const xfail = true

// Fixture dirs relative to this source file (cmd/archfit/).
const (
	fixtureSingleModule       = "../../internal/extract/golang/testdata/single-module"
	fixtureOneMemberWorkspace = "../../internal/extract/golang/testdata/one-member-workspace"
)

// TestByteIdentical_SingleModule captures a pre-refactor baseline for a
// minimal single-module Go fixture and verifies byte-identical output on
// re-run. xfail-tolerant until Task 9.
//
// Divergence risk (Task 9): old code picks modPath from the first pkg with
// pkg.Module != nil; Task 6 changes to pkg.Module.Dir per package. They
// diverge on Module==nil packages (vendored/synthetic). Single-module is
// expected to be stable across the refactor — any diff here is a regression.
func TestByteIdentical_SingleModule(t *testing.T) {
	t.Parallel()
	runByteIdenticalTest(t, fixtureSingleModule)
}

// TestByteIdentical_OneMemberWorkspace captures a pre-refactor baseline for a
// go.work fixture with one real member plus one excluded (testdata/extra)
// member. xfail-tolerant until Task 9.
//
// Divergence risk (Task 9): the current extractor loads ./... from the
// workspace root; after Tasks 5-6 only the surviving member's packages are
// loaded. The baseline is expected to differ before and after — Task 9 updates
// it and flips xfail to false. After the refactor both fixtures must produce
// byte-identical output (the Tier-0 gate).
func TestByteIdentical_OneMemberWorkspace(t *testing.T) {
	t.Parallel()
	runByteIdenticalTest(t, fixtureOneMemberWorkspace)
}

// runByteIdenticalTest materialises the fixture into a fresh temp git repo,
// runs archfit check --full --format json in-process, normalises volatile
// fields (absolute paths), and diffs the result against baseline.json beside
// the fixture.
//
// First run (no baseline.json): the file is written so the developer can
// review and commit it. Subsequent runs compare. When xfail is true a diff is
// logged, not fatal.
func runByteIdenticalTest(t *testing.T, fixtureRelPath string) {
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
	// Files with the .go.txt extension (repo convention to bypass golangci-lint
	// typechecking) are renamed to .go during the copy.
	root := t.TempDir()
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

	// scope.Resolve requires a git repo root.
	gitInitFixtureRepo(t, root)

	// Run archfit in-process. Exit 0 = pass, 1 = gate violation; both are
	// valid analysis results. 2/3 indicate a config or runtime error.
	cfgPath := filepath.Join(root, ".archfit.yaml")
	var buf bytes.Buffer
	code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)
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
		diff := firstDiffLine(string(want), string(got))
		if xfail {
			t.Logf("XFAIL (Task 9 will resolve): output differs from baseline\n%s", diff)
		} else {
			t.Fatalf("output differs from baseline:\n%s", diff)
		}
	}
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
