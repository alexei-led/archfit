package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	ownerTeamX  = "team-x"
	ownerTeamY  = "team-y"
	deployUnitA = "svc-a"
	deployUnitB = "svc-b"

	// path glob constants reused across multiple test cases.
	pathsA        = "services/a/**"
	pathsB        = "services/b/**"
	publicB       = "services/b/api/**"
	subdomainCore = "core"
)

// makeGraph builds a minimal sealed Graph from a slice of edges.
func makeGraph(edges []graph.Edge) *graph.Graph {
	// Collect all unique node IDs referenced by the edges.
	seen := make(map[string]bool)
	var nodes []graph.Node
	for _, e := range edges {
		for _, id := range []string{e.From, e.To} {
			if !seen[id] {
				seen[id] = true
				// Parse "kind:path" to build the Node.
				kind, path := graph.NodeKindFile, id
				for _, k := range []graph.NodeKind{
					graph.NodeKindFile, graph.NodeKindPackage,
					graph.NodeKindModule, graph.NodeKindRepo, graph.NodeKindExternal,
				} {
					prefix := string(k) + ":"
					if len(id) > len(prefix) && id[:len(prefix)] == prefix {
						kind = k
						path = id[len(prefix):]
						break
					}
				}
				nodes = append(nodes, graph.Node{Kind: kind, Path: path})
			}
		}
	}
	return graph.Build([]graph.Facts{{
		Nodes:    nodes,
		Edges:    edges,
		Language: "go",
	}})
}

// edgeKey returns the coupling.Index key for an edge.
func edgeKey(e graph.Edge) string {
	return e.From + "\x00" + e.To + "\x00" + string(e.Kind)
}

