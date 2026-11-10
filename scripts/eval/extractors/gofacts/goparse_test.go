package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	fixtureImporter = "internal/a/a.go"
	fixtureTagged   = "internal/a/never.go"
)

// goFixture is a two-member go.work tree that exercises each rule the parser
// side must share with the typed loader.
var goFixture = map[string]string{
	"go.work":                 "go 1.21\n\nuse (\n\t.\n\t./tools\n)\n",
	"go.mod":                  "module example.com/q\n\ngo 1.21\n",
	"tools/go.mod":            "module example.com/q/tools\n\ngo 1.21\n",
	"root.go":                 "package q\n",
	fixtureImporter:           "package a\n\nimport (\n\t\"fmt\"\n\t\"example.com/q\"\n\t\"example.com/q/internal/b\"\n\t\"example.com/q/tools/gen\"\n)\n\nvar _ = fmt.Sprint\n",
	fixtureTagged:             "//go:build never_set_tag\n\npackage a\n\nimport \"example.com/q/internal/c\"\n",
	"internal/a/a_test.go":    "package a\n\nimport \"example.com/q/internal/c\"\n",
	"internal/a/missing.go":   "package a\n\nimport \"example.com/q/internal/gone\"\n",
	"internal/b/b.go":         "package b\n",
	"internal/c/c.go":         "package c\n",
	"internal/gen/gen.go":     "package gen\n",
	"internal/d/d.go":         "package d\n\nimport \"example.com/q/internal/gen\"\n",
	"tools/gen/gen.go":        "package gen\n\nimport \"example.com/q/internal/b\"\n",
	"testdata/t/t.go":         "package t\n\nimport \"example.com/q/internal/c\"\n",
	"vendor/v/v.go":           "package v\n\nimport \"example.com/q/internal/c\"\n",
	"_hidden/h.go":            "package h\n\nimport \"example.com/q/internal/c\"\n",
	"nested/go.mod":           "module example.com/nested\n\ngo 1.21\n",
	"nested/n/n.go":           "package n\n\nimport \"example.com/q/internal/c\"\n",
	"internal/skip/s.go":      "package skip\n\nimport \"example.com/q/internal/c\"\n",
	"internal/cgo/cgo.go":     "package cgo\n\nimport \"C\"\n",
	"internal/cgo/plain.go":   "package cgo\n",
	"internal/gen/gen_doc.go": "package gen\n",
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func fixtureConfig() config.Config {
	return config.Config{Exclude: []string{"internal/skip/**", "internal/gen", "internal/gen/**"}}
}

func TestGoparseRules(t *testing.T) {
	root := writeTree(t, goFixture)
	got, err := goparse(root, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	wantEdges := []edge{
		{From: fixtureImporter, To: ""},
		{From: fixtureImporter, To: "internal/b"},
		{From: fixtureImporter, To: "tools/gen"},
		{From: "tools/gen/gen.go", To: "internal/b"},
	}
	if !slices.Equal(got.Edges, wantEdges) {
		t.Errorf("edges = %v, want %v", got.Edges, wantEdges)
	}
	wantUnknown := []unresolved{{From: "internal/a/missing.go", Specifier: "internal/gone"}}
	if !slices.Equal(got.Unknown, wantUnknown) {
		t.Errorf("unknown = %v, want %v", got.Unknown, wantUnknown)
	}
	for _, src := range []string{fixtureTagged, "internal/a/a_test.go", "testdata/t/t.go", "vendor/v/v.go", "_hidden/h.go", "nested/n/n.go", "internal/skip/s.go"} {
		if slices.Contains(got.Sources, src) {
			t.Errorf("%s is a source; the go command does not read it", src)
		}
	}
	if !slices.Contains(got.Sources, "internal/d/d.go") {
		t.Error("internal/d/d.go is not a source")
	}
	if got.External != 2 {
		t.Errorf("external = %d, want 2: fmt and \"C\", as the typed loader counts them", got.External)
	}
}

func TestGoparseBuildTags(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=never_set_tag")
	root := writeTree(t, goFixture)
	got, err := goparse(root, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Edges, edge{From: fixtureTagged, To: "internal/c"}) {
		t.Errorf("a tag the run sets must select the file: %v", got.Edges)
	}
}

func TestGoparseCgoSetting(t *testing.T) {
	for _, tc := range []struct {
		cgo    string
		reads  bool
		reason string
	}{
		{cgo: "0", reads: false, reason: "the go command ignores a cgo file when cgo is off"},
		{cgo: "1", reads: true, reason: "cgo on reads the file"},
	} {
		t.Run("CGO_ENABLED="+tc.cgo, func(t *testing.T) {
			t.Setenv("CGO_ENABLED", tc.cgo)
			got, err := goparse(writeTree(t, goFixture), fixtureConfig())
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(got.Sources, "internal/cgo/cgo.go") != tc.reads {
				t.Errorf("internal/cgo/cgo.go source = %v: %s", !tc.reads, tc.reason)
			}
		})
	}
}

func TestModulesKeepsTheRootPackage(t *testing.T) {
	const module = "app"
	mm := config.Config{Modules: map[string]policy.ModuleDef{module: {Paths: []string{"**"}}}}.ModuleMapView()
	got, err := modules(mm, config.LangGo, strings.NewReader(`["", "internal/a"]`))
	if err != nil {
		t.Fatal(err)
	}
	if got[""] != module || got["internal/a"] != module {
		t.Errorf("modules = %v, want both paths in app", got)
	}
	if _, err := modules(mm, config.LangGo, strings.NewReader("internal/a\n")); err == nil {
		t.Error("a line list is not a JSON array and must be refused")
	}
}

// TestGoparseMatchesTypedLoader runs today's extractor on the same fixture: the
// equivalence the corpus measured, kept as a regression check.
func TestGoparseMatchesTypedLoader(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the Go toolchain")
	}
	root := writeTree(t, goFixture)
	newSide, err := goparse(root, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := reference(context.Background(), config.LangGo, root, fixtureConfig())
	if err != nil {
		t.Fatal(err)
	}
	if ref.Coverage.Status == "absent" {
		t.Fatalf("typed loader did not run: %+v", ref.Coverage)
	}
	if !slices.Equal(ref.Edges, newSide.Edges) || !slices.Equal(ref.Sources, newSide.Sources) || !slices.Equal(ref.Unknown, newSide.Unknown) {
		t.Errorf("typed loader edges %v sources %v unknown %v\nparser edges %v sources %v unknown %v",
			ref.Edges, ref.Sources, ref.Unknown, newSide.Edges, newSide.Sources, newSide.Unknown)
	}
}
