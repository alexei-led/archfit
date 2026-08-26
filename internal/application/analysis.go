// Package application owns user-facing analysis use-case contracts.
package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/evidence"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/scope"
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
}

// AnalysisRequest is the narrow technical-stage input. The application owns
// validation and sequencing; the stage owns evidence collection and scoring.
type AnalysisRequest struct {
	BaseRef      string
	NoAdvisories bool
	RequireTools bool
	// ApplyToolGate lets a missing required analyzer stamp the verdict fail and
	// hard-gate the run. Only analyze/check set it: baseline, explain, enrich,
	// config compare, and the --base sub-run render a verdict but consume no
	// exit code from it, so a coverage gap must not rewrite what they report.
	ApplyToolGate bool
	// DiscloseHealthWarnings prints the assessment's health hints to stderr. Only
	// analyze/check set it. Every other caller either renders a tree the user
	// cannot act on — the --base sub-run scores a temp worktree that is deleted
	// before control returns, so its hints name paths that no longer exist — or
	// is a report-only stage that never disclosed them.
	DiscloseHealthWarnings bool
	// Comparison and use-case stages may override the technical context while
	// keeping the analyzer implementation shared.
	ConfigSource        string
	BundleDir           string
	Root                string
	EvaluatedAt         time.Time
	EmptyBaseline       bool
	SuppressGateReasons bool
	// WarnLabel prefixes the ACQUISITION stage's stderr warnings, so a sub-run's
	// degradation is not misread as the head run's. Health warnings do not use
	// it: they are analyze/check-only (see DiscloseHealthWarnings).
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
	Scope        scope.Scope
	BaseRef      string
	Full         bool
	Now          time.Time
	ConfigHash   string
	ConfigSource string
	BundleDir    string
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
	// scorings — acquisition allocates a fresh slice per run and that is what
	// keeps the two sides of --base and config compare independent.
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
// acquired under. Application passes it along without inspecting the facts.
type Acquired struct {
	Facts   evidence.Facts
	Context AnalysisContext
}

// EvidenceStage acquires scope-bound facts before relationship analysis. It is
// a real boundary: acquisition walks the source tree, runs external tools, and
// resolves repository ownership. Relationship analysis and assessment are pure
// decisions this package calls directly — a port for either would only hide the
// call.
type EvidenceStage interface {
	Acquire(context.Context, AnalysisRequest) (Acquired, error)
}

// AnalyzerFamilies records which optional analyzer families the config
// activated. The git-origin delta needs it to decide whether two runs compared
// like with like.
type AnalyzerFamilies struct {
	Patterns, Syntax, SCIP, Clones, CargoModules bool
}

// WorktreeProvider materialises a base ref in a clean detached worktree and
// returns the scan root mirroring headRoot inside it. cleanup always runs.
type WorktreeProvider interface {
	Checkout(ctx context.Context, baseRef, anchorDir, headRoot string) (root string, cleanup func(), err error)
}

// StageExecutor sequences one analysis run. It owns the stage order, the single
// baseline read, the scoring boundary, and the base-tree comparison, so analyze,
// check, baseline, explain, and compare cannot diverge.
type StageExecutor struct {
	Preparer PolicyPreparer
	Evidence EvidenceStage
	// Baseline is optional: a nil loader, or a request with EmptyBaseline, runs
	// against an empty accepted set.
	Baseline BaselineLoader
	Stderr   io.Writer
	Progress func(stage string)

	// Worktree and NewBaseEvidence enable `--base`. NewBaseEvidence builds the
	// evidence stage for the base tree; it is a constructor, not a port, because
	// only the composition root knows how to build one.
	Worktree        WorktreeProvider
	NewBaseEvidence func(baseRoot string) EvidenceStage
	Analyzers       AnalyzerFamilies
}

// Execute prepares policy, acquires evidence, classifies relationships, assesses
// the result, scores it, and — when the request names a base ref — attaches the
// base comparison.
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
	base, err := s.loadBaseline(ctx, req, acquired.Context)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("baseline: %w", err)
	}
	assessed, err := s.assess(ctx, req, acquired, base)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("assessment: %w", err)
	}
	return assessed, nil
}

