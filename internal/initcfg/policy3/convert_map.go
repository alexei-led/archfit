package policy3

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/application/repostate"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// convertHeader opens a converted policy.
const convertHeader = "# The Archfit policy, converted by archfit init from the Archfit 3 files.\n" +
	"# Review the modules, rules and exceptions, then commit it.\n"

// Reasons the conversion writes.
const (
	// whyBaseline is the reason of an exception for an error of a rule the
	// Archfit 3 baseline accepted.
	whyBaseline = "Accepted in the Archfit 3 baseline."
	// whyUnchecked is the reason of a note for an Archfit 3 rule that had none.
	whyUnchecked = "Archfit 3 checked this rule; Archfit 4 does not check it."
	// whyWaiver is the reason of an exception for an Archfit 3 waiver that had
	// none.
	whyWaiver = "Converted from an Archfit 3 waiver."
)

// Archfit 3 rule types and gates the conversion reads.
const (
	gateWarn = "warn"
	gateOff  = "off"

	typeForbiddenDependency = "forbidden_dependency"
)

// builtinOf maps the Archfit 3 rule types a built-in rule replaces.
var builtinOf = map[string]string{
	"module_dependencies":         resolved.RuleGraph,
	"new_cross_module_dependency": resolved.RuleGraph,
	"module_cycle":                resolved.RuleCycle,
	"cycle":                       resolved.RuleCycle,
	"forbidden_layer_direction":   resolved.RuleLayers,
}

// builtinMode is the mode of a built-in rule when the policy names none.
var builtinMode = map[string]policyfile.Mode{
	resolved.RuleGraph:  policyfile.ModeAsk,
	resolved.RuleCycle:  policyfile.ModeDeny,
	resolved.RuleLayers: policyfile.ModeDeny,
}

// noteOf is the note text of the Archfit 3 rule types nothing checks in
// Archfit 4.
var noteOf = map[string]string{
	"public_api_max":       "A module has at most %d exported declarations.",
	"public_api_change":    "A change to the public part of a module is a change of its contract.",
	"public_api_type_leak": "The public part of a module does not expose a type from its inside.",
	"forbidden_pattern":    "Code does not use the patterns the Archfit 3 rule named.",
}

// coveredTypes are the Archfit 3 rule types the coupling rule and public
// cover.
var coveredTypes = []string{"public_api_only", "internal_api_access"}

// syntheticRules maps the Archfit 3 rule ids that no policy rule declared to
// the built-in rule that replaces them.
var syntheticRules = map[string]string{
	"bc/imbalanced_coupling":  resolved.RuleCoupling,
	"bc/duplicated_knowledge": resolved.RuleDuplication,
}

// codeID is a rule id like D-12: a code, not a name.
var codeID = regexp.MustCompile(`^[A-Za-z]{1,4}-?[0-9]+$`)

// Setting keys that repeat in settingsKeys.
const (
	enabledKey = "enabled"
	excludeKey = "exclude"
)

// globMetaChars make a path a glob.
const globMetaChars = "*?[{"

// keptModuleKeys are the module keys schema 3 keeps; volatility is changed,
// depends_on expanded.
var keptModuleKeys = []string{"paths", "public", "layer", "subdomain", "volatility", "depends_on"}

// droppedModuleKey says why a module key is dropped.
var droppedModuleKey = map[string]string{
	"visible_to": "the graph rule and public cover it",
	"internal":   "public covers it: what is not public is the inside",
}

// readTopKeys are the top-level keys of an Archfit 3 policy the conversion
// reads.
var readTopKeys = []string{"version", "layers", "modules", "rules", "waivers", "languages", "analyzers", "coverage", excludeKey}

