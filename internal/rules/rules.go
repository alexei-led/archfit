// Package rules defines the Rule interface and the built-in rule
// implementations: ForbiddenDependency, PublicAPIOnly, ForbiddenLayerDirection,
// InternalAPIAccess, NewCrossModuleDependency, CycleRule, ForbiddenRoleDependency,
// PublicAPIMax, PublicAPIChange.
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/syntax"
)

// kindGate and kindAdvisory are the two Finding.Kind values emitted by rules.
// "gate" blocks the verdict; "advisory" surfaces but does not block.
const (
	kindGate     = "gate"
	kindAdvisory = "advisory"
)

// matchedByModule is the MatchedBy key for the owning module path, shared
// across publicAPIMax, publicAPIChange, structFieldMax, and publicAPITypeLeak.
const matchedByModule = "module"

// matchedByFile is the MatchedBy key for the source file path, shared
// across publicAPIChange, testInProduction, and publicAPITypeLeak.
const matchedByFile = "file"

// Evidence carries supplemental evidence provided to a rule's Check method.
// Lifecycle status (new vs baselined) is NOT evidence — it is assigned after
// rules run, by status.Assign against the baseline.
type Evidence struct {
	PatternMatches []pattern.Match
	Roles          *syntax.NodeRoleIndex   // nil when syntax is off; consumed by forbidden_role_dependency
	SyntaxFacts    []diagnostic.SyntaxFact // nil/empty when syntax is off; consumed by public_api_max
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
//	"forbidden_role_dependency"   → forbiddenRoleDependency
//	"public_api_max"              → publicAPIMax
//	"public_api_change"           → publicAPIChange
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
		case "forbidden_role_dependency":
			if err := validateRoleDependencyDef(def); err != nil {
				return nil, err
			}
			inner = &forbiddenRoleDependency{def: def}
		case "public_api_max":
			if err := validatePublicAPIMaxDef(def); err != nil {
				return nil, err
			}
			inner = &publicAPIMax{def: def, mm: cfg.ModuleMap, max: *def.Max}
		case "public_api_change":
			inner = &publicAPIChange{def: def, mm: cfg.ModuleMap}
		case "test_in_production":
			inner = &testInProduction{def: def, mm: cfg.ModuleMap}
		case "public_api_type_leak":
			inner = &publicAPITypeLeak{def: def, mm: cfg.ModuleMap}
		case "struct_field_max":
			if err := validateStructFieldMaxDef(def); err != nil {
				return nil, err
			}
			inner = &structFieldMax{def: def, mm: cfg.ModuleMap, max: *def.Max}
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
// test_in_production default to "warn" (advisory drift signal).
func defaultGateForType(ruleType string) string {
	switch ruleType {
	case "public_api_change", "test_in_production", "struct_field_max", "public_api_type_leak":
		return "warn"
	}
	return ""
}

// validateRoleDependencyDef validates a RuleDef for the forbidden_role_dependency
// rule type. Returns an error describing any invalid field.
func validateRoleDependencyDef(def config.RuleDef) error {
	if def.FromRole == "" || def.ToRole == "" {
		return fmt.Errorf("rules: forbidden_role_dependency %q requires both from_role and to_role", def.ID)
	}
	if !slices.Contains(syntax.KnownRoles, def.FromRole) {
		return fmt.Errorf("rules: forbidden_role_dependency %q: unknown from_role %q (known: %s)", def.ID, def.FromRole, strings.Join(syntax.KnownRoles, ", "))
	}
	if !slices.Contains(syntax.KnownRoles, def.ToRole) {
		return fmt.Errorf("rules: forbidden_role_dependency %q: unknown to_role %q (known: %s)", def.ID, def.ToRole, strings.Join(syntax.KnownRoles, ", "))
	}
	if def.MinConfidence != "" && !slices.Contains(syntax.KnownConfidences, def.MinConfidence) {
		return fmt.Errorf("rules: forbidden_role_dependency %q: unknown min_confidence %q (known: %s)", def.ID, def.MinConfidence, strings.Join(syntax.KnownConfidences, ", "))
	}
	return nil
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

// ---------------------------------------------------------------------------
// ForbiddenRoleDependency
// ---------------------------------------------------------------------------

// forbiddenRoleDependency fires when an edge exists from a node whose role
// matches def.FromRole (at or above def.MinConfidence) to a node whose role
// matches def.ToRole (at or above def.MinConfidence). When def.MinConfidence
// is empty, syntax.ConfHigh is used as the default threshold. When
// ev.Roles is nil (syntax off), the rule silently returns nil.
type forbiddenRoleDependency struct {
	def config.RuleDef
}

func (r *forbiddenRoleDependency) ID() string { return r.def.ID }

func (r *forbiddenRoleDependency) Check(g *graph.Graph, ev Evidence) []finding.Finding {
	if ev.Roles == nil {
		return nil
	}
	minConf := r.def.MinConfidence
	if minConf == "" {
		minConf = syntax.ConfHigh
	}

	var out []finding.Finding
	for _, e := range g.Edges() {
		fromHits := ev.Roles.RolesFor(e.From)
		toHits := ev.Roles.RolesFor(e.To)

		fromMatch := false
		for _, h := range fromHits {
			if h.Role == r.def.FromRole && syntax.ConfidenceMeets(h.Confidence, minConf) {
				fromMatch = true
				break
			}
		}
		if !fromMatch {
			continue
		}

		toMatch := false
		for _, h := range toHits {
			if h.Role == r.def.ToRole && syntax.ConfidenceMeets(h.Confidence, minConf) {
				toMatch = true
				break
			}
		}
		if !toMatch {
			continue
		}

		f := finding.New(r.def.ID, e, e.Locations)
		f.Severity = finding.SeverityHigh
		f.MatchedBy = map[string]string{
			"from_role": r.def.FromRole,
			"to_role":   r.def.ToRole,
		}
		f.Why = "Role dependency from " + r.def.FromRole + " to " + r.def.ToRole + " is forbidden"
		f.Constraint = "Remove the dependency or restructure the architectural layers"
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

// ---------------------------------------------------------------------------
// PublicAPIMax
// ---------------------------------------------------------------------------

// validatePublicAPIMaxDef validates a RuleDef for the public_api_max rule type.
func validatePublicAPIMaxDef(def config.RuleDef) error {
	if def.Max == nil {
		return fmt.Errorf("rules: public_api_max %q requires max to be set", def.ID)
	}
	if *def.Max < 0 {
		return fmt.Errorf("rules: public_api_max %q: max must be non-negative, got %d", def.ID, *def.Max)
	}
	return nil
}

// publicAPIMax fires when a module's exported-declaration count exceeds the
// configured maximum. It counts exported SyntaxFacts per module (via
// ModuleMap file→module resolution) and emits one finding per violating module.
// When ev.SyntaxFacts is empty (syntax off), the rule returns nil silently.
type publicAPIMax struct {
	def config.RuleDef
	mm  config.ModuleMap
	max int
}

func (r *publicAPIMax) ID() string { return r.def.ID }

func (r *publicAPIMax) Check(_ *graph.Graph, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	// Count exported declarations per module.
	counts := make(map[string]int)
	for _, f := range ev.SyntaxFacts {
		if !f.Exported {
			continue
		}
		mod, ok := r.mm.ModuleFor(f.File)
		if !ok {
			continue // file not owned by any declared module — skip
		}
		counts[mod]++
	}

	if len(counts) == 0 {
		return nil
	}

	// Emit one finding per module that exceeds the limit.
	// Sort module names for deterministic output.
	modules := make([]string, 0, len(counts))
	for mod := range counts {
		modules = append(modules, mod)
	}
	sort.Strings(modules)

	var out []finding.Finding
	for _, mod := range modules {
		count := counts[mod]
		if count <= r.max {
			continue
		}
		h := sha256.Sum256([]byte(r.def.ID + "\x00" + mod))
		f := finding.Finding{
			ID:       hex.EncodeToString(h[:16]),
			Kind:     kindGate, // pre-wrap default; gatedRule overrides to kindAdvisory when gate: warn
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityMedium,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: mod},
				To:   finding.Endpoint{Path: mod},
			},
			MatchedBy: map[string]string{
				matchedByModule: mod,
				"count":         strconv.Itoa(count),
				"max":           strconv.Itoa(r.max),
			},
			Why:        fmt.Sprintf("Module %q has %d exported declarations, exceeding the limit of %d", mod, count, r.max),
			Constraint: "Reduce the public API surface or raise the max threshold",
		}
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// PublicAPIChange
// ---------------------------------------------------------------------------

// publicAPIChange emits one finding per exported declaration per module, using
// the baseline/status stage (status.Assign) to surface newly-added public API
// as StatusNew. It mirrors newCrossModuleDependency: the rule emits every
// exported decl unconditionally; baseline suppression happens outside the rule.
//
// Fingerprint = ruleID + "\x00" + module + "\x00" + name, so two same-named
// decls in one module map to the same ID — deduplication ensures at most one
// finding per (module, name) pair.
//
// When ev.SyntaxFacts is empty (syntax off), the rule returns nil silently.
// Default gate is "warn" (advisory drift signal), applied at construction time
// in New via defaultGateForType.
type publicAPIChange struct {
	def config.RuleDef
	mm  config.ModuleMap
}

func (r *publicAPIChange) ID() string { return r.def.ID }

func (r *publicAPIChange) Check(_ *graph.Graph, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	// Collect unique (module, name) pairs for exported declarations.
	type key struct{ mod, name string }
	seen := make(map[key]struct{})

	var out []finding.Finding
	for _, f := range ev.SyntaxFacts {
		if !f.Exported {
			continue
		}
		mod, ok := r.mm.ModuleFor(f.File)
		if !ok {
			continue // file not owned by any declared module — skip
		}
		k := key{mod: mod, name: f.Name}
		if _, dup := seen[k]; dup {
			continue // same (module, name) already emitted — dedup
		}
		seen[k] = struct{}{}
		// Ceiling: two TS exports with identical names across different files collapse to one finding.
		h := sha256.Sum256([]byte(r.def.ID + "\x00" + mod + "\x00" + f.Name))
		fnd := finding.Finding{
			ID:       hex.EncodeToString(h[:16]),
			Kind:     kindGate, // pre-wrap default; gatedRule overrides to kindAdvisory when gate: warn
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityLow,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: mod},
				To:   finding.Endpoint{Path: mod},
			},
			MatchedBy: map[string]string{
				matchedByModule: mod,
				"name":          f.Name,
				"kind":          f.Kind,
				matchedByFile:   f.File,
			},
			Why:        fmt.Sprintf("Exported declaration %q added to module %q (%s in %s)", f.Name, mod, f.Kind, f.File),
			Constraint: "Review new public API additions; baseline when intentional",
		}
		out = append(out, fnd)
	}
	return out
}

// ---------------------------------------------------------------------------
// TestInProduction
// ---------------------------------------------------------------------------

// testInProduction fires when a production (non-test) file imports a test
// framework. This detects test code compiled into the production binary —
// the canonical case is mock files with package X (no _test.go suffix) that
// import testify/mock or gomock.
//
// Evidence: SyntaxFacts of Kind="test_import" whose File passes IsTestFile=false.
// One finding per (file, framework) pair. When ev.SyntaxFacts is empty, returns nil.
// Default gate is "warn" (advisory, non-blocking); config can promote to "fail".
type testInProduction struct {
	def config.RuleDef
	mm  config.ModuleMap
}

func (r *testInProduction) ID() string { return r.def.ID }

func (r *testInProduction) Check(_ *graph.Graph, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	// Deduplicate on (file, framework) to avoid repeated identical findings.
	type key struct{ file, framework string }
	seen := make(map[key]struct{})

	var out []finding.Finding
	for _, f := range ev.SyntaxFacts {
		if f.Kind != "test_import" {
			continue
		}
		if syntax.IsTestFile(f.Language, f.File) {
			continue // expected: test files may import test frameworks
		}

		k := key{file: f.File, framework: f.Framework}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}

		// Use module path for the endpoint when the file is owned by a declared module.
		endpoint := f.File
		if mod, ok := r.mm.ModuleFor(f.File); ok {
			endpoint = mod
		}

		h := sha256.Sum256([]byte(r.def.ID + "\x00" + f.File + "\x00" + f.Framework))
		fnd := finding.Finding{
			ID:       hex.EncodeToString(h[:16]),
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: endpoint},
				To:   finding.Endpoint{Path: endpoint},
			},
			MatchedBy: map[string]string{
				matchedByFile: f.File,
				"framework":   f.Framework,
				"language":    f.Language,
			},
			Why:        fmt.Sprintf("Production file %q imports test framework %q", f.File, f.Framework),
			Constraint: "Move to *_test.go or add a build tag; test frameworks must not ship in the production binary",
		}
		out = append(out, fnd)
	}
	return out
}

