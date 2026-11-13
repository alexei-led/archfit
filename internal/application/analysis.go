// Package application owns user-facing analysis use-case contracts.
package application

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/evidence"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
	"github.com/alexei-led/archfit/v3/internal/relationship/labels"
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

// Request contains the user intent shared by analyze and check.
type Request struct {
	ConfigSource string
	BundleDir    string
	BaseRef      string

	JSON     bool
	Markdown bool
	SARIF    bool
	Formats  []string

	NoAdvisories   bool
	RequireTools   bool
	ValidationArgs []string
	// ValidationConfig and ValidationRoot replace the config and root a repair's
	// validation command names; empty keeps the run's own. The staged-index git
	// hook analyses a temporary checkout that is gone when the reader runs the
	// command, so it names the repository instead.
	ValidationConfig string
	ValidationRoot   string
	// AddedBase is the ref the added-file set is measured against when BaseRef
	// is empty (scope.Config.AddedBase).
	AddedBase string
	// GateReasonsInState keeps the coupling gate's reasons off stderr: the
	// state carries them as gate_reasons and check reports them as attention.
	GateReasonsInState bool
	// Identity carries the run facts only the caller can read: the policy
	// files' bytes, the git tree, the engine build.
	Identity IdentityInputs
}

// AnalysisRequest is the narrow technical-stage input. The application owns
// validation and sequencing; the stage owns evidence collection and scoring.
type AnalysisRequest struct {
	BaseRef        string
	NoAdvisories   bool
	RequireTools   bool
	ValidationArgs []string
	// ValidationConfig and ValidationRoot: see Request.
	ValidationConfig string
	ValidationRoot   string
	// GateReasonsInState: see Request.
	GateReasonsInState bool
	// AddedBase: see Request.
	AddedBase string
	// ApplyToolGate lets a missing required analyzer stamp the report verdict fail.
	// Enrichment leaves it off so a coverage gap cannot rewrite its result.
	ApplyToolGate bool
	// DiscloseHealthWarnings prints assessment health hints on the report path.
	// Enrichment consumes the assessment without disclosing those hints.
	DiscloseHealthWarnings bool
	// Use-case stages may override the technical context while
	// keeping the analyzer implementation shared.
	ConfigSource         string
	BundleDir            string
	Root                 string
	EvaluatedAt          time.Time
	SuppressGateReasons  bool
	CaptureRelationships bool
}

// AnalysisResult is the stage output consumed by the application projection.
type AnalysisResult struct {
	Diagnostic         result.Result
	Score              score.Scorecard
	EnrichmentEvidence *EnrichmentEvidence
	relationships      relationship.Set
	// gateReasons are the coupling gate's explanations, carried into the state.
	gateReasons []string
}

// PolicyPreparer validates decoded policy and emits config-quality diagnostics
// before any technical stage runs. It is a real boundary: validation reads the
// policy language the config adapter owns.
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
	// ModelHash and LabelsHash identify the module map and approved labels.
	ModelHash  string
	LabelsHash string
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
	PinnedLabels          []labels.Label
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
	// may touch. Rule and metric evaluation deliberately reads the RAW coverage
	// on Facts, so a config opt-out cannot move a measured metric.
	//
	// CoverageGaps elements are mutated IN PLACE by the assessment stage: under
	// --require-tools, the tool gate stamps Gate on the gap it fails on. An
	// AnalysisContext must therefore never be cached or shared across two
	// scorings: acquisition allocates a fresh slice per run.
	MarkedCoverage []modevidence.Coverage
	CoverageGaps   []modevidence.CoverageGap
	// CrateRootDirs maps a Rust crate name to its repo-relative source dir, for
	// resolving crate-scoped repair-task paths.
	CrateRootDirs map[string]string
	// VolatilityCorroboration is report-only git-history evidence. It never
	// changes a score, a finding, or a verdict.
	VolatilityCorroboration *modevidence.VolatilityCorroboration
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
	assessed, err := s.assess(ctx, req, acquired)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("assessment: %w", err)
	}
	return assessed, nil
}

// relate runs the relationship stage. It is a pure decision over the acquired
// facts and the run policy resolved during acquisition.
func relate(acquired Acquired) relationship.AnalysisResult {
	facts, runCtx := acquired.Facts, acquired.Context
	return analysis.Analyze(analysis.Input{
		Graph: facts.Graph, Policy: runCtx.Policy.Relationship,
		Labels:        runCtx.PinnedLabels,
		CloneClusters: facts.Clones, FileClassIndex: facts.FileClassIndex,
		RuntimeSites: facts.RuntimeAsyncSites, RuntimeConfidence: facts.RuntimeConfidence,
		DynamicImportSites: facts.DynamicImports,
	})
}

