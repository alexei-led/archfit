package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/view"
)

// ---------------------------------------------------------------------------
// ForbiddenDependency
// ---------------------------------------------------------------------------

// validateForbiddenDependencyDef validates a RuleDef for the
// forbidden_dependency rule type. An empty from/to glob matches nothing, so
// the rule would load clean yet never fire — a silently-vacuous gate.
func validateForbiddenDependencyDef(def view.RuleDef) error {
	if def.From == "" || def.To == "" {
		return fmt.Errorf("rules: forbidden_dependency %q requires both from and to globs", def.ID)
	}
	return validateScopeGlobs(def)
}

// validateScopeGlobs rejects malformed from/to globs. doublestar.Match
// returns ErrBadPattern at check time, which Check discards — a malformed
// glob would make the rule silently fire zero findings forever, the same
// silently-vacuous-gate failure the emptiness check above guards against.
func validateScopeGlobs(def view.RuleDef) error {
	for field, pat := range map[string]string{"from": def.From, "to": def.To} {
		if pat != "" && !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("rules: rule %q has a malformed %s glob %q", def.ID, field, pat)
		}
	}
	return nil
}

type forbiddenDependency struct {
	def view.RuleDef
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

// sameModule reports whether fromPath and toPath resolve to the same module —
// a module reaching into its own internal path (e.g. domain importing
// domain/internal) is idiomatic, not a violation; only cross-module access to
// another module's internal surface is. When either endpoint isn't covered by
// the module map, we can't rule out same-module, so callers must treat that
// as "not same module" (module-blind fallback: the edge still fires).
func sameModule(mm module.Map, fromPath, toPath string) bool {
	fromModule, fromOK := mm.ModuleFor(fromPath)
	toModule, toOK := mm.ModuleFor(toPath)
	return fromOK && toOK && fromModule == toModule
}

// ---------------------------------------------------------------------------
// PublicAPIOnly
// ---------------------------------------------------------------------------

type publicAPIOnly struct {
	def view.RuleDef
	mm  module.Map
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

		if sameModule(r.mm, fromPath, toPath) {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"edge_kind": string(e.Kind),
			"to_path":   toPath,
		}
		why := "Access to internal path " + toPath
		if fromModule, fromOK := r.mm.ModuleFor(fromPath); fromOK {
			if toModule, toOK := r.mm.ModuleFor(toPath); toOK {
				why = fmt.Sprintf("Cross-module access from %q (%s) to internal path %q (%s)", fromPath, fromModule, toPath, toModule)
			}
		}
		f.Why = why
		f.Constraint = "Only import from the module's public API"
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// ForbiddenLayerDirection
// ---------------------------------------------------------------------------

type forbiddenLayerDirection struct {
	def    view.RuleDef
	layers []string
	mm     module.Map
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
// filtered by from/to glob. Supports the same from/to glob and module-map
// semantics as publicAPIOnly but is a distinct rule type so teams can
// configure them independently with different IDs, severities, and exceptions.
type internalAPIAccess struct {
	def view.RuleDef
	mm  module.Map
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

		if sameModule(r.mm, fromPath, toPath) {
			continue
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
// StatusExpiredWaiver findings gate. Filtering inside the rule would break
// fixed-finding detection (a suppressed finding's fingerprint would vanish
// from the current set and be falsely reported as fixed). Bootstrap behavior:
// with no baseline every cross-module edge fires — run `archfit baseline` to
// accept the current state.
type newCrossModuleDependency struct {
	def view.RuleDef
	mm  module.Map
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
	def view.RuleDef
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
			Kind:     kindGate,
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
