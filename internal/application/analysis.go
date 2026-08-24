// Package application owns user-facing analysis use-case contracts.
package application

import (
	"context"
	"errors"
)

const (
	// FormatJSON selects JSON output.
	FormatJSON = "json"
	// FormatText selects the terminal report.
	FormatText = "text"
	// FormatMarkdown selects Markdown output.
	FormatMarkdown = "markdown"
	// FormatMD is the Markdown shorthand.
	FormatMD = "md"
	// FormatSARIF selects SARIF output.
	FormatSARIF = "sarif"
	// FormatScorecard selects scorecard output.
	FormatScorecard = "scorecard"
)

// Request contains the user intent shared by analyze and check.
type Request struct {
	ConfigPath string
	Root       string
	BaseRef    string

	JSON     bool
	Markdown bool
	SARIF    bool
	Formats  []string

	NoAdvisories bool
	MinSeverity  string
	Lang         []string
	RequireTools bool
	Progress     string
	Quiet        bool
	Refresh      bool
	AISummary    bool
	ReportOnly   bool
}

// InvalidFormatsError reports mutually exclusive output format flags.
type InvalidFormatsError struct{ Message string }

func (e *InvalidFormatsError) Error() string { return e.Message }

// ResolveFormats validates shorthand and repeatable output format flags.
func ResolveFormats(jsonFlag, markdownFlag, sarifFlag bool, formats []string) ([]string, error) {
	shorthands := 0
	if jsonFlag {
		shorthands++
	}
	if markdownFlag {
		shorthands++
	}
	if sarifFlag {
		shorthands++
	}
	if shorthands > 1 {
		return nil, &InvalidFormatsError{Message: "error: --json, --markdown, and --sarif are mutually exclusive; use at most one"}
	}
	if shorthands > 0 && len(formats) > 0 {
		return nil, &InvalidFormatsError{Message: "error: --json/--markdown/--sarif and --format are mutually exclusive"}
	}
	switch {
	case jsonFlag:
		return []string{FormatJSON}, nil
	case markdownFlag:
		return []string{FormatMarkdown}, nil
	case sarifFlag:
		return []string{FormatSARIF}, nil
	case len(formats) > 0:
		return append([]string(nil), formats...), nil
	default:
		return []string{FormatText}, nil
	}
}

// Runner executes one validated analysis request.
type Runner interface {
	Run(context.Context, Request) error
}

// Service validates and dispatches analysis use cases.
type Service struct{ Runner Runner }

// Execute validates output selection before dispatching the request.
func (s Service) Execute(ctx context.Context, req Request) error {
	if s.Runner == nil {
		return errors.New("analysis runner is required")
	}
	formats, err := ResolveFormats(req.JSON, req.Markdown, req.SARIF, req.Formats)
	if err != nil {
		return err
	}
	req.Formats = formats
	return s.Runner.Run(ctx, req)
}
