package resolved

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// Direction says whether a policy change loosens it, tightens it, or neither.
type Direction string

// Direction values.
const (
	Loosens  Direction = "loosens"
	Tightens Direction = "tightens"
	Neither  Direction = "neither"
)

// Change is one change between two policies.
type Change struct {
	Direction Direction
	// Key names the changed part, such as "modules.pkg_tc.depends_on".
	Key string
	// Text says what changed, such as `adds "pkg_util"`.
	Text string
}

// Comparison is the answer of Compare.
type Comparison struct {
	// Direction loosens when any change loosens; otherwise tightens when any
	// change tightens; otherwise neither.
	Direction Direction
	// Changes are every change, one entry each, in policy order.
	Changes []Change
}

// Compare says whether after loosens before, tightens it, or neither, and
// lists every change. A loosening needs an owner (design 9.2), so a change the
// comparison cannot prove tighter counts as loosening:
//
//   - loosens: an added depends_on entry; an added or removed module or
//     module path (a removed one can hand files to a nested module's parent or
//     take them out of a deny rule that names the module); a wider public; a
//     mode moved toward off; a lower min_balance; a removed rule; an added
//     exception, a later expires or a removed one; an added generated, tests or
//     exclude glob; a lower volatility; any change to owners; gate required to
//     advisory; a removed or changed module layer; any change to layers; any
//     change to a tool or language setting.
//   - tightens: the reverse of each of those that has a reverse.
//   - neither: the text of why, instead and note; a subdomain that leaves the
//     volatility as it was; coverage settings, which only the report reads.
//
// A rule whose body changes counts each part it adds or drops by what that
// part does: a deny entry added tightens and one dropped loosens; an only.in
// entry added loosens and one dropped tightens; a changed only.use loosens; a
// changed form loosens, unless a note becomes a check. Path globs compare by
// cover: "pkg/**" covers "pkg/chaos/**", so replacing the second by the first
// only adds. A module name covers only itself, never a directory of that name.
// Both policies must have their defaults applied.
func Compare(before, after Policy) Comparison {
	d := diff{before: before, after: after}
	d.owners(before.Owners, after.Owners)
	if before.Gate != after.Gate {
		dir := Loosens
		if after.Gate == GateRequired {
			dir = Tightens
		}
		d.add(dir, "gate", "%s → %s", before.Gate, after.Gate)
	}
	if !slices.Equal(before.Layers, after.Layers) {
		d.add(Loosens, "layers", "[%s] → [%s]", strings.Join(before.Layers, ", "), strings.Join(after.Layers, ", "))
	}
	d.modules(before.Modules, after.Modules)
	d.builtin(before.Builtin, after.Builtin)
	d.rules(before.Rules, after.Rules)
	d.exceptions(before.Exceptions, after.Exceptions)
	d.globs("generated", before.Generated, after.Generated)
	d.globs("tests", before.Tests, after.Tests)
	d.globs("exclude", before.Exclude, after.Exclude)
	d.settings(before.Settings, after.Settings)

	c := Comparison{Direction: Neither, Changes: d.changes}
	for _, ch := range d.changes {
		switch {
		case ch.Direction == Loosens:
			c.Direction = Loosens
		case ch.Direction == Tightens && c.Direction == Neither:
			c.Direction = Tightens
		}
	}
	return c
}

type diff struct {
	before, after Policy
	changes       []Change
}

func (d *diff) add(dir Direction, key, format string, args ...any) {
	d.changes = append(d.changes, Change{Direction: dir, Key: key, Text: fmt.Sprintf(format, args...)})
}

func (d *diff) owners(before, after []string) {
	if !slices.Equal(slices.Sorted(slices.Values(before)), slices.Sorted(slices.Values(after))) {
		d.add(Loosens, "owners", "[%s] → [%s]", strings.Join(before, ", "), strings.Join(after, ", "))
	}
}

func (d *diff) modules(before, after map[string]Module) {
	for _, name := range unionKeys(before, after) {
		b, inBefore := before[name]
		a, inAfter := after[name]
		key := "modules." + name
		switch {
		case !inAfter:
			d.add(Loosens, key, "removes the module")
		case !inBefore:
			d.add(Loosens, key, "adds the module")
		default:
			d.module(key, b, a)
		}
	}
}

