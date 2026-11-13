package request_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/application/request"
)

func TestCanSourceReachesSender(t *testing.T) {
	last := answered(t)
	sender := &fakeSender{number: 431}
	s := service(last)
	s.Sender = sender
	if _, err := s.Ask(context.Background(), request.Input{Reason: "why"}); err != nil {
		t.Fatal(err)
	}
	if len(sender.got) != 1 || sender.got[0].Can != last.Can {
		t.Fatal("can source lost")
	}
}

func TestReasonWireLimits(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		valid        bool
	}{
		{"500 Unicode characters", strings.Repeat("界", 500), true},
		{"501 Unicode characters", strings.Repeat("界", 501), false},
		{"line feed", "one\ntwo", false},
		{"carriage return", "one\rtwo", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service(checked(t, "graph")).Ask(context.Background(), request.Input{Reason: tc.reason})
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !errors.Is(err, request.ErrNoReason) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
