package config

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/v3/internal/extract/acquire"
	suppliedcoverage "github.com/alexei-led/archfit/v3/internal/extract/coverage"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

const languagesDocsURL = "https://github.com/alexei-led/archfit/blob/main/docs/languages.md"

// PolicySnapshot converts decoded config into the authoritative policy model.
// The config package remains a YAML lifecycle adapter and the policy package
// remains free of decode concerns.
func (c Config) PolicySnapshot() policy.PolicySnapshot {
	owners := make(map[string]string, len(c.Modules))
	deployUnits := make(map[string]string, len(c.Modules))
	for name, def := range c.Modules {
		if def.Owner != "" {
			owners[name] = def.Owner
		}
		if def.DeployUnit != "" {
			deployUnits[name] = def.DeployUnit
		}
	}
	topology := policy.TopologyView{Modules: c.Modules, Layers: c.Layers, ModuleMap: policy.BuildModuleMap(c.Modules), ExternalSystems: c.ExternalSystems}
	relationship := policy.RelationshipPolicy{MinimumSeverity: c.Coupling.MinSeverity, VolatilityCascadeEnabled: c.Coupling.VolatilityCascade}
	stale := c.ForStaleness()
	assessment := policy.AssessmentPolicy{
		Staleness:            policy.StalenessPolicy{Enabled: stale.Enabled, Threshold: stale.Threshold},
		FunctionLOCThreshold: c.Metrics.FunctionLOCThresholdValue(),
	}
	return policy.New(topology, relationship, assessment, policy.GatePolicy{Metrics: c.Metrics.MetricEntries(), ModuleReview: policy.GateMode(c.ModuleReview.Gate), UncoveredChange: policy.GateMode(c.ModuleReview.ChangeGate)}, owners, deployUnits)
}

// CoverageOptions projects analyzer gates and applicability probes. Every probe
// is the language's OWN applicability function — the one its extractor calls —
// so a probe can never disagree with the extractor that produced the row.
func (c Config) CoverageOptions() acquisition.CoverageOptions {
	gates := make(map[string]string)
	modes := make(map[string]string)
	for _, lang := range registry.All() {
		gates[lang.ID] = string(c.ToolGate(lang.ID))
		modes[lang.ID] = string(c.ToolMode(lang.ID))
	}
	gates[ToolCargoModules] = string(c.ToolGate(ToolCargoModules))
	gates[ToolRoslyn] = string(c.ToolGate(ToolRoslyn))
	modes[ToolRoslyn] = string(c.ToolMode(ToolRoslyn))
	gates[suppliedcoverage.ToolName] = string(c.Coverage.Gate)
	probes := make(map[string]func(string) bool)
	for _, lang := range registry.All() {
		probe := lang.ProjectPresent
		extractCfg := c.ForExtract(lang.ID)
		probes[lang.PrimaryTool] = func(root string) bool { return probe(root, extractCfg) }
		if lang.ID == LangRust {
			probes[registry.ToolCargoModules] = func(root string) bool { return probe(root, extractCfg) }
		}
		if lang.ID == LangDotNet {
			probes[registry.ToolRoslyn] = func(root string) bool { return probe(root, extractCfg) }
		}
	}
	return acquisition.CoverageOptions{Gates: gates, Modes: modes, ProjectPresent: probes}
}

// SuppliedCoverageOptions projects only the artifact-ingestion contract into
// its filesystem adapter. The adapter receives no architecture policy.
func (c Config) SuppliedCoverageOptions() suppliedcoverage.Options {
	sources := make([]suppliedcoverage.Source, len(c.Coverage.Sources))
	for i, source := range c.Coverage.Sources {
		format := source.Format
		if format == "" {
			format = suppliedcoverage.FormatAuto
		}
		maxBytes := suppliedcoverage.DefaultMaxBytes
		if source.MaxBytes != nil {
			maxBytes = *source.MaxBytes
		}
		maxFacts := suppliedcoverage.DefaultMaxFacts
		if source.MaxFacts != nil {
			maxFacts = *source.MaxFacts
		}
		sources[i] = suppliedcoverage.Source{
			Path: source.Path, Format: format, SidecarPath: source.SidecarPath,
			MaxBytes: maxBytes, MaxFacts: maxFacts,
		}
	}
	return suppliedcoverage.Options{Enabled: c.Coverage.Enabled, Gate: string(c.Coverage.Gate), Sources: sources}
}

