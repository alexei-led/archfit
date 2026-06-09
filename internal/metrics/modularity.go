package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// modularitySmallN is the node count below which concentration metrics are not
// meaningful (a handful of packages always has one depended-upon core). Below it
// the metric reports low confidence (same discipline as encapsulation coverage).
const modularitySmallN = 15

// hubBlastThreshold is the relative-blast fraction at which a module is a "hub"
// whose change is expensive. Tunable; report-only until calibrated on more repos.
const hubBlastThreshold = 0.30

// moduleKey collapses a graph node id ("kind:path") to its package/module unit so
// blast radius is computed at the granularity the metric reports: Go file nodes
// collapse to their package directory; module/package/TS-file nodes pass through.
func moduleKey(nodeID string) string {
	path := nodeID
	if _, after, ok := strings.Cut(nodeID, ":"); ok {
		path = after
	}
	if strings.HasSuffix(path, ".go") {
		if j := strings.LastIndexByte(path, '/'); j >= 0 {
			return path[:j] // package = directory
		}
	}
	return path
}

// blastRadius returns, per first-party module, the number of other first-party
// modules that transitively depend on it. The graph is collapsed to module units
// (moduleKey), restricted to first-party modules (those that import something — a
// node that only ever appears as an import target is an external dependency, since
// archfit never parses its source), and SCC-condensed so import cycles do not
// inflate the count (Martin's metrics and blast radius assume a DAG). Returns the
// per-module blast and the count of first-party modules.
func blastRadius(g *graph.Graph) (map[string]int, int) {
	// First-party = the nodes archfit actually parsed (g.Nodes()). External
	// dependencies appear only as edge targets, never as nodes, so this excludes
	// stdlib/third-party packages without dropping pure-leaf internal modules.
	firstParty := make(map[string]struct{})
	for _, n := range g.Nodes() {
		firstParty[moduleKey(n.ID())] = struct{}{}
	}
	// Collapsed module adjacency over first-party modules only.
	adj := make(map[string]map[string]struct{})
	for m := range firstParty {
		adj[m] = make(map[string]struct{})
	}
	for _, e := range g.Edges() {
		from, to := moduleKey(e.From), moduleKey(e.To)
		if from == to {
			continue
		}
		if _, ok := firstParty[to]; !ok {
			continue // edge into an external dependency
		}
		adj[from][to] = struct{}{}
	}

	comp := tarjanSCC(adj) // module -> component id
	compMembers := make(map[int]int)
	for _, c := range comp {
		compMembers[c]++
	}
	// Condensed reverse adjacency (component -> set of components that import it).
	crev := make(map[int]map[int]struct{})
	for from, tos := range adj {
		for to := range tos {
			cf, ct := comp[from], comp[to]
			if cf == ct {
				continue
			}
			if crev[ct] == nil {
				crev[ct] = make(map[int]struct{})
			}
			crev[ct][cf] = struct{}{}
		}
	}

	blast := make(map[string]int, len(firstParty))
	for m := range firstParty {
		// Transitive reverse-reach in the condensed DAG, counting member modules.
		start := comp[m]
		seen := map[int]struct{}{start: {}}
		stack := []int{start}
		members := 0
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for pc := range crev[c] {
				if _, ok := seen[pc]; !ok {
					seen[pc] = struct{}{}
					members += compMembers[pc]
					stack = append(stack, pc)
				}
			}
		}
		// Members of m's own SCC (besides m) also depend on m.
		blast[m] = members + (compMembers[start] - 1)
	}
	return blast, len(firstParty)
}

