// Package application owns user-facing analysis use-case contracts.
package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/evidence"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
	"github.com/alexei-led/archfit/v3/internal/scope"
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

// Request contains the report measurement inputs.
type Request struct {
	ConfigSource string
	BundleDir    string
	BaseRef      string

	JSON     bool
	Markdown bool
	SARIF    bool
	Formats  []string

	NoAdvisories bool
	AddedBase    string
	// Identity carries the run facts only the caller can read: the policy
	// files' bytes, the git tree, the engine build.
	Identity IdentityInputs
}

// AnalysisRequest is the narrow technical-stage input. The application owns
// validation and sequencing; the stage owns evidence collection and scoring.
type AnalysisRequest struct {
	BaseRef      string
	NoAdvisories bool
	AddedBase    string
	// DiscloseHealthWarnings prints assessment health hints on the report path.
	DiscloseHealthWarnings bool
	// Use-case stages may override the technical context while
	// keeping the analyzer implementation shared.
	ConfigSource string
	BundleDir    string
	Root         string
	EvaluatedAt  time.Time
}

// AnalysisResult is the stage output consumed by the application projection.
type AnalysisResult struct {
	Diagnostic result.Result
	Score      score.Scorecard
}

// PolicyPreparer discloses report config warnings before evidence acquisition.
// Decoding and tree validation belong to the policy adapters.
type PolicyPreparer interface {
	Prepare(context.Context) error
}

// AnalysisContext is the application-owned run context that accompanies neutral
// evidence through the relationship and assessment stages. Evidence answers
// "what did the tools observe"; this answers "under which boundary, instant,
// identity, and policy". Topology enrichment (owners, deploy units) is resolved
// once during preparation and every later stage reads the same immutable
// snapshot rather than re-deriving it.
type AnalysisContext struct {
	Scope      scope.Scope
	Now        time.Time
	ConfigHash string
	// ModelHash identifies the module map.
	ModelHash string
	// ClassificationHash identifies policy leaves that change extracted facts.
	// ConfigHash separately identifies the raw policy bytes.
	ClassificationHash string
	ConfigSource       string
	BundleDir          string
	// ScanRoot is the analysis boundary AS THE CALLER GAVE IT (empty means "the
	// whole repository"). Scope.Root is its resolved, canonical form. The repair
	// tasks' validation command must echo the caller's form, not the resolved
	// one, or a copy-pasted command would not reproduce the run.
	ScanRoot              string
	PrimaryExtractorTools []string
	Policy                policy.PolicySnapshot
	// OwnerSource records how module ownership was resolved for this run, and
	// OwnerWarnings the degradation disclosed while resolving it. Both are
	// produced by the single resolution pass so assessment never repeats it.
	OwnerSource   string
	OwnerWarnings []string
	// ConfigWarnings is the advisory config-quality block acquisition assembled
	// from config lint plus every degradation it disclosed on stderr, in
	// emission order. Assessment attaches it; it never re-derives it.
	ConfigWarnings []string
	// MarkedCoverage and CoverageGaps are the analyzer-coverage evidence after
	// acquisition rewrote the rows of primaries the config switched off over a
	// language that IS present. They are resolved here because both depend on
	// analyzer applicability probes and the source tree, which only acquisition
	// may touch. Metric evaluation deliberately reads the RAW coverage
	// on Facts, so a config opt-out cannot move a measured metric.
	//
	MarkedCoverage []modevidence.Coverage
	CoverageGaps   []modevidence.CoverageGap
	// DeployUnitDetectedModules counts the modules deploy-unit detection mapped.
	// It is report-only distance context and is NOT the declared-unit count.
	DeployUnitDetectedModules int
}

// Acquired pairs the neutral evidence of one run with the context it was
// acquired under plus the assessment observation projection acquisition built.
// Application passes full facts only to relationship analysis.
type Acquired struct {
	Facts        evidence.Facts
	Observations evaluation.Observations
	Context      AnalysisContext
}

// EvidenceStage acquires scope-bound facts before relationship analysis. It is
// a real boundary: acquisition walks the source tree, runs external tools, and
// resolves repository ownership. Relationship analysis and assessment are pure
// decisions this package calls directly — a port for either would only hide the
// call.
type EvidenceStage interface {
	Acquire(context.Context, AnalysisRequest) (Acquired, error)
}

// StageExecutor sequences evidence acquisition, assessment, and scoring for reports.
type StageExecutor struct {
	Preparer PolicyPreparer
	Evidence EvidenceStage
	Stderr   io.Writer
	Progress func(stage string)
}

// Execute prepares policy, acquires evidence, classifies relationships, and scores the assessment.
func (s StageExecutor) Execute(ctx context.Context, req AnalysisRequest) (AnalysisResult, error) {
	if s.Preparer == nil || s.Evidence == nil {
		return AnalysisResult{}, errors.New("analysis stages are required")
	}
	if err := s.Preparer.Prepare(ctx); err != nil {
		return AnalysisResult{}, fmt.Errorf("policy preparation: %w", err)
	}
	acquired, err := s.Evidence.Acquire(ctx, req)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("evidence acquisition: %w", err)
	}
	discloseRawCoverageReasons(s.stderr(), "", acquired.Context.MarkedCoverage)
	return s.assess(req, acquired), nil
}

