package evaluation

import (
	"math"
	"sort"

	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	syntaxKindFunction = "function"
	syntaxKindMethod   = "method"
)

// moduleGraphComplexity collapses source relationships to distinct directed
// module dependencies. Degree counts distinct neighbouring modules. Dependency
// depth is the longest path, in edges, through the SCC-condensation DAG: edges
// inside a cycle add no depth, leaving cycle severity to the existing cycle
// evidence.
func moduleGraphComplexity(modules map[string]policy.ModuleDef, set relationship.Set) *result.ModuleGraphComplexity {
	if len(modules) == 0 {
		return nil
	}

	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)

	adjSets := make(map[string]map[string]struct{}, len(names))
	fanInSets := make(map[string]map[string]struct{}, len(names))
	for _, name := range names {
		adjSets[name] = map[string]struct{}{}
		fanInSets[name] = map[string]struct{}{}
	}
	for _, edge := range set.DependencyEdges() {
		_, fromDeclared := modules[edge.FromModule]
		_, toDeclared := modules[edge.ToModule]
		if !fromDeclared || !toDeclared || edge.FromModule == edge.ToModule {
			continue
		}
		adjSets[edge.FromModule][edge.ToModule] = struct{}{}
		fanInSets[edge.ToModule][edge.FromModule] = struct{}{}
	}

	adj := make(map[string][]string, len(names))
	fanIn, fanOut := make([]int, 0, len(names)), make([]int, 0, len(names))
	for _, name := range names {
		adj[name] = sortedSet(adjSets[name])
		fanIn = append(fanIn, len(fanInSets[name]))
		fanOut = append(fanOut, len(adjSets[name]))
	}

	components := stronglyConnectedComponents(names, adj)
	componentOf := make(map[string]int, len(names))
	for component, members := range components {
		for _, module := range members {
			componentOf[module] = component
		}
	}
	componentAdj := make([]map[int]struct{}, len(components))
	for i := range componentAdj {
		componentAdj[i] = map[int]struct{}{}
	}
	for _, from := range names {
		fromComponent := componentOf[from]
		for _, to := range adj[from] {
			toComponent := componentOf[to]
			if fromComponent != toComponent {
				componentAdj[fromComponent][toComponent] = struct{}{}
			}
		}
	}

	memo := make([]int, len(components))
	known := make([]bool, len(components))
	var depth func(int) int
	depth = func(component int) int {
		if known[component] {
			return memo[component]
		}
		known[component] = true
		for next := range componentAdj[component] {
			memo[component] = max(memo[component], 1+depth(next))
		}
		return memo[component]
	}
	maxDepth := 0
	for component := range components {
		maxDepth = max(maxDepth, depth(component))
	}

	return &result.ModuleGraphComplexity{
		Modules: len(names), MaxDependencyChain: maxDepth,
		FanInP90: nearestRankValue(fanIn, 0.90), FanOutP90: nearestRankValue(fanOut, 0.90),
	}
}

func sortedSet(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for value := range in {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// stronglyConnectedComponents returns Tarjan SCCs over names and adjacency.
// Both inputs are sorted, making component discovery deterministic even though
// only component-independent aggregate values leave this helper.
func stronglyConnectedComponents(names []string, adj map[string][]string) [][]string {
	index := 0
	indices := make(map[string]int, len(names))
	lowlink := make(map[string]int, len(names))
	onStack := make(map[string]bool, len(names))
	stack := make([]string, 0, len(names))
	components := make([][]string, 0)

	var visit func(string)
	visit = func(module string) {
		indices[module] = index
		lowlink[module] = index
		index++
		stack = append(stack, module)
		onStack[module] = true

		for _, next := range adj[module] {
			nextIndex, visited := indices[next]
			if !visited {
				visit(next)
				lowlink[module] = min(lowlink[module], lowlink[next])
			} else if onStack[next] {
				lowlink[module] = min(lowlink[module], nextIndex)
			}
		}
		if lowlink[module] != indices[module] {
			return
		}

		component := make([]string, 0, 1)
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			component = append(component, member)
			if member == module {
				break
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}

	for _, module := range names {
		if _, visited := indices[module]; !visited {
			visit(module)
		}
	}
	return components
}

func nearestRankValue(values []int, percentile float64) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	index = max(0, min(index, len(sorted)-1))
	return sorted[index]
}

type functionLengthStats struct {
	Total         int
	Observed      int
	P50           int
	P90           int
	Max           int
	OverThreshold int
}

// functionLengthDistribution measures inclusive source extents for functions
// and methods. A missing or malformed extent remains in Total but not Observed;
// it is never converted to a zero-length function.
func functionLengthDistribution(facts []modevidence.SyntaxFact, threshold int) functionLengthStats {
	if threshold <= 0 {
		threshold = policy.DefaultFunctionLOCThreshold
	}
	lengths := make([]int, 0, len(facts))
	for _, fact := range facts {
		if fact.Kind != syntaxKindFunction && fact.Kind != syntaxKindMethod {
			continue
		}
		if fact.EndLine == 0 || fact.StartLine <= 0 || fact.EndLine < fact.StartLine {
			continue
		}
		lengths = append(lengths, fact.EndLine-fact.StartLine+1)
	}

	stats := functionLengthStats{}
	for _, fact := range facts {
		if fact.Kind == syntaxKindFunction || fact.Kind == syntaxKindMethod {
			stats.Total++
		}
	}
	stats.Observed = len(lengths)
	if len(lengths) == 0 {
		return stats
	}
	sort.Ints(lengths)
	stats.P50 = nearestRankValue(lengths, 0.50)
	stats.P90 = nearestRankValue(lengths, 0.90)
	stats.Max = lengths[len(lengths)-1]
	for _, length := range lengths {
		if length > threshold {
			stats.OverThreshold++
		}
	}
	return stats
}
