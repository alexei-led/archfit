package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/tools/go/packages"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

const (
	reportSourceFile = "a/a.go"
	reportRootFile   = "a.go"
	reportPackage    = "package q\n"
)

func TestReportWithoutGoReadsImports(t *testing.T) {
	setGoEnv(t, "0")
	t.Setenv("PATH", t.TempDir())
	root := writeTree(t, map[string]string{
		evidenceGoModFile:     queryGoMod,
		reportSourceFile:      "package a\n\nimport (\n _ \"fmt\"\n _ \"example.com/q/pkg/internal/b\"\n)\n",
		"pkg/internal/b/b.go": "package b\n",
		"a/a_test.go":         "package a\nimport _ \"example.com/q/missing\"\n",
		"gen/g.go":            "package gen\n",
	})
	for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
		t.Run(string(mode), func(t *testing.T) {
			ex := New(evidenceports.ExtractConfig{Mode: mode, Exclusions: []string{"gen/**"}})
			var loads atomic.Int32
			ex.load = func(*packages.Config, ...string) ([]*packages.Package, error) {
				loads.Add(1)
				return nil, errors.New("typed loader reached without Go")
			}
			facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if loads.Load() != 0 {
				t.Fatal("typed loader ran without Go")
			}
			want := []graph.Edge{
				{From: "file:a/a.go", To: "package:fmt", Kind: graph.EdgeKindImports, Language: graph.LangGo, Confidence: "high", Locations: []graph.Location{{File: reportSourceFile, Line: 4}}},
				{From: "file:a/a.go", To: "package:pkg/internal/b", Kind: graph.EdgeKindUsesInternal, Language: graph.LangGo, Confidence: "high", Locations: []graph.Location{{File: reportSourceFile, Line: 5}}},
			}
			if !reflect.DeepEqual(facts.Edges, want) {
				t.Errorf("edges = %+v, want %+v", facts.Edges, want)
			}
			if !reflect.DeepEqual(facts.GoModules, []graph.GoModule{{Path: parseModule, RelDir: "."}}) {
				t.Errorf("modules = %+v", facts.GoModules)
			}
			if len(facts.Nodes) != 4 {
				t.Errorf("nodes = %+v", facts.Nodes)
			}
			if cov.Tool != toolGoPackages || cov.Status != statusPartial || cov.FilesSeen != 2 || cov.FilesApplicable != 2 ||
				cov.UnresolvedInputsMissing != 0 || cov.UnresolvedPrecisionOnly == 0 || !strings.Contains(cov.Reason, "go/parser") {
				t.Errorf("coverage = %+v", cov)
			}
		})
	}
}

func TestReportParserDisclosesUnknownInputs(t *testing.T) {
	setGoEnv(t, "")
	t.Setenv("PATH", t.TempDir())
	root := writeTree(t, map[string]string{
		evidenceGoModFile: queryGoMod,
		reportRootFile:    "package q\nimport _ \"example.com/q/gone\"\n",
		"broken.go":       "package\n",
		"cgo.go":          "package q\nimport \"C\"\n",
	})
	facts, cov, err := New(evidenceports.ExtractConfig{}).Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if cov.Status != statusPartial || cov.FilesSeen != 1 || cov.FilesApplicable != 3 || cov.UnresolvedInputsMissing != 3 {
		t.Errorf("coverage = %+v", cov)
	}
	if facts.Unresolved != cov.Unresolved || len(cov.Details) != 3 {
		t.Errorf("unknowns: facts = %+v, coverage = %+v", facts, cov)
	}
}

func TestReportWithGoKeepsTypedLoader(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 91\n"), 0o700); err != nil { //nolint:gosec // the test's executable trap
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	root := writeTree(t, map[string]string{evidenceGoModFile: queryGoMod, reportRootFile: reportPackage})
	want := errors.New("typed loader selected")
	ex := New(evidenceports.ExtractConfig{Mode: evidenceports.ModeOn})
	ex.load = func(*packages.Config, ...string) ([]*packages.Package, error) { return nil, want }
	_, _, err := ex.Extract(context.Background(), scope.Scope{Root: root})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want typed loader error", err)
	}
}

func TestReportWithoutGoAbsentAndOff(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, tc := range []struct {
		name  string
		files map[string]string
		mode  evidenceports.ToolMode
	}{
		{name: "no project"},
		{name: "empty module", files: map[string]string{evidenceGoModFile: queryGoMod}},
		{name: "off", files: map[string]string{evidenceGoModFile: queryGoMod, reportRootFile: reportPackage}, mode: evidenceports.ModeOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, tc.files)
			facts, cov, err := New(evidenceports.ExtractConfig{Mode: tc.mode}).Extract(context.Background(), scope.Scope{Root: root})
			if err != nil || cov.Status != statusAbsent || len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
				t.Fatalf("facts = %+v, coverage = %+v, error = %v", facts, cov, err)
			}
		})
	}
}
