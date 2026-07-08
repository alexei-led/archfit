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
	// metrics is split into family sub-packages; assert they all load.
	modulePrefix + "internal/metrics/boundary",
	modulePrefix + "internal/metrics/modularity",
	modulePrefix + "internal/metrics/internal/result",
	modulePrefix + "internal/status",
	modulePrefix + "internal/staleness",
	modulePrefix + "internal/facts",
	// score synthesises the banded scorecard from an already-computed
	// Diagnostic — a pure decision over collected facts, no tools or I/O.
	modulePrefix + "internal/score",
	// decision converts a Diagnostic + Scorecard into a human-decision view-model —
	// pure synthesis, no I/O, no subprocess, no YAML.
	modulePrefix + "internal/decision",
	// scope resolves the analysis boundary from config + git; it uses os.Stat
	// and filepath.EvalSymlinks for path canonicalization (justified I/O — no
	// subprocess, no YAML, no adapter). Excluded from the os-forbidden check.
	// modulePrefix + "internal/scope" — see scopeOsAllowed carve-out below.
	// syntax derives roles from already-gathered SyntaxFacts — pure decision,
	// no I/O, no subprocess.
	modulePrefix + "internal/syntax",
}

// coreRingPrefixes are path prefixes whose packages — and ALL their
// sub-packages — must obey the core-ring import rules. internal/metrics is split
// into family sub-packages (boundary, modularity, internal/result, metricstest),
// so a prefix match keeps every current and future sub-package covered without
// editing this list.
var coreRingPrefixes = []string{
	modulePrefix + "internal/classify",
	modulePrefix + "internal/rules",
	modulePrefix + "internal/metrics",
	modulePrefix + "internal/status",
	modulePrefix + "internal/staleness",
	modulePrefix + "internal/facts",
	modulePrefix + "internal/score",
	modulePrefix + "internal/scope",
	modulePrefix + "internal/syntax",
	modulePrefix + "internal/decision",
}

// inCoreRing reports whether pkgPath is a core-ring package: an exact prefix
// match or a sub-package of one (prefix + "/").
func inCoreRing(pkgPath string) bool {
	for _, p := range coreRingPrefixes {
		if pkgPath == p || strings.HasPrefix(pkgPath, p+"/") {
			return true
		}
	}
	return false
}

// modelPkgs must not import anything outside the standard library (or each
// other, which is stdlib-only by this rule applied transitively).
var modelPkgs = []string{
	modulePrefix + "internal/model/graph",
	modulePrefix + "internal/model/finding",
	modulePrefix + "internal/model/coupling",
	modulePrefix + "internal/model/diagnostic",
	modulePrefix + "internal/model/fileclass",
	modulePrefix + "internal/model/symbol",
	modulePrefix + "internal/model/clone",
	modulePrefix + "internal/model/pattern",
	modulePrefix + "internal/model/signal",
	modulePrefix + "internal/model/module",
}

// modelThirdPartyAllowed lists vetted, pure third-party imports allowed for a
// specific model package. doublestar is a pure glob matcher (no I/O) used by
// module path resolution.
var modelThirdPartyAllowed = map[string]map[string]bool{
	modulePrefix + "internal/model/module": {"github.com/bmatcuk/doublestar/v4": true},
}

