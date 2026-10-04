package initcfg

import (
	"fmt"
	"sort"
	"strings"
)

// Starter rule IDs config init emits.
const (
	ruleIDModuleCycles  = "no-module-cycles"
	ruleIDLayerBackEdge = "no-layer-back-edges"
)

// Starter gate values.
const (
	gateFail = "fail"
	gateWarn = "warn"
)

// writeLayersHowTo emits the commented layers stanza Render writes when
// discovery could not infer two or more layers. Directory names do not prove a
// layer, and a guessed single layer (or one that puts domain and adapters
// together) yields a direction rule that can never fire.
func writeLayersHowTo(b *strings.Builder) {
	b.WriteString("# layers: not inferred. Directory names do not prove a layer, so none is guessed.\n")
	b.WriteString("# To enforce dependency direction: list layers innermost first (a module may\n")
	b.WriteString("# import its own layer or an earlier one), set layer: on every module, and\n")
	b.WriteString("# uncomment the no-layer-back-edges rule under rules:. For example:\n")
	b.WriteString("# layers:\n")
	b.WriteString("#   - domain\n")
	b.WriteString("#   - application\n")
	b.WriteString("#   - adapter\n")
	b.WriteString("#   - entrypoint\n\n")
}

// writeStarterRules emits the starter rule set. Each rule can fail on the
// violation it names: module_cycle checks every declared module, and the
// layer rule is live only when two or more layers were inferred (otherwise it
// is written commented, next to the layers how-to).
//
// The gate is adaptive: fail when the init-time import graph covers every
// module and shows no violation, so the first new one blocks; warn with the
// current count when it shows violations, or, with the reason, when init
// cannot prove the graph complete (cfg.GraphGap: TypeScript/Python discovery
// builds none, Rust analysis adds intra-crate modules discovery cannot see, a
// Go module owns source in a language init's graph omits).
func writeStarterRules(b *strings.Builder, cfg DiscoveredConfig) {
	b.WriteString("  # Catches: a dependency cycle between declared modules (A imports B and B\n")
	b.WriteString("  # imports A, through any files).\n")
	gate := writeGateNote(b, moduleCycleCount(cfg.Edges), cfg, "module cycle(s)")
	fmt.Fprintf(b, "  - id: %s\n    type: module_cycle\n    gate: %s\n", ruleIDModuleCycles, gate)

	if len(cfg.Layers) >= 2 {
		b.WriteString("  # Catches: a module importing a module in a later (outer) layer.\n")
		gate = writeGateNote(b, layerBackEdgeCount(cfg), cfg, "layer back-edge(s)")
		fmt.Fprintf(b, "  - id: %s\n    type: forbidden_layer_direction\n    gate: %s\n", ruleIDLayerBackEdge, gate)
		return
	}
	b.WriteString("  # Uncomment after declaring layers: (see above). Catches: a module importing a\n")
	b.WriteString("  # module in a later (outer) layer, e.g. domain code importing an adapter.\n")
	fmt.Fprintf(b, "  # - id: %s\n", ruleIDLayerBackEdge)
	b.WriteString("  #   type: forbidden_layer_direction\n")
	b.WriteString("  #   gate: fail\n")
}

// writeGateNote writes the comment explaining a starter rule's gate and
// returns the gate.
func writeGateNote(b *strings.Builder, violations int, cfg DiscoveredConfig, noun string) string {
	switch {
	case !cfg.ImportGraphComplete:
		b.WriteString("  # gate: warn — init saw no complete import graph. Run archfit check, fix or\n")
		b.WriteString("  # baseline what this rule reports, then set gate: fail.\n")
		if cfg.GraphGap != "" {
			fmt.Fprintf(b, "  # Why: %s.\n", sanitizeComment(cfg.GraphGap))
		}
		if violations > 0 {
			fmt.Fprintf(b, "  # The partial graph already shows %d %s.\n", violations, noun)
		}
		return gateWarn
	case violations > 0:
		fmt.Fprintf(b, "  # gate: warn — %d current %s at init. Fix them or run archfit baseline,\n", violations, noun)
		b.WriteString("  # then set gate: fail.\n")
		return gateWarn
	default:
		b.WriteString("  # gate: fail — none at init, so the first new one blocks archfit check.\n")
		return gateFail
	}
}

// moduleCycleCount returns the number of dependency cycles among discovered
// modules: strongly connected components with more than one module.
func moduleCycleCount(edges []ModuleEdge) int {
	adj := make(map[string][]string)
	nodes := make(map[string]struct{})
	for _, e := range edges {
		if e.From == e.To {
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
		nodes[e.From], nodes[e.To] = struct{}{}, struct{}{}
	}
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	sort.Strings(names)

	// Module graphs at init are small (tens of nodes), so per-node reachability
	// is cheaper to read than Tarjan and exact for component membership.
	reach := make(map[string]map[string]bool, len(names))
	for _, n := range names {
		seen := map[string]bool{}
		stack := append([]string(nil), adj[n]...)
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[cur] {
				continue
			}
			seen[cur] = true
			stack = append(stack, adj[cur]...)
		}
		reach[n] = seen
	}

	assigned := make(map[string]bool, len(names))
	cycles := 0
	for _, n := range names {
		if assigned[n] || !reach[n][n] {
			continue
		}
		for _, m := range names {
			if reach[n][m] && reach[m][n] {
				assigned[m] = true
			}
		}
		cycles++
	}
	return cycles
}

// layerBackEdgeCount counts discovered module edges that import a later
// (outer) layer — what forbidden_layer_direction reports.
func layerBackEdgeCount(cfg DiscoveredConfig) int {
	rank := make(map[string]int, len(cfg.Layers))
	for i, l := range cfg.Layers {
		rank[l] = i
	}
	moduleRank := make(map[string]int, len(cfg.Modules))
	for _, m := range cfg.Modules {
		if r, ok := rank[m.Layer]; ok {
			moduleRank[m.Name] = r
		}
	}
	count := 0
	for _, e := range cfg.Edges {
		from, okFrom := moduleRank[e.From]
		to, okTo := moduleRank[e.To]
		if okFrom && okTo && from < to {
			count++
		}
	}
	return count
}
