// Seam-ledger behavior tests. They pin the unit of coupling reporting: one
// record per ordered module pair, its measurement denominator, its distribution,
// and the raw distance facts behind the collapsed rung.
package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

const (
	subdomainCore = "core"
	nodeM         = "file:m/m.go"
)

// seamFixture builds a graph whose module a imports module b over `imports`
// edges, each carrying the given strength hint.
func seamGraph(edges int, hint string) *graph.Graph {
	nodes := make([]graph.Node, 0, edges+1)
	nodes = append(nodes, graph.Node{Kind: graph.NodeKindFile, Path: fileB, Language: graph.LangGo})
	es := make([]graph.Edge, 0, edges)
	for i := range edges {
		from := "a/f" + string(rune('0'+i)) + ".go"
		nodes = append(nodes, graph.Node{Kind: graph.NodeKindFile, Path: from, Language: graph.LangGo})
		es = append(es, graph.Edge{
			From: "file:" + from, To: nodeB, Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: hint, Locations: []graph.Location{{File: from, Line: 3}},
		})
	}
	return graph.Build([]graph.Facts{{Nodes: nodes, Edges: es}})
}

// seamAB returns the a -> b seam, the fixture topology every seam test uses.
func seamAB(t *testing.T, res relationship.AnalysisResult) relationship.Seam {
	t.Helper()
	for _, s := range res.Assessment.Seams {
		if s.FromModule == moduleA && s.ToModule == moduleB {
			return s
		}
	}
	t.Fatalf("seam %s -> %s not found in %d seams", moduleA, moduleB, len(res.Assessment.Seams))
	return relationship.Seam{}
}

// TestSeamsGroupEdgesByOrderedModulePair is the whole point of the ledger:
// twelve imports expressing one logical seam must report as one seam carrying
// an edge count, not as twelve coupling facts (plan R4).
func TestSeamsGroupEdgesByOrderedModulePair(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(4, string(relationship.StrengthFunctional)),
		Policy: relationshipPolicy(twoModules()),
	})

	if len(got.Assessment.Seams) != 1 {
		t.Fatalf("seams = %d, want exactly one for four edges across one module pair", len(got.Assessment.Seams))
	}
	s := got.Assessment.Seams[0]
	if s.FromModule != moduleA || s.ToModule != moduleB {
		t.Fatalf("seam = %s -> %s, want a -> b", s.FromModule, s.ToModule)
	}
	if s.ID != relationship.SeamID(moduleA, moduleB) {
		t.Errorf("seam ID = %q, want the frozen SeamID of the module pair", s.ID)
	}
	if s.Edges != 4 {
		t.Errorf("edges = %d, want 4 — the edge count is seam evidence, not four seams", s.Edges)
	}
	if s.ScoredEdges+s.AbstainedEdges != s.Edges {
		t.Errorf("scored %d + abstained %d != edges %d — the denominator must account for every edge",
			s.ScoredEdges, s.AbstainedEdges, s.Edges)
	}
	if s.Scores.N != s.ScoredEdges {
		t.Errorf("distribution n = %d, want the scored-edge count %d", s.Scores.N, s.ScoredEdges)
	}
	if s.Scores.Min > s.Scores.Median || s.Scores.Median > s.Scores.Max {
		t.Errorf("distribution is not ordered: min %d median %d max %d", s.Scores.Min, s.Scores.Median, s.Scores.Max)
	}
	if s.Scores.P10 != nil || s.Scores.P90 != nil {
		t.Error("deciles reported over four samples — p10/p90 must abstain below ten")
	}
}