// droppedTopKey says why a top-level key is dropped.
var droppedTopKey = map[string]string{
	"coupling":      "the coupling rule replaces it: a new use across modules needs a balance of at least 7",
	"metrics":       "Archfit 4 has no metric ratchets; the report shows the metrics",
	"module_review": "the unowned rule replaces it",
	"ai":            "Archfit 4 has no AI settings",
	"target_branch": "the check compares a change with its base",
}

// settingsKeys are the keys of the settings schema 3 keeps (task 1): true is
// a value, a map lists its keys, a one-element list is the schema of each
// element.
var settingsKeys = map[string]any{
	"languages": map[string]any{
		"go":         map[string]any{enabledKey: true, "modules": map[string]any{"include": true, excludeKey: true}},
		"typescript": map[string]any{enabledKey: true},
		"python":     map[string]any{enabledKey: true, "package": true},
		"rust":       map[string]any{enabledKey: true, "manifest": true, "features": true, "include_dev_deps": true},
		"dotnet":     map[string]any{enabledKey: true, "projects": true},
	},
	"analyzers": map[string]any{
		"syntax":        map[string]any{enabledKey: true},
		"cargo_modules": map[string]any{enabledKey: true},
		"roslyn":        map[string]any{enabledKey: true, "timeout": true},
	},
	"coverage": map[string]any{
		enabledKey: true,
		"sources":  []any{map[string]any{"path": true, "format": true, "sidecar_path": true, "max_bytes": true, "max_facts": true}},
	},
	excludeKey: true,
}

// Note is something of the Archfit 3 files the conversion did not convert,
// or changed: the key it comes from and why.
type Note struct {
	Key string
	Why string
}

// Conversion is what Convert wrote and what it could not convert.
type Conversion struct {
	// File is the policy written over the Archfit 3 policy.
	File policyfile.File
	// NoInstead are the ids of the Archfit 3 forbidden_dependency rules with
	// no alternatives. They are notes: nothing checks them until an owner
	// writes instead and makes them deny rules.
	NoInstead []string
	// NotConverted are the keys of the Archfit 3 files that are dropped or
	// changed, in the order of the conversion.
	NotConverted []Note
	// Unknown are the items of the head facts that are not a missing tool.
	Unknown []evidence.UnknownItem
	// Removed are the Archfit 3 files Convert deleted.
	Removed []string
}

// Text is the conversion's list for people: the rules that wait for an
// instead first, then every note.
func (c Conversion) Text() string {
	var b strings.Builder
	if len(c.NoInstead) > 0 {
		fmt.Fprintf(&b, "not checked until an owner writes instead (Archfit 3 named no alternatives): %s\n",
			strings.Join(c.NoInstead, ", "))
	}
	if len(c.NotConverted) == 0 {
		b.WriteString("not converted: nothing\n")
		return b.String()
	}
	b.WriteString("not converted or changed:\n")
	for _, n := range c.NotConverted {
		fmt.Fprintf(&b, "  %s: %s\n", n.Key, n.Why)
	}
	return b.String()
}

// converter holds the state of one conversion.
type converter struct {
	old     archfit3
	facts   evidence.CheckFacts
	sources []string
	dirs    map[string]bool
	file    policyfile.File
	// roles are the Archfit 3 roles of the modules, for role: selectors.
	roles map[string]string
	// checked maps an Archfit 3 rule id to the Archfit 4 rule that checks it.
	checked map[string]string
	ids     map[string]bool
	conv    Conversion
}

func (c *converter) note(key, format string, args ...any) {
	c.conv.NotConverted = append(c.conv.NotConverted, Note{Key: key, Why: fmt.Sprintf(format, args...)})
}

