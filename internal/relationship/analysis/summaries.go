package analysis

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
	"github.com/alexei-led/archfit/v3/internal/relationship/scoring"
)

func buildClassifiedSummary(set relationship.Set, clones []relationship.ClonePair, duplicated policy.DuplicatedKnowledgePolicy, tree classify.Containment, modules map[string]policy.ModuleDef) *relationship.ClassifiedEdgeSummary {
	s := &relationship.ClassifiedEdgeSummary{ByStrength: map[string]int{}, ByDistance: map[string]int{}, ByDistanceBasis: map[string]int{}, ByVolatility: map[string]int{}, BySeverity: map[string]int{}, ByBalanceDriver: map[string]int{}, ByCriticalDriver: map[string]int{}, ByModulePair: map[string]int{}, DistanceCompression: distanceCompression()}
	firstParty := make(map[string]bool, len(set.Nodes))
	for _, n := range set.Nodes {
		if !n.FirstParty {
			continue
		}
		firstParty[n.ID] = true
		s.FirstPartyNodes++
		if n.BoundaryClassified {
			s.AttributedFirstPartyNodes++
		}
	}
	connected := map[string]struct{}{}
	dependencyModules := map[string]struct{}{}
	tail := tailAccumulator{}
	sum := 0
	span := spanAccumulator{}
	unrated := unratedAccumulator{modules: map[string]struct{}{}}
	for _, e := range set.Edges {
		if e.IsDependency() {
			s.DependencyEdges++
			switch {
			case e.FromModule != "" && e.ToModule != "":
				s.InternalDependencies++
				// buildSet preserves the directed kind plus both module and
				// layer placements. An empty layer on a known module is an
				// observed unlayered placement, not a failed attribution.
				if e.StructureClassified {
					s.ClassifiedInternalDependencies++
				}
				dependencyModules[e.FromModule] = struct{}{}
				dependencyModules[e.ToModule] = struct{}{}
				if e.FromModule == e.ToModule {
					s.SameModuleDependencies++
				}
			case firstParty[e.ToID]:
				// A first-party target, unowned or imported from unowned
				// source: a gap in the module map, unlike a library import.
				s.UnmappedFirstPartyDependencies++
			default:
				// An external node, or a target no first-party node stands for
				// (a Go standard-library or third-party package has no node).
				// Ceiling: a first-party target the extractor emits no node for
				// (a failed Go package load, the Go module root package) lands
				// here too; the sum with the unmapped count stays exact.
				s.LibraryDependencies++
			}
		}
		qualifies := relationship.QualifiesDistributedMonolith(e.Classified.Score.Scored && e.Classified.Score.Balance > 0, e.Strength, e.Distance, e.Volatility, classify.CohesiveRole(modules[e.FromModule].Role))
		sum += addSummary(s, e.Classified, e.Strength, e.Distance, e.Volatility, e.Provenance, qualifies)
		unrated.addEdge(e.Classified, e.Distance, e.Volatility, e.Strength, e.FromModule, e.ToModule, modules)
		addDriver(s, e.Classified, e.Distance, e.FromModule, e.ToModule)
		tail.add(e.Classified, e.Distance, false, qualifies)
		if e.Distance != relationship.DistanceSameModule && e.Distance != relationship.DistanceUnknown {
			connected[e.FromModule] = struct{}{}
			connected[e.ToModule] = struct{}{}
		}
		span.add(tree, e.FromModule, e.ToModule, e.Distance)
	}
	if policy.NormalizeDuplicatedKnowledgePolicy(duplicated) == policy.DuplicatedKnowledgePolicyScore {
		for _, p := range clones {
			if !p.Connected {
				s.CloneOnlyScored++
			}
			sum += addSummary(s, p.Classified, p.Strength, p.Distance, p.Volatility, relationship.Provenance{}, false)
			unrated.addEdge(p.Classified, p.Distance, p.Volatility, relationship.StrengthSymmetric, p.FromModule, p.ToModule, modules)
			addDriver(s, p.Classified, p.Distance, p.FromModule, p.ToModule)
			tail.add(p.Classified, p.Distance, !p.Connected, false)
			connected[p.FromModule] = struct{}{}
			connected[p.ToModule] = struct{}{}
			span.add(tree, p.FromModule, p.ToModule, p.Distance)
		}
	} else {
		for _, p := range clones {
			if !p.Connected {
				s.CloneOnlyAdvisory++
			}
		}
	}
	for k := range connected {
		if k != "" {
			s.ConnectedModules++
		}
	}
	s.DependencyModules = len(dependencyModules)
	if s.Scored > 0 {
		s.MeanBalance = float64(sum) / float64(s.Scored)
		s.TailRisk = tail.result(s.Scored)
	}
	span.apply(s.DistanceCompression)
	s.UnratedVolatilityEdges, s.UnratedVolatilityModules = unrated.edges, unrated.names()
	return s
}
func addSummary(s *relationship.ClassifiedEdgeSummary, c relationship.Classification, strength relationship.Strength, distance relationship.Distance, volatility relationship.Volatility, prov relationship.Provenance, qualifies bool) int {
	s.Total++
	if distance == relationship.DistanceSameModule {
		s.SameModule++
		return 0
	}
	if distance == relationship.DistanceUnknown {
		s.External++
		return 0
	}
	if c.DistanceBasis != "" {
		s.ByDistanceBasis[c.DistanceBasis]++
	}
	if distance == relationship.DistanceExternal {
		s.DeclaredExternal++
	}
	s.ByStrength[string(strength)]++
	s.ByDistance[string(distance)]++
	s.ByVolatility[string(volatility)]++
	// Label provenance is per-edge, so it is counted here — on cross-boundary
	// edges only, after the same-module/unknown early-outs. LabeledLLM feeds the
	// coupling_balance evidence line and LLMLowConfidenceEdges lowers its
	// confidence band by one.
	if prov.StrengthFromLLM {
		s.LabeledLLM++
	}
	if prov.StrengthFromNonHighLLM {
		s.LLMLowConfidenceEdges++
	}
	if c.Score.Scored {
		s.Scored++
		s.BySeverity[string(c.Score.Band)]++
		if qualifies {
			s.DistributedMonolith++
		}
		return c.Score.Balance
	}
	s.Abstained++
	s.BySeverity["abstained"]++
	return 0
}

