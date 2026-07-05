package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/initcfg"
)

const (
	jsonFieldEvidenceRefs           = "evidence_refs"
	ruleTypeForbiddenDependency     = "forbidden_dependency"
	ruleTypeForbiddenRoleDependency = "forbidden_role_dependency"
	ruleTypePublicAPIMax            = "public_api_max"
	ruleTypePublicAPIChange         = "public_api_change"
	ruleTypeCouplingGate            = "coupling.gate"
)

var allowedDraftRuleTypes = map[string]struct{}{
	ruleTypeForbiddenDependency:     {},
	ruleTypeForbiddenRoleDependency: {},
	ruleTypePublicAPIMax:            {},
	ruleTypePublicAPIChange:         {},
	ruleTypeCouplingGate:            {},
}

type ruleSuggestionResponse struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Gate         string   `json:"gate"`
	From         string   `json:"from"`
	To           string   `json:"to"`
	Max          *int     `json:"max"`
	MinBand      string   `json:"min_band"`
	MaxDrop      *int     `json:"max_drop"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
	Basis        string   `json:"basis"`
}

type externalSystemSuggestionResponse struct {
	Name         string   `json:"name"`
	Targets      []string `json:"targets"`
	Volatility   string   `json:"volatility"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
	Basis        string   `json:"basis"`
}

func draftMetadata(scope, name, basis string, refs []string, requireEvidence bool, allowedRefs ...map[string]struct{}) (string, []string, error) {
	basis = strings.TrimSpace(basis)
	cleaned := cleanEvidenceRefs(refs)
	if basis != "" && !validDraftBasis(basis) {
		return "", nil, fmt.Errorf("%s %q has invalid basis %q", scope, name, basis)
	}
	knownRefs := firstAllowedEvidenceRefs(allowedRefs)
	if len(cleaned) > 0 {
		for _, ref := range cleaned {
			if !validEvidenceRef(ref) {
				return "", nil, fmt.Errorf("%s %q has invalid %s %q", scope, name, jsonFieldEvidenceRefs, ref)
			}
			if knownRefs != nil {
				if _, ok := knownRefs[ref]; !ok {
					return "", nil, fmt.Errorf("%s %q has unsupported %s %q", scope, name, jsonFieldEvidenceRefs, ref)
				}
			}
		}
	}
	if !requireEvidence {
		return basis, cleaned, nil
	}
	if basis == "" {
		return "", nil, fmt.Errorf("%s %q missing basis", scope, name)
	}
	if len(cleaned) == 0 {
		return "", nil, fmt.Errorf("%s %q missing %s", scope, name, jsonFieldEvidenceRefs)
	}
	return basis, cleaned, nil
}

func firstAllowedEvidenceRefs(allowedRefs []map[string]struct{}) map[string]struct{} {
	if len(allowedRefs) == 0 || len(allowedRefs[0]) == 0 {
		return nil
	}
	return allowedRefs[0]
}

func validDraftBasis(s string) bool {
	switch s {
	case initcfg.DraftBasisDeterministicFact, initcfg.DraftBasisSemanticJudgment:
		return true
	default:
		return false
	}
}

