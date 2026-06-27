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

	"github.com/goccy/go-yaml"
)

// Config is the parsed and validated content of an archfit.yaml file.
type Config struct {
	Version                  int                  `yaml:"version"`
	Modules                  map[string]ModuleDef `yaml:"modules"`
	Layers                   []string             `yaml:"layers"`
	Rules                    []RuleDef            `yaml:"rules"`
	Exclusions               []string             `yaml:"exclusions"`
	Tools                    ToolsConfig          `yaml:"tools"`
	Metrics                  MetricsConfig        `yaml:"metrics"`
	Exceptions               []ExceptionDef       `yaml:"exceptions"`
	MapReview                MapReviewConfig      `yaml:"map_review"`
	Outputs                  OutputsConfig        `yaml:"outputs"`
	PythonPackage            string               `yaml:"python_package"`             // top-level Python package name for grimp
	RustManifest             string               `yaml:"rust_manifest"`              // path to Cargo.toml (empty = auto, root manifest)
	RustFeatures             []string             `yaml:"rust_features"`              // cargo features to activate for metadata
	RustIncludeDevDeps       bool                 `yaml:"rust_include_dev_deps"`      // include dev-dependencies as crate edges
	BCAdvisoryMinSeverity    string               `yaml:"bc_advisory_min_severity"`   // minimum severity to emit BC coupling advisories: low|medium|high|critical (default: low)
	VolatilityCascadeEnabled bool                 `yaml:"volatility_cascade_enabled"` // propagate high volatility across strongly-coupled module pairs (book Ch9)

	// explicitOwners records which modules had a hand-authored `owner:` in YAML,
	// populated by Load before any resolver fill. Distinguishes a user's explicit
	// ownership (authoritative for distance) from a resolver-filled owner (e.g. the
	// git-author degenerate fallback). Not a YAML field; the decoder ignores it.
	explicitOwners map[string]bool
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

	// Record hand-authored owners BEFORE any resolver fill (FillMissingOwners runs
	// later in the pipeline). Anything here is explicit; an Owner set later without
	// an entry here is resolver-filled.
	cfg.explicitOwners = make(map[string]bool)
	for name, def := range cfg.Modules {
		if def.Owner != "" {
			cfg.explicitOwners[name] = true
		}
	}
	return cfg, nil
}

// WithExplicitOwners marks the named modules as having hand-authored owners and
// returns the updated config. Test seam: tests build Config literals directly,
// bypassing Load (which populates the explicit-owner set), so they use this to
// exercise the explicit-owner precedence branch in classify.
//
// It mirrors Load's invariant exactly: a module is marked only if it actually
// carries a non-empty Owner. Marking an ownerless module would route
// classifyDistance into ownershipDistance("", other), which is a footgun this
// guard removes by construction — explicitOwners[m] always implies Owner != "".
func (c Config) WithExplicitOwners(modules ...string) Config {
	c.explicitOwners = make(map[string]bool, len(modules))
	for _, m := range modules {
		if c.Modules[m].Owner != "" {
			c.explicitOwners[m] = true
		}
	}
	return c
}

// bcSeverities are the accepted bc_advisory_min_severity values (low→critical).
var bcSeverities = map[string]struct{}{"low": {}, "medium": {}, "high": {}, "critical": {}}

// gateValues are the accepted gate policy markers (spec §rules: off | warn | fail),
// shared by rule, metric, and map_review gates. Empty means "use the default".
var gateValues = map[string]struct{}{"off": {}, "warn": {}, "fail": {}}

// validate checks required config fields. An invalid enum is a hard error rather
// than a silent skip: a typo in bc_advisory_min_severity or a gate must not
// quietly disable the check it was meant to configure.
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
	if s := cfg.BCAdvisoryMinSeverity; s != "" {
		if _, ok := bcSeverities[s]; !ok {
			return fmt.Errorf("bc_advisory_min_severity %q is not one of: low, medium, high, critical", s)
		}
	}
	for _, name := range sortedKeys(cfg.Modules) {
		if r := cfg.Modules[name].Role; r != "" {
			if _, ok := moduleRoles[r]; !ok {
				return fmt.Errorf("modules.%s.role %q is not one of: composition_root, adapter, core, shared_model, generated, test", name, r)
			}
		}
	}
	for i, r := range cfg.Rules {
		id := r.ID
		if id == "" {
			id = fmt.Sprintf("#%d", i)
		}
		if err := validateGate(fmt.Sprintf("rules[%s]", id), r.Gate); err != nil {
			return err
		}
	}
	for _, name := range sortedMetricKeys(cfg.Metrics) {
		if err := validateGate("metrics."+name, cfg.Metrics[name].Gate); err != nil {
			return err
		}
	}
	for _, name := range sortedToolKeys(cfg.Tools) {
		if err := validateGate("tools."+name, string(cfg.Tools[name].Gate)); err != nil {
			return err
		}
	}
	if s := cfg.MapReview.StaleAfter; s != "" {
		if _, err := time.ParseDuration(s); err != nil {
			return fmt.Errorf("map_review.stale_after %q is not a valid Go duration (e.g. 720h, 30m): %w", s, err)
		}
	}
	for _, name := range sortedToolKeys(cfg.Tools) {
		if s := cfg.Tools[name].Timeout; s != "" {
			if _, err := time.ParseDuration(s); err != nil {
				return fmt.Errorf("tools.%s.timeout %q is not a valid Go duration (e.g. 5m, 10m30s): %w", name, s, err)
			}
		}
	}
	return validateGate("map_review", cfg.MapReview.Gate)
}

// validateGate rejects a non-empty gate that is not one of off|warn|fail.
// field is the dotted config path used in the error (e.g. "rules[cycle]").
func validateGate(field, gate string) error {
	if gate == "" {
		return nil
	}
	if _, ok := gateValues[gate]; !ok {
		return fmt.Errorf("%s.gate %q is not one of: off, warn, fail", field, gate)
	}
	return nil
}

// sortedMetricKeys returns metric names in sorted order so validation reports a
// deterministic first offender when multiple metrics carry an invalid gate.
func sortedMetricKeys(m MetricsConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedToolKeys returns tool names in sorted order so gate validation reports a
// deterministic first offender when multiple tools carry an invalid gate.
func sortedToolKeys(m ToolsConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
			LangRust:       {Enabled: ModeAuto},
		},
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

// FillMissingDeployUnits sets the DeployUnit field on modules that have no
// configured deploy unit, using the resolved map (module name → unit string)
// produced by the deploy-unit detector. Config-authored DeployUnit always wins:
// a module with a non-empty DeployUnit field is never overwritten. Modules absent
// from resolved, or with an empty resolved value, are left unchanged.
func (c Config) FillMissingDeployUnits(resolved map[string]string) {
	for name, unit := range resolved {
		if unit == "" {
			continue
		}
		def, ok := c.Modules[name]
		if !ok || def.DeployUnit != "" {
			continue
		}
		def.DeployUnit = unit
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
