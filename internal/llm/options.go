package llm

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Options configures one provider. Credentials stay in environment variables.
type Options struct {
	Model     string
	BaseURL   string
	APIKeyEnv string
	Effort    string
	MaxTokens int
	Timeout   time.Duration
}

// Default model IDs are explicit release choices, not automatic latest selection.
const (
	DefaultAnthropicModel = "claude-opus-5-5"
	DefaultOpenAIModel    = "gpt-6.1-sol"
	DefaultDeepSeekModel  = "deepseek-flash"
	EffortNone            = "none"
	EffortMinimal         = "minimal"
	EffortLow             = "low"
	EffortMedium          = "medium"
	EffortHigh            = "high"
	EffortXHigh           = "xhigh"
	EffortMax             = "max"
	defaultTimeout        = 5 * time.Minute
)

func (o Options) validate() error {
	if strings.TrimSpace(o.Model) == "" {
		return errors.New("model is required")
	}
	if o.MaxTokens < 0 {
		return errors.New("max_tokens must be positive")
	}
	if o.Timeout < 0 {
		return errors.New("timeout must be positive")
	}
	if o.BaseURL != "" {
		u, err := url.Parse(o.BaseURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return errors.New("base_url must be an http(s) URL without credentials, query, or fragment")
		}
	}
	switch o.Effort {
	case "", EffortNone, EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax:
	default:
		return fmt.Errorf("unsupported effort %q", o.Effort)
	}
	return nil
}

func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return defaultTimeout
}

func (o Options) maxTokens(req Request) int64 {
	if o.MaxTokens > 0 {
		return int64(o.MaxTokens)
	}
	return maxTokensOrDefault(req)
}

// cacheIdentity excludes credentials, but separates transport and generation settings.
func (o Options) cacheIdentity(protocol string) string {
	return strings.Join([]string{protocol, o.Model, strings.TrimRight(o.BaseURL, "/"), o.APIKeyEnv, o.Effort, strconv.Itoa(o.MaxTokens)}, "\x00")
}

func apiKey(provider, env string) (string, error) {
	key := os.Getenv(env)
	if key == "" {
		return "", &NotConfiguredError{Provider: provider, Reason: env + " is not set"}
	}
	return key, nil
}
