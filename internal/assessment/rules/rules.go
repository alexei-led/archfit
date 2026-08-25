// Package rules defines the Rule interface and the built-in rule
// implementations: ForbiddenDependency, PublicAPIOnly, ForbiddenLayerDirection,
// InternalAPIAccess, NewCrossModuleDependency, CycleRule,
// PublicAPIMax, PublicAPIChange, PublicAPITypeLeak.
package rules

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/view"
)

// kindGate and kindAdvisory are the two Finding.Kind values emitted by rules.
// "gate" blocks the verdict; "advisory" surfaces but does not block.
const (
	kindGate     = "gate"
	kindAdvisory = "advisory"
)

// matchedByModule is the MatchedBy key for the owning module path, shared
// across publicAPIMax, publicAPIChange, and publicAPITypeLeak.
const matchedByModule = "module"

// matchedByFile is the MatchedBy key for the source file path, shared
// across publicAPIChange and publicAPITypeLeak.
const matchedByFile = "file"

// Evidence carries supplemental evidence provided to a rule's Check method.
// Lifecycle status (new vs baselined) is NOT evidence — it is assigned after
// rules run, by status.Assign against the baseline.
type Evidence struct {
	PatternMatches []pattern.Match
	SyntaxFacts    []evidence.SyntaxFact // nil/empty when syntax is off; consumed by public_api_max
}

// Rule is the interface implemented by every built-in and user-defined rule.
type Rule interface {
	ID() string
	Check(s relationship.Set, ev Evidence) []finding.Finding
}

// New constructs the slice of Rule values declared in cfg.
// Config type strings (snake_case per spec §9):
//
//	"forbidden_dependency"        → ForbiddenDependency
//	"public_api_only"             → PublicAPIOnly
//	"forbidden_layer_direction"   → ForbiddenLayerDirection
//	"internal_api_access"         → internalAPIAccess
//	"new_cross_module_dependency" → newCrossModuleDependency
//	"cycle"                       → cycleRule
//	"public_api_max"              → publicAPIMax
//	"public_api_change"           → publicAPIChange
//	"public_api_type_leak"        → publicAPITypeLeak
//
// Unknown type strings are a config error.
func New(cfg view.RuleConfig) ([]Rule, error) {
	rs := make([]Rule, 0, len(cfg.Rules))
	for _, def := range cfg.Rules {
		var inner Rule
		switch def.Type {
		case "forbidden_dependency":
			if err := validateForbiddenDependencyDef(def); err != nil {
				return nil, err
			}
			inner = &forbiddenDependency{def: def}
		case "public_api_only":
			if err := validateScopeGlobs(def); err != nil {
				return nil, err
			}
			inner = &publicAPIOnly{def: def, mm: cfg.ModuleMap}
		case "forbidden_layer_direction":
			inner = &forbiddenLayerDirection{
				def:    def,
				layers: cfg.Layers,
				mm:     cfg.ModuleMap,
			}
		case "internal_api_access":
			if err := validateScopeGlobs(def); err != nil {
				return nil, err
			}
			inner = &internalAPIAccess{def: def, mm: cfg.ModuleMap}
		case "new_cross_module_dependency":
			inner = &newCrossModuleDependency{def: def, mm: cfg.ModuleMap}
		case "cycle":
			inner = &cycleRule{def: def}
		case "public_api_max":
			if err := validatePublicAPIMaxDef(def); err != nil {
				return nil, err
			}
			inner = &publicAPIMax{def: def, mm: cfg.ModuleMap, max: *def.Max}
		case "public_api_change":
			inner = &publicAPIChange{def: def, mm: cfg.ModuleMap}
		case "public_api_type_leak":
			inner = &publicAPITypeLeak{def: def, mm: cfg.ModuleMap}
		default:
			return nil, fmt.Errorf("rules: unknown rule type %q (id=%q)", def.Type, def.ID)
		}
		// Apply per-rule gate default before wrapping: public_api_change defaults
		// to "warn" (advisory) when gate is unset, so it never blocks by default.
		gate := def.Gate
		if gate == "" {
			gate = defaultGateForType(def.Type)
		}
		rs = append(rs, &gatedRule{inner: inner, gate: gate})
	}
	return rs, nil
}

// defaultGateForType returns the per-type gate default for types that diverge
// from the global default of "" (= fail). public_api_change and
// public_api_type_leak default to "warn" (advisory drift signal).
func defaultGateForType(ruleType string) string {
	switch ruleType {
	case "public_api_change", "public_api_type_leak":
		return "warn"
	}
	return ""
}

// gatedRule wraps a Rule and applies gate semantics to its findings:
//   - gate "off"  → suppress all findings
//   - gate "warn" → set Kind="advisory" (non-blocking)
//   - gate "fail" or "" → pass findings through unchanged (Kind stays "gate")
type gatedRule struct {
	inner Rule
	gate  string // "off" | "warn" | "fail" | ""
}

func (r *gatedRule) ID() string { return r.inner.ID() }

func (r *gatedRule) Check(s relationship.Set, ev Evidence) []finding.Finding {
	raw := r.inner.Check(s, ev)
	switch r.gate {
	case "off":
		return nil
	case "warn":
		for i := range raw {
			raw[i].Kind = kindAdvisory
		}
		return raw
	default: // "fail" or ""
		return raw // Kind already "gate" (default from finding.New)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// layerRank returns the zero-based index of layer in layers, or -1 if absent.
func layerRank(layer string, layers []string) int {
	for i, l := range layers {
		if l == layer {
			return i
		}
	}
	return -1
}
