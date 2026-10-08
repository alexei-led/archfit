// Self-model purity gate: `.archfit.yaml` must describe the physical package
// tree it gates, and nothing else. These tests fail when the source moves and
// the model does not — a dead path glob, an unowned package, a rule aimed at a
// package that no longer exists, or a public entry outside its own module.
//
// The rule, public-surface, layer, and ownership checks are the production
// `archfit config lint` predicates run over this repository, so the self-model
// holds the engine's config to exactly what lint and check enforce for users.
//
// Run with: go test ./internal/ -run TestSelfModel
package arch_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const selfConfigPath = "../.archfit.yaml"

// guardRules match nothing on purpose: each one forbids re-introducing a
// package this migration deleted, and declares `guard: true`, which exempts it
// from the dead-selector check. The test fails if a guard is deleted, loses
// `guard: true`, or starts matching real source; every guard needs an entry
// here saying what it guards.
var guardRules = map[string]string{
	"no_stage_view":       "internal/view was dissolved; the rule blocks a new shared stage-view package",
	"no_analysispipeline": "internal/analysispipeline was dissolved; the rule blocks a new orchestration hub",
}

func loadSelfConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(context.Background(), selfConfigPath)
	if err != nil {
		t.Fatalf("load self-config: %v", err)
	}
	return cfg
}

// lintRepo runs the production config lint for cfg over this repository: the
// source inventory `archfit config lint` reads and the predicates it applies.
func lintRepo(t *testing.T, cfg config.Config) []evaluation.PolicyDiagnostic {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	inventory, err := acquisition.Inventory{ConfigPath: selfConfigPath, Options: cfg.RunOptions(), Runner: toolrun.New()}.
		SourceInventory(context.Background(), root)
	if err != nil {
		t.Fatalf("read source inventory: %v", err)
	}
	if len(inventory.FileClassIndex) == 0 {
		t.Fatal("empty source inventory: every lint check would pass vacuously")
	}
	return evaluation.LintPolicy(cfg.PolicySnapshot(), inventory)
}

// lintFindings keeps the diagnostics carrying one of codes.
func lintFindings(diagnostics []evaluation.PolicyDiagnostic, codes ...string) []evaluation.PolicyDiagnostic {
	var out []evaluation.PolicyDiagnostic
	for _, d := range diagnostics {
		if slices.Contains(codes, d.Code) {
			out = append(out, d)
		}
	}
	return out
}

// repoDirs lists every repo-relative directory that holds at least one source
// file, plus the repo-relative path of every Go file. Module path globs and
// rule endpoints are matched against these, so "matches something" means
// "matches real source", not "is syntactically plausible".
// isToolStateDir reports a dot directory below the walk root. Those hold tool
// state (agent worktrees under .claude/, editor dirs), which the Go toolchain
// and archfit's LOC walk skip too; walking them double-counts every package.
func isToolStateDir(rel, name string) bool {
	return rel != "." && strings.HasPrefix(name, ".")
}

