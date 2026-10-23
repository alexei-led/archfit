package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// AnthropicProvider uses the official Messages API. It sends no sampling or
// thinking-disable parameters; current Opus models always think adaptively.
type AnthropicProvider struct {
	options Options
	client  anthropic.Client
}

// NewAnthropic constructs a Messages provider using ANTHROPIC_API_KEY.
func NewAnthropic(o Options) (*AnthropicProvider, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if o.Effort == EffortNone || o.Effort == EffortMinimal {
		return nil, errors.New("anthropic: effort must be low, medium, high, xhigh, or max")
	}
	key, err := apiKey("anthropic", "ANTHROPIC_API_KEY")
	if err != nil {
		return nil, err
	}
	return newAnthropic(o, option.WithAPIKey(key)), nil
}

func newAnthropic(o Options, opts ...option.RequestOption) *AnthropicProvider {
	if o.BaseURL == "" {
		o.BaseURL = "https://api.anthropic.com"
	}
	if o.Effort == "" && o.Model == DefaultAnthropicModel {
		o.Effort = EffortMedium
	}
	base := make([]option.RequestOption, 0, 3+len(opts))
	base = append(base, option.WithBaseURL(o.BaseURL), option.WithRequestTimeout(o.timeout()), option.WithoutOpenTelemetry())
	return &AnthropicProvider{options: o, client: anthropic.NewClient(append(base, opts...)...)}
}

// Name returns the display identity, without credentials or endpoint details.
func (p *AnthropicProvider) Name() string { return "anthropic/" + p.options.Model }

func (p *AnthropicProvider) cacheIdentity() string {
	return p.options.cacheIdentity("anthropic/messages")
}

// Complete returns only a finished text reply, never a refusal or partial output.
func (p *AnthropicProvider) Complete(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.timeout())
	defer cancel()
	params := anthropic.MessageNewParams{
		Model: p.options.Model, MaxTokens: p.options.maxTokens(req),
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(req.User))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(p.options.Effort)},
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", p.Name(), err)
	}
	if msg == nil {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	if msg.StopReason != anthropic.StopReasonEndTurn && msg.StopReason != anthropic.StopReasonStopSequence {
		return Response{}, fmt.Errorf("%s: response not complete (stop_reason %q)", p.Name(), msg.StopReason)
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	if strings.TrimSpace(b.String()) == "" {
		return Response{}, fmt.Errorf("%s: empty response", p.Name())
	}
	return Response{Text: b.String()}, nil
}
