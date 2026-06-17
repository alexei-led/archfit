// Package initcfg discovers project structure and renders a starter .archfit.yaml.
// It is an adapter (uses toolrun.Runner for go list) and may import os for
// filesystem inspection (DiscoverTS, DiscoverPy).
package initcfg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// ModuleDef is a candidate module discovered from the project structure.
type ModuleDef struct {
	// Name is a short human-readable identifier (e.g. "extract", "model").
	Name string
	// Paths contains the glob patterns that own this module's files.
	Paths []string
	// Public contains globs for exported/public surface files.
	Public []string
	// Internal contains globs for package-private files.
	Internal []string
	// Layer is the inferred architectural layer (e.g. "adapter", "core", "cmd").
	Layer string
}

// DiscoveredConfig holds all discovered modules from Go, TypeScript, and Python.
type DiscoveredConfig struct {
	// ModulePath is the Go module path (e.g. "github.com/alexei-led/archfit").
	ModulePath string
	// Modules are the discovered candidate modules.
	Modules []ModuleDef
	// Layers are the inferred layers in order (outermost to innermost).
	Layers []string
	// PyPackage is the primary Python top-level package name (e.g. "ccgram").
	PyPackage string
	// HasGo is true when a go.mod was found at root.
	HasGo bool
	// HasPython is true when Python packages were discovered.
	HasPython bool
	// HasTS is true when TypeScript packages were discovered.
	HasTS bool
}

// Layer name constants used for inference and YAML output.
const (
	layerModel   = "model"
	layerCore    = "core"
	layerAdapter = "adapter"
	layerEngine  = "engine"
	layerCmd     = "cmd"
)

// Python source layout constants.
const (
	pyInitFile   = "__init__.py"
	pySrcDir     = "src"
	pyGlobSuffix = "/**"
)

