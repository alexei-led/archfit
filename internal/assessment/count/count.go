// Package count gives a change its status: it judges the merge base and the
// head with the same policy, counts the errors of each by key, applies the
// policy's exceptions on a date, and reads what neither tree could read
// (design 6.4, 7.1, 7.2).
//
// Added means more. A key the base does not have, or one with more sites on
// the head, is added by the change; moved code and a renamed file keep their
// keys and counts, so they are existing. Two rules need an added line as well:
// duplication (copies.Added, because R12 counts holders per tree) and unowned
// (its key is the file, so a pure rename, which adds no line, gives a new key
// without new code).
//
// An exception holds through its expires date. While it holds it removes the
// errors it matches from both trees before they are counted. After that date,
// while the head still breaks its rule between its from and to, it is an
// error of every change; FromChange says whether this change added to it.
//
// The status, in this order:
//
//  1. unknown, with the reasons and no error, when the comparison cannot be
//     made at all: the base cannot be read. (The policy checks and the merge
//     base are the caller's: it does not call Assess without them.)
//  2. fail, with the errors and any unknown reasons, when the change adds an
//     error or an exception has expired: the worker must fix that anyway.
//  3. unknown, with the reasons, when an unknown item qualifies: a tool is
//     missing; an item touches a file the change adds lines to; or the head
//     has an item the base does not have (an unchanged file whose import no
//     longer resolves, for example after a change to tsconfig paths).
//  4. pass.
//
// The engine never sets waiting: only the App knows the open requests.
package count

