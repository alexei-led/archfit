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
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/goccy/go-yaml"
)

// Config is the parsed and validated content of an archfit.yaml file.
//
// Top-level layout (see docs/guide/configuration-reference.md):
//   - exclude        — path globs to skip during scanning
//   - languages      — per-language extractor settings (go/typescript/python/rust)
//   - analyzers      — opt-in deeper analysis backends (syntax/scip/complexity/…)
//   - ai             — off-gate LLM provider for enrich/explain
//   - coupling       — Balanced-Coupling advisory tuning
//   - layers/modules — the architecture map
//   - rules/waivers  — gates and their approved deviations
//   - module_review  — staleness gating of the module declarations
//   - file_class / outputs — classification overrides and output formats
type Config struct {
	Version      int                  `yaml:"version"`
	Exclude      []string             `yaml:"exclude"`
	Languages    LanguagesConfig      `yaml:"languages"`
	Analyzers    AnalyzersConfig      `yaml:"analyzers"`
	AI           AIConfig             `yaml:"ai"`
	Coupling     CouplingConfig       `yaml:"coupling"`
	Layers       []string             `yaml:"layers"`
	Modules      map[string]ModuleDef `yaml:"modules"`
	Rules        []RuleDef            `yaml:"rules"`
	Waivers      []WaiverDef          `yaml:"waivers"`
	Metrics      MetricsConfig        `yaml:"metrics"`
	ModuleReview ModuleReviewConfig   `yaml:"module_review"`
	FileClass    FileClassDef         `yaml:"file_class"`
	Outputs      OutputsConfig        `yaml:"outputs"`

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
		return Config{}, fmt.Errorf("config: decode %q: %w", path, deprecatedConfigHint(err))
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

// deprecatedConfigHint augments a strict-decode failure with v0.x→v1.0 migration
// guidance when the unknown field is a key renamed or removed before v1.0. The raw
// "unknown field" error is kept (it quotes the line); the hint tells the user what
// to change. Removed `metrics` map keys are caught later in validate() instead,
// since a map key is not an "unknown field" at decode time.
func deprecatedConfigHint(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, `unknown field "tools"`):
		return fmt.Errorf("%w\nhint: `tools:` was renamed to `analyzers:` in v1.0 (and `analyzers.complexity` was removed)", err)
	case strings.Contains(msg, `unknown field "complexity"`):
		return fmt.Errorf("%w\nhint: `analyzers.complexity` was removed in v1.0 (gocyclo/lizard backends dropped)", err)
	case strings.Contains(msg, `unknown field "gitnexus"`):
		return fmt.Errorf("%w\nhint: gitnexus integration was removed in v1.0; remove the key", err)
	}
	return err
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

// bcSeverities are the accepted coupling.min_severity values (low→critical).
var bcSeverities = map[string]struct{}{"low": {}, "medium": {}, "high": {}, "critical": {}}

// gateValues are the accepted gate policy markers (spec §rules: off | warn | fail),
// shared by rule, metric, and module_review gates. Empty means "use the default".
var gateValues = map[string]struct{}{"off": {}, "warn": {}, "fail": {}}

// metricKnob classifies which delta-threshold knob a metric's gate accepts.
// A metric's polarity is definitional (stamped as Direction on its result),
// not a user choice — the knob kind follows it: count metrics (higher is
// worse) accept max_new, ratio metrics (higher is better) accept min_delta,
// informational metrics accept neither because they never carry a baseline
// delta and never gate.
type metricKnob int

const (
	knobRatio metricKnob = iota // min_delta applies (higher is better)
	knobCount                   // max_new applies (higher is worse)
	knobNone                    // informational: no gate, no thresholds
)

// knownMetrics maps each metric key archfit implements to its threshold-knob
// kind. `metrics` is a map, so unknown keys escape DisallowUnknownField —
// validate() rejects them so a typo or a removed metric is a loud config
// error, not a silent no-op (consistency with the strict `analyzers` struct).
var knownMetrics = map[string]metricKnob{
	"encapsulation":   knobRatio,
	"coverage":        knobRatio,
	"unbalanced_edge": knobCount,
	"cycle":           knobCount,
	"blast_radius":    knobNone,
}

// removedConfigKeys maps config keys removed before v1.0 to a short reason, so a
// stale config gets an actionable error naming the removed key rather than a
// generic "unknown metric". Top-level renames (tools→analyzers) and removed
// struct fields (analyzers.complexity) are caught at decode time by
// deprecatedConfigHint instead.
var removedConfigKeys = map[string]string{
	"risk_hub":              "removed in v1.0",
	"functional_candidates": "removed in v1.0",
	"complexity":            "removed in v1.0 (gocyclo/lizard backends dropped)",
	"gitnexus":              "removed in v1.0 (gitnexus integration dropped)",
}

