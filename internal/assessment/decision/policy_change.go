package decision

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

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
	CoverageLanguageLowered     = "language_mode_lowered"
	CoverageLanguageRaised      = "language_mode_raised"
	CoverageModuleRemoved       = "module_removed"
	CoverageModulePathsRemoved  = "module_paths_removed"
	CoverageModuleReviewLowered = "module_review_gate_lowered"
	CoverageModuleReviewRaised  = "module_review_gate_raised"
)

// fieldGate names a rule's gate in a RuleChange.
const fieldGate = "gate"

// Enabled modes, as the configuration spells them.
const (
	modeOn   = "on"
	modeAuto = "auto"
)

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
	// Modules: paths, depends_on and visible_to are ordered here; every other
	// module field is in ModuleDeclarations.
	Modules map[string]policy.ModuleDef
	// ModuleDeclarations maps a module name to the canonical encoding of its
	// other fields (layer, role, public, internal, owner, review dates, ...).
	ModuleDeclarations map[string]string
	// Metrics maps a configured metric name to its ratchet. A metric without an
	// entry ratchets at the default (fail, no tolerance).
	Metrics map[string]MetricDecl
	// Exclude are the effective exclusion globs: defaults merged with the
	// configured ones, re-includes applied.
	Exclude []string
	// Languages map an id to its enabled mode: off, auto, on (on fails when the
	// tool is missing; auto skips).
	Languages map[string]string
	// Analyzers map an optional analyzer id to whether it runs (on, off).
	Analyzers map[string]string
	// ModuleReviewGate is the effective module_review gate: off when the
	// review pass does not run, else warn or fail.
	ModuleReviewGate string
	// Declarations are every other declaration that can change a verdict,
	// keyed by config path, in a canonical encoding. They cannot be ordered, so
	// any difference counts as loosening.
	Declarations map[string]string
}

// MetricDecl is one metric ratchet: its gate (off, warn, fail; "" is the
// default, fail) and its tolerances (nil is 0).
type MetricDecl struct {
	Gate     string
	MaxNew   *int
	MinDelta *float64
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

// DeclarationChange is a declaration that differs and cannot be ordered: a
// module's layer or public surface, a gate outside rules and metrics, a new
// module that may take code from another. It counts as loosening.
type DeclarationChange struct {
	Key  string
	From string
	To   string
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
	// DeclarationsChanged are differences that cannot be ordered.
	DeclarationsChanged []DeclarationChange
	// Tightening: each can fail a change that passes today.
	RulesAdded         []RuleRef
	RulesTightened     []RuleChange
	WaiversRemoved     []policy.WaiverDef
	AllowlistsNarrowed []AllowlistChange
	CoverageGained     []CoverageChange
}

// ComparePolicies classifies the change from current to candidate. Only a
// difference this function can order is a tightening; every other difference
// counts as loosening (a moved selector, an unordered declaration, a new module
// that may take code from another). The result decides who may approve, so a
// missed loosening is the failure to avoid.
func ComparePolicies(current, candidate PolicyInput) PolicyChange {
	var out PolicyChange
	compareRules(current, candidate, &out)
	compareWaivers(current.Waivers, candidate.Waivers, &out)
	compareAllowlists(current.Modules, candidate.Modules, &out)
	comparePolicyCoverage(current, candidate, &out)
	compareDeclarations(current, candidate, &out)
	out.Direction = direction(out)
	return out
}

func direction(c PolicyChange) PolicyChangeDirection {
	loosens := len(c.RulesRemoved)+len(c.RulesLoosened)+len(c.WaiversAdded)+len(c.AllowlistsWidened)+len(c.CoverageLost)+len(c.DeclarationsChanged) > 0
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
		rs, ok := cand[id]
		switch {
		case !ok:
			for _, r := range cur[id] {
				out.RulesRemoved = append(out.RulesRemoved, RuleRef{ID: id, Type: r.Type, Gate: effectiveGate(r)})
			}
		case len(cur[id]) == 1 && len(rs) == 1:
			compareRule(cur[id][0], rs[0], out)
		default:
			// Several declarations share the ID (the config accepts it and
			// every one is evaluated): they cannot be paired, so any
			// difference between the two sets is a loosening.
			if from, to := ruleSet(cur[id]), ruleSet(rs); from != to {
				out.RulesLoosened = append(out.RulesLoosened, RuleChange{ID: id, Field: "declaration", From: from, To: to})
			}
		}
	}
	for _, id := range sortedKeys(cand) {
		if _, ok := cur[id]; !ok {
			for _, r := range cand[id] {
				out.RulesAdded = append(out.RulesAdded, RuleRef{ID: id, Type: r.Type, Gate: effectiveGate(r)})
			}
		}
	}
	// Metric ratchets: a lowered gate or a larger tolerance lets a regression
	// through. A metric with no entry ratchets at the default, fail.
	for _, name := range unionKeys(current.Metrics, candidate.Metrics) {
		cur, cand := current.Metrics[name], candidate.Metrics[name]
		id := "metric/" + name
		from, to := orFail(cur.Gate), orFail(cand.Gate)
		order(gateRank(from), gateRank(to), RuleChange{ID: id, Field: fieldGate, From: from, To: to}, out)
		order(-intOrZero(cur.MaxNew), -intOrZero(cand.MaxNew),
			RuleChange{ID: id, Field: "max_new", From: strconv.Itoa(intOrZero(cur.MaxNew)), To: strconv.Itoa(intOrZero(cand.MaxNew))}, out)
		curMin, candMin := floatOrZero(cur.MinDelta), floatOrZero(cand.MinDelta)
		if curMin != candMin {
			change := RuleChange{ID: id, Field: "min_delta", From: strconv.FormatFloat(curMin, 'g', -1, 64), To: strconv.FormatFloat(candMin, 'g', -1, 64)}
			// A tolerance that cannot be ordered (NaN, infinity) turns the
			// ratchet off: loosening.
			if candMin > curMin || !finite(curMin) || !finite(candMin) {
				out.RulesLoosened = append(out.RulesLoosened, change)
			} else {
				out.RulesTightened = append(out.RulesTightened, change)
			}
		}
	}
}

