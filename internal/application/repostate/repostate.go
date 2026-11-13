// Package repostate gives the repository state for owners (design 7.1, 10.1
// and 10.6): the whole head tree judged with the rules and the exceptions of
// the policy on a date, with no base to compare with.
//
// The state is one of three:
//
//   - expired: an exception passed its date and the code still breaks its
//     rule. Every change fails until an owner extends it or the code is
//     fixed, so this state wins over drift.
//   - drift: an error exists that no exception matches.
//   - clean: no error outside the exceptions.
//
// Beside the state the answer lists the errors of each state, the exceptions
// within 14 days of their date that still cover an error, the rules nothing
// checks ("not checked"), the balance of each allowed dependency, and the
// errors that exist as the exceptions `archfit init` writes for them.
//
// The balance of an allowed dependency is the balance of a use of the
// target's public part, the number `archfit can <from> <to>` shows. It is not
// the lowest balance of the uses in the tree: a use past the public part that
// breaks coupling is an error of its own, listed with its balance.
//
// Errors and exceptions are matched by internal/assessment/count, the same
// matching the check applies to a change. The package reads the checkout
// through its ports and imports no adapter: cmd/archfit wires them.
package repostate

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// State is the state of the repository for owners.
type State string

// State values.
const (
	StateClean   State = "clean"
	StateDrift   State = "drift"
	StateExpired State = "expired"
)

// ExpiryWindow is how many days before its date an exception is listed:
// time for the owners to extend it or have the code fixed.
const ExpiryWindow = 14

// Repository is the state of the repository and what decides it.
type Repository struct {
	State State
	// Drift are the errors no exception matches, in record order, with
	// FromChange false.
	Drift []count.Error
	// Expired are the exceptions past their date that the head still
	// breaks, in policy order, with FromChange false. The errors they match
	// are not in Drift.
	Expired []count.Error
	// Expiring are the exceptions within ExpiryWindow days of their date
	// that still cover an error, in policy order.
	Expiring []Expiring
	// NotChecked are the note rules, in policy order.
	NotChecked []resolved.Rule
	// Balances are the allowed dependencies with their balance, by from
	// module, then to module.
	Balances []Balance
	// Existing are the errors of Drift as exceptions: one per rule and place,
	// with no date and no why, which `archfit init` writes. An error whose
	// importer is in no module has no place an exception can name and is
	// left out; the unowned error of its file names that file.
	Existing []resolved.Exception
	// Unknown are the places of the head the check could not read. The state
	// does not cover them: a missing tool leaves a whole language unread, and
	// the state can then be clean for code nothing judged. Missing evidence is
	// not zero, so a consumer that shows State shows Unknown beside it.
	Unknown []evidence.UnknownItem
}

// Expiring is an exception close to its date.
type Expiring struct {
	Exception resolved.Exception
	// Expires is the date with the days left: "2026-11-19 (9 days)".
	Expires string
}

// Balance is an allowed dependency and its balance.
type Balance struct {
	From, To string
	Balance  result.Balance
}

// Input is what the state is judged on.
type Input struct {
	// Policy has its defaults.
	Policy resolved.Policy
	// Head are the check facts of the whole head tree.
	Head evidence.CheckFacts
	// Today is the date the exceptions are judged on.
	Today time.Time
}

// Evaluate gives the repository state of in.Head.
func Evaluate(in Input) Repository {
	p := in.Policy
	s := count.Stand(p, in.Head, in.Today)
	r := Repository{
		State: StateClean, Drift: s.Drift, Expired: s.Expired,
		Expiring:   expiring(s.Holding, in.Today),
		NotChecked: notes(p),
		Balances:   balances(p),
		Existing:   existing(p, s.Drift),
		Unknown:    slices.Clone(in.Head.Unknown),
	}
	switch {
	case len(r.Expired) > 0:
		r.State = StateExpired
	case len(r.Drift) > 0:
		r.State = StateDrift
	}
	return r
}

// expiring returns the holding exceptions with at most ExpiryWindow days
// left. An exception holds through its date, so that day has 0 days left.
func expiring(holding []resolved.Exception, today time.Time) []Expiring {
	last := day(today).AddDate(0, 0, ExpiryWindow)
	out := []Expiring{}
	for _, x := range holding {
		if !x.Expires.IsZero() && !day(x.Expires).After(last) {
			out = append(out, Expiring{Exception: x, Expires: resolved.FormatExpires(x.Expires, today)})
		}
	}
	return out
}