// addDriver records one scored CROSS-BOUNDARY edge. Same-module edges are
// excluded for the same reason tailAccumulator.add excludes them: all three
// maps are reported against s.Scored, which addSummary increments for
// cross-boundary edges only, so counting the same-module rung here would make
// the driver histograms sum past the scored denominator printed beside them.
func addDriver(s *relationship.ClassifiedEdgeSummary, c relationship.Classification, distance relationship.Distance, from, to string) {
	if !c.Score.Scored || distance == relationship.DistanceSameModule {
		return
	}
	driver := "tie"
	mod := c.Score.Breakdown.Modularity
	vol := 10 - c.Score.Breakdown.VolatilityValue
	if mod > vol {
		driver = "strength_distance"
	} else if vol > mod {
		driver = "volatility"
	}
	s.ByBalanceDriver[driver]++
	if c.Score.Band == relationship.SeverityCritical {
		s.ByCriticalDriver[driver]++
	}
	if from != "" && to != "" {
		s.ByModulePair[from+" -> "+to]++
	}
}

type tailAccumulator struct {
	balances                                                        []int
	high, critical, distributed, cloneScored, cloneHigh, cloneWorst int
}

// add records one scored CROSS-BOUNDARY edge. Same-module and unknown-distance
// edges are excluded: the tail-risk share is reported against s.Scored, which
// counts cross-boundary edges only, so admitting them here would put a
// 505-edge numerator over a 362-edge denominator. Band "none" (balance 9-10) is
// a well-balanced cross-boundary edge and belongs in the balance distribution —
// it is what makes WorstBalance and LowerDecileBalance honest.
func (a *tailAccumulator) add(c relationship.Classification, distance relationship.Distance, clone, qualifies bool) {
	if !c.Score.Scored || distance == relationship.DistanceSameModule || distance == relationship.DistanceUnknown || c.Score.Balance <= 0 {
		return
	}
	a.balances = append(a.balances, c.Score.Balance)
	if clone {
		a.cloneScored++
		if a.cloneWorst == 0 || c.Score.Balance < a.cloneWorst {
			a.cloneWorst = c.Score.Balance
		}
	}
	if c.Score.Band == relationship.SeverityHigh || c.Score.Band == relationship.SeverityCritical {
		a.high++
		if clone {
			a.cloneHigh++
		}
	}
	if c.Score.Band == relationship.SeverityCritical {
		a.critical++
		if qualifies {
			a.distributed++
		}
	}
}
func (a tailAccumulator) result(total int) *relationship.CouplingTailRiskSummary {
	if len(a.balances) == 0 {
		return nil
	}
	sort.Ints(a.balances)
	rank := (len(a.balances) + 9) / 10
	share := 0
	if total > 0 {
		share = a.high * 100 / total
	}
	return &relationship.CouplingTailRiskSummary{WorstBalance: a.balances[0], LowerDecileBalance: a.balances[rank-1], HighOrWorseEdges: a.high, HighOrWorseSharePct: share, CriticalEdges: a.critical, DistributedMonolithEdges: a.distributed, CloneOnlyScored: a.cloneScored, CloneOnlyHighOrWorseEdges: a.cloneHigh, CloneOnlyWorstBalance: a.cloneWorst}
}

