package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider completes requests via the official anthropic-sdk-go
// Messages API. Auth comes from the standard ANTHROPIC_API_KEY env var (the
// SDK reads it); archfit never stores keys in config.
//
// No temperature is sent: current Opus-tier models reject sampling parameters
// (400), and enrich determinism comes from the cache + reviewed labels anyway.
type AnthropicProvider struct {
	model  string
	client anthropic.Client
}

// NewAnthropic returns a provider for the given model id (e.g.
// "claude-opus-4-8"). Returns NotConfiguredError when ANTHROPIC_API_KEY is
// not set — fail at construction, not on the first network call.
func NewAnthropic(model string) (*AnthropicProvider, error) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return nil, &NotConfiguredError{Provider: "anthropic", Reason: "ANTHROPIC_API_KEY is not set"}
	}
	return newAnthropic(model), nil
}

// newAnthropic builds the provider with extra SDK options (tests inject a
// base URL and key here; production goes through NewAnthropic).
func newAnthropic(model string, opts ...option.RequestOption) *AnthropicProvider {
	return &AnthropicProvider{
		model:  model,
		client: anthropic.NewClient(opts...),
	}
}

// Name returns "anthropic/<model>".
func (p *AnthropicProvider) Name() string { return "anthropic/" + p.model }

// Complete executes one non-streaming Messages call and concatenates the
// response text blocks.
func (p *AnthropicProvider) Complete(ctx context.Context, req Request) (Response, error) {
	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: maxTokensOrDefault(req),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.User)),
		},
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("%s: %w", p.Name(), err)
	}

	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	if b.Len() == 0 {
		return Response{}, fmt.Errorf("%s: empty response (stop_reason %q)", p.Name(), msg.StopReason)
	}
	return Response{Text: b.String()}, nil
}
