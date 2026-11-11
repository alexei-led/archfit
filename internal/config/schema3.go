package config

import (
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// ReportConfig reads a schema 3 policy and returns the Config the report runs
// on. The report is the Archfit 3 analyze pipeline, which reads Config through
// its views (ModuleMapView, ForClassify, ForScope, ForExtract, RunOptions,
// PolicySnapshot and the others). It measures and does not gate (design D2).
//
// The modules come from the resolved policy, with the defaults of design 6.1.
// The language, analyzer and coverage keys that schema 3 keeps come from the
// decoded file, because the resolved policy holds them only as flat strings.
// Every Archfit 3 knob that has no schema 3 key keeps a value that gates
// nothing:
//   - no rules and no waivers;
//   - no coupling.gate: the distributed-monolith rule warns, and only fail
//     blocks;
//   - no metrics entries: the default metrics, whose gates need a baseline;
//   - no module_review and no tool gate: warn, so a missing tool is still
//     disclosed and blocks nothing (off would hide it);
//   - SCIP and clones off;
//   - coupling advisory tuning and AI as in Default.
//
// Module owners and deploy units stay empty, so the run resolves them from
// CODEOWNERS and git as before. The owners of schema 3 approve requests. They
// do not own modules.
//
// Known limits of this projection:
//   - Schema 3 paths are file globs. Go package nodes match a directory glob
//     (pkg/**). TypeScript and .NET file nodes match, and so do Rust crates,
//     which bind through a file in the crate directory. A Go glob that names
//     files (cmd/main.go) does not match a package node. Python nodes are
//     dotted ids (myapp.chaos), which no file glob matches. The report on a
//     Python repository therefore owns no module, and its coupling metrics are
//     n/a, not zero. Task 41 moves the report's Python evidence to file nodes.
//   - Internal stays empty. The check scores a use past public as functional,
//     and an Archfit 3 internal glob would force it to intrusive. With no
//     internal glob the analyzer's own evidence decides (Go types read a call
//     as functional). Without such evidence the scorer abstains.
func ReportConfig(data []byte) (Config, error) {
	p, err := policyfile.Resolve(data)
	if err != nil {
		return Config{}, err
	}
	f, err := policyfile.Decode(data)
	if err != nil {
		return Config{}, err
	}
	return reportConfig(f, p), nil
}

// reportConfig projects a resolved policy and the tool keys of its file.
func reportConfig(f policyfile.File, p resolved.Policy) Config {
	cfg := Default()
	cfg.Version = policyfile.Version
	cfg.Exclude = p.Exclude
	cfg.Layers = p.Layers
	cfg.Modules = make(map[string]policy.ModuleDef, len(p.Modules))
	for name, m := range p.Modules {
		cfg.Modules[name] = policy.ModuleDef{
			Paths:      m.Paths,
			Public:     m.Public,
			Layer:      m.Layer,
			Subdomain:  string(m.Subdomain),
			Volatility: string(m.Volatility),
			// In schema 3 no depends_on means no other module. In Archfit 3 a
			// nil list means any module, so the list is never nil.
			DependsOn: append([]string{}, m.DependsOn...),
		}
	}
	cfg.FileClass = FileClassDef{GeneratedGlobs: p.Generated, TestGlobs: p.Tests}

	l := f.Languages
	cfg.Languages = LanguagesConfig{
		Go:         GoLanguage{Enabled: toolMode(l.Go.Enabled), Modules: GoModuleFilter{Include: l.Go.Modules.Include, Exclude: l.Go.Modules.Exclude}},
		TypeScript: TypeScriptLanguage{Enabled: toolMode(l.TypeScript.Enabled)},
		Python:     PythonLanguage{Enabled: toolMode(l.Python.Enabled), Package: l.Python.Package},
		Rust: RustLanguage{
			Enabled: toolMode(l.Rust.Enabled), Manifest: l.Rust.Manifest,
			Features: l.Rust.Features, IncludeDevDeps: l.Rust.IncludeDevDeps,
		},
		DotNet: DotNetLanguage{Enabled: toolMode(l.DotNet.Enabled), Projects: l.DotNet.Projects},
	}
	a := f.Analyzers
	cfg.Analyzers = AnalyzersConfig{
		Syntax:       Analyzer{Enabled: toolMode(a.Syntax.Enabled)},
		Scip:         TimedAnalyzer{Enabled: ModeOff},
		Clones:       TimedAnalyzer{Enabled: ModeOff},
		CargoModules: Analyzer{Enabled: toolMode(a.CargoModules.Enabled)},
		Roslyn:       TimedAnalyzer{Enabled: toolMode(a.Roslyn.Enabled), Timeout: a.Roslyn.Timeout},
	}
	cfg.Coverage = CoverageConfig{Enabled: f.Coverage.Enabled}
	for _, s := range f.Coverage.Sources {
		cfg.Coverage.Sources = append(cfg.Coverage.Sources, CoverageSource{
			Path: s.Path, Format: string(s.Format), SidecarPath: s.SidecarPath,
			MaxBytes: s.MaxBytes, MaxFacts: s.MaxFacts,
		})
	}
	return cfg
}

// toolMode maps a schema 3 switch to the enable mode. An absent switch is
// auto, as in schema 2. Each analyzer reads auto as its own default: syntax
// and cargo_modules stay off, roslyn runs where its helper is installed.
func toolMode(s policyfile.Switch) evidenceports.ToolMode {
	switch s {
	case policyfile.SwitchOn:
		return ModeOn
	case policyfile.SwitchOff:
		return ModeOff
	default:
		return ModeAuto
	}
}
