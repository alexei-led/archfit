package change

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/balance"
)

// The texts of the built-in rules. Design 8.1 gives the graph texts and the
// two kinds of next step word for word.
const (
	whyGraph    = "a new dependency between modules needs an owner"
	whyCoupling = "new knowledge across a module boundary needs a balance of at least %d"
	whyLayers   = "a module may depend only on its own layer or an inner one"
	whyCycle    = "a module in a cycle cannot change without the others"
	whyUnowned  = "new production code belongs to a module"

	nextFixText     = "fix the code"
	nextRequestText = `archfit request -m "<why you need it>"`
	nextRequestNote = "do not copy the code"
)

// End is one side of a use: a module, a file or directory of the tree, or a
// library.
type End struct {
	// Module is the module of the end. Empty with a Path of the tree: the
	// module that owns Path (ModuleOf). Empty for a library.
	Module string
	// Path is a repository-relative, slash-separated file or directory, or a
	// library as imported. Empty when the end is a module named on its own,
	// which as a target means its public part.
	Path string
	// IsFile says Path is a file. A Go import names a package directory, a
	// TypeScript or Python import a file.
	IsFile bool
	// Files are, for a directory, the source files directly in it, without
	// declared tests and generated files. A glob that names files covers the
	// directory through them: pkg/foo/*.go covers the package pkg/foo.
	Files []string
	// Library says Path is a library or package outside the tree.
	Library bool
}

// Use is one dependency, existing or asked about: From uses To.
type Use struct {
	From, To End
	// Lang is the language of the importer. An only rule that names a
	// package set reads the set's entries for that language; with no
	// language it reads the entries of every language.
	Lang resolved.Language
}

// Judge returns the errors of every rule that one use breaks: the built-in
// graph, coupling and layers rules, then the custom deny and only rules in
// policy order. It is the one judgment of a single dependency: Evaluate calls
// it for every import of a tree, and `archfit can` calls it for a dependency
// that does not exist yet. The errors have no sites. p has its defaults.
func Judge(p resolved.Policy, u Use) []Error {
	u.From.Module = moduleOf(p, u.From)
	u.To.Module = moduleOf(p, u.To)
	errs := builtin(p, u)
	for _, r := range p.Rules {
		var err *Error
		switch r.Form {
		case resolved.FormDeny:
			err = deny(p, r, u)
		case resolved.FormOnly:
			err = only(p, r, u)
		case resolved.FormNote:
		}
		if err != nil {
			errs = append(errs, *err)
		}
	}
	return errs
}

// moduleOf is the module of an end: the one it names, or the one that owns
// its path. A library is in no module.
func moduleOf(p resolved.Policy, e End) string {
	if e.Module != "" || e.Library || e.Path == "" {
		return e.Module
	}
	m, _ := p.ModuleOf(e.Path)
	return m
}