import (
	"cmp"
	"regexp"
	"slices"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/copies"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// RuleException is the rule id of an exception past its date.
const RuleException = "exception"

// proposedDays is how far ahead the request a copy names proposes its
// exception to expire. Design 8.1 shows 2026-11-10 -> 2026-12-10.
const proposedDays = 30

const (
	whyExpired       = "an exception with a date fails every change after that date"
	nextExtendText   = "fix the code, or ask an owner to extend the exception"
	nextWaitText     = "wait for an owner: #"
	whyBaseUnread    = "the base cannot be read, so the change cannot be compared with it"
	whyTool          = "a tool the check needs is missing, so some files are not read"
	whyBaseChanged   = "the base cannot read a file the change touches"
	whyHeadChanged   = "the change touches a file the check cannot read"
	whyHeadNotOnBase = "the head cannot read what the base could, so the change may hide an error"
)

// requestNumber finds the request an exception's why names, such as "#431".
var requestNumber = regexp.MustCompile(`#([0-9]+)`)

// Input is what a change is judged on.
type Input struct {
	// Policy judges both trees; it has its defaults. The caller has checked
	// that it is the protected policy.
	Policy resolved.Policy
	// Base and Head are the check facts of the merge base and the head.
	Base, Head evidence.CheckFacts
	// Added are the lines the change adds to the head.
	Added evidence.AddedLines
	// Today is the date the exceptions are judged on. It is an input: the same
	// trees, policy, engine and date give the same verdict.
	Today time.Time
}

// Verdict is the status of one change and what decides it.
type Verdict struct {
	// Status is pass, fail or unknown.
	Status result.Status
	// Errors are the errors the change adds, then the expired exceptions in
	// policy order. Empty when the base cannot be read.
	Errors []Error
	// Unknown are the reasons the check could not read everything, by path
	// and line. With a fail status they are listed beside the errors.
	Unknown []result.Unknown
}

// Error is an error the change adds, or an exception past its date.
type Error struct {
	change.Error
	// FromChange is false for an expired exception the change did not add
	// to; true for every other error.
	FromChange bool
	// Exception is the expired exception the error reports, nil for a rule
	// error. Such an error has no sites: its place is the policy file.
	Exception *resolved.Exception
}

// Assess gives a change its status. See the package doc for the order.
func Assess(in Input) Verdict {
	if reasons := baseUnread(in.Base); len(reasons) > 0 {
		return Verdict{Status: result.StatusUnknown, Errors: []Error{}, Unknown: reasons}
	}
	p := in.Policy
	proposed := day(in.Today).AddDate(0, 0, proposedDays)
	base := judge(p, in.Base, nil, proposed)
	head := judge(p, in.Head, in.Added, proposed)
	var holding, expired []resolved.Exception
	for _, x := range p.Exceptions {
		if isExpired(x, in.Today) {
			expired = append(expired, x)
		} else {
			holding = append(holding, x)
		}
	}
	added := addedErrors(uncovered(p, holding, base), uncovered(p, holding, head), in.Added)
	errs := make([]Error, 0, len(added))
	addedKeys := map[change.Key]bool{}
	for _, e := range added {
		addedKeys[e.Key] = true
		errs = append(errs, Error{Error: e, FromChange: true})
	}
	for _, x := range expired {
		if err, ok := expiredError(p, x, head, addedKeys); ok {
			errs = append(errs, err)
		}
	}
	reasons := unknowns(in.Base.Unknown, in.Head.Unknown, in.Added)
	status := result.StatusPass
	switch {
	case len(errs) > 0:
		status = result.StatusFail
	case len(reasons) > 0:
		status = result.StatusUnknown
	}
	return Verdict{Status: status, Errors: errs, Unknown: reasons}
}

// judge returns the errors of one tree: every rule of change.Evaluate and
// the duplication rule.
func judge(p resolved.Policy, facts evidence.CheckFacts, added evidence.AddedLines, proposed time.Time) []change.Error {
	return slices.Concat(change.Evaluate(p, facts).Errors, copies.Find(facts.Literals, p, added, proposed))
}

// uncovered returns errs without those a holding exception matches.
func uncovered(p resolved.Policy, holding []resolved.Exception, errs []change.Error) []change.Error {
	return slices.DeleteFunc(slices.Clone(errs), func(e change.Error) bool {
		return slices.ContainsFunc(holding, func(x resolved.Exception) bool { return matches(p, x, e) })
	})
}

// matches reports whether an exception covers an error: the same rule, and
// the error's from and to modules. A duplication key holds its pair sorted,
// so the exception's pair matches in either order; an unowned exception's
// from is a path glob over the error's file.
func matches(p resolved.Policy, x resolved.Exception, e change.Error) bool {
	if x.Rule != e.Key.Rule {
		return false
	}
	switch x.Rule {
	case resolved.RuleUnowned:
		return p.Covers(x.From, e.Key.Symbol)
	case resolved.RuleDuplication:
		pair := []string{x.From, x.To}
		slices.Sort(pair)
		return pair[0] == e.Key.From && pair[1] == e.Key.To
	default:
		return x.From == e.Key.From && x.To == e.Key.To
	}
}

// addedErrors returns the errors of head the change adds, in record order:
// an error whose first site is on an added line first, then by file, line,
// rule and key. Each error's sites on added lines come first.
func addedErrors(base, head []change.Error, lines evidence.AddedLines) []change.Error {
	onBase := map[change.Key]int{}
	var dupBase, dupHead, out []change.Error
	for _, e := range base {
		if e.Key.Rule == resolved.RuleDuplication {
			dupBase = append(dupBase, e)
			continue
		}
		onBase[e.Key] = len(e.Sites)
	}
	for _, e := range head {
		more := len(e.Sites) > onBase[e.Key]
		switch e.Key.Rule {
		case resolved.RuleDuplication:
			dupHead = append(dupHead, e)
		case resolved.RuleUnowned:
			if more && len(lines[e.Key.Symbol]) > 0 {
				out = append(out, e)
			}
		default:
			if more {
				out = append(out, e)
			}
		}
	}
	out = append(out, copies.Added(dupBase, dupHead, lines)...)
	for i := range out {
		out[i].Sites = addedFirst(out[i].Sites, lines)
	}
	slices.SortFunc(out, func(a, b change.Error) int {
		return cmp.Or(
			cmp.Compare(rank(a.Sites[0], lines), rank(b.Sites[0], lines)),
			cmp.Compare(a.Sites[0].File, b.Sites[0].File),
			cmp.Compare(a.Sites[0].Line, b.Sites[0].Line),
			cmp.Compare(a.Key.Rule, b.Key.Rule),
			cmp.Compare(a.Key.From, b.Key.From),
			cmp.Compare(a.Key.To, b.Key.To),
			cmp.Compare(a.Key.Symbol, b.Key.Symbol),
		)
	})
	return out
}

// addedFirst returns sites with those on added lines first, each group in
// its existing order.
func addedFirst(sites []change.Site, lines evidence.AddedLines) []change.Site {
	out := slices.Clone(sites)
	slices.SortStableFunc(out, func(a, b change.Site) int { return cmp.Compare(rank(a, lines), rank(b, lines)) })
	return out
}

// rank is 0 for a site on an added line, else 1.
func rank(s change.Site, lines evidence.AddedLines) int {
	if _, ok := slices.BinarySearch(lines[s.File], s.Line); ok {
		return 0
	}
	return 1
}

// expiredError returns the error of an expired exception when the head still
// breaks its rule between its from and to. head holds every head error,
// covered or not: an expired exception fails while the code breaks the rule.
func expiredError(p resolved.Policy, x resolved.Exception, head []change.Error, added map[change.Key]bool) (Error, bool) {
	broken, fromChange := false, false
	for _, e := range head {
		if matches(p, x, e) {
			broken = true
			fromChange = fromChange || added[e.Key]
		}
	}
	if !broken {
		return Error{}, false
	}
	place := x.From
	if x.To != "" {
		place += " -> " + x.To
	}
	return Error{
		Error: change.Error{
			Key:  change.Key{Rule: RuleException, From: x.From, To: x.To, Symbol: x.Rule},
			From: x.From, To: x.To,
			What: x.Rule + " " + place + " expired on " + x.Expires.UTC().Format(time.DateOnly),
			Why:  whyExpired,
			Next: nextExpired(x),
		},
		FromChange: fromChange,
		Exception:  &x,
	}, true
}

// nextExpired is the step after an exception's date: wait for the request
// its why names, else fix the code or have it extended. Milestone 1 reads
// the request from the why; milestone 2 may add it to the exception.
func nextExpired(x resolved.Exception) result.Next {
	if m := requestNumber.FindStringSubmatch(x.Why); m != nil {
		return result.Next{Kind: result.NextWait, Text: nextWaitText + m[1]}
	}
	return result.Next{Kind: result.NextFix, Text: nextExtendText}
}

// isExpired reports whether today is past an exception's last day. Days are
// UTC calendar days, as resolved.FormatExpires counts them. No date never
// expires.
func isExpired(x resolved.Exception, today time.Time) bool {
	return !x.Expires.IsZero() && day(today).After(day(x.Expires))
}

// day is 00:00 UTC of the calendar day t falls on in UTC.
func day(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// baseUnread returns the reasons when the base cannot be read at all.
func baseUnread(base evidence.CheckFacts) []result.Unknown {
	var out []result.Unknown
	for _, u := range base.Unknown {
		if u.Cause == evidence.CauseBase {
			out = append(out, reason(u, result.CauseBase, whyBaseUnread))
		}
	}
	sortReasons(out)
	return out
}

// itemKey names an unknown item in a tree. An unchanged file has the same
// lines on the base and the head.
type itemKey struct {
	file, specifier string
	line            int
	cause           evidence.UnknownCause
}

func keyOf(u evidence.UnknownItem) itemKey {
	return itemKey{file: u.File, specifier: u.Specifier, line: u.Line, cause: u.Cause}
}

// unknowns returns the unknown items that stop a verdict: a missing tool on
// either tree, an item in a file the change adds lines to, and a head item
// the base does not have. Other base items are existing and change nothing.
func unknowns(base, head []evidence.UnknownItem, added evidence.AddedLines) []result.Unknown {
	changed := func(u evidence.UnknownItem) bool { return u.File != "" && len(added[u.File]) > 0 }
	out := []result.Unknown{}
	onBase := map[itemKey]bool{}
	for _, u := range base {
		onBase[keyOf(u)] = true
		switch {
		case u.Cause == evidence.CauseTool:
			out = append(out, reason(u, result.CauseTool, whyTool))
		case changed(u):
			out = append(out, reason(u, result.CauseBase, whyBaseChanged))
		}
	}
	for _, u := range head {
		switch {
		case u.Cause == evidence.CauseTool:
			out = append(out, reason(u, result.CauseTool, whyTool))
		case changed(u):
			out = append(out, reason(u, causeOf(u.Cause), whyHeadChanged))
		case !onBase[keyOf(u)]:
			out = append(out, reason(u, causeOf(u.Cause), whyHeadNotOnBase))
		}
	}
	sortReasons(out)
	return slices.Compact(out)
}

// causeOf maps an unknown item's cause to the result's.
func causeOf(c evidence.UnknownCause) result.Cause {
	switch c {
	case evidence.CauseTool:
		return result.CauseTool
	case evidence.CauseBase:
		return result.CauseBase
	default:
		return result.CauseImport
	}
}

func reason(u evidence.UnknownItem, cause result.Cause, why string) result.Unknown {
	return result.Unknown{
		Path: u.File, Line: u.Line, Cause: cause, What: u.Reason, Why: why,
		Next: result.Next{Kind: result.NextRestore, Text: u.Fix},
	}
}

func sortReasons(rs []result.Unknown) {
	slices.SortFunc(rs, func(a, b result.Unknown) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Cause, b.Cause), cmp.Compare(a.What, b.What), cmp.Compare(a.Why, b.Why),
		)
	})
}
