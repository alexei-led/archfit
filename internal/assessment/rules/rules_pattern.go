package rules

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// ---------------------------------------------------------------------------
// ForbiddenPattern
// ---------------------------------------------------------------------------

// edgeKindPatternMatch is the finding edge kind of a pattern match: its subject
// is a construct inside one file, not a dependency between two nodes.
const edgeKindPatternMatch = "pattern_match"

// matchedByPattern is the MatchedBy key naming the matched pattern ID.
const matchedByPattern = "pattern"

// validateForbiddenPatternDef checks a forbidden_pattern rule. It needs at least
// one pattern, takes no to: (its subject is a file, not an edge), and every one
// of its pattern IDs must be unique across ALL rules' patterns: acquisition runs
// every rule's patterns through one ast-grep pass and a match carries only the
// pattern ID, so a shared ID would attribute another rule's matches to this one.
func validateForbiddenPatternDef(def policy.RuleDef, all []policy.RuleDef) error {
	if len(def.Patterns) == 0 {
		return fmt.Errorf("rules: forbidden_pattern %q requires at least one patterns entry", def.ID)
	}
	if def.To != "" {
		return fmt.Errorf("rules: forbidden_pattern %q takes no to: it scopes source files with from", def.ID)
	}
	if err := validateScopeGlobs(def); err != nil {
		return err
	}
	for _, p := range def.Patterns {
		var owners []string
		for _, r := range all {
			for _, q := range r.Patterns {
				if q.ID == p.ID {
					owners = append(owners, r.ID)
				}
			}
		}
		if len(owners) > 1 {
			return fmt.Errorf("rules: forbidden_pattern %q pattern id %q is declared %d times (rules: %s); pattern ids must be unique across rules",
				def.ID, p.ID, len(owners), strings.Join(owners, ", "))
		}
	}
	return nil
}

// forbiddenPattern fires on ast-grep matches of its own patterns in production
// source files under its from: scope. The matches are acquired for every rule's
// patterns; this is the only rule type that reads them.
//
// One finding per (pattern, file, match text with all whitespace removed),
// keyed on exactly that: the line stays out of the key, so a moved or
// reformatted match keeps its ID, and every site of the same text in one file is
// one finding with all its locations. The match text is source code, so it reaches the key's hash and
// nothing else — never why, constraint, or matched_by.
type forbiddenPattern struct {
	def policy.RuleDef
	mm  policy.ModuleMap
	ids map[string]struct{}
}

func newForbiddenPattern(def policy.RuleDef, mm policy.ModuleMap) *forbiddenPattern {
	ids := make(map[string]struct{}, len(def.Patterns))
	for _, p := range def.Patterns {
		ids[p.ID] = struct{}{}
	}
	return &forbiddenPattern{def: def, mm: mm, ids: ids}
}

func (r *forbiddenPattern) ID() string { return r.def.ID }

func (r *forbiddenPattern) Check(_ relationship.Set, ev Evidence) []finding.Finding {
	type key struct{ pattern, file, text string }
	sites := make(map[key][]relationship.Location)
	for _, m := range ev.PatternMatches {
		if _, own := r.ids[m.Pattern]; !own {
			continue
		}
		if class, inventoried := ev.FileClasses[m.File]; !inventoried || !fileclass.IsProduction(class) {
			continue
		}
		if !r.inScope(m.File) {
			continue
		}
		k := key{pattern: m.Pattern, file: m.File, text: strings.Join(strings.Fields(m.Text), "")}
		sites[k] = append(sites[k], relationship.Location{File: m.File, Line: m.Line})
	}
	keys := make([]key, 0, len(sites))
	for k := range sites {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].file != keys[b].file {
			return keys[a].file < keys[b].file
		}
		if keys[a].pattern != keys[b].pattern {
			return keys[a].pattern < keys[b].pattern
		}
		return keys[a].text < keys[b].text
	})

	out := make([]finding.Finding, 0, len(keys))
	for _, k := range keys {
		locs, total := sortedCappedLocations(sites[k])
		module, _ := r.mm.ModuleForFile(k.file)
		f := finding.NewKeyed(r.def.ID, edgeKindPatternMatch, k.pattern, k.file, k.text)
		f.Severity = finding.SeverityHigh
		f.Edge.From = finding.Endpoint{Module: module, Path: k.file}
		f.Locations = locs
		f.MatchedBy = map[string]string{
			matchedByPattern:        k.pattern,
			matchedByFile:           k.file,
			matchedByLocationsTotal: strconv.Itoa(total),
		}
		f.Why = fmt.Sprintf("%s:%d matches forbidden pattern %q", k.file, locs[0].Line, k.pattern)
		if total > 1 {
			f.Why += fmt.Sprintf(" (+%d more in this file)", total-1)
		}
		scope := "production code"
		if r.def.From != "" {
			scope = "code under " + r.def.From
		}
		f.Constraint = fmt.Sprintf("Remove the construct matching pattern %q: %s must not contain it", k.pattern, scope)
		out = append(out, f)
	}
	return out
}

// inScope reports whether the rule's from: glob covers file, matched against
// the repo-relative path or the file's language-convention selector (a dotted
// Python module, a Rust module key) — the same matcher the rule's producer
// scope uses, so a from: glob that scopes as applicable can also match here.
func (r *forbiddenPattern) inScope(file string) bool {
	if r.def.From == "" {
		return true
	}
	if matched, _ := doublestar.Match(r.def.From, file); matched {
		return true
	}
	_, selector, ok := r.mm.RuleSelectorForFile(file)
	if !ok || selector == "" || selector == file {
		return false
	}
	matched, _ := doublestar.Match(r.def.From, selector)
	return matched
}