// ---------------------------------------------------------------------------
// StructFieldMax
// ---------------------------------------------------------------------------

// validateStructFieldMaxDef validates a RuleDef for the struct_field_max rule type.
func validateStructFieldMaxDef(def config.RuleDef) error {
	if def.Max == nil {
		return fmt.Errorf("rules: struct_field_max %q requires max to be set", def.ID)
	}
	if *def.Max < 0 {
		return fmt.Errorf("rules: struct_field_max %q: max must be non-negative, got %d", def.ID, *def.Max)
	}
	return nil
}

// structFieldMax fires when a struct's estimated field count exceeds the
// configured maximum. It uses struct_field SyntaxFacts (one per exported
// struct; Count = line-range heuristic) and emits one finding per violating
// struct per module. When ev.SyntaxFacts is empty (syntax off), returns nil.
// Default gate is "warn"; config can promote to "fail".
//
// Field count source: SyntaxFact.Count when > 0 (line-range heuristic from
// the go-struct-field/rs-struct-field rules). Facts with Count == 0 are
// skipped (tuple/unit structs with no named fields).
type structFieldMax struct {
	def config.RuleDef
	mm  config.ModuleMap
	max int
}

func (r *structFieldMax) ID() string { return r.def.ID }

func (r *structFieldMax) Check(_ *graph.Graph, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	// Collect max field count per (module, struct-name).
	// A struct name may appear in multiple files of the same module;
	// use the maximum count to surface the worst case.
	type key struct{ mod, name string }
	maxCounts := make(map[key]int)
	for _, f := range ev.SyntaxFacts {
		if f.Kind != "struct_field" || f.Count == 0 {
			continue
		}
		mod, ok := r.mm.ModuleFor(f.File)
		if !ok {
			continue
		}
		k := key{mod: mod, name: f.Name}
		if f.Count > maxCounts[k] {
			maxCounts[k] = f.Count
		}
	}

	if len(maxCounts) == 0 {
		return nil
	}

	// Sort for deterministic output.
	type entry struct {
		mod, name string
		count     int
	}
	entries := make([]entry, 0, len(maxCounts))
	for k, c := range maxCounts {
		entries = append(entries, entry{mod: k.mod, name: k.name, count: c})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].mod != entries[j].mod {
			return entries[i].mod < entries[j].mod
		}
		return entries[i].name < entries[j].name
	})

	var out []finding.Finding
	for _, e := range entries {
		if e.count <= r.max {
			continue
		}
		h := sha256.Sum256([]byte(r.def.ID + "\x00" + e.mod + "\x00" + e.name))
		f := finding.Finding{
			ID:       hex.EncodeToString(h[:16]),
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityMedium,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: e.mod},
				To:   finding.Endpoint{Path: e.mod},
			},
			MatchedBy: map[string]string{
				matchedByModule: e.mod,
				"struct":        e.name,
				"count":         strconv.Itoa(e.count),
				"max":           strconv.Itoa(r.max),
			},
			Why:        fmt.Sprintf("Struct %q in module %q has ~%d fields, exceeding the limit of %d", e.name, e.mod, e.count, r.max),
			Constraint: "Consider splitting this struct or extracting nested concerns",
		}
		out = append(out, f)
	}
	return out
}

