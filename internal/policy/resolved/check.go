package resolved

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// maxSuggestDistance is the largest edit distance at which a misspelled name
// gets a suggestion.
const maxSuggestDistance = 2

// messages collects "key: text" lines in policy order.
type messages []string

func (m *messages) add(key, format string, args ...any) {
	*m = append(*m, key+": "+fmt.Sprintf(format, args...))
}

func (m messages) err() error {
	if len(m) == 0 {
		return nil
	}
	return errors.New(strings.Join(m, "\n"))
}

// Check reports what refers to nothing: a module name that is not a module, a
// layer that is not in layers, an exception for a rule that does not exist, two
// rules with one id, and a rule with the id of a built-in rule. The error lists
// every message, one per line, as "key: text". The decoder has already checked
// each key on its own.
func (p Policy) Check() error {
	var m messages
	modules := slices.Sorted(maps.Keys(p.Modules))
	for _, name := range modules {
		mod := p.Modules[name]
		key := "modules." + name
		for _, dep := range mod.DependsOn {
			p.checkModule(&m, key+".depends_on", dep, modules)
		}
		if mod.Layer != "" && !slices.Contains(p.Layers, mod.Layer) {
			if len(p.Layers) == 0 {
				m.add(key+".layer", "%q is not in layers: add layers, inner first", mod.Layer)
			} else {
				m.add(key+".layer", "%q is not in layers (%s)", mod.Layer, strings.Join(p.Layers, ", "))
			}
		}
	}
	firstID := map[string]int{}
	for i, r := range p.Rules {
		key := ruleKey(i, r)
		switch j, dup := firstID[r.ID]; {
		case slices.Contains(BuiltinRules, r.ID):
			m.add(key+".id", "%q is a built-in rule: pick another id", r.ID)
		case dup:
			m.add(key+".id", "%q is also the id of rules[%d]: give each rule its own id", r.ID, j)
		default:
			firstID[r.ID] = i
		}
		for _, name := range r.From {
			p.checkName(&m, key+".deny.from", name, modules)
		}
		for _, name := range r.To {
			p.checkName(&m, key+".deny.to", name, modules)
		}
		for _, name := range r.In {
			p.checkName(&m, key+".only.in", name, modules)
		}
	}
	rules := slices.Concat(BuiltinRules, slices.Collect(maps.Keys(firstID)))
	for i, e := range p.Exceptions {
		key := fmt.Sprintf("exceptions[%d]", i)
		if !slices.Contains(rules, e.Rule) {
			m.add(key+".rule", "%q is not a rule%s", e.Rule, suggestion(e.Rule, rules))
		}
		if e.Rule != RuleUnowned {
			p.checkModule(&m, key+".from", e.From, modules)
		}
		switch {
		case e.To == "":
		case p.isOnlyRule(e.Rule):
			m.add(key+".to", "%q is an only rule, which names its library itself: remove to", e.Rule)
		default:
			p.checkModule(&m, key+".to", e.To, modules)
		}
	}
	return m.err()
}

// isOnlyRule reports whether id names a custom only rule.
func (p Policy) isOnlyRule(id string) bool {
	return slices.ContainsFunc(p.Rules, func(r Rule) bool { return r.ID == id && r.Form == FormOnly })
}

func ruleKey(i int, r Rule) string {
	return fmt.Sprintf("rules[%d] (%s)", i, r.ID)
}

// checkModule requires name to be a module.
func (p Policy) checkModule(m *messages, key, name string, modules []string) {
	if _, ok := p.Modules[name]; !ok {
		m.add(key, "%q is not a module%s", name, suggestion(name, modules))
	}
}

// checkName requires name to be a module or a path glob. CheckTree checks
// that a path glob matches a file.
func (p Policy) checkName(m *messages, key, name string, modules []string) {
	if !p.isPathGlob(name) {
		p.checkModule(m, key, name, modules)
	}
}

