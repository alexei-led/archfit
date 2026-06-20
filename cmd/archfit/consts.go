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
	toolLizard   = "lizard"
	toolJscpd    = "jscpd"
	toolGitnexus = "gitnexus"
)

// Primary dependency-graph analyzer coverage names (as they appear in
// ToolCoverage). Their absence drops the structural metrics to n/a; shared by the
// coverage-gap table and its config-key map.
const (
	toolGoPackages = "go/packages"
	toolDepCruiser = "dependency-cruiser"
	toolGrimp      = "grimp"
)