// builtin applies graph, coupling and layers to a use between two modules.
// Each error carries the check's What, which names the pair, and the Phrase
// `archfit can` prints after its own pair.
func builtin(p resolved.Policy, u Use) []Error {
	bal := BalanceOf(p, u)
	if bal == nil {
		return nil
	}
	from, to := u.From.Module, u.To.Module
	fromMod, toMod := p.Modules[from], p.Modules[to]
	symbol := u.To.Path
	if symbol == "" {
		symbol = to
	}
	dependency := func(rule string, mode resolved.Mode, what, phrase, why string, b *result.Balance) Error {
		return Error{
			Key: Key{Rule: rule, From: from, To: to, Symbol: symbol}, From: from, To: to,
			What: what, Phrase: phrase, Why: why, Balance: b, Next: nextOf(mode),
		}
	}
	var errs []Error
	if p.Builtin.Graph != resolved.ModeOff && !slices.Contains(fromMod.DependsOn, to) {
		const phrase = "not an allowed dependency"
		errs = append(errs, dependency(resolved.RuleGraph, p.Builtin.Graph,
			fmt.Sprintf("%s -> %s is %s", from, to, phrase), phrase, whyGraph, bal))
	}
	if p.Builtin.Coupling != resolved.ModeOff && bal.Value < p.Builtin.MinBalance {
		phrase := "reaches past the public part"
		if publicUse(p, u.To) {
			phrase = "uses the public part"
		}
		errs = append(errs, dependency(resolved.RuleCoupling, p.Builtin.Coupling,
			fmt.Sprintf("%s -> %s %s of %s", from, to, phrase, to), phrase,
			fmt.Sprintf(whyCoupling, p.Builtin.MinBalance), bal))
	}
	if p.Builtin.Layers != resolved.ModeOff && inverted(p.Layers, fromMod.Layer, toMod.Layer) {
		errs = append(errs, dependency(resolved.RuleLayers, p.Builtin.Layers,
			fmt.Sprintf("%s (%s) -> %s (%s) goes against the layer order", from, fromMod.Layer, to, toMod.Layer),
			fmt.Sprintf("layer %s -> layer %s goes against the layer order", fromMod.Layer, toMod.Layer),
			whyLayers, nil))
	}
	return errs
}

// BalanceOf is the balance a use between two modules has, or would have: of
// a use of the public part, or past it (design 6.2). A use whose To is a
// module named on its own uses its public part. nil when the use is not
// between two modules: an end in no module, one module, or a library. The
// graph and coupling errors of Judge carry this balance; `archfit can` shows
// it for an allowed dependency. p has its defaults.
func BalanceOf(p resolved.Policy, u Use) *result.Balance {
	u.From.Module = moduleOf(p, u.From)
	u.To.Module = moduleOf(p, u.To)
	if !betweenModules(u) {
		return nil
	}
	value := balanceOf(publicUse(p, u.To), p.Modules[u.From.Module].Volatility, p.Modules[u.To.Module].Volatility)
	return &result.Balance{Value: value, Min: p.Builtin.MinBalance}
}

// betweenModules reports whether a use, its modules resolved, crosses from
// one module into another.
func betweenModules(u Use) bool {
	from, to := u.From.Module, u.To.Module
	return from != "" && to != "" && from != to && !u.To.Library
}

// publicUse reports whether a target end, its module resolved, is in that
// module's public part. A module named on its own means its public part.
func publicUse(p resolved.Policy, to End) bool {
	return to.Path == "" || isPublic(p, to.Module, to)
}

// balanceOf is the balance of a use of the public part (contract strength,
// the target's volatility) or past it (functional strength, the higher of the
// two volatilities): design 6.2, computed by the book formula.
func balanceOf(public bool, from, to resolved.Volatility) int {
	strength := relationship.StrengthFunctional
	if public {
		strength = relationship.StrengthContract
	}
	// Contract and functional always score, so the abstain flag is never false
	// here; TestJudgeCouplingMatrix reads the balance of every cell.
	value, _ := balance.Of(strength, relationship.Volatility(from), relationship.Volatility(to))
	return value
}

// isPublic reports whether an end is in the public part of module: its path,
// or, for a directory, one of its files.
func isPublic(p resolved.Policy, module string, e End) bool {
	if p.IsPublic(module, e.Path, e.IsFile) {
		return true
	}
	return slices.ContainsFunc(e.Files, func(f string) bool { return p.IsPublic(module, f, true) })
}

// covers reports whether a path glob covers an end: its path, or, for a
// directory, one of its files.
func covers(p resolved.Policy, glob string, e End) bool {
	if e.Path == "" || e.Library {
		return false
	}
	return p.Covers(glob, e.Path) || slices.ContainsFunc(e.Files, func(f string) bool { return p.Covers(glob, f) })
}

// inverted reports whether a module of layer from depending on a module of
// layer to goes outward: layers are inner first, and a module may depend on
// its own layer or an inner one. No layer on either side: no layer rule.
func inverted(layers []string, from, to string) bool {
	f, t := slices.Index(layers, from), slices.Index(layers, to)
	return f >= 0 && t >= 0 && f < t
}

