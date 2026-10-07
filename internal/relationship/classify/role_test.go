package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// roleModules returns two modules — "src" (the edge source, role applied by the
// caller) and "dst" (a core/high-volatility target with an internal glob) — owned
// by distinct teams so the boundary token is cross_module_different_owner.
func roleModules(srcRole policy.Role) map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{
		"src": {
			Paths: []string{"src/**"},
			Owner: ownerTeamX,
			Role:  srcRole,
		},
		"dst": {
			Paths:     []string{"dst/**"},
			Internal:  []string{"dst/internal/**"},
			Owner:     ownerTeamY,
			Subdomain: subdomainCore, // high volatility
		},
	}
}

// TestRun_RoleDoesNotChangeEdgeScoring: the source's declared role no longer
// caps distance. Under bc_score.v7 every module boundary is D=9, so a
// composition root's fan-out scores like anyone else's; the role is read later,
// at the seam, where it explains the seam instead of hiding its edges.
func TestRun_RoleDoesNotChangeEdgeScoring(t *testing.T) {
	// Intrusive (the target hits dst's internal glob), cross-module, different
	// owner, high volatility: S=10, D=9, V=10 → max(1,0)+1 = 2 → critical.
	edge := graph.Edge{
		From: "file:src/main.go", To: "file:dst/internal/db.go",
		Kind: graph.EdgeKindImports, Language: "go",
	}
	for _, role := range []policy.Role{
		policy.RoleCompositionRoot, policy.RoleGenerated, policy.RoleTest,
		policy.RoleAdapter, policy.RoleCore, policy.RoleSharedModel, policy.Role(""),
	} {
		t.Run(string(role), func(t *testing.T) {
			cl, ok := classify.Run(makeGraph([]graph.Edge{edge}), classify.Config{Modules: roleModules(role)})[edgeKey(edge)]
			if !ok {
				t.Fatal("edge not classified")
			}
			if cl.Severity != coupling.SeverityCritical {
				t.Errorf("Severity = %q, want critical (dist=%q str=%q vol=%q)", cl.Severity, cl.Distance, cl.Strength, cl.Volatility)
			}
			if cl.Distance != coupling.DistanceCrossModuleDiffOwner {
				t.Errorf("Distance = %q, want %q: a role must not cap distance", cl.Distance, coupling.DistanceCrossModuleDiffOwner)
			}
		})
	}
}
