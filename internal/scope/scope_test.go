package scope_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	fakeRoot    = "/fake/root"
	fakeGitRoot = "/repo"
	baseBranch  = "main"
)

// fakeResolver is an in-memory scope.Resolver — scope tests need no git
// repo and no process runner.
type fakeResolver struct {
	root    string
	head    string
	changed []string
	err     error
}

func (f fakeResolver) RepoRoot(_ context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.root, nil
}

func (f fakeResolver) HeadRef(_ context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.head, nil
}

func (f fakeResolver) Changed(_ context.Context, _, _ string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.changed, nil
}

func TestResolve_Full(t *testing.T) {
	r := fakeResolver{root: fakeRoot}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Full: true}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Mode != scope.ModeFull {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeFull)
	}
	if s.Root != fakeRoot {
		t.Errorf("root: got %q, want %q", s.Root, fakeRoot)
	}
	if s.GitRoot != fakeRoot {
		t.Errorf("git root: got %q, want %q", s.GitRoot, fakeRoot)
	}
	if s.SubtreePrefix != "" {
		t.Errorf("subtree prefix: got %q, want %q (equal root/gitroot)", s.SubtreePrefix, "")
	}
	if len(s.Changed) != 0 {
		t.Errorf("changed: expected empty, got %v", s.Changed)
	}
}

func TestResolve_Delta(t *testing.T) {
	// Changed files arrive unsorted: Resolve must sort them — the
	// determinism contract does not depend on resolver discipline.
	r := fakeResolver{root: fakeRoot, head: "abc123", changed: []string{"z.go", "a.go"}}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Base: baseBranch}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Mode != scope.ModeDelta {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeDelta)
	}
	if s.Root != fakeRoot {
		t.Errorf("root: got %q, want %q", s.Root, fakeRoot)
	}
	if s.GitRoot != fakeRoot {
		t.Errorf("git root: got %q, want %q", s.GitRoot, fakeRoot)
	}
	if s.SubtreePrefix != "" {
		t.Errorf("subtree prefix: got %q, want empty (root==gitroot)", s.SubtreePrefix)
	}
	if s.Head != "abc123" {
		t.Errorf("head: got %q, want %q", s.Head, "abc123")
	}
	want := []string{"a.go", "z.go"}
	if len(s.Changed) != len(want) {
		t.Fatalf("changed len: got %d, want %d", len(s.Changed), len(want))
	}
	for i, f := range want {
		if s.Changed[i] != f {
			t.Errorf("changed[%d]: got %q, want %q", i, s.Changed[i], f)
		}
	}
}

// TestResolve_ResolverError_DeltaMode verifies that a RepoRoot error is a hard
// error in delta mode (no git → no diff base).
func TestResolve_ResolverError_DeltaMode(t *testing.T) {
	r := fakeResolver{err: errors.New("not a git repo")}

	_, err := scope.Resolve(context.Background(), config.ScopeConfig{Base: baseBranch}, r)
	if err == nil {
		t.Fatal("expected error in delta mode with no git, got nil")
	}
}

// TestResolve_NonGitFullMode verifies that a RepoRoot error in full mode is
// non-fatal: GitRoot is set to "" and analysis continues.
func TestResolve_NonGitFullMode(t *testing.T) {
	r := fakeResolver{err: errors.New("not a git repo")}

	// WorkDir acts as the fallback scan root when both cfg.Root and gitRoot are empty.
	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Full: true, WorkDir: "/some/dir"}, r)
	if err != nil {
		t.Fatalf("full mode with non-git dir must not error; got: %v", err)
	}
	if s.Mode != scope.ModeFull {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeFull)
	}
	if s.GitRoot != "" {
		t.Errorf("git root: got %q, want empty (non-git)", s.GitRoot)
	}
	if s.Root != "/some/dir" {
		t.Errorf("root: got %q, want %q (fallback to WorkDir)", s.Root, "/some/dir")
	}
	if s.SubtreePrefix != "" {
		t.Errorf("subtree prefix: got %q, want empty (non-git)", s.SubtreePrefix)
	}
}

// TestResolve_ScanRootVsGitRoot verifies that cfg.Root sets the analysis
// boundary independently of the git toplevel. When --root is a subdirectory,
// Root=subdir and GitRoot=toplevel with a non-empty SubtreePrefix.
func TestResolve_ScanRootVsGitRoot(t *testing.T) {
	gitTop := fakeGitRoot
	subdir := fakeGitRoot + "/services/api"
	r := fakeResolver{root: gitTop}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{
		Full: true,
		Root: subdir,
	}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Root != subdir {
		t.Errorf("root: got %q, want %q (cfg.Root must win)", s.Root, subdir)
	}
	if s.GitRoot != gitTop {
		t.Errorf("git root: got %q, want %q", s.GitRoot, gitTop)
	}
	if s.SubtreePrefix != "services/api" {
		t.Errorf("subtree prefix: got %q, want %q", s.SubtreePrefix, "services/api")
	}
}

