package config

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
)

const languagesDocsURL = "https://github.com/alexei-led/archfit/blob/main/docs/guide/languages.md"

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
	topology := policy.TopologyView{Modules: c.Modules, Layers: c.Layers, ModuleMap: policy.BuildModuleMap(c.Modules), ExternalSystems: c.ExternalSystems, ExplicitOwners: c.ExplicitOwnersView()}
	relationship := policy.RelationshipPolicy{MinimumSeverity: c.Coupling.MinSeverity, VolatilityCascadeEnabled: c.Coupling.VolatilityCascade, DuplicatedKnowledge: policy.NormalizeDuplicatedKnowledgePolicy(c.Coupling.DuplicatedKnowledge)}
	stale := c.ForStaleness()
	assessment := policy.AssessmentPolicy{Waivers: c.ForWaivers(), Staleness: policy.StalenessPolicy{Enabled: stale.Enabled, Threshold: stale.Threshold}}
	gate := policy.CouplingGate{Mode: policy.DistributedMonolithWarn}
	if g := c.Coupling.Gate; g != nil && g.DistributedMonolith != nil {
		if m := g.DistributedMonolith.Mode; m != "" {
			gate.Mode = policy.DistributedMonolithMode(m)
		}
		if n := g.DistributedMonolith.MaxNewSeams; n != nil {
			gate.MaxNewSeams = *n
		}
	}
	return policy.New(topology, relationship, assessment, policy.GatePolicy{Rules: c.ForRules(), Metrics: c.Metrics, Coupling: gate, ModuleReview: policy.GateMode(c.ModuleReview.Gate)}, owners, deployUnits)
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
	gates[ToolClones] = string(c.ToolGate(ToolClones))
	gates[ToolCargoModules] = string(c.ToolGate(ToolCargoModules))
	probes := make(map[string]func(string) bool)
	for _, lang := range registry.All() {
		probe := lang.ProjectPresent
		extractCfg := c.ForExtract(lang.ID)
		probes[lang.PrimaryTool] = func(root string) bool { return probe(root, extractCfg) }
		if lang.ID == LangRust {
			probes[registry.ToolCargoModules] = func(root string) bool { return probe(root, extractCfg) }
		}
	}
	return acquisition.CoverageOptions{Gates: gates, Modes: modes, ProjectPresent: probes}
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
	return acquisition.RunOptions{Exclusions: exclusions, Scope: scopeConfig, Extractors: c.ExtractConfigs(), Acquisition: acquisitionOpts, Syntax: c.ForSyntax(), Patterns: c.ForPatterns(), Lint: lintWarningStrings, Coverage: c.CoverageOptions()}
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
	return registry.Configs{
		LangGo:         c.ForExtract(LangGo),
		LangTypeScript: c.ForExtract(LangTypeScript),
		LangPython:     c.ForExtract(LangPython),
		LangRust:       c.ForExtract(LangRust),
	}
}

// AcquisitionOptions projects config into external acquisition options.
func (c Config) AcquisitionOptions() acquire.Options {
	return acquire.Options{
		Exclusions: c.Exclude, FileClass: c.ForFileClass(), ModuleMap: c.ModuleMapView(),
		ClonesEnabled: c.ClonesEnabled(), CloneTimeout: c.ToolTimeout(ToolClones),
		SCIPEnabled: c.ScipEnabled(), SCIPTimeout: c.ToolTimeout(ToolScip),
		Syntax: c.ForSyntax(), GoExtract: c.ForExtract(LangGo),
	}
}

// AnalyzerFamilies projects config into the optional analyzer families it
// activated. The git-origin delta compares two runs on exactly this set.
func (c Config) AnalyzerFamilies() application.AnalyzerFamilies {
	return application.AnalyzerFamilies{Patterns: len(c.ForPatterns()) > 0, Syntax: c.ForSyntax().Enabled, SCIP: c.ScipEnabled(), Clones: c.ClonesEnabled(), CargoModules: c.CargoModulesEnabled()}
}

// WithIndependentModules returns a copy whose module map is independent of the
// receiver's. Config is a value but its Modules map is shared, and the run's
// owner/deploy-unit backfill writes through it — without the copy a base-tree
// sub-run would inherit head-tree owners and skip its own resolution.
func (c Config) WithIndependentModules() Config {
	modules := make(map[string]policy.ModuleDef, len(c.Modules))
	maps.Copy(modules, c.Modules)
	c.Modules = modules
	return c
}

// ValidateRules validates decoded rule definitions at the adapter boundary.
func ValidateRules(cfg Config) error {
	_, err := evaluation.NewRuleset(cfg.ForRules())
	return err
}

// ApplyFlagOverrides applies command-line overrides to decoded config.
func ApplyFlagOverrides(cfg *Config, severity string, lang []string) error {
	if severity != "" {
		cfg.Coupling.MinSeverity = severity
	}
	for _, key := range lang {
		canonical := registry.ByAlias(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown analyzer %q; see %s", key, languagesDocsURL)
		}
		cfg.SetToolMode(canonical, ModeOn)
	}
	return nil
}

// Preparer implements the application's policy-preparation stage: it validates
// decoded policy and discloses config-quality problems before any tool runs.
// Validation belongs here because the rule language is this package's lifecycle.
type Preparer struct {
	Config Config
	Stderr io.Writer
	// DiscloseLint writes the config-quality block to stderr. Only analyze/check
	// set it. Every other stage runs the same executor without owning the user's
	// stderr conversation: baseline/explain/enrich never disclosed it, and
	// `config compare` builds TWO preparers over one stream, so an ungated
	// disclosure prints the block twice with nothing naming the side that
	// produced it — the labelling contract that command documents.
	DiscloseLint bool
}

// Prepare validates rule definitions and writes config-quality warnings.
func (p Preparer) Prepare(context.Context) error {
	if err := ValidateRules(p.Config); err != nil {
		return err
	}
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