// ---------------------------------------------------------------------------
// PublicAPITypeLeak
// ---------------------------------------------------------------------------

// versionSegmentRe matches trailing versioned path segments like "v2", "v3".
var versionSegmentRe = regexp.MustCompile(`^v\d+$`)

// externalPackageSegments builds a set of last meaningful path segments for
// every NodeKindPackage node whose path looks like a fully-qualified import
// path (contains a "."). Versioned suffixes (/v2, /v3, …) are skipped so that
// "github.com/urfave/cli/v2" maps to "cli", not "v2".
//
// Ceiling: first-party check uses graph's NodeKindPackage with dotted paths
// (Go-style). Rust/TS external nodes use NodeKindExternal; extend if adding
// those languages.
func externalPackageSegments(g *graph.Graph) map[string]struct{} {
	set := make(map[string]struct{})
	for _, n := range g.Nodes() {
		if n.Kind != graph.NodeKindPackage {
			continue
		}
		if !strings.Contains(n.Path, ".") {
			continue // not a fully-qualified import path (no dot → first-party or stdlib)
		}
		// Find the last segment that is not a version tag.
		segs := strings.Split(n.Path, "/")
		for i := len(segs) - 1; i >= 0; i-- {
			if !versionSegmentRe.MatchString(segs[i]) {
				set[path.Base(segs[i])] = struct{}{}
				break
			}
		}
	}
	return set
}