// order files a change by rank: lower in the candidate loosens, higher
// tightens, equal is no change.
func order(from, to int, change RuleChange, out *PolicyChange) {
	switch {
	case to < from:
		out.RulesLoosened = append(out.RulesLoosened, change)
	case to > from:
		out.RulesTightened = append(out.RulesTightened, change)
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
	// Everything else a rule declares (type, selectors, patterns, guard,
	// threshold) cannot be ordered: a moved selector or a new guard stops
	// checking what the rule checked. Rationale, alternatives and docs are
	// text for the reader and never change a verdict.
	if from, to := ruleDeclaration(cur), ruleDeclaration(cand); from != to {
		out.RulesLoosened = append(out.RulesLoosened, RuleChange{ID: cur.ID, Field: "declaration", From: from, To: to})
	}
}

// ruleDeclaration encodes a rule without the fields compared in order (gate,
// max) and the reader-only text (rationale, alternatives, docs).
func ruleDeclaration(r policy.RuleDef) string {
	r.Gate, r.Max, r.Rationale, r.Alternatives, r.Docs = "", nil, "", nil, ""
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf("%#v", r)
	}
	return string(raw)
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
	languageModes(current.Languages, candidate.Languages, out)
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
		// A path a module gains may be taken from another module, whose
		// allowlists then stop applying to it: not orderable.
		for _, p := range cand.Paths {
			if !slices.Contains(current.Modules[name].Paths, p) {
				out.DeclarationsChanged = append(out.DeclarationsChanged, DeclarationChange{Key: "modules." + name + ".paths", To: p})
			}
		}
	}
	for _, name := range sortedKeys(candidate.Modules) {
		if _, ok := current.Modules[name]; !ok {
			out.DeclarationsChanged = append(out.DeclarationsChanged, DeclarationChange{Key: "modules." + name, To: "added: " + strings.Join(candidate.Modules[name].Paths, ", ")})
		}
	}
	from, to := orOff(current.ModuleReviewGate), orOff(candidate.ModuleReviewGate)
	detail := "module_review.gate: " + from + " -> " + to
	switch {
	case gateRank(to) < gateRank(from):
		out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageModuleReviewLowered, Detail: detail})
	case gateRank(to) > gateRank(from):
		out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageModuleReviewRaised, Detail: detail})
	}
}

