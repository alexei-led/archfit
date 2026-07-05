// Package initcfg discovers project structure and renders a starter .archfit.yaml.
// It is an adapter (uses toolrun.Runner for go list) and may import os for
// filesystem inspection (DiscoverTS, DiscoverPy).
package initcfg

import (
	"context"
	"fmt"
	"path/filepath"
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

// ModuleEdge is a directed dependency edge between two discovered modules.
// From depends on (imports) To.
type ModuleEdge struct {
	From string // module Name of the dependent
	To   string // module Name of the dependency
}

// DiscoveredConfig holds all discovered modules from Go, TypeScript, and Python.
type DiscoveredConfig struct {
	// ModulePath is the Go module path (e.g. "github.com/alexei-led/archfit").
	ModulePath string
	// Modules are the discovered candidate modules.
	Modules []ModuleDef
	// Layers are the inferred layers in order (innermost to outermost).
	Layers []string
	// Edges are the directed module-level dependency edges, populated when a
	// dependency graph was available at discovery time (go list Imports, cargo
	// metadata resolve). Empty when no graph data was available.
	Edges []ModuleEdge
	// PyPackage is the primary Python top-level package name (e.g. "ccgram").
	PyPackage string
	// HasGo is true when a go.mod was found at root.
	HasGo bool
	// HasPython is true when Python packages were discovered.
	HasPython bool
	// HasTS is true when TypeScript packages were discovered.
	HasTS bool
	// HasRust is true when a Cargo.toml was found at root.
	HasRust bool
}

// toolModeBool renders a language's enabled mode as a YAML boolean. The canonical
// enable vocabulary is true|false|auto; init emits true/false (never on/off).
func toolModeBool(present bool) string {
	if present {
		return "true"
	}
	return "false"
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
	var allEdges []ModuleEdge
	var modPath string
	hasGo := fileExists(filepath.Join(root, "go.mod"))

	if hasGo {
		goMods, goEdges, goModPath, err := discoverGo(ctx, root, runner)
		if err != nil {
			return DiscoveredConfig{}, err
		}
		modPath = goModPath
		allModules = append(allModules, goMods...)
		allEdges = append(allEdges, goEdges...)
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

	// Rust discovery is gated on a root Cargo.toml (the project marker). A missing
	// cargo yields no crate modules but still flips HasRust on, so Render emits
	// tools.rust ready for when cargo is installed; a present-but-failing cargo
	// (broken manifest, parse error) surfaces the error like go list does.
	hasRust := fileExists(filepath.Join(root, markerCargoToml))
	if hasRust {
		rustMods, rustEdges, rerr := DiscoverRust(ctx, root, runner)
		if rerr != nil {
			return DiscoveredConfig{}, rerr
		}
		allModules = append(allModules, rustMods...)
		allEdges = append(allEdges, rustEdges...)
	}

	allModules = disambiguateNames(allModules)

	return DiscoveredConfig{
		ModulePath: modPath,
		Modules:    allModules,
		Layers:     inferLayers(allModules),
		Edges:      allEdges,
		PyPackage:  detectPyPackage(root),
		HasGo:      hasGo,
		HasPython:  len(pyMods) > 0,
		HasTS:      len(tsMods) > 0,
		HasRust:    hasRust,
	}, nil
}

// ModuleAnnotation carries optional LLM-suggested metadata for a module.
// Layer holds the raw LLM layer suggestion; whether it is written live vs as a
// comment is decided in writeModuleStanza based on allowedLayers.
type ModuleAnnotation struct {
	Subdomain     string
	Volatility    string
	Owner         string
	Layer         string
	Role          string
	SuggestedName string
	Rationale     string
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
func writeModuleStanza(b *strings.Builder, name string, m ModuleDef, allowedLayers []string, ann *ModuleAnnotation, apply bool) {
	const indent = "  "

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
		// Nil annotation: write m.Layer only when it is in allowedLayers.
		// For init output cfg.Layers ⊇ every m.Layer, so this is a no-op there.
		// For update AddModule with a mismatched layers: list it prevents a silently
		// out-of-set layer from being written live.
		if m.Layer != "" && allowed[m.Layer] {
			fmt.Fprintf(b, "%s  layer: %s\n", indent, yamlScalar(m.Layer))
		}
		return
	}

	// Write live layer only if resolved layer is in allowedLayers.
	if resolvedLayer != "" && allowed[resolvedLayer] {
		fmt.Fprintf(b, "%s  layer: %s\n", indent, yamlScalar(resolvedLayer))
	}

	if apply {
		// apply mode: write live subdomain/volatility; never rename the module key.
		if ann.Subdomain != "" {
			fmt.Fprintf(b, "%s  subdomain: %s\n", indent, ann.Subdomain)
		}
		if ann.Volatility != "" {
			fmt.Fprintf(b, "%s  volatility: %s\n", indent, ann.Volatility)
		}
		if ann.Owner != "" {
			fmt.Fprintf(b, "%s  owner: %s\n", indent, yamlScalar(ann.Owner))
		}
		if ann.Role != "" {
			fmt.Fprintf(b, "%s  role: %s\n", indent, ann.Role)
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
		if ann.Owner != "" {
			fmt.Fprintf(b, "%s  # owner: %s  # llm-suggested — review and uncomment\n", indent, sanitizeComment(ann.Owner))
		}
		if ann.Role != "" {
			fmt.Fprintf(b, "%s  # role: %s  # llm-suggested — review and uncomment\n", indent, sanitizeComment(ann.Role))
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
	b.WriteString("# Balanced-Coupling advisory tuning.\n")
	b.WriteString("coupling:\n")
	b.WriteString("  # Minimum severity for a coupling advisory: low|medium|high|critical\n")
	b.WriteString("  min_severity: medium\n")
	b.WriteString("  # Clone-only duplicated knowledge: score|advisory (default score)\n")
	b.WriteString("  duplicated_knowledge: score\n\n")

	// languages: section — always emitted so operators can flip modes without
	// needing to know the YAML shape. enabled is true|false|auto.
	b.WriteString("languages:\n")
	for _, lang := range []string{"go", langPython, "typescript", "rust"} {
		var present bool
		switch lang {
		case "go":
			present = cfg.HasGo
		case langPython:
			present = cfg.HasPython
		case "typescript":
			present = cfg.HasTS
		case "rust":
			present = cfg.HasRust
		}
		fmt.Fprintf(&b, "  %s:\n    enabled: %s\n", lang, toolModeBool(present))
		if lang == langPython && cfg.PyPackage != "" {
			fmt.Fprintf(&b, "    package: %s\n", cfg.PyPackage)
		}
	}
	b.WriteString("\n")
	b.WriteString("# Opt-in analyzers (deeper facts; off by default). Uncomment to enable.\n")
	b.WriteString("# analyzers:\n")
	b.WriteString("#   syntax: { enabled: true }       # ast-grep: roles, routes, exported surface\n")
	b.WriteString("#   scip: { enabled: true }         # symbol-level coupling strength\n")
	b.WriteString("#   complexity: { enabled: true }   # cyclomatic complexity hotspots\n")
	b.WriteString("#   clones: { enabled: true }       # cross-module duplication\n")
	b.WriteString("\n")
	b.WriteString("# Off-gate LLM for `archfit enrich` / `explain --llm` (never used by analyze).\n")
	b.WriteString("# ai:\n")
	b.WriteString("#   provider: anthropic   # anthropic | openai | ollama\n")
	b.WriteString("#   model: claude-opus-4-8\n")
	b.WriteString("#   base_url: \"\"          # ollama only\n")
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
			writeModuleStanza(&b, m.Name, m, cfg.Layers, moduleAnn, apply)
		}
		b.WriteString("\n")
	}

	// rules:
	//
	// forbidden_layer_direction is checked by forbiddenLayerDirection.Check,
	// which derives layer ordering from cfg.Layers and endpoint layers from the
	// module map (config.ModuleMap.LayerFor) — it never reads a per-rule
	// from_layer/to_layer, so those keys are not emitted here. Because the check
	// is global (every rule instance re-detects every back-edge in the graph),
	// exactly ONE rule is emitted: a second instance would duplicate each
	// violation under a different rule ID.
	b.WriteString("rules:\n")
	switch {
	case hasCrossLayerEdge(cfg):
		writeLayerRule(&b, "no-layer-back-edges")
	case len(cfg.Layers) >= 2:
		// Layers are assigned but no cross-layer edge was visible at init
		// (Python/TypeScript discovery builds no dependency graph). The rule is
		// live: it checks the real graph at analyze time.
		b.WriteString("  # NOTE: no cross-layer dependency edge was visible at init time; this\n")
		b.WriteString("  # rule checks the real dependency graph at analyze time.\n")
		writeLayerRule(&b, "no-layer-back-edges")
	default:
		b.WriteString("  # NOTE: fewer than two layers were inferred — this rule has nothing to\n")
		b.WriteString("  # check until layers: lists at least two layers and each module is\n")
		b.WriteString("  # assigned a layer: matching one of them.\n")
		writeLayerRule(&b, "no-layer-violations")
	}

	return b.String()
}

// writeLayerRule emits the single forbidden_layer_direction rule stanza.
func writeLayerRule(b *strings.Builder, id string) {
	fmt.Fprintf(b, "  - id: %s\n", id)
	b.WriteString("    type: forbidden_layer_direction\n")
	b.WriteString("    gate: warn\n")
}

// hasCrossLayerEdge reports whether the discovered graph proves the generated
// forbidden_layer_direction rule already has something to check: at least two
// layers and at least one edge between modules in different layers. When false,
// Render picks a NOTE comment by cause: layers assigned but no cross-layer edge
// visible at init (Python/TypeScript discovery builds no graph — the rule still
// checks the real graph at analyze time) vs fewer than two inferred layers (the
// rule has nothing to check until layers are assigned).
func hasCrossLayerEdge(cfg DiscoveredConfig) bool {
	if len(cfg.Edges) == 0 || len(cfg.Layers) < 2 {
		return false
	}

	// moduleLayer maps module name → layer name.
	moduleLayer := make(map[string]string, len(cfg.Modules))
	for _, m := range cfg.Modules {
		if m.Layer != "" {
			moduleLayer[m.Name] = m.Layer
		}
	}

	for _, e := range cfg.Edges {
		fl := moduleLayer[e.From]
		tl := moduleLayer[e.To]
		if fl != "" && tl != "" && fl != tl {
			return true
		}
	}
	return false
}

// yamlKey sanitizes a module name for use as a YAML mapping key.
// Replaces characters that would require quoting.
func yamlKey(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}
