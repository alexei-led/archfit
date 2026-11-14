// Package config projects schema 3 policies into report measurement inputs.
package config

import (
	"sort"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Config is an internal report projection, not a policy file format.
// ReportConfig constructs it from the schema 3 policy decoder.
type Config struct {
	Exclude         []string
	Languages       LanguagesConfig
	Analyzers       AnalyzersConfig
	Coverage        CoverageConfig
	Coupling        CouplingConfig
	Layers          []string
	Modules         map[string]policy.ModuleDef
	ExternalSystems map[string]policy.ExternalSystemDef
	Metrics         MetricsConfig
	ModuleReview    ModuleReviewConfig
	FileClass       FileClassDef
}

const levelMedium = "medium"

// Default returns report measurement defaults.
func Default() Config {
	return Config{

		Coupling: CouplingConfig{
			MinSeverity: levelMedium,
		},
		Languages: LanguagesConfig{
			Go:         GoLanguage{Enabled: evidenceports.ModeAuto},
			TypeScript: TypeScriptLanguage{Enabled: evidenceports.ModeAuto},
			Python:     PythonLanguage{Enabled: evidenceports.ModeAuto},
			Rust:       RustLanguage{Enabled: evidenceports.ModeAuto},
			DotNet:     DotNetLanguage{Enabled: evidenceports.ModeAuto},
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

// sortedKeys returns a sorted slice of keys from any string-keyed map, so
// validation reports a deterministic first offender when multiple entries
// carry an invalid value.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