// adapterPrefixes are packages the core ring must never import — adapters AND
// the orchestrator. Adapters depend on internal/ports, never the orchestrator.
var adapterPrefixes = []string{
	modulePrefix + "internal/engine",
	modulePrefix + "internal/baseline",
	modulePrefix + "internal/toolrun",
	modulePrefix + "internal/extract/",
	modulePrefix + "internal/history/",
	modulePrefix + "internal/output/",
	modulePrefix + "internal/labels/labelsio", // labels file I/O adapter (os + YAML)
	modulePrefix + "internal/factcache",       // extractor-fact cache adapter (os I/O)
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
		// Prefix scan over every loaded package so metrics family sub-packages
		// (and any future ones) are covered, not just the exact paths above.
		for pkgPath, pkg := range loaded {
			if !inCoreRing(pkgPath) {
				continue
			}
			for imp := range pkg.Imports {
				if isForbiddenForCoreIn(pkgPath, imp) {
					t.Errorf("core ring package %s must not import %q", pkgPath, imp)
				}
			}
		}
	})

	t.Run("llm_ring_unreachable_from_internal", func(t *testing.T) {
		// The LLM-off-gate guarantee, enforced structurally: NO internal
		// package may import internal/llm — only cmd may host explicit LLM flows.
		// This covers the whole check pipeline: engine, classify, labels,
		// metrics, renderers can never call a model.
		const llmPkg = modulePrefix + "internal/llm"
		for pkgPath, pkg := range loaded {
			if pkgPath == llmPkg {
				continue
			}
			if _, imports := pkg.Imports[llmPkg]; imports {
				t.Errorf("package %s must not import %s: the check gate is LLM-free; only cmd may use the LLM layer", pkgPath, llmPkg)
			}
		}
	})

	t.Run("adapters_no_engine_import", func(t *testing.T) {
		// Adapters (toolrun, extract/*, history/*, output/*) must depend on
		// internal/ports, never on the orchestrator (internal/engine). This
		// ensures the hexagonal boundary: ports live in a neutral package that
		// both adapters and the orchestrator can import without creating a cycle.
		const enginePkg = modulePrefix + "internal/engine"
		adapterPkgPrefixes := []string{
			modulePrefix + "internal/toolrun",
			modulePrefix + "internal/extract/",
			modulePrefix + "internal/history/",
			modulePrefix + "internal/output/",
		}
		for pkgPath, pkg := range loaded {
			isAdapter := false
			for _, prefix := range adapterPkgPrefixes {
				if strings.HasPrefix(pkgPath, prefix) {
					isAdapter = true
					break
				}
			}
			if !isAdapter {
				continue
			}
			if _, imports := pkg.Imports[enginePkg]; imports {
				t.Errorf("adapter package %s must not import %s: use internal/ports instead", pkgPath, enginePkg)
			}
		}
	})

	t.Run("labelsio_unreachable_from_internal", func(t *testing.T) {
		// The labels I/O adapter (os + YAML) must be reachable only from cmd. No
		// internal package — the engine, the core ring, anything — may import it:
		// the engine consumes the PURE internal/labels helpers and receives loaded
		// labels from cmd. Keeps the gate's import closure free of label-file I/O.
		const labelsioPkg = modulePrefix + "internal/labels/labelsio"
		for pkgPath, pkg := range loaded {
			if pkgPath == labelsioPkg {
				continue
			}
			if _, imports := pkg.Imports[labelsioPkg]; imports {
				t.Errorf("package %s must not import %s: only cmd may load label files; internal code uses the pure internal/labels", pkgPath, labelsioPkg)
			}
		}
	})

	t.Run("model_stdlib_only", func(t *testing.T) {
		checkModelStdlibOnly(t, loaded)
	})

	t.Run("view_kernel_only", func(t *testing.T) { checkViewKernelOnly(t, loaded) })
}

// checkModelStdlibOnly asserts every model kernel package imports only the
// stdlib, sibling model packages, or an explicitly vetted pure third-party
// dependency (modelThirdPartyAllowed).
func checkModelStdlibOnly(t *testing.T, loaded map[string]*packages.Package) {
	t.Helper()
	for _, pkgPath := range modelPkgs {
		pkg, ok := loaded[pkgPath]
		if !ok {
			continue // already reported in presence check
		}
		for imp := range pkg.Imports {
			if !isStdlib(imp) && !isModelPkg(imp) && !modelThirdPartyAllowed[pkgPath][imp] {
				t.Errorf("model package %s must not import non-stdlib %q", pkgPath, imp)
			}
		}
	}
}

// checkViewKernelOnly asserts internal/view (stage-contract types: data-only
// projections) imports only the stdlib and the model kernel — never config,
// engine, adapters, or any behavior-bearing package.
func checkViewKernelOnly(t *testing.T, loaded map[string]*packages.Package) {
	t.Helper()
	const viewPkg = modulePrefix + "internal/view"
	pkg, ok := loaded[viewPkg]
	if !ok {
		t.Fatalf("expected view package not loaded: %s", viewPkg)
	}
	for imp := range pkg.Imports {
		if !isStdlib(imp) && !isModelPkg(imp) {
			t.Errorf("view package must not import non-kernel %q: stage contracts are data-only", imp)
		}
	}
}