// day is 00:00 UTC of the calendar day t falls on in UTC, as count and
// resolved.FormatExpires count days.
func day(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func notes(p resolved.Policy) []resolved.Rule {
	out := []resolved.Rule{}
	for _, r := range p.Rules {
		if r.Form == resolved.FormNote {
			out = append(out, r)
		}
	}
	return out
}

// balances returns the balance of a use of each allowed dependency's public
// part.
func balances(p resolved.Policy) []Balance {
	out := []Balance{}
	for _, from := range slices.Sorted(maps.Keys(p.Modules)) {
		// The policy file may repeat a depends_on entry; it is one allowed
		// dependency.
		for _, to := range slices.Compact(slices.Sorted(slices.Values(p.Modules[from].DependsOn))) {
			use := change.Use{From: change.End{Module: from}, To: change.End{Module: to}}
			if b := change.BalanceOf(p, use); b != nil {
				out = append(out, Balance{From: from, To: to, Balance: *b})
			}
		}
	}
	return out
}

// existing returns one exception for each rule and place of errs, as
// count matches them: duplication by its sorted pair, unowned by the file as
// a glob, a custom only rule by its from alone, every other rule by from and
// to. Order: the built-in rules, then the custom rules in policy order, then
// from and to.
func existing(p resolved.Policy, errs []count.Error) []resolved.Exception {
	seen := map[resolved.Exception]bool{}
	out := []resolved.Exception{}
	for _, e := range errs {
		x, ok := exceptionFor(p, e.Key)
		if ok && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	order := map[string]int{}
	for i, id := range resolved.BuiltinRules {
		order[id] = i
	}
	for i, r := range p.Rules {
		order[r.ID] = len(resolved.BuiltinRules) + i
	}
	slices.SortFunc(out, func(a, b resolved.Exception) int {
		return cmp.Or(cmp.Compare(order[a.Rule], order[b.Rule]), cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return out
}

// exceptionFor returns the exception that matches the errors of key, and
// false when no exception can name its place.
func exceptionFor(p resolved.Policy, k change.Key) (resolved.Exception, bool) {
	switch {
	case k.Rule == resolved.RuleUnowned:
		return resolved.Exception{Rule: k.Rule, From: literal(k.Symbol)}, k.Symbol != ""
	case k.From == "":
		return resolved.Exception{}, false
	case slices.ContainsFunc(p.Rules, func(r resolved.Rule) bool { return r.ID == k.Rule && r.Form == resolved.FormOnly }):
		return resolved.Exception{Rule: k.Rule, From: k.From}, true
	default:
		return resolved.Exception{Rule: k.Rule, From: k.From, To: k.To}, true
	}
}

// literal is a glob that matches exactly the path p.
func literal(p string) string {
	var b strings.Builder
	for _, r := range p {
		if strings.ContainsRune(`*?[]{}\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// PolicySource gives the local policy, resolved with its defaults.
type PolicySource interface {
	Policy(ctx context.Context) (resolved.Policy, error)
}

// Tree lists the files of the working tree, as a commit of every change
// would hold them: the subset of changecheck.Repository the state needs.
type Tree interface {
	// WorkingTree returns the git tree id of the working tree.
	WorkingTree(ctx context.Context) (string, error)
	// Files returns every file of the git tree id tree, repository-relative
	// and slash-separated.
	Files(ctx context.Context, tree string) ([]string, error)
}

// HeadFacts reads the check facts of the working tree of the repository at
// root, as a commit of every change would hold it.
type HeadFacts interface {
	Head(ctx context.Context, root string) (evidence.CheckFacts, error)
}

// Service gives the state of a checkout. Every port is required.
type Service struct {
	Policies PolicySource
	Tree     Tree
	Facts    HeadFacts
}

// State gives the repository state of the checkout at root on today. A
// policy that does not fit the tree gives no state: its error comes back,
// as does an error of a port. The policy is checked against every file of
// the working tree, as `archfit check` does, not against the files the
// extractors read: those leave test files out, and a language that is off
// or a missing tool would make a path look empty.
func (s Service) State(ctx context.Context, root string, today time.Time) (Repository, error) {
	p, err := s.Policies.Policy(ctx)
	if err != nil {
		return Repository{}, err
	}
	tree, err := s.Tree.WorkingTree(ctx)
	if err != nil {
		return Repository{}, err
	}
	files, err := s.Tree.Files(ctx, tree)
	if err != nil {
		return Repository{}, err
	}
	if err := p.CheckTree(files); err != nil {
		return Repository{}, err
	}
	head, err := s.Facts.Head(ctx, root)
	if err != nil {
		return Repository{}, err
	}
	return Evaluate(Input{Policy: p, Head: head, Today: today}), nil
}
