// Package copies is the duplication rule: knowledge that another module
// already owns is not copied (design 6.2, 7.3). In 4.0 the rule finds one kind
// of copy, an identical string literal that production code of two modules
// holds.
//
// Find applies rule R12, measured in milestone 0 (design appendix D.4), to the
// literals of one tree: a value of at least 12 runes; declared tests and
// generated paths out; single words and struct tags out, as the extractor
// shaped them; held by fewer than 3 modules. Code in no module is the unowned
// rule's case and holds nothing here. The measured precision is about 80% on
// the dogfood repositories, and a re-typed key of the repository's own model
// counts as a copy.
//
// One error per key: the rule, the sorted module pair, and the digest of the
// literal. Its sites are every occurrence in the two modules, so one more
// occurrence is a higher count and a moved one is not.
//
// Added keeps the errors a change adds. R12 counts holders per tree, so a key
// can appear without a new copy: deleting the third holder of a literal leaves
// two. A key is added only when the head has more sites than the base and one
// head site is on an added line. Known limit: a change that deletes the third
// holder and also edits a line of a remaining holder that holds the literal
// reports a copy nobody made. Telling the two apart needs the base's holders
// of literals R12 drops; until that is measured as noise, the error is
// answered like any other.
//
// Not found: a copy in other words, a table of values, a literal across lines.
package copies

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// minRunes is R12's length bound: a shorter literal is too common to be
// knowledge that one module owns.
const minRunes = 12

const why = "two copies must change together and nothing checks that they do"

// site is an occurrence of a literal in production code of a module.
type site struct {
	change.Site
	module string
	added  bool
}

// Find returns the duplication errors of one tree, one per key, in record
// order. lits are the literals of the tree, p the resolved policy. added are
// the lines the change adds to the tree: the head's, or nil for the base.
// expires is the date the request in next proposes; the caller picks it.
// Find returns nil when the policy turns the rule off.
func Find(lits []evidence.LiteralOccurrence, p resolved.Policy, added evidence.AddedLines, expires time.Time) []change.Error {
	mode := p.Builtin.Duplication
	if mode == resolved.ModeOff {
		return nil
	}
	byValue := map[string][]site{}
	for _, l := range lits {
		if utf8.RuneCountInString(l.Value) < minRunes || l.Shape == evidence.ShapeWord || l.Shape == evidence.ShapeTag {
			continue
		}
		module, ok := p.ModuleOf(l.File)
		if !ok || p.IsTest(l.File) || p.IsGenerated(l.File) {
			continue
		}
		byValue[l.Value] = append(byValue[l.Value], site{
			File:   l.File,
			Line:   l.Line,
			module: module,
			added:  onAddedLine(added, l.File, l.Line),
		})
	}
	var errs []change.Error
	for value, sites := range byValue {
		holders := map[string]bool{}
		for _, s := range sites {
			holders[s.module] = true
		}
		// R12: a literal three or more modules hold is shared vocabulary,
		// not one module's knowledge.
		if len(holders) != 2 {
			continue
		}
		errs = append(errs, record(value, slices.Sorted(maps.Keys(holders)), sites, mode, expires))
	}
	slices.SortFunc(errs, func(a, b change.Error) int {
		return cmp.Or(
			cmp.Compare(a.Sites[0].File, b.Sites[0].File),
			cmp.Compare(a.Sites[0].Line, b.Sites[0].Line),
			cmp.Compare(a.Key.Symbol, b.Key.Symbol),
		)
	})
	return errs
}

// Added returns the errors of head that the change adds, in head order: a key
// absent on the base or with more sites on the head, with a head site on an
// added line. base and head are what Find returns for the base and the head,
// and added are the head's added lines.
func Added(base, head []change.Error, added evidence.AddedLines) []change.Error {
	onBase := make(map[change.Key]int, len(base))
	for _, e := range base {
		onBase[e.Key] = len(e.Sites)
	}
	var out []change.Error
	for _, e := range head {
		if len(e.Sites) > onBase[e.Key] && slices.ContainsFunc(e.Sites, func(s change.Site) bool {
			return onAddedLine(added, s.File, s.Line)
		}) {
			out = append(out, e)
		}
	}
	return out
}

// record builds the error of one copy. It starts in the module of the first
// added site in file and line order, else in the first module of the pair;
// that site comes first and the others follow in file and line order. what
// names the first site of the other module.
func record(value string, pair []string, sites []site, mode resolved.Mode, expires time.Time) change.Error {
	slices.SortFunc(sites, func(a, b site) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	from, to := pair[0], pair[1]
	shown := slices.IndexFunc(sites, func(s site) bool { return s.added })
	if shown >= 0 && sites[shown].module == to {
		from, to = to, from
	}
	if shown < 0 {
		shown = slices.IndexFunc(sites, func(s site) bool { return s.module == from })
	}
	other := sites[slices.IndexFunc(sites, func(s site) bool { return s.module == to })]
	ordered := make([]change.Site, 0, len(sites))
	ordered = append(ordered, sites[shown].Site)
	for i, s := range sites {
		if i != shown {
			ordered = append(ordered, s.Site)
		}
	}
	return change.Error{
		Key:   change.Key{Rule: resolved.RuleDuplication, From: pair[0], To: pair[1], Symbol: digest(value)},
		From:  from,
		To:    to,
		Sites: ordered,
		What:  fmt.Sprintf("this literal already lives in %s:%d", other.File, other.Line),
		Why:   why,
		Next:  next(mode, to, expires),
	}
}

// next is the step of a copy: in ask mode, use the module that owns the
// literal or request the copy; in deny mode, fix the code.
func next(mode resolved.Mode, owner string, expires time.Time) result.Next {
	if mode == resolved.ModeDeny {
		return result.Next{Kind: result.NextFix, Text: "fix the code"}
	}
	return result.Next{
		Kind: result.NextCommand,
		Text: fmt.Sprintf(`use %s, or run: archfit request -m "<why a copy is needed>" --expires %s`,
			owner, expires.Format(time.DateOnly)),
	}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// onAddedLine reports whether the change adds line of file. added holds
// ascending lines per file.
func onAddedLine(added evidence.AddedLines, file string, line int) bool {
	_, ok := slices.BinarySearch(added[file], line)
	return ok
}
