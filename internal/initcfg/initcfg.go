// Package initcfg discovers project structure and renders a starter .archfit.yaml.
// It is an adapter (uses toolrun.Runner for go list) and may import os for
// filesystem inspection (DiscoverTS, DiscoverPy).
package initcfg

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
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
	// Sources are the graph-node paths discovery found inside the module (Go
	// package directories, Python dotted packages and modules). config update
	// uses them to tell a module the configured map already owns from a
	// genuinely new one.
	// Empty when the discoverer does not enumerate sources (TypeScript, Rust):
	// such a module is matched by name only.
	Sources []string
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
	// HasGo, HasPython, HasTS and HasRust are the extractor registry's
	// applicability answers (Presence), not discovery results: a language is
	// present when its extractor would analyse it, even if discovery found no
	// module to propose.
	HasGo     bool
	HasPython bool
	HasTS     bool
	HasRust   bool
	// ImportGraphComplete is true when Edges is the module graph check will
	// judge the starter rules over (see graphGap). Render gates a starter rule
	// on fail only when this graph shows it clean.
	ImportGraphComplete bool
	// GraphGap says why the graph is not complete; Render writes it beside
	// gate: warn. Empty when ImportGraphComplete is true.
	GraphGap string
}

// Presence is the extractor registry's answer to "is this language present
// under root?", computed by the composition root with each extractor's own
// applicability function (registry.ProjectPresent). Discovery never decides
// presence from its own marker files: a marker list that disagrees with the
// extractor writes `enabled: false` over a language analysis would measure.
type Presence struct {
	Go         bool
	TypeScript bool
	Python     bool
	Rust       bool
	// GoMembers are the absolute Go module directories the Go extractor loads
	// (go.work members, a root go.mod, or nested go.mod dirs).
	GoMembers []string
	// GoWorkOff is true when Go-toolchain subprocesses must ignore the go.work
	// governing root (see registry.GoWorkOff).
	GoWorkOff bool
	// Sources is the rule-scope source inventory of root, read the way config
	// lint reads it. Discovery keeps only modules that own production source
	// in it. nil means the caller supplied none (discovery unit tests); the
	// composition root always supplies it.
	Sources []SourceFile
}

// languageMode renders a language's enabled mode. A present language is
// enabled explicitly; an absent one stays at the config default `auto`, so a
// presence probe that missed a project can never switch its analysis off.
// Rust is `auto` even when present: missing cargo then reports a coverage gap
// instead of failing the run.
func languageMode(lang string, present bool) string {
	if present && lang != langRust {
		return "true"
	}
	return "auto"
}

