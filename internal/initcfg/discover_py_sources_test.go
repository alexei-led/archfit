package initcfg

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	appDomain = "app.domain"
	libPkg    = "lib"
)

// TestDiscoverPy_SourcesCoverTheSubtree pins Python Sources to every dotted
// module a discovered module's paths (mod, mod.*) claim, the way Go discovery
// lists every package, so config update's ownership pass sees nested code.
func TestDiscoverPy_SourcesCoverTheSubtree(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{
		"pyproject.toml",
		"src/app/__init__.py", "src/app/domain/__init__.py", "src/app/domain/service.py",
		"src/app/domain/_private.py", "src/app/domain/sub/__init__.py", "src/app/domain/sub/x.py",
		"src/app/domain/__pycache__/service.cpython-312.pyc", "src/app/domain/notes/readme.py",
		"lib/__init__.py", "lib/util.py",
	} {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), testDirPerm); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mods, err := DiscoverPy(root)
	if err != nil {
		t.Fatalf("DiscoverPy: %v", err)
	}
	got := map[string][]string{}
	for _, m := range mods {
		got[m.Name] = m.Sources
	}
	want := map[string][]string{
		"domain": {appDomain, appDomain + "._private", appDomain + ".service", appDomain + ".sub", appDomain + ".sub.x"},
		libPkg:   {libPkg, libPkg + ".util"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sources = %v, want %v", got, want)
	}

	// A stanza owning only the package node leaves nested modules unowned, so
	// the discovered module stays an addition rather than reading as covered.
	existing := []ExistingModule{{Name: "domain-root", Paths: []string{appDomain}, HasOwner: true, HasSubdomain: true}}
	ownerOf := policy.BuildModuleMap(map[string]policy.ModuleDef{"domain-root": {Paths: []string{appDomain}}}).ModuleFor
	var domain ModuleDef
	for _, m := range mods {
		if m.Name == "domain" {
			domain = m
		}
	}
	report := DiffModules(existing, []ModuleDef{domain}, false, ownerOf)
	if len(report.Added) != 1 || len(report.Covered) != 0 {
		t.Fatalf("added = %v, covered = %v; want the partly owned module added", report.Added, report.Covered)
	}
}
