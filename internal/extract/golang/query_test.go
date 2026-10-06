package golang

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	queryTargetDir = "internal/b"
	queryImporter  = "internal/a/a.go"
	queryNever     = "internal/a/never.go"
	queryEnt       = "internal/a/ent.go"
	queryGoMod     = "module example.com/q\n\ngo 1.21\n"
	queryNoCgo     = "internal/a/nocgo.go"
	queryUsesC     = "internal/a/usesc.go"
	queryUsesCEnt  = "internal/a/usesc_ent.go"
)

func TestQueryEdgeSpellsTheExtractedEdge(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, body := range map[string]string{
		evidenceGoModFile:  queryGoMod,
		"tools/go.mod":     "module example.com/q/tools\n\ngo 1.21\n",
		queryImporter:      "package a\n",
		"tools/gen/gen.go": "package gen\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, target, wantTo string
		wantKind             graph.EdgeKind
	}{
		{name: "repo-relative dir", target: queryTargetDir, wantTo: queryTargetDir, wantKind: graph.EdgeKindImports},
		{name: "import path", target: "example.com/q/internal/b", wantTo: queryTargetDir, wantKind: graph.EdgeKindImports},
		{name: "nested member import path", target: "example.com/q/tools/gen", wantTo: "tools/gen", wantKind: graph.EdgeKindImports},
		{name: "root package", target: "example.com/q", wantTo: "", wantKind: graph.EdgeKindImports},
		{name: "go internal segment", target: "pkg/b/internal/x", wantTo: "pkg/b/internal/x", wantKind: graph.EdgeKindUsesInternal},
		{name: "external package", target: "github.com/x/y", wantTo: "github.com/x/y", wantKind: graph.EdgeKindImports},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, err := QueryEdge(root, evidenceports.ExtractConfig{}, queryImporter, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			e := facts.Edges[0]
			if e.From != "file:internal/a/a.go" || e.To != "package:"+tc.wantTo || e.Kind != tc.wantKind || e.Language != graph.LangGo {
				t.Errorf("edge = %+v", e)
			}
		})
	}
	if _, err := QueryEdge(root, evidenceports.ExtractConfig{}, "internal/a/a.ts", queryTargetDir); err == nil {
		t.Error("a non-Go importer must be an error")
	}
	facts, err := QueryEdge(root, evidenceports.ExtractConfig{}, queryImporter, queryTargetDir)
	if err != nil || len(facts.GoModules) == 0 || facts.GoModules[0].Path != "example.com/q" {
		t.Errorf("facts carry no loaded members (%v): %+v", err, facts.GoModules)
	}
}

func TestQueryEdgeDropsWhatTheExtractorDrops(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, body := range map[string]string{
		evidenceGoModFile: queryGoMod,
		queryImporter:     "package a\n",
		queryNever:        "//go:build never_set_tag\n\npackage a\n",
		"nested/go.mod":   "module example.com/nested\n\ngo 1.21\n",
		"nested/n/n.go":   "package n\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := evidenceports.ExtractConfig{Exclusions: []string{"internal/gen", "internal/gen/**", "internal/skip/**"}}
	for _, tc := range []struct {
		name, from, target string
		tags               []string
		extracted          bool
	}{
		{name: "ordinary import", from: queryImporter, target: queryTargetDir, extracted: true},
		{name: "file not written yet", from: "internal/a/new.go", target: queryTargetDir, extracted: true},
		{name: "test file", from: "internal/a/a_test.go", target: queryTargetDir},
		{name: "build-constrained file", from: queryNever, target: queryTargetDir},
		{name: "tag the run sets", from: queryNever, target: queryTargetDir, tags: []string{"-tags", "never_set_tag"}, extracted: true},
		{name: "excluded target", from: queryImporter, target: "internal/gen"},
		{name: "excluded importer", from: "internal/skip/s.go", target: queryTargetDir},
		{name: "unloaded nested module", from: "nested/n/n.go", target: queryTargetDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			run := cfg
			run.BuildFlags = tc.tags
			_, err := QueryEdge(root, run, tc.from, tc.target)
			if notExtracted := errors.Is(err, evidenceports.ErrNotExtracted); notExtracted == tc.extracted || (tc.extracted && err != nil) {
				t.Errorf("err = %v, want extracted = %v", err, tc.extracted)
			}
		})
	}
}

// TestQueryEdgeBuildEnvironment pins that the build filter reads the
// environment the run's go toolchain inherits: -tags in GOFLAGS, and
// CGO_ENABLED when it is set. Unset, a file either cgo setting includes counts.
func TestQueryEdgeBuildEnvironment(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		evidenceGoModFile: queryGoMod,
		queryEnt:          "//go:build enterprise\n\npackage a\n",
		queryNoCgo:        "//go:build !cgo\n\npackage a\n",
		queryUsesC:        "package a\n\nimport \"C\"\n",
		queryUsesCEnt:     "//go:build enterprise\n\npackage a\n\nimport \"C\"\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, from, goflags, cgo string
		flags                    []string
		goenvFile                string
		extracted, undecided     bool
	}{
		{name: "tag from GOFLAGS", from: queryEnt, goflags: "-tags=enterprise", extracted: true},
		{name: "tag missing", from: queryEnt},
		{name: "a later -tags overrides GOFLAGS", from: queryEnt, goflags: "-tags=enterprise", flags: []string{"-tags=other"}},
		{name: "!cgo with CGO_ENABLED unset is undecided", from: queryNoCgo, undecided: true},
		{name: "tag from the go env file", from: queryEnt, goenvFile: "GOFLAGS=-tags=enterprise\n", extracted: true},
		{name: "!cgo with CGO_ENABLED=0", from: queryNoCgo, cgo: "0", extracted: true},
		{name: "!cgo with CGO_ENABLED=1", from: queryNoCgo, cgo: "1"},
		{name: "import \"C\" with CGO_ENABLED=0", from: queryUsesC, cgo: "0"},
		{name: "import \"C\" with CGO_ENABLED=1 is undecided", from: queryUsesC, cgo: "1", undecided: true},
		{name: "import \"C\" with CGO_ENABLED unset is undecided", from: queryUsesC, undecided: true},
		{name: "import \"C\" excluded by a build tag", from: queryUsesCEnt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOFLAGS", tc.goflags)
			t.Setenv("CGO_ENABLED", tc.cgo)
			if tc.cgo == "" {
				t.Setenv("GOENV", "off")
				if tc.goenvFile != "" {
					file := filepath.Join(t.TempDir(), "env")
					if err := os.WriteFile(file, []byte(tc.goenvFile), 0o600); err != nil {
						t.Fatal(err)
					}
					t.Setenv("GOENV", file)
					if err := os.Unsetenv("GOFLAGS"); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Unsetenv("CGO_ENABLED"); err != nil {
					t.Fatal(err)
				}
			}
			_, err := QueryEdge(root, evidenceports.ExtractConfig{BuildFlags: tc.flags}, tc.from, queryTargetDir)
			if tc.undecided {
				if !errors.Is(err, evidenceports.ErrNotDecidable) {
					t.Errorf("err = %v, want not decidable", err)
				}
				return
			}
			if notExtracted := errors.Is(err, evidenceports.ErrNotExtracted); notExtracted == tc.extracted || (tc.extracted && err != nil) {
				t.Errorf("err = %v, want extracted = %v", err, tc.extracted)
			}
		})
	}
}
