package pipeline

import (
	"fmt"
	"io"
	"maps"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/config"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
)

// PolicySnapshot converts decoded config at the application boundary. The
// config package remains a YAML adapter and the policy package remains free of
// decode concerns.
func PolicySnapshot(cfg config.Config) policy.PolicySnapshot {
	owners := make(map[string]string, len(cfg.Modules))
	deployUnits := make(map[string]string, len(cfg.Modules))
	for name, def := range cfg.Modules {
		if def.Owner != "" {
			owners[name] = def.Owner
		}
		if def.DeployUnit != "" {
			deployUnits[name] = def.DeployUnit
		}
	}
	topology := policy.TopologyView{Modules: cfg.Modules, Layers: cfg.Layers, ModuleMap: policy.BuildModuleMap(cfg.Modules), ExternalSystems: cfg.ExternalSystems, ExplicitOwners: cfg.ExplicitOwnersView()}
	relationship := policy.RelationshipPolicy{MinimumSeverity: cfg.Coupling.MinSeverity, VolatilityCascadeEnabled: cfg.Coupling.VolatilityCascade, DuplicatedKnowledge: policy.NormalizeDuplicatedKnowledgePolicy(cfg.Coupling.DuplicatedKnowledge)}
	stale := cfg.ForStaleness()
	assessment := policy.AssessmentPolicy{Waivers: cfg.ForWaivers(), Staleness: policy.StalenessPolicy{Enabled: stale.Enabled, Threshold: stale.Threshold}}
	gate := policy.CouplingGate{}
	if g := cfg.Coupling.Gate; g != nil {
		maxDrop := g.MaxDrop
		if maxDrop != nil {
			v := *maxDrop
			maxDrop = &v
		}
		gate = policy.CouplingGate{Enabled: true, MinBand: g.MinBand, MaxDrop: maxDrop}
	}
	return policy.New(topology, relationship, assessment, policy.GatePolicy{Rules: cfg.ForRules(), Metrics: cfg.Metrics, Coupling: gate, ModuleReview: policy.GateMode(cfg.ModuleReview.Gate)}, owners, deployUnits)
}

// CoverageOptions is the narrow analyzer activation input built by the config adapter.
type CoverageOptions struct {
	Gates          map[string]string
	Modes          map[string]string
	ProjectPresent map[string]func(string) bool
}

// Coverage projects analyzer gates and applicability probes without exposing config.
func Coverage(cfg config.Config) CoverageOptions {
	gates := make(map[string]string)
	modes := make(map[string]string)
	for _, lang := range registry.All() {
		gates[lang.ID] = string(cfg.ToolGate(lang.ID))
		modes[lang.ID] = string(cfg.ToolMode(lang.ID))
	}
	gates[config.ToolClones] = string(cfg.ToolGate(config.ToolClones))
	gates[config.ToolCargoModules] = string(cfg.ToolGate(config.ToolCargoModules))
	probes := make(map[string]func(string) bool)
	for _, lang := range registry.All() {
		probe := lang.ProjectPresent
		extractCfg := cfg.ForExtract(lang.ID)
		tool := lang.PrimaryTool
		probes[tool] = func(root string) bool { return probe(root, extractCfg) }
		if lang.ID == config.LangRust {
			probes[toolCargoModules] = func(root string) bool { return probe(root, extractCfg) }
		}
	}
	return CoverageOptions{Gates: gates, Modes: modes, ProjectPresent: probes}
}

// RunOptions is the narrow application input built by the config adapter. Policy
// declarations are deliberately absent; they travel only in PolicySnapshot.
type RunOptions struct {
	Exclusions   []string
	Scope        scope.Config
	Extractors   registry.Configs
	Acquisition  acquire.Options
	Syntax       evidenceports.SyntaxConfig
	Patterns     pattern.Config
	LintWarnings []string
	Coverage     CoverageOptions
}

// ValidateConfigRules validates decoded rule definitions at the adapter boundary.
func ValidateConfigRules(cfg config.Config) error {
	_, err := evaluation.NewRuleset(cfg.ForRules())
	return err
}