// isPathGlob reports whether a deny or only name is a path glob: it is not a
// module, and it has a slash or a wildcard. Any other name is a module name.
func (p Policy) isPathGlob(name string) bool {
	_, isModule := p.Modules[name]
	return !isModule && strings.ContainsAny(name, "/"+globMeta)
}

// suggestion returns `: did you mean "x"?` for the closest of names within
// maxSuggestDistance edits, the first by order on a tie, or "".
func suggestion(name string, names []string) string {
	best, bestDistance := "", maxSuggestDistance+1
	for _, candidate := range slices.Sorted(slices.Values(names)) {
		if d := editDistance(name, candidate); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf(": did you mean %q?", best)
}

// editDistance is the Levenshtein distance between a and b, by bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// CheckTree checks the policy against the files of a tree: every path glob
// must match a file, and no file may be owned by two modules at the same
// specificity. A public glob may also match a directory that holds a file:
// public: [pkg/chaos] names a package directory. Every other glob is matched
// against files, as ModuleOf matches them. files are repository-relative and
// slash-separated. The error lists every message, one per line.
func (p Policy) CheckTree(files []string) error {
	var m messages
	dirs := directories(files)
	for _, g := range p.pathGlobs() {
		if !matchesAny(g.glob, files) && (!g.dirs || !matchesAny(g.glob, dirs)) {
			m.add(g.key, "%q matches no file", g.glob)
		}
	}
	type overlap struct {
		first string
		more  int
	}
	overlaps := map[string]*overlap{}
	for _, file := range files {
		owners := p.owners(file)
		if len(owners) < 2 {
			continue
		}
		pair := strings.Join(owners[:len(owners)-1], ", ") + " and " + owners[len(owners)-1]
		if o, ok := overlaps[pair]; ok {
			o.more++
		} else {
			overlaps[pair] = &overlap{first: file}
		}
	}
	for _, pair := range slices.Sorted(maps.Keys(overlaps)) {
		o := overlaps[pair]
		count := ""
		if o.more > 0 {
			count = fmt.Sprintf(" (%d files in all)", o.more+1)
		}
		m.add("modules", "%s own the same files, such as %s%s: make their paths not overlap", pair, o.first, count)
	}
	return m.err()
}

// keyedGlob is a path glob of the policy with its key. dirs: it may match a
// directory.
type keyedGlob struct {
	key, glob string
	dirs      bool
}

// pathGlobs lists every path glob of the policy with its key, in policy order.
func (p Policy) pathGlobs() []keyedGlob {
	var globs []keyedGlob
	add := func(key string, list []string) {
		for _, g := range list {
			globs = append(globs, keyedGlob{key: key, glob: g})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(p.Modules)) {
		m := p.Modules[name]
		add("modules."+name+".paths", m.Paths)
		// A public the file does not name is the paths, checked above.
		if !wholeModule(m) {
			for _, g := range m.Public {
				globs = append(globs, keyedGlob{key: "modules." + name + ".public", glob: g, dirs: true})
			}
		}
	}
	for i, r := range p.Rules {
		key := ruleKey(i, r)
		add(key+".deny.from", slices.DeleteFunc(slices.Clone(r.From), p.isModuleName))
		add(key+".deny.to", slices.DeleteFunc(slices.Clone(r.To), p.isModuleName))
		add(key+".only.in", slices.DeleteFunc(slices.Clone(r.In), p.isModuleName))
	}
	for i, e := range p.Exceptions {
		if e.Rule == RuleUnowned {
			add(fmt.Sprintf("exceptions[%d].from", i), []string{e.From})
		}
	}
	add("generated", p.Generated)
	add("tests", p.Tests)
	return globs
}

func (p Policy) isModuleName(name string) bool { return !p.isPathGlob(name) }

// directories lists every directory that holds a file.
func directories(files []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		for d := path.Dir(f); d != "." && d != "/" && !seen[d]; d = path.Dir(d) {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func matchesAny(glob string, paths []string) bool {
	return slices.ContainsFunc(paths, func(p string) bool { return matchGlob(glob, p) })
}