func distanceCompression() *relationship.DistanceCompressionSummary {
	d := classify.DistanceCompression()
	out := &relationship.DistanceCompressionSummary{CompressedMiddleRungs: d.CompressedMiddleRungs, ImplementedRungs: append([]int(nil), d.ImplementedRungs...), OmittedRungs: append([]int(nil), d.OmittedRungs...), DeterministicSplits: append([]string(nil), d.DeterministicSplits...), Rationale: d.Rationale}
	for _, r := range d.OmittedRungReasons {
		out.OmittedRungReasons = append(out.OmittedRungReasons, relationship.DistanceOmittedRungReason{Rung: r.Rung, Reason: r.Reason})
	}
	return out
}

// unratedAccumulator finds scored edges whose volatility nobody declared. The
// edge's effective volatility is undeclared only when no end declared high, so
// the modules to name are the ends whose own definition declares nothing.
type unratedAccumulator struct {
	edges   int
	modules map[string]struct{}
}

func (a *unratedAccumulator) addEdge(c relationship.Classification, distance relationship.Distance, v relationship.Volatility, strength relationship.Strength, from, to string, modules map[string]policy.ModuleDef) {
	if !c.Score.Scored || c.Score.Balance <= 0 || !relationship.IsModuleBoundary(distance) {
		return
	}
	if v != relationship.VolatilityUndeclared && v != relationship.VolatilityUnknown {
		return
	}
	a.edges++
	ends := []string{to}
	if strength == relationship.StrengthFunctional || strength == relationship.StrengthSymmetric {
		ends = append(ends, from)
	}
	for _, m := range ends {
		if def, ok := modules[m]; ok && def.Volatility == "" && def.Subdomain == "" {
			a.modules[m] = struct{}{}
		}
	}
}

