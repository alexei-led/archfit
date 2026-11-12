package policy3

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/application/repostate"
	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// header opens the written policy.
const header = "# The Archfit policy, written by archfit init from the code as it is today.\n" +
	"# Review the modules, public, depends_on and exceptions, then commit it.\n"

// whyException is the reason of an exception for an error that existed when
// init wrote the policy. It has no date: the owner decides when it ends.
const whyException = "The error existed when archfit init wrote this policy."

// packageSets are the sets init writes an only rule for, in this order.
var packageSets = []string{resolved.SetNetwork, resolved.SetProcess, resolved.SetDatabase}

// probePrefix starts the ids of the only rules that find the users of a
// package set. They are never written.
const probePrefix = "init-probe-"

// build returns the policy of the code the head facts describe, and its bytes.
// The bytes load (policyfile.Resolve) and fit the tree (CheckTree).
func build(root, owner string, facts evidence.CheckFacts) (policyfile.File, []byte, error) {
	testGlobs := tests(factFiles(facts))
	genGlobs, err := generated(root, facts.Sources)
	if err != nil {
		return policyfile.File{}, nil, err
	}
	files := slices.DeleteFunc(slices.Clone(facts.Sources), func(f string) bool { return matchesGlob(testGlobs, f) })
	mods := discover(files)
	if len(mods) == 0 {
		return policyfile.File{}, nil, errors.New("no source file outside the tests: archfit init writes the modules from the code")
	}
	f := policyfile.File{
		Version: policyfile.Version, Owners: []string{owner},
		Modules: make(map[string]policyfile.Module, len(mods)), Tests: testGlobs, Generated: genGlobs,
	}
	for _, m := range mods {
		f.Modules[m.name] = policyfile.Module{Paths: m.paths}
	}
	today := probe(f, facts)
	for _, m := range mods {
		mod := f.Modules[m.name]
		mod.Public = publicOf(m, today.dirs[m.name], otherDirs(mods, m))
		mod.DependsOn = today.deps[m.name]
		f.Modules[m.name] = mod
	}
	f.Rules = containment(today.users, len(mods))
	_, p, err := encode(f, factFiles(facts), header)
	if err != nil {
		return policyfile.File{}, nil, err
	}
	f.Exceptions = exceptions(p, facts)
	data, _, err := encode(f, factFiles(facts), header)
	if err != nil {
		return policyfile.File{}, nil, err
	}
	return f, data, nil
}

// uses are the uses between modules a tree holds today.
type uses struct {
	// deps are the modules each module depends on, sorted.
	deps map[string][]string
	// dirs are the packages of each module other modules use: a Go package
	// directory, or the directory of a TypeScript or Python file.
	dirs map[string][]string
	// users are the modules that use each package set.
	users map[string][]string
}

// probe judges the tree against the modules alone, with an only rule that no
// module may use each package set: every graph error is a dependency that
// exists, and every probe error a user of its set. The rules of the check
// decide, so the policy init writes and the check agree.
func probe(f policyfile.File, facts evidence.CheckFacts) uses {
	p := resolved.Policy{Modules: map[string]resolved.Module{}, Tests: f.Tests, Generated: f.Generated}
	for name, m := range f.Modules {
		p.Modules[name] = resolved.Module{Paths: m.Paths}
	}
	for _, set := range packageSets {
		p.Rules = append(p.Rules, resolved.Rule{ID: probePrefix + set, Form: resolved.FormOnly, Use: set})
	}
	dirs := directories(facts.Sources)
	add := func(m map[string]map[string]bool, key, value string) {
		if m[key] == nil {
			m[key] = map[string]bool{}
		}
		m[key][value] = true
	}
	deps, used, users := map[string]map[string]bool{}, map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, e := range change.Evaluate(p.WithDefaults(), facts).Errors {
		switch k := e.Key; {
		case k.Rule == resolved.RuleGraph:
			add(deps, k.From, k.To)
			add(used, k.To, packageOf(k.Symbol, dirs))
		case strings.HasPrefix(k.Rule, probePrefix) && k.From != "":
			add(users, strings.TrimPrefix(k.Rule, probePrefix), k.From)
		}
	}
	return uses{deps: sortedValues(deps), dirs: sortedValues(used), users: sortedValues(users)}
}

// packageOf is the package of an import target: the target when it is a
// directory, else the directory of the file. A file that is not a source, such
// as an image in a directory with no source, takes the nearest directory that
// holds a source, because a public glob must match the tree the check reads.
func packageOf(target string, dirs map[string]bool) string {
	if dirs[target] {
		return target
	}
	d := path.Dir(target)
	for !dirs[d] && d != rootKey && d != "/" {
		d = path.Dir(d)
	}
	return d
}

