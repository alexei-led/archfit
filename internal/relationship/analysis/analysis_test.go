// Behavior tests for the relationship analysis seam. They pin what Analyze
// classifies — module resolution, label precedence and staleness, abstention,
// clone-only pairs, runtime rollups, and advisory candidacy — so the capability
// migration can move ownership without moving semantics.
package analysis_test

import (
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

const (
	fileA       = "a/a.go"
	fileB       = "b/b.go"
	nodeA       = "file:" + fileA
	nodeB       = "file:" + fileB
	moduleA     = "a"
	moduleB     = "b"
	teamA       = "team-a"
	teamB       = "team-b"
	ruleBC      = "bc/imbalanced_coupling"
	ruleClone   = "bc/duplicated_knowledge"
	kindImports = "imports"
	volHigh     = "high"
	libNATS     = "nats"
	kindQueue   = "queue"
)

// twoModules is the standard fixture topology: module a owned by team-a,
// module b owned by team-b, so an a→b edge is a cross-owner relationship.
func twoModules() map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{
		moduleA: {Paths: []string{"a/**"}, Owner: teamA, DeployUnit: moduleA, Subdomain: "core", Volatility: volHigh},
		moduleB: {Paths: []string{"b/**"}, Owner: teamB, DeployUnit: moduleB, Subdomain: "supporting", Volatility: volHigh},
	}
}

func relationshipPolicy(modules map[string]policy.ModuleDef) policy.RelationshipPolicy {
	return policy.RelationshipPolicy{Topology: policy.TopologyView{
		Modules: modules, ModuleMap: policy.BuildModuleMap(modules),
		ExplicitOwners: map[string]bool{moduleA: true, moduleB: true},
	}}
}

// graphWith builds a two-file graph carrying one a→b import edge with the given
// extractor strength hint.
func graphWith(hint string) *graph.Graph {
	return graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileB, Language: graph.LangGo},
		},
		Edges: []graph.Edge{{
			From: nodeA, To: nodeB, Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: hint, Locations: []graph.Location{{File: fileA, Line: 7}},
		}},
	}})
}

func onlyEdge(t *testing.T, res relationship.AnalysisResult) relationship.Edge {
	t.Helper()
	if len(res.Relationships.Edges) != 1 {
		t.Fatalf("relationship edges = %d, want 1", len(res.Relationships.Edges))
	}
	return res.Relationships.Edges[0]
}

func TestAnalyzeResolvesModulesAndDistance(t *testing.T) {
	got := analysis.Analyze(analysis.Input{Graph: graphWith(""), Policy: relationshipPolicy(twoModules())})

	edge := onlyEdge(t, got)
	if edge.FromModule != moduleA || edge.ToModule != moduleB {
		t.Fatalf("edge modules = %s -> %s, want a -> b", edge.FromModule, edge.ToModule)
	}
	if edge.FromPath != fileA || edge.ToPath != fileB {
		t.Errorf("edge paths = %s -> %s, want %s -> %s", edge.FromPath, edge.ToPath, fileA, fileB)
	}
	if edge.Distance != relationship.DistanceCrossDeployUnit {
		t.Errorf("distance = %q, want cross_deploy_unit for differing deploy units", edge.Distance)
	}
	if len(edge.Locations) != 1 || edge.Locations[0].File != fileA || edge.Locations[0].Line != 7 {
		t.Errorf("locations = %+v, want the extractor location carried through", edge.Locations)
	}
	if edge.Provenance.ClassificationKey == "" {
		t.Error("provenance classification key = empty, want the classifier key preserved")
	}
	if len(got.Relationships.Nodes) != 2 {
		t.Errorf("nodes = %d, want both graph nodes projected", len(got.Relationships.Nodes))
	}
	for _, n := range got.Relationships.Nodes {
		if !n.FirstParty {
			t.Errorf("node %s FirstParty = false, want true for a file node", n.ID)
		}
	}
}