func repoDirs(t *testing.T) (dirs []string, goFiles []string) {
	t.Helper()
	root := ".."
	// Mirrors scope.DefaultExclusions: the model gates analysed source, and
	// archfit never walks fixtures or vendored trees, so neither does this test.
	skip := map[string]bool{
		dirGit: true, ".bin": true, dirFactCache: true, ".gitnexus": true,
		"node_modules": true, dirVendor: true, "target": true, ".venv": true,
		dirTestdata: true,
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if skip[d.Name()] || isToolStateDir(rel, d.Name()) {
				return filepath.SkipDir
			}
			if rel != "." {
				dirs = append(dirs, filepath.ToSlash(rel))
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), goSourceExt) {
			goFiles = append(goFiles, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	return dirs, goFiles
}

// goPackageDirs returns the repo-relative directory of every Go package in the
// module. These are the paths archfit's own classifier resolves edges to, so
// every one of them must have exactly one owning module.
func goPackageDirs(t *testing.T) []string {
	t.Helper()
	_, goFiles := repoDirs(t)
	seen := map[string]bool{}
	var dirs []string
	for _, f := range goFiles {
		dir := filepath.ToSlash(filepath.Dir(f))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	return dirs
}

func matchesAny(glob string, candidates []string) bool {
	for _, c := range candidates {
		if ok, err := doublestar.Match(glob, c); err == nil && ok {
			return true
		}
	}
	return false
}

// TestSelfModelHasNoDeadPathGlobs fails when a module declares a path that no
// longer exists. A dead glob makes the module map look broader than the code
// it actually owns.
func TestSelfModelHasNoDeadPathGlobs(t *testing.T) {
	cfg := loadSelfConfig(t)
	dirs, goFiles := repoDirs(t)
	candidates := append(append([]string{}, dirs...), goFiles...)

	for _, name := range sortedModuleNames(cfg.Modules) {
		for _, glob := range cfg.Modules[name].Paths {
			if !matchesAny(glob, candidates) {
				t.Errorf("module %q: path glob %q matches no directory or Go file", name, glob)
			}
		}
	}
}

// TestSelfModelCoversEveryGoPackage fails when a Go package has no owning
// module. An unowned package is invisible to every dependency rule.
func TestSelfModelCoversEveryGoPackage(t *testing.T) {
	cfg := loadSelfConfig(t)
	mm := policy.BuildModuleMap(cfg.Modules)

	for _, dir := range goPackageDirs(t) {
		if _, ok := mm.ModuleFor(dir); !ok {
			t.Errorf("Go package %q has no owning module in .archfit.yaml", dir)
		}
	}
}

// TestSelfModelHasNoAmbiguousPackageOwnership fails when two modules both claim
// source by an equally specific glob. Most-specific-wins resolves genuine
// nesting; an exact tie means one module silently shadows the other.
func TestSelfModelHasNoAmbiguousPackageOwnership(t *testing.T) {
	for _, d := range lintFindings(lintRepo(t, loadSelfConfig(t)), evaluation.LintAmbiguousOwnership) {
		t.Errorf("%s: %s", d.Path, d.Message)
	}
}

// TestSelfModelHasNoDeadRules fails when a rule selector matches no source,
// unless the rule is a declared re-introduction guard, and when a guard is
// missing, undeclared, or matches real source again.
func TestSelfModelHasNoDeadRules(t *testing.T) {
	cfg := loadSelfConfig(t)

	seen := map[string]bool{}
	graded := 0
	for _, rule := range cfg.Rules {
		if rule.ID == "" {
			t.Errorf("rule with type %q has no id", rule.Type)
			continue
		}
		if seen[rule.ID] {
			t.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = true

		_, registered := guardRules[rule.ID]
		switch {
		case registered && !rule.Guard:
			t.Errorf("guard rule %q must declare guard: true", rule.ID)
		case !registered && rule.Guard:
			t.Errorf("rule %q declares guard: true but guardRules does not say what it guards", rule.ID)
		case !rule.Guard && (rule.From != "" || rule.To != ""):
			graded++
		}
	}
	for id, why := range guardRules {
		if !seen[id] {
			t.Errorf("guard rule %q is missing from .archfit.yaml (%s)", id, why)
		}
	}
	for _, d := range lintFindings(lintRepo(t, cfg), evaluation.LintDeadSelector, evaluation.LintGuardMatchesSource) {
		t.Errorf("%s: %s", d.Path, d.Message)
	}

	// Non-vacuity guard. With an empty or all-guard rule set the lint grades
	// nothing and still passes, which reads as "every boundary is live" for a
	// config that declares no boundary at all.
	if graded == 0 {
		t.Error("no rule was graded for dead selectors — the dead-rule gate checked nothing")
	}
}

// TestSelfModelPublicSurfacesAreRealAndOwned fails when a module publishes a
// path it does not own or that names no package. A public entry is a coupling
// classification input: a stale one silences a real intrusive edge.
func TestSelfModelPublicSurfacesAreRealAndOwned(t *testing.T) {
	cfg := loadSelfConfig(t)
	if !slices.ContainsFunc(sortedModuleNames(cfg.Modules), func(name string) bool { return len(cfg.Modules[name].Public) > 0 }) {
		t.Fatal("no module declares a public surface: the check would pass vacuously")
	}
	for _, d := range lintFindings(lintRepo(t, cfg), evaluation.LintPublicOutsideModule, evaluation.LintPublicMatchesNothing) {
		t.Errorf("%s: %s", d.Path, d.Message)
	}
}

// TestSelfModelLayersAreDeclaredAndUsed fails on a module in an undeclared
// layer and on a declared layer no module occupies. Both break
// forbidden_layer_direction: the first silently opts out of ranking, the second
// leaves a rung in the ladder that means nothing.
func TestSelfModelLayersAreDeclaredAndUsed(t *testing.T) {
	cfg := loadSelfConfig(t)
	for _, d := range lintFindings(lintRepo(t, cfg), evaluation.LintUndeclaredLayer) {
		t.Errorf("%s: %s", d.Path, d.Message)
	}

	used := map[string]bool{}
	for _, name := range sortedModuleNames(cfg.Modules) {
		layer := cfg.Modules[name].Layer
		if layer == "" {
			t.Errorf("module %q declares no layer", name)
			continue
		}
		used[layer] = true
	}
	for _, layer := range cfg.Layers {
		if !used[layer] {
			t.Errorf("layer %q is declared but no module occupies it", layer)
		}
	}
}

// TestSelfModelDeclaresNoDissolvedPackage fails when the model names a package
// this migration deleted. The model is the last place a dissolved boundary can
// survive as documentation of an architecture that no longer exists.
func TestSelfModelDeclaresNoDissolvedPackage(t *testing.T) {
	cfg := loadSelfConfig(t)

	for _, dissolved := range []string{"internal/engine", "internal/analysispipeline", "internal/view"} {
		for _, name := range sortedModuleNames(cfg.Modules) {
			def := cfg.Modules[name]
			for _, glob := range append(append([]string{}, def.Paths...), def.Public...) {
				if strings.HasPrefix(glob, dissolved) {
					t.Errorf("module %q still declares dissolved package %q", name, glob)
				}
			}
		}
	}
	if slices.Contains(cfg.Layers, "engine") {
		t.Error(`layers: still declares the dissolved "engine" layer`)
	}
}

func sortedModuleNames(modules map[string]policy.ModuleDef) []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
