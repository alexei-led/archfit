package main

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

// Volatility enum values used in classify validation and tests.
const (
	volatilityLow    = "low"
	volatilityMedium = "medium"
	volatilityHigh   = "high"
)

// Optional-analyzer tool names shared by doctor and the coverage-gap table.
const (
	toolLizard = "lizard"
	toolJscpd  = "jscpd"
)

// Primary dependency-graph analyzer coverage names (as they appear in
// ToolCoverage). Their absence drops the structural metrics to n/a; shared by the
// coverage-gap table and its config-key map.
const (
	toolGoPackages   = "go/packages"
	toolDepCruiser   = "dependency-cruiser"
	toolGrimp        = "grimp"
	toolCargo        = "cargo"
	toolCargoModules = "cargo-modules" // opt-in intra-crate module graph; mirrors config.ToolCargoModules
)

// SCIP indexer binary names shared by the language registry (DoctorTools) and
// the doctor command.
const (
	scipGo         = "scip-go"
	scipTypeScript = "scip-typescript"
	scipPython     = "scip-python"
	scipRust       = "rust-analyzer"
)

// Project-marker filenames used by the language registry and test fixtures.
const (
	markerGoMod     = "go.mod"
	markerCargoToml = "Cargo.toml"
)