// observationsOf narrows the neutral acquisition snapshot to the observations
// assessment is allowed to decide over. The dependency graph stops here: only
// relationship analysis receives it, and assessment reads its conclusions.
func observationsOf(f evidence.Facts) evaluation.Observations {
	return evaluation.Observations{
		Coverage: f.Coverage, Symbols: f.Symbols, PatternMatches: f.PatternMatches,
		SyntaxFacts: f.SyntaxFacts, FileLOC: f.FileLOC, FileClassIndex: f.FileClassIndex,
		FileFacts: f.FileFacts, Clones: f.Clones, DynamicImports: f.DynamicImports,
		RuntimeAsyncSites: f.RuntimeAsyncSites, RuntimeConfidence: f.RuntimeConfidence,
		DeprecatedDeps: f.DeprecatedDeps, SemanticStrengthOverlay: f.SemanticStrengthOverlay,
	}
}

// relate runs the relationship stage. It is a pure decision over the acquired
// facts and the run policy resolved during acquisition.
func relate(acquired Acquired) relationship.AnalysisResult {
	facts, runCtx := acquired.Facts, acquired.Context
	return analysis.Analyze(analysis.Input{
		Graph: facts.Graph, Policy: runCtx.Policy.Relationship,
		Mode: analysis.Mode{Base: runCtx.BaseRef, Full: runCtx.Full}, Labels: runCtx.PinnedLabels,
		CloneClusters: facts.Clones, FileClassIndex: facts.FileClassIndex,
		RuntimeSites: facts.RuntimeAsyncSites, RuntimeConfidence: facts.RuntimeConfidence,
		DynamicImportSites: facts.DynamicImports,
	})
}

func (s StageExecutor) loadBaseline(ctx context.Context, req AnalysisRequest, runCtx AnalysisContext) (Baseline, error) {
	if req.EmptyBaseline || s.Baseline == nil {
		return Baseline{}, nil
	}
	bundleDir := runCtx.BundleDir
	if bundleDir == "" {
		bundleDir = filepath.Dir(runCtx.ConfigSource)
	}
	return s.Baseline.Load(ctx, bundleDir)
}

// assess runs relationship analysis, assessment, scoring, and the base-tree
// comparison in the one order every use case shares.
func (s StageExecutor) assess(ctx context.Context, req AnalysisRequest, acquired Acquired, base Baseline) (AnalysisResult, error) {
	runCtx := acquired.Context
	facts := observationsOf(acquired.Facts)
	s.reportPhase("Analyzing dependencies")
	relationships := relate(acquired)
	assessed, err := evaluation.Assess(evaluation.AssessInput{
		Facts: facts, Relationships: relationships, Policy: runCtx.Policy,
		Accepted: base.Accepted, BaseMetrics: result.MetricSnapshot(base.Metrics),
		Scope: runCtx.Scope, Now: runCtx.Now, BaseRef: req.BaseRef,
		Advisory: !req.NoAdvisories, CaptureRelationships: req.CaptureRelationships,
		ConfigSource: runCtx.ConfigSource, ScanRoot: runCtx.ScanRoot, ConfigHash: runCtx.ConfigHash,
		PrimaryExtractorTools: runCtx.PrimaryExtractorTools, OwnerSource: runCtx.OwnerSource,
		ConfigWarnings: runCtx.ConfigWarnings, MarkedCoverage: runCtx.MarkedCoverage,
		CoverageGaps: runCtx.CoverageGaps, VolatilityCorroboration: runCtx.VolatilityCorroboration,
		DeployUnitDetectedModules: runCtx.DeployUnitDetectedModules,
	})
	if err != nil {
		return AnalysisResult{}, err
	}
	diag := assessed.Diagnostic
	if req.DiscloseHealthWarnings {
		for _, warning := range assessed.Warnings {
			s.warn(warning)
		}
	}
	s.reportPhase("Scoring architecture")
	scored := evaluation.Score(&diag, evaluation.ScoreInput{
		Policy: runCtx.Policy, Facts: facts,
		Anchor:        evaluation.BaselineAnchor{CouplingScore: base.CouplingScore, SnapshotMismatches: base.SnapshotMismatches},
		ConfigSource:  runCtx.ConfigSource,
		ScanRoot:      runCtx.ScanRoot,
		Root:          runCtx.Scope.Root,
		CrateRootDirs: runCtx.CrateRootDirs, RequireTools: req.RequireTools,
		ConfigWarnings: runCtx.ConfigWarnings, MarkedCoverage: runCtx.MarkedCoverage,
		CoverageGaps: runCtx.CoverageGaps, ApplyToolGate: req.ApplyToolGate,
	})
	if !req.SuppressGateReasons {
		s.discloseGate(scored, base)
	}
	out := AnalysisResult{Score: scored.Score, HardGate: scored.HardGate}
	if req.BaseRef != "" {
		baseScore, err := s.attachBaseComparison(ctx, req, runCtx, &diag)
		if err != nil {
			return AnalysisResult{}, err
		}
		out.BaseScore = baseScore
	}
	out.Diagnostic = diag
	if req.CaptureRelationships {
		out.EnrichmentEvidence = projectEnrichmentEvidence(assessed.Captured)
	}
	return out, nil
}

