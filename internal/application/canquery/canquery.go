// Package canquery answers `archfit can <from> <to>` (design 8.1, 10.2 and
// 11.1): may this dependency or library be used. It judges the dependency
// with change.Judge, the judgment `archfit check` applies to every import, so
// `can` and `check` give the same answer for the same import.
//
// # Arguments
//
// Each argument is read in this order:
//
//  1. A module name of the policy. As the target it means the module's
//     public part.
//  2. A file or a directory of the checkout (Tree). A directory is the
//     package directory change.DirEnd makes, as check makes it.
//  3. A Go import path inside a Go module of the checkout (GoModules): the
//     package directory it names.
//  4. Anything else is, for the importer, a path of a file not written yet,
//     and, for the target, a library. A target stays a path of the tree when
//     it is written ./path, or when its first segment, up to a / or a ., is
//     an entry of the repository root: a mistyped pkg/utl, or a dotted
//     Python name app.core, is then in no module, not a library.
//
// A path not in the tree is a file when it has an extension. A Python or
// TypeScript module is named by its file or its directory: a dotted name is
// not resolved.
//
// # The answer
//
//   - yes: no rule breaks, with the balance when the two ends are different
//     modules. An importer that the policy declares a test or generated file
//     is out of every rule, as in check: yes, with no balance.
//   - no: a rule the dependency breaks, with its phrase, instead, balance and
//     next as check gives them. When it breaks more than one, the answer
//     names one: one with instead first (the owner has answered), then one
//     that says to fix the code (a request cannot clear it), then the first
//     in the judge's order. As design 8.1 shows, the answer leaves out a
//     built-in rule's why and the note of its next step; a custom rule's why
//     is the owner's and stays.
//   - unknown: a path in no module, on either side; or an answer that needs a
//     file where the argument names more than one. Name a file then.
//
// An importer directory is judged as check judges it, one file at a time:
// when its files answer differently, the answer is unknown.
//
// A module name has no path, so two rules cannot tell from it alone. A deny
// or only rule that names a path glob which may cover a path of the module,
// and an only rule that names a package set or a library the target is
// below, which reads the importer's language: when such a rule decides the
// answer, it is unknown.
//
// # Limits
//
// `can` applies no exception and does not find a cycle the dependency would
// close. `check` judges both.
//
// The package reads the checkout through its ports and imports no adapter:
// cmd/archfit wires them.
package canquery

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// PolicySource gives the local policy, resolved with its defaults.
type PolicySource interface {
	Policy(ctx context.Context) (resolved.Policy, error)
}

// Kind is what a path of the checkout is.
type Kind int

// The kinds of path.
const (
	// KindNone: nothing is at the path.
	KindNone Kind = iota
	// KindFile: the path is a file.
	KindFile
	// KindDir: the path is a directory.
	KindDir
)

// Entry is what is at a path of the checkout.
type Entry struct {
	Kind Kind
	// Files are, for a directory, the source files directly in it, declared
	// tests and generated files included: the files of the directory that
	// evidence.CheckFacts.Sources lists for the same tree.
	Files []string
}

// Tree reads the checkout.
type Tree interface {
	// Entry returns what is at a repository-relative, slash-separated path.
	Entry(ctx context.Context, path string) (Entry, error)
}

// GoModules maps a Go import path through the go.mod files of the checkout.
type GoModules interface {
	// Dir returns the repository-relative, slash-separated directory of the
	// package an import path names ("." for a module at the root), and false
	// when the import path is in no Go module of the checkout.
	Dir(ctx context.Context, importPath string) (dir string, ok bool, err error)
}

// Stamp is what an answer records about the run. The caller reads it; the
// answer copies it.
type Stamp struct {
	// PolicyPath is the policy file, relative to the repository root.
	PolicyPath string
	// Protected is true when a workflow runs the check on pull requests.
	Protected bool
	// Head is the HEAD commit. `archfit request` uses an answer only while
	// HEAD is the same.
	Head string
	// Date is the date of the answer, YYYY-MM-DD.
	Date string
	// Engine is the version of the archfit build.
	Engine string
}