// tarjanSCC assigns each node a strongly-connected-component id over the adjacency.
func tarjanSCC(adj map[string]map[string]struct{}) map[string]int {
	const unvisited = -1
	index := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	comp := make(map[string]int)
	for n := range adj {
		index[n] = unvisited
	}
	var stack []string
	counter, compID := 0, 0

	// Iterative DFS to avoid stack overflow on large graphs.
	strongConnect := func(root string) {
		type frame struct {
			node string
			next []string
			i    int
		}
		neighbors := func(n string) []string {
			ns := make([]string, 0, len(adj[n]))
			for to := range adj[n] {
				ns = append(ns, to)
			}
			return ns
		}
		dfs := []frame{{node: root, next: neighbors(root)}}
		index[root], low[root] = counter, counter
		counter++
		stack = append(stack, root)
		onStack[root] = true
		for len(dfs) > 0 {
			f := &dfs[len(dfs)-1]
			if f.i < len(f.next) {
				w := f.next[f.i]
				f.i++
				if index[w] == unvisited {
					index[w], low[w] = counter, counter
					counter++
					stack = append(stack, w)
					onStack[w] = true
					dfs = append(dfs, frame{node: w, next: neighbors(w)})
				} else if onStack[w] && index[w] < low[f.node] {
					low[f.node] = index[w]
				}
				continue
			}
			if low[f.node] == index[f.node] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp[w] = compID
					if w == f.node {
						break
					}
				}
				compID++
			}
			child := f.node
			dfs = dfs[:len(dfs)-1]
			if len(dfs) > 0 {
				parent := &dfs[len(dfs)-1]
				if low[child] < low[parent.node] {
					low[parent.node] = low[child]
				}
			}
		}
	}
	for n := range adj {
		if index[n] == unvisited {
			strongConnect(n)
		}
	}
	return comp
}

// BlastRadiusMetric reports change-impact concentration: how many modules are
// "hubs" whose change forces a wide transitive rebuild, and which ones. It is a
// factual finding (report-only), not a quality verdict — a stable hub is fine; the
// volatile-hub judgement belongs to change_amplification.
type BlastRadiusMetric struct{}

// Name returns "blast_radius".
func (m BlastRadiusMetric) Name() string { return "blast_radius" }

// Version returns "blast_radius.v1".
func (m BlastRadiusMetric) Version() string { return "blast_radius.v1" }

// hubInfo is one high-blast module for display.
type hubInfo struct {
	module string
	blast  int
	rel    float64
}

// Calculate computes per-module blast radius and reports the hubs above the
// threshold. Indeterminate (n/a) for graphs too small to have meaningful
// concentration.
func (m BlastRadiusMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.naResult()
	}
	blast, n := blastRadius(in.Graph)
	if n < 2 {
		return m.naResult()
	}
	denom := float64(n - 1)

	hubs := make([]hubInfo, 0)
	for mod, b := range blast {
		rel := float64(b) / denom
		if rel >= hubBlastThreshold {
			hubs = append(hubs, hubInfo{module: mod, blast: b, rel: rel})
		}
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].blast > hubs[j].blast })

	confidence := confidenceHigh
	if n < modularitySmallN {
		confidence = confidenceLow
	}
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(hubs)),
		Display:    blastDisplay(hubs, n),
		Band:       bandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: "modules whose transitive reverse-dependencies exceed " +
			fmt.Sprintf("%.0f%%", hubBlastThreshold*100) + " of the codebase (change-impact hubs)",
	}
}

func (m BlastRadiusMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: m.Name(), Value: 0, Display: bandNA, Band: bandNA,
		Confidence: confidenceLow, Version: m.Version(), Mode: modeCount,
		Definition: "modules whose transitive reverse-dependencies are a large fraction of the codebase",
	}
}

// blastDisplay renders the top hubs compactly for human/LLM output.
func blastDisplay(hubs []hubInfo, n int) string {
	if len(hubs) == 0 {
		return fmt.Sprintf("0 change-impact hubs (%d modules)", n)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d change-impact hub(s): ", len(hubs))
	for i, h := range hubs {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(hubs)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%.0f%%, %d deps)", shortModule(h.module), h.rel*100, h.blast)
	}
	return b.String()
}

