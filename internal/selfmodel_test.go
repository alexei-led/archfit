// The self-model checks that the schema 3 policy describes this source tree.
// They check ownership, live selectors, public surfaces, and layers.
// Run with: go test ./internal/ -run TestSelfModel
package arch_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const selfConfigPath = "../.archfit.yaml"

// These notes record packages the earlier migration removed. Schema 3 does
// not enforce a rule with an endpoint that does not exist.
var guardRules = map[string]string{
	"no_stage_view":       "internal/view was dissolved; the rule blocks a new shared stage-view package",
	"no_analysispipeline": "internal/analysispipeline was dissolved; the rule blocks a new orchestration hub",
}

func selfPolicy3(t *testing.T) resolved.Policy {
	t.Helper()
	data, err := os.ReadFile(selfConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		t.Fatalf("resolve self-policy: %v", err)
	}
	return p
}

func selfTreeFiles(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("list repository files: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
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
	p := selfPolicy3(t)
	dirs, files := repoDirs(t)
	for name, module := range p.Modules {
		for _, glob := range module.Paths {
			if !matchesAny(glob, append(dirs, files...)) {
				t.Errorf("module %s: dead path %s", name, glob)
			}
		}
	}
}

func TestSelfModelCoversEveryGoPackage(t *testing.T) {
	p := selfPolicy3(t)
	_, files := repoDirs(t)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		if _, ok := p.ModuleOf(file); !ok {
			t.Errorf("unowned Go file: %s", file)
		}
	}
}

func TestSelfModelHasNoAmbiguousPackageOwnership(t *testing.T) {
	p := selfPolicy3(t)
	if err := p.CheckTree(selfTreeFiles(t)); err != nil {
		t.Error(err)
	}
}

func TestSelfModelHasNoDeadRules(t *testing.T) {
	p := selfPolicy3(t)
	if err := p.CheckTree(selfTreeFiles(t)); err != nil {
		t.Error(err)
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		seen[r.ID] = true
	}
	for id := range guardRules {
		if !seen[id] {
			t.Errorf("missing guard note %s", id)
		}
	}
}

func TestSelfModelPublicSurfacesAreRealAndOwned(t *testing.T) {
	p := selfPolicy3(t)
	files := selfTreeFiles(t)
	if err := p.CheckTree(files); err != nil {
		t.Error(err)
	}
	count := 0
	for name, mod := range p.Modules {
		for _, glob := range mod.Public {
			count++
			for _, file := range files {
				matched, _ := doublestar.Match(glob, file)
				if matched {
					if owner, ok := p.ModuleOf(file); !ok || owner != name {
						t.Errorf("%s publishes %s owned by %s", name, file, owner)
					}
				}
			}
		}
	}
	if count == 0 {
		t.Error("no public surfaces declared")
	}
}

func TestSelfModelLayersAreDeclaredAndUsed(t *testing.T) {
	p := selfPolicy3(t)
	used := map[string]bool{}
	for name, mod := range p.Modules {
		if !slices.Contains(p.Layers, mod.Layer) {
			t.Errorf("%s has undeclared layer %s", name, mod.Layer)
		}
		used[mod.Layer] = true
	}
	for _, layer := range p.Layers {
		if !used[layer] {
			t.Errorf("unused layer %s", layer)
		}
	}
}

func TestSelfModelDeclaresNoDissolvedPackage(t *testing.T) {
	p := selfPolicy3(t)
	for _, dissolved := range []string{"internal/engine", "internal/analysispipeline", "internal/view"} {
		for name, mod := range p.Modules {
			for _, glob := range append(slices.Clone(mod.Paths), mod.Public...) {
				if strings.HasPrefix(glob, dissolved) {
					t.Errorf("%s declares dissolved %s", name, glob)
				}
			}
		}
	}
	if slices.Contains(p.Layers, "engine") {
		t.Error("engine layer still declared")
	}
}