func TestCouplingModelDoesNotImportGraph(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedImports, Dir: "."}
	pkgs, err := packages.Load(cfg, modulePrefix+"internal/model/coupling")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("packages.Load returned %d packages, want 1", len(pkgs))
	}
	const graphPkg = modulePrefix + "internal/model/graph"
	if _, imports := pkgs[0].Imports[graphPkg]; imports {
		t.Fatalf("internal/model/coupling must not import %s; keep clone evidence on coupling's narrow Location DTO", graphPkg)
	}
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

// coreRingStdlibAllowlist maps a core-ring package prefix to stdlib imports that
// are explicitly allowed despite being forbidden for the rest of the ring.
// Use sparingly — each entry needs a justification comment.
var coreRingStdlibAllowlist = map[string]map[string]bool{
	// scope performs path-identity checks (os.Stat/os.SameFile in snapScanRoot)
	// — same class of I/O as filepath.EvalSymlinks already used there.
	// os/exec and YAML remain forbidden.
	modulePrefix + "internal/scope": {"os": true},
}

// isForbiddenForCoreIn is isForbiddenForCore with per-package allowlist support.
func isForbiddenForCoreIn(pkgPath, imp string) bool {
	if !isForbiddenForCore(imp) {
		return false
	}
	for prefix, allowed := range coreRingStdlibAllowlist {
		if (pkgPath == prefix || strings.HasPrefix(pkgPath, prefix+"/")) && allowed[imp] {
			return false
		}
	}
	return true
}

// isStdlib reports whether imp is a standard library package.
// A package is stdlib if its first path segment contains no dot
// (e.g. "fmt", "encoding/json" — not "github.com/..." or "golang.org/...").
func isStdlib(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}

// isModelPkg reports whether imp is a model/* package within this module,
// which model packages are allowed to import each other.
func isModelPkg(imp string) bool {
	return strings.HasPrefix(imp, modulePrefix+"internal/model/")
}

// TestInCoreRing verifies the prefix matcher covers metrics family sub-packages
// without over-matching a same-prefix sibling (e.g. "internal/metricsx").
func TestInCoreRing(t *testing.T) {
	cases := map[string]bool{
		modulePrefix + "internal/metrics":                 true,
		modulePrefix + "internal/metrics/boundary":        true,
		modulePrefix + "internal/metrics/internal/result": true,
		modulePrefix + "internal/metrics/metricstest":     true,
		modulePrefix + "internal/scope":                   true,
		modulePrefix + "internal/engine":                  false,
		modulePrefix + "internal/output/markdown":         false,
		modulePrefix + "internal/metricsx":                false, // must not over-match the prefix
	}
	for path, want := range cases {
		if got := inCoreRing(path); got != want {
			t.Errorf("inCoreRing(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestIsForbiddenForCore verifies that a core-ring package (including a metrics
// sub-package) importing an adapter, os/exec, or a YAML library is rejected,
// while stdlib, model, config, and the shared result package are allowed.
func TestIsForbiddenForCore(t *testing.T) {
	forbidden := []string{
		"os", "os/exec",
		"github.com/goccy/go-yaml",
		"gopkg.in/yaml.v3",
		modulePrefix + "internal/engine",
		modulePrefix + "internal/output/markdown",
		modulePrefix + "internal/toolrun",
		modulePrefix + "internal/factcache",
	}
	for _, imp := range forbidden {
		if !isForbiddenForCore(imp) {
			t.Errorf("isForbiddenForCore(%q) = false, want true", imp)
		}
	}
	allowed := []string{
		"fmt", "strings", "sort",
		modulePrefix + "internal/model/diagnostic",
		modulePrefix + "internal/metrics/internal/result",
		modulePrefix + "internal/config",
	}
	for _, imp := range allowed {
		if isForbiddenForCore(imp) {
			t.Errorf("isForbiddenForCore(%q) = true, want false", imp)
		}
	}
}