// Question is one `archfit can` question.
type Question struct {
	// From and To are the arguments as given.
	From, To string
	Stamp    Stamp
}

// Service answers Questions. Every port is required.
type Service struct {
	Policies PolicySource
	Tree     Tree
	Go       GoModules
}

// ErrUsage marks a question without both arguments.
var ErrUsage = errors.New("usage")

// The texts of an unknown answer.
const (
	whatNoModule = "%s is in no module"
	whatFiles    = "the files of %s answer differently"
	whatLanguage = "rule %s reads %s by the language of the importer, and %s has none"
	whatPaths    = "rule %s names paths, and %s is a module"
	nextRules    = "archfit rules %s"
	nextFromFile = "archfit can <a file of %s> %s"
	nextToFile   = "archfit can %s <a file of %s>"
)

// Can answers q. An error of a port comes back as it is, with no answer.
func (s Service) Can(ctx context.Context, q Question) (result.CanAnswer, error) {
	if q.From == "" || q.To == "" {
		return result.CanAnswer{}, fmt.Errorf("%w: name a file, directory or module, then a file, directory, module or library", ErrUsage)
	}
	p, err := s.Policies.Policy(ctx)
	if err != nil {
		return result.CanAnswer{}, err
	}
	from, err := s.side(ctx, p, q.From, false)
	if err != nil {
		return result.CanAnswer{}, err
	}
	to, err := s.side(ctx, p, q.To, true)
	if err != nil {
		return result.CanAnswer{}, err
	}
	a := result.CanAnswer{
		Schema: result.CanSchemaVersion, From: from.label(), To: to.label(),
		Policy: result.Policy{Path: q.Stamp.PolicyPath, Digest: p.Digest, Protected: q.Stamp.Protected},
		Head:   q.Stamp.Head, Date: q.Stamp.Date, Engine: q.Stamp.Engine,
	}
	if from.end.Path == "" || from.end.IsFile || len(from.end.Files) == 0 {
		return judge(p, a, from, to), nil
	}
	// Check judges each file of a directory on its own.
	var first result.CanAnswer
	for i, file := range from.end.Files {
		got := judge(p, a, fileSide(p, from.arg, file), to)
		switch {
		case i == 0:
			first = got
		case !same(got, first):
			return unknown(a, fmt.Sprintf(whatFiles, from.arg), fmt.Sprintf(nextFromFile, from.arg, to.arg)), nil
		}
	}
	return first, nil
}

// judge answers for one importer: a file, a module, or a directory with no
// source file.
func judge(p resolved.Policy, a result.CanAnswer, from, to side) result.CanAnswer {
	if from.end.IsFile && (p.IsTest(from.end.Path) || p.IsGenerated(from.end.Path)) {
		a.Answer = result.ReplyYes
		return a
	}
	for _, sd := range []side{from, to} {
		if !sd.end.Library && sd.end.Module == "" {
			return unknown(a, fmt.Sprintf(whatNoModule, sd.arg), fmt.Sprintf(nextRules, sd.arg))
		}
	}
	use := change.Use{From: from.end, To: to.end, Lang: from.lang}
	errs := change.Judge(p, use)
	// Judge again with each path glob that may cover a module named on its
	// own taken to cover it. An error of one judgment and not the other
	// turns on a path the module name does not give.
	wide := change.Judge(widen(p, use), use)
	inErrs, inWide := keys(errs), keys(wide)
	var certain []change.Error
	for _, e := range errs {
		if inWide[e.Key] && !byLanguage(p, use, e) {
			certain = append(certain, e)
		}
	}
	if len(certain) > 0 {
		return no(a, pick(certain))
	}
	for _, e := range errs {
		if byLanguage(p, use, e) {
			return unknown(a, fmt.Sprintf(whatLanguage, e.Key.Rule, to.end.Path, from.arg),
				fmt.Sprintf(nextFromFile, from.arg, to.arg))
		}
	}
	for _, e := range slices.Concat(errs, wide) {
		if inErrs[e.Key] && inWide[e.Key] {
			continue
		}
		if from.end.Path == "" {
			return unknown(a, fmt.Sprintf(whatPaths, e.Key.Rule, from.arg), fmt.Sprintf(nextFromFile, from.arg, to.arg))
		}
		return unknown(a, fmt.Sprintf(whatPaths, e.Key.Rule, to.arg), fmt.Sprintf(nextToFile, from.arg, to.arg))
	}
	a.Answer = result.ReplyYes
	a.Balance = change.BalanceOf(p, use)
	return a
}

