package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const providerCompatible = "openai-compatible"

func validateAI(c AIConfig) error {
	if c.Provider == "" {
		if c != (AIConfig{}) {
			return errors.New("ai.provider is required when ai settings are set")
		}
		return nil
	}
	if _, ok := LLMProviders[c.Provider]; !ok {
		return fmt.Errorf("ai.provider %q is not one of: anthropic, openai, ollama, deepseek, openai-compatible", c.Provider)
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.New("ai.model is required when ai.provider is set")
	}
	if err := validateAIEndpoint(c); err != nil {
		return err
	}
	if c.MaxTokens < 0 {
		return errors.New("ai.max_tokens must be positive")
	}
	if c.Timeout != "" {
		d, err := time.ParseDuration(c.Timeout)
		if err != nil || d <= 0 {
			return errors.New("ai.timeout must be a positive Go duration")
		}
	}
	return validateAIEffort(c)
}

func validateAIEndpoint(c AIConfig) error {
	if c.Provider == providerCompatible && c.BaseURL == "" {
		return errors.New("ai.base_url is required for openai-compatible")
	}
	if c.APIKeyEnv != "" && c.Provider != providerCompatible {
		return errors.New("ai.api_key_env is only supported for openai-compatible")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return errors.New("ai.base_url must be an http(s) URL without credentials, query, or fragment")
		}
	}
	return nil
}

func validateAIEffort(c AIConfig) error {
	switch c.Effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("ai.effort %q is not supported", c.Effort)
	}
	if c.Provider == "deepseek" && c.Effort != "" && c.Effort != "low" && c.Effort != "high" && c.Effort != "max" {
		return errors.New("ai.effort for deepseek must be low, high, or max")
	}
	if (c.Provider == "anthropic" || c.Provider == "openai" && c.Model == "gpt-6.1-sol") && (c.Effort == "none" || c.Effort == "minimal") {
		return errors.New("ai.effort for this model must be low, medium, high, xhigh, or max")
	}
	return nil
}