// TestSeamsAbstainedEdgesStayInTheDenominator pins abstain-not-fake at seam
// level: an unknown strength lowers confidence, it does not shrink the seam.
func TestSeamsAbstainedEdgesStayInTheDenominator(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(2, ""), // no hint → strength unknown → abstain
		Policy: relationshipPolicy(twoModules()),
	})

	s := seamAB(t, got)
	if s.ScoredEdges != 0 || s.AbstainedEdges != 2 {
		t.Fatalf("scored/abstained = %d/%d, want 0/2 for unknown strength", s.ScoredEdges, s.AbstainedEdges)
	}
	if s.Edges != 2 {
		t.Errorf("edges = %d, want 2 — an abstained edge is still a seam edge", s.Edges)
	}
	if s.Confidence != relationship.SeamConfidenceUnrated {
		t.Errorf("confidence = %q, want unrated when nothing scored", s.Confidence)
	}
	if s.Scores.N != 0 {
		t.Errorf("distribution n = %d, want 0 — no invented scores for abstained edges", s.Scores.N)
	}
	if s.Hypothesis != "" {
		t.Errorf("hypothesis = %q, want none when no edge was scored", s.Hypothesis)
	}
}

// TestSeamsExcludeNonSeamEdges pins what is not a seam: same-module edges are a
// different fractal level and an unresolved target is external hygiene.
func TestSeamsExcludeNonSeamEdges(t *testing.T) {
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: "a/one.go", Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: "a/two.go", Language: graph.LangGo},
			{Kind: graph.NodeKindExternal, Path: "github.com/vendor/sdk", Language: graph.LangGo},
		},
		Edges: []graph.Edge{
			{From: "file:a/one.go", To: "file:a/two.go", Kind: graph.EdgeKindImports, Language: graph.LangGo,
				StrengthHint: string(relationship.StrengthFunctional)},
			{From: "file:a/one.go", To: "external:github.com/vendor/sdk", Kind: graph.EdgeKindImports,
				Language: graph.LangGo, StrengthHint: string(relationship.StrengthFunctional)},
		},
	}})

	got := analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(twoModules())})
	if len(got.Assessment.Seams) != 0 {
		t.Fatalf("seams = %+v, want none: same-module coupling and undeclared external targets are not seams",
			got.Assessment.Seams)
	}
}

// TestSeamRawDistanceExposesOwnerAndDeployFacts pins R3: the collapsed rung
// alone cannot be audited, so the owner and deploy facts travel beside it.
func TestSeamRawDistanceExposesOwnerAndDeployFacts(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(1, string(relationship.StrengthFunctional)),
		Policy: relationshipPolicy(twoModules()),
	})

	raw := seamAB(t, got).RawDistance
	if raw.Level != relationship.DistanceCrossDeployUnit {
		t.Errorf("level = %q, want cross_deploy_unit for differing deploy units", raw.Level)
	}
	if raw.FromOwner != teamA || raw.ToOwner != teamB {
		t.Errorf("owners = %q -> %q, want the declared owners exposed raw", raw.FromOwner, raw.ToOwner)
	}
	if raw.SameOwner {
		t.Error("same_owner = true for two distinct declared owners")
	}
	if raw.FromDeployUnit != moduleA || raw.ToDeployUnit != moduleB || raw.SameDeployUnit {
		t.Errorf("deploy units = %q -> %q (same=%v), want the declared units exposed raw",
			raw.FromDeployUnit, raw.ToDeployUnit, raw.SameDeployUnit)
	}
	if raw.Basis == "" {
		t.Error("basis = empty, want the deterministic signal that selected the rung")
	}
}

// TestSeamSameOwnerNeedsTwoDeclaredOwners pins the honest reading of a
// degenerate ownership map: two undeclared owners are not shared ownership.
func TestSeamSameOwnerNeedsTwoDeclaredOwners(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		moduleA: {Paths: []string{globA}, Subdomain: subdomainCore, Volatility: volHigh},
		moduleB: {Paths: []string{globB}, Subdomain: subdomainCore, Volatility: volHigh},
	}
	got := analysis.Analyze(analysis.Input{
		Graph: seamGraph(1, string(relationship.StrengthFunctional)),
		Policy: policy.RelationshipPolicy{Topology: policy.TopologyView{
			Modules: modules, ModuleMap: policy.BuildModuleMap(modules),
		}},
	})

	raw := seamAB(t, got).RawDistance
	if raw.SameOwner {
		t.Error("same_owner = true with no declared owner on either side")
	}
	if raw.SameDeployUnit {
		t.Error("same_deploy_unit = true with no declared deploy unit on either side")
	}
}

