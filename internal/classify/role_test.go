package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
)

// roleModules returns two modules — "src" (the edge source, role applied by the
// caller) and "dst" (a core/high-volatility target with an internal glob) — owned
// by distinct teams so the base distance is cross_module_different_owner.
func roleModules(srcRole module.Role) map[string]module.ModuleDef {
	return map[string]module.ModuleDef{
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

// TestRun_RoleSuppressesOutboundImbalance verifies that a cohesive-role source
// (composition_root/generated/test) has its high-distance fan-out capped to
// cross_module_same_owner while ordinary roles (adapter/core/shared_model/none)
// keep the full cross_module_diff_owner distance.
//
// With the book formula (bc_score.v3) the severity for capped edges is no longer
// forced to SeverityNone: intrusive+same_owner+high_vol (S=10,D=4,V=10) →
// max(|10-4|=6, 10-10=0)+1=7 → SeverityLow. This is correct — the distance cap
// moves the edge out of the distributed-monolith zone, but the intrusive+volatile
// coupling is still a local advisory finding.
func TestRun_RoleSuppressesOutboundImbalance(t *testing.T) {
	// Intrusive (target hits dst's internal glob), cross-module, different owner,
	// high volatility. Base classification (no role): intrusive+diff_owner+high_vol
	// (S=10,D=7,V=10) → max(3,0)+1=4 → SeverityHigh.
	edge := graph.Edge{
		From: "file:src/main.go", To: "file:dst/internal/db.go",
		Kind: graph.EdgeKindImports, Language: "go",
	}

	tests := []struct {
		name         string
		role         module.Role
		wantSeverity coupling.Severity
		wantDistance coupling.Distance
	}{
		// Capped to same_owner: S=10,D=4,V=10 → max(6,0)+1=7 → low (not none).
		{"composition_root → distance capped, severity low", module.RoleCompositionRoot, coupling.SeverityLow, coupling.DistanceCrossModuleSameOwner},
		{"generated → distance capped, severity low", module.RoleGenerated, coupling.SeverityLow, coupling.DistanceCrossModuleSameOwner},
		{"test → distance capped, severity low", module.RoleTest, coupling.SeverityLow, coupling.DistanceCrossModuleSameOwner},
		// Uncapped: S=10,D=7,V=10 → max(3,0)+1=4 → high.
		{"adapter → still flagged", module.RoleAdapter, coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
		{"core → still flagged", module.RoleCore, coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
		{"shared_model → still flagged", module.RoleSharedModel, coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
		{"no role → still flagged", module.Role(""), coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.ClassifyConfig{Modules: roleModules(tc.role)}
			cl, ok := classify.Run(makeGraph([]graph.Edge{edge}), cfg)[edgeKey(edge)]
			if !ok {
				t.Fatalf("edge not classified")
			}
			if cl.Severity != tc.wantSeverity {
				t.Errorf("Severity = %q, want %q (dist=%q str=%q vol=%q)",
					cl.Severity, tc.wantSeverity, cl.Distance, cl.Strength, cl.Volatility)
			}
			if cl.Distance != tc.wantDistance {
				t.Errorf("Distance = %q, want %q", cl.Distance, tc.wantDistance)
			}
		})
	}
}

// TestRun_RoleDoesNotAffectInboundEdges verifies the cap applies to the source's
// outbound edges only: an edge INTO a composition_root keeps its real distance,
// so a genuine imbalance pointing at the wiring module is not hidden.
func TestRun_RoleDoesNotAffectInboundEdges(t *testing.T) {
	modules := map[string]module.ModuleDef{
		"wire": {
			Paths:     []string{"wire/**"},
			Internal:  []string{"wire/internal/**"},
			Owner:     ownerTeamX,
			Role:      module.RoleCompositionRoot,
			Subdomain: subdomainCore,
		},
		"caller": {
			Paths: []string{"caller/**"},
			Owner: ownerTeamY,
		},
	}
	// caller (no role) → wire/internal: intrusive, diff owner, high vol → high.
	edge := graph.Edge{
		From: "file:caller/x.go", To: "file:wire/internal/secret.go",
		Kind: graph.EdgeKindImports, Language: "go",
	}
	cfg := config.ClassifyConfig{Modules: modules}
	cl := classify.Run(makeGraph([]graph.Edge{edge}), cfg)[edgeKey(edge)]
	if cl.Severity != coupling.SeverityHigh {
		t.Errorf("inbound edge Severity = %q, want %q — role must cap outbound only", cl.Severity, coupling.SeverityHigh)
	}
	if cl.Distance != coupling.DistanceCrossModuleDiffOwner {
		t.Errorf("inbound edge Distance = %q, want %q", cl.Distance, coupling.DistanceCrossModuleDiffOwner)
	}
}
