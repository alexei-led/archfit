// Package resolved is the Archfit 4 policy model: a schema 3 policy with the
// defaults of design section 6.1 applied, so that every consumer reads the same
// meaning from a short file.
//
// The package states what the policy means. It reads no file and imports no
// config package: internal/config/policyfile decodes the file and projects it
// here. It owns the checks that need more than one key or the file tree
// (Check, CheckTree), which file belongs to which module (ModuleOf), the digest
// of the file, the comparison of two policies (Compare) and the package sets of
// the only rule (Uses).
//
// Known limits of the package sets, which see imports only: TypeScript's global
// fetch and Python's os.system need no import, so the network and process sets
// do not see them. C# has no process entry: Process shares the System.Diagnostics
// namespace with Stopwatch and Debug, and a using names only the namespace.
package resolved

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Gate says whether the check blocks a pull request.
type Gate string

// Gate values.
const (
	GateAdvisory Gate = "advisory"
	GateRequired Gate = "required"
)

// Mode is how a built-in rule treats an error.
type Mode string

// Mode values, from the strictest.
const (
	ModeDeny Mode = "deny"
	ModeAsk  Mode = "ask"
	ModeOff  Mode = "off"
)

// Volatility is how often a module is expected to change.
type Volatility string

// Volatility values.
const (
	VolatilityLow    Volatility = "low"
	VolatilityMedium Volatility = "medium"
	VolatilityHigh   Volatility = "high"
)

// Subdomain is the business role of a module.
type Subdomain string

// Subdomain values.
const (
	SubdomainCore       Subdomain = "core"
	SubdomainSupporting Subdomain = "supporting"
	SubdomainGeneric    Subdomain = "generic"
)

// The ids of the built-in rules.
const (
	RuleGraph       = "graph"
	RuleCoupling    = "coupling"
	RuleDuplication = "duplication"
	RuleCycle       = "cycle"
	RuleLayers      = "layers"
	RuleUnowned     = "unowned"
)

// BuiltinRules are the ids of the built-in rules, in the order of design 6.2.
var BuiltinRules = []string{RuleGraph, RuleCoupling, RuleDuplication, RuleCycle, RuleLayers, RuleUnowned}

// DefaultMinBalance is the lowest balance the coupling rule accepts when the
// policy names none.
const DefaultMinBalance = 7

// Policy is a resolved schema 3 policy.
type Policy struct {
	// Digest is "sha256:" and the hex digest of the file bytes.
	Digest string
	Owners []string
	Gate   Gate
	// Layers are layer names, inner first. Empty: no layer rule.
	Layers []string
	// Modules are keyed by module name.
	Modules    map[string]Module
	Builtin    Builtin
	Rules      []Rule
	Exceptions []Exception
	// Generated and Tests are globs of code the rules skip.
	Generated []string
	Tests     []string
	// Exclude are globs of paths no extractor reads.
	Exclude []string
	// Settings are the tool, language and coverage keys of the file, flattened
	// to "languages.go.enabled" -> "true". The model does not read them; Compare
	// reports their changes.
	Settings map[string]string
}

// Module is a named set of paths.
type Module struct {
	Paths []string
	// Public are the globs other modules may use. With defaults: the module's
	// paths when the file names none.
	Public []string
	// Volatility with defaults: the declared value, else from Subdomain (core
	// high; supporting and generic low), else high.
	Volatility Volatility
	// Subdomain is the declared business role, or empty.
	Subdomain Subdomain
	// Layer is a name from Policy.Layers, or empty.
	Layer string
	// DependsOn are the modules this module may depend on. Empty: none.
	DependsOn []string
}

// Builtin holds the modes of the built-in rules and the minimum balance.
type Builtin struct {
	Graph       Mode
	Coupling    Mode
	Duplication Mode
	Cycle       Mode
	Layers      Mode
	Unowned     Mode
	MinBalance  int
}

// Form is the kind of a custom rule.
type Form string

// Form values.
const (
	FormDeny Form = "deny"
	FormOnly Form = "only"
	FormNote Form = "note"
)