func (d *diff) module(key string, b, a Module) {
	// A path dropped from a module goes to no module or to an enclosing one,
	// and leaves every deny rule that names the module: both can loosen.
	// Ownership goes to the most specific glob, so a glob that another glob of
	// the module covers still decides who owns its files: paths compare by
	// text, not by cover.
	d.list(key+".paths", asNames(b.Paths), asNames(a.Paths), Loosens, Loosens)
	// A public that is the whole module on both sides follows paths; the paths
	// change already says it.
	if !wholeModule(b) || !wholeModule(a) {
		d.globs(key+".public", b.Public, a.Public)
	}
	if b.Volatility != a.Volatility {
		dir := Loosens
		if rank(volatilityRank, a.Volatility) > rank(volatilityRank, b.Volatility) {
			dir = Tightens
		}
		d.add(dir, key+".volatility", "%s → %s", b.Volatility, a.Volatility)
	}
	if b.Subdomain != a.Subdomain {
		d.add(Neither, key+".subdomain", "%s → %s", orAbsent(string(b.Subdomain)), orAbsent(string(a.Subdomain)))
	}
	if b.Layer != a.Layer {
		dir := Loosens
		if b.Layer == "" {
			dir = Tightens
		}
		d.add(dir, key+".layer", "%s → %s", orAbsent(b.Layer), orAbsent(a.Layer))
	}
	d.list(key+".depends_on", asNames(b.DependsOn), asNames(a.DependsOn), Loosens, Tightens)
}

var (
	volatilityRank = []Volatility{VolatilityLow, VolatilityMedium, VolatilityHigh}
	modeRank       = []Mode{ModeOff, ModeAsk, ModeDeny}
)

// rank is the place of v in order, from the loosest.
func rank[T comparable](order []T, v T) int { return slices.Index(order, v) }

func (d *diff) builtin(b, a Builtin) {
	bm, am := b.modes(), a.modes()
	for i := range bm {
		if bm[i].mode == am[i].mode {
			continue
		}
		dir := Loosens
		if rank(modeRank, am[i].mode) > rank(modeRank, bm[i].mode) {
			dir = Tightens
		}
		d.add(dir, "builtin."+bm[i].rule, "%s → %s", bm[i].mode, am[i].mode)
	}
	if b.MinBalance != a.MinBalance {
		dir := Loosens
		if a.MinBalance > b.MinBalance {
			dir = Tightens
		}
		d.add(dir, "builtin.min_balance", "%d → %d", b.MinBalance, a.MinBalance)
	}
}

func (d *diff) rules(before, after []Rule) {
	byID := func(rules []Rule) map[string]Rule {
		m := make(map[string]Rule, len(rules))
		for _, r := range rules {
			m[r.ID] = r
		}
		return m
	}
	bm, am := byID(before), byID(after)
	for _, id := range orderedIDs(before, after) {
		b, inBefore := bm[id]
		a, inAfter := am[id]
		key := "rules." + id
		switch {
		case !inAfter:
			d.add(Loosens, key, "removes the %s rule", b.Form)
		case !inBefore:
			d.add(Tightens, key, "adds a %s rule", a.Form)
		default:
			d.rule(key, b, a)
		}
	}
}