// TestResolve_RootAbsent_PrefixEmpty verifies that when cfg.Root is empty the
// scan root equals the git root and SubtreePrefix is "". This is the --root-absent
// path that must be byte-identical to the pre-change behavior.
func TestResolve_RootAbsent_PrefixEmpty(t *testing.T) {
	r := fakeResolver{root: fakeRoot}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Full: true}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Root != fakeRoot {
		t.Errorf("root: got %q, want %q (should equal git root when cfg.Root absent)", s.Root, fakeRoot)
	}
	if s.GitRoot != fakeRoot {
		t.Errorf("git root: got %q, want %q", s.GitRoot, fakeRoot)
	}
	if s.SubtreePrefix != "" {
		t.Errorf("subtree prefix: got %q, want empty (root == git root)", s.SubtreePrefix)
	}
}

// TestResolve_ScanRootVsGitRoot_Delta verifies the subtree-prefix is set in delta
// mode too, and the scan root derives from cfg.Root even when Changed is populated.
func TestResolve_ScanRootVsGitRoot_Delta(t *testing.T) {
	gitTop := fakeGitRoot
	subdir := fakeGitRoot + "/cmd"
	r := fakeResolver{root: gitTop, head: "deadbeef", changed: []string{"cmd/main.go"}}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{
		Root: subdir,
		Base: baseBranch,
	}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Root != subdir {
		t.Errorf("root: got %q, want %q", s.Root, subdir)
	}
	if s.GitRoot != gitTop {
		t.Errorf("git root: got %q, want %q", s.GitRoot, gitTop)
	}
	if s.SubtreePrefix != "cmd" {
		t.Errorf("subtree prefix: got %q, want %q", s.SubtreePrefix, "cmd")
	}
	if s.Mode != scope.ModeDelta {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeDelta)
	}
}

// TestSubtreePrefix_NotUnderGitRoot verifies that a cfg.Root outside the git
// tree (unusual but possible in CI) produces an empty SubtreePrefix rather than
// a ".." escape that would confuse git path filters.
func TestSubtreePrefix_NotUnderGitRoot(t *testing.T) {
	gitTop := fakeGitRoot
	outside := "/other/project"
	r := fakeResolver{root: gitTop}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{
		Full: true,
		Root: outside,
	}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SubtreePrefix != "" {
		t.Errorf("subtree prefix: got %q, want empty (root outside git tree)", s.SubtreePrefix)
	}
}

func TestMergeExclusions(t *testing.T) {
	const vendorGlob = "**/vendor/**"

	t.Run("defaults applied when no config exclusions", func(t *testing.T) {
		got := scope.MergeExclusions(nil)
		for _, want := range scope.DefaultExclusions {
			if !slices.Contains(got, want) {
				t.Errorf("default %q missing from merged set %v", want, got)
			}
		}
	})

	t.Run("config exclusions merged, not replaced", func(t *testing.T) {
		got := scope.MergeExclusions([]string{"my/custom/**"})
		if !slices.Contains(got, "my/custom/**") {
			t.Errorf("config exclusion dropped: %v", got)
		}
		if !slices.Contains(got, "**/.gitnexus/**") || !slices.Contains(got, "**/reports/**") {
			t.Errorf("defaults must still be present alongside config: %v", got)
		}
	})

	t.Run("bare-name negation re-includes a default", func(t *testing.T) {
		got := scope.MergeExclusions([]string{"!reports"})
		if slices.Contains(got, "**/reports/**") {
			t.Errorf("!reports should remove the reports default: %v", got)
		}
		if !slices.Contains(got, "**/.gitnexus/**") {
			t.Errorf("unrelated defaults must survive a negation: %v", got)
		}
	})

	t.Run("exact-glob and file negation re-include", func(t *testing.T) {
		got := scope.MergeExclusions([]string{"!**/.gitnexus/**", "!.archfit-baseline.json"})
		if slices.Contains(got, "**/.gitnexus/**") {
			t.Errorf("exact-glob negation should remove the default: %v", got)
		}
		if slices.Contains(got, "**/.archfit-baseline.json") {
			t.Errorf("file negation should remove the default: %v", got)
		}
	})

	t.Run("deterministic, sorted, de-duplicated", func(t *testing.T) {
		// A config entry duplicating a default must not appear twice; order is stable.
		got := scope.MergeExclusions([]string{vendorGlob, vendorGlob})
		count := 0
		for _, p := range got {
			if p == vendorGlob {
				count++
			}
		}
		if count != 1 {
			t.Errorf("duplicate vendor entry not de-duplicated: %v", got)
		}
		if !slices.IsSorted(got) {
			t.Errorf("merged exclusions must be sorted for double-run stability: %v", got)
		}
	})

	t.Run("testdata excluded by default", func(t *testing.T) {
		got := scope.MergeExclusions(nil)
		if !slices.Contains(got, "**/testdata/**") {
			t.Errorf("testdata glob must be in DefaultExclusions; got %v", got)
		}
	})

	t.Run("testdata re-included by negation", func(t *testing.T) {
		// A user who intentionally wants to analyse testdata can opt in with
		// "!testdata" (bare name) or "!**/testdata/**" (exact glob).
		gotBare := scope.MergeExclusions([]string{"!testdata"})
		if slices.Contains(gotBare, "**/testdata/**") {
			t.Errorf("!testdata should remove the testdata default; got %v", gotBare)
		}
		gotExact := scope.MergeExclusions([]string{"!**/testdata/**"})
		if slices.Contains(gotExact, "**/testdata/**") {
			t.Errorf("!**/testdata/** should remove the testdata default; got %v", gotExact)
		}
	})
}
