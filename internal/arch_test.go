// Package arch_test enforces import ring rules for the archfit module.
// It is CI gate 1: core ring packages (classify, rules, metrics, status)
// must not directly import os, os/exec, any YAML library, or adapter
// packages. model/* packages must not import anything outside stdlib.
//
// Run with: go test ./internal/ -run TestArchImports
package arch_test

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/alexei-led/archfit/internal/config"
)

const (
	modulePrefix = "github.com/alexei-led/archfit/"
)

// coreRingPkgs are the packages that must not import os, os/exec, YAML libs,
// or adapter packages.
var coreRingPkgs = []string{
	modulePrefix + "internal/relationship",
	modulePrefix + "internal/relationship/classify",
	modulePrefix + "internal/assessment/rules",
	modulePrefix + "internal/assessment/metrics",
	// metrics is split into family sub-packages; assert they all load.
	modulePrefix + "internal/assessment/metrics/boundary",
	modulePrefix + "internal/assessment/metrics/modularity",
	modulePrefix + "internal/assessment/metrics/internal/result",
	modulePrefix + "internal/assessment/status",
	modulePrefix + "internal/assessment/staleness",
	modulePrefix + "internal/relationship/facts",
	// score synthesises the banded scorecard from an already-computed
	// Diagnostic — a pure decision over collected facts, no tools or I/O.
	modulePrefix + "internal/assessment/score",
	// decision converts a Diagnostic + Scorecard into a human-decision view-model —
	// pure synthesis, no I/O, no subprocess, no YAML.
	modulePrefix + "internal/assessment/decision",
	// scope resolves the analysis boundary from config + git; it uses os.Stat
	// and filepath.EvalSymlinks for path canonicalization (justified I/O — no
	// subprocess, no YAML, no adapter). Excluded from the os-forbidden check.
	// modulePrefix + "internal/scope" — see scopeOsAllowed carve-out below.
	// syntax derives roles from already-gathered SyntaxFacts — pure decision,
	// no I/O, no subprocess.
	modulePrefix + "internal/syntax",
}