// TestSeamCompositionRootExpectationLeavesTheSeamAlone pins the role-aware
// diagnostic: wiring fan-out from a declared composition root is the module's
// purpose, and saying so must not require a strength label.
func TestSeamCompositionRootExpectationLeavesTheSeamAlone(t *testing.T) {
	modules := twoModules()
	def := modules[moduleA]
	def.Role = policy.RoleCompositionRoot
	modules[moduleA] = def

	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(1, string(relationship.StrengthIntrusive)),
		Policy: relationshipPolicy(modules),
	})

	s := seamAB(t, got)
	if s.RoleExpectation != relationship.SeamRoleCompositionRoot {
		t.Errorf("role expectation = %q, want composition_root from the declared module role", s.RoleExpectation)
	}
	if s.Hypothesis != relationship.SeamHypothesisLeaveAlone {
		t.Errorf("hypothesis = %q, want leave_alone for a composition-root source", s.Hypothesis)
	}
	if len(s.Labels) != 0 {
		t.Errorf("labels = %v, want none: a role expectation is never encoded as a strength label", s.Labels)
	}
}

// TestSeamWithoutRoleReportsNoExpectation pins the abstention: an undeclared
// role produces no expectation rather than a default one.
func TestSeamWithoutRoleReportsNoExpectation(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(1, string(relationship.StrengthIntrusive)),
		Policy: relationshipPolicy(twoModules()),
	})

	s := seamAB(t, got)
	if s.RoleExpectation != "" {
		t.Errorf("role expectation = %q, want none for a module that declared no role", s.RoleExpectation)
	}
	if s.Hypothesis == "" {
		t.Error("hypothesis = empty for a scored seam, want a concrete cheapest move")
	}
}

// TestSeamDistributedMonolithNeedsCriticalAndHighDistance pins the seam
// qualification: a critical band at short distance is local coupling, and a
// long distance in a healthy band is a declared boundary. Only both together
// qualify.
func TestSeamDistributedMonolithNeedsCriticalAndHighDistance(t *testing.T) {
	tests := []struct {
		name    string
		modules map[string]policy.ModuleDef
		hint    string
		want    bool
	}{
		{
			name: "intrusive across deploy units qualifies",
			// intrusive S=10, cross_deploy_unit D=9, high volatility V=10
			// → max(1, 0)+1 = 2 → critical.
			modules: twoModules(), hint: string(relationship.StrengthIntrusive), want: true,
		},
		{
			name: "critical band at same-owner distance is local coupling",
			modules: map[string]policy.ModuleDef{
				moduleA: {Paths: []string{globA}, Owner: teamA, DeployUnit: moduleA, Subdomain: subdomainCore, Volatility: volHigh},
				moduleB: {Paths: []string{globB}, Owner: teamA, DeployUnit: moduleA, Subdomain: subdomainCore, Volatility: volHigh},
			},
			hint: string(relationship.StrengthContract), want: false,
		},
		{
			name: "contract across deploy units is a declared boundary",
			// contract S=1, cross_deploy_unit D=9 → max(8, 0)+1 = 9 → none.
			modules: twoModules(), hint: string(relationship.StrengthContract), want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := analysis.Analyze(analysis.Input{
				Graph: seamGraph(1, tc.hint), Policy: relationshipPolicy(tc.modules),
			})
			s := seamAB(t, got)
			if s.DistributedMonolith != tc.want {
				t.Errorf("distributed_monolith = %v, want %v (severity %q, distance %q)",
					s.DistributedMonolith, tc.want, s.Severity, s.Distance)
			}
		})
	}
}

