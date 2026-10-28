package decision

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

// PolicyChangeDirection classifies a policy change for the owner who approves
// it. A loosening always needs a human; a tightening may later be delegated.
type PolicyChangeDirection string

// Policy change directions.
const (
	PolicyNoChange   PolicyChangeDirection = "none"
	PolicyTightening PolicyChangeDirection = "tightening"
	PolicyLoosening  PolicyChangeDirection = "loosening"
	PolicyMixed      PolicyChangeDirection = "mixed"
)

// Coverage change kinds: what a configuration change does to the code the
// analysis reads.
const (
	CoverageExcludeAdded        = "exclude_added"
	CoverageExcludeRemoved      = "exclude_removed"
	CoverageLanguageDisabled    = "language_disabled"
	CoverageLanguageEnabled     = "language_enabled"
	CoverageAnalyzerDisabled    = "analyzer_disabled"
	CoverageAnalyzerEnabled     = "analyzer_enabled"
	CoverageModuleRemoved       = "module_removed"
	CoverageModuleAdded         = "module_added"
	CoverageModulePathsRemoved  = "module_paths_removed"
	CoverageModulePathsAdded    = "module_paths_added"
	CoverageModuleReviewLowered = "module_review_gate_lowered"
	CoverageModuleReviewRaised  = "module_review_gate_raised"
)

// fieldGate names a rule's gate in a RuleChange.
const fieldGate = "gate"

// Gate switch values, as the configuration spells them.
const (
	gateOffValue  = "off"
	gateWarnValue = "warn"
	gateFailValue = "fail"
)

// PolicyInput is one side of a policy comparison: the declarations that decide
// what blocks and what the analysis reads. The composition root builds it from
// a configuration; this package never reads a configuration file.
type PolicyInput struct {
	Rules   []policy.RuleDef
	Waivers []policy.WaiverDef
	Modules map[string]policy.ModuleDef
	// MetricGates maps a metric name to its gate (off, warn, fail; "" is off).
	MetricGates map[string]string
	Exclude     []string
	// Languages and Analyzers map an id to its enabled switch: auto, on, off.
	Languages map[string]string
	Analyzers map[string]string
	// ModuleReviewGate is module_review.gate (off, warn, fail; "" is warn).
	ModuleReviewGate string
}

// RuleRef names a rule and its effective gate.
type RuleRef struct {
	ID   string
	Type string
	Gate string
}

// RuleChange is one rule present on both sides whose declaration differs.
type RuleChange struct {
	ID string
	// Field is the declaration that changed: gate, max, selector, patterns.
	Field string
	From  string
	To    string
}

// AllowlistChange is one allowlist that admits more (widened) or fewer
// (narrowed) modules.
type AllowlistChange struct {
	Module string
	// Field is depends_on or visible_to.
	Field string
	// From and To render the list; "any" is an absent key, "none" an empty list.
	From string
	To   string
}

// CoverageChange is one declaration that changes what the analysis reads.
type CoverageChange struct {
	Kind   string
	Detail string
}

// PolicyChange is the classified difference between two policies.
type PolicyChange struct {
	Direction PolicyChangeDirection
	// Loosening: each can let a change pass that would fail today.
	RulesRemoved      []RuleRef
	RulesLoosened     []RuleChange
	WaiversAdded      []policy.WaiverDef
	AllowlistsWidened []AllowlistChange
	CoverageLost      []CoverageChange
	// Tightening: each can fail a change that passes today.
	RulesAdded         []RuleRef
	RulesTightened     []RuleChange
	WaiversRemoved     []policy.WaiverDef
	AllowlistsNarrowed []AllowlistChange
	CoverageGained     []CoverageChange
}

// ComparePolicies classifies the change from current to candidate. A
// declaration whose effect cannot be ordered (a rule's selector or patterns
// changed) counts as loosened: the safe reading for an approver, since a
// moved selector stops checking what it used to check.
func ComparePolicies(current, candidate PolicyInput) PolicyChange {
	var out PolicyChange
	compareRules(current, candidate, &out)
	compareWaivers(current.Waivers, candidate.Waivers, &out)
	compareAllowlists(current.Modules, candidate.Modules, &out)
	comparePolicyCoverage(current, candidate, &out)
	out.Direction = direction(out)
	return out
}

func direction(c PolicyChange) PolicyChangeDirection {
	loosens := len(c.RulesRemoved)+len(c.RulesLoosened)+len(c.WaiversAdded)+len(c.AllowlistsWidened)+len(c.CoverageLost) > 0
	tightens := len(c.RulesAdded)+len(c.RulesTightened)+len(c.WaiversRemoved)+len(c.AllowlistsNarrowed)+len(c.CoverageGained) > 0
	switch {
	case loosens && tightens:
		return PolicyMixed
	case loosens:
		return PolicyLoosening
	case tightens:
		return PolicyTightening
	default:
		return PolicyNoChange
	}
}

