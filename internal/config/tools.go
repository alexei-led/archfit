package config

import (
	"time"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"

	"github.com/alexei-led/archfit/internal/policy"
)

// Language id constants. These are the YAML keys under `languages:` and the
// extractor ids used by the language registry and the ToolMode/ToolGate/
// SetToolMode accessors.
const (
	LangGo         = "go"
	LangTypeScript = "typescript"
	LangPython     = "python"
	LangRust       = "rust"
)

// Analyzer id constants. These are internal string ids used by cmd coverage /
// registry code and by the ToolMode/ToolGate accessors. They are decoupled from
// the YAML keys (which come from the struct tags on AnalyzersConfig), so an
// internal id may differ from its YAML key (e.g. ToolCargoModules vs cargo_modules).
const (
	ToolSyntax       = "syntax"
	ToolScip         = "scip"
	ToolClones       = "clones"
	ToolCargoModules = "cargo-modules"
	// ToolLLM is the id for the off-gate AI provider used by enrich and explain.
	// NEVER consumed by analyze's gate path — the arch ring test forbids internal
	// packages from importing the LLM layer, so the LLM commands live in cmd.
	ToolLLM = "llm"
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

// LanguagesConfig groups the per-language extractor settings (`languages:`).
type LanguagesConfig struct {
	Go         GoLanguage         `yaml:"go,omitempty"`
	TypeScript TypeScriptLanguage `yaml:"typescript,omitempty"`
	Python     PythonLanguage     `yaml:"python,omitempty"`
	Rust       RustLanguage       `yaml:"rust,omitempty"`
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

// TimedAnalyzer adds a per-run subprocess timeout (`analyzers.scip`,
// `analyzers.clones`). On timeout the result is dropped and dependent metrics
// report n/a; the run continues. Timeout is a Go duration string (e.g. "5m").
type TimedAnalyzer struct {
	Enabled evidenceports.ToolMode `yaml:"enabled"`
	Gate    GateMode               `yaml:"gate,omitempty"`
	Timeout string                 `yaml:"timeout,omitempty"`
}

// AnalyzersConfig groups the opt-in analyzer settings (`analyzers:`).
type AnalyzersConfig struct {
	Syntax       Analyzer      `yaml:"syntax,omitempty"`
	Scip         TimedAnalyzer `yaml:"scip,omitempty"`
	Clones       TimedAnalyzer `yaml:"clones,omitempty"`
	CargoModules Analyzer      `yaml:"cargo_modules,omitempty"`
}

// ---------------------------------------------------------------------------
// AI — off-gate LLM provider for init/update/enrich/analyze/explain LLM flows (`ai:`).
// Presence of provider and model is what makes it usable; there is no enable flag.
// ---------------------------------------------------------------------------

// AIConfig configures the off-gate LLM provider (`ai:`). Consumed only by
// explicit LLM flows — never by analyze's deterministic gate path.
type AIConfig struct {
	Provider string `yaml:"provider,omitempty"`
	Model    string `yaml:"model,omitempty"`
	BaseURL  string `yaml:"base_url,omitempty"`
}

// LLMProviders are the accepted ai.provider values.
var LLMProviders = map[string]struct{}{"anthropic": {}, "openai": {}, "ollama": {}}

// LLMConfig is the resolved off-gate LLM settings.
type LLMConfig struct {
	Provider string
	Model    string
	BaseURL  string
}

// LLM returns the ai: settings and whether they are usable (provider and model
// both set).
func (c Config) LLM() (LLMConfig, bool) {
	cfg := LLMConfig{Provider: c.AI.Provider, Model: c.AI.Model, BaseURL: c.AI.BaseURL}
	return cfg, c.AI.Provider != "" && c.AI.Model != ""
}

// ---------------------------------------------------------------------------
// Enable-state accessors. Signatures are stable across the schema rewrite so
// call sites outside this package are unaffected; only the bodies moved.
// ---------------------------------------------------------------------------

// SyntaxEnabled reports whether the syntax-facts provider is explicitly enabled
// (analyzers.syntax.enabled: true). Opt-in only — auto/false/absent all disable it.
func (c Config) SyntaxEnabled() bool { return c.Analyzers.Syntax.Enabled == evidenceports.ModeOn }

// ScipEnabled reports whether the SCIP strength provider is explicitly enabled
// (analyzers.scip.enabled: true). Opt-in only — running a SCIP indexer is
// whole-repo and slow, so it must not run on the fast gate path by default.
// Config-driven (not PATH presence) preserves same-config→same-metrics.
func (c Config) ScipEnabled() bool { return c.Analyzers.Scip.Enabled == evidenceports.ModeOn }

// ClonesEnabled reports whether the clone-detection analyzer is explicitly
// enabled (analyzers.clones.enabled: true). Opt-in only.
func (c Config) ClonesEnabled() bool { return c.Analyzers.Clones.Enabled == evidenceports.ModeOn }

// CargoModulesEnabled reports whether the cargo-modules intra-crate module-graph
// analyzer is explicitly enabled (analyzers.cargo_modules.enabled: true). Opt-in
// only — it compiles the crate (minutes).
func (c Config) CargoModulesEnabled() bool {
	return c.Analyzers.CargoModules.Enabled == evidenceports.ModeOn
}

// ToolTimeout returns the configured per-analyzer subprocess timeout for the
// given analyzer id (ToolScip, ToolClones). Returns 0 when not set or unparseable
// (callers use their built-in default).
func (c Config) ToolTimeout(id string) time.Duration {
	var s string
	switch id {
	case ToolScip:
		s = c.Analyzers.Scip.Timeout
	case ToolClones:
		s = c.Analyzers.Clones.Timeout
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
// internal id (LangGo…LangRust, ToolSyntax…ToolCargoModules). Unknown ids return
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
	case ToolSyntax:
		return c.Analyzers.Syntax.Enabled
	case ToolScip:
		return c.Analyzers.Scip.Enabled
	case ToolClones:
		return c.Analyzers.Clones.Enabled
	case ToolCargoModules:
		return c.Analyzers.CargoModules.Enabled
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
	case ToolSyntax:
		return c.Analyzers.Syntax.Gate
	case ToolScip:
		return c.Analyzers.Scip.Gate
	case ToolClones:
		return c.Analyzers.Clones.Gate
	case ToolCargoModules:
		return c.Analyzers.CargoModules.Gate
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
	case ToolSyntax:
		c.Analyzers.Syntax.Enabled = mode
	case ToolScip:
		c.Analyzers.Scip.Enabled = mode
	case ToolClones:
		c.Analyzers.Clones.Enabled = mode
	case ToolCargoModules:
		c.Analyzers.CargoModules.Enabled = mode
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
