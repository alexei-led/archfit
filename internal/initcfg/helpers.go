package initcfg

import (
	"fmt"
	"os"
	"strings"
)

// Layer name constants used for inference and YAML output.
const (
	layerModel   = "model"
	layerCore    = "core"
	layerAdapter = "adapter"
	layerEngine  = "engine"
	layerCmd     = "cmd"
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

// inferLayers derives an ordered, deduplicated layer list from discovered modules.
// Canonical order: model → core → adapter → engine → cmd.
func inferLayers(mods []ModuleDef) []string {
	order := []string{layerModel, layerCore, layerAdapter, layerEngine, layerCmd}
	seen := make(map[string]bool)
	for _, m := range mods {
		if m.Layer != "" {
			seen[m.Layer] = true
		}
	}
	var layers []string
	for _, l := range order {
		if seen[l] {
			layers = append(layers, l)
		}
	}
	return layers
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
