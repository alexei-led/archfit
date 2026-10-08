// Worked examples of bc_score.v7 (docs/design/bc-measurement-v7.md). Each test
// builds the scenario with the names the example uses and checks the score the
// design promises, so a change to distance or strength rules fails here first.
package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/clone"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
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
	fileAPIX            = "a/api/x.go"
	fileAPIY            = "b/api/y.go"
	fileOrders          = "orders/o.go"
	subdomainSupporting = "supporting"
	volMedium           = "medium"
	fileCart            = "sales/cart/c.go"
	hintInterface       = "contract"
	hintCall            = "functional"
	hintIntrusive       = "intrusive"
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
			res := analyze(modules, exampleEdge{fromFile: fileOrders, toFile: "billing/b.go", hint: hintInterface})
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
		{subdomainSupporting, 8, relationship.SeverityLow},
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
	a, b := build("sales", "cart"), build("deep/er/sales", "very/deep/er/cart")
	if a.Severity != b.Severity || a.Distance != b.Distance || a.RawDistance.BoundaryCrossings != b.RawDistance.BoundaryCrossings || a.RawDistance.Basis != b.RawDistance.Basis {
		t.Errorf("rename changed the seam: %+v vs %+v", a.RawDistance, b.RawDistance)
	}
}

// Example 3: two teams, one deploy unit. `orders` (core) calls the non-public
// `pricing/engine` (supporting). Functional coupling ties both sides, so V is the
// worse of the two: high. The seam qualifies. Through `pricing/api`, a declared
// public surface, the same call is contract and balanced.
func TestV7Example3_FunctionalCallTiesBothSides(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"orders":  core("team-a", "svc", "orders/**"),
		"pricing": {Paths: []string{"pricing/**"}, Public: []string{"pricing/api/**"}, Owner: "team-b", DeployUnit: "svc", Subdomain: subdomainSupporting},
	}
	res := analyze(modules, exampleEdge{fromFile: fileOrders, toFile: "pricing/engine/e.go", hint: hintCall})
	e := requireScore(t, res, relationship.StrengthFunctional, 2, relationship.SeverityCritical)
	if e.Volatility != relationship.VolatilityHigh {
		t.Errorf("volatility = %s, want high: the worse of core and supporting", e.Volatility)
	}
	s := seamBetween(t, res, "orders", "pricing")
	if !s.DistributedMonolith {
		t.Error("functional coupling into a non-public module must qualify the seam")
	}
	if s.Hypothesis != relationship.SeamHypothesisIntroduceContract {
		t.Errorf("hypothesis = %q, want introduce_contract: the target declares a public surface", s.Hypothesis)
	}

	through := analyze(modules, exampleEdge{fromFile: fileOrders, toFile: "pricing/api/p.go", hint: hintCall})
	requireScore(t, through, relationship.StrengthContract, 9, relationship.SeverityNone)
}

// Example 6: no volatility declared anywhere. An interface call is contract and
// balanced. A call to a concrete function scores critical, but nobody declared
// the volatility, so the seam is unrated: it never qualifies, it says
// declare_volatility, and the coupling summary counts the unrated edge.
func TestV7Example6_UndeclaredVolatilityIsUnrated(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"app":      {Paths: []string{"app/**"}},
		"adapters": {Paths: []string{"adapters/**"}},
		"rules":    {Paths: []string{"rules/**"}},
	}
	res := analyze(modules,
		exampleEdge{fromFile: "app/a.go", toFile: "adapters/s.go", hint: hintInterface},
		exampleEdge{fromFile: "app/a.go", toFile: "rules/d.go", hint: hintCall},
	)
	port := seamBetween(t, res, "app", "adapters")
	if port.Severity != relationship.SeverityNone || port.DistributedMonolith {
		t.Errorf("port seam = %q qualifying %t, want none and not qualifying", port.Severity, port.DistributedMonolith)
	}
	concrete := seamBetween(t, res, "app", "rules")
	if concrete.Severity != relationship.SeverityCritical {
		t.Errorf("concrete seam severity = %q, want critical", concrete.Severity)
	}
	if concrete.DistributedMonolith {
		t.Error("an unrated seam must not qualify")
	}
	if concrete.Hypothesis != relationship.SeamHypothesisDeclareVolatility {
		t.Errorf("hypothesis = %q, want declare_volatility", concrete.Hypothesis)
	}
	s := res.Assessment.ClassifiedEdges
	if s.UnratedVolatilityEdges != 2 {
		t.Errorf("unrated edges = %d, want 2 (both edges end in undeclared volatility)", s.UnratedVolatilityEdges)
	}
	if got := s.UnratedVolatilityModules; len(got) != 3 {
		t.Errorf("unrated modules = %v, want app, adapters, rules", got)
	}
}