func keys(errs []change.Error) map[change.Key]bool {
	out := make(map[change.Key]bool, len(errs))
	for _, e := range errs {
		out[e.Key] = true
	}
	return out
}

// side is one argument, read.
type side struct {
	// arg is the argument as given.
	arg string
	end change.End
	// lang is the language whose package sets an only rule reads; empty for
	// a module name and for a file of no known language.
	lang resolved.Language
}

// label names a side in the answer: its module, else the library, else the
// argument as given.
func (sd side) label() string {
	switch {
	case sd.end.Module != "":
		return sd.end.Module
	case sd.end.Library:
		return sd.end.Path
	default:
		return sd.arg
	}
}

// side reads one argument in the order the package doc gives.
func (s Service) side(ctx context.Context, p resolved.Policy, arg string, target bool) (side, error) {
	if _, ok := p.Modules[arg]; ok {
		return side{arg: arg, end: change.End{Module: arg}}, nil
	}
	rel := path.Clean(filepath.ToSlash(arg))
	entry, err := s.Tree.Entry(ctx, rel)
	if err != nil {
		return side{}, err
	}
	switch entry.Kind {
	case KindFile:
		return fileSide(p, arg, rel), nil
	case KindDir:
		return side{arg: arg, end: change.DirEnd(p, rel, entry.Files)}, nil
	}
	dir, ok, err := s.Go.Dir(ctx, arg)
	if err != nil {
		return side{}, err
	}
	if ok {
		pkg, err := s.Tree.Entry(ctx, dir)
		if err != nil {
			return side{}, err
		}
		return side{arg: arg, end: change.DirEnd(p, dir, pkg.Files), lang: resolved.LanguageGo}, nil
	}
	if target {
		inTree, err := s.pathOfTree(ctx, arg, rel)
		if err != nil {
			return side{}, err
		}
		if !inTree {
			return side{arg: arg, end: change.End{Path: rel, Library: true}}, nil
		}
	}
	sd := fileSide(p, arg, rel)
	sd.end.IsFile = path.Ext(rel) != ""
	return sd, nil
}

// fileSide is a file of the tree, or one not written yet; arg names it in an
// answer.
func fileSide(p resolved.Policy, arg, file string) side {
	m, _ := p.ModuleOf(file)
	return side{arg: arg, end: change.End{Path: file, IsFile: true, Module: m}, lang: change.LanguageOf(file)}
}

// pathOfTree reports whether a target that names nothing in the tree is a
// path of the tree all the same: it is written ./path, it leaves the root, or
// its first segment, up to a / or a ., is an entry of the root.
func (s Service) pathOfTree(ctx context.Context, arg, rel string) (bool, error) {
	if strings.HasPrefix(filepath.ToSlash(arg), "./") || path.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return true, nil
	}
	first, _, nested := strings.Cut(rel, "/")
	if dotted, _, ok := strings.Cut(first, "."); ok {
		first, nested = dotted, true
	}
	if !nested || first == "" {
		return false, nil
	}
	e, err := s.Tree.Entry(ctx, first)
	return e.Kind != KindNone, err
}

// widen returns p with each end of u that is a module named on its own added
// to the deny and only names on its side that are path globs which may cover
// a path of that module. Judge matches a path glob against a path, and a
// module name has none.
func widen(p resolved.Policy, u change.Use) resolved.Policy {
	q := p
	q.Rules = slices.Clone(p.Rules)
	for i, r := range q.Rules {
		switch r.Form {
		case resolved.FormDeny:
			r.From = widenNames(p, r.From, u.From)
			r.To = widenNames(p, r.To, u.To)
		case resolved.FormOnly:
			r.In = widenNames(p, r.In, u.From)
		}
		q.Rules[i] = r
	}
	return q
}