// TestSeamQuadrantSeparatesTightFromLoose pins the book Ch10 quadrant: the same
// distance reports tight or loose depending only on integration strength.
func TestSeamQuadrantSeparatesTightFromLoose(t *testing.T) {
	tests := []struct {
		name string
		hint string
		want relationship.SeamQuadrant
	}{
		{"intrusive far apart is tight", string(relationship.StrengthIntrusive), relationship.SeamQuadrantTight},
		{"contract far apart is loose", string(relationship.StrengthContract), relationship.SeamQuadrantLoose},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := analysis.Analyze(analysis.Input{
				Graph: seamGraph(1, tc.hint), Policy: relationshipPolicy(twoModules()),
			})
			if q := seamAB(t, got).Quadrant; q != tc.want {
				t.Errorf("quadrant = %q, want %q", q, tc.want)
			}
		})
	}
}

// TestSeamsAreDeterministicallyOrdered pins reproducibility: the ledger is
// built from map-keyed accumulation, so the output order must not depend on
// Go's map iteration.
func TestSeamsAreDeterministicallyOrdered(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"z": {Paths: []string{"z/**"}, Owner: teamA, Subdomain: subdomainCore, Volatility: volHigh},
		"m": {Paths: []string{"m/**"}, Owner: teamB, Subdomain: subdomainCore, Volatility: volHigh},
		"a": {Paths: []string{"a/**"}, Owner: teamA, Subdomain: subdomainCore, Volatility: volHigh},
	}
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: "z/z.go", Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: "m/m.go", Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: "a/a.go", Language: graph.LangGo},
		},
		Edges: []graph.Edge{
			{From: "file:z/z.go", To: nodeM, Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(relationship.StrengthFunctional)},
			{From: "file:a/a.go", To: nodeM, Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(relationship.StrengthFunctional)},
			{From: nodeM, To: "file:a/a.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(relationship.StrengthFunctional)},
		},
	}})
	in := analysis.Input{Graph: g, Policy: policy.RelationshipPolicy{Topology: policy.TopologyView{
		Modules: modules, ModuleMap: policy.BuildModuleMap(modules),
	}}}

	want := []string{"a -> m", "m -> a", "z -> m"}
	for range 5 {
		got := make([]string, 0, len(want))
		for _, s := range analysis.Analyze(in).Assessment.Seams {
			got = append(got, s.FromModule+" -> "+s.ToModule)
		}
		if len(got) != len(want) {
			t.Fatalf("seams = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("seam order = %v, want %v", got, want)
			}
		}
	}
}

// TestStaleLabelDisablesTheOverrideAndIsReported pins the stale-evidence rule:
// a label whose evidence hash no longer matches the dependency surface stops
// applying — the strength falls back to what the static sources actually saw —
// and the run says so. Silently applying a label approved against a different
// surface is the failure mode; silently dropping it without a word is the other.
func TestStaleLabelDisablesTheOverrideAndIsReported(t *testing.T) {
	stale := labels.Label{
		From: moduleA, To: moduleB, Strength: string(relationship.StrengthContract),
		Status: labels.StatusApproved, EvidenceHash: "hash-from-a-different-tree",
		Provenance: labels.ProvenanceHuman,
	}
	in := analysis.Input{
		Graph:  seamGraph(1, string(relationship.StrengthIntrusive)),
		Policy: relationshipPolicy(twoModules()),
		Labels: []labels.Label{stale},
	}

	got := analysis.Analyze(in)
	if len(got.Assessment.StaleLabelKeys) != 1 {
		t.Fatalf("stale label keys = %v, want the stale pair reported", got.Assessment.StaleLabelKeys)
	}
	s := seamAB(t, got)
	if s.Strength != relationship.StrengthIntrusive {
		t.Errorf("strength = %q, want the extractor's intrusive hint — a stale label must not override", s.Strength)
	}
	if len(s.Labels) != 0 {
		t.Errorf("seam labels = %v, want none: a disabled override is not in effect", s.Labels)
	}
}