// shortModule trims a module path to its last two segments for compact display.
func shortModule(mod string) string {
	parts := strings.Split(strings.ReplaceAll(mod, ".", "/"), "/")
	if len(parts) <= 2 {
		return mod
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}

// ---------------------------------------------------------------------------
// change_amplification + hidden_coupling (volatility-weighted modularity)
// ---------------------------------------------------------------------------

// ampHubThreshold is the per-repo change-amplification level (blast×volatility)
// above which a module is a volatile hub. Tunable; the metric is report-only and
// the ranked hub list — not a cross-repo count — is the intended signal.
const ampHubThreshold = 0.15

// hiddenLCThreshold is the logical-coupling level (co-change / min churn) at which
// a non-importing module pair counts as hidden coupling. Tunable, report-only.
const hiddenLCThreshold = 0.5

// hiddenMinSupport is the minimum co-change commits before a pair is considered
// (small samples are noise).
const hiddenMinSupport = 4

// dominantLanguage returns the language of most edges (drives file→module mapping).
func dominantLanguage(g *graph.Graph) string {
	cnt := map[string]int{}
	for _, e := range g.Edges() {
		cnt[e.Language]++
	}
	best, bn := "", 0
	for l, n := range cnt {
		if n > bn {
			best, bn = l, n
		}
	}
	return best
}

// fileToModuleKey maps a git file path to the same module unit as moduleKey, so
// per-file churn/co-change aggregates onto graph nodes.
func fileToModuleKey(file, lang string) string {
	switch lang {
	case "python":
		if !strings.HasSuffix(file, ".py") {
			return ""
		}
		p := strings.TrimSuffix(file, ".py")
		p = strings.TrimPrefix(p, "src/")
		p = strings.TrimSuffix(p, "/__init__")
		return strings.ReplaceAll(p, "/", ".")
	case "go":
		if !strings.HasSuffix(file, ".go") {
			return ""
		}
		if i := strings.LastIndexByte(file, '/'); i >= 0 {
			return file[:i]
		}
		return ""
	default: // typescript / javascript: the file is the node
		return file
	}
}

// moduleChurn aggregates per-file churn onto module keys.
func moduleChurn(fileChurn map[string]int, lang string) map[string]int {
	mc := map[string]int{}
	for f, c := range fileChurn {
		if k := fileToModuleKey(f, lang); k != "" {
			mc[k] += c
		}
	}
	return mc
}

// ChangeAmplificationMetric reports modules whose change is expensive AND likely:
// blast radius (transitive reverse-deps) weighted by volatility (recent churn). A
// stable hub scores low (fine); a volatile hub scores high (the real risk). This is
// archfit's on-mission signal — expected change cost — reported as a ranked finding.
type ChangeAmplificationMetric struct{}

// Name returns "change_amplification".
func (m ChangeAmplificationMetric) Name() string { return "change_amplification" }

// Version returns "change_amplification.v1".
func (m ChangeAmplificationMetric) Version() string { return "change_amplification.v1" }

type ampHub struct {
	module string
	amp    float64
	blast  int
	churn  int
}

// Calculate ranks modules by change amplification. n/a without a graph or churn.
func (m ChangeAmplificationMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	if in.Graph == nil || len(in.FileChurn) == 0 {
		return naCount(m.Name(), m.Version(), "blast radius weighted by recent churn (expected change cost)")
	}
	blast, n := blastRadius(in.Graph)
	if n < 2 {
		return naCount(m.Name(), m.Version(), "blast radius weighted by recent churn (expected change cost)")
	}
	mc := moduleChurn(in.FileChurn, dominantLanguage(in.Graph))
	maxChurn := 0
	for _, c := range mc {
		if c > maxChurn {
			maxChurn = c
		}
	}
	if maxChurn == 0 {
		return naCount(m.Name(), m.Version(), "blast radius weighted by recent churn (expected change cost)")
	}
	denom := float64(n - 1)

	var hubs []ampHub
	for mod, b := range blast {
		amp := (float64(b) / denom) * (float64(mc[mod]) / float64(maxChurn))
		if amp >= ampHubThreshold {
			hubs = append(hubs, ampHub{module: mod, amp: amp, blast: b, churn: mc[mod]})
		}
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].amp > hubs[j].amp })

	confidence := confidenceHigh
	if n < modularitySmallN {
		confidence = confidenceLow
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(len(hubs)), Display: ampDisplay(hubs),
		Band: bandInformational, Confidence: confidence, Version: m.Version(), Mode: modeCount,
		Definition: "modules ranked by blast radius × recent churn (volatile change-impact hubs)",
	}
}

