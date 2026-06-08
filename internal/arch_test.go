// Package arch_test enforces import ring rules for the archfit module.
// It is CI gate 1: core ring packages (classify, rules, metrics, status)
// must not directly import os, os/exec, any YAML library, or adapter
// packages. model/* packages must not import anything outside stdlib.
//
// Run with: go test ./internal/ -run TestArchImports
package arch_test

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const modulePrefix = "github.com/alexei-led/archfit/"

// coreRingPkgs are the packages that must not import os, os/exec, YAML libs,
// or adapter packages.
var coreRingPkgs = []string{
	modulePrefix + "internal/classify",
	modulePrefix + "internal/rules",
	modulePrefix + "internal/metrics",
	modulePrefix + "internal/status",
}

// modelPkgs must not import anything outside the standard library (or each
// other, which is stdlib-only by this rule applied transitively).
var modelPkgs = []string{
	modulePrefix + "internal/model/graph",
	modulePrefix + "internal/model/finding",
	modulePrefix + "internal/model/coupling",
	modulePrefix + "internal/model/diagnostic",
}

// adapterPrefixes are adapter package paths that core ring packages must not
// import.
var adapterPrefixes = []string{
	modulePrefix + "internal/toolrun",
	modulePrefix + "internal/extract/",
	modulePrefix + "internal/history/",
	modulePrefix + "internal/output/",
}

// TestArchImports verifies the import ring rules for core and model packages.
func TestArchImports(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports,
		Dir:  ".",
	}

	// Load all internal packages.
	pkgs, err := packages.Load(cfg, modulePrefix+"internal/...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}

	// Index loaded packages by path for presence checks and import inspection.
	loaded := make(map[string]*packages.Package, len(pkgs))
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				t.Errorf("load error in %s: %v", pkg.PkgPath, e)
			}
		}
		loaded[pkg.PkgPath] = pkg
	}
	if t.Failed() {
		t.FailNow()
	}

	t.Run("core_ring_packages_present", func(t *testing.T) {
		for _, want := range coreRingPkgs {
			if _, ok := loaded[want]; !ok {
				t.Errorf("expected core ring package not loaded: %s", want)
			}
		}
	})

	t.Run("model_packages_present", func(t *testing.T) {
		for _, want := range modelPkgs {
			if _, ok := loaded[want]; !ok {
				t.Errorf("expected model package not loaded: %s", want)
			}
		}
	})

	t.Run("core_ring_no_forbidden_imports", func(t *testing.T) {
		for _, pkgPath := range coreRingPkgs {
			pkg, ok := loaded[pkgPath]
			if !ok {
				continue // already reported in presence check
			}
			for imp := range pkg.Imports {
				if isForbiddenForCore(imp) {
					t.Errorf("core ring package %s must not import %q", pkgPath, imp)
				}
			}
		}
	})

	t.Run("model_stdlib_only", func(t *testing.T) {
		for _, pkgPath := range modelPkgs {
			pkg, ok := loaded[pkgPath]
			if !ok {
				continue // already reported in presence check
			}
			for imp := range pkg.Imports {
				if !isStdlib(imp) && !isModelPkg(imp) {
					t.Errorf("model package %s must not import non-stdlib %q", pkgPath, imp)
				}
			}
		}
	})
}

// isForbiddenForCore reports whether imp is forbidden for a core ring package.
// Forbidden: os, os/exec, any YAML library, any adapter package.
func isForbiddenForCore(imp string) bool {
	// Stdlib forbidden paths.
	if imp == "os" || imp == "os/exec" {
		return true
	}
	// YAML libraries.
	if strings.Contains(imp, "go-yaml") || strings.Contains(imp, "yaml.v3") {
		return true
	}
	// Adapter packages.
	for _, prefix := range adapterPrefixes {
		if strings.HasPrefix(imp, prefix) {
			return true
		}
	}
	return false
}

// isStdlib reports whether imp is a standard library package.
// A package is stdlib if its first path segment contains no dot
// (e.g. "fmt", "encoding/json" — not "github.com/..." or "golang.org/...").
func isStdlib(imp string) bool {
	first := imp
	if i := strings.IndexByte(imp, '/'); i >= 0 {
		first = imp[:i]
	}
	return !strings.Contains(first, ".")
}

// isModelPkg reports whether imp is a model/* package within this module,
// which model packages are allowed to import each other.
func isModelPkg(imp string) bool {
	return strings.HasPrefix(imp, modulePrefix+"internal/model/")
}
