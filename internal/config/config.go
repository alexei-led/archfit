// Package config defines the Config struct, its view projections, and the
// Load function that parses and validates an archfit.yaml configuration file.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/internal/model/coupling"
)

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
)

// ToolScip is the Tools map key for the SCIP symbol-level strength provider.
const ToolScip = "scip"

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

// ToolGitnexus is the Tools map key for the optional gitnexus symbol-impact provider.
const ToolGitnexus = "gitnexus"

// GitnexusEnabled reports whether the gitnexus symbol-impact provider is explicitly
// enabled (tools.gitnexus.enabled: on). Never auto — gitnexus requires network access
// and must not run by default. Config-driven (not PATH presence) for reproducibility.
func (c Config) GitnexusEnabled() bool {
	return c.Tools[ToolGitnexus].Enabled == ModeOn
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

// ToolConfig holds the settings for a single external tool.
// Provider/Model/BaseURL apply to the "llm" key only (see Config.LLM).
type ToolConfig struct {
	Enabled  ToolMode `yaml:"enabled"`
	Provider string   `yaml:"provider,omitempty"`
	Model    string   `yaml:"model,omitempty"`
	BaseURL  string   `yaml:"base_url,omitempty"`
}

// ToolsConfig holds settings for all known external tools, keyed by language name.
type ToolsConfig map[string]ToolConfig

// ModuleDef defines a module's path ownership and metadata.
type ModuleDef struct {
	Paths      []string  `yaml:"paths"`
	Public     []string  `yaml:"public"`
	Internal   []string  `yaml:"internal"`
	Layer      string    `yaml:"layer"`
	Subdomain  string    `yaml:"subdomain"`
	Volatility string    `yaml:"volatility"`
	Owner      string    `yaml:"owner"`
	DeployUnit string    `yaml:"deploy_unit"`
	ReviewedAt time.Time `yaml:"reviewed_at,omitempty"`
	ReviewedBy string    `yaml:"reviewed_by"`
}

// RuleDef declares a single architecture rule.
type RuleDef struct {
	ID        string       `yaml:"id"`
	Type      string       `yaml:"type"`
	Gate      string       `yaml:"gate"`
	From      string       `yaml:"from"`
	To        string       `yaml:"to"`
	FromLayer string       `yaml:"from_layer"`
	ToLayer   string       `yaml:"to_layer"`
	Patterns  []PatternDef `yaml:"patterns,omitempty"`
}

// ExceptionDef grants a temporary exception to a rule.
type ExceptionDef struct {
	Rule       string `yaml:"rule"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
	Reason     string `yaml:"reason"`
	ApprovedBy string `yaml:"approved_by"`
	Expires    string `yaml:"expires"`
}

// MetricEntry holds the settings for a single metric inside the metrics map.
type MetricEntry struct {
	Enabled    bool    `yaml:"enabled"`
	Gate       string  `yaml:"gate"`
	MinDelta   float64 `yaml:"min_delta"`
	MaxNewHigh int     `yaml:"max_new_high"`
	MaxNew     int     `yaml:"max_new"`
}

// MetricsConfig holds settings for all metrics, keyed by metric name.
type MetricsConfig map[string]MetricEntry

// MapReviewConfig configures architecture-map staleness gating.
type MapReviewConfig struct {
	StaleAfter string `yaml:"stale_after"`
	Gate       string `yaml:"gate"`
}

// OutputsConfig controls which output formats are produced.
type OutputsConfig struct {
	JSON     bool `yaml:"json"`
	Markdown bool `yaml:"markdown"`
	SARIF    bool `yaml:"sarif"`
}

// Config is the parsed and validated content of an archfit.yaml file.
type Config struct {
	Version               int                  `yaml:"version"`
	Modules               map[string]ModuleDef `yaml:"modules"`
	Layers                []string             `yaml:"layers"`
	Rules                 []RuleDef            `yaml:"rules"`
	Exclusions            []string             `yaml:"exclusions"`
	Tools                 ToolsConfig          `yaml:"tools"`
	Metrics               MetricsConfig        `yaml:"metrics"`
	Exceptions            []ExceptionDef       `yaml:"exceptions"`
	MapReview             MapReviewConfig      `yaml:"map_review"`
	Outputs               OutputsConfig        `yaml:"outputs"`
	PythonPackage         string               `yaml:"python_package"`           // top-level Python package name for grimp
	BCAdvisoryMinSeverity string               `yaml:"bc_advisory_min_severity"` // minimum severity to emit BC coupling advisories: low|medium|high|critical (default: low)

	// derivedVolatility holds churn-derived volatility bands recorded by
	// ApplyVolatility. It is a separate store on purpose: Modules always
	// carries hand-authored values only, so consumers that must never see
	// derived volatility (risk_hub, enrich prompt context) read Modules
	// directly, while classification reads the effective view. This makes
	// the metrics.New / ApplyVolatility call order irrelevant by
	// construction — derived values cannot overwrite config.
	derivedVolatility map[string]string
}

// Load reads and strictly decodes an archfit.yaml file at path.
// Unknown YAML fields are rejected. Returns a descriptive error if validation fails.
func Load(_ context.Context, path string) (Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 — path is caller-supplied config file
	if err != nil {
		return Config{}, fmt.Errorf("config: read %q: %w", path, err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField())
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: decode %q: %w", path, err)
	}

	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// validate checks required config fields.
func validate(cfg Config) error {
	if cfg.Version <= 0 {
		return fmt.Errorf("version must be > 0 (got %d)", cfg.Version)
	}
	if t, ok := cfg.Tools[ToolLLM]; ok && t.Provider != "" {
		if _, valid := LLMProviders[t.Provider]; !valid {
			return fmt.Errorf("tools.llm.provider %q is not one of: anthropic, openai, ollama", t.Provider)
		}
		if t.Model == "" {
			return errors.New("tools.llm.model is required when tools.llm.provider is set")
		}
	}
	return nil
}

// Default returns a Config suitable for use when no archfit.yaml is present.
// All tool modes are auto, BC advisory minimum severity is medium, and no
// modules, layers, or rules are defined — only metric checks run.
func Default() Config {
	return Config{
		Version:               1,
		BCAdvisoryMinSeverity: "medium",
		Tools: map[string]ToolConfig{
			LangGo:         {Enabled: ModeAuto},
			LangTypeScript: {Enabled: ModeAuto},
			LangPython:     {Enabled: ModeAuto},
		},
	}
}

// ---------------------------------------------------------------------------
// View types — narrow projections of Config passed to each pipeline stage.
// ---------------------------------------------------------------------------

// ScopeConfig is the view passed to the scope resolution stage.
type ScopeConfig struct {
	Base       string // git ref to diff against (empty = none)
	Full       bool   // if true, full-repo mode (no diff)
	Exclusions []string
	WorkDir    string // working directory for git commands; empty = process cwd
}

// ExtractConfig is the view passed to a language extractor.
// Built via ForExtract(lang); holds only what the extractor needs.
type ExtractConfig struct {
	// Common fields.
	Src        string   // source root (first path of first module, or ".")
	Paths      []string // all module paths
	Exclusions []string
	Internal   []string // all internal globs across modules
	Mode       ToolMode // derived from Tools map for the given language/tool

	// Go-specific.
	BuildFlags []string // extra build flags passed to go/packages (e.g. ["-tags", "extractortest"])

	// TypeScript-specific.
	TSConfig string // path to tsconfig.json (empty = auto)

	// Python-specific.
	PyPackage   string // top-level package name
	ProjectRoot string // project root for uv --directory
}

// ClassifyConfig is the view passed to the classify stage.
type ClassifyConfig struct {
	Modules               map[string]ModuleDef
	Layers                []string
	ModuleMap             ModuleMap
	BCAdvisoryMinSeverity string // minimum severity to emit BC coupling advisories
	// ApprovedLabels pins integration strength per ordered module pair, keyed
	// by from+"\x00"+to (labels.Key). Human-approved enrich output, validated
	// for freshness by the engine before injection. Precedence in classify:
	// config globs > approved labels > extractor hint.
	ApprovedLabels map[string]string
	// Scorer is the coupling scorer applied to each cross-boundary edge.
	// When nil, classify.Run uses coupling.DefaultScorer() (LegacyShim).
	Scorer coupling.Scorer
}

// RuleConfig is the view passed to the rules stage.
type RuleConfig struct {
	Rules     []RuleDef
	Layers    []string
	ModuleMap ModuleMap
}

// MetricConfig is the per-metric view returned by ForMetric.
type MetricConfig = MetricEntry

// ExceptionSet is the view passed to the status stage.
type ExceptionSet struct {
	Exceptions []ExceptionDef
}

// OutputConfig is the view passed to the output stage.
type OutputConfig struct {
	JSON     bool
	Markdown bool
	SARIF    bool
}

// ModuleMap resolves a repo-relative path to the owning module name.
// It uses doublestar glob matching against module path patterns.
type ModuleMap struct {
	// sorted module names for deterministic iteration when globs overlap
	names   []string
	modules map[string]ModuleDef
}

// buildModuleMap constructs a ModuleMap from the Config's Modules.
// Module names are sorted alphabetically so iteration is deterministic.
func buildModuleMap(modules map[string]ModuleDef) ModuleMap {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return ModuleMap{names: names, modules: modules}
}

// ModuleFor returns the first module name whose path globs match the given
// repo-relative path (forward-slash separated). When multiple modules match,
// the alphabetically-first module name wins (deterministic tiebreak).
// Returns ("", false) if no module matches.
func (mm ModuleMap) ModuleFor(path string) (string, bool) {
	for _, name := range mm.names {
		def := mm.modules[name]
		for _, pattern := range def.Paths {
			matched, _ := doublestar.Match(pattern, path)
			if matched {
				return name, true
			}
		}
	}
	return "", false
}

// LayerFor returns the layer name for the module that owns the given repo-relative
// path. Returns ("", false) if no module matches or the module has no layer set.
func (mm ModuleMap) LayerFor(path string) (string, bool) {
	name, ok := mm.ModuleFor(path)
	if !ok {
		return "", false
	}
	def := mm.modules[name]
	if def.Layer == "" {
		return "", false
	}
	return def.Layer, true
}

// ---------------------------------------------------------------------------
// Projection methods on Config.
// ---------------------------------------------------------------------------

// ForScope returns the ScopeConfig view.
func (c Config) ForScope() ScopeConfig {
	return ScopeConfig{
		Exclusions: c.Exclusions,
	}
}

// ForExtract returns an ExtractConfig for the given language.
// Mode is derived from the Tools map (defaults to ModeAuto when absent).
func (c Config) ForExtract(lang string) ExtractConfig {
	ec := ExtractConfig{
		Src:        ".",
		Exclusions: c.Exclusions,
	}

	// Collect all module paths and internal globs.
	var paths, internal []string
	first := true
	for _, name := range sortedKeys(c.Modules) {
		def := c.Modules[name]
		paths = append(paths, def.Paths...)
		internal = append(internal, def.Internal...)
		if first && len(def.Paths) > 0 {
			ec.Src = def.Paths[0]
			first = false
		}
	}
	ec.Paths = paths
	ec.Internal = internal

	// Python-specific fields.
	if lang == LangPython {
		ec.PyPackage = c.PythonPackage
	}

	// Derive Mode from the Tools map. The key is the language name itself.
	if tc, ok := c.Tools[lang]; ok {
		ec.Mode = tc.Enabled
	} else {
		ec.Mode = ModeAuto
	}

	return ec
}

// ForClassify returns the ClassifyConfig view. Classification sees only
// hand-authored module definitions — explicit volatility and subdomain fields
// only. Churn-derived volatility (recorded by ApplyVolatility) is intentionally
// excluded: Balanced Coupling forbids commit-history volatility on the gate path.
// Churn is stored in the separate derivedVolatility map for future report-only metrics.
func (c Config) ForClassify() ClassifyConfig {
	return ClassifyConfig{
		Modules:               c.Modules,
		Layers:                c.Layers,
		ModuleMap:             buildModuleMap(c.Modules),
		BCAdvisoryMinSeverity: c.BCAdvisoryMinSeverity,
	}
}

// ForRules returns the RuleConfig view.
func (c Config) ForRules() RuleConfig {
	return RuleConfig{
		Rules:     c.Rules,
		Layers:    c.Layers,
		ModuleMap: buildModuleMap(c.Modules),
	}
}

// ForMetric returns the MetricConfig for the named metric.
// Returns a zero MetricConfig (Enabled=false) if the metric is not configured.
func (c Config) ForMetric(name string) MetricConfig {
	if c.Metrics == nil {
		return MetricConfig{}
	}
	return c.Metrics[name]
}

// ForStatus returns the ExceptionSet view.
func (c Config) ForStatus() ExceptionSet {
	return ExceptionSet{Exceptions: c.Exceptions}
}

// ForOutput returns the OutputConfig view.
func (c Config) ForOutput() OutputConfig {
	return OutputConfig{
		JSON:     c.Outputs.JSON,
		Markdown: c.Outputs.Markdown,
		SARIF:    c.Outputs.SARIF,
	}
}

// ModuleMapView returns a ModuleMap built from this Config's Modules.
func (c Config) ModuleMapView() ModuleMap {
	return buildModuleMap(c.Modules)
}

// PatternDef defines a single structural pattern rule for ast-grep.
type PatternDef struct {
	ID   string `yaml:"id"`
	Lang string `yaml:"lang"`
	Rule string `yaml:"rule"`
}

// PatternConfig is the list of pattern definitions passed to a PatternProvider.
type PatternConfig []PatternDef

// ForPatterns returns the PatternConfig view: all PatternDef values collected
// from rules that declare a patterns: block.
func (c Config) ForPatterns() PatternConfig {
	var out PatternConfig
	for _, r := range c.Rules {
		out = append(out, r.Patterns...)
	}
	return out
}

// StalenessConfig is the view passed to the staleness check stage.
type StalenessConfig struct {
	Enabled   bool
	Threshold time.Duration // zero value defaults to 90*24*time.Hour in Check
	Modules   map[string]ModuleDef
}

// ForStaleness returns the StalenessConfig view.
func (c Config) ForStaleness() StalenessConfig {
	var threshold time.Duration
	if c.MapReview.StaleAfter != "" {
		if d, err := time.ParseDuration(c.MapReview.StaleAfter); err == nil {
			threshold = d
		}
	}
	return StalenessConfig{
		Enabled:   c.MapReview.StaleAfter != "" || c.MapReview.Gate != "",
		Threshold: threshold,
		Modules:   c.Modules,
	}
}

// FillMissingOwners sets the Owner field on modules that have no configured owner,
// using the resolved map (module name → owner string) produced by the ownership
// resolver. Explicit config owner always wins: a module with a non-empty Owner
// field is never overwritten. Modules absent from resolved, or with an empty
// resolved value, are left unchanged.
func (c Config) FillMissingOwners(resolved map[string]string) {
	for name, owner := range resolved {
		if owner == "" {
			continue
		}
		def, ok := c.Modules[name]
		if !ok || def.Owner != "" {
			continue
		}
		def.Owner = owner
		c.Modules[name] = def
	}
}

// sortedKeys returns a sorted slice of keys from a map[string]ModuleDef.
func sortedKeys(m map[string]ModuleDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