// widenNames adds the module of e to names when e is a module named on its
// own and a path glob of names may cover one of the module's paths.
func widenNames(p resolved.Policy, names []string, e change.End) []string {
	if e.Path != "" || e.Library || slices.Contains(names, e.Module) {
		return names
	}
	paths := p.Modules[e.Module].Paths
	for _, name := range names {
		if _, isModule := p.Modules[name]; isModule {
			continue
		}
		if slices.ContainsFunc(paths, func(glob string) bool { return mayOverlap(name, glob) }) {
			return append(slices.Clone(names), e.Module)
		}
	}
	return names
}

// mayOverlap reports whether two path globs may match a common path. Each
// path a glob matches starts with the glob's part before its first
// wildcard, so when neither part starts the other, no path matches both. It
// can say yes for globs that share no path; it never says no for globs that
// do.
func mayOverlap(a, b string) bool {
	la, lb := literal(a), literal(b)
	return strings.HasPrefix(la, lb) || strings.HasPrefix(lb, la)
}

// literal is a glob's part before its first wildcard.
func literal(glob string) string {
	if i := strings.IndexAny(glob, "*?[{"); i >= 0 {
		return glob[:i]
	}
	return glob
}

// byLanguage reports whether an error of the judge depends on the
// importer's language that the use does not have. A file always has the
// language check reads for it, known or not; a module name and a directory
// with no source file have none. Judge then reads an only rule for every
// language, which can only add errors. The answer does not depend on the
// language when the target is the rule's library itself; it does for a
// package set, whose entries differ by language, and for a path below a
// library, whose separator does.
func byLanguage(p resolved.Policy, u change.Use, e change.Error) bool {
	if u.Lang != "" || u.From.IsFile || e.Key.Symbol == e.Key.To {
		return false
	}
	return slices.ContainsFunc(p.Rules, func(r resolved.Rule) bool { return r.ID == e.Key.Rule && r.Form == resolved.FormOnly })
}

// pick returns the error the answer names: one with instead, then one that
// says to fix the code, then the first; ties keep the judge's order.
func pick(errs []change.Error) change.Error {
	rank := func(e change.Error) int {
		switch {
		case e.Instead != "":
			return 0
		case e.Next.Kind == result.NextFix:
			return 1
		default:
			return 2
		}
	}
	return slices.MinFunc(errs, func(a, b change.Error) int { return cmp.Compare(rank(a), rank(b)) })
}

// no is the answer for one error. Design 8.1 prints a built-in rule's answer
// without its generic why and without the note of its next step; a custom
// rule's why is the owner's.
func no(a result.CanAnswer, e change.Error) result.CanAnswer {
	a.Answer = result.ReplyNo
	a.Rule = e.Key.Rule
	a.What = e.Phrase
	a.Instead = e.Instead
	a.Balance = e.Balance
	next := e.Next
	if slices.Contains(resolved.BuiltinRules, e.Key.Rule) {
		next.Note = ""
	} else {
		a.Why = e.Why
	}
	a.Next = &next
	return a
}

func unknown(a result.CanAnswer, what, command string) result.CanAnswer {
	a.Answer = result.ReplyUnknown
	a.What = what
	a.Next = &result.Next{Kind: result.NextCommand, Text: command}
	return a
}

// same reports whether two answers for one question say the same thing.
func same(x, y result.CanAnswer) bool {
	return x.Answer == y.Answer && x.Rule == y.Rule && x.What == y.What && x.Why == y.Why &&
		x.Instead == y.Instead && equal(x.Balance, y.Balance) && equal(x.Next, y.Next)
}

func equal[T comparable](x, y *T) bool {
	return x == nil && y == nil || x != nil && y != nil && *x == *y
}