// convert maps the Archfit 3 files to a schema 3 policy for the tree the head
// facts describe. owner is the first owner when the App file names none. It
// returns the conversion and the bytes of the policy, which load and fit the
// tree.
func convert(old archfit3, facts evidence.CheckFacts, generatedGlobs []string, owner string) (Conversion, []byte, error) {
	c := &converter{
		old: old, facts: facts, sources: facts.Sources, dirs: directories(facts.Sources),
		roles: map[string]string{}, checked: maps.Clone(syntheticRules), ids: map[string]bool{},
	}
	for _, id := range resolved.BuiltinRules {
		c.ids[id] = true
	}
	c.file = policyfile.File{
		Version: policyfile.Version, Layers: old.policy.Layers, Modules: map[string]policyfile.Module{},
		Tests: tests(facts.Sources), Generated: generatedGlobs,
	}
	c.app(owner)
	c.topKeys()
	if err := c.settings(); err != nil {
		return Conversion{}, nil, err
	}
	c.modules()
	c.dependsOn()
	c.rules()
	_, p, err := encode(c.file, c.sources, convertHeader)
	if err != nil {
		return Conversion{}, nil, err
	}
	waived := c.waivers(p)
	c.labels()
	for _, d := range old.drafts {
		c.note(d, "a draft nobody approved: not read and not deleted; apply it with Archfit 3 first, or delete it")
	}
	c.file.Exceptions = c.exceptions(p, waived)
	data, _, err := encode(c.file, c.sources, convertHeader)
	if err != nil {
		return Conversion{}, nil, err
	}
	c.conv.File = c.file
	return c.conv, data, nil
}

// app takes owners and gate from the App file.
func (c *converter) app(owner string) {
	app := c.old.app
	if app == nil || len(app.Owners) == 0 {
		c.file.Owners = []string{owner}
		c.note("owners", "%s names no owners: the owner of the origin remote, %s", AppFile, owner)
	} else {
		c.file.Owners = app.Owners
	}
	if app == nil {
		return
	}
	switch g := policyfile.Gate(app.Gate); {
	case slices.Contains(policyfile.Gates, g):
		c.file.Gate = g
	case g != "":
		c.note(AppFile+" gate", "%q is not advisory or required: the gate is advisory", app.Gate)
	}
	for _, item := range app.raw {
		switch k := fmt.Sprint(item.Key); k {
		case "version", "owners", "gate":
		default:
			c.note(AppFile+" "+k, "dropped: Archfit 4 has no such key")
		}
	}
}

// topKeys notes the top-level keys of the policy the conversion does not read.
func (c *converter) topKeys() {
	for _, item := range c.old.raw {
		k := fmt.Sprint(item.Key)
		if slices.Contains(readTopKeys, k) {
			continue
		}
		why, ok := droppedTopKey[k]
		if !ok {
			why = "Archfit 4 has no such key"
		}
		c.note(k, "dropped: %s", why)
	}
}

// settings keeps the languages, analyzers, coverage and exclude keys schema 3
// has, and notes every other key under them.
func (c *converter) settings() error {
	var kept yaml.MapSlice
	for _, item := range c.old.raw {
		k := fmt.Sprint(item.Key)
		if schema, ok := settingsKeys[k]; ok {
			kept = append(kept, yaml.MapItem{Key: k, Value: c.keep(item.Value, schema, k)})
		}
	}
	data, err := yaml.Marshal(kept)
	if err != nil {
		return err
	}
	var s struct {
		Languages policyfile.Languages `yaml:"languages"`
		Analyzers policyfile.Analyzers `yaml:"analyzers"`
		Coverage  policyfile.Coverage  `yaml:"coverage"`
		Exclude   []string             `yaml:"exclude"`
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%s: the language and analyzer keys do not convert: %w", PolicyFile, err)
	}
	c.file.Languages, c.file.Analyzers, c.file.Coverage, c.file.Exclude = s.Languages, s.Analyzers, s.Coverage, s.Exclude
	return nil
}