// validate checks required config fields. An invalid enum is a hard error rather
// than a silent skip: a typo in coupling.min_severity or a gate must not
// quietly disable the check it was meant to configure.
func validate(cfg Config) error {
	if cfg.Version <= 0 {
		return fmt.Errorf("version must be > 0 (got %d)", cfg.Version)
	}
	if cfg.AI.Provider != "" {
		if _, valid := LLMProviders[cfg.AI.Provider]; !valid {
			return fmt.Errorf("ai.provider %q is not one of: anthropic, openai, ollama", cfg.AI.Provider)
		}
		if cfg.AI.Model == "" {
			return errors.New("ai.model is required when ai.provider is set")
		}
	}
	if s := cfg.Coupling.MinSeverity; s != "" {
		if _, ok := bcSeverities[s]; !ok {
			return fmt.Errorf("coupling.min_severity %q is not one of: low, medium, high, critical", s)
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
		if reason, removed := removedConfigKeys[name]; removed {
			return fmt.Errorf("metrics.%s was %s — remove it", name, reason)
		}
		knob, ok := knownMetrics[name]
		if !ok {
			return fmt.Errorf("metrics.%s is not a known metric (known: blast_radius, coverage, cycle, encapsulation, unbalanced_edge)", name)
		}
		if err := validateMetricEntry(name, knob, cfg.Metrics[name]); err != nil {
			return err
		}
	}
	for _, t := range cfg.toolEntries() {
		if err := validateGate(t.path, string(t.gate)); err != nil {
			return err
		}
		if t.timeout != "" {
			if _, err := time.ParseDuration(t.timeout); err != nil {
				return fmt.Errorf("%s.timeout %q is not a valid Go duration (e.g. 5m, 10m30s): %w", t.path, t.timeout, err)
			}
		}
	}
	if s := cfg.ModuleReview.StaleAfter; s != "" {
		if _, err := time.ParseDuration(s); err != nil {
			return fmt.Errorf("module_review.stale_after %q is not a valid Go duration (e.g. 720h, 30m): %w", s, err)
		}
	}
	if err := validateGate("module_review", cfg.ModuleReview.Gate); err != nil {
		return err
	}
	return validateFileClass(cfg.FileClass)
}

// validateFileClass checks file_class glob patterns and mock framework entries.
// GeneratedGlobs and TestGlobs are passed to doublestar.Match so they must be
// valid glob syntax; MockFrameworks are plain prefix/suffix strings — only
// emptiness is checked.
func validateFileClass(fc FileClassDef) error {
	for i, pat := range fc.GeneratedGlobs {
		if pat == "" {
			return fmt.Errorf("file_class.generated_globs[%d] must not be empty", i)
		}
		if !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("file_class.generated_globs[%d] %q is not a valid glob pattern", i, pat)
		}
	}
	for i, pat := range fc.TestGlobs {
		if pat == "" {
			return fmt.Errorf("file_class.test_globs[%d] must not be empty", i)
		}
		if !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("file_class.test_globs[%d] %q is not a valid glob pattern", i, pat)
		}
	}
	for i, mf := range fc.MockFrameworks {
		if mf == "" {
			return fmt.Errorf("file_class.mock_frameworks[%d] must not be empty", i)
		}
	}
	return nil
}

// validateMetricEntry checks one metrics.<name> entry: a valid gate value and
// threshold knobs that actually apply to the metric's kind. A knob on a metric
// of the wrong kind is a hard error, not a silent no-op — a validated-but-inert
// setting hides the exact misconfiguration it was meant to express.
func validateMetricEntry(name string, knob metricKnob, e MetricEntry) error {
	if err := validateGate("metrics."+name, e.Gate); err != nil {
		return err
	}
	if e.MinDelta < 0 {
		return fmt.Errorf("metrics.%s.min_delta must be >= 0 (a tolerated drop, got %v)", name, e.MinDelta)
	}
	if e.MaxNew < 0 {
		return fmt.Errorf("metrics.%s.max_new must be >= 0 (an allowed increase, got %d)", name, e.MaxNew)
	}
	switch knob {
	case knobRatio:
		if e.MaxNew != 0 {
			return fmt.Errorf("metrics.%s.max_new applies only to count metrics (cycle, unbalanced_edge) — use min_delta", name)
		}
	case knobCount:
		if e.MinDelta != 0 {
			return fmt.Errorf("metrics.%s.min_delta applies only to ratio metrics (encapsulation, coverage) — use max_new", name)
		}
	case knobNone:
		if e.Gate != "" || e.MinDelta != 0 || e.MaxNew != 0 {
			return fmt.Errorf("metrics.%s is informational and never gates — only `enabled` applies", name)
		}
	}
	return nil
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

// Default returns a Config suitable for use when no archfit.yaml is present.
// All language modes are auto, coupling advisory minimum severity is medium, and
// no modules, layers, or rules are defined — only metric checks run.
func Default() Config {
	return Config{
		Version:  1,
		Coupling: CouplingConfig{MinSeverity: "medium"},
		Languages: LanguagesConfig{
			Go:         GoLanguage{Enabled: ModeAuto},
			TypeScript: TypeScriptLanguage{Enabled: ModeAuto},
			Python:     PythonLanguage{Enabled: ModeAuto},
			Rust:       RustLanguage{Enabled: ModeAuto},
		},
	}
}

// toolEntry is one language/analyzer entry for validation: its dotted YAML path,
// coverage gate, and optional subprocess timeout (empty when not applicable).
type toolEntry struct {
	path    string
	gate    GateMode
	timeout string
}

// toolEntries lists every language and analyzer in deterministic order so gate
// and timeout validation report a stable first offender.
func (c Config) toolEntries() []toolEntry {
	return []toolEntry{
		{"languages.go", c.Languages.Go.Gate, ""},
		{"languages.typescript", c.Languages.TypeScript.Gate, ""},
		{"languages.python", c.Languages.Python.Gate, ""},
		{"languages.rust", c.Languages.Rust.Gate, ""},
		{"analyzers.syntax", c.Analyzers.Syntax.Gate, ""},
		{"analyzers.scip", c.Analyzers.Scip.Gate, c.Analyzers.Scip.Timeout},
		{"analyzers.clones", c.Analyzers.Clones.Gate, c.Analyzers.Clones.Timeout},
		{"analyzers.cargo_modules", c.Analyzers.CargoModules.Gate, ""},
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
