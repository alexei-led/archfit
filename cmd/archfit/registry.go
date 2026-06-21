package main

import (
	"slices"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// doctorTool names one external binary archfit can probe, with a one-line
// install hint shown when it is missing. Cross-language (shared) tools stay
// literal in doctor.go; language-specific ones are sourced from the registry.
type doctorTool struct {
	name    string
	cmd     string
	install string
}

// LanguageDescriptor is one row of the language registry: everything cmd needs
// to wire a language into the pipeline, doctor, and install commands. Adding a
// language becomes one row here plus an internal/extract/<lang> package — no
// edits scattered across pipeline.go/doctor.go/install.go.
type LanguageDescriptor struct {
	// ID is the canonical config language name (config.LangGo etc.) passed to
	// cfg.ForExtract and used as the Tools-map gate key.
	ID string
	// Aliases are the extra --lang / install short forms that resolve to ID (the
	// ID itself always resolves) — e.g. typescript ← "ts".
	Aliases []string
	// ProjectMarkers are repo-root filenames that signal the language is present
	// (e.g. go.mod, package.json, Cargo.toml). Used by language discovery.
	ProjectMarkers []string
	// NewExtractor builds the language's ports.Extractor from the shared runner
	// and the language's projected ExtractConfig view.
	NewExtractor func(toolrun.Runner, config.ExtractConfig) ports.Extractor
	// PrimaryTool is the coverage name of the dependency-graph analyzer this
	// language unlocks (as it appears in ToolCoverage, e.g. "go/packages").
	PrimaryTool string
	// InstallHint is the one-line install command for PrimaryTool, shown in the
	// coverage-gap block when the analyzer is absent.
	InstallHint string
	// DoctorTools are the language-specific binaries `archfit doctor` probes.
	DoctorTools []doctorTool
	// SCIPIndexer is the scip indexer binary that provides symbol-level strength
	// for this language (empty when none). Wired by extract/scip.
	SCIPIndexer string
}

// languageRegistry is the single ordered source of truth for supported
// languages. Extractor build order (go → ts → py → rust) is load-bearing: the
// graph merge dedups by NodeConvention priority but ties resolve by insertion
// order, and the engine golden test pins it. Append new languages; never reorder.
var languageRegistry = []LanguageDescriptor{
	{
		ID:             config.LangGo,
		ProjectMarkers: []string{markerGoMod},
		NewExtractor:   func(_ toolrun.Runner, cfg config.ExtractConfig) ports.Extractor { return golang.New(cfg) },
		PrimaryTool:    toolGoPackages,
		InstallHint:    "https://go.dev/dl (bundled with the Go toolchain)",
		DoctorTools: []doctorTool{
			{"go", "go", "https://go.dev/dl"},
			{scipGo, scipGo, "go install github.com/sourcegraph/scip-go/cmd/scip-go@latest"},
		},
		SCIPIndexer: scipGo,
	},
	{
		ID:             config.LangTypeScript,
		Aliases:        []string{"ts"},
		ProjectMarkers: []string{"package.json", "tsconfig.json"},
		NewExtractor:   func(r toolrun.Runner, cfg config.ExtractConfig) ports.Extractor { return ts.New(r, cfg) },
		PrimaryTool:    toolDepCruiser,
		InstallHint:    "npm install -g dependency-cruiser",
		DoctorTools: []doctorTool{
			{"node", "node", "https://nodejs.org"},
			{"bunx", "bunx", "https://bun.sh"},
			{"npx", "npx", "ships with node"},
			{scipTypeScript, scipTypeScript, "npm install -g @sourcegraph/scip-typescript"},
		},
		SCIPIndexer: scipTypeScript,
	},
	{
		ID:             config.LangPython,
		Aliases:        []string{"py"},
		ProjectMarkers: []string{"pyproject.toml", "setup.py", "setup.cfg"},
		NewExtractor:   func(r toolrun.Runner, cfg config.ExtractConfig) ports.Extractor { return py.New(r, cfg) },
		PrimaryTool:    toolGrimp,
		InstallHint:    "uv tool install grimp / pip install grimp",
		DoctorTools: []doctorTool{
			{"python3", "python3", "https://www.python.org/downloads"},
			{scipPython, scipPython, "npm install -g @sourcegraph/scip-python"},
		},
		SCIPIndexer: scipPython,
	},
	{
		ID:             config.LangRust,
		Aliases:        []string{"rs"},
		ProjectMarkers: []string{markerCargoToml},
		NewExtractor:   func(r toolrun.Runner, cfg config.ExtractConfig) ports.Extractor { return rust.New(r, cfg) },
		PrimaryTool:    toolCargo,
		InstallHint:    "https://rustup.rs (rustup installs cargo)",
		DoctorTools: []doctorTool{
			{toolCargo, toolCargo, "https://rustup.rs"},
			{scipRust, scipRust, "rustup component add rust-analyzer"},
		},
		SCIPIndexer: scipRust,
	},
}

// buildExtractors instantiates the per-language extractors in registry order,
// each fed its projected ExtractConfig view. The slice order is the graph-merge
// order the engine golden test pins — registry order is go → ts → py.
func buildExtractors(runner toolrun.Runner, cfg config.Config) []ports.Extractor {
	exs := make([]ports.Extractor, 0, len(languageRegistry))
	for _, lang := range languageRegistry {
		exs = append(exs, lang.NewExtractor(runner, cfg.ForExtract(lang.ID)))
	}
	return exs
}

// languageByAlias resolves a --lang / install key (canonical ID or any alias)
// to the canonical language ID. Returns "" for an unknown key.
func languageByAlias(key string) string {
	for _, lang := range languageRegistry {
		if key == lang.ID || slices.Contains(lang.Aliases, key) {
			return lang.ID
		}
	}
	return ""
}

// primaryExtractorTools returns the dependency-graph analyzer coverage names in
// registry order. Injected into engine.RunInput so score synthesis names the
// primary extractors without the core ring hardcoding tool strings.
func primaryExtractorTools() []string {
	tools := make([]string, 0, len(languageRegistry))
	for _, lang := range languageRegistry {
		tools = append(tools, lang.PrimaryTool)
	}
	return tools
}
