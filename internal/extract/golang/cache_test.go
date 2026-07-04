package golang

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
)

// writeWorkspaceFixture materialises a two-member go.work workspace where
// neither member requires the other, so per-member cache keys are fully
// independent. Returns root, memberA dir, memberB dir.
func writeWorkspaceFixture(t *testing.T) (root, dirA, dirB string) {
	t.Helper()
	root = t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.work", "go 1.24\n\nuse (\n\t./a\n\t./b\n)\n")
	write("a/go.mod", "module example.com/a\n\ngo 1.24\n")
	write("a/a.go", "package a\n")
	write("b/go.mod", "module example.com/b\n\ngo 1.24\n")
	write("b/b.go", "package b\n")
	return root, filepath.Join(root, "a"), filepath.Join(root, "b")
}

// fakeLoader counts packages.Load calls per member dir and returns a minimal
// clean package for it — no Syntax, no TypesInfo, just enough for
// deriveMemberFacts to produce a cacheable fact set.
type fakeLoader struct {
	mu    sync.Mutex
	calls map[string]int
}

func (f *fakeLoader) load(cfg *packages.Config, _ ...string) ([]*packages.Package, error) {
	f.mu.Lock()
	f.calls[cfg.Dir]++
	f.mu.Unlock()
	name := filepath.Base(cfg.Dir)
	return []*packages.Package{{
		PkgPath: "example.com/" + name,
		Module:  &packages.Module{Path: "example.com/" + name, Dir: cfg.Dir},
	}}, nil
}

// TestFactCache_PerMemberInvalidation pins the Wave 6 per-member granularity:
// a warm run loads zero members, and editing one member's file re-loads ONLY
// that member (the other still hits its cache entry).
func TestFactCache_PerMemberInvalidation(t *testing.T) {
	t.Parallel()
	root, dirA, dirB := writeWorkspaceFixture(t)
	loader := &fakeLoader{calls: map[string]int{}}
	ex := New(config.ExtractConfig{})
	ex.Cache = factcache.NewStore(t.TempDir())
	ex.load = loader.load
	s := scope.Scope{Root: root}
	ctx := context.Background()

	extract := func() {
		t.Helper()
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}

	extract() // cold: both members load
	if loader.calls[dirA] != 1 || loader.calls[dirB] != 1 {
		t.Fatalf("cold run: want 1 load per member, got %v", loader.calls)
	}

	extract() // warm: full hit, no loads
	if loader.calls[dirA] != 1 || loader.calls[dirB] != 1 {
		t.Fatalf("warm run: want no additional loads, got %v", loader.calls)
	}

	// Edit one file in member a → only member a re-loads.
	if err := os.WriteFile(filepath.Join(dirA, "a.go"), []byte("package a\n\nvar X = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if loader.calls[dirA] != 2 {
		t.Errorf("member a: want re-load after edit (2 loads), got %d", loader.calls[dirA])
	}
	if loader.calls[dirB] != 1 {
		t.Errorf("member b: want cache hit (still 1 load), got %d", loader.calls[dirB])
	}
}

// TestFactCache_GoModChangeInvalidates covers the manifest component of the
// key: a go.mod edit invalidates that member.
func TestFactCache_GoModChangeInvalidates(t *testing.T) {
	t.Parallel()
	root, dirA, dirB := writeWorkspaceFixture(t)
	loader := &fakeLoader{calls: map[string]int{}}
	ex := New(config.ExtractConfig{})
	ex.Cache = factcache.NewStore(t.TempDir())
	ex.load = loader.load
	ctx := context.Background()
	s := scope.Scope{Root: root}

	for range 2 {
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dirB, "go.mod"), []byte("module example.com/b\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if loader.calls[dirB] != 2 {
		t.Errorf("member b: want re-load after go.mod edit, got %d loads", loader.calls[dirB])
	}
	if loader.calls[dirA] != 1 {
		t.Errorf("member a: want cache hit, got %d loads", loader.calls[dirA])
	}
}

// TestFactCache_DependentMemberInvalidates pins the dependency-closure rule:
// when member b requires member a, editing a's source invalidates BOTH (b
// compiles against a's source in workspace mode).
func TestFactCache_DependentMemberInvalidates(t *testing.T) {
	t.Parallel()
	root, dirA, dirB := writeWorkspaceFixture(t)
	if err := os.WriteFile(filepath.Join(dirB, "go.mod"),
		[]byte("module example.com/b\n\ngo 1.24\n\nrequire example.com/a v0.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := &fakeLoader{calls: map[string]int{}}
	ex := New(config.ExtractConfig{})
	ex.Cache = factcache.NewStore(t.TempDir())
	ex.load = loader.load
	ctx := context.Background()
	s := scope.Scope{Root: root}

	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "a.go"), []byte("package a\n\nvar Y = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(ctx, s); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if loader.calls[dirA] != 2 {
		t.Errorf("member a: want re-load after edit, got %d loads", loader.calls[dirA])
	}
	if loader.calls[dirB] != 2 {
		t.Errorf("member b (requires a): want re-load after a's edit, got %d loads", loader.calls[dirB])
	}
}
