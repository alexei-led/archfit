// Package analysis owns relationship classification. It accepts acquired graph
// evidence and relationship policy, and returns relationship facts plus the
// report-only inputs needed by the assessment stage. It does not know findings,
// rules, metrics, or report schemas.
package analysis

import (
	"path/filepath"
	"sort"

	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/syntax"
)

// Mode contains only relationship-relevant run posture.
type Mode struct {
	Base string
	Full bool
}

// Input is the relationship stage boundary. Graph and runtime signals are
// acquired facts; policy is the sole source of classification declarations.
type Input struct {
	Graph             *graph.Graph
	Policy            policy.RelationshipPolicy
	Mode              Mode
	Labels            []labels.Label
	CloneClusters     []clone.Cluster
	FileClassIndex    map[string]fileclass.FileClass
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

	evidenceHashes := pairEvidence(in.Graph, cfg.ModuleMap, in.Labels, in.Mode)
	approved, llm, stale := labels.Approved(in.Labels, evidenceHashes)
	cfg.ApprovedLabels = approved
	cfg.LLMLabels = llm
	cfg.LLMLabelConfidence = labels.LLMConfidenceByKey(in.Labels, evidenceHashes)

	if len(in.CloneClusters) > 0 {
		cfg.CrossModuleClonePairs, cfg.CloneEvidence = clonePairs(in.CloneClusters, cfg.ModuleMap, in.FileClassIndex)
	}
	idx := classify.Run(in.Graph, cfg)
	set := buildSet(in.Graph, idx, cfg.ModuleMap, cfg.Modules)
	clones := cloneOnlyPairs(in.Graph, cfg)
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
	classifiedEdges := buildClassifiedSummary(set, clones, cfg.DuplicatedKnowledgePolicy)
	distanceCandidates := append(buildStaticDistanceCandidates(in.Graph, idx, cfg.ModuleMap),
		BuildDistanceConfigCandidates(dynamicImports, runtimeAsyncEdges, dynamicConnascence)...)
	sortDistanceConfigCandidates(distanceCandidates)
	out := relationship.AnalysisResult{
		Relationships: set,
		Assessment: relationship.AssessmentSignals{
			AdvisoryCandidates: advisoryCandidates(set, clones, cfg),
			ClassifiedEdges:    classifiedEdges,
			Seams: buildSeams(seamInput{Set: set, Config: cfg, DeclaredModules: in.Policy.Topology.Modules,
				Graph: in.Graph, EvidenceHashes: evidenceHashes,
				LabelEvidenceHashes: labels.EvidenceHashByKey(in.Labels, evidenceHashes)}),
		},
		Evidence: relationship.AnalysisEvidence{
			LLMApprovedCount:          labels.LLMApprovedCount(in.Labels, evidenceHashes),
			RuntimeModules:            runtimeModules(in.RuntimeSites, in.RuntimeConfidence, cfg.ModuleMap),
			RuntimeEdges:              runtimeAsyncEdges,
			DynamicImports:            dynamicImports,
			DynamicConnascenceSignals: dynamicConnascence,
			CloneOnly:                 clones,
			Connascence:               connascence,
			DistanceConfigCandidates:  distanceCandidates,
			LocalCoupling:             buildLocalCouplingSummary(set),
			VolatilityProvenance:      classify.ComputeVolatilityProvenance(in.Graph, in.Policy.Topology.Modules, cfg),
		},
	}
	for _, l := range stale {
		out.Assessment.StaleLabelKeys = append(out.Assessment.StaleLabelKeys, labels.Key(l.From, l.To))
	}
	return out
}

func cloneOnlyPairs(g *graph.Graph, cfg classify.Config) []relationship.CloneOnlyPair {
	pairs := classify.CloneOnlyPairs(g, cfg)
	out := make([]relationship.CloneOnlyPair, 0, len(pairs))
	for _, p := range pairs {
		locs := locations(p.Locations)
		out = append(out, relationship.CloneOnlyPair{FromModule: p.FromModule, ToModule: p.ToModule, FromPath: p.FromPath, ToPath: p.ToPath, Strength: p.Classification.Strength, Distance: p.Classification.Distance, Volatility: p.Classification.Volatility, Severity: p.Classification.Severity, Locations: locs, Classified: classification(p.Classification)})
	}
	return out
}

func pairEvidence(g *graph.Graph, mm policy.ModuleMap, lbls []labels.Label, mode Mode) map[string]string {
	if (!mode.Full && mode.Base != "") || len(lbls) == 0 || g == nil {
		return nil
	}
	wanted := make(map[string]struct{}, len(lbls))
	for _, l := range lbls {
		wanted[labels.Key(l.From, l.To)] = struct{}{}
	}
	items := map[string][]string{}
	for _, e := range g.Edges() {
		from, to := relationship.NodePath(e.From), relationship.NodePath(e.To)
		fm, okf := mm.ModuleFor(from)
		tm, okt := mm.ModuleFor(to)
		if !okf || !okt || fm == tm {
			continue
		}
		key := labels.Key(fm, tm)
		if _, ok := wanted[key]; !ok {
			continue
		}
		items[key] = append(items[key], from+"\x00"+to+"\x00"+string(e.Kind))
	}
	out := make(map[string]string, len(items))
	for k, v := range items {
		out[k] = labels.HashItems(v)
	}
	return out
}

