package relationship

// IsModuleBoundary reports whether a distance token names a module boundary.
// Under bc_score.v7 every boundary sits at the same rung (D=9); owner and deploy
// unit only name it. Same module, unknown and declared-external are not seam
// boundaries.
func IsModuleBoundary(d Distance) bool {
	return d == DistanceCrossModule || d == DistanceCrossModuleDiffOwner || d == DistanceCrossDeployUnit
}

// QualifiesDistributedMonolith is the v7 edge rule behind a qualifying seam. A
// scored import edge qualifies when all four checks pass:
//
//  1. the strength is functional, intrusive or symmetric (a pinned label; a clone
//     fact is never an edge);
//  2. the edge crosses a module boundary (D=9);
//  3. the effective volatility is high, from a declared, inherited or cascade
//     end — undeclared volatility is unrated and never qualifies;
//  4. the source is not a cohesive role (composition root, generated, test),
//     unless the coupling is intrusive.
//
// Under the book formula such an edge always scores critical, so the qualifying
// set is a subset of the severity set by construction.
func QualifiesDistributedMonolith(scored bool, strength Strength, distance Distance, volatility Volatility, cohesiveRole bool) bool {
	if !scored || !IsModuleBoundary(distance) || volatility != VolatilityHigh {
		return false
	}
	switch strength {
	case StrengthIntrusive:
		return true
	case StrengthFunctional, StrengthSymmetric:
		return !cohesiveRole
	default:
		return false
	}
}
