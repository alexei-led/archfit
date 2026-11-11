package change

import (
	"cmp"
	"maps"
	"path"
	"slices"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// Judgment is one tree judged against the policy.
type Judgment struct {
	// Errors are the errors of the tree, in record order: by the first
	// site's file and line, then by rule.
	Errors []Error
	// NotChecked are the note rules, in policy order: rules nothing checks
	// today, listed as "not checked".
	NotChecked []resolved.Rule
}

// Evaluate judges the check facts of one tree against p, which has its
// defaults. Every import is judged by Judge; the errors that share a key are
// one error with all their sites. The cycle rule reads the dependencies
// between modules of the whole tree, and the unowned rule reads facts.Sources.
// Files that p declares tests or generated are out of every rule. Evaluate
// reads neither facts.Unknown nor exceptions: the check decides those.
func Evaluate(p resolved.Policy, facts evidence.CheckFacts) Judgment {
	t := newTree(p, facts.Sources)
	errs := newCollector()
	graph := moduleGraph{}
	for _, imp := range facts.Imports {
		if t.excluded(imp.File) {
			continue
		}
		use := t.use(imp)
		site := Site{File: imp.File, Line: imp.Line}
		for _, err := range Judge(p, use) {
			errs.add(err, site)
		}
		graph.add(use, site)
	}
	if p.Builtin.Cycle != resolved.ModeOff {
		for _, err := range graph.cycles(p.Builtin.Cycle) {
			errs.add(err, err.Sites...)
		}
	}
	if p.Builtin.Unowned != resolved.ModeOff {
		for _, file := range t.files {
			if _, owned := p.ModuleOf(file); owned || t.excluded(file) {
				continue
			}
			errs.add(Error{
				Key:  Key{Rule: resolved.RuleUnowned, Symbol: file},
				What: "production code in no module", Why: whyUnowned, Next: nextOf(p.Builtin.Unowned),
			}, Site{File: file})
		}
	}
	return Judgment{Errors: errs.sorted(ruleOrder(p)), NotChecked: notes(p)}
}

// tree is what Evaluate knows about the files of a tree.
type tree struct {
	p     resolved.Policy
	files []string
	// isDir holds every directory that holds a source file. An import
	// target that is one is a package directory; any other target is a
	// file, even one that is not a source (a .vue or .json file).
	isDir map[string]bool
	// dirFiles are the source files directly in a directory: its production
	// files, or, when it has only declared tests and generated files (a
	// protobuf package), those. A generated file imports nothing the rules
	// judge, but a production import of its package is a dependency.
	dirFiles map[string][]string
	// dirModule is the module of a package directory, from its dirFiles: a
	// Go import names the directory, and the module's paths may be file
	// globs (pkg/foo/*.go) that do not match the directory itself.
	dirModule map[string]string
}

func newTree(p resolved.Policy, sources []string) tree {
	t := tree{
		p: p, files: slices.Sorted(slices.Values(sources)),
		isDir: map[string]bool{}, dirFiles: map[string][]string{}, dirModule: map[string]string{},
	}
	production, excluded := map[string][]string{}, map[string][]string{}
	for _, file := range t.files {
		for d := path.Dir(file); !t.isDir[d]; d = path.Dir(d) {
			t.isDir[d] = true
			if d == "." || d == "/" {
				break
			}
		}
		dir := path.Dir(file)
		if t.excluded(file) {
			excluded[dir] = append(excluded[dir], file)
		} else {
			production[dir] = append(production[dir], file)
		}
	}
	for dir, files := range excluded {
		if _, ok := production[dir]; !ok {
			production[dir] = files
		}
	}
	for dir, files := range production {
		t.dirFiles[dir] = files
		owners := map[string]bool{}
		for _, file := range files {
			if m, ok := p.ModuleOf(file); ok {
				owners[m] = true
			}
		}
		if len(owners) == 1 {
			for m := range owners {
				t.dirModule[dir] = m
			}
		}
	}
	return t
}

func (t tree) excluded(file string) bool {
	return t.p.IsTest(file) || t.p.IsGenerated(file)
}

// use is the use one import makes. A directory target takes the module of
// its files when they agree, else the module that owns the directory.
func (t tree) use(imp evidence.ImportFact) Use {
	u := Use{From: End{Path: imp.File, IsFile: true}, Lang: languageOf(imp.File)}
	if imp.External {
		u.To = End{Path: imp.Target, Library: true}
		return u
	}
	u.To = End{Path: imp.Target, IsFile: !t.isDir[imp.Target]}
	if !u.To.IsFile {
		u.To.Module = t.dirModule[imp.Target]
		u.To.Files = t.dirFiles[imp.Target]
	}
	// Resolve both modules once, for Judge and for the module graph.
	u.From.Module = moduleOf(t.p, u.From)
	u.To.Module = moduleOf(t.p, u.To)
	return u
}

// extLanguages maps a file extension to the language whose package sets an
// only rule reads.
var extLanguages = map[string]resolved.Language{
	".go": resolved.LanguageGo,
	".ts": resolved.LanguageTypeScript, ".tsx": resolved.LanguageTypeScript, ".mts": resolved.LanguageTypeScript,
	".cts": resolved.LanguageTypeScript, ".js": resolved.LanguageTypeScript, ".jsx": resolved.LanguageTypeScript,
	".mjs": resolved.LanguageTypeScript, ".cjs": resolved.LanguageTypeScript,
	".py": resolved.LanguagePython, ".pyi": resolved.LanguagePython,
	".rs": resolved.LanguageRust,
	".cs": resolved.LanguageDotNet,
}

// languageOf is the language of a file by its extension, or "" when unknown.
func languageOf(file string) resolved.Language {
	return extLanguages[path.Ext(file)]
}

// collector groups errors by key and gathers their sites.
type collector struct {
	byKey map[Key]*Error
}

func newCollector() collector { return collector{byKey: map[Key]*Error{}} }

func (c collector) add(err Error, sites ...Site) {
	if have, ok := c.byKey[err.Key]; ok {
		have.Sites = append(have.Sites, sites...)
		return
	}
	err.Sites = slices.Clone(sites)
	c.byKey[err.Key] = &err
}

// sorted returns the errors in record order, each with its sites sorted and
// without repeats.
func (c collector) sorted(order map[string]int) []Error {
	errs := make([]Error, 0, len(c.byKey))
	for _, err := range c.byKey {
		slices.SortFunc(err.Sites, compareSites)
		err.Sites = slices.Compact(err.Sites)
		errs = append(errs, *err)
	}
	slices.SortFunc(errs, func(a, b Error) int {
		return cmp.Or(
			compareSites(a.Sites[0], b.Sites[0]),
			cmp.Compare(order[a.Key.Rule], order[b.Key.Rule]),
			cmp.Compare(a.Key.Rule, b.Key.Rule),
			cmp.Compare(a.Key.From, b.Key.From),
			cmp.Compare(a.Key.To, b.Key.To),
			cmp.Compare(a.Key.Symbol, b.Key.Symbol),
		)
	})
	return errs
}

func compareSites(a, b Site) int {
	return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}

// ruleOrder ranks the rules: the built-in rules in design 6.2 order, then the
// custom rules in policy order.
func ruleOrder(p resolved.Policy) map[string]int {
	order := map[string]int{}
	for i, id := range resolved.BuiltinRules {
		order[id] = i
	}
	for i, r := range p.Rules {
		order[r.ID] = len(resolved.BuiltinRules) + i
	}
	return order
}

func notes(p resolved.Policy) []resolved.Rule {
	var out []resolved.Rule
	for _, r := range p.Rules {
		if r.Form == resolved.FormNote {
			out = append(out, r)
		}
	}
	return out
}

// moduleGraph holds the dependencies between modules of a tree, with the
// sites of the imports that make each one.
type moduleGraph map[string]map[string][]Site

func (g moduleGraph) add(u Use, site Site) {
	from, to := u.From.Module, u.To.Module
	if from == "" || to == "" || from == to || u.To.Library {
		return
	}
	if g[from] == nil {
		g[from] = map[string][]Site{}
	}
	g[from][to] = append(g[from][to], site)
}

// cycles returns one error for each dependency inside a set of modules that
// depend on each other (a strongly connected set of two or more), with the
// sites of its imports. The key is the dependency, not the set: a change that
// deletes a dependency of a cycle leaves a smaller set, and a set key would
// make that new; a dependency key does not.
func (g moduleGraph) cycles(mode resolved.Mode) []Error {
	reach := map[string]map[string]bool{}
	for m := range g {
		reach[m] = g.reachable(m)
	}
	seen := map[string]bool{}
	var errs []Error
	for _, m := range slices.Sorted(maps.Keys(g)) {
		if seen[m] {
			continue
		}
		set := []string{m}
		for other := range reach[m] {
			if other != m && reach[other][m] {
				set = append(set, other)
			}
		}
		if len(set) < 2 {
			continue
		}
		slices.Sort(set)
		members := joinNames(set)
		for _, from := range set {
			seen[from] = true
			for _, to := range set {
				sites := g[from][to]
				if len(sites) == 0 {
					continue
				}
				errs = append(errs, Error{
					Key: Key{Rule: resolved.RuleCycle, From: from, To: to}, From: from, To: to,
					What: from + " -> " + to + " is in a cycle: " + members + " depend on each other",
					Why:  whyCycle, Next: nextOf(mode), Sites: sites,
				})
			}
		}
	}
	return errs
}

// reachable lists the modules a module reaches through one or more
// dependencies.
func (g moduleGraph) reachable(start string) map[string]bool {
	seen := map[string]bool{}
	stack := []string{start}
	for len(stack) > 0 {
		m := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for next := range g[m] {
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return seen
}
