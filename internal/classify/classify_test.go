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
	pathsA              = "services/a/**"
	pathsB              = "services/b/**"
	publicB             = "services/b/api/**"
	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"
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
			Subdomain:  subdomainCore,
		},
		"b": {
			Paths:      []string{"services/b/**"},
			Public:     []string{"services/b/api/**"},
			Internal:   []string{"services/b/internal/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  subdomainSupporting,
		},
		"c": {
			Paths:      []string{"services/c/**"},
			Public:     []string{"services/c/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  subdomainGeneric,
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
		"a": {Paths: []string{globPkgA}, Public: []string{"pkg/a/api/**"}},
		"b": {Paths: []string{globPkgB}, Public: []string{"pkg/b/api/**"}},
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
		"a": {Paths: []string{globPkgA}},
		"b": {Paths: []string{globPkgB}, Subdomain: "core", Volatility: "low"},
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
			// contract + cross_deploy_unit → low+high → XOR modular quadrant → none (BC-correct).
			name:         "contract cross-deploy medium-vol → none (XOR loose quadrant)",
			edge:         importEdge("services/a/impl.go", "services/b/api/client.go"),
			wantSeverity: coupling.SeverityNone,
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

// TestRun_ApprovedLabelPrecedence verifies the strength precedence chain:
// config globs > approved pinned labels > extractor hint.
const (
	globPkgA    = "pkg/a/**"
	globPkgB    = "pkg/b/**"
	pinnedModel = "model"
)

func TestRun_ApprovedLabelPrecedence(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{globPkgA}},
		"b": {Paths: []string{globPkgB}},
	}
	edge := graph.Edge{
		From:         "file:pkg/a/a.go",
		To:           "file:pkg/b/b.go",
		Kind:         graph.EdgeKindImports,
		StrengthHint: "functional",
	}
	g := graph.Build([]graph.Facts{{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: "pkg/a/a.go"},
			{Kind: graph.NodeKindFile, Path: "pkg/b/b.go"},
		},
		Edges: []graph.Edge{edge},
	}})
	key := edge.From + "\x00" + edge.To + "\x00" + string(edge.Kind)

	t.Run("hint applies without a label", func(t *testing.T) {
		idx := classify.Run(g, config.ClassifyConfig{Modules: modules})
		if got := idx[key].Strength; got != coupling.StrengthFunctional {
			t.Errorf("strength = %q, want functional (hint)", got)
		}
	})

	t.Run("approved label beats hint", func(t *testing.T) {
		idx := classify.Run(g, config.ClassifyConfig{
			Modules:        modules,
			ApprovedLabels: map[string]string{"a\x00b": pinnedModel},
		})
		if got := idx[key].Strength; got != coupling.StrengthModel {
			t.Errorf("strength = %q, want model (pinned label)", got)
		}
	})

	t.Run("config glob beats label", func(t *testing.T) {
		withGlobs := map[string]config.ModuleDef{
			"a": {Paths: []string{globPkgA}},
			"b": {Paths: []string{globPkgB}, Public: []string{globPkgB}},
		}
		idx := classify.Run(g, config.ClassifyConfig{
			Modules:        withGlobs,
			ApprovedLabels: map[string]string{"a\x00b": pinnedModel},
		})
		if got := idx[key].Strength; got != coupling.StrengthContract {
			t.Errorf("strength = %q, want contract (glob wins over label)", got)
		}
	})

	t.Run("label for a different pair does not apply", func(t *testing.T) {
		idx := classify.Run(g, config.ClassifyConfig{
			Modules:        modules,
			ApprovedLabels: map[string]string{"b\x00a": pinnedModel},
		})
		if got := idx[key].Strength; got != coupling.StrengthFunctional {
			t.Errorf("strength = %q, want functional (label is directional)", got)
		}
	})
}

