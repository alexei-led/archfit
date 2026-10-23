package llm

import (
	"net/http"
	"strings"
	"testing"
)

func TestProvidersRejectNullResponses(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("DEEPSEEK_API_KEY", "test")
	cases := []struct {
		name        string
		newProvider func(Options) (Provider, error)
	}{
		{"messages-null", func(o Options) (Provider, error) { return NewAnthropic(o) }},
		{"responses-null", func(o Options) (Provider, error) { return NewOpenAI(o) }},
		{"deepseek-null", func(o Options) (Provider, error) { return NewDeepSeek(o) }},
		{"compatible-null", func(o Options) (Provider, error) { return NewCompatible(o) }},
		{"ollama-null", func(o Options) (Provider, error) { return NewOllama(o) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := serve(t, http.StatusOK, "null", nil)
			inner, err := tc.newProvider(Options{Model: testModel, BaseURL: s.URL})
			if err != nil {
				t.Fatal(err)
			}
			p := NewCache(inner, t.TempDir())
			resp, err := p.Complete(t.Context(), Request{User: "hi"})
			if err == nil || !strings.Contains(err.Error(), "empty response") || resp.Text != "" {
				t.Fatalf("null reply: response=%+v error=%v", resp, err)
			}
		})
	}
}