func (s StageExecutor) discloseGate(scored evaluation.Scored, base Baseline) {
	for _, reason := range scored.GateReasons {
		_, _ = fmt.Fprintln(s.stderr(), "coupling gate: "+reason)
	}
	if scored.AnchorStale {
		_, _ = fmt.Fprintf(s.stderr(), "coupling gate: max_drop skipped — baseline score snapshot is incompatible (%s); run `archfit baseline` to re-anchor\n",
			strings.Join(snapshotMismatchDetails(base), ", "))
	}
}

// snapshotMismatchDetails names each incompatible score-snapshot input with the
// stored and current value, so the notice says what actually changed.
func snapshotMismatchDetails(base Baseline) []string {
	out := make([]string, 0, len(base.SnapshotMismatches))
	for _, input := range base.SnapshotMismatches {
		switch input {
		case baselineInputScoreVersion:
			out = append(out, fmt.Sprintf("%s %q, current %q", input, base.ScoreVersion, report.ScoreVersion))
		case baselineInputRubricVersion:
			out = append(out, fmt.Sprintf("%s %d, current %d", input, base.RubricVersion, report.RubricVersion))
		default:
			out = append(out, input)
		}
	}
	return out
}

// Score-snapshot input names shared with the baseline persistence adapter.
const (
	baselineInputScoreVersion  = "score_version"
	baselineInputRubricVersion = "rubric_version"
)

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

// ValidateFormats reports a mutually exclusive output-format combination
// without running the use case, so a caller can fail a usage error before it
// prints progress or reads a config file it does not need. Execute validates
// again — this is a fail-fast entry point, not the authority.
func ValidateFormats(jsonFlag, markdownFlag, sarifFlag bool, formats []string) error {
	_, err := resolveFormats(jsonFlag, markdownFlag, sarifFlag, formats)
	return err
}

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
		BaseRef: req.BaseRef, NoAdvisories: req.NoAdvisories,
		RequireTools: req.RequireTools, ApplyToolGate: true, DiscloseHealthWarnings: true,
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
		Document: ProjectReport(out.Diagnostic, out.Score, out.BaseScore, out.HardGate),
		Formats:  formats,
		Outcome:  outcomeFor(out),
	}, nil
}

// outcomeFor maps the analysis verdict and the hard analyzer gate onto the
// use-case outcome. A hard gate outranks the verdict: a required analyzer that
// did not run means the run never measured what the policy asked for.
func outcomeFor(out AnalysisResult) Outcome {
	switch {
	case out.HardGate, out.Diagnostic.Verdict == result.VerdictFail:
		return OutcomeFail
	case out.Diagnostic.Verdict == result.VerdictWarn:
		return OutcomeWarn
	default:
		return OutcomePass
	}
}
