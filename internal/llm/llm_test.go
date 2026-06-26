package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaioption "github.com/openai/openai-go/option"
)

const testModel = "test-model"

// anthropicOK is a minimal valid Messages API response.
const anthropicOK = `{
	"id": "msg_1", "type": "message", "role": "assistant", "model": "test-model",
	"content": [{"type": "text", "text": "hello "}, {"type": "text", "text": "world"}],
	"stop_reason": "end_turn",
	"usage": {"input_tokens": 1, "output_tokens": 2}
}`

// openaiOK is a minimal valid Chat Completions response.
const openaiOK = `{
	"id": "c1", "object": "chat.completion", "created": 0, "model": "test-model",
	"choices": [{"index": 0, "message": {"role": "assistant", "content": "hello world"}, "finish_reason": "stop"}]
}`

// serve returns an httptest server replying with status + body and capturing
// the last request body into got.
func serve(t *testing.T, status int, body string, got *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			*got = m
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnthropic_Complete(t *testing.T) {
	var got map[string]any
	srv := serve(t, http.StatusOK, anthropicOK, &got)
	p := newAnthropic(testModel,
		anthropicoption.WithBaseURL(srv.URL),
		anthropicoption.WithAPIKey("test-key"),
		anthropicoption.WithMaxRetries(0),
	)

	resp, err := p.Complete(context.Background(), Request{System: "be terse", User: "hi", MaxTokens: 100})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("text = %q, want concatenated blocks", resp.Text)
	}

	// Request shape: model, max_tokens, system, single user message, NO temperature.
	if got["model"] != testModel {
		t.Errorf("model = %v", got["model"])
	}
	if got["max_tokens"] != float64(100) {
		t.Errorf("max_tokens = %v, want 100", got["max_tokens"])
	}
	if _, present := got["temperature"]; present {
		t.Error("temperature must NOT be sent (Opus-tier rejects sampling params)")
	}
	sys, ok := got["system"].([]any)
	if !ok || len(sys) != 1 {
		t.Fatalf("system = %v, want one block", got["system"])
	}
	if p.Name() != "anthropic/"+testModel {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestAnthropic_ErrorPaths(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, "401"},
		{"rate limited", http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, "429"},
		{"malformed body", http.StatusOK, "not json", ""},
		{"empty content", http.StatusOK, `{"id":"m","type":"message","role":"assistant","model":"m","content":[],"stop_reason":"refusal","usage":{"input_tokens":0,"output_tokens":0}}`, "empty response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := serve(t, tc.status, tc.body, nil)
			p := newAnthropic(testModel,
				anthropicoption.WithBaseURL(srv.URL),
				anthropicoption.WithAPIKey("test-key"),
				anthropicoption.WithMaxRetries(0),
			)
			_, err := p.Complete(context.Background(), Request{User: "hi"})
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want substring %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), p.Name()) {
				t.Errorf("error must carry provider name: %v", err)
			}
		})
	}
}

func TestAnthropic_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Honor request cancellation so srv.Close() during t.Cleanup doesn't
		// block ~200ms waiting for the handler goroutine to finish.
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(anthropicOK))
	}))
	t.Cleanup(srv.Close)
	p := newAnthropic(testModel,
		anthropicoption.WithBaseURL(srv.URL),
		anthropicoption.WithAPIKey("test-key"),
		anthropicoption.WithMaxRetries(0),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Complete(ctx, Request{User: "hi"}); err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestOpenAI_Complete(t *testing.T) {
	var got map[string]any
	srv := serve(t, http.StatusOK, openaiOK, &got)
	p := newOpenAI("openai", testModel,
		openaioption.WithBaseURL(srv.URL),
		openaioption.WithAPIKey("test-key"),
		openaioption.WithMaxRetries(0),
	)

	resp, err := p.Complete(context.Background(), Request{System: "be terse", User: "hi"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("text = %q", resp.Text)
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages = %v, want system + user", got["messages"])
	}
	if got["temperature"] != float64(0) {
		t.Errorf("temperature = %v, want 0", got["temperature"])
	}
	if got["max_completion_tokens"] != float64(4096) {
		t.Errorf("max_completion_tokens = %v, want default 4096", got["max_completion_tokens"])
	}
}

func TestOpenAI_ErrorAndEmpty(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"id":"c1","object":"chat.completion","choices":[]}`, nil)
	p := newOpenAI("openai", testModel,
		openaioption.WithBaseURL(srv.URL),
		openaioption.WithAPIKey("test-key"),
		openaioption.WithMaxRetries(0),
	)
	if _, err := p.Complete(context.Background(), Request{User: "hi"}); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("err = %v, want empty response", err)
	}
}

func TestNotConfigured(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := NewAnthropic(testModel); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("NewAnthropic without key: err = %v", err)
	}
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := NewOpenAI(testModel); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("NewOpenAI without key: err = %v", err)
	}
	// Ollama needs no key.
	if p := NewOllama("", testModel); p.Name() != "ollama/"+testModel {
		t.Errorf("NewOllama Name = %q", p.Name())
	}
}