// The qualification rule: strong strength, a module boundary, declared high
// volatility, and no cohesive source role unless the coupling is intrusive.
// Every qualifying edge is critical, so the gate set is a subset of the severity set.
func TestV7QualificationRule(t *testing.T) {
	type tc struct {
		name     string
		hint     string
		srcVol   string
		dstVol   string
		role     policy.Role
		want     bool
		wantBand relationship.Severity
	}
	tests := []tc{
		{"functional, high", hintCall, volHigh, volHigh, "", true, relationship.SeverityCritical},
		{"functional, source high, target low", hintCall, volHigh, volLow, "", true, relationship.SeverityCritical},
		{"functional, both low", hintCall, volLow, volLow, "", false, relationship.SeverityLow},
		{"functional, medium never qualifies", hintCall, volMedium, volMedium, "", false, relationship.SeverityMedium},
		{"functional, undeclared is unrated", hintCall, "", "", "", false, relationship.SeverityCritical},
		{"functional from a composition root", hintCall, volHigh, volHigh, policy.RoleCompositionRoot, false, relationship.SeverityCritical},
		{"functional from generated code", hintCall, volHigh, volHigh, policy.RoleGenerated, false, relationship.SeverityCritical},
		{"functional from test code", hintCall, volHigh, volHigh, policy.RoleTest, false, relationship.SeverityCritical},
		{"intrusive, high", hintIntrusive, volHigh, volHigh, "", true, relationship.SeverityCritical},
		{"intrusive from a composition root still qualifies", hintIntrusive, volHigh, volHigh, policy.RoleCompositionRoot, true, relationship.SeverityCritical},
		{"interface call is contract", hintInterface, volHigh, volHigh, "", false, relationship.SeverityNone},
		{"model coupling is below the bar", "model", volHigh, volHigh, "", false, relationship.SeverityLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modules := map[string]policy.ModuleDef{
				"src": {Paths: []string{"src/**"}, Volatility: tt.srcVol, Role: tt.role, Internal: nil},
				"dst": {Paths: []string{"dst/**"}, Volatility: tt.dstVol},
			}
			if tt.hint == hintIntrusive {
				d := modules["dst"]
				d.Internal = []string{"dst/internal/**"}
				modules["dst"] = d
			}
			toFile := "dst/d.go"
			if tt.hint == hintIntrusive {
				toFile = "dst/internal/d.go"
			}
			res := analyze(modules, exampleEdge{fromFile: "src/s.go", toFile: toFile, hint: tt.hint})
			s := seamBetween(t, res, "src", "dst")
			if s.DistributedMonolith != tt.want {
				t.Errorf("qualifies = %t, want %t (severity %q)", s.DistributedMonolith, tt.want, s.Severity)
			}
			if s.Severity != tt.wantBand {
				t.Errorf("severity = %q, want %q", s.Severity, tt.wantBand)
			}
			if s.DistributedMonolith && s.Severity != relationship.SeverityCritical {
				t.Errorf("a qualifying seam is critical, got %q", s.Severity)
			}
		})
	}
}

// A clone fact attaches to the seam between its modules, scores like an edge,
// and sets the seam's severity and hypothesis, but it never adds an edge and
// never makes the seam qualify.
func TestV7CloneFactAttachesToSeam(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"a": core("team", "svc", "a/**"),
		"b": core("team", "svc", "b/**"),
	}
	// "a" and "b" both declare a public surface so the import is a plain contract.
	for _, k := range []string{"a", "b"} {
		d := modules[k]
		d.Public = []string{k + "/api/**"}
		modules[k] = d
	}
	g := exampleGraph(exampleEdge{fromFile: fileAPIX, toFile: fileAPIY, hint: hintInterface})
	cloneA, cloneB := "a/dup.go", "b/dup.go"
	res := analysis.Analyze(analysis.Input{
		Graph: g, Policy: relationshipPolicy(modules),
		CloneClusters: []clone.Cluster{{Files: []string{cloneA, cloneB}, Lines: 40, Locations: []clone.LineRange{{StartLine: 1}, {StartLine: 2}}}},
		FileClassIndex: map[string]fileclass.FileClass{
			cloneA: fileclass.Production, cloneB: fileclass.Production,
			fileAPIX: fileclass.Production, fileAPIY: fileclass.Production,
		},
	})
	s := seamBetween(t, res, "a", "b")
	if s.Edges != 1 || s.ScoredEdges != 2 {
		t.Errorf("edges = %d, scored = %d, want 1 edge and 2 scored facts (edge + clone fact)", s.Edges, s.ScoredEdges)
	}
	if s.Severity != relationship.SeverityCritical {
		t.Errorf("severity = %q, want critical from the symmetric clone fact", s.Severity)
	}
	if s.DistributedMonolith {
		t.Error("a clone fact must never qualify a seam")
	}
	if s.Hypothesis != relationship.SeamHypothesisMoveFunctionality {
		t.Errorf("hypothesis = %q, want move_functionality for a clone fact", s.Hypothesis)
	}
	if len(res.Evidence.CloneOnly) != 0 {
		t.Errorf("clone-only block = %+v, want empty: the pair is a fact on the seam", res.Evidence.CloneOnly)
	}
	if edge := onlyEdge(t, res); edge.Strength != relationship.StrengthContract {
		t.Errorf("edge strength = %s, want contract: a clone never upgrades an import edge", edge.Strength)
	}
}

