package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/v2/option"
	openaioption "github.com/openai/openai-go/v3/option"
)

const testSystem = "be terse"
const testAnswer = "hello world"

const responsesOK = `{"id":"r1","object":"response","status":"completed","output":[{"id":"m1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello world","annotations":[]}]}]}`

func TestOpenAIResponsesTransport(t *testing.T) {
	var body map[string]any
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responsesOK))
	}))
	t.Cleanup(server.Close)
	p := newOpenAI("openai", Options{Model: DefaultOpenAIModel, Effort: "max"}, openaioption.WithBaseURL(server.URL), openaioption.WithAPIKey("test"), openaioption.WithMaxRetries(0))
	resp, err := p.Complete(t.Context(), Request{System: testSystem, User: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/responses" || resp.Text != testAnswer {
		t.Fatalf("path=%s response=%+v", path, resp)
	}
	if body["model"] != DefaultOpenAIModel || body["input"] != "hi" || body["instructions"] != testSystem || body["store"] != false {
		t.Fatalf("request=%v", body)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "max" {
		t.Fatalf("reasoning=%v; max must not become xhigh", body["reasoning"])
	}
	assertOmitted(t, body, "temperature", "top_p", "top_logprobs", "tools", "thinking")
}

func TestAnthropicOpus55Request(t *testing.T) {
	var got map[string]any
	body := strings.Replace(anthropicOK, `"content": [`, `"content": [{"type":"thinking","thinking":"","signature":"sig"},`, 1)
	s := serve(t, http.StatusOK, body, &got)
	p := newAnthropic(Options{Model: DefaultAnthropicModel, Effort: "medium", MaxTokens: 65536}, anthropicoption.WithBaseURL(s.URL), anthropicoption.WithAPIKey("test"), anthropicoption.WithMaxRetries(0))
	resp, err := p.Complete(t.Context(), Request{User: "hi", MaxTokens: 10})
	if err != nil || resp.Text != testAnswer {
		t.Fatalf("response=%+v error=%v", resp, err)
	}
	if got["model"] != DefaultAnthropicModel || got["max_tokens"] != float64(65536) {
		t.Fatalf("request=%v", got)
	}
	output, ok := got["output_config"].(map[string]any)
	if !ok || output["effort"] != "medium" {
		t.Fatalf("output_config=%v", got["output_config"])
	}
	assertOmitted(t, got, "temperature", "top_p", "top_k", "thinking", "tools", "tool_choice")
}

func TestAnthropicRejectsUnfinishedResponses(t *testing.T) {
	for _, reason := range []string{"max_tokens", "model_context_window_exceeded", "refusal", "pause_turn", "tool_use", ""} {
		t.Run(reason, func(t *testing.T) {
			body := strings.Replace(anthropicOK, "end_turn", reason, 1)
			s := serve(t, http.StatusOK, body, nil)
			p := newAnthropic(Options{Model: testModel}, anthropicoption.WithBaseURL(s.URL), anthropicoption.WithAPIKey("test"), anthropicoption.WithMaxRetries(0))
			resp, err := p.Complete(t.Context(), Request{User: "hi"})
			if err == nil || resp.Text != "" {
				t.Fatalf("unfinished response accepted: %+v %v", resp, err)
			}
		})
	}
}

func TestOpenAIRejectsUnfinishedResponses(t *testing.T) {
	cases := []struct{ name, body string }{
		{"truncated", strings.Replace(responsesOK, "completed", "incomplete", 1)},
		{"failed", strings.Replace(responsesOK, "completed", "failed", 1)},
		{"queued", strings.Replace(responsesOK, "completed", "queued", 1)},
		{"refusal", strings.Replace(responsesOK, `"type":"output_text","text":"hello world"`, `"type":"refusal","refusal":"no"`, 1)},
		{"empty", `{"status":"completed","output":[]}`},
		{"malformed", "not json"},
		{"missing status", `{"output":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := serve(t, http.StatusOK, tc.body, nil)
			p := newOpenAI("openai", Options{Model: DefaultOpenAIModel}, openaioption.WithBaseURL(s.URL), openaioption.WithAPIKey("test"), openaioption.WithMaxRetries(0))
			resp, err := p.Complete(t.Context(), Request{User: "hi"})
			if err == nil || resp.Text != "" {
				t.Fatalf("bad response accepted: %+v %v", resp, err)
			}
		})
	}
}

func TestCompatibleProviders(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")
	t.Setenv("EXAMPLE_LLM_KEY", "compatible-test-key")
	t.Setenv("OPENAI_API_KEY", "must-not-leak")
	cases := []struct {
		name         string
		makeProvider func(Options) (*OpenAIProvider, error)
		key, effort  string
	}{
		{"deepseek", NewDeepSeek, "deepseek-test-key", "high"},
		{"compatible", NewCompatible, "compatible-test-key", ""},
		{"keyless", NewCompatible, "", ""},
		{"ollama", NewOllama, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			var path, auth string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, auth = r.URL.Path, r.Header.Get("Authorization")
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(strings.Replace(openaiOK, `"content": "hello world"`, `"content":"hello world","reasoning_content":"not the answer"`, 1)))
			}))
			t.Cleanup(s.Close)
			opts := Options{Model: DefaultDeepSeekModel, BaseURL: s.URL + "/v1", MaxTokens: 64000}
			if tc.name == "compatible" {
				opts.APIKeyEnv = "EXAMPLE_LLM_KEY"
			}
			p, err := tc.makeProvider(opts)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := p.Complete(t.Context(), Request{System: "system", User: "hi"})
			if err != nil || resp.Text != testAnswer {
				t.Fatalf("response=%+v error=%v", resp, err)
			}
			wantAuth := ""
			if tc.key != "" {
				wantAuth = "Bearer " + tc.key
			}
			if path != "/v1/chat/completions" || auth != wantAuth {
				t.Fatalf("path=%q auth=%q", path, auth)
			}
			if got["model"] != DefaultDeepSeekModel || got["max_tokens"] != float64(64000) {
				t.Fatalf("request=%v", got)
			}
			assertOmitted(t, got, "temperature", "top_p", "max_completion_tokens")
			if tc.effort != "" {
				if got["reasoning_effort"] != tc.effort {
					t.Fatalf("effort=%v", got["reasoning_effort"])
				}
				thinking, ok := got["thinking"].(map[string]any)
				if !ok || thinking["type"] != "enabled" {
					t.Fatalf("thinking=%v", got["thinking"])
				}
			} else {
				assertOmitted(t, got, "thinking", "reasoning_effort")
			}
		})
	}
}

func TestChatRejectsUnfinishedResponses(t *testing.T) {
	for _, reason := range []string{"length", "content_filter", "tool_calls", "insufficient_system_resource", ""} {
		t.Run(reason, func(t *testing.T) {
			body := strings.Replace(openaiOK, `"finish_reason": "stop"`, `"finish_reason": "`+reason+`"`, 1)
			s := serve(t, http.StatusOK, body, nil)
			p, err := NewOllama(Options{Model: testModel, BaseURL: s.URL})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := p.Complete(t.Context(), Request{User: "hi"})
			if err == nil || resp.Text != "" {
				t.Fatalf("unfinished response accepted: %+v %v", resp, err)
			}
		})
	}
}

func TestOpenAIHTTPFailures(t *testing.T) {
	for _, tag := range []string{"openai", "openai-compatible"} {
		for _, status := range []int{400, 401, 429, 500} {
			t.Run(tag+"/"+http.StatusText(status), func(t *testing.T) {
				s := serve(t, status, `{"error":{"message":"request failed","type":"api_error"}}`, nil)
				p := newOpenAI(tag, Options{Model: testModel}, openaioption.WithBaseURL(s.URL), openaioption.WithAPIKey("test"), openaioption.WithMaxRetries(0))
				resp, err := p.Complete(t.Context(), Request{User: "hi"})
				if err == nil || resp.Text != "" || !strings.Contains(err.Error(), p.Name()) {
					t.Fatalf("response=%+v error=%v", resp, err)
				}
			})
		}
	}
}

func TestProviderConfigurationErrors(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("MISSING_TEST_KEY", "")
	cases := []struct {
		name string
		call func() error
	}{
		{"missing model", func() error { _, err := NewOpenAI(Options{}); return err }},
		{"sol none", func() error { _, err := NewOpenAI(Options{Model: DefaultOpenAIModel, Effort: "none"}); return err }},
		{"sol minimal", func() error { _, err := NewOpenAI(Options{Model: DefaultOpenAIModel, Effort: "minimal"}); return err }},
		{"opus disabled", func() error {
			_, err := NewAnthropic(Options{Model: DefaultAnthropicModel, Effort: "none"})
			return err
		}},
		{"missing deepseek key", func() error { _, err := NewDeepSeek(Options{Model: DefaultDeepSeekModel}); return err }},
		{"missing endpoint", func() error { _, err := NewCompatible(Options{Model: testModel}); return err }},
		{"missing compatible key", func() error {
			_, err := NewCompatible(Options{Model: testModel, BaseURL: "https://example.test/v1", APIKeyEnv: "MISSING_TEST_KEY"})
			return err
		}},
		{"negative cap", func() error { _, err := NewOllama(Options{Model: testModel, MaxTokens: -1}); return err }},
		{"negative timeout", func() error { _, err := NewOllama(Options{Model: testModel, Timeout: -time.Second}); return err }},
		{"invalid effort", func() error { _, err := NewOpenAI(Options{Model: testModel, Effort: "typo"}); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, url := range []string{"file:///tmp/llm", "https://user:secret@example.test", "https://example.test?key=secret", "https://example.test/#fragment", "/v1"} {
		t.Run(url, func(t *testing.T) {
			_, err := NewCompatible(Options{Model: testModel, BaseURL: url})
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("invalid endpoint accepted or credential leaked: %v", err)
			}
		})
	}
}

func TestConfiguredTimeoutBoundsRequest(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	t.Cleanup(s.Close)
	p := newOpenAI("openai", Options{Model: DefaultOpenAIModel, Timeout: 20 * time.Millisecond}, openaioption.WithBaseURL(s.URL), openaioption.WithAPIKey("test"), openaioption.WithMaxRetries(0))
	_, err := p.Complete(t.Context(), Request{User: "hi"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want deadline exceeded", err)
	}
}

func TestCacheSeparatesGenerationSettings(t *testing.T) {
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiOK))
	})
	a, b := httptest.NewServer(handler), httptest.NewServer(handler)
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	dir := filepath.Join(t.TempDir(), "llm")
	cases := []Options{
		{Model: testModel, BaseURL: a.URL},
		{Model: testModel, BaseURL: b.URL},
		{Model: testModel, BaseURL: a.URL, Effort: "high"},
		{Model: testModel, BaseURL: a.URL, MaxTokens: 65536},
	}
	for _, opts := range cases {
		inner, err := NewCompatible(opts)
		if err != nil {
			t.Fatal(err)
		}
		p := NewCache(inner, dir)
		for range 2 {
			if _, err := p.Complete(context.Background(), Request{User: "hi"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if calls != len(cases) {
		t.Fatalf("transport calls=%d; endpoint, effort and cap must separate cached responses", calls)
	}
}

func assertOmitted(t *testing.T, body map[string]any, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := body[field]; ok {
			t.Errorf("%s must be omitted; body=%v", field, body)
		}
	}
}