// deny applies a custom deny rule: the use breaks it when From matches one of
// its from names and To one of its to names.
func deny(p resolved.Policy, r resolved.Rule, u Use) *Error {
	if u.To.Library {
		return nil
	}
	from, okFrom := firstMatch(p, r.From, u.From)
	to, okTo := firstMatch(p, r.To, u.To)
	if !okFrom || !okTo {
		return nil
	}
	// The key holds modules, as every key does, so exceptions match it and two
	// modules under one path glob keep apart; the text names the rule's names.
	what := fmt.Sprintf("%s must not depend on %s", from, to)
	return &Error{
		Key:  Key{Rule: r.ID, From: u.From.Module, To: u.To.Module, Symbol: symbolOf(u.To)},
		From: u.From.Module, To: u.To.Module,
		What: what, Phrase: what,
		Why: r.Why, Instead: r.Instead, Next: nextOf(resolved.ModeDeny),
	}
}

// only applies a custom only rule: a use of its library or package set
// breaks it unless the importer is in one of its in names.
func only(p resolved.Policy, r resolved.Rule, u Use) *Error {
	if !u.To.Library || !uses(r.Use, u.Lang, u.To.Path) {
		return nil
	}
	if _, in := firstMatch(p, r.In, u.From); in {
		return nil
	}
	// An importer in no module has an empty From: its file is an unowned
	// error of its own. The text names the file.
	label := u.From.Module
	if label == "" {
		label = u.From.Path
	}
	used := u.To.Path
	if used != r.Use {
		used += " (" + r.Use + ")"
	}
	return &Error{
		Key:  Key{Rule: r.ID, From: u.From.Module, To: r.Use, Symbol: u.To.Path},
		From: u.From.Module, To: r.Use,
		What:   fmt.Sprintf("%s uses %s, which only %s may use", label, used, joinNames(r.In)),
		Phrase: fmt.Sprintf("only %s may use %s", joinNames(r.In), r.Use),
		Why:    r.Why, Instead: r.Instead, Next: nextOf(resolved.ModeDeny),
	}
}

// languages are the languages an only rule reads a package set for when the
// importer's language is unknown.
var languages = []resolved.Language{
	resolved.LanguageGo, resolved.LanguageTypeScript, resolved.LanguagePython,
	resolved.LanguageRust, resolved.LanguageDotNet,
}

// uses reports whether importPath is what an only rule names in use. With no
// language, a package set matches the entries of every language: an unknown
// language must not hide a use.
func uses(use string, lang resolved.Language, importPath string) bool {
	if lang != "" {
		return resolved.Uses(use, lang, importPath)
	}
	return slices.ContainsFunc(languages, func(l resolved.Language) bool {
		return resolved.Uses(use, l, importPath)
	})
}

// firstMatch returns the first name that an end matches: a module name the
// end is in, or a path glob that covers the end's path.
func firstMatch(p resolved.Policy, names []string, e End) (string, bool) {
	for _, name := range names {
		if _, isModule := p.Modules[name]; isModule {
			if e.Module == name {
				return name, true
			}
			continue
		}
		if covers(p, name, e) {
			return name, true
		}
	}
	return "", false
}

// symbolOf is what a use reaches: its path, or the module named as the target.
func symbolOf(e End) string {
	if e.Path != "" {
		return e.Path
	}
	return e.Module
}

// nextOf is the next step of an error of a rule in a mode: ask names the
// request command, deny says fix the code.
func nextOf(mode resolved.Mode) result.Next {
	if mode == resolved.ModeAsk {
		return result.Next{Kind: result.NextCommand, Text: nextRequestText, Note: nextRequestNote}
	}
	return result.Next{Kind: result.NextFix, Text: nextFixText}
}

// joinNames writes names as "a", "a and b", or "a, b and c".
func joinNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
