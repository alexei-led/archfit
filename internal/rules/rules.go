// Package rules defines the Rule interface and the built-in rule
// implementations: ForbiddenDependency, PublicAPIOnly, ForbiddenLayerDirection,
// InternalAPIAccess, NewCrossModuleDependency, CycleRule.
package rules

import (
	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// PatternMatch is a single ast-grep match (Phase 2+). Empty in Phase 1.
type PatternMatch struct {
	File  string
	Line  int
	Match string
}

// Evidence carries supplemental evidence provided to a rule's Check method.
// PatternMatches is empty in Phase 1 (no ast-grep yet).
// Findings carries status-tagged findings for rules that gate on finding status.
type Evidence struct {
	PatternMatches []PatternMatch
	Findings       []finding.Finding
}

// Rule is the interface implemented by every built-in and user-defined rule.
type Rule interface {
	ID() string
	Check(g *graph.Graph, ev Evidence) []finding.Finding
}

// New constructs the slice of Rule values declared in cfg.
// Config type strings (snake_case per spec §9):
//
//	"forbidden_dependency"      → ForbiddenDependency
//	"public_api_only"           → PublicAPIOnly
//	"forbidden_layer_direction" → ForbiddenLayerDirection
//
// Unknown type strings are silently skipped.
//
// Note: RuleDef.Gate ("fail"/"warn") is intentionally not consumed here.
// In Phase 1, all rules produce gate findings (Kind="gate"). The advisory
// severity channel (gate:warn) is deferred to Phase 2.
func New(cfg config.RuleConfig) []Rule {
	rules := make([]Rule, 0, len(cfg.Rules))
	for _, def := range cfg.Rules {
		switch def.Type {
		case "forbidden_dependency":
			rules = append(rules, &forbiddenDependency{def: def})
		case "public_api_only":
			rules = append(rules, &publicAPIOnly{def: def})
		case "forbidden_layer_direction":
			rules = append(rules, &forbiddenLayerDirection{
				def:    def,
				layers: cfg.Layers,
				mm:     cfg.ModuleMap,
			})
		}
	}
	return rules
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

// ---------------------------------------------------------------------------
// ForbiddenDependency
// ---------------------------------------------------------------------------

type forbiddenDependency struct {
	def config.RuleDef
}

func (r *forbiddenDependency) ID() string { return r.def.ID }

func (r *forbiddenDependency) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

		fromMatch, _ := doublestar.Match(r.def.From, fromPath)
		toMatch, _ := doublestar.Match(r.def.To, toPath)
		if !fromMatch || !toMatch {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"from_glob": r.def.From,
			"to_glob":   r.def.To,
		}
		f.Why = "Import from " + r.def.From + " to " + r.def.To + " is explicitly forbidden"
		f.Constraint = "Remove the dependency or move the code"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// PublicAPIOnly
// ---------------------------------------------------------------------------

type publicAPIOnly struct {
	def config.RuleDef
}

func (r *publicAPIOnly) ID() string { return r.def.ID }

func (r *publicAPIOnly) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		if e.Kind != graph.EdgeKindUsesInternal {
			continue
		}
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

		// Apply From/To scope globs when set; empty glob means match-all.
		if r.def.From != "" {
			if matched, _ := doublestar.Match(r.def.From, fromPath); !matched {
				continue
			}
		}
		if r.def.To != "" {
			if matched, _ := doublestar.Match(r.def.To, toPath); !matched {
				continue
			}
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"edge_kind": string(e.Kind),
			"to_path":   toPath,
		}
		f.Why = "Cross-module access to internal path " + toPath
		f.Constraint = "Only import from the module's public API"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// ForbiddenLayerDirection
// ---------------------------------------------------------------------------

type forbiddenLayerDirection struct {
	def    config.RuleDef
	layers []string
	mm     config.ModuleMap
}

func (r *forbiddenLayerDirection) ID() string { return r.def.ID }

func (r *forbiddenLayerDirection) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

		fromLayer, ok := r.mm.LayerFor(fromPath)
		if !ok {
			continue
		}
		toLayer, ok := r.mm.LayerFor(toPath)
		if !ok {
			continue
		}

		fromRank := layerRank(fromLayer, r.layers)
		toRank := layerRank(toLayer, r.layers)

		// Skip if either layer is not in the ordered list.
		if fromRank < 0 || toRank < 0 {
			continue
		}

		// Violation: dependency flows from lower-index (higher-priority/inner) to
		// higher-index (lower-priority/outer) layer — the wrong direction.
		// E.g. layers=[domain(0), application(1), infrastructure(2)]:
		//   domain(0) → infrastructure(2) is forbidden (fromRank < toRank).
		//   infrastructure(2) → domain(0) is allowed  (fromRank > toRank).
		if fromRank >= toRank {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"from_layer": fromLayer,
			"to_layer":   toLayer,
		}
		f.Why = "Dependency from layer " + fromLayer + " to layer " + toLayer + " violates layer ordering"
		f.Constraint = "Dependencies must flow from higher layers to lower layers"
		out = append(out, f)
	}
	return out
}