// TestApprovedLabelIsReportedOnTheSeam pins the other half: a fresh approved
// label applies and the seam discloses which label did it, so a reader can tell
// a measured strength from a reviewed one.
func TestApprovedLabelIsReportedOnTheSeam(t *testing.T) {
	// A full run (not a base run) with no evidence hash recorded on the label
	// leaves the freshness check unanchored, which is the "approved and in
	// effect" path.
	fresh := labels.Label{
		From: moduleA, To: moduleB, Strength: string(relationship.StrengthContract),
		Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman,
	}
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(1, string(relationship.StrengthIntrusive)),
		Policy: relationshipPolicy(twoModules()),
		Labels: []labels.Label{fresh},
		Mode:   analysis.Mode{Full: true},
	})

	if len(got.Assessment.StaleLabelKeys) != 0 {
		t.Errorf("stale label keys = %v, want none for a label in effect", got.Assessment.StaleLabelKeys)
	}
	s := seamAB(t, got)
	if len(s.Labels) != 1 || s.Labels[0] != labels.Key(moduleA, moduleB) {
		t.Errorf("seam labels = %v, want the applied label key disclosed", s.Labels)
	}
}

// TestHandAuthoredLabelPublishesNoEvidenceHash pins what
// `label_evidence_hash` means. A hand-authored label with no stored hash is
// effective — isEffective skips the freshness check for it — and the seam used
// to publish THIS RUN'S computed hash under a field documenting the evidence the
// approval rested on. That is a claim nobody made: the reviewer never saw a
// hash, so the honest answer is none.
func TestHandAuthoredLabelPublishesNoEvidenceHash(t *testing.T) {
	fresh := labels.Label{
		From: moduleA, To: moduleB, Strength: string(relationship.StrengthContract),
		Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman,
	}
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(1, string(relationship.StrengthIntrusive)),
		Policy: relationshipPolicy(twoModules()),
		Labels: []labels.Label{fresh},
		Mode:   analysis.Mode{Full: true},
	})

	s := seamAB(t, got)
	if len(s.Labels) != 1 {
		t.Fatalf("seam labels = %v, want the applied label key disclosed", s.Labels)
	}
	if s.LabelEvidenceHash != "" {
		t.Errorf("label_evidence_hash = %q, want empty: the label stored none", s.LabelEvidenceHash)
	}
}

// TestSeamOfWhollyAbstainedEdgesReportsUnknownStrength pins the strength rung,
// whose "worst so far" comparison is rank-based. The zero Strength shares a
// rank with StrengthUnknown, so an all-abstained seam — the normal state on a
// heuristic-strength language with no SCIP and no public:/internal: globs —
// never overwrote the zero value and published "" outside its own vocabulary.
// Its distribution has no samples at all, so N must stay 0 rather than report a
// median of zero, which is below the book's 1..10 range.
//
// Severity is exempt: SeverityNone IS the zero value, so the zero is correct.
func TestSeamOfWhollyAbstainedEdgesReportsUnknownStrength(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph:  seamGraph(2, ""),
		Policy: relationshipPolicy(twoModules()),
	})

	s := seamAB(t, got)
	if s.ScoredEdges != 0 || s.AbstainedEdges == 0 {
		t.Fatalf("seam scored=%d abstained=%d, want a wholly abstained seam", s.ScoredEdges, s.AbstainedEdges)
	}
	if s.Strength != relationship.StrengthUnknown {
		t.Errorf("strength = %q, want %q", s.Strength, relationship.StrengthUnknown)
	}
	if s.Severity != relationship.SeverityNone {
		t.Errorf("severity = %q, want none — no edge produced a band", s.Severity)
	}
	if s.Scores.N != 0 {
		t.Errorf("scores.n = %d, want 0 — no edge contributed a balance", s.Scores.N)
	}
}