// RunOptions projects decoded config into the non-policy acquisition inputs.
func (c Config) RunOptions() acquisition.RunOptions {
	exclusions := scope.MergeExclusions(c.Exclude)
	// Every stage must see the same normalized exclusions. Projecting extractors
	// or applicability probes from the raw config would drop built-in excludes
	// and !re-includes even though scope/acquisition use the merged set.
	c.Exclude = exclusions
	acquisitionOpts := c.AcquisitionOptions()
	acquisitionOpts.Exclusions = exclusions
	scopeConfig := c.ForScope()
	scopeConfig.Exclusions = exclusions
	return acquisition.RunOptions{Exclusions: exclusions, Scope: scopeConfig, Extractors: c.ExtractConfigs(), Acquisition: acquisitionOpts, Syntax: c.ForSyntax(), Lint: lintWarningStrings, Coverage: c.CoverageOptions(), SuppliedCoverage: c.SuppliedCoverageOptions()}
}

// lintWarningStrings renders LintModules for a resolved module set. Acquisition
// calls it after ownership resolution, so a module whose owner CODEOWNERS filled
// no longer reports as omitting one.
func lintWarningStrings(modules map[string]policy.ModuleDef) []string {
	lint := LintModules(modules)
	out := make([]string, 0, len(lint))
	for _, warning := range lint {
		out = append(out, warning.String())
	}
	return out
}

// ExtractConfigs projects config into registry extractor configs.
func (c Config) ExtractConfigs() registry.Configs {
	// Iterated from the registry, not hand-listed: registry.Build indexes this
	// map by every registered language, so a language added there but missed
	// here would receive the ZERO ExtractConfig — empty mode, no exclusions —
	// and still construct an extractor. CoverageOptions() already iterates
	// registry.All() for exactly this reason; the two must not disagree about
	// which languages exist.
	all := registry.All()
	out := make(registry.Configs, len(all))
	for _, lang := range all {
		out[lang.ID] = c.ForExtract(lang.ID)
	}
	return out
}

// AcquisitionOptions projects config into external acquisition options.
func (c Config) AcquisitionOptions() acquire.Options {
	return acquire.Options{
		Exclusions: c.Exclude, FileClass: c.ForFileClass(), ModuleMap: c.ModuleMapView(),
		Syntax: c.ForSyntax(), GoExtract: c.ForExtract(LangGo),
	}
}

// ApplyFlagOverrides applies command-line overrides to decoded config.
func ApplyFlagOverrides(cfg *Config, severity string, lang []string) error {
	if severity != "" {
		cfg.Coupling.MinSeverity = severity
	}
	for _, key := range lang {
		canonical := registry.ByAlias(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown language %q; see %s", key, languagesDocsURL)
		}
		cfg.SetToolMode(canonical, ModeOn)
	}
	return nil
}

// Preparer discloses report config warnings before evidence acquisition.
// File decoding validates syntax; this stage does not evaluate legacy rules.
type Preparer struct {
	Config Config
	Stderr io.Writer
	// DiscloseLint prints the config-warning block when the report use case
	// owns stderr. Internal measurement callers can suppress this disclosure.
	DiscloseLint bool
}

// Prepare writes config-quality warnings when disclosure is enabled.
func (p Preparer) Prepare(context.Context) error {
	if p.DiscloseLint {
		PrintLint(p.stderr(), p.Config.Lint())
	}
	return nil
}

func (p Preparer) stderr() io.Writer {
	if p.Stderr != nil {
		return p.Stderr
	}
	return os.Stderr
}

// PrintLint writes config-quality warnings to w.
func PrintLint(w io.Writer, warnings []LintWarning) {
	if len(warnings) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "config-quality: %d module(s) under-specified — degrades distance/volatility classification (can cause BC advisory floods):\n", len(warnings))
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(w, "  - %s\n", warning.String())
	}
}