// Discover detects Go, Python, TypeScript, and Rust modules at root.
// presence decides which languages are present; it comes from the extractor
// registry, so discovery and analysis agree on what is in the tree. Go
// discovery runs `go list` in every member presence names (a go.work
// monorepo with no root go.mod included). Python and TypeScript discovery
// still find their own module candidates; Rust discovery reads the root
// Cargo.toml it has always used.
//
// Layers are kept only when discovery assigns at least two of them: a single
// layer orders nothing, and Render then asks the owner to declare layers.
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
func Discover(ctx context.Context, root string, runner toolrun.Runner, presence Presence) (DiscoveredConfig, error) {
	var allModules []ModuleDef
	// origins records the discoverer of each module in allModules.
	var origins []string
	var allEdges []ModuleEdge
	var modPath string
	add := func(lang string, mods []ModuleDef) {
		allModules = append(allModules, mods...)
		for range mods {
			origins = append(origins, lang)
		}
	}

	if presence.Go && len(presence.GoMembers) > 0 {
		goMods, goEdges, goModPath, err := discoverGo(ctx, root, runner, presence.GoMembers, presence.GoWorkOff)
		if err != nil {
			return DiscoveredConfig{}, err
		}
		modPath = goModPath
		add(langGo, goMods)
		allEdges = append(allEdges, goEdges...)
	}

	pyMods, err := DiscoverPy(root)
	if err != nil {
		return DiscoveredConfig{}, err
	}
	add(langPython, pyMods)

	tsMods, err := DiscoverTS(root)
	if err != nil {
		return DiscoveredConfig{}, err
	}
	add(langTypeScript, tsMods)
	judged := len(allModules)

	// Rust discovery runs `cargo metadata` at root, so it still needs the root
	// Cargo.toml; a configured sub-crate manifest makes Rust present without
	// giving this discoverer a manifest to read. A missing cargo yields no crate
	// modules; a present-but-failing cargo (broken manifest, parse error)
	// surfaces the error like go list does.
	if presence.Rust && fileExists(filepath.Join(root, markerCargoToml)) {
		rustMods, rustEdges, rerr := DiscoverRust(ctx, root, runner)
		if rerr != nil {
			return DiscoveredConfig{}, rerr
		}
		add(langRust, rustMods)
		allEdges = append(allEdges, rustEdges...)
	}

	if presence.Sources != nil {
		discovered := allModules
		allModules, origins = keepModulesWithSource(allModules, origins, judged, presence.Sources)
		allEdges = foldDroppedEdges(allEdges, discovered, allModules)
	}
	allModules = disambiguateNames(allModules)
	// Only Rust discovery assigns layers, from the crate dependency graph; a
	// single layer orders nothing.
	layers := inferLayers(allModules)
	if len(layers) < 2 {
		layers = nil
		for i := range allModules {
			allModules[i].Layer = ""
		}
	}

	gap := graphGap(allModules, origins, presence.Rust, presence.Sources)
	return DiscoveredConfig{
		ModulePath:          modPath,
		Modules:             allModules,
		Layers:              layers,
		Edges:               allEdges,
		PyPackage:           detectPyPackage(root),
		HasGo:               presence.Go,
		HasPython:           presence.Python,
		HasTS:               presence.TypeScript,
		HasRust:             presence.Rust,
		ImportGraphComplete: gap == "",
		GraphGap:            gap,
	}, nil
}

// Draft basis values distinguish deterministic facts from semantic judgments in
// LLM draft metadata.
const (
	DraftBasisDeterministicFact = "deterministic_fact"
	DraftBasisSemanticJudgment  = "semantic_judgment"
)

// RuleSuggestion is one review-only LLM proposal for a deterministic config rule
// or coupling gate tuning. It is rendered for humans and never applied by plan or
// update modes.
type RuleSuggestion struct {
	SourceModule string
	ID           string
	Type         string
	Gate         string
	From         string
	To           string
	Max          *int
	Mode         string
	MaxNewSeams  *int
	Rationale    string
	EvidenceRefs []string
	Basis        string
}

// ExternalSystemSuggestion is one review-only LLM proposal for an
// external_systems entry. It is rendered for humans and never applied by plan or
// update modes.
type ExternalSystemSuggestion struct {
	SourceModule string
	Name         string
	Targets      []string
	Volatility   string
	Rationale    string
	EvidenceRefs []string
	Basis        string
}

