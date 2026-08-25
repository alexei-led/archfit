// Package application owns user-facing analysis use-case contracts.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	// FormatJSON selects JSON output.
	FormatJSON = "json"
	// FormatText selects the terminal report.
	FormatText = "text"
	// FormatMarkdown selects Markdown output.
	FormatMarkdown = "markdown"
	// FormatSARIF selects SARIF output.
	FormatSARIF = "sarif"
	// FormatScorecard selects scorecard output.
	FormatScorecard = "scorecard"
)

// Request contains the user intent shared by analyze and check.
type Request struct {
	BaseRef string

	JSON     bool
	Markdown bool
	SARIF    bool
	Formats  []string

	NoAdvisories bool
	RequireTools bool
	ReportOnly   bool
}

// AnalysisRequest is the narrow technical-stage input. The application owns
// validation and sequencing; the stage owns evidence collection and scoring.
type AnalysisRequest struct {
	BaseRef      string
	Formats      []string
	NoAdvisories bool
	RequireTools bool
	ReportOnly   bool
	// Comparison and use-case stages may override the technical context while
	// keeping the analyzer implementation shared.
	ConfigSource         string
	BundleDir            string
	Root                 string
	EvaluatedAt          time.Time
	EmptyBaseline        bool
	SuppressGateReasons  bool
	WarnLabel            string
	CaptureRelationships bool
}

// AnalysisResult is the stage output consumed by the application projection.
type AnalysisResult struct {
	Diagnostic         result.Result
	Score              score.Scorecard
	BaseScore          *score.Scorecard
	HardGate           bool
	EnrichmentEvidence *EnrichmentEvidence
}

// PolicyPreparer is the consumer port for the policy/config preparation stage.
// Application invokes it before technical analysis stages.
type PolicyPreparer interface {
	Prepare(context.Context) error
}

// EvidenceStage acquires scope-bound facts before relationship analysis.
type EvidenceStage interface {
	Acquire(context.Context, AnalysisRequest) (evidence.Snapshot, error)
}

// RelationshipStage classifies acquired evidence into relationship-owned facts.
type RelationshipStage interface {
	Relate(context.Context, evidence.Snapshot) (relationship.AnalysisResult, error)
}

// AssessmentStage evaluates relationship facts and returns the application
// analysis result. Report projection remains in application.
type AssessmentStage interface {
	Assess(context.Context, AnalysisRequest, evidence.AssessmentView, relationship.AnalysisResult) (AnalysisResult, error)
}

// StageExecutor is the shared application sequencing helper. All analysis
// use cases use these explicit ports so stage order cannot diverge between
// analyze, baseline, explain, and compare.
type StageExecutor struct {
	Preparer     PolicyPreparer
	Evidence     EvidenceStage
	Relationship RelationshipStage
	Assessment   AssessmentStage
}

// Execute prepares policy, then acquires evidence, classifies relationships,
// assesses the result, and returns the application-owned outcome.
func (s StageExecutor) Execute(ctx context.Context, req AnalysisRequest) (AnalysisResult, error) {
	if s.Preparer == nil || s.Evidence == nil || s.Relationship == nil || s.Assessment == nil {
		return AnalysisResult{}, errors.New("analysis stages are required")
	}
	if err := s.Preparer.Prepare(ctx); err != nil {
		return AnalysisResult{}, fmt.Errorf("policy preparation: %w", err)
	}
	evidenceResult, err := s.Evidence.Acquire(ctx, req)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("evidence acquisition: %w", err)
	}
	relationshipResult, err := s.Relationship.Relate(ctx, evidenceResult)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("relationship analysis: %w", err)
	}
	out, err := s.Assessment.Assess(ctx, req, evidenceResult.AssessmentView(), relationshipResult)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("assessment: %w", err)
	}
	return out, nil
}

// Response contains the completed use-case state for cmd-owned rendering.
type Response struct {
	Document report.Document
	Formats  []string
	Outcome  Outcome
}

// Outcome is the typed Analyze/Check result. It is not a CLI exit code: cmd
// translates it to a process exit status (and report-only analyze always exits
// zero regardless of the underlying verdict).
type Outcome string

// Outcome values for the Analyze/Check use case.
const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
	OutcomeWarn Outcome = "warn"
)

// InvalidFormatsError reports mutually exclusive output format flags.
type InvalidFormatsError struct{ Message string }

func (e *InvalidFormatsError) Error() string { return e.Message }

// ExecutionError reports a controlled use-case failure. The CLI owns process
// status translation for this error.
type ExecutionError struct{ Message string }

func (e *ExecutionError) Error() string { return e.Message }

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

// Service owns the Analyze/Check use case. Cmd supplies the four explicit
// staged ports and renders the returned document.
type Service struct {
	Preparer     PolicyPreparer
	Evidence     EvidenceStage
	Relationship RelationshipStage
	Assessment   AssessmentStage
}

// Execute validates the request, invokes the technical stage, projects the
// report, and assigns outcome semantics.
func (s Service) Execute(ctx context.Context, req Request) (Response, error) {
	formats, err := ResolveFormats(req.JSON, req.Markdown, req.SARIF, req.Formats)
	if err != nil {
		return Response{}, err
	}
	if s.Preparer == nil || s.Evidence == nil || s.Relationship == nil || s.Assessment == nil {
		return Response{}, errors.New("analysis stages are required")
	}
	out, err := StageExecutor(s).Execute(ctx, AnalysisRequest{
		BaseRef: req.BaseRef, Formats: formats, NoAdvisories: req.NoAdvisories,
		RequireTools: req.RequireTools, ReportOnly: req.ReportOnly,
	})
	if err != nil {
		return Response{}, &ExecutionError{Message: fmt.Sprintf("error: %v", err)}
	}

	outcome := OutcomePass
	switch {
	case out.HardGate:
		outcome = OutcomeFail
	case out.Diagnostic.Verdict == result.VerdictFail:
		outcome = OutcomeFail
	case out.Diagnostic.Verdict == result.VerdictWarn:
		outcome = OutcomeWarn
	}
	return Response{
		Document: ProjectReport(out.Diagnostic, out.Score, out.BaseScore, out.HardGate),
		Formats:  formats,
		Outcome:  outcome,
	}, nil
}