func TestRun(t *testing.T) {
	// Shared module config used by most sub-tests.
	//   module "a": paths=["services/a/**"], public=["services/a/api/**"], internal=["services/a/internal/**"]
	//               owner="team-x", deploy_unit="svc-a", subdomain="core"
	//   module "b": paths=["services/b/**"], public=["services/b/api/**"], internal=["services/b/internal/**"]
	//               owner="team-y", deploy_unit="svc-b", subdomain="supporting"
	//   module "c": paths=["services/c/**"], public=["services/c/api/**"]
	//               owner="team-x", deploy_unit="svc-a", subdomain="generic"
	//   module "d": paths=["services/d/**"], public=["services/d/api/**"]
	//               owner="team-x", deploy_unit="svc-a", subdomain=""  (unknown)
	modules := map[string]config.ModuleDef{
		"a": {
			Paths:      []string{"services/a/**"},
			Public:     []string{"services/a/api/**"},
			Internal:   []string{"services/a/internal/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  "core",
		},
		"b": {
			Paths:      []string{"services/b/**"},
			Public:     []string{"services/b/api/**"},
			Internal:   []string{"services/b/internal/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  "supporting",
		},
		"c": {
			Paths:      []string{"services/c/**"},
			Public:     []string{"services/c/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  "generic",
		},
		"d": {
			Paths:      []string{"services/d/**"},
			Public:     []string{"services/d/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  "",
		},
	}

	cfg := config.ClassifyConfig{Modules: modules}

	// Helper to build a simple imports edge between two file paths.
	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From:       "file:" + from,
			To:         "file:" + to,
			Kind:       graph.EdgeKindImports,
			Language:   "go",
			Confidence: "high",
		}
	}

	tests := []struct {
		name     string
		edge     graph.Edge
		wantStr  coupling.Strength
		wantDist coupling.Distance
		wantVol  coupling.Volatility
		wantExp  coupling.Explicitness
	}{
		{
			name:     "contract edge — target matches public glob",
			edge:     importEdge("services/a/impl.go", "services/b/api/client.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceCrossDeployUnit, // different owner, different deploy_unit
			wantVol:  coupling.VolatilityMedium,        // b.subdomain = "supporting"
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "intrusive edge — target matches internal glob",
			edge:     importEdge("services/a/impl.go", "services/b/internal/secret.go"),
			wantStr:  coupling.StrengthIntrusive,
			wantDist: coupling.DistanceCrossDeployUnit,
			wantVol:  coupling.VolatilityMedium,
			wantExp:  coupling.ExplicitnessImplicit,
		},
		{
			name:     "unknown strength — target matches neither public nor internal",
			edge:     importEdge("services/a/impl.go", "services/b/util/helper.go"),
			wantStr:  coupling.StrengthUnknown,
			wantDist: coupling.DistanceCrossDeployUnit,
			wantVol:  coupling.VolatilityMedium,
			wantExp:  coupling.ExplicitnessUnknown,
		},
		{
			name:     "same-module edge — distance same_module",
			edge:     importEdge("services/a/x.go", "services/a/api/types.go"),
			wantStr:  coupling.StrengthContract, // a.public matches services/a/api/**
			wantDist: coupling.DistanceSameModule,
			wantVol:  coupling.VolatilityHigh, // a.subdomain = "core"
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "cross-module same-owner — different module, same owner",
			edge:     importEdge("services/a/impl.go", "services/c/api/client.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceCrossModuleSameOwner, // both owner=team-x
			wantVol:  coupling.VolatilityLow,                // c.subdomain = "generic"
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "cross-module different owner same deploy-unit — a and d",
			edge:     importEdge("services/a/impl.go", "services/d/api/types.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceCrossModuleSameOwner, // both owner=team-x
			wantVol:  coupling.VolatilityUnknown,            // d.subdomain = ""
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "cross-deploy-unit — different owner, different deploy_unit",
			edge:     importEdge("services/a/impl.go", "services/b/api/client.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceCrossDeployUnit,
			wantVol:  coupling.VolatilityMedium,
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "unknown subdomain — volatility unknown",
			edge:     importEdge("services/a/impl.go", "services/d/internal/impl.go"),
			wantStr:  coupling.StrengthUnknown, // d has no internal globs defined
			wantDist: coupling.DistanceCrossModuleSameOwner,
			wantVol:  coupling.VolatilityUnknown,
			wantExp:  coupling.ExplicitnessUnknown,
		},
		{
			name:     "unresolvable from-path — distance unknown",
			edge:     importEdge("external/foo.go", "services/b/api/client.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceUnknown, // external/foo.go matches no module
			wantVol:  coupling.VolatilityMedium,
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "unresolvable to-path — strength and volatility unknown",
			edge:     importEdge("services/a/impl.go", "external/pkg/foo.go"),
			wantStr:  coupling.StrengthUnknown,
			wantDist: coupling.DistanceUnknown,
			wantVol:  coupling.VolatilityUnknown,
			wantExp:  coupling.ExplicitnessUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := makeGraph([]graph.Edge{tc.edge})
			idx := classify.Run(g, cfg)

			key := edgeKey(tc.edge)
			cl, ok := idx[key]
			if !ok {
				t.Fatalf("edge key %q not found in index", key)
			}

			if cl.Strength != tc.wantStr {
				t.Errorf("Strength = %q, want %q", cl.Strength, tc.wantStr)
			}
			if cl.Distance != tc.wantDist {
				t.Errorf("Distance = %q, want %q", cl.Distance, tc.wantDist)
			}
			if cl.Volatility != tc.wantVol {
				t.Errorf("Volatility = %q, want %q", cl.Volatility, tc.wantVol)
			}
			if cl.Explicitness != tc.wantExp {
				t.Errorf("Explicitness = %q, want %q", cl.Explicitness, tc.wantExp)
			}
		})
	}
}

// TestRun_EmptyGraph verifies that Run on an empty graph returns an empty index.
func TestRun_EmptyGraph(t *testing.T) {
	g := graph.Build(nil)
	cfg := config.ClassifyConfig{Modules: map[string]config.ModuleDef{}}
	idx := classify.Run(g, cfg)
	if len(idx) != 0 {
		t.Errorf("expected empty index for empty graph, got %d entries", len(idx))
	}
}

// TestRun_IndexKeyMatchesEdge verifies that the index key format is consistent
// with edge canonical key (from + NUL + to + NUL + kind).
func TestRun_IndexKeyMatchesEdge(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{"pkg/a/**"}, Public: []string{"pkg/a/api/**"}},
		"b": {Paths: []string{"pkg/b/**"}, Public: []string{"pkg/b/api/**"}},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	e := graph.Edge{
		From:     "file:pkg/a/main.go",
		To:       "file:pkg/b/api/client.go",
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	g := makeGraph([]graph.Edge{e})
	idx := classify.Run(g, cfg)

	want := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
	if _, ok := idx[want]; !ok {
		t.Errorf("expected key %q in index; got keys: %v", want, keys(idx))
	}
}

// TestRun_ExplicitVolatilityFieldOverridesSubdomain verifies that an explicit
// Volatility field on a ModuleDef takes precedence over the Subdomain heuristic.
func TestRun_ExplicitVolatilityFieldOverridesSubdomain(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{"pkg/a/**"}},
		"b": {Paths: []string{"pkg/b/**"}, Subdomain: "core", Volatility: "low"},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	e := graph.Edge{
		From:     "file:pkg/a/x.go",
		To:       "file:pkg/b/y.go",
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	g := makeGraph([]graph.Edge{e})
	idx := classify.Run(g, cfg)

	key := edgeKey(e)
	cl := idx[key]
	// Volatility field "low" should override subdomain "core" (which would give high).
	if cl.Volatility != coupling.VolatilityLow {
		t.Errorf("Volatility = %q, want %q (explicit field should override subdomain)", cl.Volatility, coupling.VolatilityLow)
	}
}

// TestRun_ExplicitnessHintOverridesGlob verifies that ExplicitnessHint on the
// edge takes precedence over the config-glob-derived explicitness.
func TestRun_ExplicitnessHintOverridesGlob(t *testing.T) {
	// module "a" treats services/a/internal/** as internal → StrengthIntrusive → ExplicitnessImplicit by glob.
	// An AST signal can flip this to "explicit" via ExplicitnessHint.
	modules := map[string]config.ModuleDef{
		"a": {
			Paths:    []string{pathsA},
			Internal: []string{"services/a/internal/**"},
			Owner:    ownerTeamX, DeployUnit: deployUnitA,
		},
		"b": {
			Paths:  []string{pathsB},
			Public: []string{publicB},
			Owner:  ownerTeamY, DeployUnit: deployUnitB,
		},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	tests := []struct {
		name    string
		hint    string
		wantExp coupling.Explicitness
	}{
		{
			name:    "hint=explicit overrides intrusive-derived implicit",
			hint:    "explicit",
			wantExp: coupling.ExplicitnessExplicit,
		},
		{
			name:    "hint=implicit overrides contract-derived explicit",
			hint:    "implicit",
			wantExp: coupling.ExplicitnessImplicit,
		},
		{
			name:    "hint empty — glob result used",
			hint:    "",
			wantExp: coupling.ExplicitnessImplicit, // intrusive → implicit
		},
	}

	// Base edge: a→b/internal → StrengthIntrusive → ExplicitnessImplicit without hint.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := graph.Edge{
				From:             "file:services/a/impl.go",
				To:               "file:services/a/internal/secret.go",
				Kind:             graph.EdgeKindImports,
				Language:         "go",
				ExplicitnessHint: tc.hint,
			}
			g := makeGraph([]graph.Edge{e})
			idx := classify.Run(g, cfg)
			key := edgeKey(e)
			cl, ok := idx[key]
			if !ok {
				t.Fatalf("edge key %q not found in index", key)
			}
			if cl.Explicitness != tc.wantExp {
				t.Errorf("Explicitness = %q, want %q", cl.Explicitness, tc.wantExp)
			}
		})
	}
}

// TestRun_Severity verifies that BalanceResult-derived severity is stored on
// the Classification for cross-boundary edges.
func TestRun_Severity(t *testing.T) {
	// Modules:
	//   "a": paths=services/a/**, owner=team-x, deploy=svc-a, subdomain=core (high volatility)
	//   "b": paths=services/b/**, public=services/b/api/**, internal=services/b/internal/**,
	//        owner=team-y, deploy=svc-b, subdomain=supporting (medium volatility)
	//   "c": paths=services/c/**, public=services/c/api/**, owner=team-x, deploy=svc-a, subdomain=generic (low vol)
	modules := map[string]config.ModuleDef{
		"a": {
			Paths:      []string{pathsA},
			Public:     []string{"services/a/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  subdomainCore,
		},
		"b": {
			Paths:      []string{pathsB},
			Public:     []string{publicB},
			Internal:   []string{"services/b/internal/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  "supporting",
		},
		"c": {
			Paths:      []string{"services/c/**"},
			Public:     []string{"services/c/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  "generic",
		},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "file:" + from, To: "file:" + to,
			Kind: graph.EdgeKindImports, Language: "go",
		}
	}

	tests := []struct {
		name         string
		edge         graph.Edge
		wantSeverity coupling.Severity
	}{
		{
			// contract + cross_deploy_unit + medium vol → low+high → asymmetric → always low
			name:         "imbalanced contract cross-deploy medium-vol → low",
			edge:         importEdge("services/a/impl.go", "services/b/api/client.go"),
			wantSeverity: coupling.SeverityLow,
		},
		{
			// intrusive + cross_deploy_unit → critical
			name:         "intrusive cross-deploy → critical",
			edge:         importEdge("services/a/impl.go", "services/b/internal/secret.go"),
			wantSeverity: coupling.SeverityCritical,
		},
		{
			// contract + cross_module_same_owner + low vol → strength=low, distance=low → balanced → none
			name:         "balanced contract cross-module-same-owner low-vol → none",
			edge:         importEdge("services/a/impl.go", "services/c/api/client.go"),
			wantSeverity: coupling.SeverityNone,
		},
		{
			// same-module edge: no severity computed
			name:         "same-module edge — no severity",
			edge:         importEdge("services/a/impl.go", "services/a/api/types.go"),
			wantSeverity: coupling.SeverityNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := makeGraph([]graph.Edge{tc.edge})
			idx := classify.Run(g, cfg)
			key := edgeKey(tc.edge)
			cl, ok := idx[key]
			if !ok {
				t.Fatalf("edge key %q not found in index", key)
			}
			if cl.Severity != tc.wantSeverity {
				t.Errorf("Severity = %q, want %q (str=%q dist=%q vol=%q)",
					cl.Severity, tc.wantSeverity, cl.Strength, cl.Distance, cl.Volatility)
			}
		})
	}
}

func TestRun_StrengthHintFallbackAndPrecedence(t *testing.T) {
	// Module "b" declares services/b/api/** as its public surface but no internal
	// globs, so a plain import of services/b/impl.go has no config-derived strength.
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{pathsA}, Owner: ownerTeamX, DeployUnit: deployUnitA},
		"b": {Paths: []string{pathsB}, Public: []string{publicB}, Owner: ownerTeamY, DeployUnit: deployUnitB},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	const toBImpl = "file:services/b/impl.go" // no config-derived strength

	tests := []struct {
		name         string
		to           string
		hint         string
		wantStrength coupling.Strength
	}{
		{
			name:         "hint=intrusive used when config does not decide",
			to:           toBImpl,
			hint:         string(coupling.StrengthIntrusive),
			wantStrength: coupling.StrengthIntrusive,
		},
		{
			name:         "config public glob beats hint",
			to:           "file:services/b/api/client.go",
			hint:         string(coupling.StrengthIntrusive),
			wantStrength: coupling.StrengthContract,
		},
		{
			name:         "contract hint honored (trusted symbol-level source)",
			to:           toBImpl,
			hint:         string(coupling.StrengthContract),
			wantStrength: coupling.StrengthContract,
		},
		{
			name:         "unrecognized hint stays unknown",
			to:           toBImpl,
			hint:         "garbage",
			wantStrength: coupling.StrengthUnknown,
		},
		{
			name:         "no hint stays unknown",
			to:           toBImpl,
			hint:         "",
			wantStrength: coupling.StrengthUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := graph.Edge{
				From:         "file:services/a/impl.go",
				To:           tc.to,
				Kind:         graph.EdgeKindImports,
				Language:     "go",
				StrengthHint: tc.hint,
			}
			g := makeGraph([]graph.Edge{e})
			cl, ok := classify.Run(g, cfg)[edgeKey(e)]
			if !ok {
				t.Fatal("edge not found in index")
			}
			if cl.Strength != tc.wantStrength {
				t.Errorf("Strength = %q, want %q", cl.Strength, tc.wantStrength)
			}
		})
	}
}

// keys returns all keys of the index as a slice (for error messages).
func keys(idx coupling.Index) []string {
	ks := make([]string, 0, len(idx))
	for k := range idx {
		ks = append(ks, k)
	}
	return ks
}
