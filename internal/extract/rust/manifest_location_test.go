package rust_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// appManifest spells dependencies every way this repository's crates do: a
// dotted workspace key, a quoted key with a multi-line inline table, a renamed
// package, a target-specific table, a dev table, a dependency sub-table header,
// and a dependency named like an inline-table field so a continuation line can
// never be taken for it.
const appManifest = `[package]
name = "app"
version = "0.1.0"

[dependencies]
core.workspace = true # shared core
"util" = { path = "../util",
  features = ["x"] }
renamed-log = { package = "log", version = "1" }
features = "0.1"

[target.'cfg(unix)'.dependencies]
unixy = "1"

[dev-dependencies]
core = { workspace = true }
tester = "1"

[build-dependencies.builder]
path = "../builder"
`

const (
	nodeApp     = "package:app"
	nodeCore    = "package:core"
	nameBuilder = "builder"
	nameCore    = "core"
	nameUtil    = "util"
)

const rootManifest = `[workspace]
members = ["crates/*"]

[workspace.dependencies]
core = { path = "crates/core" }

[package]
name = "root-app"

[dependencies]
core.workspace = true
`

type metaDep struct {
	Name   string  `json:"name"`
	Kind   *string `json:"kind"`
	Rename *string `json:"rename"`
}

type metaPkg struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ManifestPath string    `json:"manifest_path"`
	Dependencies []metaDep `json:"dependencies"`
}

func ptr(s string) *string { return &s }

// writeManifests lays out a workspace on disk and returns the cargo metadata
// JSON cargo would print for it. cargo reports canonical absolute manifest
// paths, so they are built from the symlink-resolved root while the caller
// keeps scanning the root as spelled.
func writeManifests(t *testing.T, root string, outside string) []byte {
	t.Helper()
	write := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("Cargo.toml", rootManifest)
	write("crates/app/Cargo.toml", appManifest)
	for _, name := range []string{nameCore, nameUtil, nameBuilder} {
		write("crates/"+name+"/Cargo.toml", "[package]\nname = \""+name+"\"\n")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := func(rel string) string { return filepath.Join(canonical, filepath.FromSlash(rel)) }
	pkgs := []metaPkg{
		{ID: "root-app", Name: "root-app", ManifestPath: manifest("Cargo.toml"), Dependencies: []metaDep{{Name: nameCore}}},
		{ID: "app", Name: "app", ManifestPath: manifest("crates/app/Cargo.toml"), Dependencies: []metaDep{
			{Name: nameCore}, {Name: nameUtil}, {Name: "log", Rename: ptr("renamed-log")}, {Name: "features"},
			{Name: "unixy"}, {Name: nameCore, Kind: ptr("dev")}, {Name: "tester", Kind: ptr("dev")},
			{Name: nameBuilder, Kind: ptr("build")},
		}},
		{ID: nameCore, Name: nameCore, ManifestPath: manifest("crates/core/Cargo.toml")},
		{ID: nameUtil, Name: nameUtil, ManifestPath: manifest("crates/util/Cargo.toml")},
		{ID: nameBuilder, Name: nameBuilder, ManifestPath: manifest("crates/builder/Cargo.toml")},
		{ID: "far", Name: "far", ManifestPath: filepath.Join(outside, "Cargo.toml"), Dependencies: []metaDep{{Name: nameCore}}},
	}
	members := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		members = append(members, p.ID)
	}
	data, err := json.Marshal(map[string]any{"packages": pkgs, "workspace_members": members, "workspace_root": canonical})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// lineOf returns the 1-based line of the first line in text containing needle.
func lineOf(t *testing.T, text, needle string) int {
	t.Helper()
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	t.Fatalf("%q not in manifest", needle)
	return 0
}

// TestExtract_DependencyEdgesPointAtTheDeclaringManifestLine pins where a crate
// dependency edge is located: the member's own Cargo.toml, at the line that
// declares the dependency in the table of its kind. A form the scan cannot read
// keeps the manifest with line 0, and a member outside the analysed root gets
// no location rather than an escaping path.
func TestExtract_DependencyEdgesPointAtTheDeclaringManifestLine(t *testing.T) {
	root := t.TempDir()
	data := writeManifests(t, root, t.TempDir())
	e := rust.New(mockRunner(data), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto})
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	const appToml = "crates/app/Cargo.toml"
	tests := []struct {
		name, from, to string
		want           []graph.Location
	}{
		{"dotted workspace key", nodeApp, nodeCore, []graph.Location{{File: appToml, Line: lineOf(t, appManifest, "core.workspace")}}},
		{"quoted key with a multi-line inline table", nodeApp, "package:util", []graph.Location{{File: appToml, Line: lineOf(t, appManifest, `"util"`)}}},
		{"renamed package uses its key", nodeApp, "external:log", []graph.Location{{File: appToml, Line: lineOf(t, appManifest, "renamed-log")}}},
		{"a continuation line is never a dependency", nodeApp, "external:features", []graph.Location{{File: appToml, Line: lineOf(t, appManifest, `features = "0.1"`)}}},
		{"target-specific table", nodeApp, "external:unixy", []graph.Location{{File: appToml, Line: lineOf(t, appManifest, "unixy")}}},
		{"dependency sub-table header", nodeApp, "package:builder", []graph.Location{{File: appToml, Line: lineOf(t, appManifest, "[build-dependencies.builder]")}}},
		{"root member ignores workspace.dependencies", "package:root-app", nodeCore, []graph.Location{{File: "Cargo.toml", Line: lineOf(t, rootManifest, "core.workspace")}}},
		{"member outside the root has no location", "package:far", nodeCore, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			edge := hasEdge(facts, tc.from, tc.to)
			if edge == nil {
				t.Fatalf("missing edge %s -> %s", tc.from, tc.to)
			}
			if len(edge.Locations) != len(tc.want) || (len(tc.want) > 0 && edge.Locations[0] != tc.want[0]) {
				t.Errorf("locations = %+v, want %+v", edge.Locations, tc.want)
			}
		})
	}
	if hasEdge(facts, nodeApp, "external:tester") != nil {
		t.Error("dev-dependency tester must not become an edge without include_dev_deps")
	}
}

