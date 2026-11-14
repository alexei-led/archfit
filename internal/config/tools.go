package config

import (
	"time"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Language id constants. These are the YAML keys under `languages:` and the
// extractor ids used by the language registry and the ToolMode/ToolGate/
// SetToolMode accessors.
const (
	LangGo         = "go"
	LangTypeScript = "typescript"
	LangPython     = "python"
	LangRust       = "rust"
	LangDotNet     = "dotnet"
)

// Analyzer id constants. These are internal string ids used by cmd coverage /
// registry code and by the ToolMode/ToolGate accessors. They are decoupled from
// the YAML keys (which come from the struct tags on AnalyzersConfig), so an
// internal id may differ from its YAML key (e.g. ToolCargoModules vs cargo_modules).
const (
	ToolSyntax       = "syntax"
	ToolCargoModules = "cargo-modules"
	ToolRoslyn       = "roslyn"
)

// ---------------------------------------------------------------------------
// Languages — one block per language under `languages:`. Each carries only the
// fields that language actually uses, so a reader (and the JSON schema) never
// sees an inapplicable knob.
// ---------------------------------------------------------------------------

// GoLanguage configures the Go extractor (`languages.go`).
type GoLanguage struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
	Modules GoModuleFilter         `yaml:"modules,omitempty"`
}

// TypeScriptLanguage configures the TypeScript extractor (`languages.typescript`).
type TypeScriptLanguage struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
}

// PythonLanguage configures the Python extractor (`languages.python`).
type PythonLanguage struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
	// Package is the top-level importable package name passed to grimp
	// (e.g. "myapp"). Empty = auto-detect from the project layout.
	Package string `yaml:"package,omitempty"`
}

// RustLanguage configures the Rust extractor (`languages.rust`).
type RustLanguage struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
	// Manifest is the path to Cargo.toml. Empty = auto (the root manifest).
	Manifest string `yaml:"manifest,omitempty"`
	// Features are the cargo features to activate for `cargo metadata`.
	Features []string `yaml:"features,omitempty"`
	// IncludeDevDeps adds dev-dependencies as crate edges.
	IncludeDevDeps bool `yaml:"include_dev_deps,omitempty"`
}

// DotNetLanguage configures the .NET extractor (`languages.dotnet`).
type DotNetLanguage struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
	// Projects are the solution (.slnx, .sln), solution filter (.slnf), or
	// project (.csproj) files to analyze, relative to the scan root. Empty =
	// auto: the first .slnx or .sln at the root, else every .csproj outside
	// obj/, bin/ and exclusions.
	Projects []string `yaml:"projects,omitempty"`
}

// LanguagesConfig groups the per-language extractor settings (`languages:`).
type LanguagesConfig struct {
	Go         GoLanguage         `yaml:"go,omitempty"`
	TypeScript TypeScriptLanguage `yaml:"typescript,omitempty"`
	Python     PythonLanguage     `yaml:"python,omitempty"`
	Rust       RustLanguage       `yaml:"rust,omitempty"`
	DotNet     DotNetLanguage     `yaml:"dotnet,omitempty"`
}

// ---------------------------------------------------------------------------
// Analyzers — opt-in deeper analysis backends under `analyzers:`. They produce
// extra facts (not gate rules); the gates live in `rules:`. Each carries only
// the fields it uses.
// ---------------------------------------------------------------------------

// Analyzer is the common enable/gate shape for analyzers with no extra knobs
// (`analyzers.syntax`, `analyzers.cargo_modules`).
type Analyzer struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
}

// TimedAnalyzer adds a per-run timeout for Roslyn (a Go duration, e.g. "5m").
// On a Roslyn timeout the projects fall back to their ProjectReference
// edges and the coupling dimension cannot reach measured: it stays partial.
// The run continues either way.
type TimedAnalyzer struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
	Timeout string                 `yaml:"timeout,omitempty"`
}

// AnalyzersConfig groups the opt-in analyzer settings (`analyzers:`).
type AnalyzersConfig struct {
	Syntax       Analyzer      `yaml:"syntax,omitempty"`
	CargoModules Analyzer      `yaml:"cargo_modules,omitempty"`
	Roslyn       TimedAnalyzer `yaml:"roslyn,omitempty"`
}

// ---------------------------------------------------------------------------
// Enable-state accessors. Signatures are stable across the schema rewrite so
// call sites outside this package are unaffected; only the bodies moved.
// ---------------------------------------------------------------------------

// SyntaxEnabled reports whether the syntax-facts provider is explicitly enabled
// (analyzers.syntax.enabled: true). Opt-in only — auto/false/absent all disable it.
func (c Config) SyntaxEnabled() bool { return c.Analyzers.Syntax.Enabled == evidenceports.ModeOn }

