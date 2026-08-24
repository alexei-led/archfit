// Package registry owns the concrete language-extractor registry and construction facade.
package registry

import (
	"slices"

	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/toolrun"
	"github.com/alexei-led/archfit/internal/view"
)

// Tool names one external binary archfit can probe, with a one-line
// install hint shown when it is missing. Cross-language (shared) tools stay
// literal in doctor.go; language-specific ones are sourced from the registry.
type Tool struct {
	Name        string
	Command     string
	InstallHint string
}

// Configs maps canonical language IDs to projected extractor configuration.
type Configs map[string]view.ExtractConfig

// Descriptor is one row of the language registry: everything cmd needs
// to wire a language into the pipeline, doctor, and install commands. Adding a
// language becomes one row here plus an internal/extract/<lang> package — no
// edits scattered across pipeline.go/doctor.go/install.go.
type Descriptor struct {
	// ID is the canonical config language name (config.LangGo etc.) passed to
	// cfg.ForExtract and used as the Tools-map gate key.
	ID string
	// Aliases are the extra --lang / install short forms that resolve to ID (the
	// ID itself always resolves) — e.g. typescript ← "ts".
	Aliases []string
	// ProjectPresent reports whether this language's extractor has a project to
	// analyse under root. It is the coverage layer's applicability probe: when it
	// says "not present", an absent primary analyzer is read as "the language is
	// not in this tree" rather than a coverage gap or a declared opt-out.
	//
	// Every row MUST answer by calling the extractor's own applicability code
	// (golang.AnalysableMembers, ts.Applicable, py.Applicable, rust.Applicable) —
	// never a hand-rolled marker list. A parallel reimplementation is how "we did
	// not look" repeatedly rendered as "there is nothing here": a marker the
	// extractor ignores (tsconfig.json without package.json, setup.cfg, a go.mod
	// the module filter removes) fabricates presence, and a marker it accepts but
	// the list omits (a configured python package dir, a sub-crate Cargo.toml, a
	// go.work member) fabricates absence.
	ProjectPresent func(root string, cfg view.ExtractConfig) bool
	// NewExtractor builds the language's ports.Extractor from the shared runner,
	// the language's projected ExtractConfig view, and the fact-cache store.
	// Store.RefreshMode lets a caller force fresh extraction while still writing
	// the refreshed fact back to disk.
	NewExtractor func(toolrun.Runner, view.ExtractConfig, *factcache.Store) ports.Extractor
	// PrimaryTool is the coverage name of the dependency-graph analyzer this
	// language unlocks (as it appears in ToolCoverage, e.g. "go/packages").
	PrimaryTool string
	// InstallHint is the one-line install command for PrimaryTool, shown in the
	// coverage-gap block when the analyzer is absent.
	InstallHint string
	// DoctorTools are the language-specific binaries `archfit doctor` probes.
	DoctorTools []Tool
}

// languages is the single ordered source of truth for supported
// languages. Extractor build order (go → ts → py → rust) is load-bearing: the
// graph merge dedups by NodeConvention priority but ties resolve by insertion
// order, and the engine golden test pins it. Append new languages; never reorder.
// Primary analyzer and optional Rust tool names.
const (
	ToolGoPackages   = "go/packages"
	ToolDepCruiser   = "dependency-cruiser"
	ToolGrimp        = "grimp"
	ToolCargo        = "cargo"
	ToolCargoModules = "cargo-modules"

	SCIPGo         = "scip-go"
	SCIPTypeScript = "scip-typescript"
	SCIPPython     = "scip-python"
	SCIPRust       = "rust-analyzer"
)

var languages = []Descriptor{
	{
		ID:             "go",
		ProjectPresent: goProjectPresent,
		NewExtractor: func(r toolrun.Runner, cfg view.ExtractConfig, fc *factcache.Store) ports.Extractor {
			ex := golang.New(cfg)
			ex.Runner = r // go-toolchain version probe for the fact-cache key
			ex.Cache = fc
			return ex
		},
		PrimaryTool: ToolGoPackages,
		InstallHint: "https://go.dev/dl (bundled with the Go toolchain)",
		DoctorTools: []Tool{
			{"go", "go", "https://go.dev/dl"},
			{SCIPGo, SCIPGo, "go install github.com/sourcegraph/scip-go/cmd/scip-go@latest"},
		},
	},
	{
		ID:             "typescript",
		Aliases:        []string{"ts"},
		ProjectPresent: tsProjectPresent,
		NewExtractor: func(r toolrun.Runner, cfg view.ExtractConfig, fc *factcache.Store) ports.Extractor {
			ex := ts.New(r, cfg)
			ex.Cache = fc
			return ex
		},
		PrimaryTool: ToolDepCruiser,
		InstallHint: "npm install -g dependency-cruiser",
		DoctorTools: []Tool{
			{"node", "node", "https://nodejs.org"},
			{"bunx", "bunx", "https://bun.sh"},
			{"npx", "npx", "ships with node"},
			{SCIPTypeScript, SCIPTypeScript, "npm install -g @sourcegraph/scip-typescript"},
		},
	},
	{
		ID:             "python",
		Aliases:        []string{"py"},
		ProjectPresent: pyProjectPresent,
		NewExtractor: func(r toolrun.Runner, cfg view.ExtractConfig, fc *factcache.Store) ports.Extractor {
			ex := py.New(r, cfg)
			ex.Cache = fc
			return ex
		},
		PrimaryTool: ToolGrimp,
		InstallHint: "uv tool install grimp / pip install grimp",
		DoctorTools: []Tool{
			{"python3", "python3", "https://www.python.org/downloads"},
			{SCIPPython, SCIPPython, "npm install -g @sourcegraph/scip-python"},
		},
	},
	{
		ID:             "rust",
		Aliases:        []string{"rs"},
		ProjectPresent: rustProjectPresent,
		NewExtractor: func(r toolrun.Runner, cfg view.ExtractConfig, fc *factcache.Store) ports.Extractor {
			ex := rust.New(r, cfg)
			ex.Cache = fc
			return ex
		},
		PrimaryTool: ToolCargo,
		InstallHint: "https://rustup.rs (rustup installs cargo)",
		DoctorTools: []Tool{
			{ToolCargo, ToolCargo, "https://rustup.rs"},
			{SCIPRust, SCIPRust, "rustup component add rust-analyzer"},
			{ToolCargoModules, ToolCargoModules, "cargo install cargo-modules (opt-in: analyzers.cargo_modules.enabled: true)"},
		},
	},
}