func clonePairs(clusters []clone.Cluster, mm policy.ModuleMap, index map[string]fileclass.FileClass) (map[string]struct{}, map[string][]graph.Location) {
	filtered := make([]clone.Cluster, 0, len(clusters))
	cfg := syntax.FileClassConfig{}
	for _, c := range clusters {
		bad := false
		for _, f := range c.Files {
			if !fileclass.IsProduction(syntax.LookupFileClass(filepath.ToSlash(f), index, cloneLanguage(f), cfg)) {
				bad = true
				break
			}
		}
		if !bad {
			filtered = append(filtered, c)
		}
	}
	moduleFor := func(f string) string {
		m, ok := mm.ModuleForFile(f)
		if ok {
			return m
		}
		return ""
	}
	pairs := clone.ModulePairs(filtered, moduleFor)
	set := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		set[p[0]+"\x00"+p[1]] = struct{}{}
	}
	return set, cloneEvidence(clusters, mm, index)
}

func cloneEvidence(clusters []clone.Cluster, mm policy.ModuleMap, index map[string]fileclass.FileClass) map[string][]graph.Location {
	out := map[string][]graph.Location{}
	cfg := syntax.FileClassConfig{}
	moduleFor := func(f string) string {
		m, ok := mm.ModuleForFile(f)
		if ok {
			return m
		}
		return ""
	}
	for _, c := range clusters {
		bad := false
		for _, f := range c.Files {
			if !fileclass.IsProduction(syntax.LookupFileClass(filepath.ToSlash(f), index, cloneLanguage(f), cfg)) {
				bad = true
				break
			}
		}
		if bad {
			continue
		}
		for i, f := range c.Files {
			a := moduleFor(f)
			if a == "" {
				continue
			}
			for j := i + 1; j < len(c.Files); j++ {
				b := moduleFor(c.Files[j])
				if b == "" || a == b {
					continue
				}
				x, y := a, b
				if x > y {
					x, y = y, x
				}
				k := x + "\x00" + y
				out[k] = appendUniqueLocation(out[k], f, cloneStartLine(c, i))
				out[k] = appendUniqueLocation(out[k], c.Files[j], cloneStartLine(c, j))
			}
		}
	}
	for k, locs := range out {
		sort.Slice(locs, func(i, j int) bool {
			if locs[i].File != locs[j].File {
				return locs[i].File < locs[j].File
			}
			return locs[i].Line < locs[j].Line
		})
		out[k] = locs
	}
	return out
}

// cloneStartLine returns the start line jscpd reported for Files[i] in c, or 0
// when the cluster carries no per-file location data.
func cloneStartLine(c clone.Cluster, i int) int {
	if i < len(c.Locations) {
		return c.Locations[i].StartLine
	}
	return 0
}

// appendUniqueLocation appends loc to locs unless an identical entry is already
// present. One cluster can pair the same two modules through several file
// combinations, so without this the same file:line ships repeatedly in a
// bc/duplicated_knowledge finding's locations[] and inflates the clone counts.
func appendUniqueLocation(locs []graph.Location, file string, line int) []graph.Location {
	loc := graph.Location{File: file, Line: line}
	for _, l := range locs {
		if l == loc {
			return locs
		}
	}
	return append(locs, loc)
}

// cloneLanguage maps a file extension to the language tag LookupFileClass expects
// for built-in test/generated detection. The TS/JS family all routes through the
// TypeScript classifier because IsTestFile handles .test./.spec. and __tests__
// there for both TS and JS extensions. An extension outside the supported set
// abstains with "": defaulting it to TypeScript would apply TS test-path
// heuristics to unrelated languages and silently drop those clusters.
func cloneLanguage(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return graph.LangGo
	case ".py":
		return graph.LangPython
	case ".rs":
		return graph.LangRust
	}
	for _, tsExt := range cloneTypeScriptSourceExts {
		if ext == tsExt {
			return graph.LangTypeScript
		}
	}
	return ""
}

var cloneTypeScriptSourceExts = graph.TypeScriptSourceExtensions()

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
		set.Edges = append(set.Edges, relationship.Edge{FromID: e.From, ToID: e.To, FromPath: fp, ToPath: tp, FromModule: fm, ToModule: tm, FromLayer: fromDef.Layer, ToLayer: toDef.Layer, StructureClassified: fromClassified && toClassified, Kind: string(e.Kind), Language: e.Language, Strength: cl.Strength, Distance: cl.Distance, Volatility: cl.Volatility, Severity: cl.Severity, Locations: locations(e.Locations), Provenance: relationship.Provenance{ClassificationKey: key, DistanceBasis: string(cl.DistanceBasis), StrengthFromLLM: cl.StrengthFromLLM, StrengthFromNonHighLLM: cl.StrengthFromNonHighLLM, StrengthFromConnascence: cl.StrengthFromConnascence, ConnascenceKinds: connascenceKinds(cl.Connascence), CloneLocationCount: len(cl.CloneLocations)}, Classified: classification(cl)})
	}
	return set
}
func classification(in coupling.Classification) relationship.Classification {
	return relationship.Classification{Explicitness: relationship.Explicitness(in.Explicitness), ContractRecommended: in.ContractRecommended, Score: relationship.Score{Scored: in.Score.Scored, Balance: in.Score.Balance, Value: in.Score.Value, Band: in.Score.Band, Reason: in.Score.Reason, CheapestMove: in.Score.CheapestMove, Breakdown: relationship.ScoreBreakdown{StrengthValue: in.Score.Breakdown.StrengthVal, DistanceValue: in.Score.Breakdown.DistanceVal, VolatilityValue: in.Score.Breakdown.VolatilityVal, Modularity: in.Score.Breakdown.Modularity, VolDiscount: in.Score.Breakdown.VolDiscount}}, DistanceBasis: string(in.DistanceBasis), CloneLocations: locationsFromCoupling(in.CloneLocations), Connascence: connascence(in.Connascence)}
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

func locationsFromCoupling(in []coupling.Location) []relationship.Location {
	out := make([]relationship.Location, 0, len(in))
	for _, v := range in {
		out = append(out, relationship.Location{File: v.File, Line: v.Line})
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
