package analysis_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/clone"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
)

func TestCloneAdvisoryAsksWhetherKnowledgeEvolvesTogether(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph: graph.Build([]graph.Facts{{Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileB, Language: graph.LangGo},
		}}}),
		Policy: relationshipPolicy(twoModules()),
		CloneClusters: []clone.Cluster{{
			Files: []string{fileA, fileB}, Lines: 40,
			Locations: []clone.LineRange{{StartLine: 1}, {StartLine: 2}},
		}},
		FileClassIndex: map[string]fileclass.FileClass{fileA: fileclass.Production, fileB: fileclass.Production},
	})
	if len(got.Assessment.AdvisoryCandidates) != 1 || len(got.Evidence.CloneOnly) != 1 {
		t.Fatalf("want one clone advisory and pair, got %+v", got)
	}
	candidate, pair := got.Assessment.AdvisoryCandidates[0], got.Evidence.CloneOnly[0]
	if candidate.ID == "" || candidate.RuleID != ruleClone || candidate.Kind != "advisory" || candidate.EdgeKind != "clone" ||
		candidate.FromModule != moduleA || candidate.ToModule != moduleB || candidate.Severity != pair.Severity ||
		candidate.MatchedBy["strength"] != string(relationship.StrengthSymmetric) ||
		candidate.MatchedBy["score_policy"] != string(policy.DuplicatedKnowledgePolicyScore) ||
		candidate.MatchedBy["hypothesis"] != string(relationship.SeamHypothesisMoveFunctionality) {
		t.Errorf("non-copy clone facts changed: %+v", candidate)
	}
	for _, want := range []string{
		"code clones between a and b with no import edge",
		"Inspect whether the similar code represents shared knowledge that must evolve together",
		"If it does, review how to keep changes consistent across the modules",
		"extraction is not automatically the right fix",
	} {
		if !strings.Contains(candidate.Why, want) {
			t.Errorf("why = %q, want %q", candidate.Why, want)
		}
	}
	for _, unsupported := range []string{"must be repeated in both modules", "Extract the shared knowledge", "approved label"} {
		if strings.Contains(candidate.Why, unsupported) {
			t.Errorf("why prescribes a fix without inspecting shared knowledge: %q", candidate.Why)
		}
	}
}

// TestCouplingAdvisoryNeverCallsACriticalEdgeCheap pins the advisory wording
// at every severity and distance: a critical edge is the worst band the book
// scorer gives, so no text may call it cheap to change.
func TestCouplingAdvisoryNeverCallsACriticalEdgeCheap(t *testing.T) {
	for _, sev := range []relationship.Severity{relationship.SeverityCritical, relationship.SeverityHigh, relationship.SeverityMedium} {
		for _, dist := range []relationship.Distance{relationship.DistanceCrossModule, relationship.DistanceCrossDeployUnit} {
			why := analysis.BCAdvisoryWhy(relationship.Edge{
				Strength: relationship.StrengthModel, Distance: dist, Volatility: relationship.VolatilityHigh, Severity: sev,
			})
			if strings.Contains(why, "cheap") {
				t.Errorf("%s at %s: why calls the edge cheap: %s", sev, dist, why)
			}
		}
	}
}

func TestCouplingAdvisoryExplainsEffectiveVolatility(t *testing.T) {
	modules := twoModules()
	target := modules[moduleB]
	target.Volatility = volLow
	modules[moduleB] = target
	got := analysis.Analyze(analysis.Input{
		Graph: graphWith(string(relationship.StrengthFunctional)), Policy: relationshipPolicy(modules),
	})
	if edge := onlyEdge(t, got); edge.Volatility != relationship.VolatilityHigh {
		t.Fatalf("effective volatility = %s, want high from source", edge.Volatility)
	}
	if len(got.Assessment.AdvisoryCandidates) != 1 {
		t.Fatalf("advisories = %d, want 1", len(got.Assessment.AdvisoryCandidates))
	}
	why := got.Assessment.AdvisoryCandidates[0].Why
	for _, want := range []string{"high effective volatility", "higher of source and target"} {
		if !strings.Contains(why, want) {
			t.Errorf("why = %q, want %q", why, want)
		}
	}
	if strings.Contains(why, "volatile target") || strings.Contains(why, "forces a change") {
		t.Errorf("why overclaims target volatility or certain propagation: %s", why)
	}
}

func TestCouplingAdvisoryExternalVolatilityUsesTarget(t *testing.T) {
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindPackage, Path: "vendor.example/sdk", Language: graph.LangGo},
		},
		Edges: []graph.Edge{{From: nodeA, To: "package:vendor.example/sdk", Kind: graph.EdgeKindImports,
			Language: graph.LangGo, StrengthHint: string(relationship.StrengthFunctional)}},
	}})
	modules := twoModules()
	source := modules[moduleA]
	source.Volatility = volLow
	modules[moduleA] = source
	p := relationshipPolicy(modules)
	p.Topology.ExternalSystems = map[string]policy.ExternalSystemDef{
		"vendor": {Targets: []string{"vendor.example/**"}, Volatility: volHigh},
	}
	got := analysis.Analyze(analysis.Input{Graph: g, Policy: p})
	if edge := onlyEdge(t, got); edge.Distance != relationship.DistanceExternal || edge.Volatility != relationship.VolatilityHigh {
		t.Fatalf("external distance/volatility = %s/%s, want declared_external/high", edge.Distance, edge.Volatility)
	}
	if len(got.Assessment.AdvisoryCandidates) != 1 {
		t.Fatalf("advisories = %d, want 1", len(got.Assessment.AdvisoryCandidates))
	}
	why := got.Assessment.AdvisoryCandidates[0].Why
	if !strings.Contains(why, "high effective volatility") || strings.Contains(why, "higher of source and target") {
		t.Fatalf("external why must not attribute volatility to the source: %s", why)
	}
}
