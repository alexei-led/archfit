package initcfg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// adapterExtract is the second-segment name for the extract adapter packages.
const adapterExtract = "extract"

// goListPkg mirrors the subset of `go list -json` output that we need.
type goListPkg struct {
	ImportPath string
	Dir        string
	Module     *struct {
		Path string
	}
}

// discoverGo runs `go list -json ./...` from root and groups packages into
// candidate modules. Returns modules, module path, and any error.
func discoverGo(ctx context.Context, root string, runner toolrun.Runner) ([]ModuleDef, string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    "go",
		Args:    []string{"list", "-json", "./..."},
		WorkDir: root,
	})
	if err != nil {
		return nil, "", fmt.Errorf("initcfg: go list: %w", err)
	}
	if out.ExitCode != 0 {
		return nil, "", fmt.Errorf("initcfg: go list exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	var modPath string
	// segments groups import path segments → set of full paths seen.
	// Key is "seg1/seg2" (first 2 path segments after module root).
	segments := make(map[string][]string)

	dec := json.NewDecoder(bytes.NewReader(out.Stdout))
	for {
		var pkg goListPkg
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", fmt.Errorf("initcfg: parse go list output: %w", err)
		}
		if pkg.Module != nil && pkg.Module.Path != "" && modPath == "" {
			modPath = pkg.Module.Path
		}
		rel := stripPrefix(pkg.ImportPath, modPath)
		if rel == "" {
			// Root package — record as a top-level module.
			rel = "."
		}
		key := groupKey(rel)
		segments[key] = append(segments[key], rel)
	}

	return buildGoModules(segments), modPath, nil
}

// stripPrefix removes the module path prefix from an import path.
// e.g. "github.com/foo/bar/pkg/a" with modPath "github.com/foo/bar" → "pkg/a".
func stripPrefix(importPath, modPath string) string {
	if modPath == "" {
		return importPath
	}
	trimmed := strings.TrimPrefix(importPath, modPath)
	return strings.TrimPrefix(trimmed, "/")
}

// groupKey returns the first 2 path segments of a relative package path.
// e.g. "internal/extract/golang" → "internal/extract"
// e.g. "cmd/archfit" → "cmd/archfit"
// e.g. "." → "."
func groupKey(rel string) string {
	if rel == "." || rel == "" {
		return "."
	}
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// buildGoModules converts the segments map into sorted ModuleDef slice.
func buildGoModules(segments map[string][]string) []ModuleDef {
	// Sort keys for determinism.
	keys := make([]string, 0, len(segments))
	for k := range segments {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var mods []ModuleDef
	for _, key := range keys {
		if key == "." {
			// Skip the root package as a standalone module entry.
			continue
		}
		name := moduleNameFromKey(key)
		// Doublestar glob (classify/extractor node paths use "/"-separated package
		// and file paths). "key/**" matches the package node "key" and its files.
		// NOT the go-list "key/..." form, which doublestar does not match.
		paths := []string{key + "/**"}
		layer := inferLayerFromKey(key)

		// Public is the importable package path itself (an import targets the package
		// node "key"). Go cross-package imports go through exported APIs — the compiler
		// forbids importing unexported symbols — so they are contract coupling, not
		// intrusive. (Go's `internal/` is module-visibility, NOT BC-intrusive; do not
		// mark it internal here, or normal shared code reads as a false leak.)
		public := []string{key}
		var internal []string

		mods = append(mods, ModuleDef{
			Name:     name,
			Paths:    paths,
			Public:   public,
			Internal: internal,
			Layer:    layer,
		})
	}
	return mods
}

// moduleNameFromKey produces a short name from a 2-segment path key.
// "internal/extract" → "extract", "cmd/archfit" → "cmd_archfit"
func moduleNameFromKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) == 2 && parts[0] == "internal" {
		return parts[1]
	}
	return strings.Join(parts, "_")
}

// inferLayerFromKey maps common Go path prefixes to architectural layer names.
func inferLayerFromKey(key string) string {
	parts := strings.Split(key, "/")
	top := parts[0]
	switch top {
	case layerCmd:
		return layerCmd
	case "internal":
		second := ""
		if len(parts) >= 2 {
			second = parts[1]
		}
		switch second {
		case layerModel:
			return layerModel
		case "toolrun", adapterExtract, "output", "history", "initcfg":
			return layerAdapter
		case layerEngine:
			return layerEngine
		default:
			return layerCore
		}
	default:
		return layerCore
	}
}