func (a *unratedAccumulator) names() []string {
	out := make([]string, 0, len(a.modules))
	for m := range a.modules {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

type spanAccumulator struct{ boundary, ancestor map[int]int }

func (a *spanAccumulator) add(tree classify.Containment, from, to string, distance relationship.Distance) {
	if distance == relationship.DistanceSameModule || distance == relationship.DistanceUnknown || distance == relationship.DistanceExternal || from == "" || to == "" {
		return
	}
	span := classify.HierarchySpan(tree, from, to)
	if span.BoundaryCrossings <= 0 {
		return
	}
	if a.boundary == nil {
		a.boundary = map[int]int{}
		a.ancestor = map[int]int{}
	}
	a.boundary[span.BoundaryCrossings]++
	a.ancestor[span.SharedAncestor]++
}
func (a spanAccumulator) apply(d *relationship.DistanceCompressionSummary) {
	if d == nil {
		return
	}
	d.ContainmentBoundaryCounts = distanceCounts(a.boundary)
	d.ContainmentAncestorDepths = distanceCounts(a.ancestor)
}
func distanceCounts(m map[int]int) []relationship.DistanceCount {
	if len(m) == 0 {
		return nil
	}
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	out := make([]relationship.DistanceCount, 0, len(ks))
	for _, k := range ks {
		out = append(out, relationship.DistanceCount{Value: k, Count: m[k]})
	}
	return out
}

func buildConnascenceSummary(set relationship.Set) *evidence.ConnascenceReport {
	r := &evidence.ConnascenceReport{ByKind: map[string]int{}, BySource: map[string]int{}}
	for _, e := range set.Edges {
		if len(e.Classified.Connascence) == 0 {
			r.AbstainedEdges++
			continue
		}
		r.EdgesWithEvidence++
		for _, v := range e.Classified.Connascence {
			r.TotalEvidence++
			r.ByKind[v.Kind]++
			r.BySource[v.Source]++
		}
	}
	kinds := []string{"position", "execution", "timing", "value", "identity"}
	for _, k := range kinds {
		if r.ByKind[k] == 0 {
			r.Unmeasured = append(r.Unmeasured, k)
		}
	}
	r.Roadmap = connascenceRoadmap(r.ByKind)
	if len(r.ByKind) == 0 {
		r.ByKind = nil
	}
	if len(r.BySource) == 0 {
		r.BySource = nil
	}
	return r
}

const (
	connascenceDeterministicStatic = "deterministic_static"
	connascenceUnmeasuredStatic    = "unmeasured_static"
	connascenceUnmeasuredDynamic   = "unmeasured_dynamic"
	connascenceSourceGoTypes       = "go/types"
	connascenceSourceDepCruise     = "dependency-cruiser"
	connascenceSourceGrimp         = "grimp"
	connascenceSourceSCIP          = "scip"
)

func connascenceRoadmap(by map[string]int) []evidence.ConnascenceRoadmapItem {
	items := make([]evidence.ConnascenceRoadmapItem, 0, 9)
	items = append(items,
		evidence.ConnascenceRoadmapItem{Kind: "name", CurrentStatus: connascenceDeterministicStatic, Sources: []string{connascenceSourceGoTypes, connascenceSourceDepCruise, connascenceSourceGrimp, connascenceSourceSCIP}},
		evidence.ConnascenceRoadmapItem{Kind: "type", CurrentStatus: connascenceDeterministicStatic, Sources: []string{connascenceSourceGoTypes, connascenceSourceDepCruise, connascenceSourceSCIP}},
		evidence.ConnascenceRoadmapItem{Kind: "meaning", CurrentStatus: connascenceDeterministicStatic, Sources: []string{connascenceSourceGoTypes, connascenceSourceSCIP}},
		evidence.ConnascenceRoadmapItem{Kind: "algorithm", CurrentStatus: connascenceDeterministicStatic, Sources: []string{connascenceSourceGoTypes, connascenceSourceSCIP}},
		evidence.ConnascenceRoadmapItem{Kind: "position", CurrentStatus: positionStatus(by), Sources: positionSources(by), UpgradeTrigger: "deterministic argument-order or tuple-position facts from an extractor"},
	)
	for _, kind := range []string{"execution", "timing", "value", "identity"} {
		items = append(items, evidence.ConnascenceRoadmapItem{Kind: kind, CurrentStatus: connascenceUnmeasuredDynamic, RelatedSignals: []string{"dynamic_imports", "runtime_async_edges"}, UpgradeTrigger: "deterministic source-module to runtime-order/value/identity facts; LLM narrative alone is insufficient"})
	}
	return items
}
func positionStatus(by map[string]int) string {
	if by["position"] > 0 {
		return connascenceDeterministicStatic
	}
	return connascenceUnmeasuredStatic
}
func positionSources(by map[string]int) []string {
	if by["position"] > 0 {
		return []string{connascenceSourceSCIP}
	}
	return nil
}

func buildLocalCouplingSummary(set relationship.Set) []evidence.LocalCouplingModule {
	type agg struct {
		scored, abstained, complexity, sum int
		off                                []evidence.LocalCouplingEdge
	}
	groups := map[string]*agg{}
	for _, e := range set.Edges {
		if e.Distance != relationship.DistanceSameModule {
			continue
		}
		m := e.FromModule
		if m == "" {
			m = e.ToModule
		}
		if m == "" {
			continue
		}
		a := groups[m]
		if a == nil {
			a = &agg{}
			groups[m] = a
		}
		if !e.Classified.Score.Scored {
			a.abstained++
			continue
		}
		a.scored++
		a.sum += e.Classified.Score.Balance
		if scoring.LocalComplexity(toCoupling(e)) {
			a.complexity++
		}
		if e.Classified.Score.Band != relationship.SeverityNone {
			off := evidence.LocalCouplingEdge{From: e.FromPath, To: e.ToPath, Strength: string(e.Strength), Balance: e.Classified.Score.Balance, Band: string(e.Classified.Score.Band)}
			if len(e.Locations) > 0 {
				off.File = e.Locations[0].File
				off.Line = e.Locations[0].Line
			}
			a.off = append(a.off, off)
		}
	}
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]evidence.LocalCouplingModule, 0, len(names))
	for _, n := range names {
		a := groups[n]
		m := evidence.LocalCouplingModule{Module: n, ScoredEdges: a.scored, AbstainedEdges: a.abstained, ComplexityEdges: a.complexity, WorstOffenders: a.off}
		if a.scored > 0 {
			m.ComplexitySharePct = 100 * a.complexity / a.scored
			m.MeanBalance = float64(a.sum) / float64(a.scored)
		}
		sort.Slice(m.WorstOffenders, func(i, j int) bool {
			a, b := m.WorstOffenders[i], m.WorstOffenders[j]
			if a.Balance != b.Balance {
				return a.Balance < b.Balance
			}
			if a.From != b.From {
				return a.From < b.From
			}
			return a.To < b.To
		})
		if len(m.WorstOffenders) > 5 {
			m.WorstOffenders = m.WorstOffenders[:5]
		}
		out = append(out, m)
	}
	return out
}
func toCoupling(e relationship.Edge) coupling.Classification {
	return coupling.Classification{Strength: e.Strength, Distance: e.Distance, Volatility: e.Volatility, Severity: e.Severity, Score: coupling.EdgeScore{Scored: e.Classified.Score.Scored, Balance: e.Classified.Score.Balance, Value: e.Classified.Score.Value, Band: e.Classified.Score.Band, Breakdown: coupling.ScoreBreakdown{Modularity: e.Classified.Score.Breakdown.Modularity, VolatilityVal: e.Classified.Score.Breakdown.VolatilityValue}}}
}

