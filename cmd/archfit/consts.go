package main

// LLM provider name constants shared across init, enrich, doctor, and explain.
const (
	providerAnthropic = "anthropic"
	providerOpenAI    = "openai"
	providerOllama    = "ollama"
)

// defaultLLMProvider and defaultLLMModel are the CLI flag defaults.
const (
	defaultLLMProvider = providerAnthropic
	defaultLLMModel    = "claude-opus-4-8"
)

// Subdomain enum values used in classify validation and tests.
const (
	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"
)

// Volatility enum values used in classify validation and tests.
const (
	volatilityLow    = "low"
	volatilityMedium = "medium"
	volatilityHigh   = "high"
)