func ampDisplay(hubs []ampHub) string {
	if len(hubs) == 0 {
		return "0 volatile change-impact hubs"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d volatile hub(s): ", len(hubs))
	for i, h := range hubs {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(hubs)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (amp %.2f: %d deps × %d commits)", shortModule(h.module), h.amp, h.blast, h.churn)
	}
	return b.String()
}

// HiddenCouplingMetric reports module pairs that change together (git co-change)
// but do NOT import each other — coupling invisible to the dependency graph (e.g.
// deflected by boundary tests or mediated by a shared data model). The canonical
// case is a module with low import fan-in that co-changes with many others.
type HiddenCouplingMetric struct{}

// Name returns "hidden_coupling".
func (m HiddenCouplingMetric) Name() string { return "hidden_coupling" }

// Version returns "hidden_coupling.v1".
func (m HiddenCouplingMetric) Version() string { return "hidden_coupling.v1" }

// Calculate counts hidden-coupling module pairs (co-change without a static edge).
func (m HiddenCouplingMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	def := "module pairs that co-change without importing each other (coupling unseen by the graph)"
	if in.Graph == nil || len(in.CoChange) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}
	lang := dominantLanguage(in.Graph)
	mc := moduleChurn(in.FileChurn, lang)

	// First-party module nodes: restrict co-change to real graph modules so docs
	// (CHANGELOG.md), config files, and pre-rename historical paths are excluded.
	nodeSet := make(map[string]struct{})
	for _, n := range in.Graph.Nodes() {
		nodeSet[moduleKey(n.ID())] = struct{}{}
	}
	// Static undirected module edges (importing relationship).
	edge := make(map[[2]string]struct{})
	for _, e := range in.Graph.Edges() {
		a, b := moduleKey(e.From), moduleKey(e.To)
		if a == b {
			continue
		}
		edge[orderedPair(a, b)] = struct{}{}
	}
	// Aggregate file-pair co-change onto module pairs (graph modules only).
	modPair := make(map[[2]string]int)
	for fp, c := range in.CoChange {
		a, b := fileToModuleKey(fp[0], lang), fileToModuleKey(fp[1], lang)
		if a == "" || b == "" || a == b {
			continue
		}
		if _, ok := nodeSet[a]; !ok {
			continue
		}
		if _, ok := nodeSet[b]; !ok {
			continue
		}
		modPair[orderedPair(a, b)] += c
	}

	hiddenPartners := map[string]int{}
	pairs := 0
	for mp, nab := range modPair {
		if nab < hiddenMinSupport {
			continue
		}
		if _, imports := edge[mp]; imports {
			continue // expected coupling — they depend on each other
		}
		minChurn := min(mc[mp[0]], mc[mp[1]])
		if minChurn == 0 {
			continue
		}
		if float64(nab)/float64(minChurn) >= hiddenLCThreshold {
			hiddenPartners[mp[0]]++
			hiddenPartners[mp[1]]++
			pairs++
		}
	}

	type hp struct {
		module string
		count  int
	}
	var ranked []hp
	for mod, c := range hiddenPartners {
		ranked = append(ranked, hp{mod, c})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].count > ranked[j].count })

	n := len(mc)
	confidence := confidenceHigh
	if n < modularitySmallN {
		confidence = confidenceLow
	}
	var disp strings.Builder
	fmt.Fprintf(&disp, "%d hidden-coupling pair(s)", pairs)
	if len(ranked) > 0 {
		disp.WriteString("; top: ")
		for i, r := range ranked {
			if i == 4 {
				break
			}
			if i > 0 {
				disp.WriteString(", ")
			}
			fmt.Fprintf(&disp, "%s (%d)", shortModule(r.module), r.count)
		}
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(pairs), Display: disp.String(),
		Band: bandInformational, Confidence: confidence, Version: m.Version(), Mode: modeCount,
		Definition: def,
	}
}