func TestAnalyzeStrengthFromExtractorHint(t *testing.T) {
	tests := []struct {
		name string
		hint string
		want relationship.Strength
	}{
		{name: "no hint abstains", hint: "", want: relationship.StrengthUnknown},
		{name: "contract hint", hint: string(relationship.StrengthContract), want: relationship.StrengthContract},
		{name: "intrusive hint", hint: string(relationship.StrengthIntrusive), want: relationship.StrengthIntrusive},
		{name: "functional hint", hint: string(relationship.StrengthFunctional), want: relationship.StrengthFunctional},
		{name: "model hint", hint: string(relationship.StrengthModel), want: relationship.StrengthModel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			edge := onlyEdge(t, analysis.Analyze(analysis.Input{
				Graph: graphWith(test.hint), Policy: relationshipPolicy(twoModules()),
			}))
			if edge.Strength != test.want {
				t.Fatalf("strength = %q, want %q", edge.Strength, test.want)
			}
		})
	}
}

// An unknown strength must abstain from scoring rather than invent an ordinal.
func TestAnalyzeAbstainsWhenStrengthIsUnknown(t *testing.T) {
	unknown := onlyEdge(t, analysis.Analyze(analysis.Input{Graph: graphWith(""), Policy: relationshipPolicy(twoModules())}))
	if unknown.Strength != relationship.StrengthUnknown {
		t.Fatalf("strength = %q, want unknown", unknown.Strength)
	}
	if unknown.Classified.Score.Scored {
		t.Fatal("Score.Scored = true for an unknown-strength edge, want abstention")
	}
	if unknown.Severity != relationship.SeverityNone {
		t.Fatalf("severity = %q, want none for an unscored edge", unknown.Severity)
	}

	known := onlyEdge(t, analysis.Analyze(analysis.Input{
		Graph: graphWith(string(relationship.StrengthFunctional)), Policy: relationshipPolicy(twoModules()),
	}))
	if !known.Classified.Score.Scored {
		t.Fatal("Score.Scored = false for a classified edge, want a score")
	}
}

// An approved human label is the reviewer's verdict and outranks the
// extractor's hint; a draft label and a stale label do not.
func TestAnalyzeLabelPrecedence(t *testing.T) {
	hash := labels.HashItems([]string{fileA + "\x00" + fileB + "\x00" + kindImports})
	label := func(status, provenance, hashValue string) labels.Label {
		return labels.Label{
			From: moduleA, To: moduleB, Strength: string(relationship.StrengthContract),
			Status: status, Provenance: provenance, EvidenceHash: hashValue, Confidence: labels.ConfidenceHigh,
		}
	}
	tests := []struct {
		name         string
		label        labels.Label
		wantStrength relationship.Strength
		wantStale    bool
	}{
		{
			name:         "approved human label with matching evidence wins",
			label:        label(labels.StatusApproved, labels.ProvenanceHuman, hash),
			wantStrength: relationship.StrengthContract,
		},
		{
			name:         "approved label with empty evidence hash applies",
			label:        label(labels.StatusApproved, labels.ProvenanceHuman, ""),
			wantStrength: relationship.StrengthContract,
		},
		{
			name:         "approved label with stale evidence hash is ignored and reported",
			label:        label(labels.StatusApproved, labels.ProvenanceHuman, "0000stale0000"),
			wantStrength: relationship.StrengthFunctional,
			wantStale:    true,
		},
		{
			name:         "draft label never applies and is not stale",
			label:        label("draft", labels.ProvenanceHuman, hash),
			wantStrength: relationship.StrengthFunctional,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analysis.Analyze(analysis.Input{
				Graph:  graphWith(string(relationship.StrengthFunctional)),
				Policy: relationshipPolicy(twoModules()),
				Labels: []labels.Label{test.label},
				Mode:   analysis.Mode{Full: true},
			})
			if s := onlyEdge(t, got).Strength; s != test.wantStrength {
				t.Errorf("strength = %q, want %q", s, test.wantStrength)
			}
			wantKeys := []string(nil)
			if test.wantStale {
				wantKeys = []string{labels.Key(moduleA, moduleB)}
			}
			if !slices.Equal(got.StaleLabelKeys, wantKeys) {
				t.Errorf("StaleLabelKeys = %v, want %v", got.StaleLabelKeys, wantKeys)
			}
		})
	}
}