// TestExtract_DevDependencyLineComesFromTheDevTable pins kind matching: with
// dev-dependencies included, a dev-only dependency is located in
// [dev-dependencies], never at a same-named key of another table.
func TestExtract_DevDependencyLineComesFromTheDevTable(t *testing.T) {
	root := t.TempDir()
	data := writeManifests(t, root, t.TempDir())
	e := rust.New(mockRunner(data), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, IncludeDevDeps: true})
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	edge := hasEdge(facts, nodeApp, "external:tester")
	want := graph.Location{File: "crates/app/Cargo.toml", Line: lineOf(t, appManifest, `tester = "1"`)}
	if edge == nil || len(edge.Locations) != 1 || edge.Locations[0] != want {
		t.Fatalf("tester edge = %+v, want located at %+v", edge, want)
	}
}

// TestExtract_UnreadableManifestKeepsTheFileWithoutALine pins the no-guess
// rule: a manifest with a multi-line string cannot be scanned safely, so its
// edges carry the member manifest with line 0.
func TestExtract_UnreadableManifestKeepsTheFileWithoutALine(t *testing.T) {
	root := t.TempDir()
	data := writeManifests(t, root, t.TempDir())
	odd := strings.Replace(appManifest, "[package]", "[package]\ndescription = \"\"\"\n[dependencies]\ncore = 1\n\"\"\"", 1)
	if err := os.WriteFile(filepath.Join(root, "crates", "app", "Cargo.toml"), []byte(odd), 0o600); err != nil {
		t.Fatal(err)
	}
	e := rust.New(mockRunner(data), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto})
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	edge := hasEdge(facts, nodeApp, nodeCore)
	want := graph.Location{File: "crates/app/Cargo.toml"}
	if edge == nil || len(edge.Locations) != 1 || edge.Locations[0] != want {
		t.Fatalf("core edge = %+v, want located at %+v", edge, want)
	}
}