// orderedIDs lists rule ids as before has them, then the new ones of after.
func orderedIDs(before, after []Rule) []string {
	var ids []string
	for _, r := range slices.Concat(before, after) {
		if !slices.Contains(ids, r.ID) {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func (d *diff) rule(key string, b, a Rule) {
	switch {
	case b.Form != a.Form:
		// A note checks nothing: turning it into a check only adds; turning a
		// check into a note only removes; deny to only does both.
		dir := Loosens
		if b.Form == FormNote {
			dir = Tightens
		}
		d.add(dir, key, "%s → %s", b.Form, a.Form)
	case a.Form == FormDeny:
		d.list(key+".deny.from", d.before.selectors(b.From), d.after.selectors(a.From), Tightens, Loosens)
		d.list(key+".deny.to", d.before.selectors(b.To), d.after.selectors(a.To), Tightens, Loosens)
	case a.Form == FormOnly:
		if b.Use != a.Use {
			d.add(Loosens, key+".only.use", "%q → %q", b.Use, a.Use)
		}
		d.list(key+".only.in", d.before.selectors(b.In), d.after.selectors(a.In), Loosens, Tightens)
	}
	d.text(key+".note", b.Note, a.Note)
	d.text(key+".why", b.Why, a.Why)
	d.text(key+".instead", b.Instead, a.Instead)
}

func (d *diff) text(key, b, a string) {
	if b != a {
		d.add(Neither, key, "%q → %q", b, a)
	}
}

func (d *diff) exceptions(before, after []Exception) {
	group := func(list []Exception) (map[string][]Exception, []string) {
		m := map[string][]Exception{}
		var keys []string
		for _, e := range list {
			k := exceptionKey(e)
			if _, ok := m[k]; !ok {
				keys = append(keys, k)
			}
			m[k] = append(m[k], e)
		}
		return m, keys
	}
	bm, bkeys := group(before)
	am, akeys := group(after)
	for _, k := range akeys {
		if bm[k] == nil {
			bkeys = append(bkeys, k)
		}
	}
	for _, k := range bkeys {
		key := "exceptions (" + k + ")"
		bs, as := bm[k], am[k]
		for i := range max(len(bs), len(as)) {
			switch {
			case i >= len(as):
				d.add(Tightens, key, "removes the exception")
			case i >= len(bs):
				d.add(Loosens, key, "adds the exception")
			default:
				d.expires(key+".expires", bs[i].Expires, as[i].Expires)
				d.text(key+".why", bs[i].Why, as[i].Why)
			}
		}
	}
}

// exceptionKey names the place of an exception: the rule, from and to. A
// duplication exception matches the unordered module pair.
func exceptionKey(e Exception) string {
	from, to := e.From, e.To
	if e.Rule == RuleDuplication && to < from {
		from, to = to, from
	}
	if to == "" {
		return e.Rule + " " + from
	}
	return e.Rule + " " + from + " → " + to
}

func (d *diff) expires(key string, b, a time.Time) {
	switch {
	case b.Equal(a):
	case a.IsZero():
		d.add(Loosens, key, "%s → no date", date(b))
	case b.IsZero():
		d.add(Tightens, key, "no date → %s", date(a))
	case a.After(b):
		d.add(Loosens, key, "%s → %s", date(b), date(a))
	default:
		d.add(Tightens, key, "%s → %s", date(b), date(a))
	}
}

func date(t time.Time) string { return t.Format(time.DateOnly) }

// item is one entry of a list Compare diffs: a path glob, or a name that
// covers only itself (a module name, or a module path, whose specificity
// matters).
type item struct {
	text string
	glob bool
}

func asGlobs(list []string) []item { return asItems(list, func(string) bool { return true }) }

func asNames(list []string) []item { return asItems(list, func(string) bool { return false }) }

// selectors reads deny and only entries as p does: module names or path globs.
func (p Policy) selectors(list []string) []item { return asItems(list, p.isPathGlob) }

func asItems(list []string, isGlob func(string) bool) []item {
	items := make([]item, len(list))
	for i, s := range list {
		items[i] = item{text: s, glob: isGlob(s)}
	}
	return items
}

// globs diffs two lists of path globs that take code in (public) or out of
// the rules (generated, tests, exclude): an added one loosens, a removed one
// tightens.
func (d *diff) globs(key string, before, after []string) {
	d.list(key, asGlobs(before), asGlobs(after), Loosens, Tightens)
}

// list reports the items after has that no item of before covers (added) and
// the ones before has that no item of after covers (removed).
func (d *diff) list(key string, before, after []item, added, removed Direction) {
	for _, g := range after {
		if !coveredBy(g, before) {
			d.add(added, key, "adds %q", g.text)
		}
	}
	for _, g := range before {
		if !coveredBy(g, after) {
			d.add(removed, key, "removes %q", g.text)
		}
	}
}

func coveredBy(g item, list []item) bool {
	return slices.ContainsFunc(list, func(c item) bool {
		if c.glob != g.glob {
			return false
		}
		if !g.glob {
			return c.text == g.text
		}
		return covers(c.text, g.text)
	})
}

// covers reports whether every path glob g matches is one that c matches, as
// far as the text shows: c equals g, c is "**", or c is "dir/**" and g is dir
// or below it. Anything else counts as not covered.
func covers(c, g string) bool {
	if c == g || c == "**" {
		return true
	}
	dir, ok := strings.CutSuffix(c, "/**")
	if !ok || strings.ContainsAny(dir, globMeta) {
		return false
	}
	return g == dir || strings.HasPrefix(g, dir+"/")
}

// settings compares the tool, language and coverage keys. Coverage is read
// only by the report, so its changes are neither; any other change can hide
// code from the check and loosens.
func (d *diff) settings(before, after map[string]string) {
	for _, k := range unionKeys(before, after) {
		b, inBefore := before[k]
		a, inAfter := after[k]
		if inBefore == inAfter && b == a {
			continue
		}
		dir := Loosens
		if k == "coverage" || strings.HasPrefix(k, "coverage.") {
			dir = Neither
		}
		d.add(dir, k, "%s → %s", orAbsent(b), orAbsent(a))
	}
}

// unionKeys returns the keys of a and b, in order.
func unionKeys[V any](a, b map[string]V) []string {
	keys := slices.Collect(maps.Keys(a))
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func orAbsent(s string) string {
	if s == "" {
		return "absent"
	}
	return s
}
