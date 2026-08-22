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

	res, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
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

	res, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
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

	res, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
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
	res, err := goextract.DiscoverMembers(root, exclusions)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
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
	res, err := goextract.DiscoverMembers(inner, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
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
	res, err := goextract.DiscoverMembers(root, exclusions)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
	if len(got) != 1 || got[0] != root {
		t.Errorf("got %v, want [%s]", got, root)
	}
	// Discovery ignored the workspace; the toolchain must be told to as well or
	// `go list ./...` fails with "directory prefix . does not contain modules
	// listed in go.work" and the extractor reports absent over a readable tree.
	if !res.GoWorkOff {
		t.Errorf("GoWorkOff = false after falling back past an in-scope-empty go.work, want true")
	}
}

// TestDiscoverMembers_GoWorkOutOfScope is the shape that made `analyze --base`
// inert on every Go repo that gitignores go.work (the `go help work` default):
// the base ref is checked out as TRACKED FILES ONLY into a directory inside the
// repo, so the repo's go.work is absent from the checkout but still governs it
// from above — and names nothing inside it.
func TestDiscoverMembers_GoWorkOutOfScope(t *testing.T) {
	outer := t.TempDir()
	// The workspace names only the outer module, never the checkout below it.
	writeFile(t, filepath.Join(outer, "go.work"), "go 1.21\nuse .\n")
	writeFile(t, filepath.Join(outer, "go.mod"), "module example.com/outer\ngo 1.21\n")

	checkout := filepath.Join(outer, ".archfit-cache", "worktrees", "abc", "wt")
	writeFile(t, filepath.Join(checkout, "go.mod"), "module example.com/outer\ngo 1.21\n")

	res, err := goextract.DiscoverMembers(checkout, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if len(res.Dirs) != 1 || res.Dirs[0] != checkout {
		t.Fatalf("Dirs = %v, want [%s]", res.Dirs, checkout)
	}
	if !res.GoWorkOff {
		t.Errorf("GoWorkOff = false for a checkout an out-of-scope go.work governs, want true")
	}
}

// TestDiscoverMembers_GoWorkOutOfScope_WalkFallback covers the same out-of-scope
// go.work reaching the walk branch: no go.mod at the scan root, modules below.
func TestDiscoverMembers_GoWorkOutOfScope_WalkFallback(t *testing.T) {
	outer := t.TempDir()
	writeFile(t, filepath.Join(outer, "go.work"), "go 1.21\nuse ./elsewhere\n")
	writeFile(t, filepath.Join(outer, "elsewhere", "go.mod"), "module example.com/elsewhere\ngo 1.21\n")

	sub := filepath.Join(outer, "sub")
	writeFile(t, filepath.Join(sub, "svc", "go.mod"), "module example.com/svc\ngo 1.21\n")

	res, err := goextract.DiscoverMembers(sub, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if len(res.Dirs) != 1 || res.Dirs[0] != filepath.Join(sub, "svc") {
		t.Fatalf("Dirs = %v, want [%s]", res.Dirs, filepath.Join(sub, "svc"))
	}
	if !res.GoWorkOff {
		t.Errorf("GoWorkOff = false on the walk fallback past an out-of-scope go.work, want true")
	}
}

// TestDiscoverMembers_NoGoWork_KeepsWorkspaceOn pins the negative: with no
// go.work anywhere above the scan root there is nothing for the toolchain to
// ignore, so GOWORK must be left alone.
func TestDiscoverMembers_NoGoWork_KeepsWorkspaceOn(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/plain\ngo 1.21\n")

	res, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	if res.GoWorkOff {
		t.Errorf("GoWorkOff = true with no go.work in play, want false")
	}
}

func TestDiscoverMembers_NoMembers_ReturnsNil(t *testing.T) {
	// No go.work, no go.mod — result is nil.
	root := t.TempDir()
	res, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
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

	res, err := goextract.DiscoverMembers(root, nil)
	if err != nil {
		t.Fatalf("DiscoverMembers: %v", err)
	}
	got := res.Dirs
	if !sort.StringsAreSorted(got) {
		t.Errorf("members not sorted: %v", got)
	}
}

// TestFilterMembers verifies include/exclude glob filtering for tools.go.modules.
func TestFilterMembers(t *testing.T) {
	root := t.TempDir()
	abs := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	all := []string{
		abs("svc/a"),
		abs("svc/b"),
		abs("lib/c"),
		abs("tools/d"),
	}

	const globSvc = "svc/**"

	tests := []struct {
		name    string
		members []string
		include []string
		exclude []string
		want    []string
	}{
		{
			name:    "no_filter_returns_all",
			members: all,
			want:    all,
		},
		{
			name:    "include_selects_subset",
			members: all,
			include: []string{globSvc},
			want:    []string{abs("svc/a"), abs("svc/b")},
		},
		{
			name:    "exclude_removes_members",
			members: all,
			exclude: []string{"tools/**"},
			want:    []string{abs("svc/a"), abs("svc/b"), abs("lib/c")},
		},
		{
			name:    "include_matches_nothing_returns_nil",
			members: all,
			include: []string{"nonexistent/**"},
			want:    nil,
		},
		{
			name:    "include_and_exclude_combined",
			members: all,
			include: []string{globSvc},
			exclude: []string{"svc/b"},
			want:    []string{abs("svc/a")},
		},
		{
			name:    "exact_member_name_include",
			members: all,
			include: []string{"lib/c"},
			want:    []string{abs("lib/c")},
		},
		{
			name:    "empty_members_returns_nil",
			members: nil,
			include: []string{globSvc},
			want:    nil,
		},
		{
			name:    "exclude_all_returns_nil",
			members: all,
			exclude: []string{"**"},
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := goextract.FilterMembers(tc.members, root, tc.include, tc.exclude)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), tc.want, len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("members[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestDiscoverMembers_ArchfitSelfCollapses verifies the critical invariant:
// archfit's own go.work has three use directives (., ./testdata/fixture-go,
// ./testdata/golang) but the two testdata members are excluded by the default
// **/testdata/** glob, so exactly one member — the repo root itself — survives.
// This ensures the single-module analysis path stays byte-identical.
func TestDiscoverMembers_ArchfitSelfCollapses(t *testing.T) {
	repoRoot := archfitRepoRoot(t)

	res, err := goextract.DiscoverMembers(repoRoot, scope.DefaultExclusions)
	if err != nil {
		t.Fatalf("DiscoverMembers on archfit root: %v", err)
	}
	got := res.Dirs
	// The repo root IS a go.work member, so the workspace governs the load and
	// must stay ON. GoWorkOff here would drop the workspace on the head side of
	// every run.
	if res.GoWorkOff {
		t.Errorf("GoWorkOff = true on the archfit repo root, want false — the root is a go.work member")
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 member, got %d: %v", len(got), got)
	}
	if got[0] != repoRoot {
		t.Errorf("member = %q, want %q", got[0], repoRoot)
	}
}