// effectiveGate is the gate a rule runs at: an absent gate blocks for every
// type except the advisory API rules (policy.RuleDef.Blocks).
func effectiveGate(r policy.RuleDef) string {
	switch {
	case r.Gate != "":
		return r.Gate
	case r.Blocks():
		return gateFailValue
	default:
		return gateWarnValue
	}
}

func gateRank(gate string) int {
	switch gate {
	case gateFailValue:
		return 2
	case gateWarnValue:
		return 1
	default:
		return 0
	}
}

func compareRules(current, candidate PolicyInput, out *PolicyChange) {
	cur := rulesByID(current.Rules)
	cand := rulesByID(candidate.Rules)
	for _, id := range sortedKeys(cur) {
		r, ok := cand[id]
		if !ok {
			out.RulesRemoved = append(out.RulesRemoved, RuleRef{ID: id, Type: cur[id].Type, Gate: effectiveGate(cur[id])})
			continue
		}
		compareRule(cur[id], r, out)
	}
	for _, id := range sortedKeys(cand) {
		if _, ok := cur[id]; !ok {
			out.RulesAdded = append(out.RulesAdded, RuleRef{ID: id, Type: cand[id].Type, Gate: effectiveGate(cand[id])})
		}
	}
	// Metric gates are ratchets: a lowered gate stops a regression blocking.
	for _, name := range unionKeys(current.MetricGates, candidate.MetricGates) {
		from, to := current.MetricGates[name], candidate.MetricGates[name]
		change := RuleChange{ID: "metric/" + name, Field: fieldGate, From: orOff(from), To: orOff(to)}
		switch {
		case gateRank(to) < gateRank(from):
			out.RulesLoosened = append(out.RulesLoosened, change)
		case gateRank(to) > gateRank(from):
			out.RulesTightened = append(out.RulesTightened, change)
		}
	}
}

func compareRule(cur, cand policy.RuleDef, out *PolicyChange) {
	from, to := effectiveGate(cur), effectiveGate(cand)
	switch {
	case gateRank(to) < gateRank(from):
		out.RulesLoosened = append(out.RulesLoosened, RuleChange{ID: cur.ID, Field: fieldGate, From: from, To: to})
	case gateRank(to) > gateRank(from):
		out.RulesTightened = append(out.RulesTightened, RuleChange{ID: cur.ID, Field: fieldGate, From: from, To: to})
	}
	if cur.Max != nil && cand.Max != nil && *cur.Max != *cand.Max {
		change := RuleChange{ID: cur.ID, Field: "max", From: strconv.Itoa(*cur.Max), To: strconv.Itoa(*cand.Max)}
		if *cand.Max > *cur.Max {
			out.RulesLoosened = append(out.RulesLoosened, change)
		} else {
			out.RulesTightened = append(out.RulesTightened, change)
		}
	} else if (cur.Max == nil) != (cand.Max == nil) {
		out.RulesLoosened = append(out.RulesLoosened, RuleChange{ID: cur.ID, Field: "max", From: maxString(cur.Max), To: maxString(cand.Max)})
	}
	selector := func(r policy.RuleDef) string {
		return fmt.Sprintf("type=%s from=%s to=%s from_module=%s to_module=%s threshold=%s",
			r.Type, r.From, r.To, r.FromModule, r.ToModule, maxString(r.Threshold))
	}
	if selector(cur) != selector(cand) {
		out.RulesLoosened = append(out.RulesLoosened, RuleChange{ID: cur.ID, Field: "selector", From: selector(cur), To: selector(cand)})
	}
	if !reflect.DeepEqual(cur.Patterns, cand.Patterns) {
		out.RulesLoosened = append(out.RulesLoosened, RuleChange{ID: cur.ID, Field: "patterns", From: fmt.Sprintf("%d patterns", len(cur.Patterns)), To: fmt.Sprintf("%d patterns", len(cand.Patterns))})
	}
}

func compareWaivers(current, candidate []policy.WaiverDef, out *PolicyChange) {
	for _, w := range candidate {
		if !slices.Contains(current, w) {
			out.WaiversAdded = append(out.WaiversAdded, w)
		}
	}
	for _, w := range current {
		if !slices.Contains(candidate, w) {
			out.WaiversRemoved = append(out.WaiversRemoved, w)
		}
	}
}

