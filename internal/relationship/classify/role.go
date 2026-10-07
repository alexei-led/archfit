package classify

import "github.com/alexei-led/archfit/internal/policy"

// CohesiveRole reports whether a module's declared role makes its outbound
// fan-out cohesion rather than coupling. A composition root assembles the
// modules it wires; generated and test sources reach across boundaries by their
// nature. Such a source never qualifies a seam unless the coupling is
// intrusive; its edges still score like any other edge.
func CohesiveRole(r policy.Role) bool {
	switch r {
	case policy.RoleCompositionRoot, policy.RoleGenerated, policy.RoleTest:
		return true
	default:
		return false
	}
}
