package main

// LLM provider name constants shared across init, enrich, doctor, and explain.
const (
	providerAnthropic  = "anthropic"
	providerOpenAI     = "openai"
	providerOllama     = "ollama"
	providerDeepSeek   = "deepseek"
	providerCompatible = "openai-compatible"
)

// Project-marker filenames used by the language registry and test fixtures.
const (
	markerGoMod     = "go.mod"
	markerCargoToml = "Cargo.toml"
)
