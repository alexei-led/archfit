// Package clone holds the neutral value types for clone-detection results so
// that core-ring packages (e.g. internal/metrics) can consume them without
// importing an adapter under internal/extract. The clone-detection adapter
// (internal/extract/clones) produces these; metrics reads them.
package clone

import "sort"

// Cluster represents a group of files that share a duplicated code block.
type Cluster struct {
	// Files contains the repo-relative (or absolute) paths involved in the clone.
	Files []string
	// Lines is the number of duplicated lines in the block.
	Lines int
}

// ModulePairs converts a slice of Clusters to canonical cross-module pairs
// using an injected key function. Same-module pairs are skipped. Each pair is
// returned in sorted order ([a,b] where a <= b) and the result slice is deduped
// and sorted for determinism.
//
// The key function is typically fileToModuleKey(file, lang) from the metrics
// package, injected at the call site so this package stays free of that import.
func ModulePairs(clusters []Cluster, key func(string) string) [][2]string {
	seen := make(map[[2]string]struct{})
	for _, c := range clusters {
		// Collect distinct module keys across all files in this cluster.
		mods := make(map[string]struct{})
		for _, f := range c.Files {
			if k := key(f); k != "" {
				mods[k] = struct{}{}
			}
		}
		// Emit all cross-module pairs from this cluster.
		keys := make([]string, 0, len(mods))
		for k := range mods {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				pair := [2]string{keys[i], keys[j]}
				seen[pair] = struct{}{}
			}
		}
	}

	out := make([][2]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}
