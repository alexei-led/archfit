package initcfg

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Language keys reused in Render's language loop and language-specific cases.
const (
	langGo         = "go"
	langPython     = "python"
	langTypeScript = "typescript"
	langRust       = "rust"
)

// disambiguateNames ensures every ModuleDef in mods has a unique Name.
// Non-colliding names are returned unchanged.
// See Discover for the full two-pass algorithm.
func disambiguateNames(mods []ModuleDef) []ModuleDef {
	if len(mods) == 0 {
		return mods
	}

	// Count occurrences of each name.
	count := make(map[string]int, len(mods))
	for _, m := range mods {
		count[m.Name]++
	}

	// First pass: replace every colliding name with its path slug.
	for i, m := range mods {
		if count[m.Name] > 1 {
			mods[i].Name = pathSlug(m.Paths)
		}
	}

	// Second pass: resolve any remaining collisions (slug vs slug, or slug vs
	// an original name that was not changed) with a numeric suffix.
	seen := make(map[string]bool, len(mods))
	for i, m := range mods {
		name := m.Name
		if !seen[name] {
			seen[name] = true
			continue
		}
		// Find the next free suffix _2, _3, …
		for n := 2; ; n++ {
			candidate := fmt.Sprintf("%s_%d", name, n)
			if !seen[candidate] {
				mods[i].Name = candidate //nolint:gosec // i is a valid range index
				seen[candidate] = true
				break
			}
		}
	}

	return mods
}

// pathSlug derives a short, filesystem-safe name from the first element of
// paths: strips a trailing "/**", then replaces "/" and "." with "_".
func pathSlug(paths []string) string {
	if len(paths) == 0 {
		return "unknown"
	}
	s := strings.TrimSuffix(paths[0], "/**")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// inferLayers derives the ordered, deduplicated layer list from discovered
// modules. Only Rust discovery assigns layers — topological tiers of the crate
// dependency graph (layer-0, layer-1, …) — so the list is those tiers in
// numeric order. No layer is ever inferred from a directory or package name.
func inferLayers(mods []ModuleDef) []string {
	assigned := make(map[string]string)
	for _, m := range mods {
		if m.Layer != "" {
			assigned[m.Layer] = m.Layer
		}
	}
	if len(assigned) == 0 {
		return nil
	}
	return topoLayerList(assigned)
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// topoLayerAssign computes a topological level (0 = foundation/leaves, N =
// entrypoints) for each module name from the directed edge set, then returns a
// map from module name → layer name ("layer-0", "layer-1", …).
//
// Modules not reachable in the graph (isolated) are placed at level 0.
// When edges is empty the returned map is also empty.
func topoLayerAssign(names []string, edges []ModuleEdge) map[string]string {
	if len(edges) == 0 {
		return nil
	}

	// Build adjacency list (from → to) and in-degree count.
	// "from" imports "to", so "to" is a foundation dependency of "from".
	// Topological level: leaves (no outgoing edges / pure foundations) = 0;
	// callers of leaves = 1; and so on.
	// We compute this as longest-path-from-leaf (Kahn's algorithm on the
	// *reverse* graph so that leaves come out at level 0).
	inDeg := make(map[string]int, len(names))
	adj := make(map[string][]string, len(names))
	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
		inDeg[n] = 0
	}
	for _, e := range edges {
		if _, ok := nameSet[e.From]; !ok {
			continue
		}
		if _, ok := nameSet[e.To]; !ok {
			continue
		}
		// In the reverse graph (to → from) we want to propagate level upward.
		adj[e.To] = append(adj[e.To], e.From)
		inDeg[e.From]++
	}

	// Kahn's BFS on the reverse graph: nodes with inDeg 0 are leaves (level 0).
	level := make(map[string]int, len(names))
	queue := make([]string, 0, len(names))
	for _, n := range names {
		if inDeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	// Sort for determinism.
	sort.Strings(queue)

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if level[next] < level[cur]+1 {
				level[next] = level[cur] + 1
			}
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
				sort.Strings(queue) // keep deterministic
			}
		}
	}

	result := make(map[string]string, len(names))
	for _, n := range names {
		result[n] = fmt.Sprintf("layer-%d", level[n])
	}
	return result
}

// topoLayerList returns the sorted unique layer names from a topo assignment map,
// ordered by their numeric tier (layer-0 first).
func topoLayerList(assignment map[string]string) []string {
	seen := make(map[string]struct{}, len(assignment))
	for _, l := range assignment {
		seen[l] = struct{}{}
	}
	layers := make([]string, 0, len(seen))
	for l := range seen {
		layers = append(layers, l)
	}
	sort.Slice(layers, func(i, j int) bool {
		// "layer-N" — compare numerically by suffix.
		ni := layerTierNum(layers[i])
		nj := layerTierNum(layers[j])
		if ni != nj {
			return ni < nj
		}
		return layers[i] < layers[j]
	})
	return layers
}

// layerTierNum extracts the trailing integer from a "layer-N" name.
// Returns 0 for unrecognised names.
func layerTierNum(name string) int {
	const prefix = "layer-"
	if !strings.HasPrefix(name, prefix) {
		return 0
	}
	n := 0
	for _, c := range name[len(prefix):] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
