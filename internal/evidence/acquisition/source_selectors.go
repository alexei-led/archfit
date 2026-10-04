package acquisition

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/syntax"
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
	return ruleScopeObservations(ctx, i.Runner, nil, resolved.Root, facts, i.Options), nil
}

// ruleScopeObservations projects walked source into the fields rule scope
// reads: the files, the node selector each file projects to, the files the
// configuration declared out of scope, the first-party Go module paths, and
// the Go standard-library packages. Acquire and SourceInventory both build them
// here, so check and config lint judge a rule selector over the same inventory
// build; only the facts an analyzer adds (Rust crate roots) and the effective
// config of check --lang, which turns a switched-off language on, separate them.
func ruleScopeObservations(ctx context.Context, runner toolrun.Runner, store *factcache.Store, root string, f evidencecontract.Facts, opts RunOptions) evaluation.Observations {
	goModules := registry.GoModulePaths(root, opts.Acquisition.GoExtract)
	outOfScope := declaredOutOfScope(f, opts.Exclusions, opts.Coverage)
	return evaluation.Observations{
		FileLOC: f.FileLOC, FileClassIndex: f.FileClassIndex,
		SourceSelectors: sourceSelectorsOf(f),
		OutOfScopeFiles: outOfScope,
		UnanalysedFiles: unanalysedFiles(root, f, outOfScope, opts.Coverage),
		UnwalkedSourceProduction: unwalkedSourceProduction(
			f, opts.Exclusions, opts.Coverage, opts.Acquisition.FileClass),
		RustModuleNodes:       rustModuleNodes(f),
		RustCrates:            rustCrates(f),
		RustModuleGraphCrates: f.RustModuleGraphCrates,
		GoModulePaths:         goModules,
		GoStdlibPackages:      goStdlibPackages(ctx, runner, store, root, goModules),
	}
}