// ModuleAnnotation carries optional LLM-suggested metadata for a module.
// Layer holds the raw LLM layer suggestion; whether it is written live vs as a
// comment is decided in writeModuleStanza based on allowedLayers.
type ModuleAnnotation struct {
	Subdomain                 string
	Volatility                string
	Owner                     string
	Layer                     string
	Role                      string
	SuggestedName             string
	Rationale                 string
	EvidenceRefs              []string
	Basis                     string
	RuleSuggestions           []RuleSuggestion
	ExternalSystemSuggestions []ExternalSystemSuggestion
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

// TargetSchemaVersion is the config schema version emitted by config init.
const TargetSchemaVersion = 2

// Render converts a DiscoveredConfig into a YAML string suitable for saving as
// .archfit.yaml. The output uses only known config fields so it round-trips
// through config.Load.
//
// ann maps module names to LLM annotations. When ann is nil the output is
// byte-identical to the pre-annotation Render. apply controls plan vs live mode:
// false = comment-only suggestions; true = write live fields.
func Render(cfg DiscoveredConfig, ann map[string]ModuleAnnotation, apply bool) string {
	var b strings.Builder

	b.WriteString("# Generated by archfit config init. Review module names and paths, then read the\n")
	b.WriteString("# rules: section: each starter rule says what it blocks and when its gate is fail.\n")
	b.WriteString("# Next: archfit config lint · archfit check · archfit baseline\n")
	// Keep config init aligned with the schema accepted by config.Load.
	b.WriteString("version: " + strconv.Itoa(TargetSchemaVersion) + "\n\n")
	b.WriteString("# Balanced-Coupling advisory tuning.\n")
	b.WriteString("coupling:\n")
	b.WriteString("  # Minimum severity for a coupling advisory: low|medium|high|critical\n")
	b.WriteString("  min_severity: medium\n")
	b.WriteString("  # Clone-only duplicated knowledge: score|advisory (default score)\n")
	b.WriteString("  duplicated_knowledge: score\n\n")

	// languages: section — always emitted so operators can flip modes without
	// needing to know the YAML shape. enabled is true|false|auto.
	b.WriteString("languages:\n")
	for _, lang := range []string{langGo, langPython, langTypeScript, langRust} {
		var present bool
		switch lang {
		case langGo:
			present = cfg.HasGo
		case langPython:
			present = cfg.HasPython
		case langTypeScript:
			present = cfg.HasTS
		case langRust:
			present = cfg.HasRust
		}
		fmt.Fprintf(&b, "  %s:\n    enabled: %s\n", lang, languageMode(lang, present))
		if lang == langPython && cfg.PyPackage != "" {
			fmt.Fprintf(&b, "    package: %s\n", cfg.PyPackage)
		}
	}
	b.WriteString("\n")
	if cfg.HasRust {
		b.WriteString("# Rust deep analysis: explicit so single-crate Rust does not degenerate to one crate node.\n")
		b.WriteString("analyzers:\n")
		b.WriteString("  cargo_modules:\n")
		b.WriteString("    enabled: true\n")
		b.WriteString("  scip:\n")
		b.WriteString("    enabled: true\n")
		b.WriteString("\n")
	}
	b.WriteString("# Optional analyzers (deeper facts; opt in deliberately because they can be slow).\n")
	if cfg.HasRust {
		// A live analyzers: block (cargo_modules + scip) already exists above. Emitting
		// a second "# analyzers:" header here would trap a user who uncomments it into a
		// duplicate top-level key (a hard config-load error). Point at the block above.
		b.WriteString("# Extend the analyzers: block above with:\n")
	} else {
		b.WriteString("# analyzers:\n")
		b.WriteString("#   scip: { enabled: true }         # symbol-level coupling strength\n")
	}
	b.WriteString("#   syntax: { enabled: true }       # ast-grep: roles, routes, exported surface\n")
	b.WriteString("#   clones: { enabled: true }       # cross-module duplication\n")
	b.WriteString("\n")
	b.WriteString("# Off-gate LLM for config init/update/enrich and analyze/explain --ai-summary.\n")
	b.WriteString("# The deterministic gate never uses it.\n")
	b.WriteString("# ai:\n")
	b.WriteString("#   provider: anthropic   # anthropic | openai | ollama\n")
	b.WriteString("#   model: claude-opus-4-8\n")
	b.WriteString("#   base_url: \"\"          # ollama only\n")
	b.WriteString("\n")

	// layers:
	if len(cfg.Layers) > 0 {
		b.WriteString("# layers: innermost first; a module may import its own layer or an earlier one.\n")
		b.WriteString("layers:\n")
		for _, l := range cfg.Layers {
			fmt.Fprintf(&b, "  - %s\n", l)
		}
		b.WriteString("\n")
	} else {
		writeLayersHowTo(&b)
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
			writeModuleAnnotationComments(&b, moduleAnn)
		}
		b.WriteString("\n")
	}

	writeExternalSystemSuggestionComments(&b, ann)

	b.WriteString("rules:\n")
	writeStarterRules(&b, cfg)
	writeRuleSuggestionComments(&b, ann)

	return b.String()
}