// CargoModulesEnabled reports whether the cargo-modules intra-crate module-graph
// analyzer is explicitly enabled (analyzers.cargo_modules.enabled: true). Opt-in
// only — it compiles the crate (minutes).
func (c Config) CargoModulesEnabled() bool {
	return c.Analyzers.CargoModules.Enabled == evidenceports.ModeOn
}

// RoslynEnabled reports whether the Roslyn type graph may run: on unless
// analyzers.roslyn.enabled is false. Unlike the opt-in analyzers it defaults on,
// and runs where ARCHFIT_DOTNET_ROSLYN names an installed helper; false forces
// the project-only .NET graph.
func (c Config) RoslynEnabled() bool {
	return c.Analyzers.Roslyn.Enabled != evidenceports.ModeOff
}

// ToolTimeout returns the configured per-analyzer subprocess timeout for the
// given analyzer id (ToolRoslyn). Returns 0 when not set or unparseable
// (callers use their built-in default).
func (c Config) ToolTimeout(id string) time.Duration {
	var s string
	if id == ToolRoslyn {
		s = c.Analyzers.Roslyn.Timeout
	}
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// ToolMode returns the enable mode for a language or analyzer addressed by its
// internal id (LangGo…LangDotNet, ToolSyntax…ToolRoslyn). Unknown ids return
// ModeAuto. Lets cmd coverage code resolve a tool's posture without knowing which
// config section holds it.
func (c Config) ToolMode(id string) evidenceports.ToolMode {
	switch id {
	case LangGo:
		return c.Languages.Go.Enabled
	case LangTypeScript:
		return c.Languages.TypeScript.Enabled
	case LangPython:
		return c.Languages.Python.Enabled
	case LangRust:
		return c.Languages.Rust.Enabled
	case LangDotNet:
		return c.Languages.DotNet.Enabled
	case ToolSyntax:
		return c.Analyzers.Syntax.Enabled
	case ToolCargoModules:
		return c.Analyzers.CargoModules.Enabled
	case ToolRoslyn:
		return c.Analyzers.Roslyn.Enabled
	default:
		return evidenceports.ModeAuto
	}
}

// ToolGate returns the coverage-gate posture for a language or analyzer addressed
// by its internal id. Unknown ids return the empty gate ("" = default warn).
func (c Config) ToolGate(id string) GateMode {
	switch id {
	case LangGo:
		return c.Languages.Go.Gate
	case LangTypeScript:
		return c.Languages.TypeScript.Gate
	case LangPython:
		return c.Languages.Python.Gate
	case LangRust:
		return c.Languages.Rust.Gate
	case LangDotNet:
		return c.Languages.DotNet.Gate
	case ToolSyntax:
		return c.Analyzers.Syntax.Gate
	case ToolCargoModules:
		return c.Analyzers.CargoModules.Gate
	case ToolRoslyn:
		return c.Analyzers.Roslyn.Gate
	default:
		return ""
	}
}

// SetToolMode forces the enable mode for a language or analyzer addressed by its
// internal id. Used by --lang flag overrides. Unknown ids are a no-op.
func (c *Config) SetToolMode(id string, mode evidenceports.ToolMode) {
	switch id {
	case LangGo:
		c.Languages.Go.Enabled = mode
	case LangTypeScript:
		c.Languages.TypeScript.Enabled = mode
	case LangPython:
		c.Languages.Python.Enabled = mode
	case LangRust:
		c.Languages.Rust.Enabled = mode
	case LangDotNet:
		c.Languages.DotNet.Enabled = mode
	case ToolSyntax:
		c.Analyzers.Syntax.Enabled = mode
	case ToolCargoModules:
		c.Analyzers.CargoModules.Enabled = mode
	case ToolRoslyn:
		c.Analyzers.Roslyn.Enabled = mode
	}
}

// GateMode controls how a missing tool affects the gate.
type GateMode = policy.GateMode

// GateOff, GateWarn, and GateFail define gate behavior.
const (
	GateOff  = policy.GateOff
	GateWarn = policy.GateWarn
	GateFail = policy.GateFail
)

// GoModuleFilter restricts Go workspace analysis to a subset of go.work members
// (`languages.go.modules`). Globs match each member's path relative to ScanRoot
// using doublestar semantics (the same matcher as scope exclusions).
type GoModuleFilter struct {
	Include []string `yaml:"include,omitempty"` // keep only members matching these globs; empty = all
	Exclude []string `yaml:"exclude,omitempty"` // drop members matching these globs; applied after include
}