// A delta run sees a partial graph, so evidence hashes are not computed and
// staleness is never asserted against incomplete evidence.
func TestAnalyzeSkipsEvidenceHashingOnDeltaRuns(t *testing.T) {
	stale := labels.Label{
		From: moduleA, To: moduleB, Strength: string(relationship.StrengthContract),
		Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman, EvidenceHash: "0000stale0000",
	}
	tests := []struct {
		name          string
		mode          analysis.Mode
		wantStrength  relationship.Strength
		wantStaleKeys int
	}{
		{
			name: "delta run applies the label without a freshness check",
			mode: analysis.Mode{Base: "main"}, wantStrength: relationship.StrengthContract, wantStaleKeys: 0,
		},
		{
			name: "full run against a base ref still checks freshness",
			mode: analysis.Mode{Base: "main", Full: true}, wantStrength: relationship.StrengthFunctional, wantStaleKeys: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analysis.Analyze(analysis.Input{
				Graph:  graphWith(string(relationship.StrengthFunctional)),
				Policy: relationshipPolicy(twoModules()), Labels: []labels.Label{stale}, Mode: test.mode,
			})
			if s := onlyEdge(t, got).Strength; s != test.wantStrength {
				t.Errorf("strength = %q, want %q", s, test.wantStrength)
			}
			if len(got.StaleLabelKeys) != test.wantStaleKeys {
				t.Errorf("StaleLabelKeys = %v, want %d entries", got.StaleLabelKeys, test.wantStaleKeys)
			}
		})
	}
}

// An LLM label below high confidence is counted so downstream scoring can lower
// the coupling_balance confidence band.
func TestAnalyzeCountsNonHighConfidenceLLMLabels(t *testing.T) {
	tests := []struct {
		name       string
		provenance string
		confidence string
		want       int
	}{
		{name: "low-confidence llm label counts", provenance: labels.ProvenanceLLM, confidence: labels.ConfidenceLow, want: 1},
		{name: "high-confidence llm label does not count", provenance: labels.ProvenanceLLM, confidence: labels.ConfidenceHigh, want: 0},
		{name: "human label never counts", provenance: labels.ProvenanceHuman, confidence: labels.ConfidenceLow, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analysis.Analyze(analysis.Input{
				Graph: graphWith(""), Policy: relationshipPolicy(twoModules()), Mode: analysis.Mode{Full: true},
				Labels: []labels.Label{{
					From: moduleA, To: moduleB, Strength: string(relationship.StrengthContract),
					Status: labels.StatusApproved, Provenance: test.provenance, Confidence: test.confidence,
				}},
			})
			if got.LLMApprovedCount != test.want {
				t.Fatalf("LLMApprovedCount = %d, want %d", got.LLMApprovedCount, test.want)
			}
		})
	}
}

func TestAnalyzeEmitsAdvisoryCandidatesForSevereEdges(t *testing.T) {
	tests := []struct {
		name         string
		hint         relationship.Strength
		minSeverity  string
		wantSeverity relationship.Severity
		wantRules    []string
	}{
		{
			name: "no threshold emits the candidate", hint: relationship.StrengthIntrusive,
			wantSeverity: relationship.SeverityCritical, wantRules: []string{ruleBC},
		},
		{
			name: "threshold at severity emits the candidate", hint: relationship.StrengthIntrusive,
			minSeverity: "critical", wantSeverity: relationship.SeverityCritical, wantRules: []string{ruleBC},
		},
		{
			name: "threshold above severity filters it out", hint: relationship.StrengthModel,
			minSeverity: "medium", wantSeverity: relationship.SeverityLow, wantRules: nil,
		},
		{
			name: "threshold at a low severity keeps it", hint: relationship.StrengthModel,
			minSeverity: "low", wantSeverity: relationship.SeverityLow, wantRules: []string{ruleBC},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pol := relationshipPolicy(twoModules())
			pol.MinimumSeverity = test.minSeverity
			got := analysis.Analyze(analysis.Input{Graph: graphWith(string(test.hint)), Policy: pol})
			if sev := onlyEdge(t, got).Severity; sev != test.wantSeverity {
				t.Fatalf("edge severity = %q, want %q", sev, test.wantSeverity)
			}
			rules := make([]string, 0, len(got.AdvisoryCandidates))
			for _, c := range got.AdvisoryCandidates {
				rules = append(rules, c.RuleID)
				if c.From != fileA || c.To != fileB || c.FromModule != moduleA || c.ToModule != moduleB {
					t.Errorf("candidate endpoints = %+v, want the classified edge endpoints", c)
				}
				if c.Why == "" || c.MatchedBy["strength"] == "" {
					t.Errorf("candidate = %+v, want a rationale and matched-by evidence", c)
				}
			}
			if !slices.Equal(rules, test.wantRules) {
				t.Fatalf("advisory rules = %v, want %v", rules, test.wantRules)
			}
		})
	}
}

