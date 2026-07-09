package initcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tsGlobSuffix is appended to a discovered subdirectory to form its paths glob.
const tsGlobSuffix = "/**"

// DiscoverTS discovers TypeScript/JavaScript modules from a project root.
//
// Discovery order (first match wins):
//  1. npm/yarn/bun workspaces: read the "workspaces" field (array or {packages:…}
//     shape) from package.json. Each entry is treated as a literal directory or
//     as a trailing-/* glob (e.g. "code/addons/*"); full filepath.Glob patterns
//     are not supported. Each matched directory becomes one module. This handles
//     monorepos like storybook (code/addons/*, code/lib/*, …).
//  2. Flat src/ or lib/ layout: enumerate immediate subdirectories of src/ or
//     lib/ if they exist. This handles single-package repos and simple monorepos.
//
// Returns nil when no package.json is found (not a TS/JS project).
func DiscoverTS(root string) ([]ModuleDef, error) {
	pkgJSON := filepath.Join(root, "package.json")
	if !fileExists(pkgJSON) {
		return nil, nil
	}

	// Try workspace-based discovery first.
	if wsMods := discoverTSWorkspaces(root, pkgJSON); len(wsMods) > 0 {
		return wsMods, nil
	}

	// Fallback: flat src/ or lib/ subdirectory layout.
	return discoverSubdirs(root, []string{"src", "lib"})
}

// discoverTSWorkspaces reads the "workspaces" field from package.json.
// It supports both the array form (["packages/*"]) and the object form
// ({"packages": ["packages/*"]}). Each pattern is resolved under root;
// matching directories become one ModuleDef each.
// Returns nil when no workspaces field is present or no directories match.
func discoverTSWorkspaces(root, pkgJSON string) []ModuleDef {
	data, err := os.ReadFile(pkgJSON) //nolint:gosec // root chosen by discovery
	if err != nil {
		return nil // unreadable → fall through to subdirectory scan
	}

	var raw struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Workspaces == nil {
		return nil
	}

	// workspaces can be []string or {"packages": []string}.
	patterns := extractWorkspacePatterns(raw.Workspaces)
	if len(patterns) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var mods []ModuleDef
	for _, pattern := range patterns {
		dirs, _ := expandWorkspacePattern(root, pattern) // best-effort; ignore errors
		for _, dir := range dirs {
			rel, rerr := filepath.Rel(root, dir)
			if rerr != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if seen[rel] {
				continue
			}
			seen[rel] = true

			// Use the last path segment as the module name, sanitised.
			name := sanitiseTSModuleName(filepath.Base(rel))
			path := rel + tsGlobSuffix
			mods = append(mods, ModuleDef{
				Name:   name,
				Paths:  []string{path},
				Public: []string{path},
				Layer:  layerCore,
			})
		}
	}
	return mods
}

// extractWorkspacePatterns normalises the two npm workspace shapes:
// - array:  ["packages/*", "apps/*"]
// - object: {"packages": ["packages/*", "apps/*"]}
func extractWorkspacePatterns(raw json.RawMessage) []string {
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Packages
	}
	return nil
}

// expandWorkspacePattern resolves a workspace glob pattern under root.
// For patterns ending in "/*" it returns all immediate subdirectories of
// the parent. For exact paths it returns the single directory if it exists.
func expandWorkspacePattern(root, pattern string) ([]string, error) {
	if strings.HasSuffix(pattern, "/*") {
		parent := filepath.Join(root, strings.TrimSuffix(pattern, "/*"))
		entries, err := os.ReadDir(parent)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		var dirs []string
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dirs = append(dirs, filepath.Join(parent, e.Name()))
		}
		return dirs, nil
	}
	// Exact path: include if it's a directory.
	full := filepath.Join(root, pattern)
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	return []string{full}, nil
}

// sanitiseTSModuleName converts a directory name to a valid YAML key.
// Strips leading @-scope, replaces hyphens and slashes with underscores.
func sanitiseTSModuleName(name string) string {
	name = strings.TrimPrefix(name, "@")
	return strings.NewReplacer("-", "_", "/", "_", ".", "_").Replace(name)
}

// discoverSubdirs reads subdirectories within dirNames inside root.
// Returns one ModuleDef per subdirectory found.
func discoverSubdirs(root string, dirNames []string) ([]ModuleDef, error) {
	var mods []ModuleDef
	for _, dir := range dirNames {
		full := filepath.Join(root, dir)
		entries, err := os.ReadDir(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("initcfg: read dir %s: %w", full, err)
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			name := e.Name()
			path := dir + "/" + name + tsGlobSuffix
			mods = append(mods, ModuleDef{
				Name:  name,
				Paths: []string{path},
				// TS/JS cross-file imports go through module exports (you cannot import
				// a non-exported binding), so they are contract coupling. Mark the
				// module's files public; SCIP-typescript can refine this when enabled.
				Public: []string{path},
				Layer:  layerCore,
			})
		}
	}
	return mods, nil
}
