// Package classify — distance_structure.go holds the distance-ladder disclosure
// and the containment span evidence reported beside each seam.
package classify

// DistanceCompressionEvidence records the deterministic distance-ladder rungs
// archfit can currently distinguish.
type DistanceCompressionEvidence struct {
	CompressedMiddleRungs bool
	ImplementedRungs      []int
	OmittedRungs          []int
	OmittedRungReasons    []DistanceOmittedRungReason
	DeterministicSplits   []string
	Rationale             string
}

// ModuleHierarchySpan is the raw containment evidence between two modules: how
// many containment levels separate them, and how deep their container sits. It
// is report-only; scoring reads the distance token, not these counts.
type ModuleHierarchySpan struct {
	BoundaryCrossings int
	SharedAncestor    int
}

// DistanceOmittedRungReason explains why a book distance rung is not assigned.
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
		ImplementedRungs:      []int{2, 9, 10},
		OmittedRungs:          []int{1, 3, 4, 5, 6, 7, 8},
		OmittedRungReasons: []DistanceOmittedRungReason{
			{Rung: 1, Reason: "object/member-level distance is not available from module dependency edges"},
			{Rung: 3, Reason: "current facts distinguish same module vs cross-module, but not object/package micro-distance"},
			{Rung: 4, Reason: "level-relative distance: a module boundary is the far end of the in-house ladder, so no cross-module edge scores below 9"},
			{Rung: 5, Reason: "package/library middle distance is not split without explicit stable package-boundary metadata"},
			{Rung: 6, Reason: "owner and deploy unit name the boundary; they do not change the rung"},
			{Rung: 7, Reason: "level-relative distance: owner changes relabel a seam, they never lower or raise D"},
			{Rung: 8, Reason: "library-like seams remain compressed: undeclared libraries stay excluded, while declared external_systems score at D=10"},
		},
		DeterministicSplits: []string{
			"same module => D=2",
			"any module boundary (cross_module, cross_module_different_owner, cross_deploy_unit) => D=9",
			"declared external_systems target => D=10",
		},
		Rationale: "Distance is level-relative (Ch10, Ch12, Ch13): inside one codebase the module boundary is the far end. Owner and deploy unit name the boundary and never move the rung.",
	}
}

// HierarchySpan reports the containment span between two declared modules.
// The tree reads declared paths only, so renaming a module key changes nothing.
func HierarchySpan(tree Containment, fromMod, toMod string) ModuleHierarchySpan {
	crossings, shared := tree.Span(fromMod, toMod)
	return ModuleHierarchySpan{BoundaryCrossings: crossings, SharedAncestor: shared}
}