// Tool mode YAML values used in Render output.
const (
	toolModeOn  = "on"
	toolModeOff = "off"
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

// Discover detects Go, Python, and TypeScript modules at root.
// Go discovery is skipped when no go.mod exists. Python and TypeScript
// discovery run unconditionally (they guard on their own marker files).
//
// Name uniqueness is guaranteed by a two-pass disambiguation applied after
// all language discoverers have run:
//
//  1. First pass — only colliders are touched. For each module name shared by
//     more than one module, every module with that name is renamed to a slug
//     derived from its first Paths glob: strip a trailing "/**", then replace
//     every "/" and "." with "_". Modules whose names are already unique are
//     left completely unchanged.
//
//  2. Second pass — if any slug still collides with another entry (slug or an
//     original name that was never touched), a deterministic numeric suffix is
//     appended ("_2", "_3", …). Suffixes are assigned in ascending order over
//     the slice position so the result is stable across runs.
func Discover(ctx context.Context, root string, runner toolrun.Runner) (DiscoveredConfig, error) {
	var allModules []ModuleDef
	var modPath string
	hasGo := fileExists(filepath.Join(root, "go.mod"))

	if hasGo {
		goMods, goModPath, err := discoverGo(ctx, root, runner)
		if err != nil {
			return DiscoveredConfig{}, err
		}
		modPath = goModPath
		allModules = append(allModules, goMods...)
	}

	pyMods, err := DiscoverPy(root)
	if err != nil {
		return DiscoveredConfig{}, err
	}
	allModules = append(allModules, pyMods...)

	tsMods, err := DiscoverTS(root)
	if err != nil {
		return DiscoveredConfig{}, err
	}
	allModules = append(allModules, tsMods...)

	allModules = disambiguateNames(allModules)

	return DiscoveredConfig{
		ModulePath: modPath,
		Modules:    allModules,
		Layers:     inferLayers(allModules),
		PyPackage:  detectPyPackage(root),
		HasGo:      hasGo,
		HasPython:  len(pyMods) > 0,
		HasTS:      len(tsMods) > 0,
	}, nil
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

// DiscoverTS reads src/ or lib/ subdirectories if package.json is present at root.
// Returns discovered module definitions, one per subdirectory.
func DiscoverTS(root string) ([]ModuleDef, error) {
	if !fileExists(filepath.Join(root, "package.json")) {
		return nil, nil
	}
	return discoverSubdirs(root, []string{"src", "lib"})
}

// DiscoverPy reads Python packages from root and root/src (src-layout).
// A Python project is detected by pyproject.toml or setup.py at root.
// For each top-level package found, sub-packages are returned as individual
// modules. If a top-level package has no sub-packages it is returned itself.
func DiscoverPy(root string) ([]ModuleDef, error) {
	hasPyProject := fileExists(filepath.Join(root, "pyproject.toml"))
	hasSetupPy := fileExists(filepath.Join(root, "setup.py"))
	if !hasPyProject && !hasSetupPy {
		return nil, nil
	}

	type scanTarget struct {
		dir    string
		prefix string
	}
	targets := []scanTarget{
		{dir: root, prefix: ""},
		{dir: filepath.Join(root, pySrcDir), prefix: pySrcDir + "/"},
	}

	var mods []ModuleDef
	for _, t := range targets {
		entries, err := os.ReadDir(t.dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("initcfg: read dir %s: %w", t.dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			pkgDir := filepath.Join(t.dir, e.Name())
			if !fileExists(filepath.Join(pkgDir, pyInitFile)) {
				continue
			}
			// Found a top-level Python package — enumerate sub-packages.
			subMods := discoverPySubpackages(pkgDir, t.prefix+e.Name()+"/")
			if len(subMods) > 0 {
				mods = append(mods, subMods...)
			} else {
				// No sub-packages: return the top-level package as a single module.
				mod := pyDottedModule(t.prefix + e.Name())
				mods = append(mods, ModuleDef{
					Name:  e.Name(),
					Paths: pyModulePaths(mod),
					Layer: layerCore,
				})
			}
		}
	}
	return mods, nil
}

// discoverPySubpackages scans immediate subdirectories of pkgDir that contain
// __init__.py and returns one ModuleDef per sub-package.
// pathPrefix is the path prefix for glob construction, e.g. "src/ccgram/".
func discoverPySubpackages(pkgDir, pathPrefix string) []ModuleDef {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	var mods []ModuleDef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		if !fileExists(filepath.Join(pkgDir, e.Name(), pyInitFile)) {
			continue
		}
		mod := pyDottedModule(pathPrefix + e.Name())
		mods = append(mods, ModuleDef{
			Name:  e.Name(),
			Paths: pyModulePaths(mod),
			Layer: inferPyLayer(e.Name()),
		})
	}
	return mods
}

// pyDottedModule converts a slash path (e.g. "src/ccgram/handlers" or
// "ccgram/handlers") to a dotted Python module path ("ccgram.handlers"). Python
// graph nodes are dotted module names, so paths: globs must be dotted too.
func pyDottedModule(slashPath string) string {
	s := strings.TrimPrefix(slashPath, pySrcDir+"/")
	return strings.ReplaceAll(strings.Trim(s, "/"), "/", ".")
}

// pyModulePaths returns the paths globs for a dotted Python module: the package
// itself and its submodules ("ccgram.handlers" + "ccgram.handlers.*").
func pyModulePaths(mod string) []string {
	return []string{mod, mod + ".*"}
}

// inferPyLayer maps common Python sub-package names to architectural layers.
func inferPyLayer(name string) string {
	switch name {
	case "handlers", "api", "routes", "views", "providers":
		return layerAdapter
	case "model", "models", "types", "schema":
		return layerModel
	case layerCmd, "cli":
		return layerCmd
	default:
		return layerCore
	}
}

// detectPyPackage scans root then root/src for the first directory that
// contains __init__.py (not starting with '.' or '_'). Returns its name.
func detectPyPackage(root string) string {
	for _, dir := range []string{root, filepath.Join(root, pySrcDir)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			if fileExists(filepath.Join(dir, e.Name(), pyInitFile)) {
				return e.Name()
			}
		}
	}
	return ""
}

// ModuleAnnotation carries optional LLM-suggested metadata for a module.
// Layer holds the raw LLM layer suggestion; whether it is written live vs as a
// comment is decided in writeModuleStanza based on allowedLayers.
type ModuleAnnotation struct {
	Subdomain     string
	Volatility    string
	Layer         string
	SuggestedName string
}

// sanitizeComment strips or replaces control characters (< 0x20 and DEL 0x7F),
// trims surrounding whitespace, and caps the result at 200 runes.
// Use this for every dynamic string rendered into a YAML comment to prevent
// newline injection.
func sanitizeComment(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			buf.WriteRune(' ')
		} else {
			buf.WriteRune(r)
		}
	}
	result := strings.TrimSpace(buf.String())
	runes := []rune(result)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	return string(runes)
}