// Cross-module clones with no import edge are duplicated knowledge; test files
// are excluded because example duplication is not an architecture signal.
func TestAnalyzeCloneOnlyPairs(t *testing.T) {
	const (
		cloneA     = "a/dup.go"
		cloneB     = "b/dup.go"
		cloneBTest = "b/dup_test.go"
	)
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: cloneA, Language: graph.LangGo},
		{Kind: graph.NodeKindFile, Path: cloneB, Language: graph.LangGo},
	}
	g := graph.Build([]graph.Facts{{Nodes: nodes}})

	tests := []struct {
		name      string
		cluster   clone.Cluster
		index     map[string]fileclass.FileClass
		wantPairs int
	}{
		{
			name:      "production clone across modules is a pair",
			cluster:   clone.Cluster{Files: []string{cloneA, cloneB}, Lines: 40, Locations: []clone.LineRange{{StartLine: 1}, {StartLine: 2}}},
			index:     map[string]fileclass.FileClass{cloneA: fileclass.Production, cloneB: fileclass.Production},
			wantPairs: 1,
		},
		{
			name:      "clone touching a test file is excluded",
			cluster:   clone.Cluster{Files: []string{cloneA, cloneBTest}, Lines: 40},
			index:     map[string]fileclass.FileClass{cloneA: fileclass.Production, cloneBTest: fileclass.Test},
			wantPairs: 0,
		},
		{
			name:      "clone inside one module is not cross-module",
			cluster:   clone.Cluster{Files: []string{cloneA, "a/dup2.go"}, Lines: 40},
			index:     map[string]fileclass.FileClass{cloneA: fileclass.Production, "a/dup2.go": fileclass.Production},
			wantPairs: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analysis.Analyze(analysis.Input{
				Graph: g, Policy: relationshipPolicy(twoModules()),
				CloneClusters: []clone.Cluster{test.cluster}, FileClassIndex: test.index,
			})
			if len(got.CloneOnly) != test.wantPairs {
				t.Fatalf("clone-only pairs = %+v, want %d", got.CloneOnly, test.wantPairs)
			}
			if test.wantPairs == 0 {
				return
			}
			pair := got.CloneOnly[0]
			if pair.Strength != relationship.StrengthSymmetric {
				t.Errorf("clone pair strength = %q, want symmetric", pair.Strength)
			}
			if pair.FromModule != moduleA || pair.ToModule != moduleB {
				t.Errorf("clone pair modules = %s -> %s, want a -> b", pair.FromModule, pair.ToModule)
			}
			for _, c := range got.AdvisoryCandidates {
				if c.RuleID == ruleClone && c.EdgeKind != "clone" {
					t.Errorf("duplicated-knowledge candidate kind = %q, want clone", c.EdgeKind)
				}
			}
		})
	}
}