// keep returns v with only the keys schema has, and notes the others.
func (c *converter) keep(v, schema any, key string) any {
	switch s := schema.(type) {
	case map[string]any:
		m, ok := v.(yaml.MapSlice)
		if !ok {
			return v
		}
		var out yaml.MapSlice
		for _, item := range m {
			k := fmt.Sprint(item.Key)
			if sub, ok := s[k]; ok {
				out = append(out, yaml.MapItem{Key: k, Value: c.keep(item.Value, sub, key+"."+k)})
			} else {
				c.note(key+"."+k, "dropped: Archfit 4 has no such key")
			}
		}
		return out
	case []any:
		list, ok := v.([]any)
		if !ok {
			return v
		}
		out := make([]any, len(list))
		for i, e := range list {
			out[i] = c.keep(e, s[0], fmt.Sprintf("%s[%d]", key, i))
		}
		return out
	default:
		return v
	}
}

// fileGlob is a path of an Archfit 3 policy as a glob of files. Archfit 3
// matched packages: a bare directory was that package, the files right in
// it, and not the directories below it. A glob or a file stays.
func (c *converter) fileGlob(p string) string {
	p = strings.TrimSuffix(p, "/")
	if !strings.ContainsAny(p, globMetaChars) && p != "" && p != rootKey && c.dirs[p] {
		return join(literal(p), "*")
	}
	return p
}

