package golang_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

const goModFile = "go.mod"

func writeCgoFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		goModFile:     "module example.com/cg\n\ngo 1.21\n",
		"b/b.go":      "package b\n\nfunc B() int { return 1 }\n",
		"a/plain.go":  "package a\n\nimport \"example.com/cg/b\"\n\nfunc Plain() int { return b.B() }\n",
		"a/native.go": "package a\n\n/*\nstatic int one(void) { return 1; }\n*/\nimport \"C\"\n\nimport (\n\t\"unsafe\"\n\n\t\"example.com/cg/b\"\n)\n\nvar _ unsafe.Pointer\n\nfunc Native() int { return int(C.one()) + b.B() }\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func requireCgo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("no C compiler")
	}
	t.Setenv("CGO_ENABLED", "1")
}

func edgeFrom(facts graph.Facts, from string) *graph.Edge {
	id := graph.Node{Kind: graph.NodeKindFile, Path: from}.ID()
	for i := range facts.Edges {
		if facts.Edges[i].From == id && facts.Edges[i].To == (graph.Node{Kind: graph.NodeKindPackage, Path: "b"}).ID() {
			return &facts.Edges[i]
		}
	}
	return nil
}

// edgeTargets lists the package nodes a file imports, sorted.
func edgeTargets(facts graph.Facts, from string) []string {
	id := graph.Node{Kind: graph.NodeKindFile, Path: from}.ID()
	var out []string
	for _, e := range facts.Edges {
		if e.From == id {
			out = append(out, e.To)
		}
	}
	sort.Strings(out)
	return out
}

// wantNativeTargets is what the author wrote in native.go: the preprocessed copy
// also carries cmd/cgo's rewrite of import "C", which must not leak into facts.
var wantNativeTargets = []string{"package:C", "package:b", "package:unsafe"}

func TestExtract_CgoFileKeepsItsImports(t *testing.T) {
	requireCgo(t)
	dir := writeCgoFixture(t)

	facts, cov, err := goextract.New(evidenceports.ExtractConfig{}).Extract(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status != "ok" {
		t.Fatalf("status = %q (%s), want ok", cov.Status, cov.Reason)
	}
	for _, from := range []string{"a/plain.go", "a/native.go"} {
		e := edgeFrom(facts, from)
		if e == nil {
			t.Fatalf("no edge %s -> b; edges: %+v", from, facts.Edges)
		}
		if len(e.Locations) != 1 || e.Locations[0].File != from || e.Locations[0].Line == 0 {
			t.Errorf("%s: locations = %+v, want the original file and a line", from, e.Locations)
		}
	}
	if h := edgeFrom(facts, "a/native.go").StrengthHint; h != edgeFrom(facts, "a/plain.go").StrengthHint || h == "" {
		t.Errorf("cgo file strength hint = %q, want the same non-empty hint as the plain file", h)
	}
	if got := edgeTargets(facts, "a/native.go"); !slices.Equal(got, wantNativeTargets) {
		t.Errorf("native.go imports = %v, want %v", got, wantNativeTargets)
	}
	if cov.FilesSeen != 3 {
		t.Errorf("FilesSeen = %d, want 3 (b.go, plain.go, native.go)", cov.FilesSeen)
	}
}

func TestExtract_CgoOffDisclosesTheIgnoredFile(t *testing.T) {
	dir := writeCgoFixture(t)
	t.Setenv("CGO_ENABLED", "0")

	facts, cov, err := goextract.New(evidenceports.ExtractConfig{}).Extract(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if edgeFrom(facts, "a/native.go") != nil {
		t.Error("cgo file read with cgo off; the load must ignore it")
	}
	if !strings.Contains(cov.Reason, goextract.BuildConstraintExclusion) {
		t.Errorf("reason = %q, want the build-constraint disclosure naming the ignored file", cov.Reason)
	}
}

func TestExtract_CgoPreprocessFailureStaysPartialWithImports(t *testing.T) {
	requireCgo(t)
	dir := writeCgoFixture(t)
	broken := "package a\n\n/*\n#include <archfit_missing_header.h>\n*/\nimport \"C\"\n\nimport (\n\t\"unsafe\"\n\n\t\"example.com/cg/b\"\n)\n\nvar _ unsafe.Pointer\n\nfunc Native() int { return int(C.one()) + b.B() }\n"
	if err := os.WriteFile(filepath.Join(dir, "a", "native.go"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}

	facts, cov, err := goextract.New(evidenceports.ExtractConfig{}).Extract(context.Background(), scope.Scope{Root: dir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status == "ok" {
		t.Errorf("status = ok after a failed cgo preprocess; want a disclosed gap (reason %q)", cov.Reason)
	}
	if got := edgeTargets(facts, "a/native.go"); !slices.Equal(got, wantNativeTargets) {
		t.Errorf("fallback native.go imports = %v, want the same as a successful preprocess: %v", got, wantNativeTargets)
	}
}
