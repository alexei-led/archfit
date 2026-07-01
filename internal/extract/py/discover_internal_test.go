package py

import (
	"os"
	"path/filepath"
	"testing"
)

// mkPyPackage creates root/<parts...>/__init__.py — an importable Python package.
func mkPyPackage(t *testing.T, root string, parts ...string) {
	t.Helper()
	dir := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "__init__.py"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDiscoverPackages_SrcLayoutPrefersSrc locks the src-layout fix: a project whose
// real package lives under src/ (e.g. prefect at src/prefect/) with only a stray
// top-level package (tests/) must resolve to the src package, not the stray — else
// grimp analyses the wrong package and yields a near-empty import graph.
func TestDiscoverPackages_SrcLayoutPrefersSrc(t *testing.T) {
	root := t.TempDir()
	mkPyPackage(t, root, "src", "prefect") // real source under src/
	mkPyPackage(t, root, "tests")          // stray top-level package

	got := discoverPackages(root)
	if len(got) != 1 || got[0] != "prefect" {
		t.Errorf("discoverPackages = %v, want [prefect] (src-layout must ignore the top-level stray)", got)
	}
}

// TestDiscoverPackages_FlatLayout keeps the flat-layout behaviour: a package directly
// under root (no src/) is discovered as before.
func TestDiscoverPackages_FlatLayout(t *testing.T) {
	root := t.TempDir()
	mkPyPackage(t, root, "mypkg")

	got := discoverPackages(root)
	if len(got) != 1 || got[0] != "mypkg" {
		t.Errorf("discoverPackages = %v, want [mypkg]", got)
	}
}
