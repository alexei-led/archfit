package config

import "fmt"

// ToolMode represents the enabled state of an external tool.
// It accepts true/false (bool) or "auto"/"on"/"off" (string) in YAML.
type ToolMode string

// ToolMode constants for the enabled field of a tool config entry.
const (
	ModeAuto ToolMode = "auto"
	ModeOn   ToolMode = "on"
	ModeOff  ToolMode = "off"
)

// Language tool key constants used in the Tools map and ForExtract.
const (
	LangGo         = "go"
	LangTypeScript = "typescript"
	LangPython     = "python"
	LangRust       = "rust"
)

// ToolScip is the Tools map key for the SCIP symbol-level strength provider.
const ToolScip = "scip"

// ToolSyntax is the Tools map key for the ast-grep syntax-facts provider.
// Off by default — running ast-grep over the full repo adds cost to the check
// path; opt-in via tools.syntax.enabled: on. Config-driven (not PATH presence)
// for same-config→same-metrics determinism.
const ToolSyntax = "syntax"

// SyntaxEnabled reports whether the syntax-facts provider is explicitly enabled
// (tools.syntax.enabled: on). Opt-in only — auto/off/absent all disable it.
func (c Config) SyntaxEnabled() bool {
	return c.Tools[ToolSyntax].Enabled == ModeOn
}

// ToolCargoModules is the Tools map key for the cargo-modules intra-crate module
// graph provider. Off by default — it compiles the crate (minutes); opt-in via
// tools.cargo-modules.enabled: on.
const ToolCargoModules = "cargo-modules"

// CargoModulesEnabled reports whether the cargo-modules module-graph provider is
// explicitly enabled (tools.cargo-modules.enabled: on). Opt-in only — it requires
// a full crate compile. Auto/off/absent all disable it.
func (c Config) CargoModulesEnabled() bool {
	return c.Tools[ToolCargoModules].Enabled == ModeOn
}

// ScipEnabled reports whether the SCIP strength provider is explicitly enabled
// (tools.scip.enabled: on). It is opt-in only — auto/off/absent all disable it —
// because running a SCIP indexer is whole-repo and slow, which must not happen on
// the fast `archfit check` path by default. Keeping the decision in config (not in
// PATH tool presence) also preserves the same-config→same-metrics guarantee.
func (c Config) ScipEnabled() bool {
	return c.Tools[ToolScip].Enabled == ModeOn
}

// ToolComplexity is the Tools map key for the external cyclomatic-complexity tool.
const ToolComplexity = "complexity"

// ComplexityEnabled reports whether the external complexity tool (lizard) is
// explicitly enabled (tools.complexity.enabled: on). Opt-in only — like SCIP it
// shells out to an external tool and adds cost to the check path, so it stays off
// unless asked for. Config-driven (not PATH presence) for deterministic metrics.
func (c Config) ComplexityEnabled() bool {
	return c.Tools[ToolComplexity].Enabled == ModeOn
}

// ToolLLM is the Tools map key for the off-gate LLM provider used by the
// enrich and explain commands. NEVER consumed by check — the arch ring test
// forbids internal packages from importing the LLM layer.
const ToolLLM = "llm"

// LLMProviders are the accepted tools.llm.provider values.
var LLMProviders = map[string]struct{}{"anthropic": {}, "openai": {}, "ollama": {}}

// LLMConfig is the resolved off-gate LLM settings.
type LLMConfig struct {
	Provider string
	Model    string
	BaseURL  string
}

// LLM returns the tools.llm settings and whether they are usable
// (provider and model both set).
func (c Config) LLM() (LLMConfig, bool) {
	t := c.Tools[ToolLLM]
	cfg := LLMConfig{Provider: t.Provider, Model: t.Model, BaseURL: t.BaseURL}
	return cfg, t.Provider != "" && t.Model != ""
}

// ToolClones is the Tools map key for the optional clone-detection provider.
const ToolClones = "clones"

// ClonesEnabled reports whether the clone-detection tool is explicitly enabled
// (tools.clones.enabled: on). Opt-in only — running a clone detector is expensive
// and must not happen by default. Config-driven for deterministic metrics.
func (c Config) ClonesEnabled() bool {
	return c.Tools[ToolClones].Enabled == ModeOn
}

// UnmarshalYAML handles both bool (true→on, false→off) and string ("auto"/"on"/"off") values.
func (m *ToolMode) UnmarshalYAML(unmarshal func(any) error) error {
	// Try bool first (YAML parses bare true/false as bool).
	var b bool
	if err := unmarshal(&b); err == nil {
		if b {
			*m = ModeOn
		} else {
			*m = ModeOff
		}
		return nil
	}
	// Fall back to string.
	var s string
	if err := unmarshal(&s); err != nil {
		return fmt.Errorf("tool mode must be true, false, or \"auto\": %w", err)
	}
	switch s {
	case "auto":
		*m = ModeAuto
	case "on", "true":
		*m = ModeOn
	case "off", "false":
		*m = ModeOff
	default:
		return fmt.Errorf("tool mode %q is not one of: true, false, auto", s)
	}
	return nil
}

// GateMode is the coverage-gate posture for one tool: how its absence affects the
// exit code. It is distinct from ToolMode (which controls whether a tool runs).
//   - off  — report the coverage gap but never fail.
//   - warn — (default, empty) report the gap, exit 0 (warn-loud).
//   - fail — a missing tool fails the build (opt-in hard gate).
//
// Empty means "use the default (warn)". The exit decision lives in cmd/; the core
// ring never reads it. See also --require-tools, which raises every gap to fail.
type GateMode string

// GateMode constants for the tools.<x>.gate field. Values match the rule/metric
// gate vocabulary (off|warn|fail) so the whole config speaks one gate language.
const (
	GateOff  GateMode = "off"
	GateWarn GateMode = "warn"
	GateFail GateMode = "fail"
)

// ToolConfig holds the settings for a single external tool.
// Provider/Model/BaseURL apply to the "llm" key only (see Config.LLM).
type ToolConfig struct {
	Enabled  ToolMode `yaml:"enabled"`
	Gate     GateMode `yaml:"gate,omitempty"`
	Provider string   `yaml:"provider,omitempty"`
	Model    string   `yaml:"model,omitempty"`
	BaseURL  string   `yaml:"base_url,omitempty"`
}

// ToolsConfig holds settings for all known external tools, keyed by language name.
type ToolsConfig map[string]ToolConfig
