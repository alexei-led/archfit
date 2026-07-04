package classify_test

import (
	"slices"
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

	langRust = "rust"

	// Go workspace member module paths reused across multiple test cases.
	goModuleA = "example.com/a"
	goModuleB = "example.com/b"
	relDirA   = "services/a"
	relDirB   = "services/b"
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
			Paths:      []string{pathsA},
			Public:     []string{"services/a/api/**"},
			Internal:   []string{"services/a/internal/**"},
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
			wantVol:  coupling.VolatilityLow,           // b.subdomain = "supporting" → low (book Table 9.1)
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "intrusive edge — target matches internal glob",
			edge:     importEdge("services/a/impl.go", "services/b/internal/secret.go"),
			wantStr:  coupling.StrengthIntrusive,
			wantDist: coupling.DistanceCrossDeployUnit,
			wantVol:  coupling.VolatilityLow, // b.subdomain = "supporting" → low (book Table 9.1)
			wantExp:  coupling.ExplicitnessImplicit,
		},
		{
			name:     "unknown strength — target matches neither public nor internal",
			edge:     importEdge("services/a/impl.go", "services/b/util/helper.go"),
			wantStr:  coupling.StrengthUnknown,
			wantDist: coupling.DistanceCrossDeployUnit,
			wantVol:  coupling.VolatilityLow, // b.subdomain = "supporting" → low (book Table 9.1)
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
			wantVol:  coupling.VolatilityUndeclared,         // d resolves but d.subdomain = ""
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "cross-deploy-unit — different owner, different deploy_unit",
			edge:     importEdge("services/a/impl.go", "services/b/api/client.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceCrossDeployUnit,
			wantVol:  coupling.VolatilityLow, // b.subdomain = "supporting" → low (book Table 9.1)
			wantExp:  coupling.ExplicitnessExplicit,
		},
		{
			name:     "undeclared subdomain — module resolves but no subdomain/volatility",
			edge:     importEdge("services/a/impl.go", "services/d/internal/impl.go"),
			wantStr:  coupling.StrengthUnknown, // d has no internal globs defined
			wantDist: coupling.DistanceCrossModuleSameOwner,
			wantVol:  coupling.VolatilityUndeclared, // d resolves; no subdomain/volatility/path match
			wantExp:  coupling.ExplicitnessUnknown,
		},
		{
			name:     "unresolvable from-path — distance unknown",
			edge:     importEdge("external/foo.go", "services/b/api/client.go"),
			wantStr:  coupling.StrengthContract,
			wantDist: coupling.DistanceUnknown, // external/foo.go matches no module
			wantVol:  coupling.VolatilityLow,   // b.subdomain = "supporting" → low (book Table 9.1)
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

// TestRun_RegistersRustModuleGraphNodes verifies the last-mile: a Rust module-graph
// edge ("crate::mod" nodes) with NO configured modules must still classify to a
// cross-module distance, because AugmentModulesFromGraph synthesises the module
// definitions. Without it, moduleFor fails and the edge is distance-unknown (never
// counted by coupling_balance/encapsulation). Siblings under the same crate resolve
// to same-owner (the "::" segment separator).
func TestRun_RegistersRustModuleGraphNodes(t *testing.T) {
	e := graph.Edge{
		From:         "package:demo::a",
		To:           "package:demo::b",
		Kind:         graph.EdgeKindDependsOn,
		Language:     langRust,
		StrengthHint: "functional",
	}
	g := makeGraph([]graph.Edge{e})

	// Without registration: unknown distance (not counted).
	bare := classify.Run(g, config.ClassifyConfig{Modules: map[string]config.ModuleDef{}})
	if cl := bare[edgeKey(e)]; cl.Distance != coupling.DistanceUnknown {
		t.Errorf("unregistered: Distance = %q, want unknown (module nodes not in config)", cl.Distance)
	}

	// With registration: cross-module, same owner (siblings in one crate), functional.
	mods := classify.AugmentModulesFromGraph(g, map[string]config.ModuleDef{})
	idx := classify.Run(g, config.ClassifyConfig{Modules: mods})
	cl, ok := idx[edgeKey(e)]
	if !ok {
		t.Fatalf("edge not found in index")
	}
	if cl.Distance != coupling.DistanceCrossModuleSameOwner {
		t.Errorf("Distance = %q, want cross_module_same_owner (registered sibling modules)", cl.Distance)
	}
	if cl.Strength != coupling.StrengthFunctional {
		t.Errorf("Strength = %q, want functional (from hint)", cl.Strength)
	}
}

// TestAugmentModulesFromGraph_LeavesNonRustUntouched guards the "::" gate: Go/TS/Python
// graph nodes (slash/dot separators) must NOT be auto-registered, so configured-module
// semantics for those languages are unchanged.
func TestAugmentModulesFromGraph_LeavesNonRustUntouched(t *testing.T) {
	g := makeGraph([]graph.Edge{{
		From: "package:internal/x", To: "package:internal/y", Kind: graph.EdgeKindImports, Language: "go",
	}})
	in := map[string]config.ModuleDef{}
	out := classify.AugmentModulesFromGraph(g, in)
	if len(out) != 0 {
		t.Errorf("non-Rust nodes must not be registered, got %d modules: %v", len(out), out)
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
		"b": {Paths: []string{globPkgB}, Subdomain: subdomainCore, Volatility: "low"},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	e := graph.Edge{
		From:     filePkgAXGo,
		To:       filePkgBYGo,
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

// TestRun_Severity verifies that book-formula severity (cl.Score.Band) is stored
// on the Classification for cross-boundary edges.
func TestRun_Severity(t *testing.T) {
	// Modules:
	//   "a": paths=services/a/**, owner=team-x, deploy=svc-a, subdomain=core (high volatility)
	//   "b": paths=services/b/**, public=services/b/api/**, internal=services/b/internal/**,
	//        owner=team-y, deploy=svc-b, subdomain=supporting (low volatility)
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
			Subdomain:  subdomainSupporting,
		},
		"c": {
			Paths:      []string{"services/c/**"},
			Public:     []string{"services/c/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  subdomainGeneric,
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
			// contract (S=1) + cross_deploy_unit (D=9) + supporting/low (V=3):
			// max(|1-9|=8, 10-6=4)+1=9 → none (loose XOR quadrant, book-correct).
			name:         "contract cross-deploy medium-vol → none (XOR loose quadrant)",
			edge:         importEdge("services/a/impl.go", "services/b/api/client.go"),
			wantSeverity: coupling.SeverityNone,
		},
		{
			// intrusive (S=10) + cross_deploy_unit (D=9) + supporting/low (V=3, book Table 9.1):
			// max(|10-9|=1, 10-3=7)+1=8 → low.
			// (BalanceResult returned critical; book formula correctly scores low —
			// stable/low-volatility target neutralizes even a tight cross-deploy edge.)
			name:         "intrusive cross-deploy low-vol → low (book formula)",
			edge:         importEdge("services/a/impl.go", "services/b/internal/secret.go"),
			wantSeverity: coupling.SeverityLow,
		},
		{
			// contract (S=1) + cross_module_same_owner (D=4) + generic/low (V=3):
			// max(|1-4|=3, 10-3=7)+1=8 → low.
			// (BalanceResult returned none; book formula correctly scores low — volatile
			// seam even across a near boundary.)
			name:         "contract cross-module-same-owner low-vol → low (book formula)",
			edge:         importEdge("services/a/impl.go", "services/c/api/client.go"),
			wantSeverity: coupling.SeverityLow,
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
// internal glob (intrusive) is authoritative; a public glob is a not-intrusive
// floor whose KIND is resolved by approved label > extractor hint > contract.
const (
	globPkgA    = "pkg/a/**"
	globPkgB    = "pkg/b/**"
	pinnedModel = "model"

	modKeyPkgA  = "pkg/a"
	modKeyPkgB  = "pkg/b"
	filePkgAXGo = "file:pkg/a/x.go"
	filePkgBYGo = "file:pkg/b/y.go"

	modKeySvcX = "services/x"
	globSvcX   = "services/x/**"
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
		StrengthHint: hintFunctional,
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

	t.Run("approved label refines public-glob kind", func(t *testing.T) {
		// A public glob is a not-intrusive floor (contract); an approved human label
		// is the most authoritative statement of the public-coupling KIND, so it
		// refines the floor to model — beating the contract default and the
		// functional hint. (Previously the glob was treated as authoritative contract,
		// which masked the real coupling strength — the bug F2 fixes.)
		withGlobs := map[string]config.ModuleDef{
			"a": {Paths: []string{globPkgA}},
			"b": {Paths: []string{globPkgB}, Public: []string{globPkgB}},
		}
		idx := classify.Run(g, config.ClassifyConfig{
			Modules:        withGlobs,
			ApprovedLabels: map[string]string{"a\x00b": pinnedModel},
		})
		if got := idx[key].Strength; got != coupling.StrengthModel {
			t.Errorf("strength = %q, want model (approved label refines public-glob floor)", got)
		}
	})

	t.Run("public glob without label refines to hint kind", func(t *testing.T) {
		// No human label: the public-glob contract floor is refined to the hint's
		// kind (functional here), not left at the weakest contract default.
		withGlobs := map[string]config.ModuleDef{
			"a": {Paths: []string{globPkgA}},
			"b": {Paths: []string{globPkgB}, Public: []string{globPkgB}},
		}
		idx := classify.Run(g, config.ClassifyConfig{Modules: withGlobs})
		if got := idx[key].Strength; got != coupling.StrengthFunctional {
			t.Errorf("strength = %q, want functional (hint refines public-glob floor)", got)
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

// TestRun_PublicGlobFloorRefinement is the F2 regression guard: a public-glob
// match is a not-intrusive floor whose KIND is the hint's public-coupling kind,
// while an internal glob stays authoritatively intrusive and a public floor is
// never lowered to intrusive by a hint.
func TestRun_PublicGlobFloorRefinement(t *testing.T) {
	build := func(hint string) (*graph.Graph, string) {
		e := graph.Edge{
			From: "file:pkg/a/a.go", To: "file:pkg/b/b.go",
			Kind: graph.EdgeKindImports, StrengthHint: hint,
		}
		g := graph.Build([]graph.Facts{{
			Language: "go",
			Nodes: []graph.Node{
				{Kind: graph.NodeKindFile, Path: "pkg/a/a.go"},
				{Kind: graph.NodeKindFile, Path: "pkg/b/b.go"},
			},
			Edges: []graph.Edge{e},
		}})
		return g, e.From + "\x00" + e.To + "\x00" + string(e.Kind)
	}
	publicB := map[string]config.ModuleDef{
		"a": {Paths: []string{globPkgA}},
		"b": {Paths: []string{globPkgB}, Public: []string{globPkgB}},
	}
	internalB := map[string]config.ModuleDef{
		"a": {Paths: []string{globPkgA}},
		"b": {Paths: []string{globPkgB}, Internal: []string{globPkgB}},
	}
	// No public/internal globs: classifyStrength is unknown, the hint decides.
	noGlobB := map[string]config.ModuleDef{
		"a": {Paths: []string{globPkgA}},
		"b": {Paths: []string{globPkgB}},
	}
	cases := []struct {
		name    string
		modules map[string]config.ModuleDef
		hint    string
		want    coupling.Strength
	}{
		{"public + model hint → model", publicB, string(coupling.StrengthModel), coupling.StrengthModel},
		{"public + contract hint → contract", publicB, string(coupling.StrengthContract), coupling.StrengthContract},
		{"public + functional hint → functional", publicB, hintFunctional, coupling.StrengthFunctional},
		{"public + no hint → contract floor", publicB, "", coupling.StrengthContract},
		{"public + intrusive hint → contract (floor not lowered)", publicB, hintIntrusive, coupling.StrengthContract},
		{"internal glob → intrusive (authoritative)", internalB, hintFunctional, coupling.StrengthIntrusive},
		// A pure-data DTO across a declared public boundary IS the book's explicit
		// integration contract — the floor stands, the hint must not raise it.
		{"public + dto hint → contract (DTO is the boundary contract)", publicB, graph.StrengthHintDTO, coupling.StrengthContract},
		// Without the boundary declaration the same DTO is just a shared concrete
		// type: the declaration is what makes it a contract (book Ch10).
		{"no glob + dto hint → model (no declared boundary)", noGlobB, graph.StrengthHintDTO, coupling.StrengthModel},
		// An internal glob stays authoritative regardless of the DTO hint.
		{"internal + dto hint → intrusive (authoritative)", internalB, graph.StrengthHintDTO, coupling.StrengthIntrusive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, key := build(tc.hint)
			idx := classify.Run(g, config.ClassifyConfig{Modules: tc.modules})
			if got := idx[key].Strength; got != tc.want {
				t.Errorf("strength = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRun_ContractRecommended verifies the generic-subdomain contract advisory:
// ContractRecommended is set when the to-module is a generic subdomain and the
// strength is non-contract; it is NOT set when strength is contract, the edge is
// same-module, or the to-module is not a generic subdomain.
func TestRun_ContractRecommended(t *testing.T) {
	modules := map[string]config.ModuleDef{
		subdomainCore: {
			Paths:      []string{"services/core/**"},
			Public:     []string{"services/core/api/**"},
			Owner:      ownerTeamX,
			DeployUnit: deployUnitA,
			Subdomain:  subdomainCore,
		},
		subdomainGeneric: {
			Paths:      []string{"services/generic/**"},
			Public:     []string{"services/generic/api/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  subdomainGeneric,
		},
		subdomainSupporting: {
			Paths:      []string{"services/supporting/**"},
			Public:     []string{"services/supporting/api/**"},
			Owner:      ownerTeamY,
			DeployUnit: deployUnitB,
			Subdomain:  subdomainSupporting,
		},
		// Module with no explicit subdomain — must NOT be inferred as generic
		// from its path name (the path heuristic was removed; no guessing).
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
			// Undeclared subdomain (util/, no subdomain): NOT inferred as generic
			// from the path name — no path guessing, so no advisory fires.
			name:               "functional to undeclared util/ → no advisory (no path guessing)",
			e:                  edge("services/core/impl.go", "util/parser.go", "functional"),
			wantContractRecomm: false,
		},
		{
			// Contract to undeclared util/: no advisory either way.
			name:               "contract to undeclared util/ → no advisory",
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
			modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
			modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamX},
			modKeySvcX: {Paths: []string{globSvcX}, Owner: ownerTeamX},
		}
		// No explicit owners — degenerate case exercises the code-structure fallback.
		cfg := config.Config{Version: 1, Modules: modules}.ForClassify()

		// Siblings (pkg/a ↔ pkg/b): structural = SameOwner; ownership suppressed → composite = SameOwner.
		e1 := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl1 := classify.Run(makeGraph([]graph.Edge{e1}), cfg)[edgeKey(e1)]
		if cl1.Distance != coupling.DistanceCrossModuleSameOwner {
			t.Errorf("siblings with degenerate owner: Distance = %q, want %q (code-structure should dominate)",
				cl1.Distance, coupling.DistanceCrossModuleSameOwner)
		}

		// Distant subtrees (pkg/a ↔ services/x): structural = DiffOwner; ownership suppressed → composite = DiffOwner.
		e2 := importEdge("pkg/a/x.go", modKeySvcX+"/y.go")
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
			modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
			modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamY},
		}
		// Explicit distinct owners → Step 2 fires (non-degenerate explicit map).
		cfg := config.Config{Version: 1, Modules: modules}.
			WithExplicitOwners(modKeyPkgA, modKeyPkgB).ForClassify()

		e := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
		if cl.Distance != coupling.DistanceCrossModuleDiffOwner {
			t.Errorf("siblings with distinct owners: Distance = %q, want %q (ownership should lift result)",
				cl.Distance, coupling.DistanceCrossModuleDiffOwner)
		}
	})
}

// TestRun_VolatilityUndeclaredWithoutConfig verifies classify.Run produces
// VolatilityUndeclared (not Unknown) for a resolved module with no explicit
// volatility or subdomain: the module is known, only its volatility is a config
// gap. The gate never derives volatility from git churn — there is no churn path
// into classification at all.
func TestRun_VolatilityUndeclaredWithoutConfig(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{globPkgA}, Owner: ownerTeamX},
			"b": {Paths: []string{globPkgB}, Owner: ownerTeamY}, // no volatility, no subdomain
		},
	}
	classifyCfg := cfg.ForClassify()
	if got := classifyCfg.Modules["b"].Volatility; got != "" {
		t.Fatalf("precondition: ForClassify().Modules[b].Volatility = %q, want \"\"", got)
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

	if cl.Volatility != coupling.VolatilityUndeclared {
		t.Errorf("Volatility = %q, want undeclared", cl.Volatility)
	}
}

// TestRun_SingleOwnerFarModulesStayFar verifies that in a single-owner repo
// (degenerate owner map), modules in different code subtrees still classify as
// DiffOwner — code structure dominates and differentiates near from far even
// when every module shares the same owner.
func TestRun_SingleOwnerFarModulesStayFar(t *testing.T) {
	// All three modules have the same owner → degenerate → ownership suppressed.
	modules := map[string]config.ModuleDef{
		modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
		modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamX},
		modKeySvcX: {Paths: []string{globSvcX}, Owner: ownerTeamX},
	}
	// No explicit owners — degenerate case, code structure dominates.
	cfg := config.Config{Version: 1, Modules: modules}.ForClassify()

	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "file:" + from, To: "file:" + to,
			Kind: graph.EdgeKindImports, Language: "go",
		}
	}

	// Siblings (pkg/a → pkg/b): code structure = SameOwner → composite = SameOwner.
	eSib := importEdge("pkg/a/x.go", "pkg/b/y.go")
	clSib := classify.Run(makeGraph([]graph.Edge{eSib}), cfg)[edgeKey(eSib)]
	if clSib.Distance != coupling.DistanceCrossModuleSameOwner {
		t.Errorf("siblings single-owner: Distance = %q, want cross_module_same_owner", clSib.Distance)
	}
	if clSib.DistanceBasis != coupling.DistanceBasisStructure {
		t.Errorf("siblings single-owner: DistanceBasis = %q, want code_structure", clSib.DistanceBasis)
	}

	// Distant subtrees (pkg/a → services/x): code structure = DiffOwner → composite = DiffOwner.
	eFar := importEdge("pkg/a/x.go", modKeySvcX+"/y.go")
	clFar := classify.Run(makeGraph([]graph.Edge{eFar}), cfg)[edgeKey(eFar)]
	if clFar.Distance != coupling.DistanceCrossModuleDiffOwner {
		t.Errorf("distant subtrees single-owner: Distance = %q, want cross_module_different_owner", clFar.Distance)
	}
	if clFar.DistanceBasis != coupling.DistanceBasisStructure {
		t.Errorf("distant subtrees single-owner: DistanceBasis = %q, want code_structure", clFar.DistanceBasis)
	}
}

// TestRun_MultiOwnerSiblingsLifted verifies that in a multi-owner repo, distinct
// ownership lifts sibling modules from SameOwner to DiffOwner — ownership is
// authoritative (not degenerate) and the max() composite picks it up.
func TestRun_MultiOwnerSiblingsLifted(t *testing.T) {
	// Two distinct owners → not degenerate → ownership applies.
	// pkg/a and pkg/b are code-structure siblings (same_owner by structure),
	// but owned by different teams → ownership lifts to DiffOwner.
	modules := map[string]config.ModuleDef{
		modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
		modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamY},
	}
	// Both modules carry explicit hand-authored owners (distinct) → Step 2 fires.
	cfg := config.Config{Version: 1, Modules: modules}.
		WithExplicitOwners(modKeyPkgA, modKeyPkgB).ForClassify()

	e := graph.Edge{
		From: "file:pkg/a/x.go", To: "file:pkg/b/y.go",
		Kind: graph.EdgeKindImports, Language: "go",
	}
	cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
	if cl.Distance != coupling.DistanceCrossModuleDiffOwner {
		t.Errorf("siblings distinct owners: Distance = %q, want cross_module_different_owner", cl.Distance)
	}
	if cl.DistanceBasis != coupling.DistanceBasisOwnership {
		t.Errorf("siblings distinct owners: DistanceBasis = %q, want ownership", cl.DistanceBasis)
	}
}

// TestSmallOSSDistanceFixture models a 1-2 maintainer OSS repo and proves that
// code distance still differentiates nearby and distant modules even when all
// ownership resolves to a single author. The DistanceBasis must be "code_structure"
// on every edge (degenerate-owner suppression active).
func TestSmallOSSDistanceFixture(t *testing.T) {
	// Typical small OSS repo: one or two maintainers, same owner everywhere.
	// Code structure must be the sole differentiator.
	const soleOwner = "alice"
	modules := map[string]config.ModuleDef{
		"cmd/tool":       {Paths: []string{"cmd/tool/**"}, Owner: soleOwner},
		"internal/core":  {Paths: []string{"internal/core/**"}, Owner: soleOwner},
		"internal/store": {Paths: []string{"internal/store/**"}, Owner: soleOwner},
		"pkg/api":        {Paths: []string{"pkg/api/**"}, Owner: soleOwner},
	}
	// No explicit owners — degenerate case, code structure is the sole differentiator.
	cfg := config.Config{Version: 1, Modules: modules}.ForClassify()

	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "file:" + from, To: "file:" + to,
			Kind: graph.EdgeKindImports, Language: "go",
		}
	}

	tests := []struct {
		name      string
		from, to  string
		wantDist  coupling.Distance
		wantBasis coupling.DistanceBasis
	}{
		{
			// internal/core and internal/store share the "internal" parent → siblings → SameOwner.
			name:      "internal siblings — nearby",
			from:      "internal/core/domain.go",
			to:        "internal/store/repo.go",
			wantDist:  coupling.DistanceCrossModuleSameOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
		{
			// cmd/tool → internal/core: different top-level subtrees → DiffOwner.
			name:      "cmd to internal — distant",
			from:      "cmd/tool/main.go",
			to:        "internal/core/domain.go",
			wantDist:  coupling.DistanceCrossModuleDiffOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
		{
			// pkg/api → internal/store: different top-level subtrees → DiffOwner.
			name:      "pkg to internal — distant",
			from:      "pkg/api/handler.go",
			to:        "internal/store/repo.go",
			wantDist:  coupling.DistanceCrossModuleDiffOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := importEdge(tc.from, tc.to)
			cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
			if cl.Distance != tc.wantDist {
				t.Errorf("Distance = %q, want %q", cl.Distance, tc.wantDist)
			}
			if cl.DistanceBasis != tc.wantBasis {
				t.Errorf("DistanceBasis = %q, want %q", cl.DistanceBasis, tc.wantBasis)
			}
		})
	}
}

// TestRun_DistanceBasisPopulated verifies that DistanceBasis is set correctly
// on Classification after classify.Run for the three signal sources.
func TestRun_DistanceBasisPopulated(t *testing.T) {
	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "file:" + from, To: "file:" + to,
			Kind: graph.EdgeKindImports, Language: "go",
		}
	}

	t.Run("degenerate owner → code_structure basis", func(t *testing.T) {
		modules := map[string]config.ModuleDef{
			modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
			modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamX},
		}
		// No explicit owners — degenerate, structure fallback fires.
		cfg := config.Config{Version: 1, Modules: modules}.ForClassify()
		e := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
		if cl.DistanceBasis != coupling.DistanceBasisStructure {
			t.Errorf("DistanceBasis = %q, want code_structure", cl.DistanceBasis)
		}
	})

	t.Run("explicit owner → ownership basis", func(t *testing.T) {
		modules := map[string]config.ModuleDef{
			modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
			modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamY},
		}
		// Mark both as explicit via production path so Step 2 fires (non-degenerate).
		cfg := config.Config{Version: 1, Modules: modules}.
			WithExplicitOwners(modKeyPkgA, modKeyPkgB).ForClassify()
		e := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
		if cl.DistanceBasis != coupling.DistanceBasisOwnership {
			t.Errorf("DistanceBasis = %q, want ownership", cl.DistanceBasis)
		}
	})

	t.Run("deploy unit difference → deploy_unit basis", func(t *testing.T) {
		modules := map[string]config.ModuleDef{
			modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX, DeployUnit: deployUnitA},
			modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerTeamX, DeployUnit: deployUnitB},
		}
		// Deploy unit is Step 1 (absolute); ExplicitOwners irrelevant here.
		cfg := config.Config{Version: 1, Modules: modules}.ForClassify()
		e := importEdge("pkg/a/x.go", "pkg/b/y.go")
		cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
		if cl.DistanceBasis != coupling.DistanceBasisDeployUnit {
			t.Errorf("DistanceBasis = %q, want deploy_unit", cl.DistanceBasis)
		}
		if cl.Distance != coupling.DistanceCrossDeployUnit {
			t.Errorf("Distance = %q, want cross_deploy_unit", cl.Distance)
		}
	})

	t.Run("same module → unknown basis (omits from JSON)", func(t *testing.T) {
		modules := map[string]config.ModuleDef{
			modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerTeamX},
		}
		// Same-module edge; basis is unknown regardless of ownership config.
		cfg := config.Config{Version: 1, Modules: modules}.ForClassify()
		e := importEdge("pkg/a/x.go", "pkg/a/y.go")
		cl := classify.Run(makeGraph([]graph.Edge{e}), cfg)[edgeKey(e)]
		if cl.DistanceBasis != coupling.DistanceBasisUnknown {
			t.Errorf("DistanceBasis = %q, want empty (unknown)", cl.DistanceBasis)
		}
	})
}

