package main

import "github.com/alexei-led/archfit/internal/extract/registry"

// LLM provider name constants shared across init, enrich, doctor, and explain.
const (
	providerAnthropic = "anthropic"
	providerOpenAI    = "openai"
	providerOllama    = "ollama"
)

// defaultLLMModel is the CLI flag default model (provider defaults are kong
// string tags on each command).
const defaultLLMModel = "claude-opus-4-8"

// Subdomain enum values used in classify validation and tests.
const (
	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"
)

// Layer name constants used in tests.
const (
	layerCore = "core"
)

const distanceBasisCodeStructure = "code_structure"

// Volatility enum values used in classify validation and tests.
const (
	volatilityLow        = "low"
	volatilityMedium     = "medium"
	volatilityHigh       = "high"
	volatilityFrozen     = "frozen"
	volatilityLegacy     = "legacy"
	volatilityUndeclared = "undeclared"
)

// Optional-analyzer tool names shared by doctor and the coverage-gap table.
const (
	toolLoc           = "loc"             // always-on LOC walk; used in test assertions
	toolAstGrep       = "ast-grep"        // pattern pass
	toolAstGrepSyntax = "ast-grep/syntax" // syntax pass; its own row, so tool_coverage never duplicates a name
	toolDeployUnit    = "deploy-unit"
	toolScip          = "scip"
	toolScipSymbols   = "scip-symbols" // SCIP symbol-graph coverage row (distinct from the strength row "scip")
	toolJscpd         = "jscpd"
)

// Primary dependency-graph analyzer coverage names (as they appear in
// ToolCoverage). Their absence drops the structural metrics to n/a; shared by the
// coverage-gap table and its config-key map.
const (
	toolGoPackages   = registry.ToolGoPackages
	toolDepCruiser   = registry.ToolDepCruiser
	toolGrimp        = registry.ToolGrimp
	toolCargo        = registry.ToolCargo
	toolCargoModules = registry.ToolCargoModules // opt-in intra-crate module graph; mirrors config.ToolCargoModules
)

// Project-marker filenames used by the language registry and test fixtures.
const (
	markerGoMod     = "go.mod"
	markerCargoToml = "Cargo.toml"
)