// ApplyFlagOverrides applies command-line overrides to decoded config.
func ApplyFlagOverrides(cfg *config.Config, severity string, lang []string) error {
	if severity != "" {
		cfg.Coupling.MinSeverity = severity
	}
	for _, key := range lang {
		canonical := registry.ByAlias(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown analyzer %q; see %s", key, languagesDocsURL)
		}
		cfg.SetToolMode(canonical, config.ModeOn)
	}
	return nil
}

// PrintConfigLint writes config-quality warnings to w.
func PrintConfigLint(w io.Writer, warnings []config.LintWarning) {
	if len(warnings) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "config-quality: %d module(s) under-specified — degrades distance/volatility classification (can cause BC advisory floods):\n", len(warnings))
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(w, "  - %s\n", warning.String())
	}
}

// WithIndependentModules copies the decoded module map for a base-tree run.
func WithIndependentModules(cfg config.Config) config.Config {
	modules := make(map[string]policy.ModuleDef, len(cfg.Modules))
	maps.Copy(modules, cfg.Modules)
	cfg.Modules = modules
	return cfg
}

// AnalyzerOptions records which optional analyzer families a config activated.
type AnalyzerOptions struct {
	Patterns, Syntax, SCIP, Clones, CargoModules bool
}

// AnalyzerSettings projects config into the activated analyzer families.
func AnalyzerSettings(cfg config.Config) AnalyzerOptions {
	return AnalyzerOptions{Patterns: len(cfg.ForPatterns()) > 0, Syntax: cfg.ForSyntax().Enabled, SCIP: cfg.ScipEnabled(), Clones: cfg.ClonesEnabled(), CargoModules: cfg.CargoModulesEnabled()}
}

// Options projects decoded config into non-policy runtime inputs.
func Options(cfg config.Config) RunOptions {
	exclusions := scope.MergeExclusions(cfg.Exclude)
	// Every stage must see the same normalized exclusions. Projecting extractors
	// or applicability probes from the raw config would drop built-in excludes
	// and !re-includes even though scope/acquisition use the merged set.
	cfg.Exclude = exclusions
	acquisition := AcquisitionOptions(cfg)
	acquisition.Exclusions = exclusions
	scopeConfig := cfg.ForScope()
	scopeConfig.Exclusions = exclusions
	lint := cfg.Lint()
	lintWarnings := make([]string, 0, len(lint))
	for _, warning := range lint {
		lintWarnings = append(lintWarnings, warning.String())
	}
	return RunOptions{Exclusions: exclusions, Scope: scopeConfig, Extractors: ExtractConfigs(cfg), Acquisition: acquisition, Syntax: cfg.ForSyntax(), Patterns: cfg.ForPatterns(), LintWarnings: lintWarnings, Coverage: Coverage(cfg)}
}

// ExtractConfigs projects config into registry extractor configs.
func ExtractConfigs(cfg config.Config) registry.Configs {
	return registry.Configs{
		config.LangGo:         cfg.ForExtract(config.LangGo),
		config.LangTypeScript: cfg.ForExtract(config.LangTypeScript),
		config.LangPython:     cfg.ForExtract(config.LangPython),
		config.LangRust:       cfg.ForExtract(config.LangRust),
	}
}

// AcquisitionOptions projects config into external acquisition options.
func AcquisitionOptions(cfg config.Config) acquire.Options {
	return acquire.Options{
		Exclusions: cfg.Exclude, FileClass: cfg.ForFileClass(), ModuleMap: cfg.ModuleMapView(),
		ClonesEnabled: cfg.ClonesEnabled(), CloneTimeout: cfg.ToolTimeout(config.ToolClones),
		SCIPEnabled: cfg.ScipEnabled(), SCIPTimeout: cfg.ToolTimeout(config.ToolScip),
		Syntax: cfg.ForSyntax(), GoExtract: cfg.ForExtract(config.LangGo),
	}
}