// TestRun_CohesiveCloseModules proves that intra-module coupling (same module)
// produces DistanceSameModule and an empty DistanceBasis. Strong coupling within
// a module is cohesion — not a defect — and must never be flagged as a problem.
func TestRun_CohesiveCloseModules(t *testing.T) {
	// Single module "internal/core". Two files within it.
	const soleOwner = "alice"
	modules := map[string]config.ModuleDef{
		"internal/core": {Paths: []string{"internal/core/**"}, Owner: soleOwner},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	e := graph.Edge{
		From:     "file:internal/core/domain.go",
		To:       "file:internal/core/service.go",
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	g := makeGraph([]graph.Edge{e})
	idx := classify.Run(g, cfg)

	cl, ok := idx[edgeKey(e)]
	if !ok {
		t.Fatalf("edge key not found in index")
	}
	if cl.Distance != coupling.DistanceSameModule {
		t.Errorf("Distance = %q, want same_module (intra-module coupling is cohesion, not a defect)", cl.Distance)
	}
	// DistanceBasis is unknown/empty for same-module edges — no cross-boundary signal.
	if cl.DistanceBasis != coupling.DistanceBasisUnknown {
		t.Errorf("DistanceBasis = %q, want empty (same_module case has no distance basis)", cl.DistanceBasis)
	}
}

// TestRun_SmallOSSDeployUnitBoundaryStaysFar proves that even in a single-owner
// (degenerate) repo, modules with different deploy units stay at CrossDeployUnit
// distance. Deploy unit is absolute: it overrides code-structure and ownership.
func TestRun_SmallOSSDeployUnitBoundaryStaysFar(t *testing.T) {
	// Single owner "alice" everywhere (degenerate → ownership suppressed).
	// But modules declare distinct deploy units, which is absolute.
	modules := map[string]config.ModuleDef{
		modKeyPkgA: {Paths: []string{globPkgA}, Owner: "alice", DeployUnit: deployUnitA},
		modKeyPkgB: {Paths: []string{globPkgB}, Owner: "alice", DeployUnit: deployUnitB},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	e := graph.Edge{
		From:     filePkgAXGo,
		To:       filePkgBYGo,
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	g := makeGraph([]graph.Edge{e})
	cl := classify.Run(g, cfg)[edgeKey(e)]

	if cl.Distance != coupling.DistanceCrossDeployUnit {
		t.Errorf("Distance = %q, want cross_deploy_unit (deploy boundary is absolute even in single-owner repos)", cl.Distance)
	}
	if cl.DistanceBasis != coupling.DistanceBasisDeployUnit {
		t.Errorf("DistanceBasis = %q, want deploy_unit", cl.DistanceBasis)
	}
}

// TestRun_ExplicitOwnerSingleSameOwner verifies that when ALL modules carry the
// same explicit owner (degenerate explicit-owner map), Step 2 falls through and
// code structure dominates — distance_basis must be "code_structure", not
// "ownership". This is the archfit self-scan scenario: every module has
// owner: alexei-led, so without this guard all distances collapse to SameOwner.
func TestRun_ExplicitOwnerSingleSameOwner(t *testing.T) {
	const ownerAlexei = "alexei-led"
	modules := map[string]config.ModuleDef{
		modKeyPkgA: {Paths: []string{globPkgA}, Owner: ownerAlexei},
		modKeyPkgB: {Paths: []string{globPkgB}, Owner: ownerAlexei},
		modKeySvcX: {Paths: []string{globSvcX}, Owner: ownerAlexei},
	}
	cfg := config.Config{Version: 1, Modules: modules}.
		WithExplicitOwners(modKeyPkgA, modKeyPkgB, modKeySvcX)
	classifyCfg := cfg.ForClassify()

	importEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "file:" + from, To: "file:" + to,
			Kind: graph.EdgeKindImports, Language: "go",
		}
	}

	// Siblings (pkg/a → pkg/b): code structure = SameOwner → composite = SameOwner.
	eSib := importEdge("pkg/a/x.go", "pkg/b/y.go")
	clSib := classify.Run(makeGraph([]graph.Edge{eSib}), classifyCfg)[edgeKey(eSib)]
	if clSib.Distance != coupling.DistanceCrossModuleSameOwner {
		t.Errorf("siblings all-same-owner explicit: Distance = %q, want cross_module_same_owner (code structure)", clSib.Distance)
	}
	if clSib.DistanceBasis != coupling.DistanceBasisStructure {
		t.Errorf("siblings all-same-owner explicit: DistanceBasis = %q, want code_structure", clSib.DistanceBasis)
	}

	// Distant subtrees (pkg/a → services/x): code structure = DiffOwner → composite = DiffOwner.
	eFar := importEdge("pkg/a/x.go", modKeySvcX+"/y.go")
	clFar := classify.Run(makeGraph([]graph.Edge{eFar}), classifyCfg)[edgeKey(eFar)]
	if clFar.Distance != coupling.DistanceCrossModuleDiffOwner {
		t.Errorf("distant all-same-owner explicit: Distance = %q, want cross_module_different_owner (code structure)", clFar.Distance)
	}
	if clFar.DistanceBasis != coupling.DistanceBasisStructure {
		t.Errorf("distant all-same-owner explicit: DistanceBasis = %q, want code_structure", clFar.DistanceBasis)
	}
}

// TestRun_VolatilityUnknownWhenModuleUnresolved verifies that an edge whose
// to-path matches no configured module yields VolatilityUnknown — genuinely
// indeterminate, distinct from the undeclared config-gap case above.
func TestRun_VolatilityUnknownWhenModuleUnresolved(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{globPkgA}, Owner: ownerTeamX},
		},
	}
	classifyCfg := cfg.ForClassify()

	e := graph.Edge{
		From:     "file:pkg/a/main.go",
		To:       "file:external/pkg/foo.go", // matches no module
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	g := makeGraph([]graph.Edge{e})
	idx := classify.Run(g, classifyCfg)
	cl := idx[edgeKey(e)]

	if cl.Volatility != coupling.VolatilityUnknown {
		t.Errorf("Volatility = %q, want unknown", cl.Volatility)
	}
}

