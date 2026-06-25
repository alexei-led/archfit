// Package syntax provides pure role derivation over SyntaxFacts gathered by
// the ast-grep extractor. It is a core-ring package: no os, no subprocess, no
// YAML, no adapter imports.
package syntax

import (
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// Role constants — the four roles §5 defines.
const (
	RoleHandler    = "handler"
	RoleService    = "service"
	RoleRepository = "repository"
	RoleDomain     = "domain"
)

// Confidence constants — §5 confidence tiers.
const (
	ConfHigh   = "high"
	ConfMedium = "medium"
	ConfLow    = "low"
)

// KnownRoles lists the accepted role values for validation.
var KnownRoles = []string{RoleHandler, RoleService, RoleRepository, RoleDomain}

// KnownConfidences lists the accepted confidence values for validation.
var KnownConfidences = []string{ConfHigh, ConfMedium, ConfLow}

// ConfidenceMeets reports whether conf meets or exceeds minimum.
// Ordinals: high=3, medium=2, low=1. Unknown strings return false.
func ConfidenceMeets(conf, minimum string) bool {
	c, m := confRank(conf), confRank(minimum)
	if c == 0 || m == 0 {
		return false
	}
	return c >= m
}

// confRank maps a confidence string to a numeric rank (high=3, medium=2, low=1,
// unknown=0). Higher values represent higher confidence.
func confRank(c string) int {
	switch strings.ToLower(c) {
	case ConfHigh:
		return 3
	case ConfMedium:
		return 2
	case ConfLow:
		return 1
	default:
		return 0
	}
}

// DeriveRoles returns a new slice of SyntaxFacts with Role, RoleConf, and
// Evidence populated per design §5. Facts that do not match any heuristic are
// returned unchanged (Role/RoleConf/Evidence stay empty).
//
// Derivation is language-agnostic and purely structural — it inspects each
// fact's Kind, Name, File, and Framework fields without I/O.
//
// Precedence (highest wins, first match sets and stops):
//  1. annotation / route / non-empty Framework  → high  (structural evidence)
//  2. Name suffix (*Handler, *Controller, *Repository, *Service, *Domain, …) → medium
//  3. File path substring                        → low
func DeriveRoles(facts []diagnostic.SyntaxFact) []diagnostic.SyntaxFact {
	out := make([]diagnostic.SyntaxFact, len(facts))
	for i, f := range facts {
		out[i] = deriveOne(f)
	}
	return out
}

// deriveOne classifies a single SyntaxFact and returns it with Role/RoleConf/
// Evidence set (or unchanged when no heuristic fires).
func deriveOne(f diagnostic.SyntaxFact) diagnostic.SyntaxFact {
	// Tier 1 — annotation / route / framework: high confidence.
	//
	// Kind "route" is set by the ast-grep extractor for route registrations.
	// Kind "annotation" covers Java-style @Controller, @Service, @Repository
	// decorators produced by TypeScript/Python decorators and Go //go:embed-style.
	// Non-empty Framework means a known web framework route registration was
	// detected by the extractor.
	//
	// Ceiling: Go signature `http.ResponseWriter`/`*http.Request` evidence would
	// also be high-confidence but is not observable from SyntaxFact (no params
	// field); upgrade to high when a params/signature field is added.
	if f.Kind == "route" || f.Framework != "" {
		return set(f, RoleHandler, ConfHigh, handlerEvidence(f))
	}

	// Kind "annotation" — role determined by the annotation's name so we can
	// distinguish @Controller (handler) from @Service, @Repository, etc.
	if f.Kind == "annotation" {
		if r, ev := roleFromAnnotationName(f.Name); r != "" {
			return set(f, r, ConfHigh, ev)
		}
	}

	// Tier 2 — name suffix: medium confidence.
	if r, ev := roleFromName(f.Name); r != "" {
		return set(f, r, ConfMedium, ev)
	}

	// Tier 3 — file path substring: low confidence.
	if r, ev := roleFromPath(f.File); r != "" {
		return set(f, r, ConfLow, ev)
	}

	return f
}

// handlerEvidence builds the high-confidence evidence string for route/framework hits.
func handlerEvidence(f diagnostic.SyntaxFact) string {
	if f.Framework != "" {
		return "framework " + f.Framework + " route registration"
	}
	return "route registration (kind=route)"
}

// roleFromAnnotationName derives a role from an annotation/decorator name.
// Covers patterns like @Controller, @Service, @Repository, @Handler.
// Name comparison is case-insensitive.
func roleFromAnnotationName(name string) (role, evidence string) {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "controller") || strings.Contains(lower, "handler") || strings.Contains(lower, "route"):
		return RoleHandler, "decorator " + name
	case strings.Contains(lower, "repository") || strings.Contains(lower, "repo") || strings.Contains(lower, "storage") || strings.Contains(lower, "persistence"):
		return RoleRepository, "decorator " + name
	case strings.Contains(lower, "service") || strings.Contains(lower, "usecase"):
		return RoleService, "decorator " + name
	case strings.Contains(lower, "domain") || strings.Contains(lower, "entity") || strings.Contains(lower, "model"):
		return RoleDomain, "decorator " + name
	}
	return "", ""
}

