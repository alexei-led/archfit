package rust

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
)

// TestCrateRoots asserts cargo's absolute manifest paths become repo-relative crate
// roots: the root crate yields Dir "", nested members keep their slash path, the
// crate name is carried verbatim (not guessed from the dir), and members outside the
// analysed root are skipped rather than emitted with a "../" escape.
func TestCrateRoots(t *testing.T) {
	root := filepath.FromSlash("/repo")
	members := []cargoPackage{
		{Name: "root-crate", ManifestPath: filepath.Join(root, manifestFile)},
		{Name: "grep", ManifestPath: filepath.Join(root, "crates", "grep", manifestFile)},
		{Name: "yazi-fm", ManifestPath: filepath.Join(root, "crates", "fm", manifestFile)}, // name != dir
		{Name: "outside", ManifestPath: filepath.Join(filepath.FromSlash("/elsewhere"), manifestFile)},
	}

	got := crateRoots(root, members)
	want := []graph.CrateRoot{
		{Dir: "", Name: "root-crate"},
		{Dir: "crates/grep", Name: "grep"},
		{Dir: "crates/fm", Name: "yazi-fm"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("crateRoots = %+v, want %+v", got, want)
	}
}