// buildGoWorkspaceGraph builds a Graph with the given GoModules and edges, suitable
// for testing AugmentGoWorkspaceModules.
// buildCrateGraph builds a Rust crate-level graph (package:<crate> nodes) plus
// the CrateRoots that map each crate to its repo-relative directory.
func buildCrateGraph(crateRoots []graph.CrateRoot, edges []graph.Edge) *graph.Graph {
	seen := make(map[string]bool)
	var nodes []graph.Node
	for _, e := range edges {
		for _, id := range []string{e.From, e.To} {
			if !seen[id] {
				seen[id] = true
				nodes = append(nodes, graph.Node{Kind: graph.NodeKindPackage, Path: graph.NodePath(id)})
			}
		}
	}
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: edges, Language: graph.LangRust, CrateRoots: crateRoots}})
}

// TestAugmentCargoCrateNodes covers the three cases of Rust crate-node binding:
// a directory-path config (ruff-like) binds the bare-name node to the dir-covering
// module while preserving its attributes; a bare-name config (tokio/yazi-like) is a
// no-op; and a single-crate graph is left untouched by the ≥2 gate.
func TestAugmentCargoCrateNodes(t *testing.T) {
	const (
		crateAst   = "ruff_python_ast"
		crateTokio = "tokio"
		crateUtil  = "tokio-util"
		crateSolo  = "solo"
	)
	crateEdge := func(from, to string) graph.Edge {
		return graph.Edge{
			From: "package:" + from, To: "package:" + to,
			Kind: graph.EdgeKindImports, Language: graph.LangRust, StrengthHint: hintFunctional,
		}
	}

	t.Run("path-glob config binds crate node and preserves attributes (ruff-like)", func(t *testing.T) {
		e := crateEdge("ruff_linter", crateAst)
		g := buildCrateGraph([]graph.CrateRoot{
			{Dir: "crates/ruff_linter", Name: "ruff_linter"},
			{Dir: "crates/ruff_python_ast", Name: crateAst},
		}, []graph.Edge{e})
		cfg := map[string]config.ModuleDef{
			"ruff_linter": {Paths: []string{"crates/ruff_linter/**"}, Volatility: "high", Subdomain: subdomainCore},
			crateAst:      {Paths: []string{"crates/ruff_python_ast/**"}, Volatility: "medium", Subdomain: "generic"},
		}
		mods := classify.AugmentCargoCrateNodes(g, cfg)
		// Bare crate name appended to the dir-covering module, attributes preserved.
		if !slices.Contains(mods[crateAst].Paths, crateAst) {
			t.Errorf("bare crate glob not bound: %v", mods[crateAst].Paths)
		}
		if mods[crateAst].Volatility != "medium" || mods[crateAst].Subdomain != "generic" {
			t.Errorf("module attributes lost: vol=%q sub=%q", mods[crateAst].Volatility, mods[crateAst].Subdomain)
		}
		// The cross-crate edge now classifies with a real Distance (was Unknown → external).
		idx := classify.Run(g, config.ClassifyConfig{Modules: mods})
		cl, ok := idx[edgeKey(e)]
		if !ok {
			t.Fatal("cross-crate edge missing from coupling index")
		}
		if cl.Distance == coupling.DistanceUnknown {
			t.Error("cross-crate edge still distance-unknown after augmentation")
		}
	})

	t.Run("bare-name config is a no-op (tokio/yazi-like)", func(t *testing.T) {
		e := crateEdge(crateTokio, crateUtil)
		g := buildCrateGraph([]graph.CrateRoot{
			{Dir: crateTokio, Name: crateTokio},
			{Dir: crateUtil, Name: crateUtil},
		}, []graph.Edge{e})
		cfg := map[string]config.ModuleDef{
			crateTokio: {Paths: []string{crateTokio}},
			crateUtil:  {Paths: []string{crateUtil}},
		}
		mods := classify.AugmentCargoCrateNodes(g, cfg)
		if len(mods) != len(cfg) {
			t.Errorf("bare-name config not a no-op: module count %d→%d", len(cfg), len(mods))
		}
		if len(mods[crateTokio].Paths) != 1 {
			t.Errorf("tokio paths mutated: %v", mods[crateTokio].Paths)
		}
	})

	t.Run("single crate untouched (degenerate gate)", func(t *testing.T) {
		g := graph.Build([]graph.Facts{{
			Nodes:      []graph.Node{{Kind: graph.NodeKindPackage, Path: crateSolo}},
			Language:   graph.LangRust,
			CrateRoots: []graph.CrateRoot{{Dir: "crates/solo", Name: crateSolo}},
		}})
		cfg := map[string]config.ModuleDef{crateSolo: {Paths: []string{"crates/solo/**"}}}
		mods := classify.AugmentCargoCrateNodes(g, cfg)
		if len(mods[crateSolo].Paths) != 1 || mods[crateSolo].Paths[0] != "crates/solo/**" {
			t.Errorf("single-crate repo should be untouched, got %v", mods[crateSolo].Paths)
		}
	})
}

