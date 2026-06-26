package golang_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/scope"
)

// archfitRepoRoot returns the absolute path of the archfit repository root by
// navigating up from this source file.
func archfitRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file is .../internal/extract/golang/members_test.go
	// go up three levels: golang → extract → internal → repo root
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

// writeFile creates a file at path with the given content, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// sortedCopy returns a sorted copy of ss.
func sortedCopy(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

func TestDiscoverMembers_GoWorkWithMembers(t *testing.T) {
	root := t.TempDir()
	modA := filepath.Join(root, "svc", "a")
	modB := filepath.Join(root, "svc", "b")

	writeFile(t, filepath.Join(root, "go.work"), "go 1.21\nuse ./svc/a\nuse ./svc/b\n")
	writeFile(t, filepath.Join(modA, "go.mod"), "module example.com/a\ngo 1.21\n")
	writeFile(t, filepath.Join(modB, "go.mod"), "module example.com/b\ngo 1.21\n")

	got, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	want := sortedCopy([]string{modA, modB})
	if len(got) != len(want) {
		t.Fatalf("got %d members %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("members[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverMembers_SingleGoMod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/single\ngo 1.21\n")

	got, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if len(got) != 1 || got[0] != root {
		t.Errorf("got %v, want [%s]", got, root)
	}
}

func TestDiscoverMembers_MultiGoModWalk(t *testing.T) {
	// No go.work, no go.mod at root — walk finds modules in subdirs.
	root := t.TempDir()
	modA := filepath.Join(root, "alpha")
	modB := filepath.Join(root, "beta")

	writeFile(t, filepath.Join(modA, "go.mod"), "module example.com/alpha\ngo 1.21\n")
	writeFile(t, filepath.Join(modB, "go.mod"), "module example.com/beta\ngo 1.21\n")

	got, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	want := sortedCopy([]string{modA, modB})
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("members[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverMembers_ExclusionFiltering(t *testing.T) {
	// go.work with two members: one excluded, one not.
	root := t.TempDir()
	modKeep := filepath.Join(root, "keep")
	modSkip := filepath.Join(root, "testdata", "skip")

	writeFile(t, filepath.Join(root, "go.work"),
		"go 1.21\nuse ./keep\nuse ./testdata/skip\n")
	writeFile(t, filepath.Join(modKeep, "go.mod"), "module example.com/keep\ngo 1.21\n")
	writeFile(t, filepath.Join(modSkip, "go.mod"), "module example.com/skip\ngo 1.21\n")

	exclusions := []string{"**/testdata/**"}
	got, err := goextract.DiscoverMembers(root, exclusions)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if len(got) != 1 || got[0] != modKeep {
		t.Errorf("got %v, want [%s]", got, modKeep)
	}
}

func TestDiscoverMembers_SubtreeFiltering(t *testing.T) {
	// go.work exists above scanRoot; only members within scanRoot are kept.
	outer := t.TempDir()
	inner := filepath.Join(outer, "sub")
	modIn := filepath.Join(inner, "app")
	modOut := filepath.Join(outer, "other")

	writeFile(t, filepath.Join(outer, "go.work"),
		"go 1.21\nuse ./sub/app\nuse ./other\n")
	writeFile(t, filepath.Join(modIn, "go.mod"), "module example.com/app\ngo 1.21\n")
	writeFile(t, filepath.Join(modOut, "go.mod"), "module example.com/other\ngo 1.21\n")

	// scanRoot is the inner subdir — only ./sub/app should survive.
	got, err := goextract.DiscoverMembers(inner, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if len(got) != 1 || got[0] != modIn {
		t.Errorf("got %v, want [%s]", got, modIn)
	}
}

func TestDiscoverMembers_GoWorkZeroInScope_FallsBackToGoMod(t *testing.T) {
	// go.work found but all members are excluded → fall back to [scanRoot] via go.mod.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.work"),
		"go 1.21\nuse ./testdata/excluded\n")
	writeFile(t, filepath.Join(root, "testdata", "excluded", "go.mod"),
		"module example.com/excl\ngo 1.21\n")
	// scanRoot itself has go.mod.
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/root\ngo 1.21\n")

	exclusions := []string{"**/testdata/**"}
	got, err := goextract.DiscoverMembers(root, exclusions)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if len(got) != 1 || got[0] != root {
		t.Errorf("got %v, want [%s]", got, root)
	}
}

func TestDiscoverMembers_NoMembers_ReturnsNil(t *testing.T) {
	// No go.work, no go.mod — result is nil.
	root := t.TempDir()
	got, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDiscoverMembers_Sorted(t *testing.T) {
	// Members must be returned in sorted order regardless of go.work order.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.work"),
		"go 1.21\nuse ./zzz\nuse ./aaa\nuse ./mmm\n")
	for _, sub := range []string{"zzz", "aaa", "mmm"} {
		writeFile(t, filepath.Join(root, sub, "go.mod"),
			"module example.com/"+sub+"\ngo 1.21\n")
	}

	got, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("members not sorted: %v", got)
	}
}

// TestDiscoverMembers_ArchfitSelfCollapses verifies the critical invariant:
// archfit's own go.work has three use directives (., ./testdata/fixture-go,
// ./testdata/golang) but the two testdata members are excluded by the default
// **/testdata/** glob, so exactly one member — the repo root itself — survives.
// This ensures the single-module analysis path stays byte-identical.
func TestDiscoverMembers_ArchfitSelfCollapses(t *testing.T) {
	repoRoot := archfitRepoRoot(t)

	got, err := goextract.DiscoverMembers(repoRoot, scope.DefaultExclusions)
	if err != nil {
		t.Fatalf("DiscoverMembers on archfit root: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 member, got %d: %v", len(got), got)
	}
	if got[0] != repoRoot {
		t.Errorf("member = %q, want %q", got[0], repoRoot)
	}
}
