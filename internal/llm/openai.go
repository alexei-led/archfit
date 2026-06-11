package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// defaultOllamaBaseURL is the OpenAI-compatible endpoint of a local Ollama.
const defaultOllamaBaseURL = "http://localhost:11434/v1"

// OpenAIProvider completes requests via the official openai-go Chat
// Completions API. It also serves Ollama through its OpenAI-compatible
// endpoint (NewOllama) — base-URL swap, no separate SDK.
type OpenAIProvider struct {
	tag    string // "openai" or "ollama" — part of the cache key
	model  string
	client openai.Client
}

// NewOpenAI returns a provider for the given model id. Returns
// NotConfiguredError when OPENAI_API_KEY is not set.
func NewOpenAI(model string) (*OpenAIProvider, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, &NotConfiguredError{Provider: "openai", Reason: "OPENAI_API_KEY is not set"}
	}
	return newOpenAI("openai", model), nil
}

// NewOllama returns a provider talking to a local (or remote) Ollama via its
// OpenAI-compatible endpoint. No API key required; baseURL defaults to
// http://localhost:11434/v1 when empty.
func NewOllama(baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	return newOpenAI("ollama", model,
		option.WithBaseURL(baseURL),
		option.WithAPIKey("ollama"), // the endpoint ignores it; the SDK requires one
	)
}

// newOpenAI builds the provider with extra SDK options (tests inject a base
// URL and key here).
func newOpenAI(tag, model string, opts ...option.RequestOption) *OpenAIProvider {
	return &OpenAIProvider{
		tag:    tag,
		model:  model,
		client: openai.NewClient(opts...),
	}
}

// Name returns "<tag>/<model>" (e.g. "ollama/qwen2.5-coder").
func (p *OpenAIProvider) Name() string { return p.tag + "/" + p.model }

// Complete executes one non-streaming chat completion.
func (p *OpenAIProvider) Complete(ctx context.Context, req Request) (Response, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, 2)
	if req.System != "" {
		messages = append(messages, openai.SystemMessage(req.System))
	}
	messages = append(messages, openai.UserMessage(req.User))

	resp, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:               p.model,
		Messages:            messages,
		MaxCompletionTokens: param.NewOpt(maxTokensOrDefault(req)),
		// OpenAI-compatible endpoints still accept temperature; pin for the
		// least output variance (true determinism comes from the cache).
		Temperature: param.NewOpt(0.0),
	})
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", p.Name(), err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	return Response{Text: resp.Choices[0].Message.Content}, nil
}