func orderedPair(a, b string) [2]string {
	if a <= b {
		return [2]string{a, b}
	}
	return [2]string{b, a}
}

// naCount is the indeterminate result for a count-mode informational metric.
func naCount(name, version, def string) diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: name, Value: 0, Display: bandNA, Band: bandNA,
		Confidence: confidenceLow, Version: version, Mode: modeCount, Definition: def,
	}
}

// ---------------------------------------------------------------------------
// structural_weight (size skew / god-modules) — the validated cohesion_modularity
// signal. LCOM4 and "vocabulary mixing" were prototyped and dropped: they misrank
// (data registries score worst, the real 8-concern god-files score best), so size
// (with complexity once it lands) is the honest proxy for "too many responsibilities".
// Known blind spot: a god-file split across many small files is not flagged.
// ---------------------------------------------------------------------------

// godModuleMultiple is how many times the median module LOC a module must exceed
// (and godModuleFloor absolute LOC) to count as a size-skew god-module. Tunable,
// report-only until calibrated wider.
const (
	godModuleMultiple = 4
	godModuleFloor    = 400
)

// StructuralWeightMetric reports size skew: modules far larger than the codebase
// median, which concentrate responsibilities (a cohesion smell). Report-only; the
// ranked god-module list is the signal, not a quality verdict.
type StructuralWeightMetric struct{}

// Name returns "structural_weight".
func (m StructuralWeightMetric) Name() string { return "structural_weight" }

// Version returns "structural_weight.v1".
func (m StructuralWeightMetric) Version() string { return "structural_weight.v1" }

type sizeMod struct {
	module string
	loc    int
	mult   int
}

// Calculate aggregates per-file LOC onto modules and flags god-modules. n/a
// without LOC data or a graph (the latter gives the language for file→module).
func (m StructuralWeightMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	def := "modules whose size (LOC) is a large multiple of the codebase median (size-skew god-modules)"
	if in.Graph == nil || len(in.FileLOC) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}
	lang := dominantLanguage(in.Graph)
	modLOC := map[string]int{}
	for f, n := range in.FileLOC {
		if k := fileToModuleKey(f, lang); k != "" {
			modLOC[k] += n
		}
	}
	if len(modLOC) < 2 {
		return naCount(m.Name(), m.Version(), def)
	}
	locs := make([]int, 0, len(modLOC))
	for _, l := range modLOC {
		locs = append(locs, l)
	}
	sort.Ints(locs)
	median := locs[len(locs)/2]
	threshold := median * godModuleMultiple
	if threshold < godModuleFloor {
		threshold = godModuleFloor
	}

	var gods []sizeMod
	for mod, l := range modLOC {
		if l >= threshold {
			mult := 1
			if median > 0 {
				mult = l / median
			}
			gods = append(gods, sizeMod{module: mod, loc: l, mult: mult})
		}
	}
	sort.Slice(gods, func(i, j int) bool { return gods[i].loc > gods[j].loc })

	confidence := confidenceHigh
	if len(modLOC) < modularitySmallN {
		confidence = confidenceLow
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(len(gods)), Display: godDisplay(gods, median),
		Band: bandInformational, Confidence: confidence, Version: m.Version(), Mode: modeCount,
		Definition: def,
	}
}

func godDisplay(gods []sizeMod, median int) string {
	if len(gods) == 0 {
		return fmt.Sprintf("0 god-modules (median %d LOC)", median)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d god-module(s) (median %d LOC): ", len(gods), median)
	for i, g := range gods {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(gods)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%d LOC, %dx)", shortModule(g.module), g.loc, g.mult)
	}
	return b.String()
}