// coreRingPrefixes are path prefixes whose packages — and ALL their
// sub-packages — must obey the core-ring import rules. Metrics is split into
// family sub-packages (boundary, modularity, internal/result), so a prefix match
// keeps every current and future sub-package covered without editing this list.
var coreRingPrefixes = []string{
	modulePrefix + "internal/relationship",
	modulePrefix + "internal/assessment/rules",
	modulePrefix + "internal/assessment/metrics",
	modulePrefix + "internal/assessment/status",
	modulePrefix + "internal/assessment/staleness",
	modulePrefix + "internal/relationship/facts",
	modulePrefix + "internal/assessment/score",
	modulePrefix + "internal/scope",
	modulePrefix + "internal/syntax",
	modulePrefix + "internal/assessment/decision",
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

// modelThirdPartyAllowed lists vetted, pure third-party imports allowed for a
// specific model package. doublestar is a pure glob matcher (no I/O) used by
// module path resolution.
var modelThirdPartyAllowed = map[string]map[string]bool{
	modulePrefix + "internal/model/module": {"github.com/bmatcuk/doublestar/v4": true},
}

// adapterPrefixes are packages the core ring must never import — adapters AND
// the orchestrator. Adapters depend on internal/evidence/ports (or the
// internal/report/ports rendering port), never the orchestrator.
var adapterPrefixes = []string{
	modulePrefix + "internal/analysispipeline",
	modulePrefix + "internal/baseline",
	modulePrefix + "internal/toolrun",
	modulePrefix + "internal/extract/",
	modulePrefix + "internal/history/",
	modulePrefix + "internal/output/",
	modulePrefix + "internal/labels/labelsio", // labels file I/O adapter (os + YAML)
	modulePrefix + "internal/factcache",       // extractor-fact cache adapter (os I/O)
}

func TestCompositionFanoutRatchet(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../.archfit.yaml")
	if err != nil {
		t.Fatalf("load self config: %v", err)
	}
	moduleMap := cfg.ModuleMapView()
	for _, tc := range []struct {
		pkg string
		max int
	}{{modulePrefix + "cmd/archfit", 14}, {modulePrefix + "internal/analysispipeline", 14}} {
		t.Run(tc.pkg, func(t *testing.T) {
			loaded, loadErr := packages.Load(&packages.Config{Mode: packages.NeedImports, Dir: ".."}, tc.pkg)
			if loadErr != nil || len(loaded) != 1 {
				t.Fatalf("load %s: packages=%d err=%v", tc.pkg, len(loaded), loadErr)
			}
			modules := map[string]struct{}{}
			for imp := range loaded[0].Imports {
				if !strings.HasPrefix(imp, modulePrefix+"internal/") {
					continue
				}
				path := strings.TrimPrefix(imp, modulePrefix)
				name, ok := moduleMap.ModuleFor(path + "/package.go")
				if !ok {
					name, ok = moduleMap.ModuleFor(path)
				}
				if ok {
					modules[name] = struct{}{}
				}
			}
			if len(modules) > tc.max {
				t.Errorf("%s direct module fan-out = %d, want <= %d: %v", tc.pkg, len(modules), tc.max, modules)
			}
		})
	}
}

func TestCommandDoesNotImportRelationshipClassify(t *testing.T) {
	for _, file := range productionImportFiles(t, modulePrefix+"internal/relationship/classify") {
		if strings.HasPrefix(file, "cmd/archfit/") {
			t.Errorf("command composition must use engine classification stages: %s", file)
		}
	}
}

func TestAssessmentProductionDoesNotImportRawGraphOrCoupling(t *testing.T) {
	for _, tc := range []struct {
		importPath string
		label      string
	}{
		{modulePrefix + "internal/model/graph", "raw graph"},
		{modulePrefix + "internal/relationship/coupling", "classification internals"},
	} {
		for _, file := range productionImportFiles(t, tc.importPath) {
			if strings.HasPrefix(file, "internal/assessment/") {
				t.Errorf("assessment production package must consume relationship.Set, not %s: %s", tc.label, file)
			}
		}
	}
}

// TestTransitionalImportRatchet caps how many production files may depend on a
// package the capability migration is dissolving. The migration only ever
// deletes these imports, so the caps are upper bounds: a task that pushes a
// count up has widened the seam it was supposed to narrow. Lower a cap in the
// same commit that removes the imports.
func TestTransitionalImportRatchet(t *testing.T) {
	for _, tc := range []struct {
		importPath string
		max        int
		why        string
	}{
		{modulePrefix + "internal/view", 33, "internal/view is deleted in Task 2; policy owns these contracts"},
		{modulePrefix + "internal/analysispipeline", 9, "internal/analysispipeline is deleted in Task 4"},
		{modulePrefix + "internal/evidence", 3, "only relationship analysis may receive the full evidence snapshot"},
	} {
		t.Run(strings.TrimPrefix(tc.importPath, modulePrefix), func(t *testing.T) {
			files := productionImportFiles(t, tc.importPath)
			if len(files) > tc.max {
				t.Errorf("production importers of %s = %d, want <= %d (%s): %v",
					tc.importPath, len(files), tc.max, tc.why, files)
			}
		})
	}
}

// TestTransitionalContractSurfaceRatchet caps the exported surface of the
// packages the migration reshapes. Task 2-4 move declarations to their owning
// capability and privatize the rest, so every cap is an upper bound that must
// fall, never rise. It complements TestModelSurfaceNoDrift, which pins the
// frozen kernel exactly; these packages are still moving, so a count is the
// tightest honest assertion.
func TestTransitionalContractSurfaceRatchet(t *testing.T) {
	targets := []struct {
		pkg string
		max int
	}{
		{modulePrefix + "internal/analysispipeline", 88},
		{modulePrefix + "internal/relationship", 55},
		{modulePrefix + "internal/assessment/result", 47},
		{modulePrefix + "internal/view", 29},
		{modulePrefix + "internal/evidence", 8},
		{modulePrefix + "internal/policy", 8},
	}
	paths := make([]string, 0, len(targets))
	for _, tc := range targets {
		paths = append(paths, tc.pkg)
	}
	loaded, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: ".."}, paths...)
	if err != nil {
		t.Fatalf("load transitional contract packages: %v", err)
	}
	counts := make(map[string]int, len(loaded))
	for _, pkg := range loaded {
		if pkg.Types == nil {
			t.Fatalf("no type information for %s", pkg.PkgPath)
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			if scope.Lookup(name).Exported() {
				counts[pkg.PkgPath]++
			}
		}
	}
	for _, tc := range targets {
		got, ok := counts[tc.pkg]
		if !ok {
			continue // package already deleted by a later migration task
		}
		if got > tc.max {
			t.Errorf("%s exported surface = %d, want <= %d: the migration narrows these contracts, it never widens them",
				tc.pkg, got, tc.max)
		}
	}
}