// Rule is a custom rule.
type Rule struct {
	ID   string
	Form Form
	// From and To are module names or path globs (deny).
	From []string
	To   []string
	// Use is a library or a package set name; In are module names or path
	// globs (only).
	Use string
	In  []string
	// Note is the text of a note rule.
	Note    string
	Why     string
	Instead string
}

// Exception permits errors of one rule in one place.
type Exception struct {
	Rule string
	// From is a module, or for unowned a path glob.
	From string
	// To is a module; empty for unowned.
	To string
	// Expires is the last day the exception holds, at 00:00 UTC. Zero: no date.
	Expires time.Time
	Why     string
}

// WithDefaults returns p with the defaults of design 6.1 and the built-in
// defaults of 6.2 applied to every absent value.
func (p Policy) WithDefaults() Policy {
	if p.Gate == "" {
		p.Gate = GateAdvisory
	}
	modules := make(map[string]Module, len(p.Modules))
	for name, m := range p.Modules {
		modules[name] = m.withDefaults()
	}
	p.Modules = modules
	p.Builtin = p.Builtin.withDefaults()
	return p
}

func (m Module) withDefaults() Module {
	if m.Public == nil {
		m.Public = slices.Clone(m.Paths)
	}
	if m.Volatility == "" {
		m.Volatility = volatilityOf(m.Subdomain)
	}
	return m
}

// volatilityOf is the volatility a subdomain implies; no subdomain is high.
func volatilityOf(s Subdomain) Volatility {
	switch s {
	case SubdomainSupporting, SubdomainGeneric:
		return VolatilityLow
	default:
		return VolatilityHigh
	}
}

func (b Builtin) withDefaults() Builtin {
	set := func(m *Mode, def Mode) {
		if *m == "" {
			*m = def
		}
	}
	set(&b.Graph, ModeAsk)
	set(&b.Coupling, ModeDeny)
	set(&b.Duplication, ModeAsk)
	set(&b.Cycle, ModeDeny)
	set(&b.Layers, ModeDeny)
	set(&b.Unowned, ModeDeny)
	if b.MinBalance == 0 {
		b.MinBalance = DefaultMinBalance
	}
	return b
}

// modes lists the built-in rules with their modes, in BuiltinRules order.
func (b Builtin) modes() []struct {
	rule string
	mode Mode
} {
	return []struct {
		rule string
		mode Mode
	}{
		{RuleGraph, b.Graph}, {RuleCoupling, b.Coupling}, {RuleDuplication, b.Duplication},
		{RuleCycle, b.Cycle}, {RuleLayers, b.Layers}, {RuleUnowned, b.Unowned},
	}
}

// Digest is "sha256:" and the hex SHA-256 of the policy file bytes. It names
// the exact file: the same policy written another way has another digest.
func Digest(file []byte) string {
	sum := sha256.Sum256(file)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ModuleOf returns the module that owns a repository-relative, slash-separated
// file: the module whose matching path glob is the most specific (the longest
// part before the first wildcard). When two modules tie, CheckTree reports the
// file; ModuleOf then answers with the first by name. ("", false) when no
// module owns the file.
func (p Policy) ModuleOf(file string) (string, bool) {
	owners := p.owners(file)
	if len(owners) == 0 {
		return "", false
	}
	return owners[0], true
}

// owners returns the modules that claim file at the highest specificity, in
// name order.
func (p Policy) owners(file string) []string {
	best := -1
	winners := make([]string, 0, 2)
	for _, name := range slices.Sorted(maps.Keys(p.Modules)) {
		for _, glob := range p.Modules[name].Paths {
			if !matchGlob(glob, file) {
				continue
			}
			switch spec := specificity(glob); {
			case spec > best:
				best, winners = spec, append(winners[:0], name)
			case spec == best && !slices.Contains(winners, name):
				winners = append(winners, name)
			}
		}
	}
	return winners
}

// specificity is the byte length of a glob's part before the first wildcard;
// a glob without one is a path and is as specific as its length.
func specificity(glob string) int {
	if i := strings.IndexAny(glob, globMeta); i >= 0 {
		return i
	}
	return len(glob)
}

// globMeta are the characters that make a string a glob.
const globMeta = "*?[{"

func matchGlob(glob, path string) bool {
	ok, _ := doublestar.Match(glob, path)
	return ok
}