func TestAnalyzeRuntimeSignalRollup(t *testing.T) {
	const confidence = "medium"
	sites := []evidence.RuntimeAsyncSite{
		{File: fileA, Line: 1, IntegrationKind: kindQueue, Library: libNATS, Language: graph.LangGo},
		{File: fileA, Line: 2, IntegrationKind: kindQueue, Library: libNATS, Language: graph.LangGo},
		{File: fileB, Line: 3, IntegrationKind: "event", Language: graph.LangGo},
	}
	got := analysis.Analyze(analysis.Input{
		Graph: graphWith(""), Policy: relationshipPolicy(twoModules()),
		RuntimeSites: sites, RuntimeConfidence: confidence,
	})

	if len(got.RuntimeSignals) != 2 {
		t.Fatalf("runtime signals = %+v, want one per module", got.RuntimeSignals)
	}
	if got.RuntimeSignals[0].Module != moduleA || got.RuntimeSignals[1].Module != moduleB {
		t.Errorf("runtime signals = %+v, want modules sorted", got.RuntimeSignals)
	}
	if got.RuntimeSignals[0].Count != 2 || got.RuntimeSignals[0].IntegrationKind != kindQueue {
		t.Errorf("module a signal = %+v, want 2 queue sites", got.RuntimeSignals[0])
	}
	if got.RuntimeSignals[0].Confidence != confidence {
		t.Errorf("confidence = %q, want %q carried through", got.RuntimeSignals[0].Confidence, confidence)
	}
	if len(got.RuntimeRelations) != 2 {
		t.Fatalf("runtime relations = %+v, want one per (module, target, kind)", got.RuntimeRelations)
	}
	if got.RuntimeRelations[0].Target != libNATS {
		t.Errorf("relation target = %q, want the library name", got.RuntimeRelations[0].Target)
	}
	// A site with no library falls back to the integration kind as its target.
	if got.RuntimeRelations[1].Target != "event" {
		t.Errorf("libraryless relation target = %q, want the integration kind", got.RuntimeRelations[1].Target)
	}
	if len(got.RuntimeRelations[0].Sites) != 2 {
		t.Errorf("relation sites = %+v, want the sampled sites", got.RuntimeRelations[0].Sites)
	}
}

// Runtime evidence never annotates a relationship edge — it is report-only.
func TestAnalyzeRuntimeEvidenceDoesNotAnnotateEdges(t *testing.T) {
	base := analysis.Analyze(analysis.Input{Graph: graphWith(string(relationship.StrengthFunctional)), Policy: relationshipPolicy(twoModules())})
	withRuntime := analysis.Analyze(analysis.Input{
		Graph: graphWith(string(relationship.StrengthFunctional)), Policy: relationshipPolicy(twoModules()),
		RuntimeSites:      []evidence.RuntimeAsyncSite{{File: fileA, Line: 1, IntegrationKind: kindQueue, Library: libNATS}},
		RuntimeConfidence: volHigh,
	})
	before, after := onlyEdge(t, base), onlyEdge(t, withRuntime)
	if before.Strength != after.Strength || before.Distance != after.Distance || before.Severity != after.Severity {
		t.Fatalf("runtime evidence changed the classification: %+v -> %+v", before, after)
	}
}

func TestAnalyzeSummariesAreProduced(t *testing.T) {
	got := analysis.Analyze(analysis.Input{
		Graph: graphWith(string(relationship.StrengthIntrusive)), Policy: relationshipPolicy(twoModules()),
	})
	if got.ClassifiedEdges == nil {
		t.Fatal("ClassifiedEdges = nil, want a classified-edge summary")
	}
	if got.ClassifiedEdges.Total != 1 {
		t.Errorf("ClassifiedEdges.Total = %d, want 1", got.ClassifiedEdges.Total)
	}
	if got.VolatilityProvenance == nil {
		t.Fatal("VolatilityProvenance = nil, want the provenance rollup")
	}
	if got.VolatilityProvenance.Declared != 2 {
		t.Errorf("declared volatility modules = %d, want both fixture modules", got.VolatilityProvenance.Declared)
	}
}

// A same-module edge is scored but stays report-only: it never becomes a
// relationship advisory.
func TestAnalyzeSameModuleEdgesAreLocalCouplingOnly(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		moduleA: {Paths: []string{"a/**"}, Owner: teamA, DeployUnit: moduleA, Volatility: volHigh},
	}
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: "a/other.go", Language: graph.LangGo},
		},
		Edges: []graph.Edge{{
			From: nodeA, To: "file:a/other.go", Kind: graph.EdgeKindImports,
			Language: graph.LangGo, StrengthHint: string(relationship.StrengthIntrusive),
		}},
	}})
	got := analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(modules)})

	edge := onlyEdge(t, got)
	if edge.Distance != relationship.DistanceSameModule {
		t.Fatalf("distance = %q, want same_module", edge.Distance)
	}
	if edge.Severity != relationship.SeverityNone {
		t.Errorf("severity = %q, want none: same-module coupling is report-only", edge.Severity)
	}
	if len(got.AdvisoryCandidates) != 0 {
		t.Errorf("advisory candidates = %+v, want none for a same-module edge", got.AdvisoryCandidates)
	}
	if len(got.LocalCoupling) == 0 {
		t.Error("LocalCoupling = empty, want the same-module edge reported locally")
	}
}