// writeModuleStanza writes one module entry into b.
// indent is the leading whitespace (e.g. "  ").
// allowedLayers is the sole authority for whether a layer value is written live.
// When ann is nil the output is byte-identical to the original Render inline code.
func writeModuleStanza(b *strings.Builder, indent, name string, m ModuleDef, allowedLayers []string, ann *ModuleAnnotation, apply bool) { //nolint:unparam // indent is intentional API; callers may pass different values
	allowed := make(map[string]bool, len(allowedLayers))
	for _, l := range allowedLayers {
		allowed[l] = true
	}

	fmt.Fprintf(b, "%s%s:\n", indent, yamlKey(name))
	fmt.Fprintf(b, "%s  paths:\n", indent)
	for _, p := range m.Paths {
		fmt.Fprintf(b, "%s    - %q\n", indent, p)
	}
	if len(m.Public) > 0 {
		fmt.Fprintf(b, "%s  public:\n", indent)
		for _, p := range m.Public {
			fmt.Fprintf(b, "%s    - %q\n", indent, p)
		}
	}
	if len(m.Internal) > 0 {
		fmt.Fprintf(b, "%s  internal:\n", indent)
		for _, p := range m.Internal {
			fmt.Fprintf(b, "%s    - %q\n", indent, p)
		}
	}

	// Resolve the layer: prefer ann.Layer when it's in allowedLayers, else m.Layer.
	resolvedLayer := m.Layer
	if ann != nil && ann.Layer != "" && allowed[ann.Layer] {
		resolvedLayer = ann.Layer
	}

	if ann == nil {
		// Nil annotation: byte-identical to original output.
		if m.Layer != "" {
			fmt.Fprintf(b, "%s  layer: %s\n", indent, m.Layer)
		}
		return
	}

	// Write live layer only if resolved layer is in allowedLayers.
	if resolvedLayer != "" && allowed[resolvedLayer] {
		fmt.Fprintf(b, "%s  layer: %s\n", indent, resolvedLayer)
	}

	if apply {
		// apply mode: write live subdomain/volatility; never rename the module key.
		if ann.Subdomain != "" {
			fmt.Fprintf(b, "%s  subdomain: %s\n", indent, ann.Subdomain)
		}
		if ann.Volatility != "" {
			fmt.Fprintf(b, "%s  volatility: %s\n", indent, ann.Volatility)
		}
		// Emit layer suggestion comment if ann.Layer was out of set.
		if ann.Layer != "" && !allowed[ann.Layer] {
			fmt.Fprintf(b, "%s  # llm layer: %s  # not in layers: — review\n", indent, sanitizeComment(ann.Layer))
		}
	} else {
		// plan mode: everything as comments.
		if ann.Subdomain != "" {
			fmt.Fprintf(b, "%s  # subdomain: %s  # llm-suggested — review and uncomment\n", indent, sanitizeComment(ann.Subdomain))
		}
		if ann.Volatility != "" {
			fmt.Fprintf(b, "%s  # volatility: %s  # llm-suggested — review and uncomment\n", indent, sanitizeComment(ann.Volatility))
		}
		// Layer suggestion comment.
		if ann.Layer != "" {
			if allowed[ann.Layer] {
				fmt.Fprintf(b, "%s  # llm layer: %s\n", indent, sanitizeComment(ann.Layer))
			} else {
				fmt.Fprintf(b, "%s  # llm layer: %s  # not in layers: — review\n", indent, sanitizeComment(ann.Layer))
			}
		}
		// Rename suggestion.
		if ann.SuggestedName != "" && ann.SuggestedName != name {
			fmt.Fprintf(b, "%s  # llm: consider renaming to %q\n", indent, sanitizeComment(ann.SuggestedName))
		}
	}
}

