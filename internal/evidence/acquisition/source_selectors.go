package acquisition

import (
	"context"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Inventory reads the rule-scope source inventory `archfit config lint` judges
// rule selectors and module declarations against. It resolves the analysis
// boundary the way Acquire does (an empty root means the git root around the
// config) and runs the same LOC walk and projections, but no analyzer, so Rust
// crate identities, which need cargo metadata, stay unknown.
type Inventory struct {
	ConfigPath string
	Options    RunOptions
	Runner     toolrun.Runner
}

var _ application.SourceInventoryReader = Inventory{}

// SourceInventory implements application.SourceInventoryReader.
func (i Inventory) SourceInventory(ctx context.Context, root string) (evaluation.Observations, error) {
	sc := i.Options.Scope
	sc.WorkDir, sc.Root, sc.Full = scanDir(root, filepath.Dir(i.ConfigPath)), root, true
	resolved, err := scope.Resolve(ctx, sc, gitResolver{workDir: sc.WorkDir, runner: i.Runner})
	if err != nil {
		return evaluation.Observations{}, err
	}
	fileLOC, classes, _, err := loc.RunWithConfig(resolved.Root, i.Options.Acquisition.FileClass)
	if err != nil {
		return evaluation.Observations{}, err
	}
	facts := evidencecontract.Facts{FileLOC: fileLOC, FileClassIndex: classes}
	return ruleScopeObservations(resolved.Root, facts, i.Options), nil
}

// ruleScopeObservations projects walked source into the fields rule scope
// reads: the files, the node selector each file projects to, the files the
// configuration declared out of scope, and the first-party Go module paths.
// Acquire and SourceInventory both build them here, so check and config lint
// cannot disagree about which source a rule selector reaches.
func ruleScopeObservations(root string, f evidencecontract.Facts, opts RunOptions) evaluation.Observations {
	return evaluation.Observations{
		FileLOC: f.FileLOC, FileClassIndex: f.FileClassIndex,
		SourceSelectors: sourceSelectorsOf(f),
		OutOfScopeFiles: declaredOutOfScope(f, opts.Exclusions, opts.Coverage),
		GoModulePaths:   registry.GoModulePaths(root, opts.Acquisition.GoExtract),
	}
}

func sourceSelectorsOf(f evidencecontract.Facts) map[string]string {
	var roots []graph.CrateRoot
	if f.Graph != nil {
		roots = f.Graph.CrateRoots()
	}
	selectors := make(map[string]string, len(f.FileLOC)+len(f.FileClassIndex))
	mm := policy.ModuleMap{}
	add := func(file string) {
		language, selector, supported := mm.RuleSelectorForFile(file, roots...)
		if supported {
			if language == graph.LangRust && len(roots) == 0 {
				selector = ""
			}
			selectors[file] = selector
		}
	}
	for file := range f.FileLOC {
		add(file)
	}
	for file := range f.FileClassIndex {
		add(file)
	}
	return selectors
}

// declaredOutOfScope names the walked source files the configuration declared
// outside the analysis scope: a file an effective `exclude:` glob matches, or a
// file of a language switched off by `languages.<id>.enabled: false`.
//
// Rule scope skips these files. Without that, one stray tooling script (a
// `.cjs` config wrapper, a Python helper run through uv) put its language in
// scope for every rule over its directory, and the rule stayed unevaluated
// waiting for a producer the configuration had deliberately switched off. This
// is declared scope, never analyzer availability: an absent producer for a
// language that is still in scope keeps the rule unevaluated.
//
// exclusions must be the set RunOptions merged once: scope.MergeExclusions is
// not idempotent, so it is never applied again here. A switched-off language is
// read through primaryDisabledByConfig, the predicate behind the disabled
// coverage row, so a language switched off with an explicit gate stays in scope
// and its rules stay honest about the producer that did not run.
func declaredOutOfScope(f evidencecontract.Facts, exclusions []string, cov CoverageOptions) map[string]struct{} {
	disabled := make(map[string]struct{})
	for _, lang := range registry.All() {
		if primaryDisabledByConfig(cov, lang.PrimaryTool) {
			disabled[lang.ID] = struct{}{}
		}
	}
	var out map[string]struct{}
	consider := func(file string) {
		if !outOfDeclaredScope(file, exclusions, disabled) {
			return
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[file] = struct{}{}
	}
	for file := range f.FileLOC {
		consider(file)
	}
	for file := range f.FileClassIndex {
		consider(file)
	}
	return out
}

// outOfDeclaredScope reports whether one repo-relative source file is outside
// the declared analysis scope. Exclusion globs match the slash path the same
// way the Go extractor's own exclusion check does.
func outOfDeclaredScope(file string, exclusions []string, disabled map[string]struct{}) bool {
	if language, _, ok := (policy.ModuleMap{}).RuleSelectorForFile(file); ok {
		if _, off := disabled[language]; off {
			return true
		}
	}
	for _, glob := range exclusions {
		if matched, _ := doublestar.Match(glob, file); matched {
			return true
		}
	}
	return false
}