// roleFromName derives a role from a symbol name using §5 suffix heuristics.
// Name checks are case-sensitive suffix matches per §5 ("*Repository", "*Service").
func roleFromName(name string) (role, evidence string) {
	switch {
	case strings.HasSuffix(name, "Handler") || strings.HasSuffix(name, "Controller"):
		return RoleHandler, "name suffix " + name
	case strings.HasSuffix(name, "Repository") || strings.HasSuffix(name, "Repo") || strings.HasSuffix(name, "Storage") || strings.HasSuffix(name, "Persistence"):
		return RoleRepository, "name suffix " + name
	case strings.HasSuffix(name, "Service") || strings.HasSuffix(name, "UseCase") || strings.HasSuffix(name, "Usecase"):
		return RoleService, "name suffix " + name
	case strings.HasSuffix(name, "Domain") || strings.HasSuffix(name, "Entity") || strings.HasSuffix(name, "Model"):
		return RoleDomain, "name suffix " + name
	}
	return "", ""
}

// roleToken is a canonical path segment and its common plural form used for
// whole-segment matching in roleFromPath. Empty Plural means no plural form.
type roleToken struct {
	Token  string
	Plural string
}

// Path-segment token tables, ordered highest-priority first within each role group.
// Extensibility: add entries here to recognise new path-segment conventions.
var (
	handlerTokens = []roleToken{
		{Token: "handler", Plural: "handlers"},
		{Token: "controller", Plural: "controllers"},
		{Token: "route", Plural: "routes"},
		{Token: "router", Plural: "routers"},
	}
	repoTokens = []roleToken{
		{Token: "repository", Plural: "repositories"},
		{Token: "repo", Plural: "repos"},
		{Token: "storage", Plural: "storages"},
		{Token: "persistence", Plural: ""},
	}
	serviceTokens = []roleToken{
		{Token: "service", Plural: "services"},
		{Token: "usecase", Plural: "usecases"},
		{Token: "application", Plural: "applications"},
	}
	domainTokens = []roleToken{
		{Token: "domain", Plural: "domains"},
		{Token: "model", Plural: "models"},
		{Token: "entity", Plural: "entities"},
	}
)

// roleFromPath derives a role from a file path using §5 path-segment heuristics.
// Path is lowercased and split on [/\_\-.] so only whole tokens match.
// This prevents "repo" matching "update_report" or "domain" matching "subdomain".
func roleFromPath(file string) (role, evidence string) {
	lower := strings.ToLower(file)
	if seg, ok := firstSegmentMatch(lower, handlerTokens); ok {
		return RoleHandler, "path contains " + seg
	}
	if seg, ok := firstSegmentMatch(lower, repoTokens); ok {
		return RoleRepository, "path contains " + seg
	}
	if seg, ok := firstSegmentMatch(lower, serviceTokens); ok {
		return RoleService, "path contains " + seg
	}
	if seg, ok := firstSegmentMatch(lower, domainTokens); ok {
		return RoleDomain, "path contains " + seg
	}
	return "", ""
}

// set returns a copy of f with Role, RoleConf, and Evidence set.
func set(f diagnostic.SyntaxFact, role, conf, evidence string) diagnostic.SyntaxFact {
	f.Role = role
	f.RoleConf = conf
	f.Evidence = evidence
	return f
}

// firstSegmentMatch splits lower on path delimiters and returns the matched
// segment (the actual token found in the path) if any token in the table
// matches a whole segment (including plural forms). Returns "", false if none match.
// Evidence uses the matched segment from the path, not the canonical token name.
func firstSegmentMatch(lower string, tokens []roleToken) (string, bool) {
	segs := strings.FieldsFunc(lower, func(r rune) bool {
		return r == '/' || r == '\\' || r == '_' || r == '-' || r == '.'
	})
	for _, seg := range segs {
		for _, t := range tokens {
			if seg == t.Token || (t.Plural != "" && seg == t.Plural) {
				return seg, true // return the actual path segment for evidence
			}
		}
	}
	return "", false
}
