package main

import (
	"context"

	"github.com/alexei-led/archfit/v3/internal/application/request"
	requestsend "github.com/alexei-led/archfit/v3/internal/history/github"
)

type requestSenderAPI interface {
	Send(context.Context, requestsend.Input) (requestsend.Outcome, error)
}

// requestWiring implements the application port over the concrete GitHub
// adapter, keeping application types out of that adapter.
type requestWiring struct{ sender requestSenderAPI }

func (w requestWiring) Send(ctx context.Context, r request.Request) (request.Delivery, error) {
	out, err := w.sender.Send(ctx, requestsend.Input{Reason: r.Reason, Expires: r.Expires, Can: r.Can})
	if err != nil {
		return request.Delivery{}, err
	}
	return request.Delivery{Number: out.Number, URL: out.URL}, nil
}
