package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/llm"
)

func TestResolveLLMConfig(t *testing.T) {
	configured := config.LLMConfig{Provider: providerDeepSeek, Model: llm.DefaultDeepSeekModel, BaseURL: "https://api.deepseek.com", Effort: llm.EffortHigh, MaxTokens: 65536, Timeout: time.Minute}
	cases := []struct {
		name            string
		cfg             config.LLMConfig
		provider, model string
		want            config.LLMConfig
	}{
		{"default", config.LLMConfig{}, "", "", config.LLMConfig{Provider: providerAnthropic, Model: defaultLLMModel}},
		{"openai default", config.LLMConfig{}, providerOpenAI, "", config.LLMConfig{Provider: providerOpenAI, Model: "gpt-6.1-sol"}},
		{"deepseek default", config.LLMConfig{}, providerDeepSeek, "", config.LLMConfig{Provider: providerDeepSeek, Model: llm.DefaultDeepSeekModel}},
		{"configured", configured, "", "", configured},
		{"same provider preserves settings", configured, providerDeepSeek, "", configured},
		{"provider override discards endpoint and model", configured, providerOpenAI, "", config.LLMConfig{Provider: providerOpenAI, Model: "gpt-6.1-sol"}},
		{"provider and model override", configured, providerOpenAI, "custom", config.LLMConfig{Provider: providerOpenAI, Model: "custom"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveLLMConfig(tc.cfg, tc.provider, tc.model)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestBuildProviderDeepSeekAndCompatible(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test")
	t.Setenv("CUSTOM_TEST_KEY", "custom-test")
	for _, provider := range []string{providerDeepSeek, providerCompatible} {
		t.Run(provider, func(t *testing.T) {
			var path, auth string
			var got map[string]any
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, auth = r.URL.Path, r.Header.Get("Authorization")
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
			t.Cleanup(s.Close)
			cfg := config.LLMConfig{Provider: provider, Model: llm.DefaultDeepSeekModel, BaseURL: s.URL, MaxTokens: 65536, Timeout: time.Minute}
			wantKey := "Bearer deepseek-test"
			if provider == providerCompatible {
				cfg.APIKeyEnv = "CUSTOM_TEST_KEY"
				wantKey = "Bearer custom-test"
			}
			p, err := buildProvider(cfg)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := p.Complete(t.Context(), llm.Request{User: "hi"})
			if err != nil || resp.Text != "ok" {
				t.Fatalf("response=%+v error=%v", resp, err)
			}
			if path != "/chat/completions" || auth != wantKey || got["max_tokens"] != float64(65536) {
				t.Fatalf("path=%q auth=%q body=%v", path, auth, got)
			}
		})
	}
}