func compareAllowlists(current, candidate map[string]policy.ModuleDef, out *PolicyChange) {
	for _, name := range sortedKeys(current) {
		cand, ok := candidate[name]
		if !ok {
			continue // a removed module is lost coverage, reported there
		}
		cur := current[name]
		for _, field := range []struct {
			name     string
			from, to []string
		}{
			{"depends_on", cur.DependsOn, cand.DependsOn},
			{"visible_to", cur.VisibleTo, cand.VisibleTo},
		} {
			change := AllowlistChange{Module: name, Field: field.name, From: allowlistString(field.from), To: allowlistString(field.to)}
			if change.From == change.To {
				continue
			}
			switch {
			case admitsMore(field.to, field.from):
				out.AllowlistsNarrowed = append(out.AllowlistsNarrowed, change)
			default:
				// Admits more, or entries swapped: something new is admitted.
				out.AllowlistsWidened = append(out.AllowlistsWidened, change)
			}
		}
	}
}

// admitsMore reports whether allowlist to admits every module from admits, and
// more. nil admits every module; an empty list admits none.
func admitsMore(from, to []string) bool {
	if to == nil {
		return from != nil
	}
	if from == nil {
		return false
	}
	for _, entry := range from {
		if !slices.Contains(to, entry) {
			return false
		}
	}
	return len(to) > len(dedupe(from))
}

func comparePolicyCoverage(current, candidate PolicyInput, out *PolicyChange) {
	for _, glob := range candidate.Exclude {
		if !slices.Contains(current.Exclude, glob) {
			out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageExcludeAdded, Detail: glob})
		}
	}
	for _, glob := range current.Exclude {
		if !slices.Contains(candidate.Exclude, glob) {
			out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageExcludeRemoved, Detail: glob})
		}
	}
	switches(current.Languages, candidate.Languages, "languages.", CoverageLanguageDisabled, CoverageLanguageEnabled, out)
	switches(current.Analyzers, candidate.Analyzers, "analyzers.", CoverageAnalyzerDisabled, CoverageAnalyzerEnabled, out)
	for _, name := range sortedKeys(current.Modules) {
		cand, ok := candidate.Modules[name]
		if !ok {
			out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageModuleRemoved, Detail: name})
			continue
		}
		for _, p := range current.Modules[name].Paths {
			if !slices.Contains(cand.Paths, p) {
				out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageModulePathsRemoved, Detail: name + ": " + p})
			}
		}
		for _, p := range cand.Paths {
			if !slices.Contains(current.Modules[name].Paths, p) {
				out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageModulePathsAdded, Detail: name + ": " + p})
			}
		}
	}
	for _, name := range sortedKeys(candidate.Modules) {
		if _, ok := current.Modules[name]; !ok {
			out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageModuleAdded, Detail: name})
		}
	}
	from, to := orWarn(current.ModuleReviewGate), orWarn(candidate.ModuleReviewGate)
	detail := "module_review.gate: " + from + " -> " + to
	switch {
	case gateRank(to) < gateRank(from):
		out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageModuleReviewLowered, Detail: detail})
	case gateRank(to) > gateRank(from):
		out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageModuleReviewRaised, Detail: detail})
	}
}

// switches compares enabled switches: turning one off loses coverage; turning
// one on (or to auto from off) gains it. auto and on read the same code when
// the tool is present, so a change between them is not reported.
func switches(current, candidate map[string]string, prefix, lost, gained string, out *PolicyChange) {
	for _, id := range unionKeys(current, candidate) {
		from, to := current[id], candidate[id]
		wasOff, isOff := from == gateOffValue, to == gateOffValue
		detail := prefix + id + ".enabled: " + orAuto(from) + " -> " + orAuto(to)
		switch {
		case !wasOff && isOff:
			out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: lost, Detail: detail})
		case wasOff && !isOff:
			out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: gained, Detail: detail})
		}
	}
}

func rulesByID(rules []policy.RuleDef) map[string]policy.RuleDef {
	out := make(map[string]policy.RuleDef, len(rules))
	for _, r := range rules {
		out[r.ID] = r
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func unionKeys(a, b map[string]string) []string {
	set := map[string]struct{}{}
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	return sortedKeys(set)
}

func dedupe(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func allowlistString(list []string) string {
	switch {
	case list == nil:
		return "any"
	case len(list) == 0:
		return "none"
	default:
		return fmt.Sprint(dedupe(list))
	}
}

func maxString(v *int) string {
	if v == nil {
		return "unset"
	}
	return strconv.Itoa(*v)
}

func orOff(gate string) string {
	if gate == "" {
		return gateOffValue
	}
	return gate
}

func orWarn(gate string) string {
	if gate == "" {
		return gateWarnValue
	}
	return gate
}

func orAuto(mode string) string {
	if mode == "" {
		return "auto"
	}
	return mode
}