// relate runs the relationship stage. It is a pure decision over the acquired
// facts and the run policy resolved during acquisition.
func relate(acquired Acquired) relationship.AnalysisResult {
	facts, runCtx := acquired.Facts, acquired.Context
	return analysis.Analyze(analysis.Input{
		Graph: facts.Graph, Policy: runCtx.Policy.Relationship,
		CloneClusters: facts.Clones, FileClassIndex: facts.FileClassIndex,
		RuntimeSites: facts.RuntimeAsyncSites, RuntimeConfidence: facts.RuntimeConfidence,
		DynamicImportSites: facts.DynamicImports,
	})
}

// assess runs relationship analysis, assessment, and scoring in one shared order.
func (s StageExecutor) assess(req AnalysisRequest, acquired Acquired) AnalysisResult {
	runCtx := acquired.Context
	facts := acquired.Observations
	s.reportPhase("Analyzing dependencies")
	relationships := relate(acquired)
	assessed := evaluation.Assess(evaluation.AssessInput{
		Facts: facts, Relationships: relationships.Relationships, RelationshipSignals: relationships.Assessment, Policy: runCtx.Policy,
		Scope: runCtx.Scope, Now: runCtx.Now, BaseRef: req.BaseRef,
		Advisory:     !req.NoAdvisories,
		ConfigSource: runCtx.ConfigSource, ScanRoot: runCtx.ScanRoot, ConfigHash: runCtx.ConfigHash,
		ModelHash: runCtx.ModelHash, ClassificationHash: runCtx.ClassificationHash,
		PrimaryExtractorTools: runCtx.PrimaryExtractorTools, OwnerSource: runCtx.OwnerSource,
		ConfigWarnings: runCtx.ConfigWarnings, MarkedCoverage: runCtx.MarkedCoverage,
		CoverageGaps:              runCtx.CoverageGaps,
		DeployUnitDetectedModules: runCtx.DeployUnitDetectedModules,
	})
	diag := assessed.Diagnostic
	attachRelationshipEvidence(&diag, relationships.Evidence)
	diag.DistanceContext = buildDistanceContext(diag, runCtx.Policy, runCtx.DeployUnitDetectedModules)
	if req.DiscloseHealthWarnings {
		for _, warning := range assessed.Warnings {
			s.warn(warning)
		}
	}
	s.reportPhase("Scoring architecture")
	scored := evaluation.Score(&diag, evaluation.ScoreInput{
		Policy: runCtx.Policy, Facts: facts,
		ConfigWarnings: runCtx.ConfigWarnings, MarkedCoverage: runCtx.MarkedCoverage,
		CoverageGaps: runCtx.CoverageGaps,
	})
	out := AnalysisResult{Score: scored.Score}
	out.Diagnostic = diag
	return out
}

func (s StageExecutor) stderr() io.Writer {
	if s.Stderr != nil {
		return s.Stderr
	}
	return os.Stderr
}

func (s StageExecutor) warn(msg string) {
	_, _ = fmt.Fprintln(s.stderr(), "warning: "+msg)
}

func (s StageExecutor) reportPhase(stage string) {
	if s.Progress != nil {
		s.Progress(stage)
	}
}

// Response contains the completed use-case state for cmd-owned rendering.
type Response struct {
	Document report.Document
	Formats  []string
}

// InvalidFormatsError reports mutually exclusive output format flags.
type InvalidFormatsError struct{ Message string }

func (e *InvalidFormatsError) Error() string { return e.Message }

// ExecutionError reports a controlled use-case failure. The CLI owns process
// status translation for this error.
type ExecutionError struct{ Message string }

func (e *ExecutionError) Error() string { return e.Message }

// resolveFormats validates shorthand and repeatable output format flags.
func resolveFormats(jsonFlag, markdownFlag, sarifFlag bool, formats []string) ([]string, error) {
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
		return nil, &InvalidFormatsError{Message: "--json, --markdown, and --sarif are mutually exclusive; use at most one"}
	}
	if shorthands > 0 && len(formats) > 0 {
		return nil, &InvalidFormatsError{Message: "--json/--markdown/--sarif and --format are mutually exclusive"}
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

// Service owns the report measurement pipeline. Cmd supplies its stages and
// renders the completed document.
type Service struct {
	Stages StageExecutor
}

// Execute validates the request, invokes the technical stage, projects the
// report, without deciding a check outcome.
func (s Service) Execute(ctx context.Context, req Request) (Response, error) {
	formats, err := resolveFormats(req.JSON, req.Markdown, req.SARIF, req.Formats)
	if err != nil {
		return Response{}, err
	}
	if s.Stages.Preparer == nil || s.Stages.Evidence == nil {
		return Response{}, errors.New("analysis stages are required")
	}
	out, err := s.Stages.Execute(ctx, AnalysisRequest{
		ConfigSource: req.ConfigSource, BundleDir: req.BundleDir,
		BaseRef: req.BaseRef, NoAdvisories: req.NoAdvisories,
		DiscloseHealthWarnings: true,
		AddedBase:              req.AddedBase,
	})
	if err != nil {
		// A controlled stage failure already carries the user-facing wording; the
		// internal stage name the wrapper added is not part of it. Re-wrapping
		// would leak "assessment:" and a second "error:" prefix into stderr.
		var controlled *ExecutionError
		if errors.As(err, &controlled) {
			return Response{}, controlled
		}
		return Response{}, &ExecutionError{Message: err.Error()}
	}

	return Response{
		Document: withRunFacts(projectReport(out.Diagnostic, out.Score), req.Identity),
		Formats:  formats,
	}, nil
}
