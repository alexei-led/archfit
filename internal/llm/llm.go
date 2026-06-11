// Package llm is the thin off-gate LLM provider layer for archfit's enrich
// and explain commands. It is NEVER part of the check gate: the arch ring
// test forbids any internal package from importing it — only cmd may.
//
// The interface is one method deep. Classify/explain prompt construction and
// response parsing are archfit-side concerns (cmd/enrich), not provider
// methods. Determinism of the enrich workflow comes from the content-hash
// cache (cache.go) and the human-reviewed pinned labels — never from sampling
// parameters: current Anthropic Opus-tier models reject temperature outright.
package llm

import (
	"context"
	"fmt"
)

// Request is a single non-streaming completion request.
type Request struct {
	// System is the system prompt; empty means none.
	System string
	// User is the user-turn content.
	User string
	// MaxTokens caps the response; 0 uses the provider default (4096).
	MaxTokens int
}

// Response is the provider's text answer.
type Response struct {
	Text string
}

// Provider executes one completion. Implementations: Anthropic, OpenAI,
// Ollama (OpenAI-compatible), and the content-hash Cache decorator.
type Provider interface {
	// Name identifies provider+model (e.g. "anthropic/claude-opus-4-8");
	// part of the cache key, so it must be stable.
	Name() string

	// Complete executes the request and returns the text response.
	Complete(ctx context.Context, req Request) (Response, error)
}

// defaultMaxTokens is used when Request.MaxTokens is zero.
const defaultMaxTokens = 4096

// NotConfiguredError reports a provider that cannot run in this environment
// (missing API key, unknown provider name). Commands turn it into exit 3
// with a setup hint; it must never be silently swallowed.
type NotConfiguredError struct {
	Provider string
	Reason   string
}

func (e *NotConfiguredError) Error() string {
	return fmt.Sprintf("llm provider %s not configured: %s", e.Provider, e.Reason)
}

// maxTokensOrDefault resolves the request cap.
func maxTokensOrDefault(req Request) int64 {
	if req.MaxTokens > 0 {
		return int64(req.MaxTokens)
	}
	return defaultMaxTokens
}
