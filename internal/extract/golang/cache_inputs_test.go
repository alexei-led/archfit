package golang

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"

	"golang.org/x/tools/go/packages"
)

func TestFactCache_SourceDirectoriesMatchFreshLoad(t *testing.T) {
	for _, dir := range []string{"target", "venv", "node_modules"} {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			write := func(path, body string) {
				t.Helper()
				path = filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", "module example.com/cache\n\ngo 1.24\n")
			write("internal/"+dir+"/input.go", "package input\n")
			write("internal/"+dir+"/CACHEDIR.TAG", "Signature: 8a477f597d28d172789f06886806bc55\n")
			write("internal/"+dir+"/pyvenv.cfg", "home = /usr/bin\n")
			write("internal/secret/secret.go", "package secret\nconst Value = 1\n")
			loads := 0
			ex := New(evidenceports.ExtractConfig{})
			ex.Cache = factcache.NewStore(t.TempDir())
			ex.load = func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
				loads++
				return packages.Load(cfg, patterns...)
			}
			extract := func() graph.Facts {
				t.Helper()
				facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
				if err != nil || cov.Status != "ok" {
					t.Fatalf("Extract: status=%s error=%v", cov.Status, err)
				}
				slices.SortFunc(facts.Nodes, func(a, b graph.Node) int { return strings.Compare(a.ID(), b.ID()) })
				return facts
			}
			before := extract()
			extract()
			if loads != 1 {
				t.Fatalf("unchanged tree must hit cache: loads=%d", loads)
			}
			write("internal/"+dir+"/input.go", "package input\nimport \"example.com/cache/internal/secret\"\nvar Value = secret.Value\n")
			warm := extract()
			ex.Cache = nil
			fresh := extract()
			if reflect.DeepEqual(before.Edges, fresh.Edges) {
				t.Fatal("fixture did not change the dependency graph")
			}
			if !reflect.DeepEqual(warm, fresh) {
				t.Fatalf("warm facts differ from fresh facts: warm=%+v fresh=%+v", warm, fresh)
			}
			if loads != 3 {
				t.Fatalf("edit must reload before fresh run: loads=%d", loads)
			}
		})
	}
}

func TestFactCache_ImportedTestdataMatchesFreshLoad(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/cache\n\ngo 1.24\n")
	write("input.go", "package input\nimport \"example.com/cache/testdata\"\nvar Value = testdata.Value\n")
	write("testdata/input.go", "package testdata\nconst Value = 1\n")
	ex := New(evidenceports.ExtractConfig{})
	ex.Cache = factcache.NewStore(t.TempDir())
	extract := func() graph.Facts {
		t.Helper()
		facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
		if err != nil || cov.Status != "ok" {
			t.Fatalf("Extract: status=%s error=%v", cov.Status, err)
		}
		return facts
	}
	before := extract()
	write("testdata/input.go", "package testdata\nfunc Value() {}\n")
	warm := extract()
	ex.Cache = nil
	fresh := extract()
	if reflect.DeepEqual(before.Edges, fresh.Edges) {
		t.Fatal("imported testdata change did not change dependency strength")
	}
	if !reflect.DeepEqual(warm, fresh) {
		t.Fatalf("warm facts differ from fresh facts: warm=%+v fresh=%+v", warm, fresh)
	}
}

func TestFactCache_GoBuildOutputDoesNotInvalidate(t *testing.T) {
	root, dirA, dirB := writeWorkspaceFixture(t)
	loader := &fakeLoader{calls: map[string]int{}}
	ex := New(evidenceports.ExtractConfig{})
	ex.Cache = factcache.NewStore(t.TempDir())
	ex.load = loader.load
	for _, content := range []string{"initial object", "rebuilt object"} {
		if err := os.WriteFile(filepath.Join(dirA, "output.o"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ex.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
			t.Fatal(err)
		}
	}
	if loader.calls[dirA] != 1 || loader.calls[dirB] != 1 {
		t.Fatalf("Go build output must not invalidate source cache: loads=%v", loader.calls)
	}
}

func TestFactCache_ArchitectureFeaturesInvalidate(t *testing.T) {
	for _, feature := range []string{"GO386", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM"} {
		t.Run(feature, func(t *testing.T) {
			root, dirA, dirB := writeWorkspaceFixture(t)
			loader := &fakeLoader{calls: map[string]int{}}
			ex := New(evidenceports.ExtractConfig{})
			ex.Cache = factcache.NewStore(t.TempDir())
			ex.load = loader.load
			for _, value := range []string{"before", "after"} {
				t.Setenv(feature, value)
				if _, _, err := ex.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
					t.Fatal(err)
				}
			}
			if loader.calls[dirA] != 2 || loader.calls[dirB] != 2 {
				t.Fatalf("architecture feature edit must invalidate member cache: loads=%v", loader.calls)
			}
		})
	}
}
