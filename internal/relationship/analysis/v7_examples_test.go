// Worked examples of bc_score.v7 (docs/design/bc-measurement-v7.md). Each test
// builds the scenario with the names the example uses and checks the score the
// design promises, so a change to distance or strength rules fails here first.
package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
)

type exampleEdge struct {
	fromFile, toFile string
	hint, data       string
}

func exampleGraph(edges ...exampleEdge) *graph.Graph {
	var nodes []graph.Node
	es := make([]graph.Edge, 0, len(edges))
	seen := map[string]bool{}
	node := func(path string) {
		if !seen[path] {
			seen[path] = true
			nodes = append(nodes, graph.Node{Kind: graph.NodeKindFile, Path: path, Language: graph.LangGo})
		}
	}
	for _, e := range edges {
		node(e.fromFile)
		node(e.toFile)
		es = append(es, graph.Edge{
			From: "file:" + e.fromFile, To: "file:" + e.toFile, Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: e.hint, DataStrengthHint: e.data, Locations: []graph.Location{{File: e.fromFile, Line: 1}},
		})
	}
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: es}})
}

func analyze(modules map[string]policy.ModuleDef, edges ...exampleEdge) relationship.AnalysisResult {
	return analysis.Analyze(analysis.Input{Graph: exampleGraph(edges...), Policy: relationshipPolicy(modules)})
}

func seamBetween(t *testing.T, res relationship.AnalysisResult, from, to string) relationship.Seam {
	t.Helper()
	for _, s := range res.Assessment.Seams {
		if s.FromModule == from && s.ToModule == to {
			return s
		}
	}
	t.Fatalf("seam %s -> %s not found", from, to)
	return relationship.Seam{}
}

func requireScore(t *testing.T, res relationship.AnalysisResult, wantStrength relationship.Strength, wantBalance int, wantBand relationship.Severity) relationship.Edge {
	t.Helper()
	e := onlyEdge(t, res)
	if e.Strength != wantStrength || e.Classified.Score.Balance != wantBalance || e.Severity != wantBand {
		t.Fatalf("edge = %s balance %d band %q (distance %s, volatility %s), want %s balance %d band %q",
			e.Strength, e.Classified.Score.Balance, e.Severity, e.Distance, e.Volatility, wantStrength, wantBalance, wantBand)
	}
	return e
}

const (
	fileCart      = "sales/cart/c.go"
	hintInterface = "contract"
	hintCall      = "functional"
	hintIntrusive = "intrusive"
)

func core(owner, deploy string, paths ...string) policy.ModuleDef {
	return policy.ModuleDef{Paths: paths, Owner: owner, DeployUnit: deploy, Subdomain: subdomainCore}
}

// Example 1: one owner, one deploy unit. `orders` calls an interface in
// `billing`. The call is contract coupling at D=9: balanced, for any spelling of
// the module keys.
func TestV7Example1_PortCallInOneService(t *testing.T) {
	for name, keys := range map[string][2]string{
		"flat keys":   {"orders", "billing"},
		"nested keys": {"internal/shop/orders", "internal/shop/deep/billing"},
	} {
		t.Run(name, func(t *testing.T) {
			modules := map[string]policy.ModuleDef{
				keys[0]: core("team", "svc", "orders/**"),
				keys[1]: core("team", "svc", "billing/**"),
			}
			res := analyze(modules, exampleEdge{fromFile: "orders/o.go", toFile: "billing/b.go", hint: hintInterface})
			requireScore(t, res, relationship.StrengthContract, 9, relationship.SeverityNone)
		})
	}
}

// Example 2: an adapter reads `billing/store`, which `billing` declares
// internal. Intrusive at D=9 into a core module is critical; into a supporting
// module the target's low volatility rescues it.
func TestV7Example2_InternalReachInOneService(t *testing.T) {
	billing := func(subdomain string) policy.ModuleDef {
		return policy.ModuleDef{Paths: []string{"billing/**"}, Internal: []string{"billing/store/**"}, Owner: "team", DeployUnit: "svc", Subdomain: subdomain}
	}
	tests := []struct {
		subdomain string
		balance   int
		band      relationship.Severity
	}{
		{"core", 2, relationship.SeverityCritical},
		{"supporting", 8, relationship.SeverityLow},
	}
	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			modules := map[string]policy.ModuleDef{
				"adapter": core("team", "svc", "adapter/**"),
				"billing": billing(tt.subdomain),
			}
			res := analyze(modules, exampleEdge{fromFile: "adapter/a.go", toFile: "billing/store/s.go", hint: hintCall})
			requireScore(t, res, relationship.StrengthIntrusive, tt.balance, tt.band)
		})
	}
}