func cloneSetup() (map[string]policy.ModuleDef, analysis.Input) {
	modules := map[string]policy.ModuleDef{
		"a": core("team", "svc", "a/**"),
		"b": core("team", "svc", "b/**"),
	}
	for _, k := range []string{"a", "b"} {
		d := modules[k]
		d.Public = []string{k + "/api/**"}
		modules[k] = d
	}
	cloneA, cloneB := "a/dup.go", "b/dup.go"
	in := analysis.Input{
		CloneClusters: []clone.Cluster{{Files: []string{cloneA, cloneB}, Lines: 40, Locations: []clone.LineRange{{StartLine: 1}, {StartLine: 2}}}},
		FileClassIndex: map[string]fileclass.FileClass{
			cloneA: fileclass.Production, cloneB: fileclass.Production,
			fileAPIX: fileclass.Production, fileAPIY: fileclass.Production,
		},
	}
	return modules, in
}

// With imports in both directions the clone fact attaches to exactly one seam:
// the one whose ID sorts first. It is never counted twice.
func TestV7CloneFactAttachesToExactlyOneSeam(t *testing.T) {
	modules, in := cloneSetup()
	in.Graph = exampleGraph(
		exampleEdge{fromFile: fileAPIX, toFile: fileAPIY, hint: hintInterface},
		exampleEdge{fromFile: fileAPIY, toFile: fileAPIX, hint: hintInterface},
	)
	in.Policy = relationshipPolicy(modules)
	res := analysis.Analyze(in)
	ab, ba := seamBetween(t, res, "a", "b"), seamBetween(t, res, "b", "a")
	withClone, without := ab, ba
	if ba.ScoredEdges > ab.ScoredEdges {
		withClone, without = ba, ab
	}
	if withClone.ScoredEdges != 2 || without.ScoredEdges != 1 {
		t.Fatalf("scored facts = %d and %d, want 2 on one seam and 1 on the other", withClone.ScoredEdges, without.ScoredEdges)
	}
	if relationship.SeamID(withClone.FromModule, withClone.ToModule) > relationship.SeamID(without.FromModule, without.ToModule) {
		t.Error("the clone fact must attach to the seam whose ID sorts first")
	}
}

// `coupling.duplicated_knowledge: advisory` keeps clone facts out of seams and
// out of the headline score. A connected pair never becomes a clone-only
// advisory: it is a fact on the seam, and the rule means "no import edge".
func TestV7CloneFactPolicyAndAdvisories(t *testing.T) {
	modules, in := cloneSetup()
	in.Graph = exampleGraph(exampleEdge{fromFile: fileAPIX, toFile: fileAPIY, hint: hintInterface})

	score := relationshipPolicy(modules)
	score.DuplicatedKnowledge = policy.DuplicatedKnowledgePolicyScore
	in.Policy = score
	got := analysis.Analyze(in)
	s := seamBetween(t, got, "a", "b")
	if s.ScoredEdges != 2 {
		t.Errorf("score mode: scored = %d, want 2", s.ScoredEdges)
	}
	for _, c := range got.Assessment.AdvisoryCandidates {
		if c.RuleID == ruleClone {
			t.Errorf("a connected clone pair produced a %s advisory: %+v", ruleClone, c)
		}
	}
	if tr := got.Assessment.ClassifiedEdges.TailRisk; tr != nil && tr.CloneOnlyScored != 0 {
		t.Errorf("tail_risk clone-only scored = %d, want 0: the pair is connected", tr.CloneOnlyScored)
	}

	advisory := relationshipPolicy(modules)
	advisory.DuplicatedKnowledge = policy.DuplicatedKnowledgePolicyAdvisory
	in.Policy = advisory
	got = analysis.Analyze(in)
	s = seamBetween(t, got, "a", "b")
	if s.ScoredEdges != 1 || s.Severity != relationship.SeverityNone {
		t.Errorf("advisory mode: scored = %d severity %q, want 1 and none: clone facts stay out of seams", s.ScoredEdges, s.Severity)
	}
}