// All returns the ordered language descriptors.
func All() []Descriptor {
	return slices.Clone(languages)
}

func goProjectPresent(root string, cfg view.ExtractConfig) bool {
	members, err := golang.AnalysableMembers(root, cfg.Exclusions, cfg.GoModuleInclude, cfg.GoModuleExclude)
	return err != nil || len(members.Dirs) > 0
}

func tsProjectPresent(root string, _ view.ExtractConfig) bool {
	return ts.Applicable(root)
}

func pyProjectPresent(root string, cfg view.ExtractConfig) bool {
	return py.Applicable(root, cfg.PyPackage)
}

func rustProjectPresent(root string, cfg view.ExtractConfig) bool {
	return rust.Applicable(root, cfg.CargoManifest)
}

// GoWorkOff reports whether the Go toolchain must be told to ignore the go.work
// governing scanRoot, by asking the SAME discovery the Go extractor runs. It is
// a whole-run fact, not a Go-extractor detail: any Go-toolchain subprocess the
// run starts (today scip-go) sees the same workspace and must reach the same
// conclusion, or two analyzers report contradictory coverage over one tree.
//
// False whenever discovery is unavailable or errors — never disable a workspace
// on a guess.
func GoWorkOff(scanRoot string, cfg view.ExtractConfig) bool {
	m, err := golang.DiscoverMembers(scanRoot, cfg.Exclusions)
	return err == nil && m.GoWorkOff
}

// New constructs the registered extractor for one canonical language ID.
func New(id string, runner toolrun.Runner, cfg view.ExtractConfig, facts *factcache.Store) ports.Extractor {
	for _, lang := range languages {
		if lang.ID == id {
			return lang.NewExtractor(runner, cfg, facts)
		}
	}
	return nil
}

// Build instantiates the per-language extractors in registry order,
// each fed its projected ExtractConfig view. The slice order is the graph-merge
// order the engine golden test pins — registry order is go → ts → py.
func Build(runner toolrun.Runner, configs Configs, facts *factcache.Store) []ports.Extractor {
	exs := make([]ports.Extractor, 0, len(languages))
	for _, lang := range languages {
		exs = append(exs, lang.NewExtractor(runner, configs[lang.ID], facts))
	}
	return exs
}

// ProjectPresent reports whether the registered language applies under root.
func ProjectPresent(id, root string, cfg view.ExtractConfig) bool {
	for _, lang := range languages {
		if lang.ID == id {
			return lang.ProjectPresent(root, cfg)
		}
	}
	return false
}

// ByAlias resolves a --lang / install key (canonical ID or any alias)
// to the canonical language ID. Returns "" for an unknown key.
func ByAlias(key string) string {
	for _, lang := range languages {
		if key == lang.ID || slices.Contains(lang.Aliases, key) {
			return lang.ID
		}
	}
	return ""
}

// RustExtractor returns the *rust.Extractor from the extractor slice, or nil if
// the Rust extractor is not present. Used by the pipeline to collect the opt-in
// cargo-modules module-graph coverage record after engine.Run.
func RustExtractor(exs []ports.Extractor) *rust.Extractor {
	for _, ex := range exs {
		if re, ok := ex.(*rust.Extractor); ok {
			return re
		}
	}
	return nil
}

// PrimaryTools returns the dependency-graph analyzer coverage names in
// registry order. Injected into engine.RunInput so score synthesis names the
// primary extractors without the core ring hardcoding tool strings.
func PrimaryTools() []string {
	tools := make([]string, 0, len(languages))
	for _, lang := range languages {
		tools = append(tools, lang.PrimaryTool)
	}
	return tools
}
