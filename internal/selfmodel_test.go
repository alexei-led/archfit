// Self-model purity gate: `.archfit.yaml` must describe the physical package
// tree it gates, and nothing else. These tests fail when the source moves and
// the model does not — a dead path glob, an unowned package, a rule aimed at a
// package that no longer exists, or a public entry outside its own module.
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

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/policy"
)

const selfConfigPath = "../.archfit.yaml"

// guardRules match nothing on purpose: each one forbids re-introducing a
// package this migration deleted. They are the single exception to the
// dead-rule check, and removing a package from this list without deleting the
// rule turns a live guard into silent dead config.
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

// repoDirs lists every repo-relative directory that holds at least one source
// file, plus the repo-relative path of every Go file. Module path globs and
// rule endpoints are matched against these, so "matches something" means
// "matches real source", not "is syntactically plausible".
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
			if skip[d.Name()] {
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
// a package by an equally specific glob. Most-specific-wins resolves genuine
// nesting; an exact tie means one module silently shadows the other.
func TestSelfModelHasNoAmbiguousPackageOwnership(t *testing.T) {
	cfg := loadSelfConfig(t)

	for _, dir := range goPackageDirs(t) {
		best := -1
		var winners []string
		for _, name := range sortedModuleNames(cfg.Modules) {
			for _, glob := range cfg.Modules[name].Paths {
				ok, err := doublestar.Match(glob, dir)
				if err != nil || !ok {
					continue
				}
				switch spec := globWeight(glob); {
				case spec > best:
					best, winners = spec, append(winners[:0], name)
				case spec == best && !slices.Contains(winners, name):
					winners = append(winners, name)
				}
			}
		}
		if len(winners) > 1 {
			t.Errorf("Go package %q is claimed at equal specificity by %v", dir, winners)
		}
	}
}

// globWeight mirrors the ordering policy.ModuleMap uses to pick the most
// specific match: more path segments and fewer wildcards win.
func globWeight(glob string) int {
	weight := strings.Count(glob, "/") * 10
	weight -= strings.Count(glob, "*") * 3
	return weight
}

// TestSelfModelHasNoDeadRules fails when a rule points at a path that no longer
// exists, unless the rule is a declared re-introduction guard.
func TestSelfModelHasNoDeadRules(t *testing.T) {
	cfg := loadSelfConfig(t)
	dirs, goFiles := repoDirs(t)
	candidates := append(append([]string{}, dirs...), goFiles...)

	seen := map[string]bool{}
	for _, rule := range cfg.Rules {
		if rule.ID == "" {
			t.Errorf("rule with type %q has no id", rule.Type)
			continue
		}
		if seen[rule.ID] {
			t.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = true

		if _, isGuard := guardRules[rule.ID]; isGuard {
			if matchesAny(rule.To, candidates) {
				t.Errorf("guard rule %q now matches real source %q — the package it forbids was re-introduced", rule.ID, rule.To)
			}
			continue
		}
		for label, glob := range map[string]string{"from": rule.From, "to": rule.To} {
			if glob == "" || glob == "**" {
				continue
			}
			if !matchesAny(glob, candidates) {
				t.Errorf("rule %q: %s glob %q matches no directory or Go file", rule.ID, label, glob)
			}
		}
	}

	for id, why := range guardRules {
		if !seen[id] {
			t.Errorf("guard rule %q is missing from .archfit.yaml (%s)", id, why)
		}
	}
}

// TestSelfModelPublicSurfacesAreRealAndOwned fails when a module publishes a
// path it does not own or that holds no Go source. A public entry is a coupling
// classification input: a stale one silences a real intrusive edge.
func TestSelfModelPublicSurfacesAreRealAndOwned(t *testing.T) {
	cfg := loadSelfConfig(t)
	goPkgs := goPackageDirs(t)

	for _, name := range sortedModuleNames(cfg.Modules) {
		def := cfg.Modules[name]
		for _, pub := range def.Public {
			if !matchesAny(pub, goPkgs) {
				t.Errorf("module %q: public entry %q names no Go package", name, pub)
				continue
			}
			owned := false
			for _, glob := range def.Paths {
				if ok, err := doublestar.Match(glob, pub); err == nil && ok {
					owned = true
					break
				}
			}
			if !owned {
				t.Errorf("module %q: public entry %q is outside the module's own paths %v", name, pub, def.Paths)
			}
		}
	}
}

// TestSelfModelLayersAreDeclaredAndUsed fails on a module in an undeclared
// layer and on a declared layer no module occupies. Both break
// forbidden_layer_direction: the first silently opts out of ranking, the second
// leaves a rung in the ladder that means nothing.
func TestSelfModelLayersAreDeclaredAndUsed(t *testing.T) {
	cfg := loadSelfConfig(t)

	used := map[string]bool{}
	for _, name := range sortedModuleNames(cfg.Modules) {
		layer := cfg.Modules[name].Layer
		if layer == "" {
			t.Errorf("module %q declares no layer", name)
			continue
		}
		if !slices.Contains(cfg.Layers, layer) {
			t.Errorf("module %q: layer %q is not declared in layers: %v", name, layer, cfg.Layers)
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
