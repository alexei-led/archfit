package main

import (
	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/llm"
)

// Switching providers discards the old endpoint/auth/effort as well as its
// model. Otherwise an override could send the wrong key to another endpoint.
func resolveLLMConfig(c config.LLMConfig, provider, model string) config.LLMConfig {
	if provider != "" && provider != c.Provider {
		c = config.LLMConfig{Provider: provider}
	}
	if c.Provider == "" {
		c.Provider = providerAnthropic
	}
	if model != "" {
		c.Model = model
	}
	if c.Model == "" {
		switch c.Provider {
		case providerAnthropic:
			c.Model = defaultLLMModel
		case providerOpenAI:
			c.Model = llm.DefaultOpenAIModel
		case providerDeepSeek:
			c.Model = llm.DefaultDeepSeekModel
		}
	}
	return c
}
