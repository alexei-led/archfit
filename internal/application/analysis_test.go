package application

import (
	"context"
	"testing"
)

type runnerFunc func(context.Context, Request) error

func (f runnerFunc) Run(ctx context.Context, req Request) error { return f(ctx, req) }

func TestResolveFormats(t *testing.T) {
	tests := []struct {
		name     string
		json     bool
		markdown bool
		sarif    bool
		formats  []string
		want     []string
		wantErr  bool
	}{
		{name: "default", want: []string{FormatText}},
		{name: "json shorthand", json: true, want: []string{FormatJSON}},
		{name: "markdown shorthand", markdown: true, want: []string{FormatMarkdown}},
		{name: "sarif shorthand", sarif: true, want: []string{FormatSARIF}},
		{name: "repeatable", formats: []string{FormatJSON, FormatScorecard}, want: []string{FormatJSON, FormatScorecard}},
		{name: "multiple shorthands", json: true, sarif: true, wantErr: true},
		{name: "mixed shorthand and repeatable", markdown: true, formats: []string{FormatJSON}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveFormats(test.json, test.markdown, test.sarif, test.formats)
			if test.wantErr {
				if err == nil {
					t.Fatal("ResolveFormats error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveFormats: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("formats = %v, want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("formats = %v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestServiceExecuteValidatesBeforeRunner(t *testing.T) {
	called := false
	service := Service{Runner: runnerFunc(func(_ context.Context, _ Request) error {
		called = true
		return nil
	})}
	if err := service.Execute(context.Background(), Request{JSON: true, SARIF: true}); err == nil {
		t.Fatal("Execute error = nil")
	}
	if called {
		t.Fatal("runner called for invalid request")
	}
}

func TestServiceExecutePassesResolvedFormats(t *testing.T) {
	var got Request
	service := Service{Runner: runnerFunc(func(_ context.Context, req Request) error {
		got = req
		return nil
	})}
	if err := service.Execute(context.Background(), Request{Formats: []string{FormatJSON}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Formats) != 1 || got.Formats[0] != FormatJSON {
		t.Fatalf("formats = %v, want [%s]", got.Formats, FormatJSON)
	}
}