func buildStaticDistanceCandidates(g *graph.Graph, idx coupling.Index, mm policy.ModuleMap) []evidence.DistanceConfigCandidate {
	if g == nil {
		return nil
	}
	groups := map[string]*evidence.DistanceConfigCandidate{}
	seen := map[string]map[evidence.DistanceConfigEvidenceSite]struct{}{}
	// Hoisted: GoModules copies its backing slice on every call.
	goModules := g.GoModules()
	for _, e := range g.Edges() {
		c, ok := idx[e.From+"\x00"+e.To+"\x00"+string(e.Kind)]
		if !ok || c.Distance != coupling.DistanceUnknown {
			continue
		}
		from, ok := moduleForNode(e.From, mm)
		if !ok || from == "" {
			continue
		}
		if to, ok := moduleForNode(e.To, mm); ok && to != "" {
			continue
		}
		target, rawTarget, ok := normalizeExternalTarget(e.To, e.Language, goModules)
		if !ok || target == "" {
			continue
		}
		k := from + "\x00" + target + "\x00" + string(e.Kind)
		v := groups[k]
		if v == nil {
			v = &evidence.DistanceConfigCandidate{SourceBlock: "classified_external_edges", Module: from, Target: target, IntegrationKind: string(e.Kind), SuggestedReviewAction: "external_systems"}
			groups[k] = v
			seen[k] = map[evidence.DistanceConfigEvidenceSite]struct{}{}
		}
		v.Count++
		for _, l := range e.Locations {
			// Dedup per group. Several edges in one group can carry the same
			// site whenever an extractor locates them at a file without a line
			// (a Rust dependency the manifest scan cannot place keeps its
			// member's Cargo.toml with line 0).
			site := evidence.DistanceConfigEvidenceSite{File: l.File, Line: l.Line, Kind: string(e.Kind), Language: e.Language, Target: rawTarget}
			if _, dup := seen[k][site]; dup {
				continue
			}
			seen[k][site] = struct{}{}
			v.EvidenceSites = append(v.EvidenceSites, site)
		}
	}
	out := make([]evidence.DistanceConfigCandidate, 0, len(groups))
	for _, v := range groups {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].IntegrationKind < out[j].IntegrationKind
	})
	return out
}

