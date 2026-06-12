package scope_test

import (
	"context"
	"errors"
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
