package scope_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/scope"
)

const fakeRoot = "/fake/root"

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
	if len(s.Changed) != 0 {
		t.Errorf("changed: expected empty, got %v", s.Changed)
	}
}

func TestResolve_Delta(t *testing.T) {
	// Changed files arrive unsorted: Resolve must sort them — the
	// determinism contract does not depend on resolver discipline.
	r := fakeResolver{root: fakeRoot, head: "abc123", changed: []string{"z.go", "a.go"}}

	s, err := scope.Resolve(context.Background(), config.ScopeConfig{Base: "main"}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Mode != scope.ModeDelta {
		t.Errorf("mode: got %q, want %q", s.Mode, scope.ModeDelta)
	}
	if s.Root != fakeRoot {
		t.Errorf("root: got %q, want %q", s.Root, fakeRoot)
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

func TestResolve_ResolverError(t *testing.T) {
	r := fakeResolver{err: errors.New("not a git repo")}

	_, err := scope.Resolve(context.Background(), config.ScopeConfig{Full: true}, r)
	if err == nil {
		t.Fatal("expected error, got nil")
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