// Example 4: two deploy units. `web` calls an interface in `inventory`. A call
// through a port is contract and never reads as a distributed monolith.
func TestV7Example4_PortCallAcrossDeployUnits(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"web":       core("team-a", "web", "web/**"),
		"inventory": core("team-b", "inventory", "inventory/**"),
	}
	modules["inventory"] = policy.ModuleDef{Paths: []string{"inventory/**"}, Public: []string{"inventory/api/**"}, Owner: "team-b", DeployUnit: "inventory", Subdomain: subdomainCore}
	res := analyze(modules, exampleEdge{fromFile: "web/w.go", toFile: "inventory/api/r.go", hint: hintInterface})
	e := requireScore(t, res, relationship.StrengthContract, 9, relationship.SeverityNone)
	if e.Distance != relationship.DistanceCrossDeployUnit {
		t.Errorf("distance = %s, want cross_deploy_unit", e.Distance)
	}
	if seamBetween(t, res, "web", "inventory").DistributedMonolith {
		t.Error("a port call across deploy units must never qualify as a distributed monolith")
	}
}

// Example 5: nested modules. `sales/cart` calls its sibling `sales/invoice` and
// `stock/lookup` on another branch. Both calls are functional into core
// modules, so both are critical, and the seam names the container each pair
// shares.
func TestV7Example5_NestedModulesScoreAlike(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"sales":   core("team", "svc", "sales/**"),
		"cart":    core("team", "svc", "sales/cart/**"),
		"invoice": core("team", "svc", "sales/invoice/**"),
		"stock":   core("team", "svc", "stock/**"),
		"lookup":  core("team", "svc", "stock/lookup/**"),
	}
	res := analyze(modules,
		exampleEdge{fromFile: fileCart, toFile: "sales/invoice/i.go", hint: hintCall},
		exampleEdge{fromFile: fileCart, toFile: "stock/lookup/l.go", hint: hintCall},
	)
	sibling := seamBetween(t, res, "cart", "invoice")
	cross := seamBetween(t, res, "cart", "lookup")
	for name, s := range map[string]relationship.Seam{"sibling": sibling, "other branch": cross} {
		if s.Severity != relationship.SeverityCritical {
			t.Errorf("%s seam severity = %q, want critical", name, s.Severity)
		}
	}
	if got, want := sibling.RawDistance.Basis, "module_boundary@sales"; got != want {
		t.Errorf("sibling basis = %q, want %q", got, want)
	}
	if got, want := cross.RawDistance.Basis, "module_boundary@system"; got != want {
		t.Errorf("other-branch basis = %q, want %q", got, want)
	}
	if sibling.RawDistance.BoundaryCrossings != 2 || cross.RawDistance.BoundaryCrossings != 4 {
		t.Errorf("boundary crossings = %d and %d, want 2 and 4", sibling.RawDistance.BoundaryCrossings, cross.RawDistance.BoundaryCrossings)
	}
}

// Renaming module keys changes no score, no distance and no boundary count: the
// containment tree reads declared paths only.
func TestV7KeyRenameChangesNothing(t *testing.T) {
	build := func(sales, cart string) relationship.Seam {
		modules := map[string]policy.ModuleDef{
			sales: core("team", "svc", "sales/**"),
			cart:  core("team", "svc", "sales/cart/**"),
			"x":   core("team", "svc", "x/**"),
		}
		res := analyze(modules, exampleEdge{fromFile: fileCart, toFile: "x/x.go", hint: hintCall})
		return seamBetween(t, res, cart, "x")
	}
	a, b := build("sales", "cart"), build("zz-sales", "zz-cart")
	if a.Severity != b.Severity || a.Distance != b.Distance || a.RawDistance.BoundaryCrossings != b.RawDistance.BoundaryCrossings || a.RawDistance.Basis != b.RawDistance.Basis {
		t.Errorf("rename changed the seam: %+v vs %+v", a.RawDistance, b.RawDistance)
	}
}
