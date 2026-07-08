// Package classify — distance_structure.go provides codeStructureDistance,
// the always-available deterministic baseline for the distance composite (§4.2).
package classify

import (
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// DistanceCompressionEvidence records the deterministic distance-ladder rungs
// archfit can currently distinguish. Middle Ch8 package/library rungs stay
// compressed unless a stable graph/config fact separates them.
type DistanceCompressionEvidence struct {
	CompressedMiddleRungs bool
	ImplementedRungs      []int
	OmittedRungs          []int
	OmittedRungReasons    []DistanceOmittedRungReason
	DeterministicSplits   []string
	Rationale             string
}

// ModuleHierarchySpan is the raw structural-distance evidence between two
// module names: how many module-boundary crossings separate them, and how deep
// their closest common ancestor sits in the hierarchy. It is report-only book
// Ch8 evidence for compressed middle rungs; scoring still consumes the coarse
// Distance enum, not these raw counts.
type ModuleHierarchySpan struct {
	BoundaryCrossings int
	SharedAncestor    int
}

// DistanceOmittedRungReason explains why a book distance rung remains compressed
// instead of being assigned from unstable naming or package-shape guesses.
type DistanceOmittedRungReason struct {
	Rung   int
	Reason string
}

// DistanceCompression returns a deterministic summary of the distance ladder
// implemented by classifyDistance. It is disclosure-only: the scorer still reads
// the concrete Distance on each edge, not these strings.
func DistanceCompression() DistanceCompressionEvidence {
	return DistanceCompressionEvidence{
		CompressedMiddleRungs: true,
		ImplementedRungs:      []int{2, 4, 7, 9, 10},
		OmittedRungs:          []int{1, 3, 5, 6, 8},
		OmittedRungReasons: []DistanceOmittedRungReason{
			{Rung: 1, Reason: "object/member-level distance is not available from module dependency edges"},
			{Rung: 3, Reason: "current facts distinguish same module vs cross-module, but not object/package micro-distance"},
			{Rung: 5, Reason: "package/library middle distance is not split without explicit stable package-boundary metadata"},
			{Rung: 6, Reason: "intermediate ownership/library distance has no deterministic signal beyond owner and tree structure"},
			{Rung: 8, Reason: "library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10"},
		},
		DeterministicSplits: []string{
			"same module => D=2",
			"code_structure sibling/parent-child => D=4; unrelated subtrees => D=7",
			"ownership same/different owner => D=4/D=7 when the owner map is non-degenerate",
			"different deploy_unit => D=9",
			"declared external_systems target => D=10",
		},
		Rationale: "D=3/D=5/D=6/D=8 remain compressed: current graph/config facts distinguish same module, same owner, different owner, deploy unit, and declared vendor seam, but not finer package/library distance without guessing.",
	}
}

// codeStructureDistance returns the structural distance between two module names
// based on their position in the package/directory tree. This is the code-proximity
// dimension of Vlad Khononov's socio-technical distance — "how many boundaries a
// change must cross" in the source tree, before ownership/deploy signals refine it
// in classifyDistance.
//
// Module names are hierarchical: slash-separated for Go/TS path modules
// (e.g. "internal/metrics/boundary") and dot-separated for Python modules
// (e.g. "pkg.metrics.boundary"). moduleSegments splits each name on whichever
// separator it uses, so dotted siblings are compared structurally instead of
// collapsing to single segments (which forced every Python edge to DiffOwner and
// drove the advisory flood).
//
// Distance mapping (capped at cross_module_diff_owner — cross_deploy_unit is
// reserved for the deploy-unit signal in deployDistance):
//
//   - Modules sharing all but at most one trailing segment (siblings or
//     parent-child) → DistanceCrossModuleSameOwner.
//   - Modules in different subtrees (fewer shared segments) →
//     DistanceCrossModuleDiffOwner.
//
// Two flat single-segment names (e.g. "core" vs "api") carry no tree signal:
// there is no structural evidence they belong to different teams.
// codeStructureDistance is only reached via Step 4 of classifyDistance when
// ownership is absent or degenerate (single-owner or zero-owner repo), so both
// modules already share the same — or unknown — owner context. Fabricating
// DiffOwner from absent data produced false "tight coupling" findings on every
// flat-named single-team repo (eval P1: codegraph, spotinfo). The honest floor
// is SameOwner (ordinal 4 vs 7).
//
// Accepted tradeoff: two genuinely-separate-but-unowned flat modules in a
// zero-owner config are under-distanced. This is preferable to the eval-observed
// false-positive harm on real single-team repos. Multi-team repos are unaffected:
// their ≥2 distinct owners resolve via Steps 2/3 before reaching here.
//
// Returns DistanceUnknown when either name is empty.
func codeStructureDistance(fromMod, toMod, lang string) coupling.Distance {
	if fromMod == "" || toMod == "" {
		return coupling.DistanceUnknown
	}

	fromParts := moduleSegments(fromMod, lang)
	toParts := moduleSegments(toMod, lang)

	if len(fromParts) == 1 && len(toParts) == 1 {
		// No tree structure to distinguish teams; return the honest floor.
		// See comment on codeStructureDistance for the rationale and tradeoff.
		return coupling.DistanceCrossModuleSameOwner
	}

	shared := 0
	for i := 0; i < len(fromParts) && i < len(toParts); i++ {
		if fromParts[i] != toParts[i] {
			break
		}
		shared++
	}

	depth := len(fromParts)
	if len(toParts) > depth {
		depth = len(toParts)
	}

	// Siblings or parent-child: all but at most one trailing segment is shared.
	if shared >= depth-1 {
		return coupling.DistanceCrossModuleSameOwner
	}
	return coupling.DistanceCrossModuleDiffOwner
}

// moduleSegments splits a hierarchical module name into its path segments using
// the edge language's NodeConvention separator: Go/TS/Rust modules are
// slash-separated ("internal/metrics/boundary"); Python modules are dot-separated
// ("pkg.metrics.boundary"). An unknown language defaults to slash (see
// graph.ConventionRegistry.Lookup). A flat name with no separator is one segment.
func moduleSegments(mod, lang string) []string {
	return graph.BuiltinConventions.Lookup(lang).ModuleSegments(mod)
}

// HierarchySpan estimates structural distance from a module pair's closest
// common ancestor, as described in book Ch8. The counts are raw evidence, not
// score inputs: BoundaryCrossings is the number of hierarchy steps that differ
// across the pair; SharedAncestor is the depth of the closest common ancestor.
//
// Language is inferred from the module-name separator for report-only use:
// Rust uses "::"; slash-based modules (Go/TS paths and file-like module keys)
// use "/"; dotted names fall back to Python-style "." segmentation.
func HierarchySpan(fromMod, toMod string) ModuleHierarchySpan {
	fromParts := hierarchySegments(fromMod)
	toParts := hierarchySegments(toMod)
	shared := 0
	for i := 0; i < len(fromParts) && i < len(toParts); i++ {
		if fromParts[i] != toParts[i] {
			break
		}
		shared++
	}
	return ModuleHierarchySpan{
		BoundaryCrossings: (len(fromParts) - shared) + (len(toParts) - shared),
		SharedAncestor:    shared,
	}
}

func hierarchySegments(mod string) []string {
	switch {
	case strings.Contains(mod, "::"):
		return strings.Split(mod, "::")
	case strings.Contains(mod, "/"):
		return strings.Split(mod, "/")
	case strings.Contains(mod, "."):
		return strings.Split(mod, ".")
	default:
		if mod == "" {
			return nil
		}
		return []string{mod}
	}
}

// ownershipDistance returns the distance contribution from module ownership.
// Returns DistanceSameModule (no signal) when both owners are empty, deferring
// to code-structure distance in ownerless repos.
// Returns DistanceCrossModuleSameOwner when owners match.
// Returns DistanceCrossModuleDiffOwner when owners differ.
//
// Call isDegenerateOwnerMap before invoking this to suppress ownership when the
// entire repo has a single owner (git-author fallback degenerate case).
func ownershipDistance(fromOwner, toOwner string) coupling.Distance {
	if fromOwner == "" && toOwner == "" {
		return coupling.DistanceSameModule
	}
	if fromOwner == toOwner {
		return coupling.DistanceCrossModuleSameOwner
	}
	return coupling.DistanceCrossModuleDiffOwner
}

// ownerDegeneracy precomputes the two owner-map degeneracy invariants shared
// by Run and CloneOnlyPairs: degeneracy of the explicit (CODEOWNERS/config)
// owner map and of the full module owner map. Both depend only on config,
// so callers compute them once per run.
func ownerDegeneracy(c config.ClassifyConfig) (degenerateExplicit, degenerateOwners bool) {
	explicitOwnerMap := make(map[string]string, len(c.ExplicitOwners))
	for mod := range c.ExplicitOwners {
		explicitOwnerMap[mod] = c.Modules[mod].Owner
	}
	fullOwnerMap := make(map[string]string, len(c.Modules))
	for name, def := range c.Modules {
		fullOwnerMap[name] = def.Owner
	}
	return isDegenerateOwnerMap(explicitOwnerMap), isDegenerateOwnerMap(fullOwnerMap)
}

// isDegenerateOwnerMap reports whether the ownership map is degenerate — i.e.
// all modules that have a non-empty owner share exactly one distinct owner value.
// A degenerate map arises from the git-author fallback in a single/few-author
// repo: every module gets the same dominant author, making every cross-module
// edge appear as "same owner → low distance", which suppresses the code-structure
// signal that would otherwise distinguish near from far modules.
//
// When true, the caller should pass empty strings to ownershipDistance so that
// the ownership contribution is neutral (DistanceSameModule = no contribution)
// and code-structure distance dominates.
//
// A real multi-team CODEOWNERS repo will typically have two or more distinct
// owners and returns false.
func isDegenerateOwnerMap(owners map[string]string) bool {
	var seen string
	for _, o := range owners {
		if o == "" {
			continue
		}
		if seen == "" {
			seen = o
			continue
		}
		if o != seen {
			return false
		}
	}
	// Zero owned modules → no ownership signal at all; treat as degenerate so
	// ownership contributes nothing (same as all-empty).
	return true
}

// cohesiveRole reports whether a module's declared role makes its outbound
// fan-out cohesion rather than coupling. A composition root assembles the
// modules it wires; generated and test sources reach across boundaries by their
// nature. Their outbound edges must not be scored as high-distance/unbalanced.
// adapter/core/shared_model are ordinary sources — their edges are classified
// normally so a real core→core imbalance still surfaces.
func cohesiveRole(r config.ModuleRole) bool {
	switch r {
	case config.RoleCompositionRoot, config.RoleGenerated, config.RoleTest:
		return true
	default:
		return false
	}
}

// capDistanceForRole downgrades a high-distance value to cohesion for an edge
// whose source has a cohesiveRole. cross_module_different_owner and
// cross_deploy_unit collapse to cross_module_same_owner (cohesive wiring);
// same_module/same_owner/unknown are left untouched (already not high-distance).
func capDistanceForRole(d coupling.Distance) coupling.Distance {
	switch d {
	case coupling.DistanceCrossModuleDiffOwner, coupling.DistanceCrossDeployUnit:
		return coupling.DistanceCrossModuleSameOwner
	default:
		return d
	}
}

// deployDistance returns the distance contribution from deploy-unit membership.
// Returns DistanceCrossDeployUnit when both modules have a non-empty deploy unit
// that differs. Returns DistanceSameModule (no signal) otherwise, so max()
// defers to other signals when deploy units are absent or equal.
func deployDistance(fromUnit, toUnit string) coupling.Distance {
	if fromUnit != "" && toUnit != "" && fromUnit != toUnit {
		return coupling.DistanceCrossDeployUnit
	}
	return coupling.DistanceSameModule
}

// distanceLevelOrdinal maps Distance to a comparable ordinal for maxDistance.
// Ordinals mirror the scorer's distance point table (scorer.go:56-65) so that
// max() and scoring use a consistent ordering. DistanceUnknown sits between
// SameOwner and DiffOwner (ordinal 2): it is conservative but concrete evidence
// of diff-ownership beats it.
var distanceLevelOrdinal = map[coupling.Distance]int{
	coupling.DistanceSameModule:           0,
	coupling.DistanceCrossModuleSameOwner: 1,
	coupling.DistanceUnknown:              2,
	coupling.DistanceCrossModuleDiffOwner: 3,
	coupling.DistanceCrossDeployUnit:      5,
	// DistanceExternal is assigned after the composite (declared external_systems
	// match on an unknown-distance edge), so it never enters maxDistance today;
	// ranked highest so a future combination cannot silently demote it.
	coupling.DistanceExternal: 6,
}

// maxDistance returns the highest-ordinal Distance among the given values.
// Used to combine independent distance signals into the composite.
func maxDistance(distances ...coupling.Distance) coupling.Distance {
	best := coupling.DistanceSameModule
	for _, d := range distances {
		if distanceLevelOrdinal[d] > distanceLevelOrdinal[best] {
			best = d
		}
	}
	return best
}
