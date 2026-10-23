package llm

import (
	"os"
	"strings"
	"testing"
)

// Live tests are opt-in because they send prompts to providers and spend tokens.
func TestLiveProviders(t *testing.T) {
	if os.Getenv("ARCHFIT_LLM_LIVE") != "1" {
		t.Skip("set ARCHFIT_LLM_LIVE=1 to spend tokens on provider smoke tests")
	}
	cases := []struct {
		name, key, model string
		newProvider      func(Options) (Provider, error)
	}{
		{"anthropic", "ANTHROPIC_API_KEY", DefaultAnthropicModel, func(o Options) (Provider, error) { return NewAnthropic(o) }},
		{"openai", "OPENAI_API_KEY", DefaultOpenAIModel, func(o Options) (Provider, error) { return NewOpenAI(o) }},
		{"deepseek", "DEEPSEEK_API_KEY", DefaultDeepSeekModel, func(o Options) (Provider, error) { return NewDeepSeek(o) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if os.Getenv(tc.key) == "" {
				t.Skip(tc.key + " is not set")
			}
			p, err := tc.newProvider(Options{Model: tc.model})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := p.Complete(t.Context(), Request{System: "Reply with exactly one word: OK", User: "Confirm."})
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(resp.Text) != "OK" {
				t.Fatalf("unexpected smoke-test reply: %q", resp.Text)
			}
		})
	}
}