// An edge into a path no module declares is external: it must not be scored as
// an internal seam.
func TestAnalyzeUndeclaredTargetIsExternal(t *testing.T) {
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindExternal, Path: "github.com/other/lib", Language: graph.LangGo},
		},
		Edges: []graph.Edge{{
			From: nodeA, To: "external:github.com/other/lib", Kind: graph.EdgeKindImports,
			Language: graph.LangGo, StrengthHint: string(relationship.StrengthFunctional),
		}},
	}})
	got := analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(twoModules())})

	edge := onlyEdge(t, got)
	if edge.ToModule != "" {
		t.Errorf("target module = %q, want empty for an undeclared target", edge.ToModule)
	}
	if edge.Distance != relationship.DistanceUnknown {
		t.Errorf("distance = %q, want unknown for an external target", edge.Distance)
	}
	if edge.Classified.Score.Scored {
		t.Error("Score.Scored = true for an external edge, want it excluded from scoring")
	}
	for _, n := range got.Relationships.Nodes {
		if n.Kind == string(graph.NodeKindExternal) && n.FirstParty {
			t.Errorf("external node %s FirstParty = true, want false", n.ID)
		}
	}
}

// An empty graph produces an empty relationship set and no derived facts. A nil
// graph is not covered: classify.Run dereferences it, and the acquisition stage
// always supplies a built graph.
func TestAnalyzeEmptyGraph(t *testing.T) {
	got := analysis.Analyze(analysis.Input{Graph: graph.Build(nil), Policy: relationshipPolicy(twoModules())})
	if !got.Relationships.Empty() {
		t.Fatalf("relationships = %+v, want an empty set", got.Relationships)
	}
	if len(got.AdvisoryCandidates) != 0 || len(got.CloneOnly) != 0 || len(got.StaleLabelKeys) != 0 {
		t.Fatalf("result = %+v, want no derived facts from an empty graph", got)
	}
}

