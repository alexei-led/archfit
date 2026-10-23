package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const defaultOllamaBaseURL = "http://localhost:11434/v1"

// OpenAIProvider uses Responses for OpenAI and Chat Completions for compatible
// endpoints. No sampling parameters are sent on either API.
type OpenAIProvider struct {
	tag     string
	options Options
	client  openai.Client
}

// NewOpenAI constructs a Responses provider using OPENAI_API_KEY.
func NewOpenAI(o Options) (*OpenAIProvider, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if o.Model == DefaultOpenAIModel && (o.Effort == EffortNone || o.Effort == EffortMinimal) {
		return nil, errors.New("gpt-6.1-sol: none and minimal effort are unsupported")
	}
	key, err := apiKey("openai", "OPENAI_API_KEY")
	if err != nil {
		return nil, err
	}
	return newOpenAI("openai", o, option.WithAPIKey(key)), nil
}

// NewOllama constructs a keyless Chat Completions provider.
func NewOllama(o Options) (*OpenAIProvider, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if o.BaseURL == "" {
		o.BaseURL = defaultOllamaBaseURL
	}
	return newOpenAI("ollama", o, option.WithAPIKey("")), nil
}

// NewDeepSeek constructs a Chat Completions provider using DEEPSEEK_API_KEY.
func NewDeepSeek(o Options) (*OpenAIProvider, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	switch o.Effort {
	case "", EffortLow, EffortHigh, EffortMax:
	default:
		return nil, errors.New("deepseek: effort must be low, high, or max")
	}
	if o.BaseURL == "" {
		o.BaseURL = "https://api.deepseek.com"
	}
	if o.Effort == "" {
		o.Effort = EffortHigh
	}
	key, err := apiKey("deepseek", "DEEPSEEK_API_KEY")
	if err != nil {
		return nil, err
	}
	return newOpenAI("deepseek", o, option.WithAPIKey(key)), nil
}

// NewCompatible constructs a Chat Completions provider. An empty APIKeyEnv means
// keyless access; it never inherits OPENAI_API_KEY for another endpoint.
func NewCompatible(o Options) (*OpenAIProvider, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if o.BaseURL == "" {
		return nil, errors.New("openai-compatible: base_url is required")
	}
	key := ""
	if o.APIKeyEnv != "" {
		var err error
		key, err = apiKey("openai-compatible", o.APIKeyEnv)
		if err != nil {
			return nil, err
		}
	}
	return newOpenAI("openai-compatible", o, option.WithAPIKey(key)), nil
}

func newOpenAI(tag string, o Options, opts ...option.RequestOption) *OpenAIProvider {
	if tag == "openai" {
		if o.BaseURL == "" {
			o.BaseURL = "https://api.openai.com/v1"
		}
		if o.Effort == "" && o.Model == DefaultOpenAIModel {
			o.Effort = EffortMedium
		}
	}
	base := make([]option.RequestOption, 0, 1+len(opts))
	base = append(base, option.WithBaseURL(o.BaseURL))
	return &OpenAIProvider{tag: tag, options: o, client: openai.NewClient(append(base, opts...)...)}
}

// Name returns the display provider/model identity.
func (p *OpenAIProvider) Name() string { return p.tag + "/" + p.options.Model }

func (p *OpenAIProvider) cacheIdentity() string { return p.options.cacheIdentity(p.tag) }

// Complete executes one bounded call and rejects unfinished output.
func (p *OpenAIProvider) Complete(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.timeout())
	defer cancel()
	if p.tag == "openai" {
		return p.completeResponses(ctx, req)
	}
	return p.completeChat(ctx, req)
}

func (p *OpenAIProvider) completeResponses(ctx context.Context, req Request) (Response, error) {
	params := responses.ResponseNewParams{
		Model:           p.options.Model,
		Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(req.User)},
		MaxOutputTokens: openai.Int(p.options.maxTokens(req)),
		Reasoning:       shared.ReasoningParam{Effort: shared.ReasoningEffort(p.options.Effort)},
		Store:           openai.Bool(false),
	}
	if req.System != "" {
		params.Instructions = openai.String(req.System)
	}
	resp, err := p.client.Responses.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", p.Name(), err)
	}
	if resp == nil {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	if resp.Status != responses.ResponseStatusCompleted {
		return Response{}, fmt.Errorf("%s: response not complete (status %q, reason %q, code %q)", p.Name(), resp.Status, resp.IncompleteDetails.Reason, resp.Error.Code)
	}
	for _, item := range resp.Output {
		if item.Type == "message" {
			for _, part := range item.AsMessage().Content {
				if part.Type == "refusal" {
					return Response{}, fmt.Errorf("%s: response refused", p.Name())
				}
			}
		}
	}
	text := resp.OutputText()
	if strings.TrimSpace(text) == "" {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	return Response{Text: text}, nil
}

func (p *OpenAIProvider) completeChat(ctx context.Context, req Request) (Response, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, 2)
	if req.System != "" {
		messages = append(messages, openai.SystemMessage(req.System))
	}
	messages = append(messages, openai.UserMessage(req.User))
	params := openai.ChatCompletionNewParams{Model: p.options.Model, Messages: messages}
	// DeepSeek and local Chat Completions servers use max_tokens. OpenAI's
	// max_completion_tokens belongs to a different compatibility contract.
	params.MaxTokens = openai.Int(p.options.maxTokens(req))
	if p.options.Effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(p.options.Effort)
	}
	var opts []option.RequestOption
	if p.tag == "deepseek" {
		opts = append(opts, option.WithJSONSet("thinking", map[string]string{"type": "enabled"}))
	}
	resp, err := p.client.Chat.Completions.New(ctx, params, opts...)
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", p.Name(), err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	choice := resp.Choices[0]
	if choice.FinishReason != "stop" {
		return Response{}, fmt.Errorf("%s: response not complete (finish_reason %q)", p.Name(), choice.FinishReason)
	}
	if choice.Message.Refusal != "" {
		return Response{}, fmt.Errorf("%s: response refused", p.Name())
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	return Response{Text: choice.Message.Content}, nil
}