func writeExternalSystemSuggestionComments(b *strings.Builder, ann map[string]ModuleAnnotation) {
	suggestions := annotationExternalSystemSuggestions(ann)
	if len(suggestions) == 0 {
		return
	}
	b.WriteString("# LLM external_systems suggestions (review-only; copy targets/volatility after review):\n")
	b.WriteString("# external_systems:\n")
	for _, s := range suggestions {
		fmt.Fprintf(b, "#   %s:\n", sanitizeComment(yamlKey(s.Name)))
		if s.SourceModule != "" {
			fmt.Fprintf(b, "#     source_module: %s\n", sanitizeComment(s.SourceModule))
		}
		b.WriteString("#     targets:\n")
		for _, target := range s.Targets {
			fmt.Fprintf(b, "#       - %q\n", target)
		}
		if s.Volatility != "" {
			fmt.Fprintf(b, "#     volatility: %s\n", yamlScalar(s.Volatility))
		}
		if s.Basis != "" {
			fmt.Fprintf(b, "#     basis: %s\n", sanitizeComment(s.Basis))
		}
		if len(s.EvidenceRefs) > 0 {
			fmt.Fprintf(b, "#     evidence_refs: %s\n", joinEvidenceRefs(s.EvidenceRefs))
		}
		if s.Rationale != "" {
			fmt.Fprintf(b, "#     rationale: %s\n", sanitizeComment(s.Rationale))
		}
	}
	b.WriteString("\n")
}

func writeRuleSuggestionComments(b *strings.Builder, ann map[string]ModuleAnnotation) {
	suggestions := annotationRuleSuggestions(ann)
	if len(suggestions) == 0 {
		return
	}
	b.WriteString("  # LLM rule suggestions (review-only; copy into rules/coupling after review):\n")
	for _, s := range suggestions {
		fmt.Fprintf(b, "  # - type: %s\n", sanitizeComment(s.Type))
		if s.ID != "" {
			fmt.Fprintf(b, "  #   id: %s\n", sanitizeComment(s.ID))
		}
		if s.SourceModule != "" {
			fmt.Fprintf(b, "  #   source_module: %s\n", sanitizeComment(s.SourceModule))
		}
		if s.Gate != "" {
			fmt.Fprintf(b, "  #   gate: %s\n", sanitizeComment(s.Gate))
		}
		if s.From != "" {
			fmt.Fprintf(b, "  #   from: %s\n", sanitizeComment(s.From))
		}
		if s.To != "" {
			fmt.Fprintf(b, "  #   to: %s\n", sanitizeComment(s.To))
		}
		if s.Max != nil {
			fmt.Fprintf(b, "  #   max: %d\n", *s.Max)
		}
		if s.Mode != "" {
			fmt.Fprintf(b, "  #   mode: %s\n", sanitizeComment(s.Mode))
		}
		if s.MaxNewSeams != nil {
			fmt.Fprintf(b, "  #   max_new_seams: %d\n", *s.MaxNewSeams)
		}
		if s.Basis != "" {
			fmt.Fprintf(b, "  #   basis: %s\n", sanitizeComment(s.Basis))
		}
		if len(s.EvidenceRefs) > 0 {
			fmt.Fprintf(b, "  #   evidence_refs: %s\n", joinEvidenceRefs(s.EvidenceRefs))
		}
		if s.Rationale != "" {
			fmt.Fprintf(b, "  #   rationale: %s\n", sanitizeComment(s.Rationale))
		}
	}
}

func annotationRuleSuggestions(ann map[string]ModuleAnnotation) []RuleSuggestion {
	if len(ann) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []RuleSuggestion
	for module, a := range ann {
		for _, s := range a.RuleSuggestions {
			if s.SourceModule == "" {
				s.SourceModule = module
			}
			key := strings.Join([]string{s.Type, s.ID, s.From, s.To, s.SourceModule}, "\x00")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].SourceModule < out[j].SourceModule
	})
	return out
}

func annotationExternalSystemSuggestions(ann map[string]ModuleAnnotation) []ExternalSystemSuggestion {
	if len(ann) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []ExternalSystemSuggestion
	for module, a := range ann {
		for _, s := range a.ExternalSystemSuggestions {
			if s.SourceModule == "" {
				s.SourceModule = module
			}
			key := strings.Join([]string{s.Name, strings.Join(s.Targets, ","), s.SourceModule}, "\x00")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].SourceModule < out[j].SourceModule
	})
	return out
}

// yamlKey sanitizes a module name for use as a YAML mapping key.
// Replaces characters that would require quoting.
func yamlKey(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}
