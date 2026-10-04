package rust

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	kindLib  = "lib"
	kindBin  = "bin"
	nameGrep = "grep"
	nameTool = "tool"
	binYazi  = "yazi"
	pkgFM    = "yazi-fm"
)

// TestCrateRoots asserts cargo's absolute manifest paths become repo-relative crate
// roots: the root crate yields Dir "", nested members keep their slash path, the
// package name is carried verbatim (not guessed from the dir) beside the crate
// identifier cargo-modules spells its node IDs with, and members outside the
// analysed root are skipped rather than emitted with a "../" escape.
func TestCrateRoots(t *testing.T) {
	root := filepath.FromSlash("/repo")
	lib := func(name string) []cargoTarget { return []cargoTarget{{Name: name, Kind: []string{kindLib}}} }
	members := []cargoPackage{
		{Name: "root-crate", ManifestPath: filepath.Join(root, manifestFile), Targets: lib("root_crate")},
		{Name: nameGrep, ManifestPath: filepath.Join(root, "crates", "grep", manifestFile), Targets: lib(nameGrep)},
		{Name: pkgFM, ManifestPath: filepath.Join(root, "crates", "fm", manifestFile), Targets: []cargoTarget{{Name: binYazi, Kind: []string{kindBin}}}}, // name != dir
		{Name: "outside", ManifestPath: filepath.Join(filepath.FromSlash("/elsewhere"), manifestFile), Targets: lib("outside")},
	}

	got := crateRoots(root, members)
	want := []graph.CrateRoot{
		{Dir: "", Name: "root-crate", Crate: "root_crate"},
		{Dir: "crates/grep", Name: nameGrep, Crate: nameGrep},
		{Dir: "crates/fm", Name: pkgFM, Crate: binYazi},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("crateRoots = %+v, want %+v", got, want)
	}
}

// TestCrateIdentifier pins the crate identifier to the target cargo-modules
// graphs (runCargoModulesForCrate): the library target when there is one, else
// the first binary target, with Cargo's '-' to '_' crate-name normalization.
func TestCrateIdentifier(t *testing.T) {
	tests := []struct {
		name string
		pkg  cargoPackage
		want string
	}{
		{"library target", cargoPackage{Name: "yazi-shared", Targets: []cargoTarget{{Name: "yazi_shared", Kind: []string{kindLib}}}}, "yazi_shared"},
		{"renamed library", cargoPackage{Name: "python-ast", Targets: []cargoTarget{{Name: "ast", Kind: []string{kindLib}}}}, "ast"},
		{"proc-macro target", cargoPackage{Name: "yazi-codegen", Targets: []cargoTarget{{Name: "yazi_codegen", Kind: []string{"proc-macro"}}}}, "yazi_codegen"},
		{"library before binary", cargoPackage{Name: nameTool, Targets: []cargoTarget{{Name: "tool-cli", Kind: []string{kindBin}}, {Name: nameTool, Kind: []string{kindLib}}}}, nameTool},
		{"binary named differently", cargoPackage{Name: pkgFM, Targets: []cargoTarget{{Name: binYazi, Kind: []string{kindBin}}}}, binYazi},
		{"short binary name", cargoPackage{Name: "yazi-cli", Targets: []cargoTarget{{Name: "ya", Kind: []string{kindBin}}}}, "ya"},
		{"hyphenated binary", cargoPackage{Name: "yazi-build", Targets: []cargoTarget{{Name: "yazi-build", Kind: []string{kindBin}}}}, "yazi_build"},
		{"no lib or bin target", cargoPackage{Name: "only-tests", Targets: []cargoTarget{{Name: "it", Kind: []string{"test"}}}}, "only_tests"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pkg.crateIdentifier(); got != tc.want {
				t.Errorf("crateIdentifier() = %q, want %q", got, tc.want)
			}
		})
	}
}
