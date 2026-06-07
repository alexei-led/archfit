// Package config defines the Config struct, its view projections, and the
// Load function that parses and validates an archfit.yaml configuration file.
package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/goccy/go-yaml"
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
type ToolConfig struct {
	Enabled ToolMode `yaml:"enabled"`
}

// ToolsConfig holds settings for all known external tools, keyed by tool name.
// Tool names match the spec §6 YAML keys: git, dependency_cruiser, import_linter, ast_grep.
type ToolsConfig map[string]ToolConfig

// ModuleDef defines a module's path ownership and metadata.
type ModuleDef struct {
	Paths      []string `yaml:"paths"`
	Public     []string `yaml:"public"`
	Internal   []string `yaml:"internal"`
	Layer      string   `yaml:"layer"`
	Subdomain  string   `yaml:"subdomain"`
	Volatility string   `yaml:"volatility"`
	Owner      string   `yaml:"owner"`
	DeployUnit string   `yaml:"deploy_unit"`
	ReviewedAt string   `yaml:"reviewed_at"`
	ReviewedBy string   `yaml:"reviewed_by"`
}

// RuleDef declares a single architecture rule.
type RuleDef struct {
	ID        string `yaml:"id"`
	Type      string `yaml:"type"`
	Gate      string `yaml:"gate"`
	From      string `yaml:"from"`
	To        string `yaml:"to"`
	FromLayer string `yaml:"from_layer"`
	ToLayer   string `yaml:"to_layer"`
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
	Version       int                  `yaml:"version"`
	Modules       map[string]ModuleDef `yaml:"modules"`
	Layers        []string             `yaml:"layers"`
	Rules         []RuleDef            `yaml:"rules"`
	Exclusions    []string             `yaml:"exclusions"`
	Tools         ToolsConfig          `yaml:"tools"`
	Metrics       MetricsConfig        `yaml:"metrics"`
	Exceptions    []ExceptionDef       `yaml:"exceptions"`
	MapReview     MapReviewConfig      `yaml:"map_review"`
	Outputs       OutputsConfig        `yaml:"outputs"`
	PythonPackage string               `yaml:"python_package"` // top-level Python package name for grimp
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
	return nil
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
	Modules   map[string]ModuleDef
	Layers    []string
	ModuleMap ModuleMap
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

// languageToolKey maps a language name to the Tools map key used in archfit.yaml.
func languageToolKey(lang string) string {
	switch lang {
	case "typescript":
		return "dependency_cruiser"
	case "python":
		return "grimp"
	case "go":
		return "go"
	default:
		return lang
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
	if lang == "python" {
		ec.PyPackage = c.PythonPackage
	}

	// Derive Mode from the Tools map.
	toolKey := languageToolKey(lang)
	if tc, ok := c.Tools[toolKey]; ok {
		ec.Mode = tc.Enabled
	} else {
		ec.Mode = ModeAuto
	}

	return ec
}

// ForClassify returns the ClassifyConfig view.
func (c Config) ForClassify() ClassifyConfig {
	return ClassifyConfig{
		Modules:   c.Modules,
		Layers:    c.Layers,
		ModuleMap: buildModuleMap(c.Modules),
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

// sortedKeys returns a sorted slice of keys from a map[string]ModuleDef.
func sortedKeys(m map[string]ModuleDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