// switches compares on/off switches of optional analyzers: only on runs one.
func switches(current, candidate map[string]string, prefix, lost, gained string, out *PolicyChange) {
	for _, id := range unionKeys(current, candidate) {
		wasOn, isOn := current[id] == modeOn, candidate[id] == modeOn
		detail := prefix + id + ".enabled: " + onOff(wasOn) + " -> " + onOff(isOn)
		switch {
		case wasOn && !isOn:
			out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: lost, Detail: detail})
		case !wasOn && isOn:
			out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: gained, Detail: detail})
		}
	}
}

// languageModes ranks language modes off < auto < on: on fails when the
// extractor is missing, auto skips the language quietly.
func languageModes(current, candidate map[string]string, out *PolicyChange) {
	rank := map[string]int{gateOffValue: 0, modeAuto: 1, "": 1, modeOn: 2}
	for _, id := range unionKeys(current, candidate) {
		from, to := current[id], candidate[id]
		detail := "languages." + id + ".enabled: " + orAuto(from) + " -> " + orAuto(to)
		switch {
		case rank[to] < rank[from] && to == gateOffValue:
			out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageLanguageDisabled, Detail: detail})
		case rank[to] < rank[from]:
			out.CoverageLost = append(out.CoverageLost, CoverageChange{Kind: CoverageLanguageLowered, Detail: detail})
		case rank[to] > rank[from] && from == gateOffValue:
			out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageLanguageEnabled, Detail: detail})
		case rank[to] > rank[from]:
			out.CoverageGained = append(out.CoverageGained, CoverageChange{Kind: CoverageLanguageRaised, Detail: detail})
		}
	}
}

// compareDeclarations reports every unordered declaration that differs. A
// module's own declarations are compared only when the module exists on both
// sides: adding or removing it is reported once, above.
func compareDeclarations(current, candidate PolicyInput, out *PolicyChange) {
	for _, key := range unionKeys(current.Declarations, candidate.Declarations) {
		if current.Declarations[key] != candidate.Declarations[key] {
			out.DeclarationsChanged = append(out.DeclarationsChanged, DeclarationChange{Key: key, From: current.Declarations[key], To: candidate.Declarations[key]})
		}
	}
	for _, name := range sortedKeys(current.Modules) {
		if _, both := candidate.Modules[name]; !both {
			continue
		}
		if from, to := current.ModuleDeclarations[name], candidate.ModuleDeclarations[name]; from != to {
			out.DeclarationsChanged = append(out.DeclarationsChanged, DeclarationChange{Key: "modules[" + name + "]", From: from, To: to})
		}
	}
}

// rulesByID groups the rules by ID; an ID can carry several declarations.
func rulesByID(rules []policy.RuleDef) map[string][]policy.RuleDef {
	out := make(map[string][]policy.RuleDef, len(rules))
	for _, r := range rules {
		out[r.ID] = append(out[r.ID], r)
	}
	return out
}

// ruleSet encodes the declarations of one ID as an order-free set, gate and
// max included: every one of them is evaluated.
func ruleSet(rules []policy.RuleDef) string {
	parts := make([]string, 0, len(rules))
	for _, r := range rules {
		parts = append(parts, effectiveGate(r)+" "+maxString(r.Max)+" "+ruleDeclaration(r))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func unionKeys[V any](a, b map[string]V) []string {
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

func orFail(gate string) string {
	if gate == "" {
		return gateFailValue
	}
	return gate
}

func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func floatOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func onOff(on bool) string {
	if on {
		return modeOn
	}
	return gateOffValue
}

func orOff(gate string) string {
	if gate == "" {
		return gateOffValue
	}
	return gate
}

func orAuto(mode string) string {
	if mode == "" {
		return "auto"
	}
	return mode
}