// directories lists every directory that holds a file, the root included.
func directories(files []string) map[string]bool {
	dirs := map[string]bool{}
	for _, f := range files {
		for d := path.Dir(f); !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
			if d == rootKey || d == "/" {
				break
			}
		}
	}
	return dirs
}

// otherDirs are the directories that hold a file of a module other than m.
func otherDirs(mods []module, m module) []string {
	var dirs []string
	for _, o := range mods {
		if o.key == m.key {
			continue
		}
		for _, f := range o.files {
			dirs = append(dirs, path.Dir(f))
		}
	}
	return dirs
}

// publicOf returns the public part of a module: the packages other modules
// use today. Where every package under a directory is used, and no other
// module has code there, the directory is one glob dir/** (D8). A module no
// other module uses gets its key directory; the root module is public as a
// whole, because its files are not one directory.
func publicOf(m module, used, others []string) []string {
	if m.key == rootKey {
		return m.paths
	}
	if len(used) == 0 {
		return []string{literal(m.key)}
	}
	isUsed := map[string]bool{}
	packages := slices.Clone(used)
	for _, u := range used {
		isUsed[u] = true
	}
	for _, f := range m.files {
		if d := path.Dir(f); !slices.Contains(packages, d) {
			packages = append(packages, d)
		}
	}
	collapsible := func(a string) bool {
		below := false
		for _, p := range packages {
			if under(p, a) {
				if !isUsed[p] {
					return false
				}
				below = below || p != a
			}
		}
		return below && !slices.ContainsFunc(others, func(d string) bool { return under(d, a) })
	}
	var out []string
	covered := map[string]bool{}
	for _, u := range used {
		if covered[u] {
			continue
		}
		glob := literal(u)
		for _, a := range ancestors(m.key, u) {
			if collapsible(a) {
				glob = literal(a) + "/**"
				for _, p := range packages {
					covered[p] = covered[p] || under(p, a)
				}
				break
			}
		}
		covered[u] = true
		out = append(out, glob)
	}
	return out
}

// ancestors lists the directories from key down to dir, both included; dir
// alone when key does not hold it.
func ancestors(key, dir string) []string {
	rel, ok := strings.CutPrefix(dir, key+"/")
	if !ok {
		return []string{dir}
	}
	out := []string{key}
	for _, seg := range strings.Split(rel, "/") {
		out = append(out, out[len(out)-1]+"/"+seg)
	}
	return out
}

// under reports whether p is dir or lies below it.
func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// containment returns an only rule for each package set that some modules use
// and some do not: the containment of today, kept.
func containment(users map[string][]string, modules int) []policyfile.Rule {
	var rules []policyfile.Rule
	for _, set := range packageSets {
		in := users[set]
		if len(in) == 0 || len(in) == modules {
			continue
		}
		rules = append(rules, policyfile.Rule{
			ID:   "contain-" + set,
			Only: &policyfile.Only{Use: set, In: policyfile.Names(in)},
			Why:  capitalize(set) + " code stays in the modules that used it when archfit init wrote this policy.",
		})
	}
	return rules
}

// exceptions returns one exception, with no date, for each rule and place
// where the policy p reports errors on the tree (repostate's Existing): an
// error that exists already does not fail the first check. The date only
// judges exceptions, and p has none yet.
func exceptions(p resolved.Policy, facts evidence.CheckFacts) []policyfile.Exception {
	existing := repostate.Evaluate(repostate.Input{Policy: p, Head: facts, Today: time.Now()}).Existing
	out := make([]policyfile.Exception, 0, len(existing))
	for _, e := range existing {
		out = append(out, policyfile.Exception{Rule: e.Rule, From: e.From, To: e.To, Why: whyException})
	}
	return out
}

// encode writes the policy file under its header comment and checks that it
// loads and fits the tree.
func encode(f policyfile.File, sources []string, header string) ([]byte, resolved.Policy, error) {
	body, err := yaml.MarshalWithOptions(f, yaml.Indent(2), yaml.IndentSequence(true))
	if err != nil {
		return nil, resolved.Policy{}, err
	}
	data := append([]byte(header), body...)
	p, err := policyfile.Resolve(data)
	if err != nil {
		return nil, resolved.Policy{}, fmt.Errorf("the policy archfit init wrote does not load:\n%w", err)
	}
	if err := p.CheckTree(sources); err != nil {
		return nil, resolved.Policy{}, fmt.Errorf("the policy archfit init wrote does not fit the code:\n%w", err)
	}
	return data, p, nil
}

func sortedValues(m map[string]map[string]bool) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = slices.Sorted(maps.Keys(v))
	}
	return out
}

func matchesGlob(globs []string, file string) bool {
	return slices.ContainsFunc(globs, func(g string) bool { return match(g, file) })
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