func cleanEvidenceRefs(refs []string) []string {
	seen := make(map[string]struct{}, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func validEvidenceRef(ref string) bool {
	prefix, rest, ok := strings.Cut(ref, ":")
	if !ok || rest == "" {
		return false
	}
	switch prefix {
	case "doc", "api", "comment", "config", "diag":
	default:
		return false
	}
	for _, r := range ref {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func evidenceRefSet(lines []string) map[string]struct{} {
	if len(lines) == 0 {
		return nil
	}
	refs := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		id, _, ok := strings.Cut(line, " (")
		if !ok {
			id, _, ok = strings.Cut(line, " ")
		}
		if !ok {
			id = line
		}
		id = strings.TrimSpace(id)
		if validEvidenceRef(id) {
			refs[id] = struct{}{}
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func parseRuleSuggestionResponses(module string, entries []ruleSuggestionResponse, requireEvidence bool, allowedRefs ...map[string]struct{}) ([]initcfg.RuleSuggestion, error) {
	out := make([]initcfg.RuleSuggestion, 0, len(entries))
	for _, e := range entries {
		typ := strings.TrimSpace(e.Type)
		if _, ok := allowedDraftRuleTypes[typ]; !ok {
			return nil, fmt.Errorf("rule suggestion for %q has unsupported type %q", module, e.Type)
		}
		id := strings.TrimSpace(e.ID)
		name := id
		if name == "" {
			name = typ
		}
		rationale := strings.TrimSpace(e.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf("rule suggestion %q missing rationale", name)
		}
		basis, refs, err := draftMetadata("rule suggestion", name, e.Basis, e.EvidenceRefs, requireEvidence, firstAllowedEvidenceRefs(allowedRefs))
		if err != nil {
			return nil, err
		}
		from := strings.TrimSpace(e.From)
		to := strings.TrimSpace(e.To)
		s := initcfg.RuleSuggestion{
			SourceModule: module,
			ID:           id,
			Type:         typ,
			Gate:         strings.TrimSpace(e.Gate),
			From:         from,
			To:           to,
			Max:          e.Max,
			MinBand:      strings.TrimSpace(e.MinBand),
			MaxDrop:      e.MaxDrop,
			Rationale:    rationale,
			EvidenceRefs: refs,
			Basis:        basis,
		}
		if err := validateRuleSuggestionShape(s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func validateRuleSuggestionShape(s initcfg.RuleSuggestion) error {
	subject := s.ID
	if subject == "" {
		subject = s.Type
	}
	switch s.Type {
	case ruleTypeForbiddenDependency, ruleTypeForbiddenRoleDependency:
		if s.From == "" || s.To == "" {
			return fmt.Errorf("rule suggestion %q requires from and to", subject)
		}
	case ruleTypePublicAPIMax:
		if s.Max == nil {
			return fmt.Errorf("rule suggestion %q requires max", subject)
		}
	case ruleTypePublicAPIChange:
		// No extra shape: public_api_change is global and defaults to gate: warn.
	case ruleTypeCouplingGate:
		if s.MinBand == "" && s.MaxDrop == nil {
			return fmt.Errorf("rule suggestion %q requires min_band or max_drop", subject)
		}
	}
	return nil
}

func parseExternalSystemSuggestionResponses(module string, entries []externalSystemSuggestionResponse, requireEvidence bool, allowedRefs ...map[string]struct{}) ([]initcfg.ExternalSystemSuggestion, error) {
	out := make([]initcfg.ExternalSystemSuggestion, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, fmt.Errorf("external_systems suggestion for %q missing name", module)
		}
		rationale := strings.TrimSpace(e.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf("external_systems suggestion %q missing rationale", name)
		}
		basis, refs, err := draftMetadata("external_systems suggestion", name, e.Basis, e.EvidenceRefs, requireEvidence, firstAllowedEvidenceRefs(allowedRefs))
		if err != nil {
			return nil, err
		}
		s := initcfg.ExternalSystemSuggestion{
			SourceModule: module,
			Name:         name,
			Targets:      cleanExternalTargets(e.Targets),
			Volatility:   strings.TrimSpace(e.Volatility),
			Rationale:    rationale,
			EvidenceRefs: refs,
			Basis:        basis,
		}
		if err := validateExternalSystemSuggestionShape(s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func cleanExternalTargets(targets []string) []string {
	seen := make(map[string]struct{}, len(targets))
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

func validateExternalSystemSuggestionShape(s initcfg.ExternalSystemSuggestion) error {
	if len(s.Targets) == 0 {
		return fmt.Errorf("external_systems suggestion %q requires at least one target", s.Name)
	}
	for _, target := range s.Targets {
		if !doublestar.ValidatePattern(target) {
			return fmt.Errorf("external_systems suggestion %q target %q is not a valid glob pattern", s.Name, target)
		}
	}
	if s.Volatility != "" && !validExternalSystemSuggestionVolatility(s.Volatility) {
		return fmt.Errorf("external_systems suggestion %q volatility %q is not one of high|medium|low|frozen", s.Name, s.Volatility)
	}
	return nil
}

func validExternalSystemSuggestionVolatility(v string) bool {
	switch strings.ToLower(v) {
	case volatilityHigh, volatilityMedium, volatilityLow, volatilityFrozen:
		return true
	default:
		return false
	}
}
