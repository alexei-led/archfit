package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/manifest"
	reportmodel "github.com/alexei-led/archfit/internal/model/evidence"
)

const (
	fileGoMod      = "go.mod"
	filePkgJSON    = "package.json"
	kindRetract    = "retract"
	kindDeprecated = "deprecated"
)

// writeFile writes content to path (creating parent dirs).
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writefile: %v", err)
	}
}

func TestScan_GoMod_NoRetract(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, fileGoMod), `module example.com/mymod

go 1.21
`)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps, got %d: %v", len(got), got)
	}
}

func TestScan_GoMod_SingleVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, fileGoMod), `module example.com/mymod

go 1.21

retract v1.0.0 // accidental publish
`)
	got := manifest.Scan(root)
	want := []reportmodel.DeprecatedDep{
		{File: fileGoMod, Kind: kindRetract, Subject: "v1.0.0", Note: "accidental publish"},
	}
	assertDeps(t, got, want)
}

func TestScan_GoMod_Range(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, fileGoMod), `module example.com/mymod

go 1.21

retract [v1.0.0, v1.0.5]
`)
	got := manifest.Scan(root)
	want := []reportmodel.DeprecatedDep{
		{File: fileGoMod, Kind: kindRetract, Subject: "[v1.0.0, v1.0.5]", Note: ""},
	}
	assertDeps(t, got, want)
}

func TestScan_GoMod_Block(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, fileGoMod), `module example.com/mymod

go 1.21

retract (
	v1.0.0 // broken release
	[v1.1.0, v1.1.3]
	v1.2.0
)
`)
	got := manifest.Scan(root)
	want := []reportmodel.DeprecatedDep{
		{File: fileGoMod, Kind: kindRetract, Subject: "[v1.1.0, v1.1.3]", Note: ""},
		{File: fileGoMod, Kind: kindRetract, Subject: "v1.0.0", Note: "broken release"},
		{File: fileGoMod, Kind: kindRetract, Subject: "v1.2.0", Note: ""},
	}
	assertDeps(t, got, want)
}

func TestScan_GoMod_NoVersionAfterRetract(t *testing.T) {
	// A bare "retract" keyword with no version should produce nothing.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, fileGoMod), `module example.com/mymod

go 1.21

retract
`)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps, got %d", len(got))
	}
}

func TestScan_PackageJSON_Deprecated(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, filePkgJSON), `{
  "name": "my-pkg",
  "version": "1.0.0",
  "deprecated": "use new-pkg instead"
}`)
	got := manifest.Scan(root)
	want := []reportmodel.DeprecatedDep{
		{File: filePkgJSON, Kind: kindDeprecated, Subject: "my-pkg", Note: "use new-pkg instead"},
	}
	assertDeps(t, got, want)
}

func TestScan_PackageJSON_NoDeprecated(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, filePkgJSON), `{
  "name": "my-pkg",
  "version": "1.0.0"
}`)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps, got %d: %v", len(got), got)
	}
}

func TestScan_PackageJSON_DeprecatedBool(t *testing.T) {
	// {"deprecated": true} — not a string, should be skipped.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, filePkgJSON), `{
  "name": "my-pkg",
  "deprecated": true
}`)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps (bool deprecated), got %d: %v", len(got), got)
	}
}

func TestScan_PackageJSON_Malformed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, filePkgJSON), `{ not valid json `)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps from malformed JSON, got %d", len(got))
	}
}

func TestScan_PackageJSON_NoName(t *testing.T) {
	// No "name" field — Subject falls back to the relative file path.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, filePkgJSON), `{
  "deprecated": "no longer maintained"
}`)
	got := manifest.Scan(root)
	if len(got) != 1 {
		t.Fatalf("want 1 dep, got %d: %v", len(got), got)
	}
	if got[0].Subject != filePkgJSON {
		t.Errorf("subject: got %q, want %q", got[0].Subject, filePkgJSON)
	}
}

func TestScan_NodeModulesSkipped(t *testing.T) {
	root := t.TempDir()
	// A deprecated package.json inside node_modules must be skipped.
	writeFile(t, filepath.Join(root, "node_modules", "old-lib", filePkgJSON), `{
  "name": "old-lib",
  "deprecated": "use new-lib"
}`)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps (node_modules skipped), got %d: %v", len(got), got)
	}
}

func TestScan_VendorSkipped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "example.com", "mymod", fileGoMod), `module example.com/mymod

go 1.21

retract v1.0.0 // test
`)
	got := manifest.Scan(root)
	if len(got) != 0 {
		t.Errorf("want 0 deps (vendor skipped), got %d: %v", len(got), got)
	}
}

func TestScan_Mixed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, fileGoMod), `module example.com/mymod

go 1.21

retract v1.0.0 // bad
`)
	writeFile(t, filepath.Join(root, filePkgJSON), `{
  "name": "frontend",
  "deprecated": "use v2 instead"
}`)
	got := manifest.Scan(root)
	// Sort order: go.mod < package.json alphabetically.
	if len(got) != 2 {
		t.Fatalf("want 2 deps, got %d: %v", len(got), got)
	}
	if got[0].File != fileGoMod || got[0].Kind != kindRetract {
		t.Errorf("first dep: got %+v", got[0])
	}
	if got[1].File != filePkgJSON || got[1].Kind != kindDeprecated {
		t.Errorf("second dep: got %+v", got[1])
	}
}

// assertDeps checks that got matches want exactly (same length and fields).
// Both slices must already be sorted (Scan sorts by File+Kind+Subject).
func assertDeps(t *testing.T, got, want []reportmodel.DeprecatedDep) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.File != w.File || g.Kind != w.Kind || g.Subject != w.Subject || g.Note != w.Note {
			t.Errorf("[%d]: got %+v, want %+v", i, g, w)
		}
	}
}
