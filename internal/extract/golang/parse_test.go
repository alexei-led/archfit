package golang

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
)

const (
	parseImporter       = "internal/a/a.go"
	parseTagged         = "internal/a/never.go"
	parseCgoFile        = "internal/cgo/cgo.go"
	parseBroken         = "internal/a/broken.go"
	parseTestFile       = "internal/a/a_test.go"
	parseToolsGen       = "tools/gen/gen.go"
	parseTestdata       = "testdata/t/t.go"
	parseVendor         = "vendor/v/v.go"
	parseUnderscoreDir  = "_hidden/h.go"
	parseUnderscoreFile = "internal/a/_x.go"
	parseNested         = "nested/n/n.go"
	parseSkipped        = "internal/skip/s.go"
	parseModule         = "example.com/q"
	parseGen            = "internal/gen"
	parseGenBody        = "package gen\n"
	parseTagsFlag       = "-tags"
)

// parseFixture is a two-member go.work tree that exercises each rule the
// parser path shares with the typed loader.
var parseFixture = map[string]string{
	"go.work":                 "go 1.21\n\nuse (\n\t.\n\t./tools\n)\n",
	"go.mod":                  "module example.com/q\n\ngo 1.21\n",
	"tools/go.mod":            "module example.com/q/tools\n\ngo 1.21\n",
	"root.go":                 "package q\n",
	parseImporter:             "package a\n\nimport (\n\t\"fmt\"\n\t\"example.com/q\"\n\t\"example.com/q/internal/b\"\n\t\"example.com/q/tools/gen\"\n)\n\nvar _ = fmt.Sprint\n",
	parseTagged:               "//go:build never_set_tag\n\npackage a\n\nimport \"example.com/q/internal/c\"\n",
	parseTestFile:             "package a\n\nimport \"example.com/q/internal/c\"\n",
	"internal/a/missing.go":   "package a\n\nimport \"example.com/q/internal/gone\"\n",
	"internal/b/b.go":         "package b\n",
	"internal/c/c.go":         "package c\n",
	"internal/gen/gen.go":     parseGenBody,
	"internal/d/d.go":         "package d\n\nimport \"example.com/q/internal/gen\"\n",
	parseToolsGen:             "package gen\n\nimport \"example.com/q/internal/b\"\n",
	parseTestdata:             "package t\n\nimport \"example.com/q/internal/c\"\n",
	parseVendor:               "package v\n\nimport \"example.com/q/internal/c\"\n",
	parseUnderscoreDir:        "package h\n\nimport \"example.com/q/internal/c\"\n",
	parseUnderscoreFile:       "package a\n\nimport \"example.com/q/internal/c\"\n",
	"nested/go.mod":           "module example.com/nested\n\ngo 1.21\n",
	parseNested:               "package n\n\nimport \"example.com/q/internal/c\"\n",
	parseSkipped:              "package skip\n\nimport \"example.com/q/internal/c\"\n",
	parseCgoFile:              "package cgo\n\nimport \"C\"\n",
	"internal/cgo/plain.go":   "package cgo\n",
	"internal/gen/gen_doc.go": parseGenBody,
}

// logFixture has a first-party package at <root>/log beside an import of the
// standard library's log.
var logFixture = map[string]string{
	"go.mod":     "module example.com/q\n\ngo 1.21\n",
	"log/log.go": "package log\n",
	"a/a.go":     "package a\n\nimport \"log\"\n\nvar _ = log.Println\n",
	"b/b.go":     "package b\n\nimport _ \"example.com/q/log\"\n",
}

var parseConfig = evidenceports.ExtractConfig{Exclusions: []string{"internal/skip/**", parseGen, "internal/gen/**"}}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// setGoEnv pins the go environment the build context reads: no GOFLAGS, no go
// env file, and CGO_ENABLED as given ("" unsets it).
func setGoEnv(t *testing.T, cgo string) {
	t.Helper()
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOENV", "off")
	t.Setenv("CGO_ENABLED", cgo)
	if cgo == "" {
		if err := os.Unsetenv("CGO_ENABLED"); err != nil {
			t.Fatal(err)
		}
	}
}

