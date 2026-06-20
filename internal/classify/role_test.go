package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// roleModules returns two modules — "src" (the edge source, role applied by the
// caller) and "dst" (a core/high-volatility target with an internal glob) — owned
// by distinct teams so the base distance is cross_module_different_owner.
func roleModules(srcRole config.ModuleRole) map[string]config.ModuleDef {
	return map[string]config.ModuleDef{
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
// (composition_root/generated/test) has its high-distance fan-out downgraded to
// cohesion — no BC advisory severity — while ordinary roles (adapter/core/
// shared_model/none) keep an intrusive diff-owner high-volatility edge flagged.
func TestRun_RoleSuppressesOutboundImbalance(t *testing.T) {
	// Intrusive (target hits dst's internal glob), cross-module, different owner,
	// high volatility. Base classification (no role) is SeverityHigh.
	edge := graph.Edge{
		From: "file:src/main.go", To: "file:dst/internal/db.go",
		Kind: graph.EdgeKindImports, Language: "go",
	}

	tests := []struct {
		name         string
		role         config.ModuleRole
		wantSeverity coupling.Severity
		wantDistance coupling.Distance
	}{
		{"composition_root → suppressed", config.RoleCompositionRoot, coupling.SeverityNone, coupling.DistanceCrossModuleSameOwner},
		{"generated → suppressed", config.RoleGenerated, coupling.SeverityNone, coupling.DistanceCrossModuleSameOwner},
		{"test → suppressed", config.RoleTest, coupling.SeverityNone, coupling.DistanceCrossModuleSameOwner},
		{"adapter → still flagged", config.RoleAdapter, coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
		{"core → still flagged", config.RoleCore, coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
		{"shared_model → still flagged", config.RoleSharedModel, coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
		{"no role → still flagged", config.ModuleRole(""), coupling.SeverityHigh, coupling.DistanceCrossModuleDiffOwner},
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
	modules := map[string]config.ModuleDef{
		"wire": {
			Paths:     []string{"wire/**"},
			Internal:  []string{"wire/internal/**"},
			Owner:     ownerTeamX,
			Role:      config.RoleCompositionRoot,
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
