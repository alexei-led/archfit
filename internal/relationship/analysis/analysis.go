// Package analysis owns relationship classification. It accepts acquired graph
// evidence and relationship policy, and returns relationship facts plus the
// report-only inputs needed by the assessment stage. It does not know findings,
// rules, metrics, or report schemas.
package analysis

import (
	"path/filepath"
	"sort"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
)

// Input is the relationship stage boundary. Graph and runtime signals are
// acquired facts; policy is the sole source of classification declarations.
type Input struct {
	Graph             *graph.Graph
	Policy            policy.RelationshipPolicy
	RuntimeSites      []evidence.RuntimeAsyncSite
	RuntimeConfidence string
	// DynamicImportSites are acquired dynamic/lazy import sites. Analyze rolls
	// them up against the same augmented module map it classified with, so no
	// downstream consumer has to rebuild that map to key them.
	DynamicImportSites []evidence.DynamicImportSite
}

// Analyze classifies acquired graph evidence exactly once.
func Analyze(in Input) relationship.AnalysisResult {
	if in.Graph == nil {
		// A run that acquired nothing classifies nothing. Abstain rather than
		// fabricate: an empty graph and a missing one are the same conclusion.
		in.Graph = graph.Build(nil)
	}
	cfg := augmentConfig(in.Graph, classify.ConfigFrom(in.Policy))

	idx := classify.Run(in.Graph, cfg)
	set := buildSet(in.Graph, idx, cfg.ModuleMap, cfg.Modules)
	runtimeAsyncEdges := runtimeEdges(in.RuntimeSites, in.RuntimeConfidence, cfg.ModuleMap)
	// Dynamic/lazy imports are invisible to the static graph, so they hide cycles
	// and undercount coupling. Rolled up here as report-only evidence: never read
	// by a verdict, a gate, or a metric.
	dynamicImports := buildDynamicImports(in.DynamicImportSites, cfg.ModuleMap)
	connascence := buildConnascenceSummary(set)
	var unmeasuredConnascence []string
	if connascence != nil {
		unmeasuredConnascence = connascence.Unmeasured
	}
	dynamicConnascence := buildDynamicConnascenceSignals(dynamicImports, runtimeAsyncEdges, unmeasuredConnascence)
	classifiedEdges := buildClassifiedSummary(set, classify.BuildContainment(in.Policy.Topology.Modules), cfg.Modules)
	distanceCandidates := append(buildStaticDistanceCandidates(in.Graph, idx, cfg.ModuleMap),
		buildDistanceConfigCandidates(dynamicImports, runtimeAsyncEdges, dynamicConnascence)...)
	sortDistanceConfigCandidates(distanceCandidates)
	out := relationship.AnalysisResult{
		Relationships: set,
		Assessment: relationship.AssessmentSignals{
			AdvisoryCandidates: advisoryCandidates(set, cfg),
			ClassifiedEdges:    classifiedEdges,
			Seams: buildSeams(seamInput{Set: set, Config: cfg, DeclaredModules: in.Policy.Topology.Modules, Tree: classify.BuildContainment(in.Policy.Topology.Modules),
				Graph: in.Graph}),
		},
		Evidence: relationship.AnalysisEvidence{
			RuntimeModules:            runtimeModules(in.RuntimeSites, in.RuntimeConfidence, cfg.ModuleMap),
			RuntimeEdges:              runtimeAsyncEdges,
			DynamicImports:            dynamicImports,
			DynamicConnascenceSignals: dynamicConnascence,
			Connascence:               connascence,
			DistanceConfigCandidates:  distanceCandidates,
			LocalCoupling:             buildLocalCouplingSummary(set),
			VolatilityProvenance:      classify.ComputeVolatilityProvenance(in.Graph, in.Policy.Topology.Modules, cfg),
		},
	}
	return out
}

