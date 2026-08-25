package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// Fixture literals for the external_systems tests.
const (
	extSysAWS    = "aws"
	extFromGo    = "file:services/a/impl.go"
	extToAwsS3   = "package:github.com/aws/aws-sdk-go-v2/service/s3"
	extGlobAws   = "github.com/aws/aws-sdk-go-v2/**"
	extVolMedium = "medium"
	extVolHigh   = "high"
)

// TestRun_ExternalSystems covers the D=10 rung (book Ch10 Example 1): an edge
// whose target resolves to no module but matches a declared `external_systems:`
// entry classifies at declared_external distance with the entry's volatility
// (default low) and ENTERS scoring. The match runs on the classified edge
// target and is language-independent — one fixture edge per language exercises
// the glob against the target form its extractor emits: a Go import path, a TS
// resolved package path, a Python dotted module, and a Rust crate name.
func TestRun_ExternalSystems(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"a": {Paths: []string{"services/a/**"}, Owner: ownerTeamX, Subdomain: subdomainCore},
		"b": {Paths: []string{"services/b/**"}, Owner: ownerTeamY, Subdomain: subdomainSupporting},
	}

	tests := []struct {
		name        string
		systems     map[string]policy.ExternalSystemDef
		edge        graph.Edge
		wantVol     coupling.Volatility
		wantBalance int
	}{
		{
			name:    "go import path target",
			systems: map[string]policy.ExternalSystemDef{extSysAWS: {Targets: []string{extGlobAws}}},
			edge: graph.Edge{
				From: extFromGo, To: extToAwsS3,
				Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
			},
			wantVol:     coupling.VolatilityLow, // default: book generic-subdomain guidance
			wantBalance: 8,                      // S=8, D=10, V=3: max(|8-10|, 10-3)+1
		},
		{
			name:    "typescript resolved package path target",
			systems: map[string]policy.ExternalSystemDef{extSysAWS: {Targets: []string{"node_modules/@aws-sdk/**"}, Volatility: extVolMedium}},
			edge: graph.Edge{
				From: "file:services/a/impl.ts", To: "external:node_modules/@aws-sdk/client-s3/dist/index.js",
				Kind: graph.EdgeKindImports, Language: "typescript", StrengthHint: hintFunctional,
			},
			wantVol:     coupling.VolatilityMedium,
			wantBalance: 5, // S=8, D=10, V=6: max(2, 4)+1
		},
		{
			name:    "python dotted module target",
			systems: map[string]policy.ExternalSystemDef{"boto": {Targets: []string{"{boto3,boto3.*}"}}},
			edge: graph.Edge{
				From: "file:services/a/impl.py", To: "external:boto3.session",
				Kind: graph.EdgeKindImports, Language: "python", StrengthHint: "model",
			},
			wantVol:     coupling.VolatilityLow,
			wantBalance: 8, // S=3, D=10, V=3: max(7, 7)+1
		},
		{
			name:    "rust crate name target",
			systems: map[string]policy.ExternalSystemDef{"serde": {Targets: []string{"serde"}, Volatility: extVolHigh}},
			edge: graph.Edge{
				From: "file:services/a/lib.rs", To: "package:serde",
				Kind: graph.EdgeKindDependsOn, Language: langRust, StrengthHint: hintFunctional,
			},
			wantVol:     coupling.VolatilityHigh,
			wantBalance: 3, // S=8, D=10, V=10: max(2, 0)+1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := makeGraph([]graph.Edge{tt.edge})
			idx := classify.Run(g, classify.Config{Modules: modules, ExternalSystems: tt.systems})
			cl := idx[edgeKey(tt.edge)]

			if cl.Distance != coupling.DistanceExternal {
				t.Fatalf("Distance = %q, want %q", cl.Distance, coupling.DistanceExternal)
			}
			if cl.DistanceBasis != coupling.DistanceBasisExternal {
				t.Errorf("DistanceBasis = %q, want %q", cl.DistanceBasis, coupling.DistanceBasisExternal)
			}
			if cl.Volatility != tt.wantVol {
				t.Errorf("Volatility = %q, want %q", cl.Volatility, tt.wantVol)
			}
			if !cl.Score.Scored {
				t.Fatal("Score.Scored = false, want true — a declared external edge enters scoring")
			}
			if cl.Score.Breakdown.DistanceVal != 10 {
				t.Errorf("Breakdown.DistanceVal = %d, want 10", cl.Score.Breakdown.DistanceVal)
			}
			if cl.Score.Balance != tt.wantBalance {
				t.Errorf("Balance = %d, want %d", cl.Score.Balance, tt.wantBalance)
			}
			if cl.Severity != cl.Score.Band {
				t.Errorf("Severity = %q, want the score band %q", cl.Severity, cl.Score.Band)
			}
		})
	}

	t.Run("undeclared external target stays excluded", func(t *testing.T) {
		edge := graph.Edge{
			From: extFromGo, To: "package:github.com/stretchr/testify/assert",
			Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{edge})
		systems := map[string]policy.ExternalSystemDef{extSysAWS: {Targets: []string{extGlobAws}}}
		cl := classify.Run(g, classify.Config{Modules: modules, ExternalSystems: systems})[edgeKey(edge)]
		if cl.Distance != coupling.DistanceUnknown {
			t.Errorf("Distance = %q, want %q (undeclared external keeps the disclosed exclusion)", cl.Distance, coupling.DistanceUnknown)
		}
		if cl.Score.Scored {
			t.Error("undeclared external edge must not be scored")
		}
	})

	t.Run("nothing declared — behavior unchanged", func(t *testing.T) {
		edge := graph.Edge{
			From: extFromGo, To: extToAwsS3,
			Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{edge})
		cl := classify.Run(g, classify.Config{Modules: modules})[edgeKey(edge)]
		if cl.Distance != coupling.DistanceUnknown {
			t.Errorf("Distance = %q, want %q with no external_systems declared", cl.Distance, coupling.DistanceUnknown)
		}
		if cl.Score.Scored {
			t.Error("edge must not be scored when no external_systems are declared")
		}
	})

	t.Run("unknown strength at declared external abstains", func(t *testing.T) {
		edge := graph.Edge{
			From: extFromGo, To: extToAwsS3,
			Kind: graph.EdgeKindImports, Language: "go", // no strength hint
		}
		g := makeGraph([]graph.Edge{edge})
		systems := map[string]policy.ExternalSystemDef{extSysAWS: {Targets: []string{extGlobAws}}}
		cl := classify.Run(g, classify.Config{Modules: modules, ExternalSystems: systems})[edgeKey(edge)]
		if cl.Distance != coupling.DistanceExternal {
			t.Fatalf("Distance = %q, want %q", cl.Distance, coupling.DistanceExternal)
		}
		if cl.Score.Scored {
			t.Error("unknown strength must abstain — the D=10 rung does not change abstain-not-fake")
		}
	})

	t.Run("module-resolved target is never re-classified as external", func(t *testing.T) {
		// A glob that would match module b's path: the module resolution wins —
		// external_systems only applies when the target resolves to NO module.
		edge := graph.Edge{
			From: extFromGo, To: "file:services/b/util.go",
			Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{edge})
		systems := map[string]policy.ExternalSystemDef{"greedy": {Targets: []string{"services/**"}}}
		cl := classify.Run(g, classify.Config{Modules: modules, ExternalSystems: systems})[edgeKey(edge)]
		if cl.Distance == coupling.DistanceExternal {
			t.Error("a module-resolved target must keep its module distance, not declared_external")
		}
	})

	t.Run("unresolved source with module-resolved target stays unknown", func(t *testing.T) {
		// classifyDistance returns DistanceUnknown when EITHER endpoint fails
		// module resolution. The external match must key on the TARGET's own
		// resolution: an edge from uncovered glue code into a real module whose
		// path an external glob overlaps must not be fabricated into D=10.
		edge := graph.Edge{
			From: "file:scripts/tool.go", To: "file:services/b/util.go",
			Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{edge})
		systems := map[string]policy.ExternalSystemDef{"greedy": {Targets: []string{"services/**"}}}
		cl := classify.Run(g, classify.Config{Modules: modules, ExternalSystems: systems})[edgeKey(edge)]
		if cl.Distance != coupling.DistanceUnknown {
			t.Errorf("Distance = %q, want %q (module-resolved target, unresolved source)", cl.Distance, coupling.DistanceUnknown)
		}
		if cl.Score.Scored {
			t.Error("an edge with an unresolved source must not be scored as declared_external")
		}
	})

	t.Run("overlapping entries resolve deterministically by sorted name", func(t *testing.T) {
		edge := graph.Edge{
			From: extFromGo, To: extToAwsS3,
			Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{edge})
		systems := map[string]policy.ExternalSystemDef{
			"zz-vendor": {Targets: []string{"github.com/aws/**"}, Volatility: extVolHigh},
			"aa-vendor": {Targets: []string{"github.com/aws/**"}, Volatility: extVolMedium},
		}
		cl := classify.Run(g, classify.Config{Modules: modules, ExternalSystems: systems})[edgeKey(edge)]
		if cl.Volatility != coupling.VolatilityMedium {
			t.Errorf("Volatility = %q, want %q (first sorted entry wins)", cl.Volatility, coupling.VolatilityMedium)
		}
	})

	t.Run("composition root source still reaches declared external", func(t *testing.T) {
		// Locks the documented ordering: the external match runs AFTER the
		// cohesive-role distance cap — a composition root's edge to a declared
		// vendor system is a real integration seam, not cohesive wiring.
		rootModules := map[string]policy.ModuleDef{
			"a": {Paths: []string{pathsA}, Owner: ownerTeamX, Subdomain: subdomainCore, Role: policy.RoleCompositionRoot},
		}
		edge := graph.Edge{
			From: extFromGo, To: extToAwsS3,
			Kind: graph.EdgeKindImports, Language: "go", StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{edge})
		systems := map[string]policy.ExternalSystemDef{extSysAWS: {Targets: []string{extGlobAws}}}
		cl := classify.Run(g, classify.Config{Modules: rootModules, ExternalSystems: systems})[edgeKey(edge)]
		if cl.Distance != coupling.DistanceExternal {
			t.Errorf("Distance = %q, want %q (role cap must not swallow the external match)", cl.Distance, coupling.DistanceExternal)
		}
	})
}
