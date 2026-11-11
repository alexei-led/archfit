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
//     and, for the target, a library. A target whose first segment is an
//     entry of the repository root stays a path of the tree, so a mistyped
//     pkg/utl is in no module and not a library.
//
// A path not in the tree is a file when it has an extension.
//
// # The answer
//
//   - unknown: a path in no module, on either side. Or a rule whose answer
//     depends on the importer's language when the importer has none (a
//     module name, or a directory of several languages): an only rule that
//     names a package set, or a library the target is below. Name a file.
//   - yes: no rule breaks, with the balance when the two ends are different
//     modules.
//   - no: a rule the dependency breaks, with its what, instead, balance and
//     next as check gives them. When it breaks more than one, the answer
//     names one: one with instead first (the owner has answered), then one
//     that says to fix the code (a request cannot clear it), then the first
//     in the judge's order. As design 8.1 shows, the answer leaves out a
//     built-in rule's why and the note of its next step; a custom rule's why
//     is the owner's and stays.
//
// # Limits
//
// A module name matches only rule names that are modules. A deny or only
// rule that names a path glob matches a file or a directory, never a module
// name: to check such a rule, name a file or a directory.
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

// The texts of an unknown answer and the next step after a no.
const (
	whatNoModule = "%s is in no module"
	whatLanguage = "rule %s reads %s by the language of the importer: name a file of %s"
	nextRules    = "archfit rules %s"
	nextNameFile = "archfit can <a file of %s> %s"
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
	for _, sd := range []side{from, to} {
		if !sd.end.Library && sd.end.Module == "" {
			return unknown(a, fmt.Sprintf(whatNoModule, sd.arg), fmt.Sprintf(nextRules, sd.arg)), nil
		}
	}
	use := change.Use{From: from.end, To: to.end, Lang: from.lang}
	errs := change.Judge(p, use)
	certain := slices.DeleteFunc(slices.Clone(errs), func(e change.Error) bool { return byLanguage(p, use, e) })
	switch {
	case len(certain) > 0:
		return no(a, pick(certain)), nil
	case len(errs) > 0:
		e := errs[0]
		return unknown(a, fmt.Sprintf(whatLanguage, e.Key.Rule, to.end.Path, a.From),
			fmt.Sprintf(nextNameFile, a.From, q.To)), nil
	}
	a.Answer = result.ReplyYes
	a.Balance = change.BalanceOf(p, use)
	return a, nil
}

// side is one argument, read.
type side struct {
	// arg is the argument as given.
	arg string
	end change.End
	// lang is the language of the end's files; empty when it has none or
	// several.
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
		return dirSide(p, arg, rel, entry.Files), nil
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
		sd := dirSide(p, arg, dir, pkg.Files)
		sd.lang = resolved.LanguageGo
		return sd, nil
	}
	if target {
		inTree, err := s.pathOfTree(ctx, rel)
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

func fileSide(p resolved.Policy, arg, file string) side {
	m, _ := p.ModuleOf(file)
	return side{arg: arg, end: change.End{Path: file, IsFile: true, Module: m}, lang: change.LanguageOf(file)}
}

func dirSide(p resolved.Policy, arg, dir string, files []string) side {
	end := change.DirEnd(p, dir, files)
	return side{arg: arg, end: end, lang: languageOfFiles(end.Files)}
}

// languageOfFiles is the one language of files, or "" when they have none
// or more than one.
func languageOfFiles(files []string) resolved.Language {
	var lang resolved.Language
	for i, file := range files {
		switch l := change.LanguageOf(file); {
		case i == 0:
			lang = l
		case l != lang:
			return ""
		}
	}
	return lang
}

// pathOfTree reports whether a target that names nothing in the tree is a
// path of the tree all the same: it leaves the root, or its first segment is
// an entry of the root.
func (s Service) pathOfTree(ctx context.Context, rel string) (bool, error) {
	if path.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return true, nil
	}
	first, _, nested := strings.Cut(rel, "/")
	if !nested {
		return false, nil
	}
	e, err := s.Tree.Entry(ctx, first)
	return e.Kind != KindNone, err
}

// byLanguage reports whether an error of the judge depends on the
// importer's language that the use does not have. Judge reads an only rule
// for every language when the language is unknown, which can only add
// errors. The answer does not depend on the language when the target is the
// rule's library itself; it does for a package set, whose entries differ by
// language, and for a path below a library, whose separator does.
func byLanguage(p resolved.Policy, u change.Use, e change.Error) bool {
	if u.Lang != "" || e.Key.Symbol == e.Key.To {
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