// TestRun_ContractRecommended verifies the generic-subdomain contract advisory:
// ContractRecommended is set when the to-module is a generic subdomain and the
// strength is non-contract; it is NOT set when strength is contract, the edge is
// same-module, or the to-module is not a generic subdomain.
func TestRun_ContractRecommended(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"core": {
			Paths:      []string{"services/core/**"},
			Public:     []string{"services/core/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  subdomainCore,
		},
		"generic": {
			Paths:      []string{"services/generic/**"},
			Public:     []string{"services/generic/api/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  subdomainGeneric,
		},
		"supporting": {
			Paths:      []string{"services/supporting/**"},
			Public:     []string{"services/supporting/api/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  subdomainSupporting,
		},
		// Module with no explicit subdomain but a heuristic-generic path.
		"util": {
			Paths:      []string{"util/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
		},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	edge := func(from, to, hint string) graph.Edge {
		return graph.Edge{
			From:         "file:" + from,
			To:           "file:" + to,
			Kind:         graph.EdgeKindImports,
			Language:     "go",
			StrengthHint: hint,
		}
	}

	tests := []struct {
		name               string
		e                  graph.Edge
		wantContractRecomm bool
	}{
		{
			// Non-contract strength → generic subdomain: advisory fires.
			name:               "functional to generic → contract recommended",
			e:                  edge("services/core/impl.go", "services/generic/impl.go", "functional"),
			wantContractRecomm: true,
		},
		{
			// Unknown strength → generic subdomain: advisory fires.
			name:               "unknown strength to generic → contract recommended",
			e:                  edge("services/core/impl.go", "services/generic/impl.go", ""),
			wantContractRecomm: true,
		},
		{
			// Contract strength → generic subdomain: no advisory (already contracted).
			name:               "contract to generic api → no advisory",
			e:                  edge("services/core/impl.go", "services/generic/api/client.go", ""),
			wantContractRecomm: false,
		},
		{
			// Non-contract to non-generic (supporting): advisory does not fire.
			name:               "functional to supporting → no advisory",
			e:                  edge("services/core/impl.go", "services/supporting/impl.go", "functional"),
			wantContractRecomm: false,
		},
		{
			// Heuristic-generic path (util/) with no explicit subdomain: advisory fires.
			name:               "functional to heuristic-generic util/ → contract recommended",
			e:                  edge("services/core/impl.go", "util/parser.go", "functional"),
			wantContractRecomm: true,
		},
		{
			// Contract to heuristic-generic: no advisory.
			name:               "contract to heuristic-generic util/ api → no advisory",
			e:                  edge("services/core/impl.go", "util/parser.go", "contract"),
			wantContractRecomm: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := makeGraph([]graph.Edge{tc.e})
			idx := classify.Run(g, cfg)
			key := edgeKey(tc.e)
			cl, ok := idx[key]
			if !ok {
				t.Fatalf("edge key %q not found in index", key)
			}
			if cl.ContractRecommended != tc.wantContractRecomm {
				t.Errorf("ContractRecommended = %v, want %v (str=%q dist=%q vol=%q)",
					cl.ContractRecommended, tc.wantContractRecomm,
					cl.Strength, cl.Distance, cl.Volatility)
			}
		})
	}
}

// TestRun_DegenerateOwnerSuppression verifies degenerate-owner suppression (design §4.2):
//   - When ALL modules share a single owner (git-author fallback degenerate case),
//     ownership distance contributes nothing and code-structure distance dominates.
//   - When modules have DISTINCT owners (real CODEOWNERS repo), ownership distance
//     applies normally and the max() composite picks it up.
func TestRun_DegenerateOwnerSuppression(t *testing.T) {
	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "file:" + from, To: "file:" + to,
			Kind: graph.EdgeKindImports, Language: "go",
		}
	}

	t.Run("degenerate: single owner everywhere — code-structure dominates", func(t *testing.T) {
		// All modules have the same owner. isDegenerateOwnerMap returns true,
		// so ownership contributes DistanceSameModule (no signal).
		//
		// Module names use path structure so codeStructureDistance works correctly:
		//   "pkg/a" and "pkg/b" are siblings → structural = SameOwner.
		//   "pkg/a" and "services/x" are distant trees → structural = DiffOwner.
		modules := map[string]config.ModuleDef{
			"pkg/a":      {Paths: []string{globPkgA}, Owner: ownerTeamX},
			"pkg/b":      {Paths: []string{globPkgB}, Owner: ownerTeamX},
			"services/x": {Paths: []string{"services/x/**"}, Owner: ownerTeamX},
		}
		cfg := config.ClassifyConfig{Modules: modules}

		// Siblings (pkg/a ↔ pkg/b): structural = SameOwner; ownership suppressed → composite = SameOwner.
		e1 := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl1 := classify.Run(makeGraph([]graph.Edge{e1}), cfg)[edgeKey(e1)]
		if cl1.Distance != coupling.DistanceCrossModuleSameOwner {
			t.Errorf("siblings with degenerate owner: Distance = %q, want %q (code-structure should dominate)",
				cl1.Distance, coupling.DistanceCrossModuleSameOwner)
		}

		// Distant subtrees (pkg/a ↔ services/x): structural = DiffOwner; ownership suppressed → composite = DiffOwner.
		e2 := importEdge("pkg/a/x.go", "services/x/y.go")
		cl2 := classify.Run(makeGraph([]graph.Edge{e2}), cfg)[edgeKey(e2)]
		if cl2.Distance != coupling.DistanceCrossModuleDiffOwner {
			t.Errorf("distant subtrees with degenerate owner: Distance = %q, want %q (code-structure should dominate)",
				cl2.Distance, coupling.DistanceCrossModuleDiffOwner)
		}
	})

	t.Run("multi-owner: distinct owners — ownership distance applies", func(t *testing.T) {
		// Two modules with DISTINCT owners (not degenerate). isDegenerateOwnerMap returns false.
		// For sibling modules: code-structure = SameOwner, ownership = DiffOwner.
		// max(SameOwner, DiffOwner) = DiffOwner — ownership lifts the result.
		modules := map[string]config.ModuleDef{
			"pkg/a": {Paths: []string{globPkgA}, Owner: ownerTeamX},
			"pkg/b": {Paths: []string{globPkgB}, Owner: ownerTeamY},
		}
		cfg := config.ClassifyConfig{Modules: modules}

		e := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
		if cl.Distance != coupling.DistanceCrossModuleDiffOwner {
			t.Errorf("siblings with distinct owners: Distance = %q, want %q (ownership should lift result)",
				cl.Distance, coupling.DistanceCrossModuleDiffOwner)
		}
	})
}

// TestRun_ChurnVolatilityIgnoredOnGate verifies that classify.Run produces
// VolatilityUnknown for a module with no explicit volatility or subdomain,
// even when the ClassifyConfig was built from a Config that had ApplyVolatility
// called with churn data. ForClassify() must strip churn before passing to Run.
func TestRun_ChurnVolatilityIgnoredOnGate(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{"pkg/a/**"}, Owner: ownerTeamX},
			"b": {Paths: []string{"pkg/b/**"}, Owner: ownerTeamY}, // no volatility, no subdomain
		},
	}
	// Apply churn-derived high volatility to module "b".
	cfg.ApplyVolatility(map[string]string{"b": "high"})

	// ForClassify must exclude churn.
	classifyCfg := cfg.ForClassify()
	if got := classifyCfg.Modules["b"].Volatility; got != "" {
		t.Fatalf("precondition: ForClassify().Modules[b].Volatility = %q, want \"\" (churn excluded)", got)
	}

	e := graph.Edge{
		From:     "file:pkg/a/x.go",
		To:       "file:pkg/b/y.go",
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	g := makeGraph([]graph.Edge{e})
	idx := classify.Run(g, classifyCfg)
	cl := idx[edgeKey(e)]

	if cl.Volatility != coupling.VolatilityUnknown {
		t.Errorf("Volatility = %q, want unknown (churn must not reach gate)", cl.Volatility)
	}
}