func buildGoWorkspaceGraph(goMods []graph.GoModule, edges []graph.Edge) *graph.Graph {
	seen := make(map[string]bool)
	var nodes []graph.Node
	for _, e := range edges {
		for _, id := range []string{e.From, e.To} {
			if !seen[id] {
				seen[id] = true
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
		Nodes:     nodes,
		Edges:     edges,
		Language:  "go",
		GoModules: goMods,
	}})
}

// TestAugmentGoWorkspaceModules_TwoMembersAutoRegister verifies that when ≥2 Go
// workspace members are loaded, each gets a synthetic module registered (one per
// member), and that a cross-member edge with a StrengthHint classifies with a real
// Distance and a scored EdgeScore — the direct proxy for "coupling_balance measures."
func TestAugmentGoWorkspaceModules_TwoMembersAutoRegister(t *testing.T) {
	e := graph.Edge{
		From:         "package:services/a/foo",
		To:           "package:services/b/bar",
		Kind:         graph.EdgeKindImports,
		Language:     "go",
		StrengthHint: "functional",
	}
	goMods := []graph.GoModule{
		{Path: goModuleA, RelDir: relDirA},
		{Path: goModuleB, RelDir: relDirB},
	}
	g := buildGoWorkspaceGraph(goMods, []graph.Edge{e})

	mods := classify.AugmentGoWorkspaceModules(g, map[string]config.ModuleDef{})
	if len(mods) != 2 {
		t.Fatalf("want 2 synthetic modules, got %d: %v", len(mods), mods)
	}
	// Each registered module must carry the member's glob.
	for _, m := range goMods {
		found := false
		for _, def := range mods {
			for _, p := range def.Paths {
				if p == m.RelDir+"/**" {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no synthetic module with glob %q for member %q", m.RelDir+"/**", m.Path)
		}
	}

	// Cross-member edge must get a real Distance and score.
	idx := classify.Run(g, config.ClassifyConfig{Modules: mods})
	cl, ok := idx[edgeKey(e)]
	if !ok {
		t.Fatalf("edge not found in coupling index")
	}
	if cl.Distance == coupling.DistanceUnknown {
		t.Errorf("Distance = unknown; want a real distance (auto-registration broken)")
	}
	// With strength=functional + a real distance, the scorer must produce a scored result
	// — this is the unit-level proxy for "coupling_balance measures."
	if !cl.Score.Scored {
		t.Errorf("Score.Scored = false; want true (coupling_balance must measure when strength+distance are known)")
	}
}

// TestAugmentGoWorkspaceModules_OneMemberNoOp guards the ≥2 gate: a single loaded
// module must not trigger auto-registration, preserving byte-identical output for
// single-module repos and archfit's own self-scan.
func TestAugmentGoWorkspaceModules_OneMemberNoOp(t *testing.T) {
	g := buildGoWorkspaceGraph(
		[]graph.GoModule{{Path: "example.com/myapp", RelDir: "cmd/myapp"}},
		nil,
	)
	in := map[string]config.ModuleDef{"mymod": {Paths: []string{pathsA}}}
	out := classify.AugmentGoWorkspaceModules(g, in)
	if len(out) != len(in) {
		t.Errorf("1-member: want map unchanged (len %d), got len %d: %v", len(in), len(out), out)
	}
}

// TestAugmentGoWorkspaceModules_ConfigGlobWins verifies that an already-configured
// module covering a member's directory takes precedence over auto-registration.
func TestAugmentGoWorkspaceModules_ConfigGlobWins(t *testing.T) {
	goMods := []graph.GoModule{
		{Path: goModuleA, RelDir: relDirA},
		{Path: goModuleB, RelDir: relDirB},
	}
	g := buildGoWorkspaceGraph(goMods, nil)
	// Both member directories already covered by explicit config modules.
	// Use the deployUnitA/B constants ("svc-a"/"svc-b") as map keys to satisfy goconst.
	in := map[string]config.ModuleDef{
		deployUnitA: {Paths: []string{pathsA}},
		deployUnitB: {Paths: []string{pathsB}},
	}
	out := classify.AugmentGoWorkspaceModules(g, in)
	if len(out) != len(in) {
		t.Errorf("config globs win: want map unchanged (len %d), got len %d; extra: %v",
			len(in), len(out), out)
	}
}

// TestAugmentModulesFromGraph_OwnerInheritance verifies that a synthetic Rust
// submodule inherits the owner of its nearest config-declared ancestor (the crate
// module). Without this fix, inter-submodule edges classify as different_owner
// instead of cross_module_same_owner (the herdr regression).
func TestAugmentModulesFromGraph_OwnerInheritance(t *testing.T) {
	// Config declares the crate-level module with owner="team-x".
	// cargo-modules graph produces submodule nodes "mycrate::a" and "mycrate::b"
	// which are NOT in config — they should be synthesised and inherit owner "team-x".
	configMods := map[string]config.ModuleDef{
		"mycrate": {Paths: []string{"mycrate/**"}, Owner: ownerTeamX},
	}
	e := graph.Edge{
		From:         "package:mycrate::a",
		To:           "package:mycrate::b",
		Kind:         graph.EdgeKindDependsOn,
		Language:     langRust,
		StrengthHint: hintFunctional,
	}
	g := makeGraph([]graph.Edge{e})

	augmented := classify.AugmentModulesFromGraph(g, configMods)

	// Both synthetic submodules must carry owner="team-x".
	for _, key := range []string{"mycrate::a", "mycrate::b"} {
		def, ok := augmented[key]
		if !ok {
			t.Fatalf("synthetic module %q not registered", key)
		}
		if def.Owner != ownerTeamX {
			t.Errorf("module %q: Owner = %q, want %q (inherited from ancestor)", key, def.Owner, ownerTeamX)
		}
	}

	// The inter-submodule edge must classify as cross_module_same_owner, not different_owner.
	idx := classify.Run(g, config.ClassifyConfig{Modules: augmented})
	cl, ok := idx[edgeKey(e)]
	if !ok {
		t.Fatalf("edge not found in index after augmentation")
	}
	if cl.Distance != coupling.DistanceCrossModuleSameOwner {
		t.Errorf("Distance = %q, want cross_module_same_owner (submodules share inherited owner)", cl.Distance)
	}
}

// TestAugmentCargoCrateNodes_LeavesGoUntouched guards Fix group 0 Task 0.1
// (reproduces A1): AugmentCargoCrateNodes gates on
// "n.Kind != graph.NodeKindPackage", but graph.NodeKindPackage is emitted by
// the Go extractor too — nothing in the gate is Rust-specific. On a Go-only
// graph (no "::" module-graph nodes, no CrateRoots) with package nodes not
// covered by any configured module glob, the function must be a no-op: it
// must never register a synthetic module for a Go package. Today it does
// (the omni bug: a phantom "assets/dal"-style module gets registered with no
// owner), so this test is RED until Fix group 2 adds an explicit
// Rust-language gate.
func TestAugmentCargoCrateNodes_LeavesGoUntouched(t *testing.T) {
	const (
		goPkgUncovered1 = "internal/assets/dal"
		goPkgUncovered2 = "internal/assets/pkg"
	)
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindPackage, Path: goPkgUncovered1},
			{Kind: graph.NodeKindPackage, Path: goPkgUncovered2},
		},
		Language: "go",
		// No CrateRoots: this is a pure Go graph, never a Rust one.
	}})
	// One declared module that does NOT cover either uncovered package —
	// mirrors omni's "intentionally-partial module coverage" shape.
	in := map[string]config.ModuleDef{
		"declared": {Paths: []string{"cmd/**"}, Owner: ownerTeamX},
	}

	out := classify.AugmentCargoCrateNodes(g, in)

	if len(out) != len(in) {
		t.Fatalf("Go-only graph: want modules map unchanged (len %d), got len %d: %v", len(in), len(out), out)
	}
	for _, uncovered := range []string{goPkgUncovered1, goPkgUncovered2} {
		if _, exists := out[uncovered]; exists {
			t.Errorf("Go package %q must not be registered as a synthetic Rust crate module; out = %v", uncovered, out)
		}
	}
}

// wantInherited lists the ModuleDef fields a synthetic module must inherit
// from its nearest config-declared ancestor. Deliberately explicit (not
// reflect-based) so a future new ModuleDef field must be added here by a
// reviewer, rather than silently passing without coverage.
type wantInherited struct {
	Owner      string
	Volatility string
	Subdomain  string
	Layer      string
	DeployUnit string
}

// assertInherited fails t for every field of got that does not match want,
// so a single run reports every dropped attribute at once.
func assertInherited(t *testing.T, label string, got config.ModuleDef, want wantInherited) {
	t.Helper()
	if got.Owner != want.Owner {
		t.Errorf("%s: Owner = %q, want %q (ancestor inheritance)", label, got.Owner, want.Owner)
	}
	if got.Volatility != want.Volatility {
		t.Errorf("%s: Volatility = %q, want %q (ancestor inheritance)", label, got.Volatility, want.Volatility)
	}
	if got.Subdomain != want.Subdomain {
		t.Errorf("%s: Subdomain = %q, want %q (ancestor inheritance)", label, got.Subdomain, want.Subdomain)
	}
	if got.Layer != want.Layer {
		t.Errorf("%s: Layer = %q, want %q (ancestor inheritance)", label, got.Layer, want.Layer)
	}
	if got.DeployUnit != want.DeployUnit {
		t.Errorf("%s: DeployUnit = %q, want %q (ancestor inheritance)", label, got.DeployUnit, want.DeployUnit)
	}
}

// TestAugmentFunctions_InheritAllAncestorAttributes guards Fix group 0 Task
// 0.4 (reproduces B3): every Augment* function that registers a synthetic
// module hand-writes its own ancestor lookup, and every one of them copies
// only Owner — Volatility, Subdomain, Layer, and DeployUnit are dropped for
// every synthetic module, in all three functions. Undeclared Volatility
// scores the conservative worst case (V=10), which is the root cause of
// tokio's 917-finding flood. RED today for Volatility/Subdomain/Layer/
// DeployUnit on all three functions.
func TestAugmentFunctions_InheritAllAncestorAttributes(t *testing.T) {
	want := wantInherited{
		Owner:      "team-inherit",
		Volatility: "low",
		Subdomain:  "supporting",
		Layer:      "domain",
		DeployUnit: "svc-inherit",
	}
	parent := config.ModuleDef{
		Owner:      want.Owner,
		Volatility: want.Volatility,
		Subdomain:  want.Subdomain,
		Layer:      want.Layer,
		DeployUnit: want.DeployUnit,
	}

	t.Run("AugmentModulesFromGraph (Rust submodule)", func(t *testing.T) {
		// Config declares only the crate-level module "mycrate"; the
		// cargo-modules graph produces submodule node "mycrate::child" that
		// is not in config and must be synthesised as a child of "mycrate".
		configMods := map[string]config.ModuleDef{"mycrate": parent}
		e := graph.Edge{
			From: "package:mycrate", To: "package:mycrate::child",
			Kind: graph.EdgeKindDependsOn, Language: langRust, StrengthHint: hintFunctional,
		}
		g := makeGraph([]graph.Edge{e})

		out := classify.AugmentModulesFromGraph(g, configMods)
		child, ok := out["mycrate::child"]
		if !ok {
			t.Fatalf("synthetic module %q not registered", "mycrate::child")
		}
		assertInherited(t, "mycrate::child", child, want)
	})

	t.Run("AugmentGoWorkspaceModules (workspace member)", func(t *testing.T) {
		// Parent config module uses a literal (non-wildcard) path so it does
		// NOT fully cover the member directory via ModuleFor (so the member
		// still gets auto-registered), but its stripped directory IS a path
		// prefix of the member's RelDir, so ancestorOwnerByPath can donate —
		// mirrors the documented "partial ancestor donates owner" fallback.
		configMods := map[string]config.ModuleDef{"parent": {
			Paths: []string{"services"}, Owner: want.Owner, Volatility: want.Volatility,
			Subdomain: want.Subdomain, Layer: want.Layer, DeployUnit: want.DeployUnit,
		}}
		goMods := []graph.GoModule{
			{Path: goModuleA, RelDir: relDirA},
			{Path: goModuleB, RelDir: relDirB},
		}
		g := buildGoWorkspaceGraph(goMods, nil)

		out := classify.AugmentGoWorkspaceModules(g, configMods)
		member, ok := out[goModuleA]
		if !ok {
			t.Fatalf("synthetic module for member %q not registered; out = %v", goModuleA, out)
		}
		assertInherited(t, goModuleA, member, want)
	})

	t.Run("AugmentCargoCrateNodes (uncovered crate)", func(t *testing.T) {
		// Same literal-parent-path trick as above: "crates" is a directory
		// prefix of the crate's root ("crates/extra") but does not itself
		// cover "crates/extra/x" via ModuleFor, so the crate falls through to
		// synthetic registration and donation via ancestorOwnerByPath.
		configMods := map[string]config.ModuleDef{"parent": {
			Paths: []string{"crates"}, Owner: want.Owner, Volatility: want.Volatility,
			Subdomain: want.Subdomain, Layer: want.Layer, DeployUnit: want.DeployUnit,
		}}
		g := graph.Build([]graph.Facts{{
			Nodes: []graph.Node{
				{Kind: graph.NodeKindPackage, Path: "extra"},
				{Kind: graph.NodeKindPackage, Path: "other"},
			},
			Language:   langRust,
			CrateRoots: []graph.CrateRoot{{Dir: "crates/extra", Name: "extra"}},
		}})

		out := classify.AugmentCargoCrateNodes(g, configMods)
		crateMod, ok := out["extra"]
		if !ok {
			t.Fatalf("synthetic module for crate %q not registered; out = %v", "extra", out)
		}
		assertInherited(t, "extra", crateMod, want)
	})
}

// TestRun_SameModuleScoredSeverityNone verifies Wave 5 local-complexity scoring:
// a same-module edge is scored with the book formula at the same-module rung
// (D=2) but keeps SeverityNone, so the bc/imbalanced_coupling advisory pipeline
// (which keys on Severity) never fires for it. Abstain rules are identical:
// unknown strength still abstains at same-module distance.
func TestRun_SameModuleScoredSeverityNone(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{pathsA}, Subdomain: subdomainCore},
	}
	cfg := config.ClassifyConfig{Modules: modules}

	// Low strength (model) at same-module distance in a volatile (core) module:
	// |3-2|=1, 10-10=0, max(1,0)+1=2 → critical band — the ball-of-mud quadrant.
	ballOfMud := graph.Edge{
		From:         "file:services/a/mud.go",
		To:           "file:services/a/y.go",
		Kind:         graph.EdgeKindImports,
		Language:     "go",
		StrengthHint: string(coupling.StrengthModel),
	}
	// Unknown strength — must abstain even though distance is known.
	unknownStrength := graph.Edge{
		From:     "file:services/a/mud.go",
		To:       "file:services/a/z.go",
		Kind:     graph.EdgeKindImports,
		Language: "go",
	}
	idx := classify.Run(makeGraph([]graph.Edge{ballOfMud, unknownStrength}), cfg)

	cl := idx[edgeKey(ballOfMud)]
	if cl.Distance != coupling.DistanceSameModule {
		t.Fatalf("Distance = %q, want same_module", cl.Distance)
	}
	if !cl.Score.Scored {
		t.Fatal("same-module edge with known strength must be scored (local complexity quadrant)")
	}
	if cl.Score.Balance != 2 || cl.Score.Band != coupling.SeverityCritical {
		t.Errorf("Score = balance %d band %q, want balance 2 band critical", cl.Score.Balance, cl.Score.Band)
	}
	if cl.Severity != coupling.SeverityNone {
		t.Errorf("Severity = %q, want none — same-module edges must not reach the advisory pipeline", cl.Severity)
	}

	ab := idx[edgeKey(unknownStrength)]
	if ab.Score.Scored {
		t.Errorf("unknown-strength same-module edge must abstain, got balance %d", ab.Score.Balance)
	}
	if ab.Severity != coupling.SeverityNone {
		t.Errorf("abstained edge Severity = %q, want none", ab.Severity)
	}
}