// assess runs relationship analysis, assessment, and scoring in one shared order.
func (s StageExecutor) assess(_ context.Context, req AnalysisRequest, acquired Acquired) (AnalysisResult, error) {
	runCtx := acquired.Context
	facts := acquired.Observations
	s.reportPhase("Analyzing dependencies")
	relationships := relate(acquired)
	assessed, err := evaluation.Assess(evaluation.AssessInput{
		Facts: facts, Relationships: relationships.Relationships, RelationshipSignals: relationships.Assessment, Policy: runCtx.Policy,
		Scope: runCtx.Scope, Now: runCtx.Now, BaseRef: req.BaseRef,
		Advisory:     !req.NoAdvisories,
		ConfigSource: runCtx.ConfigSource, ScanRoot: runCtx.ScanRoot, ConfigHash: runCtx.ConfigHash,
		ModelHash: runCtx.ModelHash, LabelsHash: runCtx.LabelsHash, ClassificationHash: runCtx.ClassificationHash,
		PrimaryExtractorTools: runCtx.PrimaryExtractorTools, OwnerSource: runCtx.OwnerSource,
		ConfigWarnings: runCtx.ConfigWarnings, MarkedCoverage: runCtx.MarkedCoverage,
		CoverageGaps: runCtx.CoverageGaps, VolatilityCorroboration: runCtx.VolatilityCorroboration,
		DeployUnitDetectedModules: runCtx.DeployUnitDetectedModules,
	})
	if err != nil {
		return AnalysisResult{}, err
	}
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
		ConfigSource:  cmp.Or(req.ValidationConfig, runCtx.ConfigSource),
		ScanRoot:      cmp.Or(req.ValidationRoot, runCtx.ScanRoot),
		Root:          runCtx.Scope.Root,
		CrateRootDirs: runCtx.CrateRootDirs, RequireTools: req.RequireTools,
		ValidationArgs: req.ValidationArgs,
		ConfigWarnings: runCtx.ConfigWarnings, MarkedCoverage: runCtx.MarkedCoverage,
		CoverageGaps: runCtx.CoverageGaps, ApplyToolGate: req.ApplyToolGate,
	})
	if !req.SuppressGateReasons && !req.GateReasonsInState {
		s.discloseGate(scored)
	}
	out := AnalysisResult{Score: scored.Score, gateReasons: scored.GateReasons}
	out.Diagnostic = diag
	if req.CaptureRelationships {
		out.EnrichmentEvidence = projectEnrichmentEvidence(relationships.Relationships)
		out.relationships = relationships.Relationships
	}
	return out, nil
}

func (s StageExecutor) discloseGate(scored evaluation.Scored) {
	for _, reason := range scored.GateReasons {
		_, _ = fmt.Fprintln(s.stderr(), "coupling gate: "+reason)
	}
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

// Service owns the Analyze/Check use case. Cmd supplies the four explicit
// staged ports and renders the returned document.
type Service struct {
	Stages StageExecutor
}

// Execute validates the request, invokes the technical stage, projects the
// report, and assigns outcome semantics.
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
		RequireTools: req.RequireTools, ApplyToolGate: true, DiscloseHealthWarnings: true,
		ValidationArgs: req.ValidationArgs, ValidationConfig: req.ValidationConfig, ValidationRoot: req.ValidationRoot,
		GateReasonsInState: req.GateReasonsInState, AddedBase: req.AddedBase,
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
		Document: withRunFacts(projectReport(out.Diagnostic, out.Score), req.Identity, out.gateReasons),
		Formats:  formats,
		Outcome:  outcomeFor(out),
	}, nil
}

// outcomeFor maps the architecture verdict onto the use-case outcome.
//
// The verdict is the single source: blocked → fail, needs_attention → warn,
// healthy → pass. A required-analyzer gate and a failing hard rule both reach
// the verdict through the state's own hard-gate result, so there is nothing left
// to outrank it here — a second condition at this layer could only disagree with
// the report the same run just printed.
//
// A run with only coupling advisories is needs_attention, which is exit 2 for
// check and still exit 0 for analyze. It is never exit 1: a diagnostic is not a
// blocker.
func outcomeFor(out AnalysisResult) Outcome {
	switch out.Diagnostic.State.Verdict {
	case state.Blocked:
		return OutcomeFail
	case state.NeedsAttention:
		return OutcomeWarn
	default:
		return OutcomePass
	}
}
