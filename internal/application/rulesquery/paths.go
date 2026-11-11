package rulesquery

import (
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// pathGroups sorts the asked paths by what owns them.
type pathGroups struct {
	modules   map[string][]string
	unowned   []string
	tests     []string
	generated []string
}

// paths writes the paths form: one block per module, then the paths in no
// module, then the declared tests and generated code, then the notes. Paths
// are matched as given, so a file that does not exist yet has a module too.
func paths(p resolved.Policy, date time.Time, files []string) string {
	g := group(p, files)
	var w writer
	for _, name := range slices.Sorted(maps.Keys(g.modules)) {
		moduleBlock(&w, p, date, name, g.modules[name])
	}
	if len(g.unowned) > 0 {
		w.line(0, "%s: in no module", strings.Join(g.unowned, ", "))
		if p.Builtin.Unowned != resolved.ModeOff {
			w.line(1, "rules that apply:")
			w.builtinLine(2, p, resolved.RuleUnowned)
		}
		exceptions(&w, p, date, func(e resolved.Exception) bool {
			return e.Rule == resolved.RuleUnowned && slices.ContainsFunc(g.unowned, func(f string) bool { return p.Covers(e.From, f) })
		})
	}
	if len(g.tests) > 0 {
		w.line(0, "%s: declared tests", strings.Join(g.tests, ", "))
		w.line(1, "the rules skip these paths")
	}
	if len(g.generated) > 0 {
		w.line(0, "%s: declared generated", strings.Join(g.generated, ", "))
		w.line(1, "the rules skip these paths")
	}
	notes(&w, p)
	return w.b.String()
}

// group cleans the paths, drops repeats and sorts each by its owner.
func group(p resolved.Policy, files []string) pathGroups {
	g := pathGroups{modules: map[string][]string{}}
	cleaned := make([]string, 0, len(files))
	for _, f := range files {
		cleaned = append(cleaned, strings.TrimPrefix(path.Clean(f), "./"))
	}
	slices.Sort(cleaned)
	for _, f := range slices.Compact(cleaned) {
		switch name, owned := p.ModuleOf(f); {
		case p.IsTest(f):
			g.tests = append(g.tests, f)
		case p.IsGenerated(f):
			g.generated = append(g.generated, f)
		case owned:
			g.modules[name] = append(g.modules[name], f)
		default:
			g.unowned = append(g.unowned, f)
		}
	}
	return g
}

// moduleBlock writes a module, the built-in and custom rules that apply to
// its files, and its exceptions.
func moduleBlock(w *writer, p resolved.Policy, date time.Time, name string, files []string) {
	m := p.Modules[name]
	w.line(0, "%s: module %s", strings.Join(files, ", "), name)
	w.line(1, "public: %s", strings.Join(m.Public, ", "))
	volatility := string(m.Volatility)
	if m.Subdomain != "" {
		volatility += " (subdomain " + string(m.Subdomain) + ")"
	}
	w.line(1, "volatility: %s", volatility)
	w.line(1, "layer: %s", orNone(m.Layer))
	w.line(1, "allowed dependencies: %s", listOrNone(m.DependsOn))
	w.line(1, "rules that apply:")
	for _, id := range resolved.BuiltinRules {
		mode := builtinMode(p.Builtin, id)
		// unowned is about code in no module; layers needs layers.
		if mode == resolved.ModeOff || id == resolved.RuleUnowned || (id == resolved.RuleLayers && len(p.Layers) == 0) {
			continue
		}
		w.builtinLine(2, p, id)
	}
	for _, r := range p.Rules {
		if applies(p, r, name, files) {
			w.customRule(2, r)
		}
	}
	exceptions(w, p, date, func(e resolved.Exception) bool {
		return e.Rule != resolved.RuleUnowned && (e.From == name || e.To == name)
	})
}

// applies reports whether a deny or only rule applies to a module's files. An
// only rule limits a library everywhere, so it applies to every module; a deny
// rule applies to both of its sides.
func applies(p resolved.Policy, r resolved.Rule, module string, files []string) bool {
	switch r.Form {
	case resolved.FormOnly:
		return true
	case resolved.FormDeny:
		return names(p, r.From, module, files) || names(p, r.To, module, files)
	default:
		return false
	}
}

// names reports whether a deny list names the module, or holds a path glob
// that covers one of its files.
func names(p resolved.Policy, list []string, module string, files []string) bool {
	return slices.ContainsFunc(list, func(entry string) bool {
		if _, isModule := p.Modules[entry]; isModule {
			return entry == module
		}
		return slices.ContainsFunc(files, func(f string) bool { return p.Covers(entry, f) })
	})
}

// exceptions writes the exceptions that keep returns true for, in policy
// order, under one heading.
func exceptions(w *writer, p resolved.Policy, date time.Time, keep func(resolved.Exception) bool) {
	heading := false
	for _, e := range p.Exceptions {
		if !keep(e) {
			continue
		}
		if !heading {
			w.line(1, "exceptions:")
			heading = true
		}
		w.exception(2, e, date)
	}
}

// notes writes every note rule as not checked. A note names no module, so it
// belongs to the whole policy and is listed once.
func notes(w *writer, p resolved.Policy) {
	heading := false
	for _, r := range p.Rules {
		if r.Form != resolved.FormNote {
			continue
		}
		if !heading {
			w.line(0, "not checked:")
			heading = true
		}
		w.customRule(1, r)
	}
}