func TestDomainPackagesDoNotImportReportDTOs(t *testing.T) {
	const reportPackage = modulePrefix + "internal/model/report"
	for _, domain := range []string{"internal/assessment/", "internal/relationship/"} {
		for _, file := range productionImportFiles(t, reportPackage) {
			if strings.HasPrefix(file, domain) {
				t.Errorf("%s must not import report DTOs: %s", domain, file)
			}
		}
	}
}

func productionImportFiles(t *testing.T, importPath string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".archfit-cache", "testdata", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			if strings.Trim(imp.Path.Value, "\"") == importPath {
				files = append(files, filepath.ToSlash(strings.TrimPrefix(path, "../")))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production imports: %v", err)
	}
	return files
}

// TestArchImports verifies the import ring rules for core and model packages.
func TestPolicyDoesNotImportConfig(t *testing.T) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedImports, Dir: ".."}, modulePrefix+"internal/policy")
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("packages.Load policy: packages=%d err=%v", len(pkgs), err)
	}
	if _, ok := pkgs[0].Imports[modulePrefix+"internal/config"]; ok {
		t.Fatal("policy must be a domain context; YAML config is an adapter")
	}
}

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
		assertAdaptersNoEngineImport(t, loaded)
	})

	t.Run("adapters_no_assessment_application_import", func(t *testing.T) {
		assertAdaptersNoAssessmentApplicationImport(t, loaded)
	})

	t.Run("report_adapters_no_domain_imports", func(t *testing.T) {
		assertReportAdaptersNoDomainImports(t, loaded)
	})

	t.Run("report_contract_no_finding_import", func(t *testing.T) {
		const reportPkg = modulePrefix + "internal/model/report"
		const findingPkg = modulePrefix + "internal/assessment/finding"
		if pkg, ok := loaded[reportPkg]; ok {
			if _, imports := pkg.Imports[findingPkg]; imports {
				t.Errorf("report contract %s must not import finding domain %s", reportPkg, findingPkg)
			}
		} else {
			t.Errorf("report contract package %s was not loaded", reportPkg)
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
				t.Errorf("package %s must not import %s: only cmd may load label files; internal code uses the pure internal/relationship/labels", pkgPath, labelsioPkg)
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
	found := false
	for pkgPath, pkg := range loaded {
		if !isModelPkg(pkgPath) {
			continue
		}
		found = true
		for imp := range pkg.Imports {
			if !isStdlib(imp) && !isModelPkg(imp) && !modelThirdPartyAllowed[pkgPath][imp] {
				t.Errorf("model package %s must not import non-stdlib %q", pkgPath, imp)
			}
		}
	}
	if !found {
		t.Fatal("no internal/model packages loaded")
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

func TestCouplingContractDoesNotImportGraph(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedImports, Dir: "."}
	pkgs, err := packages.Load(cfg, modulePrefix+"internal/relationship/coupling")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("packages.Load returned %d packages, want 1", len(pkgs))
	}
	const graphPkg = modulePrefix + "internal/model/graph"
	if _, imports := pkgs[0].Imports[graphPkg]; imports {
		t.Fatalf("internal/relationship/coupling must not import %s; keep clone evidence on coupling's narrow Location DTO", graphPkg)
	}
}

// adapterPackagePrefixes are the adapter rings whose import closure the
// hexagonal-boundary checks constrain.
var adapterPackagePrefixes = []string{
	modulePrefix + "internal/toolrun",
	modulePrefix + "internal/extract/",
	modulePrefix + "internal/history/",
	modulePrefix + "internal/output/",
}

// isAdapterPackage reports whether pkgPath belongs to the adapter ring.
func isAdapterPackage(pkgPath string) bool {
	for _, prefix := range adapterPackagePrefixes {
		if strings.HasPrefix(pkgPath, prefix) {
			return true
		}
	}
	return false
}

func assertAdaptersNoEngineImport(t *testing.T, loaded map[string]*packages.Package) {
	t.Helper()
	// Adapters (toolrun, extract/*, history/*, output/*) must depend on
	// internal/evidence/ports (or the internal/report/ports rendering port),
	// never on the orchestrator (internal/analysispipeline). This ensures the hexagonal
	// boundary: ports live in neutral owner packages that both adapters and the
	// orchestrator can import without creating a cycle.
	const enginePkg = modulePrefix + "internal/analysispipeline"
	for pkgPath, pkg := range loaded {
		if !isAdapterPackage(pkgPath) {
			continue
		}
		if _, imports := pkg.Imports[enginePkg]; imports {
			t.Errorf("adapter package %s must not import %s: use internal/evidence/ports instead", pkgPath, enginePkg)
		}
	}
}

func assertAdaptersNoAssessmentApplicationImport(t *testing.T, loaded map[string]*packages.Package) {
	t.Helper()
	// Adapters (toolrun, extract/*, history/*, output/*) must depend on stable
	// ports/contracts, never on the assessment or application core rings. This
	// keeps adapters swappable and prevents process-boundary code from dragging
	// evaluation/use-case logic in.
	for pkgPath, pkg := range loaded {
		if !isAdapterPackage(pkgPath) {
			continue
		}
		for imp := range pkg.Imports {
			if strings.HasPrefix(imp, modulePrefix+"internal/assessment/") ||
				strings.HasPrefix(imp, modulePrefix+"internal/application/") {
				t.Errorf("adapter package %s must not import domain core %s", pkgPath, imp)
			}
		}
	}
}

func TestNamedCommandStagesDoNotImportDomainInternals(t *testing.T) {
	named := []string{
		"analyze.go", "baseline.go", "config_compare.go", "explain.go",
		"enrich.go", "enrich_abstained.go", "enrich_values.go", "update.go",
	}
	for _, name := range named {
		path := filepath.Join("..", "cmd", "archfit", name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, "\\\"")
			for _, forbidden := range []string{
				modulePrefix + "internal/assessment/",
				modulePrefix + "internal/relationship/",
				modulePrefix + "internal/model/graph",
				modulePrefix + "internal/model/evidence",
				modulePrefix + "internal/labels/labelsio",
				modulePrefix + "internal/initcfg",
			} {
				if strings.HasPrefix(importPath, forbidden) {
					t.Errorf("cmd/archfit/%s must use application/pipeline adapters, not %s", name, importPath)
				}
			}
		}
	}
}

func TestAnalyzeCheckSourceFilesDoNotImportDomainInternals(t *testing.T) {
	for _, name := range []string{"analyze.go", "check.go"} {
		path := filepath.Join("..", "cmd", "archfit", name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			for _, forbidden := range []string{
				modulePrefix + "internal/assessment/",
				modulePrefix + "internal/relationship/",
				modulePrefix + "internal/model/graph",
				modulePrefix + "internal/model/evidence",
			} {
				if strings.HasPrefix(path, forbidden) {
					t.Errorf("cmd/archfit/%s must use Application and report adapters, not %s", name, path)
				}
			}
		}
	}
}

func assertReportAdaptersNoDomainImports(t *testing.T, loaded map[string]*packages.Package) {
	t.Helper()
	for pkgPath, pkg := range loaded {
		isRenderer := strings.HasPrefix(pkgPath, modulePrefix+"internal/output/") ||
			pkgPath == modulePrefix+"internal/report/ports"
		if !isRenderer {
			continue
		}
		for imp := range pkg.Imports {
			if !strings.HasPrefix(imp, modulePrefix+"internal/") {
				continue
			}
			if imp != modulePrefix+"internal/model/report" && imp != modulePrefix+"internal/report/ports" {
				t.Errorf("report adapter %s may import only report contracts, not %s", pkgPath, imp)
			}
		}
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
// without over-matching a same-prefix sibling (e.g. "internal/assessment/metricsx").
func TestInCoreRing(t *testing.T) {
	cases := map[string]bool{
		modulePrefix + "internal/assessment/metrics":                 true,
		modulePrefix + "internal/assessment/metrics/boundary":        true,
		modulePrefix + "internal/assessment/metrics/internal/result": true,
		modulePrefix + "internal/scope":                              true,
		modulePrefix + "internal/output/markdown":                    false,
		modulePrefix + "internal/assessment/metricsx":                false, // must not over-match the prefix
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
		modulePrefix + "internal/assessment/metrics/internal/result",
		modulePrefix + "internal/config",
	}
	for _, imp := range allowed {
		if isForbiddenForCore(imp) {
			t.Errorf("isForbiddenForCore(%q) = true, want false", imp)
		}
	}
}