// An unresolved external target becomes a review candidate for an
// external_systems: declaration, normalized to the language's glob shape.
func TestAnalyzeStaticDistanceCandidates(t *testing.T) {
	tests := []struct {
		name       string
		language   string
		targetNode graph.Node
		goModules  []graph.GoModule
		wantTarget string
	}{
		{
			name:       "go third-party package collapses to the module prefix",
			language:   graph.LangGo,
			targetNode: graph.Node{Kind: graph.NodeKindPackage, Path: "github.com/vendor/lib/sub", Language: graph.LangGo},
			wantTarget: "github.com/vendor/lib/**",
		},
		{
			name:       "go first-party package is not an external candidate",
			language:   graph.LangGo,
			targetNode: graph.Node{Kind: graph.NodeKindPackage, Path: "example.com/self/pkg", Language: graph.LangGo},
			goModules:  []graph.GoModule{{Path: "example.com/self"}},
		},
		{
			name:       "go stdlib package is not an external candidate",
			language:   graph.LangGo,
			targetNode: graph.Node{Kind: graph.NodeKindPackage, Path: "net/http", Language: graph.LangGo},
		},
		{
			name:       "typescript node_modules scope collapses to the package root",
			language:   graph.LangTypeScript,
			targetNode: graph.Node{Kind: graph.NodeKindExternal, Path: "node_modules/@scope/pkg/dist/index.js", Language: graph.LangTypeScript},
			wantTarget: "node_modules/@scope/pkg/**",
		},
		{
			name:       "typescript non-node_modules target is not a candidate",
			language:   graph.LangTypeScript,
			targetNode: graph.Node{Kind: graph.NodeKindExternal, Path: "vendor/pkg", Language: graph.LangTypeScript},
		},
		{
			name:       "python dotted target expands to the top-level package",
			language:   graph.LangPython,
			targetNode: graph.Node{Kind: graph.NodeKindExternal, Path: "requests.adapters", Language: graph.LangPython},
			wantTarget: "{requests,requests.*}",
		},
		{
			name:       "python relative target is not a candidate",
			language:   graph.LangPython,
			targetNode: graph.Node{Kind: graph.NodeKindExternal, Path: ".relative", Language: graph.LangPython},
		},
		{
			name:       "rust external crate is used verbatim",
			language:   graph.LangRust,
			targetNode: graph.Node{Kind: graph.NodeKindExternal, Path: "serde", Language: graph.LangRust},
			wantTarget: "serde",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := graph.Build([]graph.Facts{{
				Language:  test.language,
				GoModules: test.goModules,
				Nodes:     []graph.Node{{Kind: graph.NodeKindFile, Path: fileA, Language: test.language}, test.targetNode},
				Edges: []graph.Edge{{
					From: nodeA, To: test.targetNode.ID(), Kind: graph.EdgeKindImports, Language: test.language,
					Locations: []graph.Location{{File: fileA, Line: 2}},
				}},
			}})
			got := analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(twoModules())})

			if test.wantTarget == "" {
				if len(got.DistanceConfigCandidates) != 0 {
					t.Fatalf("candidates = %+v, want none", got.DistanceConfigCandidates)
				}
				return
			}
			if len(got.DistanceConfigCandidates) != 1 {
				t.Fatalf("candidates = %+v, want 1", got.DistanceConfigCandidates)
			}
			c := got.DistanceConfigCandidates[0]
			if c.Target != test.wantTarget {
				t.Errorf("target = %q, want %q", c.Target, test.wantTarget)
			}
			if c.Module != moduleA || c.Count != 1 || c.SuggestedReviewAction != "external_systems" {
				t.Errorf("candidate = %+v, want module a, count 1, external_systems action", c)
			}
			if len(c.EvidenceSites) != 1 || c.EvidenceSites[0].Target != test.targetNode.Path {
				t.Errorf("evidence sites = %+v, want the raw target recorded", c.EvidenceSites)
			}
		})
	}
}

// Deterministic connascence facts ride the edge as provenance and roll up into
// the report-only connascence summary.
func TestAnalyzeConnascenceEvidence(t *testing.T) {
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: fileB, Language: graph.LangGo},
		},
		Edges: []graph.Edge{{
			From: nodeA, To: nodeB, Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(relationship.StrengthFunctional),
			ConnascenceHints: []graph.ConnascenceHint{
				{Kind: "name", Source: "go/types", Detail: "Handler"},
				{Kind: "type", Source: "go/types"},
			},
		}},
	}})
	got := analysis.Analyze(analysis.Input{Graph: g, Policy: relationshipPolicy(twoModules())})

	edge := onlyEdge(t, got)
	if !slices.Equal(edge.Provenance.ConnascenceKinds, []string{"name", "type"}) {
		t.Errorf("connascence kinds = %v, want [name type]", edge.Provenance.ConnascenceKinds)
	}
	if len(edge.Classified.Connascence) != 2 {
		t.Errorf("classified connascence = %+v, want both facts", edge.Classified.Connascence)
	}
	if got.Connascence == nil {
		t.Fatal("Connascence = nil, want the report-only summary")
	}
	if got.Connascence.EdgesWithEvidence != 1 || got.Connascence.TotalEvidence != 2 {
		t.Errorf("connascence summary = %+v, want 1 edge with 2 facts", got.Connascence)
	}
	if got.Connascence.ByKind["name"] != 1 || got.Connascence.BySource["go/types"] != 2 {
		t.Errorf("connascence rollups = %+v, want per-kind and per-source counts", got.Connascence)
	}
	// Connascence is disclosure, not a distance input.
	plain := onlyEdge(t, analysis.Analyze(analysis.Input{
		Graph: graphWith(string(relationship.StrengthFunctional)), Policy: relationshipPolicy(twoModules()),
	}))
	if plain.Distance != edge.Distance {
		t.Errorf("distance changed with connascence evidence: %q -> %q", plain.Distance, edge.Distance)
	}
}
