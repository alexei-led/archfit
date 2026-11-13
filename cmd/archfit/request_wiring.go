package main

import (
	"context"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/application/request"
	requestsend "github.com/alexei-led/archfit/v3/internal/history/github"
	"github.com/alexei-led/archfit/v3/internal/output/record"
)

type requestSenderAPI interface {
	Send(context.Context, requestsend.Input) (requestsend.Outcome, error)
}

// requestWiring implements the application port over the concrete GitHub
// adapter, keeping application types out of that adapter.
type requestWiring struct{ sender requestSenderAPI }

func (w requestWiring) Send(ctx context.Context, r request.Request) (request.Delivery, error) {
	in := requestsend.Input{Reason: r.Reason, Expires: r.Expires}
	if r.Can != nil {
		var rendered strings.Builder
		_ = record.Answer(&rendered, *r.Can, record.Options{})
		in.Can = &requestsend.CanRecord{From: r.Can.From, To: r.Can.To, Record: rendered.String()}
	}
	out, err := w.sender.Send(ctx, in)
	if err != nil {
		return request.Delivery{}, err
	}
	return request.Delivery{Number: out.Number, URL: out.URL}, nil
}