// unanalysedFiles names the in-scope walked source files no dependency
// producer analyses. Applicability is decided by the extractor, never by a
// marker list: a file's language is absent from the tree exactly when
// primaryAbsentFromTree says so, the predicate that leaves its primary
// coverage row gapless absent. Without this, 248 TypeScript files under a
// web/ui with no root package.json held every module-wide rule unevaluated
// waiting for a dependency-cruiser run that could never happen. A Rust file
// outside every cargo workspace member (a fuzz/ crate the workspace excludes)
// has no crate node, so once cargo metadata named the members it is
// unanalysed too; before that, Rust crate identities are simply unknown.
//
// Files already declared out of scope are skipped: they are out of every
// rule's scope, not merely the dependency rules'.
func unanalysedFiles(root string, f evidencecontract.Facts, outOfScope map[string]struct{}, cov CoverageOptions) map[string]struct{} {
	var roots []graph.CrateRoot
	if f.Graph != nil {
		roots = f.Graph.CrateRoots()
	}
	absent := make(map[string]bool)
	for _, lang := range registry.All() {
		absent[lang.ID] = primaryAbsentFromTree(cov, lang.PrimaryTool, root)
	}
	var out map[string]struct{}
	consider := func(file string) {
		if _, declared := outOfScope[file]; declared {
			return
		}
		language, selector, supported := (policy.ModuleMap{}).RuleSelectorForFile(file, roots...)
		if !supported {
			return
		}
		nonMember := language == graph.LangRust && len(roots) > 0 && selector == ""
		if !absent[language] && !nonMember {
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

// unwalkedSourceProduction reports, for every dependency-edge source file the
// LOC walk never visited, whether it is production code. The walk skips dot
// directories and names such as target/ and mocks/ that an analyzer can still
// load, so those files have no FileClass of their own. Each is classified
// path-only with the configured file_class globs (syntax.ClassifyFile, the
// fallback syntax.LookupFileClass documents), and is never production when the
// configuration declared it out of scope. module_cycle reads it, so a cycle
// through code/.storybook/ obeys file_class.test_globs and exclude: like any
// walked file, and a Go package under internal/target/ is not silently dropped.
func unwalkedSourceProduction(f evidencecontract.Facts, exclusions []string, cov CoverageOptions, cfg syntax.FileClassConfig) map[string]bool {
	if f.Graph == nil {
		return nil
	}
	disabled := disabledLanguages(cov)
	var out map[string]bool
	consider := func(file string) {
		if _, walked := f.FileClassIndex[file]; walked {
			return
		}
		if _, walked := f.FileLOC[file]; walked {
			return
		}
		language, _, supported := (policy.ModuleMap{}).RuleSelectorForFile(file)
		if !supported {
			return
		}
		if out == nil {
			out = make(map[string]bool)
		}
		out[file] = !outOfDeclaredScope(file, exclusions, disabled) &&
			fileclass.IsProduction(syntax.ClassifyFile(language, file, nil, cfg))
	}
	for _, e := range f.Graph.Edges() {
		if path, ok := strings.CutPrefix(e.From, string(graph.NodeKindFile)+":"); ok {
			consider(path)
		}
		for _, l := range e.Locations {
			consider(l.File)
		}
	}
	return out
}

// rustModuleNodes lists the crate::mod node IDs of the Rust module graph, in
// the graph's sorted node order. Crate nodes carry no "::" and stay out.
func rustModuleNodes(f evidencecontract.Facts) []string {
	if f.Graph == nil {
		return nil
	}
	var out []string
	for _, n := range f.Graph.Nodes() {
		if n.Language == graph.LangRust && n.Kind == graph.NodeKindPackage && strings.Contains(n.Path, "::") {
			out = append(out, n.Path)
		}
	}
	return out
}

// rustCrates lists both spellings of every loaded Rust crate: the crate
// identifier crate::mod node IDs start with (a binary target's own name, such
// as yazi for package yazi-fm) and the library spelling of the package name.
// Nil without cargo metadata.
func rustCrates(f evidencecontract.Facts) []string {
	if f.Graph == nil {
		return nil
	}
	var out []string
	for _, root := range f.Graph.CrateRoots() {
		out = append(out, root.Crate, strings.ReplaceAll(root.Name, "-", "_"))
	}
	return out
}

// goStdlibPackages lists the importable standard-library packages of the Go
// toolchain that analyses root (`go list std`, run in root so a go.mod
// toolchain line selects the same toolchain go/packages uses). Packages under an
// internal or vendor segment are dropped: no first-party import can reach them,
// and keeping internal/... would excuse every first-party internal/ typo.
//
// It probes only when root holds a Go module: a standard-library ban means
// nothing elsewhere. A failed probe returns nil, and a to: selector colliding
// with a first-party directory is then judged as before (first-party).
//
// The list is a fact of the toolchain, not of the tree, and `go list std` costs
// seconds, so it is read through the fact cache keyed on the toolchain identity
// (goStdRunner). A nil store runs it every time.
func goStdlibPackages(ctx context.Context, runner toolrun.Runner, store *factcache.Store, root string, goModules []string) []string {
	if len(goModules) == 0 {
		return nil
	}
	cmd := toolrun.ToolCmd{Name: "go", Args: []string{"list", "std"}, WorkDir: root, Timeout: goToolTimeout}
	out, err := goStdRunner(ctx, runner, store, root).Run(ctx, cmd)
	if err != nil || out.ExitCode != 0 {
		return nil
	}
	var pkgs []string
	for _, line := range strings.Split(string(out.Stdout), "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" || strings.HasPrefix(pkg, "vendor/") || slices.Contains(strings.Split(pkg, "/"), "internal") {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}

// goStdAnalyzer is the fact-cache subdirectory of the standard-library list.
const goStdAnalyzer = "go-std"

const goToolTimeout = 30 * time.Second

// goStdIdentityVars are the `go env` values that decide what `go list std`
// prints: the toolchain (version and installation) and the build context that
// selects packages by constraint.
var goStdIdentityVars = []string{"GOVERSION", "GOROOT", "GOOS", "GOARCH", "GOFLAGS", "GOEXPERIMENT", "CGO_ENABLED"}

// goStdRunner wraps runner in the fact cache under a key made from the
// toolchain identity `go env` reports in root — the same directory `go list
// std` runs in, so a go.mod toolchain line picks the same toolchain for both.
// The identity is key material and is probed every run; only the list is
// cached, and only a successful, non-empty one. Without a store or an identity
// the plain runner is returned and the list runs uncached.
func goStdRunner(ctx context.Context, runner toolrun.Runner, store *factcache.Store, root string) toolrun.Runner {
	if store == nil {
		return runner
	}
	args := append([]string{"env", "-json"}, goStdIdentityVars...)
	identity, err := runner.Run(ctx, toolrun.ToolCmd{Name: "go", Args: args, WorkDir: root, Timeout: goToolTimeout})
	if err != nil || identity.ExitCode != 0 || len(bytes.TrimSpace(identity.Stdout)) == 0 {
		return runner
	}
	return &factcache.Runner{
		Inner: runner, Store: store, Analyzer: goStdAnalyzer,
		Key:       factcache.Key(goStdAnalyzer, string(identity.Stdout), "", ""),
		Cacheable: func(out toolrun.Output) bool { return out.ExitCode == 0 && len(bytes.TrimSpace(out.Stdout)) > 0 },
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
	disabled := disabledLanguages(cov)
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

// disabledLanguages are the languages switched off with no explicit gate, the
// predicate behind the disabled coverage row (primaryDisabledByConfig).
func disabledLanguages(cov CoverageOptions) map[string]struct{} {
	disabled := make(map[string]struct{})
	for _, lang := range registry.All() {
		if primaryDisabledByConfig(cov, lang.PrimaryTool) {
			disabled[lang.ID] = struct{}{}
		}
	}
	return disabled
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