// publicAPITypeLeak fires when a type_leak SyntaxFact's package selector
// matches a known external package node in the graph. It reports one finding
// per (module, leaked-type-name) pair. Report-only by default (gate: warn).
//
// When ev.SyntaxFacts is empty (syntax off), the rule returns nil silently.
type publicAPITypeLeak struct {
	def config.RuleDef
	mm  config.ModuleMap
}

func (r *publicAPITypeLeak) ID() string { return r.def.ID }

func (r *publicAPITypeLeak) Check(g *graph.Graph, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	extPkgs := externalPackageSegments(g)
	if len(extPkgs) == 0 {
		return nil
	}

	type key struct{ mod, name string }
	seen := make(map[key]struct{})

	var out []finding.Finding
	for _, f := range ev.SyntaxFacts {
		if f.Kind != "type_leak" {
			continue
		}
		// Name is "pkg.Type"; split on first dot.
		pkg, _, ok := strings.Cut(f.Name, ".")
		if !ok || pkg == "" {
			continue
		}
		if _, isExternal := extPkgs[pkg]; !isExternal {
			continue
		}
		mod, modOK := r.mm.ModuleFor(f.File)
		if !modOK {
			mod = f.File // fall back to file path when module is unowned
		}
		k := key{mod: mod, name: f.Name}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}

		h := sha256.Sum256([]byte(r.def.ID + "\x00" + mod + "\x00" + f.Name))
		fnd := finding.Finding{
			ID:       hex.EncodeToString(h[:16]),
			Kind:     kindGate,
			RuleID:   r.def.ID,
			Status:   finding.StatusNew,
			Severity: finding.SeverityMedium,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: mod},
				To:   finding.Endpoint{Path: mod},
			},
			MatchedBy: map[string]string{
				matchedByModule: mod,
				"type":          f.Name,
				matchedByFile:   f.File,
			},
			Why:        fmt.Sprintf("Module %q leaks external type %q in its public API (file: %s)", mod, f.Name, f.File),
			Constraint: "Replace the external type with an internal abstraction or alias at the module boundary",
		}
		out = append(out, fnd)
	}
	return out
}