// modules converts the modules in name order.
func (c *converter) modules() {
	mods := c.old.policy.Modules
	names := slices.Sorted(maps.Keys(mods))
	dropped := map[string]int{}
	for _, item := range c.rawMap(c.old.raw, "modules") {
		for _, k := range c.rawMap(item.Value, "") {
			if key := fmt.Sprint(k.Key); !slices.Contains(keptModuleKeys, key) {
				dropped[key]++
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(dropped)) {
		why, ok := droppedModuleKey[key]
		if !ok {
			why = "Archfit 4 has no such key"
		}
		c.note("modules.*."+key, "dropped (%d modules): %s", dropped[key], why)
	}
	for _, name := range names {
		m := mods[name]
		key := "modules." + name
		var paths []string
		for _, p := range m.Paths {
			g := c.fileGlob(p)
			if !matchesAny(g, c.sources) {
				c.note(key+".paths", "%q matches no source file: dropped", p)
				continue
			}
			if !slices.Contains(paths, g) {
				paths = append(paths, g)
			}
		}
		if len(paths) == 0 {
			c.note(key, "no path matches a source file: the module is dropped")
			continue
		}
		c.roles[name] = m.Role
		c.file.Modules[name] = policyfile.Module{
			Paths: paths, Layer: m.Layer,
			Volatility: c.volatility(key, m.Volatility),
			Subdomain:  c.subdomain(key, m.Subdomain),
		}
	}
	owned := c.owned()
	for _, name := range names {
		mod, ok := c.file.Modules[name]
		if !ok {
			continue
		}
		var public []string
		for _, g := range mods[name].Public {
			if !matchesAny(g, c.sources) && !matchesAny(g, slices.Collect(maps.Keys(c.dirs))) {
				c.note("modules."+name+".public", "%q matches no file or directory: dropped", g)
				continue
			}
			public = append(public, g)
		}
		if len(mods[name].Public) > 0 && len(public) == 0 {
			c.note("modules."+name+".public", "no entry matches: the whole module is public")
		}
		mod.Public = collapse(public, packagesOf(owned[name]), otherPackages(owned, name))
		c.file.Modules[name] = mod
	}
}

// rawMap returns the map under key of v (v itself when key is empty), or nil.
func (c *converter) rawMap(v any, key string) yaml.MapSlice {
	m, ok := v.(yaml.MapSlice)
	if !ok {
		return nil
	}
	if key == "" {
		return m
	}
	for _, item := range m {
		if fmt.Sprint(item.Key) == key {
			sub, _ := item.Value.(yaml.MapSlice)
			return sub
		}
	}
	return nil
}

func (c *converter) volatility(key, v string) policyfile.Volatility {
	switch lv := policyfile.Volatility(strings.ToLower(v)); {
	case v == "":
		return ""
	case lv == "frozen" || lv == "legacy":
		c.note(key+".volatility", "%s is low: Archfit 4 has low, medium and high", v)
		return policyfile.VolatilityLow
	case slices.Contains(policyfile.Volatilities, lv):
		return lv
	default:
		c.note(key+".volatility", "%q is not low, medium or high: dropped", v)
		return ""
	}
}

func (c *converter) subdomain(key, v string) policyfile.Subdomain {
	s := policyfile.Subdomain(strings.ToLower(v))
	if v != "" && !slices.Contains(policyfile.Subdomains, s) {
		c.note(key+".subdomain", "%q is not core, supporting or generic: dropped", v)
		return ""
	}
	return s
}

// owned lists the production files of each module.
func (c *converter) owned() map[string][]string {
	p := resolved.Policy{Modules: map[string]resolved.Module{}, Tests: c.file.Tests}
	for name, m := range c.file.Modules {
		p.Modules[name] = resolved.Module{Paths: m.Paths}
	}
	out := map[string][]string{}
	for _, f := range c.sources {
		if p.IsTest(f) {
			continue
		}
		if m, ok := p.ModuleOf(f); ok {
			out[m] = append(out[m], f)
		}
	}
	return out
}

// packagesOf lists the directories of files, sorted.
func packagesOf(files []string) []string {
	var dirs []string
	for _, f := range files {
		if d := path.Dir(f); !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	slices.Sort(dirs)
	return dirs
}

// otherPackages lists the package directories of every module but name.
func otherPackages(owned map[string][]string, name string) []string {
	var out []string
	for m, files := range owned {
		if m != name {
			out = append(out, packagesOf(files)...)
		}
	}
	return out
}

// collapse writes public as directory globs (M1 D8): a public directory and
// its parents up to the highest directory whose packages of the module are
// all public, that holds a package below it, and that holds no package of
// another module, become that directory's dir/**.
func collapse(public, packages, others []string) []string {
	if len(public) == 0 {
		return nil
	}
	isPublic := func(d string) bool { return slices.ContainsFunc(public, func(g string) bool { return match(g, d) }) }
	collapsible := func(a string) bool {
		below := false
		for _, p := range packages {
			if under(p, a) {
				if !isPublic(p) {
					return false
				}
				below = below || p != a
			}
		}
		return below && !slices.ContainsFunc(others, func(d string) bool { return under(d, a) })
	}
	var out []string
	for _, g := range public {
		if !strings.ContainsAny(g, globMetaChars) {
			for a := g; a != rootKey && a != "/"; a = path.Dir(a) {
				if collapsible(a) {
					g = literal(a) + "/**"
				}
			}
		}
		if !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return slices.DeleteFunc(out, func(g string) bool {
		return !strings.ContainsAny(g, globMetaChars) && slices.ContainsFunc(out, func(o string) bool {
			dir, ok := strings.CutSuffix(o, "/**")
			return ok && !strings.ContainsAny(dir, globMetaChars) && under(g, dir)
		})
	})
}

// dependsOn writes the allowed dependencies: the names an Archfit 3 module
// listed, its selectors expanded; for a module with no depends_on, which
// Archfit 3 let depend on any module, the modules it uses today.
func (c *converter) dependsOn() {
	var today uses
	for _, name := range slices.Sorted(maps.Keys(c.file.Modules)) {
		mod := c.file.Modules[name]
		old := c.old.policy.Modules[name].DependsOn
		if old == nil {
			if today.deps == nil {
				today = probe(c.file, c.facts)
			}
			mod.DependsOn = today.deps[name]
			c.note("modules."+name+".depends_on", "absent, so Archfit 3 allowed any module: written as the modules it uses today (%d)",
				len(mod.DependsOn))
		} else {
			mod.DependsOn = []string{}
			for _, sel := range *old {
				names := slices.DeleteFunc(c.selectModules(sel), func(n string) bool { return n == name })
				if len(names) == 0 {
					c.note("modules."+name+".depends_on", "%q names no module: dropped", sel)
				}
				for _, n := range names {
					if !slices.Contains(mod.DependsOn, n) {
						mod.DependsOn = append(mod.DependsOn, n)
					}
				}
			}
			slices.Sort(mod.DependsOn)
		}
		c.file.Modules[name] = mod
	}
}

// selectModules returns the converted modules an Archfit 3 module selector
// names: "layer:<name>", "role:<role>", or a glob of module names.
func (c *converter) selectModules(sel string) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(c.file.Modules)) {
		var ok bool
		switch {
		case strings.HasPrefix(sel, "layer:"):
			ok = c.file.Modules[name].Layer == strings.TrimPrefix(sel, "layer:")
		case strings.HasPrefix(sel, "role:"):
			ok = c.roles[name] == strings.TrimPrefix(sel, "role:")
		default:
			ok = match(sel, name)
		}
		if ok {
			out = append(out, name)
		}
	}
	return out
}

// rules converts the custom rules in file order.
func (c *converter) rules() {
	for i, r := range c.old.policy.Rules {
		key := fmt.Sprintf("rules[%d] (%s)", i, r.ID)
		warn := r.Gate == gateWarn || r.Gate == "" && (r.Type == "public_api_change" || r.Type == "public_api_type_leak")
		switch {
		case builtinOf[r.Type] != "":
			c.builtinRule(key, r, warn)
		case r.Gate == gateOff:
			c.note(key, "gate off: dropped")
		case slices.Contains(coveredTypes, r.Type):
			c.note(key, "%s: dropped: the coupling rule and public cover it", r.Type)
		case r.Type == typeForbiddenDependency:
			c.denyRule(key, r, warn)
		case noteOf[r.Type] != "":
			text := noteOf[r.Type]
			if strings.Contains(text, "%d") {
				text = fmt.Sprintf(text, derefOr(r.Max, 0))
			}
			c.noteRule(key, r, text, r.Type+": written as a note: nothing checks it in Archfit 4")
		default:
			c.note(key, "type %q: dropped: Archfit 4 has no such rule", r.Type)
		}
	}
}

// builtinRule maps a rule to the built-in rule that replaces it.
func (c *converter) builtinRule(key string, r ruleV2, warn bool) {
	b := builtinOf[r.Type]
	if r.Gate == gateOff {
		c.setMode(b, policyfile.ModeOff)
		c.note(key, "gate off: the built-in %s rule is off", b)
		return
	}
	c.checked[r.ID] = b
	switch {
	case warn:
		c.note(key, "gate warn: the built-in %s rule replaces it, mode %s", b, builtinMode[b])
	case r.Rationale != "":
		c.note(key, "the built-in %s rule replaces it; its rationale is dropped", b)
	}
}

func (c *converter) setMode(rule string, m policyfile.Mode) {
	switch rule {
	case resolved.RuleGraph:
		c.file.Builtin.Graph = m
	case resolved.RuleCycle:
		c.file.Builtin.Cycle = m
	case resolved.RuleLayers:
		c.file.Builtin.Layers = m
	}
}

// denyRule converts a forbidden_dependency rule: a deny rule when it has
// alternatives and both sides match code, a note otherwise.
func (c *converter) denyRule(key string, r ruleV2, warn bool) {
	from, fromProblem := c.side("from", r.FromModule, r.From)
	to, toProblem := c.side("to", r.ToModule, r.To)
	text := fmt.Sprintf("No dependency from %s to %s.", strings.Join(from, ", "), strings.Join(to, ", "))
	switch {
	case warn:
		c.noteRule(key, r, text, "gate warn: written as a note")
		return
	case fromProblem != "" || toProblem != "":
		guard := ""
		if r.Guard {
			guard = " (a guard: Archfit 4 has none)"
		}
		c.noteRule(key, r, text, "written as a note: "+strings.TrimPrefix(fromProblem+"; "+toProblem, "; ")+guard)
		return
	case len(r.Alternatives) == 0:
		id := c.noteRule(key, r, text, "written as a note: Archfit 3 named no alternatives, and a deny rule needs instead")
		c.conv.NoInstead = append(c.conv.NoInstead, id)
		return
	}
	id := c.ruleID(key, r)
	c.checked[r.ID] = id
	c.file.Rules = append(c.file.Rules, policyfile.Rule{
		ID: id, Deny: &policyfile.Deny{From: from, To: to},
		Why: c.why(key, r), Instead: strings.Join(r.Alternatives, "; "),
	})
}

// side converts one side of a forbidden_dependency rule: the modules its
// selector names, or its path as a file glob. problem says why the side
// matches no code.
func (c *converter) side(name, selector, glob string) (names policyfile.Names, problem string) {
	if selector != "" {
		names = c.selectModules(selector)
		if len(names) == 0 {
			return policyfile.Names{selector}, fmt.Sprintf("%s_module %q names no module", name, selector)
		}
		return names, ""
	}
	g := c.fileGlob(glob)
	if !matchesAny(g, c.sources) {
		return policyfile.Names{g}, fmt.Sprintf("%s %q matches no file", name, glob)
	}
	return policyfile.Names{g}, ""
}

// noteRule writes r as a note rule and returns its id.
func (c *converter) noteRule(key string, r ruleV2, text, why string) string {
	id := c.ruleID(key, r)
	c.file.Rules = append(c.file.Rules, policyfile.Rule{ID: id, Note: text, Why: c.why(key, r)})
	c.note(key, "%s", why)
	return id
}

// why is the rule's rationale, with its document.
func (c *converter) why(key string, r ruleV2) string {
	why := r.Rationale
	if why == "" {
		if r.Type != typeForbiddenDependency {
			return whyUnchecked
		}
		c.note(key, "Archfit 3 gave no rationale: why says so")
		why = "Converted from the Archfit 3 rule " + r.ID + ", which gave no reason."
	}
	if r.Docs != "" {
		why += " See " + r.Docs + "."
	}
	return why
}

// ruleID is the id of a converted rule: the Archfit 3 id when it is a name, or
// one made from the type and the id when it is a code like D-12, a built-in
// rule id, or taken.
func (c *converter) ruleID(key string, r ruleV2) string {
	id := r.ID
	if id == "" || codeID.MatchString(id) || c.ids[id] {
		base := strings.ReplaceAll(r.Type, "_", "-")
		if r.ID != "" {
			base += "-" + strings.ToLower(r.ID)
		}
		id = base
		for n := 2; c.ids[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		c.note(key, "the id is now %s", id)
	}
	c.ids[id] = true
	return id
}

// waivers converts the waivers to exceptions.
func (c *converter) waivers(p resolved.Policy) []policyfile.Exception {
	var out []policyfile.Exception
	for i, w := range c.old.policy.Waivers {
		key := fmt.Sprintf("waivers[%d] (%s)", i, w.Rule)
		rule, ok := c.checked[w.Rule]
		if !ok {
			c.note(key, "dropped: nothing checks its rule in Archfit 4")
			continue
		}
		from, fromErr := c.moduleOfGlob(p, w.From)
		to, toErr := c.moduleOfGlob(p, w.To)
		if fromErr != "" || toErr != "" {
			c.note(key, "dropped: %s", strings.TrimPrefix(fromErr+"; "+toErr, "; "))
			continue
		}
		why := w.Reason
		if why == "" {
			why = whyWaiver
		}
		out = append(out, policyfile.Exception{Rule: rule, From: from, To: to, Expires: w.Expires, Why: why})
	}
	return out
}

// moduleOfGlob returns the one module whose files an Archfit 3 path glob
// matches, or why there is none.
func (c *converter) moduleOfGlob(p resolved.Policy, glob string) (string, string) {
	if glob == "" {
		return "", "it names no place"
	}
	g := c.fileGlob(glob)
	var mods []string
	for _, f := range c.sources {
		if m, ok := p.ModuleOf(f); ok && match(g, f) && !slices.Contains(mods, m) {
			mods = append(mods, m)
		}
	}
	switch len(mods) {
	case 1:
		return mods[0], ""
	case 0:
		return "", fmt.Sprintf("%q matches no file of a module", glob)
	default:
		slices.Sort(mods)
		return "", fmt.Sprintf("%q matches files of %d modules (%s)", glob, len(mods), strings.Join(mods, ", "))
	}
}

// labels notes each label: Archfit 4 has no labels.
func (c *converter) labels() {
	if c.old.labels == nil {
		return
	}
	for _, l := range c.old.labels.Labels {
		c.note(LabelsFile, "%s -> %s %s: dropped: Archfit 4 takes the strength of a use from public", l.From, l.To, l.Strength)
	}
}

// exceptions returns the waivers' exceptions, then one for each other rule
// and place where the policy p reports errors on the head (repostate's
// Existing), so the first check passes. It notes, for each rule, how many it
// wrote and how many entries the Archfit 3 baseline accepted.
func (c *converter) exceptions(p resolved.Policy, waived []policyfile.Exception) []policyfile.Exception {
	accepted, dropped := c.baselineCounts()
	place := func(rule, from, to string) string { return rule + "\x00" + from + "\x00" + to }
	seen := map[string]bool{}
	for _, x := range waived {
		seen[place(x.Rule, x.From, x.To)] = true
	}
	out := slices.Clone(waived)
	written := map[string]int{}
	for _, x := range repostate.Evaluate(repostate.Input{Policy: p, Head: c.facts, Today: time.Now()}).Existing {
		if seen[place(x.Rule, x.From, x.To)] {
			continue
		}
		why := whyException
		if accepted[x.Rule] > 0 {
			why = whyBaseline
		}
		out = append(out, policyfile.Exception{Rule: x.Rule, From: x.From, To: x.To, Why: why})
		written[x.Rule]++
	}
	reported := map[string]bool{}
	for _, rule := range slices.Concat(resolved.BuiltinRules, slices.Sorted(maps.Keys(accepted)), slices.Sorted(maps.Keys(written))) {
		if reported[rule] || written[rule] == 0 && accepted[rule] == 0 {
			continue
		}
		reported[rule] = true
		c.note("exceptions."+rule, "%d written for the errors of today; the Archfit 3 baseline accepted %d", written[rule], accepted[rule])
	}
	for _, d := range dropped {
		c.note(BaselineFile, "%s", d)
	}
	if b := c.old.baseline; b != nil && len(b.Metrics) > 0 {
		c.note(BaselineFile+" metrics", "dropped: Archfit 4 has no metric ratchets")
	}
	return out
}

// baselineCounts counts the accepted baseline entries per Archfit 4 rule, and
// says which entries no Archfit 4 rule checks.
func (c *converter) baselineCounts() (map[string]int, []string) {
	accepted := map[string]int{}
	if c.old.baseline == nil {
		return accepted, nil
	}
	var order []string
	none := map[string]int{}
	for _, a := range c.old.baseline.Accepted {
		if rule, ok := c.checked[a.RuleID]; ok {
			accepted[rule]++
			continue
		}
		if none[a.RuleID] == 0 {
			order = append(order, a.RuleID)
		}
		none[a.RuleID]++
	}
	var dropped []string
	for _, id := range order {
		dropped = append(dropped, fmt.Sprintf("%d accepted entries of %s dropped: nothing checks that rule in Archfit 4", none[id], id))
	}
	return accepted, dropped
}

func derefOr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}