func buildSet(g *graph.Graph, idx coupling.Index, mm policy.ModuleMap, modules map[string]policy.ModuleDef) relationship.Set {
	if g == nil {
		return relationship.Set{}
	}
	set := relationship.Set{Nodes: make([]relationship.Node, 0, len(g.Nodes())), Edges: make([]relationship.Edge, 0, len(g.Edges()))}
	for _, n := range g.Nodes() {
		id := n.ID()
		firstParty := n.Kind != graph.NodeKindExternal
		module := ""
		if firstParty {
			// An empty result is still an explicit outside-map attribution. The
			// separate BoundaryClassified bit prevents consumers from confusing
			// that result with a node this projection never examined.
			module, _ = moduleForNode(id, mm)
		}
		set.Nodes = append(set.Nodes, relationship.Node{
			ID: id, Path: n.Path, Kind: string(n.Kind), Language: n.Language,
			Module: module, FirstParty: firstParty, BoundaryClassified: firstParty,
		})
	}
	for _, e := range g.Edges() {
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := idx[key]
		if !ok {
			continue
		}
		fp, tp := relationship.NodePath(e.From), relationship.NodePath(e.To)
		fm, _ := moduleForNode(e.From, mm)
		tm, _ := moduleForNode(e.To, mm)
		fromDef, fromClassified := modules[fm]
		toDef, toClassified := modules[tm]
		set.Edges = append(set.Edges, relationship.Edge{FromID: e.From, ToID: e.To, FromPath: fp, ToPath: tp, FromModule: fm, ToModule: tm, FromLayer: fromDef.Layer, ToLayer: toDef.Layer, StructureClassified: fromClassified && toClassified, Kind: string(e.Kind), Language: e.Language, Strength: cl.Strength, Distance: cl.Distance, Volatility: cl.Volatility, Severity: cl.Severity, Locations: locations(e.Locations), Provenance: relationship.Provenance{ClassificationKey: key, DistanceBasis: string(cl.DistanceBasis), ConnascenceKinds: connascenceKinds(cl.Connascence)}, Classified: classification(cl)})
	}
	return set
}
func classification(in coupling.Classification) relationship.Classification {
	return relationship.Classification{Explicitness: relationship.Explicitness(in.Explicitness), ContractRecommended: in.ContractRecommended, Score: relationship.Score{Scored: in.Score.Scored, Balance: in.Score.Balance, Value: in.Score.Value, Band: in.Score.Band, Reason: in.Score.Reason, Breakdown: relationship.ScoreBreakdown{StrengthValue: in.Score.Breakdown.StrengthVal, DistanceValue: in.Score.Breakdown.DistanceVal, VolatilityValue: in.Score.Breakdown.VolatilityVal, Modularity: in.Score.Breakdown.Modularity, VolDiscount: in.Score.Breakdown.VolDiscount}}, DistanceBasis: string(in.DistanceBasis), Connascence: connascence(in.Connascence)}
}

func connascence(in []coupling.ConnascenceEvidence) []relationship.ConnascenceEvidence {
	out := make([]relationship.ConnascenceEvidence, 0, len(in))
	for _, v := range in {
		out = append(out, relationship.ConnascenceEvidence{Kind: string(v.Kind), Source: v.Source, Detail: v.Detail})
	}
	return out
}

func connascenceKinds(in []coupling.ConnascenceEvidence) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v.Kind))
	}
	return out
}

func locations(in []graph.Location) []relationship.Location {
	out := make([]relationship.Location, 0, len(in))
	for _, l := range in {
		out = append(out, relationship.Location{File: l.File, Line: l.Line})
	}
	return out
}

func runtimeModules(sites []evidence.RuntimeAsyncSite, confidence string, mm policy.ModuleMap) []evidence.RuntimeAsyncModule {
	byModule := map[string][]evidence.RuntimeAsyncSite{}
	for _, s := range sites {
		m, ok := mm.ModuleForFile(s.File)
		if !ok || m == "" {
			m = filepath.Dir(s.File)
		}
		byModule[m] = append(byModule[m], s)
	}
	mods := make([]string, 0, len(byModule))
	for m := range byModule {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	out := make([]evidence.RuntimeAsyncModule, 0, len(mods))
	for _, m := range mods {
		ss := byModule[m]
		out = append(out, evidence.RuntimeAsyncModule{Module: m, IntegrationKind: dominantKind(ss), Count: len(ss), Confidence: confidence})
	}
	return out
}
func dominantKind(sites []evidence.RuntimeAsyncSite) string {
	counts := map[string]int{}
	best := ""
	n := 0
	for _, s := range sites {
		counts[s.IntegrationKind]++
		if counts[s.IntegrationKind] > n || (counts[s.IntegrationKind] == n && s.IntegrationKind < best) {
			best = s.IntegrationKind
			n = counts[s.IntegrationKind]
		}
	}
	return best
}
func runtimeEdges(sites []evidence.RuntimeAsyncSite, confidence string, mm policy.ModuleMap) []evidence.RuntimeAsyncEdge {
	type key struct{ from, target, kind string }
	grouped := map[key][]evidence.RuntimeAsyncSite{}
	for _, s := range sites {
		m, ok := mm.ModuleForFile(s.File)
		if !ok || m == "" {
			m = filepath.Dir(s.File)
		}
		target := s.Library
		if target == "" {
			target = s.IntegrationKind
		}
		k := key{m, target, s.IntegrationKind}
		grouped[k] = append(grouped[k], s)
	}
	keys := make([]key, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		if keys[i].target != keys[j].target {
			return keys[i].target < keys[j].target
		}
		return keys[i].kind < keys[j].kind
	})
	out := make([]evidence.RuntimeAsyncEdge, 0, len(keys))
	for _, k := range keys {
		ss := grouped[k]
		sample := ss
		if len(sample) > 5 {
			sample = sample[:5]
		}
		sites := make([]evidence.RuntimeAsyncSite, 0, len(sample))
		sites = append(sites, sample...)
		out = append(out, evidence.RuntimeAsyncEdge{FromModule: k.from, Target: k.target, IntegrationKind: k.kind, Count: len(ss), Confidence: confidence, Sites: sites})
	}
	return out
}
