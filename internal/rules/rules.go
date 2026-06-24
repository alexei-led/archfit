// Package rules defines the Rule interface and the built-in rule
// implementations: ForbiddenDependency, PublicAPIOnly, ForbiddenLayerDirection,
// InternalAPIAccess, NewCrossModuleDependency, CycleRule.
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/syntax"
)

// Evidence carries supplemental evidence provided to a rule's Check method.
// Lifecycle status (new vs baselined) is NOT evidence — it is assigned after
// rules run, by status.Assign against the baseline.
type Evidence struct {
	PatternMatches []pattern.Match
	Roles          *syntax.NodeRoleIndex // nil when syntax is off; consumed by Task-11 rules
}

// Rule is the interface implemented by every built-in and user-defined rule.
type Rule interface {
	ID() string
	Check(g *graph.Graph, ev Evidence) []finding.Finding
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
//
// Unknown type strings are a config error.
func New(cfg config.RuleConfig) ([]Rule, error) {
	rs := make([]Rule, 0, len(cfg.Rules))
	for _, def := range cfg.Rules {
		var inner Rule
		switch def.Type {
		case "forbidden_dependency":
			inner = &forbiddenDependency{def: def}
		case "public_api_only":
			inner = &publicAPIOnly{def: def}
		case "forbidden_layer_direction":
			inner = &forbiddenLayerDirection{
				def:    def,
				layers: cfg.Layers,
				mm:     cfg.ModuleMap,
			}
		case "internal_api_access":
			inner = &internalAPIAccess{def: def}
		case "new_cross_module_dependency":
			inner = &newCrossModuleDependency{def: def, mm: cfg.ModuleMap}
		case "cycle":
			inner = &cycleRule{def: def}
		default:
			return nil, fmt.Errorf("rules: unknown rule type %q (id=%q)", def.Type, def.ID)
		}
		rs = append(rs, &gatedRule{inner: inner, gate: def.Gate})
	}
	return rs, nil
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

func (r *gatedRule) Check(g *graph.Graph, ev Evidence) []finding.Finding {
	raw := r.inner.Check(g, ev)
	switch r.gate {
	case "off":
		return nil
	case "warn":
		for i := range raw {
			raw[i].Kind = "advisory"
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

// ---------------------------------------------------------------------------
// InternalAPIAccess
// ---------------------------------------------------------------------------

// internalAPIAccess fires on edges with kind == uses_internal, optionally
// filtered by from/to glob. Supports the same from/to glob semantics as
// publicAPIOnly but is a distinct rule type so teams can configure them
// independently with different IDs, severities, and exceptions.
type internalAPIAccess struct {
	def config.RuleDef
}

func (r *internalAPIAccess) ID() string { return r.def.ID }

func (r *internalAPIAccess) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		if e.Kind != graph.EdgeKindUsesInternal {
			continue
		}
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

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
			"from_path": fromPath,
			"to_path":   toPath,
		}
		f.Why = "Access to internal API path " + toPath + " from " + fromPath
		f.Constraint = "Only import from the module's public API surface"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// NewCrossModuleDependency
// ---------------------------------------------------------------------------

// newCrossModuleDependency fires when an edge crosses module boundaries.
// It uses ModuleMap to determine module ownership of from/to paths.
//
// "New" semantics are deliberately NOT implemented here: the rule emits every
// cross-module edge, and the status stage (status.Assign) marks edges whose
// fingerprint is in the baseline as StatusBaseline — only StatusNew /
// StatusExpiredException findings gate. Filtering inside the rule would break
// fixed-finding detection (a suppressed finding's fingerprint would vanish
// from the current set and be falsely reported as fixed). Bootstrap behavior:
// with no baseline every cross-module edge fires — run `archfit baseline` to
// accept the current state.
type newCrossModuleDependency struct {
	def config.RuleDef
	mm  config.ModuleMap
}

func (r *newCrossModuleDependency) ID() string { return r.def.ID }

func (r *newCrossModuleDependency) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	var out []finding.Finding
	for _, e := range g.Edges() {
		fromPath := graph.NodePath(e.From)
		toPath := graph.NodePath(e.To)

		fromModule, fromOK := r.mm.ModuleFor(fromPath)
		toModule, toOK := r.mm.ModuleFor(toPath)

		// Skip edges where either endpoint is unowned or both are in the same module.
		if !fromOK || !toOK || fromModule == toModule {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityMedium
		f.MatchedBy = map[string]string{
			"from_module": fromModule,
			"to_module":   toModule,
		}
		f.Why = fmt.Sprintf("New cross-module dependency from %q (%s) to %q (%s)", fromPath, fromModule, toPath, toModule)
		f.Constraint = "Cross-module dependencies must be reviewed and approved"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// CycleRule
// ---------------------------------------------------------------------------

// cycleRule detects import cycles using Graph.Cycles() (shared Tarjan SCC).
// It emits one finding per strongly-connected component of size > 1.
// The finding ID is derived from the sorted SCC members for stability.
type cycleRule struct {
	def config.RuleDef
}

func (r *cycleRule) ID() string { return r.def.ID }

func (r *cycleRule) Check(g *graph.Graph, _ Evidence) []finding.Finding {
	sccs := g.Cycles()
	if len(sccs) == 0 {
		return nil
	}

	out := make([]finding.Finding, 0, len(sccs))
	for _, scc := range sccs {
		id := cycleFingerprintID(r.def.ID, scc)
		// Use the first two members of the SCC as representative from/to for the edge evidence.
		fromPath := graph.NodePath(scc[0])
		toPath := graph.NodePath(scc[1%len(scc)])
		f := finding.Finding{
			ID:       id,
			Kind:     "gate",
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: fromPath},
				To:   finding.Endpoint{Path: toPath},
				Kind: string(graph.EdgeKindImports),
			},
			MatchedBy: map[string]string{
				"cycle_members": strings.Join(scc, ", "),
				"cycle_size":    strconv.Itoa(len(scc)),
			},
			Why:        fmt.Sprintf("Import cycle detected among %d nodes: %s", len(scc), strings.Join(scc, " → ")),
			Constraint: "Break the cycle by introducing an abstraction or reorganizing packages",
		}
		out = append(out, f)
	}
	return out
}

// cycleFingerprintID computes a stable 32-char hex ID for a cycle finding
// from the rule ID and the sorted SCC members.
func cycleFingerprintID(ruleID string, scc []string) string {
	h := sha256.Sum256([]byte(ruleID + "\x00" + strings.Join(scc, "\x00")))
	return hex.EncodeToString(h[:16])
}
