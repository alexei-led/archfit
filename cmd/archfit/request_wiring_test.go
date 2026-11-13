package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/application/request"
	requestsend "github.com/alexei-led/archfit/v3/internal/history/github"
	"github.com/alexei-led/archfit/v3/internal/model/result"
)

type fakeRequestAPI struct {
	got     requestsend.Input
	outcome requestsend.Outcome
	err     error
}

func (f *fakeRequestAPI) Send(_ context.Context, in requestsend.Input) (requestsend.Outcome, error) {
	f.got = in
	return f.outcome, f.err
}

func TestRequestWiringMapsInputAndOutcome(t *testing.T) {
	answer := &result.CanAnswer{From: "source", To: "target", Rule: "graph", What: "not allowed", Date: "2026-11-10"}
	api := &fakeRequestAPI{outcome: requestsend.Outcome{Number: 431, URL: "https://github.com/acme/shop/issues/431"}}
	input := request.Request{Reason: "why", Expires: "2026-12-10", Can: answer}
	got, err := (requestWiring{sender: api}).Send(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantInput := requestsend.Input{Reason: input.Reason, Expires: input.Expires, Can: &requestsend.CanRecord{
		From: answer.From, To: answer.To, Record: "source -> target: graph: not allowed\npolicy: not protected\n",
	}}
	if !reflect.DeepEqual(api.got, wantInput) {
		t.Errorf("input = %+v, want %+v", api.got, wantInput)
	}
	wantOutput := request.Delivery{Number: api.outcome.Number, URL: api.outcome.URL}
	if got != wantOutput {
		t.Errorf("delivery = %+v, want %+v", got, wantOutput)
	}
}

func TestRequestWiringErrorDoesNotReturnPartialDelivery(t *testing.T) {
	boom := errors.New("failed")
	api := &fakeRequestAPI{outcome: requestsend.Outcome{Number: 431, URL: "https://github.com/acme/shop/issues/431"}, err: boom}
	got, err := (requestWiring{sender: api}).Send(t.Context(), request.Request{Reason: "why"})
	if !errors.Is(err, boom) || got != (request.Delivery{}) {
		t.Fatalf("delivery=%+v err=%v", got, err)
	}
	if api.got.Can != nil {
		t.Fatalf("check source became a can answer: %+v", api.got)
	}
}