func mustParse(t *testing.T, root string, cfg evidenceports.ExtractConfig) ParsedImports {
	t.Helper()
	got, err := ParseImports(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// pairs lists "file -> package" for the imports of one kind.
func pairs(p ParsedImports, kind ImportKind) []string {
	var out []string
	for _, imp := range p.Imports {
		if imp.Kind == kind {
			out = append(out, imp.File+" -> "+imp.Package)
		}
	}
	return out
}

func TestParseImportsRules(t *testing.T) {
	setGoEnv(t, "0")
	got := mustParse(t, writeTree(t, parseFixture), parseConfig)

	wantFirstParty := []string{
		parseImporter + " -> ",
		parseImporter + " -> internal/b",
		parseImporter + " -> tools/gen",
		"tools/gen/gen.go -> internal/b",
	}
	if p := pairs(got, ImportFirstParty); !slices.Equal(p, wantFirstParty) {
		t.Errorf("first-party = %q, want %q", p, wantFirstParty)
	}
	if p, want := pairs(got, ImportUnresolved), []string{"internal/a/missing.go -> internal/gone"}; !slices.Equal(p, want) {
		t.Errorf("unresolved = %q, want %q", p, want)
	}
	if p, want := pairs(got, ImportExternal), []string{parseImporter + " -> "}; !slices.Equal(p, want) {
		t.Errorf("external = %q, want %q: fmt only", p, want)
	}
	for _, src := range []string{parseTagged, parseTestFile, parseTestdata, parseVendor, parseUnderscoreDir, parseUnderscoreFile, parseNested, parseSkipped, parseCgoFile} {
		if slices.Contains(got.Sources, src) {
			t.Errorf("%s is a source; the go command does not read it", src)
		}
	}
	for _, src := range []string{"root.go", "internal/d/d.go", parseToolsGen} {
		if !slices.Contains(got.Sources, src) {
			t.Errorf("%s is not a source", src)
		}
	}
	if len(got.Unknown) != 0 {
		t.Errorf("unknown = %v, want none", got.Unknown)
	}
}

func TestParseImportsLinesAndRootPackage(t *testing.T) {
	setGoEnv(t, "0")
	got := mustParse(t, writeTree(t, parseFixture), parseConfig)
	var importer []FileImport
	for _, imp := range got.Imports {
		if imp.File == parseImporter {
			importer = append(importer, imp)
		}
	}
	want := []FileImport{
		{File: parseImporter, Line: 4, Path: "fmt", Kind: ImportExternal},
		{File: parseImporter, Line: 5, Path: parseModule, Package: "", Kind: ImportFirstParty},
		{File: parseImporter, Line: 6, Path: "example.com/q/internal/b", Package: "internal/b", Kind: ImportFirstParty},
		{File: parseImporter, Line: 7, Path: "example.com/q/tools/gen", Package: "tools/gen", Kind: ImportFirstParty},
	}
	if !slices.Equal(importer, want) {
		t.Errorf("imports of %s = %+v, want %+v", parseImporter, importer, want)
	}
}

func TestParseImportsBuildTags(t *testing.T) {
	for _, tc := range []struct {
		name, goflags string
		flags         []string
	}{
		{name: "GOFLAGS", goflags: "-tags=never_set_tag"},
		{name: "build flags", flags: []string{parseTagsFlag, "never_set_tag"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGoEnv(t, "0")
			t.Setenv("GOFLAGS", tc.goflags)
			cfg := parseConfig
			cfg.BuildFlags = tc.flags
			got := mustParse(t, writeTree(t, parseFixture), cfg)
			if !slices.Contains(pairs(got, ImportFirstParty), parseTagged+" -> internal/c") {
				t.Errorf("a tag the run sets must select %s: %v", parseTagged, got.Imports)
			}
		})
	}
}

func TestParseImportsCgo(t *testing.T) {
	for _, tc := range []struct {
		cgo              string
		source, unknown  bool
		externalIncludeC bool
	}{
		{cgo: "0"},
		{cgo: "1", source: true, externalIncludeC: true},
		{cgo: "", unknown: true},
	} {
		t.Run("CGO_ENABLED="+tc.cgo, func(t *testing.T) {
			setGoEnv(t, tc.cgo)
			got := mustParse(t, writeTree(t, parseFixture), parseConfig)
			if slices.Contains(got.Sources, parseCgoFile) != tc.source {
				t.Errorf("%s source = %v, want %v", parseCgoFile, !tc.source, tc.source)
			}
			if slices.Contains(pairs(got, ImportExternal), parseCgoFile+" -> ") != tc.externalIncludeC {
				t.Errorf("\"C\" external = %v, want %v", !tc.externalIncludeC, tc.externalIncludeC)
			}
			gotUnknown := slices.ContainsFunc(got.Unknown, func(u FileUnknown) bool {
				return u.File == parseCgoFile && strings.Contains(u.Reason, "CGO_ENABLED is not set")
			})
			if gotUnknown != tc.unknown {
				t.Errorf("%s unknown = %v, want %v: %v", parseCgoFile, gotUnknown, tc.unknown, got.Unknown)
			}
		})
	}
}

func TestParseImportsUnparsableFileIsUnknown(t *testing.T) {
	setGoEnv(t, "0")
	files := maps.Clone(parseFixture)
	files[parseBroken] = "package a\n\nimport (\n\t\"fmt\"\n"
	got := mustParse(t, writeTree(t, files), parseConfig)
	if slices.Contains(got.Sources, parseBroken) {
		t.Errorf("%s is a source", parseBroken)
	}
	if len(got.Unknown) != 1 || got.Unknown[0].File != parseBroken || !strings.HasPrefix(got.Unknown[0].Reason, "does not parse") {
		t.Errorf("unknown = %v, want %s that does not parse", got.Unknown, parseBroken)
	}
}

// TestParseImportsStandardLogIsNotFirstParty is the regression for the node
// collision of the standard library log with a first-party <root>/log.
func TestParseImportsStandardLogIsNotFirstParty(t *testing.T) {
	setGoEnv(t, "0")
	got := mustParse(t, writeTree(t, logFixture), evidenceports.ExtractConfig{})
	want := []FileImport{
		{File: "a/a.go", Line: 3, Path: "log", Kind: ImportExternal},
		{File: "b/b.go", Line: 3, Path: "example.com/q/log", Package: "log", Kind: ImportFirstParty},
	}
	if !slices.Equal(got.Imports, want) {
		t.Errorf("imports = %+v, want %+v", got.Imports, want)
	}
}

func TestParseImportsWithoutGoModule(t *testing.T) {
	setGoEnv(t, "0")
	got := mustParse(t, writeTree(t, map[string]string{"main.go": "package main\n"}), evidenceports.ExtractConfig{})
	if len(got.Sources) != 0 || len(got.Imports) != 0 || len(got.Unknown) != 0 {
		t.Errorf("a tree without go.mod has no Go facts: %+v", got)
	}
}