// Render converts a DiscoveredConfig into a YAML string suitable for saving as
// .archfit.yaml. The output includes a TODO comment and uses only known config
// fields so it round-trips through config.Load.
//
// ann maps module names to LLM annotations. When ann is nil the output is
// byte-identical to the pre-annotation Render. apply controls plan vs live mode:
// false = comment-only suggestions; true = write live fields.
func Render(cfg DiscoveredConfig, ann map[string]ModuleAnnotation, apply bool) string {
	var b strings.Builder

	b.WriteString("# Generated by archfit init — TODO: review and promote gate: warn to gate: fail\n")
	b.WriteString("version: 1\n\n")
	b.WriteString("# Minimum severity for BC coupling advisories: low|medium|high|critical\n")
	b.WriteString("bc_advisory_min_severity: medium\n\n")

	if cfg.PyPackage != "" {
		fmt.Fprintf(&b, "python_package: %s\n\n", cfg.PyPackage)
	}

	// tools: section — always emitted so operators can flip modes without
	// needing to know the YAML shape.
	b.WriteString("tools:\n")
	for _, lang := range []string{"go", "python", "typescript"} {
		var mode string
		switch lang {
		case "go":
			if cfg.HasGo {
				mode = toolModeOn
			} else {
				mode = toolModeOff
			}
		case "python":
			if cfg.HasPython {
				mode = toolModeOn
			} else {
				mode = toolModeOff
			}
		case "typescript":
			if cfg.HasTS {
				mode = toolModeOn
			} else {
				mode = toolModeOff
			}
		}
		fmt.Fprintf(&b, "  %s:\n    enabled: %q\n", lang, mode)
	}
	b.WriteString("  # Off-gate LLM for `archfit enrich` / `explain --llm` (never used by check).\n")
	b.WriteString("  # llm:\n")
	b.WriteString("  #   provider: anthropic   # anthropic | openai | ollama\n")
	b.WriteString("  #   model: claude-opus-4-8\n")
	b.WriteString("  #   base_url: \"\"          # ollama only\n")
	b.WriteString("\n")

	// layers:
	if len(cfg.Layers) > 0 {
		b.WriteString("layers:\n")
		for _, l := range cfg.Layers {
			fmt.Fprintf(&b, "  - %s\n", l)
		}
		b.WriteString("\n")
	}

	// modules:
	if len(cfg.Modules) > 0 {
		b.WriteString("modules:\n")
		for _, m := range cfg.Modules {
			var moduleAnn *ModuleAnnotation
			if ann != nil {
				if a, ok := ann[m.Name]; ok {
					moduleAnn = &a
				}
			}
			writeModuleStanza(&b, "  ", m.Name, m, cfg.Layers, moduleAnn, apply)
		}
		b.WriteString("\n")
	}

	// rules:
	b.WriteString("rules:\n")
	b.WriteString("  - id: no-forbidden-deps\n")
	b.WriteString("    type: forbidden_dependency\n")
	b.WriteString("    gate: warn\n")

	return b.String()
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

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
			path := dir + "/" + name + pyGlobSuffix
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

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// yamlKey sanitizes a module name for use as a YAML mapping key.
// Replaces characters that would require quoting.
func yamlKey(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}
