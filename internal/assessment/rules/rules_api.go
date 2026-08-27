package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// ---------------------------------------------------------------------------
// PublicAPIMax
// ---------------------------------------------------------------------------

// validatePublicAPIMaxDef validates a RuleDef for the public_api_max rule type.
func validatePublicAPIMaxDef(def policy.RuleDef) error {
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
	def policy.RuleDef
	mm  policy.ModuleMap
	max int
}

func (r *publicAPIMax) ID() string { return r.def.ID }

func (r *publicAPIMax) Check(_ relationship.Set, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	// Count exported declarations per module, tracking every declaring file so
	// the finding's Locations carry real files — never just the module name.
	counts := make(map[string]int)
	filesByModule := make(map[string]map[string]struct{})
	for _, f := range ev.SyntaxFacts {
		if !f.Exported {
			continue
		}
		mod, ok := r.mm.ModuleForFile(f.File)
		if !ok {
			continue // file not owned by any declared module — skip
		}
		counts[mod]++
		if filesByModule[mod] == nil {
			filesByModule[mod] = make(map[string]struct{})
		}
		filesByModule[mod][f.File] = struct{}{}
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
		locs := make([]relationship.Location, 0, len(filesByModule[mod]))
		for file := range filesByModule[mod] {
			locs = append(locs, relationship.Location{File: file})
		}
		sort.Slice(locs, func(i, j int) bool { return locs[i].File < locs[j].File })

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
			Locations:  locs,
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
	def policy.RuleDef
	mm  policy.ModuleMap
}

func (r *publicAPIChange) ID() string { return r.def.ID }

func (r *publicAPIChange) Check(_ relationship.Set, ev Evidence) []finding.Finding {
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
		mod, ok := r.mm.ModuleForFile(f.File)
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
			Locations:  []relationship.Location{{File: f.File, Line: f.StartLine}},
			Why:        fmt.Sprintf("Exported declaration %q added to module %q (%s in %s)", f.Name, mod, f.Kind, f.File),
			Constraint: "Review new public API additions; baseline when intentional",
		}
		out = append(out, fnd)
	}
	return out
}

// ---------------------------------------------------------------------------
// PublicAPITypeLeak
// ---------------------------------------------------------------------------

// versionSegmentRe matches trailing versioned path segments like "v2", "v3".
var versionSegmentRe = regexp.MustCompile(`^v\d+$`)

// Relationship node/edge kind literals used by the type-leak rule. The rule
// consumes the narrow relationship contract, whose Kind fields are strings
// rather than the graph package's typed kind constants.
const (
	relNodeKindPackage  = "package"
	relNodeKindExternal = "external"
	relEdgeKindImports  = "imports"
	relEdgeKindUsesInt  = "uses_internal"
)

// externalPackageSegments builds a set of last meaningful path segments for
// every package node whose path looks like a fully-qualified import path
// (contains a "."), every external node, and the targets of imports/uses_internal
// edges. The edge-target scan is required because the Go extractor emits
// external packages only as edge targets — never as nodes — so without it the
// set is always empty on real Go graphs.
// Versioned suffixes (/v2, /v3, …) are skipped so that
// "github.com/urfave/cli/v2" maps to "cli", not "v2".
//
// Precision ceiling: the type_leak fact carries only the package selector basename
// (e.g. "cli"), not a full import path. A repo with both "github.com/urfave/cli/v2"
// (external) AND a first-party package "cli" cannot be precisely disambiguated here
// without per-file resolution. The prior first-party-collision guard removed the
// external segment globally, which caused false negatives — missing real leaks when
// any first-party package shared a basename with an external one. This is the worst
// error for a candidate surfacer (report-only, default gate: warn). The guard is
// removed: a false positive (flagging a first-party-type reference as a leak) is
// acceptable; a false negative (silently missing a real external leak) is not.
func externalPackageSegments(s relationship.Set) map[string]struct{} {
	// addExternal extracts and registers the last non-version segment of a dotted
	// import path. No first-party-collision guard — bias to surface, not suppress.
	set := make(map[string]struct{})
	addExternal := func(importPath string) {
		if !strings.Contains(importPath, ".") {
			return // not a fully-qualified import path
		}
		segs := strings.Split(importPath, "/")
		for i := len(segs) - 1; i >= 0; i-- {
			if !versionSegmentRe.MatchString(segs[i]) {
				set[path.Base(segs[i])] = struct{}{}
				break
			}
		}
	}

	// Scan package nodes (any extractor that emits package nodes).
	for _, n := range s.Nodes {
		if n.Kind == relNodeKindPackage {
			addExternal(n.Path)
		}
	}

	// Scan external nodes (Rust/TS extractors).
	for _, n := range s.Nodes {
		if n.Kind == relNodeKindExternal {
			addExternal(n.Path)
		}
	}

	// Scan edge targets: Go emits external packages only as edge targets, never
	// as nodes. Parse the kind:path ID to recover the import path.
	for _, e := range s.Edges {
		if e.Kind != relEdgeKindImports && e.Kind != relEdgeKindUsesInt {
			continue
		}
		before, importPath, ok := strings.Cut(e.ToID, ":")
		if !ok {
			continue
		}
		if before != relNodeKindPackage && before != relNodeKindExternal {
			continue
		}
		addExternal(importPath)
	}

	return set
}

// publicAPITypeLeak fires when a type_leak SyntaxFact's package selector
// matches a known external package node in the graph. It reports one finding
// per (module, leaked-type-name) pair. Report-only by default (gate: warn).
//
// When ev.SyntaxFacts is empty (syntax off), the rule returns nil silently.
type publicAPITypeLeak struct {
	def policy.RuleDef
	mm  policy.ModuleMap
}

func (r *publicAPITypeLeak) ID() string { return r.def.ID }

func (r *publicAPITypeLeak) Check(s relationship.Set, ev Evidence) []finding.Finding {
	if len(ev.SyntaxFacts) == 0 {
		return nil
	}

	extPkgs := externalPackageSegments(s)
	// Ceiling: extPkgs is built from dotted-package external nodes (Go-style).
	// For Rust/TS/Python repos with no such nodes, type_leak facts are present
	// but cannot be matched — this check fires only when the graph has Go-style
	// external package nodes.
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
		mod, modOK := r.mm.ModuleForFile(f.File)
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
			Locations:  []relationship.Location{{File: f.File, Line: f.StartLine}},
			Why:        fmt.Sprintf("Module %q leaks external type %q in its public API (file: %s)", mod, f.Name, f.File),
			Constraint: "Replace the external type with an internal abstraction or alias at the module boundary",
		}
		out = append(out, fnd)
	}
	return out
}
