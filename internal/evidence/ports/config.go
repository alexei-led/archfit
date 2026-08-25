package ports

import "fmt"

// ToolMode is the enable state of a language extractor or analyzer.
//
// Canonical YAML values are: true (force on), false (force off), and auto (run
// only when the backing tool is detected on PATH). Only "auto" is written as a
// string; true/false are native YAML booleans. The legacy "on"/"off" spellings
// are no longer accepted — they are a hard error so configs speak one vocabulary.
type ToolMode string

// ToolMode internal states. ModeOn/ModeOff are the resolved forms of the YAML
// booleans true/false; ModeAuto is detect-on-PATH.
const (
	ModeAuto ToolMode = "auto"
	ModeOn   ToolMode = "on"
	ModeOff  ToolMode = "off"
)

// UnmarshalYAML accepts a native YAML boolean (true→on, false→off) or the string
// "auto". The legacy "on"/"off" string spellings are rejected with a clear error
// so the config speaks a single enable vocabulary.
func (m *ToolMode) UnmarshalYAML(unmarshal func(any) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		if b {
			*m = ModeOn
		} else {
			*m = ModeOff
		}
		return nil
	}
	var s string
	if err := unmarshal(&s); err != nil {
		return fmt.Errorf("enabled must be true, false, or auto: %w", err)
	}
	if s == "auto" {
		*m = ModeAuto
		return nil
	}
	return fmt.Errorf("enabled %q is not one of: true, false, auto", s)
}

// ExtractConfig is the construction input for one language extractor adapter.
// It carries only what the extractor needs to run its out-of-process tool.
type ExtractConfig struct {
	// Common fields.
	Src        string   // scan-root for extractors that need one (TS); always "." — never derived from Modules paths, which are classification globs, not filesystem dirs
	Paths      []string // all module paths
	Exclusions []string
	Internal   []string // all internal globs across modules
	Mode       ToolMode // derived from the language's enabled state

	// Go-specific.
	BuildFlags      []string // extra build flags passed to go/packages (e.g. ["-tags", "extractortest"])
	GoModuleInclude []string // languages.go.modules.include: keep only members matching these ScanRoot-relative globs
	GoModuleExclude []string // languages.go.modules.exclude: drop members matching these ScanRoot-relative globs

	// TypeScript-specific.
	TSConfig string // path to tsconfig.json (empty = auto)

	// Python-specific.
	PyPackage   string // top-level package name
	ProjectRoot string // project root for uv --directory

	// Rust-specific.
	CargoManifest  string   // path to Cargo.toml (empty = auto, root manifest)
	CargoFeatures  []string // cargo features to activate (empty = default)
	IncludeDevDeps bool     // include dev-dependencies as edges
	ModuleGraph    bool     // run cargo-modules to build intra-crate module graph (opt-in)
}

// SyntaxConfig is the request passed to a SyntaxProvider.
type SyntaxConfig struct {
	Enabled   bool
	Languages []string
}