func moduleForNode(id string, mm policy.ModuleMap) (string, bool) {
	kind, path, ok := strings.Cut(id, ":")
	if !ok || path == "" {
		return "", false
	}
	if m, ok := mm.ModuleFor(path); ok {
		return m, true
	}
	if kind == string(graph.NodeKindFile) {
		return mm.ModuleForFile(path)
	}
	if mm.Has(path) {
		return path, true
	}
	return "", false
}
func normalizeExternalTarget(id, lang string, goModules []graph.GoModule) (target, raw string, ok bool) {
	kind, path, ok := strings.Cut(id, ":")
	if !ok || path == "" {
		return "", "", false
	}
	raw = path
	switch kind {
	case string(graph.NodeKindPackage):
		if lang != graph.LangGo {
			return "", raw, false
		}
		return normalizeGoTarget(path, goModules)
	case string(graph.NodeKindExternal):
		switch lang {
		case graph.LangTypeScript:
			return normalizeTSTarget(path)
		case graph.LangPython:
			return normalizePyTarget(path)
		case graph.LangRust:
			return path, raw, true
		}
	}
	return "", raw, false
}
func normalizeGoTarget(path string, mods []graph.GoModule) (string, string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 || !strings.Contains(parts[0], ".") {
		return "", path, false
	}
	for _, m := range mods {
		if m.Path != "" && (path == m.Path || strings.HasPrefix(path, m.Path+"/")) {
			return "", path, false
		}
	}
	n := 3
	if len(parts) < n {
		n = len(parts)
	}
	return strings.Join(parts[:n], "/") + "/**", path, true
}
func normalizeTSTarget(path string) (string, string, bool) {
	if !strings.HasPrefix(path, "node_modules/") {
		return "", path, false
	}
	p := strings.TrimPrefix(path, "node_modules/")
	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", path, false
	}
	root := parts[0]
	if strings.HasPrefix(root, "@") {
		// A scope needs a package after it. A bare "@" is not a scope, so
		// node_modules/@/foo must not become an external_systems suggestion.
		if len(parts) < 2 || root == "@" {
			return "", path, false
		}
		root += "/" + parts[1]
	}
	return "node_modules/" + root + "/**", path, true
}
func normalizePyTarget(path string) (string, string, bool) {
	if path == "" || strings.HasPrefix(path, ".") || strings.Contains(path, "..") {
		return "", path, false
	}
	parts := strings.Split(path, ".")
	for _, p := range parts {
		if p == "" {
			return "", path, false
		}
		for i, r := range p {
			valid := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9'
			if !valid {
				return "", path, false
			}
		}
	}
	return "{" + parts[0] + "," + parts[0] + ".*}", path, true
}
