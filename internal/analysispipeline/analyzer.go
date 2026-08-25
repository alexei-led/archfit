package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	relationshipanalysis "github.com/alexei-led/archfit/internal/relationship/analysis"
)

// Analyzer is the concrete implementation of the application stage ports. It
// retains per-run adapters and state between the three ordered calls; the
// application owns the order and never calls a broad analyzer operation.
type Analyzer struct {
	ConfigPath       string
	Root             string
	Config           config.Config
	Deps             *Deps
	PreparedOptions  RunOptions
	PreparedSnapshot policy.PolicySnapshot
	prepared         bool
	mu               sync.Mutex
}

// NewAnalyzer builds the concrete stage from prepared configuration and the
// process adapters supplied by the composition root.
func NewAnalyzer(configPath, root string, cfg config.Config, deps *Deps) *Analyzer {
	return &Analyzer{ConfigPath: configPath, Root: root, Config: cfg, Deps: deps}
}

// Prepare validates decoded policy and emits config-quality diagnostics before
// the application invokes the technical stage.
func (a *Analyzer) Prepare(context.Context) error {
	if a == nil {
		return errors.New("analysis runner is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Deps == nil || a.Deps.Runner == nil {
		return errors.New("analysis runner is required")
	}
	if err := ValidateConfigRules(a.Config); err != nil {
		return err
	}
	PrintConfigLint(a.Deps.stderr(), a.Config.Lint())
	a.PreparedOptions = Options(a.Config)
	a.PreparedSnapshot = PolicySnapshot(a.Config)
	a.prepared = true
	return nil
}

// Acquire implements application.EvidenceStage. It returns neutral facts plus
// the run context they were acquired under, and keeps no run result on the
// analyzer. Ownership and deploy-unit resolution happens exactly once, here in
// preparation; the resolved PolicySnapshot rides the context to every later
// stage.
func (a *Analyzer) Acquire(ctx context.Context, req application.AnalysisRequest) (application.Acquired, error) {
	if a == nil {
		return application.Acquired{}, errors.New("analysis runner is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	run, err := a.prepareRun(ctx, req)
	if err != nil {
		return application.Acquired{}, err
	}
	acquired, err := acquireStage(ctx, run.input)
	if err != nil {
		return application.Acquired{}, err
	}
	coverage := append([]evidence.Coverage(nil), acquired.acquired.Coverages...)
	if ex := registry.RustExtractor(run.input.Extractors); ex != nil {
		coverage = append(coverage, ex.LastModuleGraphCoverage())
	}
	return application.Acquired{
		Facts: evidence.Facts{
			Graph: acquired.acquired.Graph, Coverage: coverage,
			Symbols: acquired.acquired.SCIPSymbols, PatternMatches: acquired.ruleEvidence.evidence.PatternMatches,
			SyntaxFacts: acquired.ruleEvidence.evidence.SyntaxFacts, FileLOC: run.input.Signals.Size.FileLOC,
			FileClassIndex: run.input.Signals.Size.FileClassIndex, Clones: run.input.Signals.Duplication.Clusters,
			DynamicImports: run.input.Signals.DynamicImports.Sites, RuntimeAsyncSites: run.input.Signals.RuntimeAsync.Sites,
			RuntimeConfidence: run.input.Signals.RuntimeAsync.Confidence, DeprecatedDeps: run.input.Signals.DeprecatedDeps,
			SemanticStrengthOverlay: acquired.acquired.SemanticStrengthOverlay,
		},
		Context: application.AnalysisContext{
			Scope: run.input.Scope, BaseRef: req.BaseRef, Full: run.input.Mode.Full,
			Now: run.input.Now, ConfigHash: run.input.ConfigHash, PrimaryExtractorTools: run.input.PrimaryExtractorTools,
			ConfigSource: run.runContext.ConfigSource, BundleDir: run.runContext.BundleDir,
			PinnedLabels: run.labels, Policy: run.policy,
			OwnerSource: run.ownerSource, OwnerWarnings: run.ownerWarnings,
		},
	}, nil
}

// Relate implements application.RelationshipStage. It consumes only the
// immutable facts and the run context: the policy it classifies against was
// resolved once during Acquire and is not re-derived here.
func (a *Analyzer) Relate(_ context.Context, in application.Acquired) (relationship.AnalysisResult, error) {
	if a == nil {
		return relationship.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.prepared {
		return relationship.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	facts, runCtx := in.Facts, in.Context
	return relationshipanalysis.Analyze(relationshipanalysis.Input{
		Graph: facts.Graph, Policy: runCtx.Policy.Relationship,
		Mode: relationshipanalysis.Mode{Base: runCtx.BaseRef, Full: runCtx.Full}, Labels: runCtx.PinnedLabels,
		CloneClusters: facts.Clones, FileClassIndex: facts.FileClassIndex,
		RuntimeSites: facts.RuntimeAsyncSites, RuntimeConfidence: facts.RuntimeConfidence,
	}), nil
}

// Assess implements application.AssessmentStage. It consumes explicit values
// and reconstructs assessment inputs without prior-run analyzer state.
func (a *Analyzer) Assess(ctx context.Context, req application.AnalysisRequest, facts evidence.AssessmentFacts, runCtx application.AnalysisContext, in relationship.AnalysisResult) (application.AnalysisResult, error) {
	if a == nil {
		return application.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.prepared {
		return application.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	p := runCtx.Policy
	ruleset, err := evaluation.NewRuleset(p.Gates.Rules)
	if err != nil {
		return application.AnalysisResult{}, err
	}
	metricset := evaluation.NewMetricset(p.Gates.Metrics)
	var captured relationship.Set
	base := baseline.Baseline{}
	if !req.EmptyBaseline {
		bundleDir := runCtx.BundleDir
		if bundleDir == "" {
			bundleDir = filepath.Dir(runCtx.ConfigSource)
		}
		base, _ = baseline.Load(ctx, filepath.Join(bundleDir, ".archfit-baseline.json"))
	}
	input := StageInput{Mode: Mode{Base: req.BaseRef, Full: true, Advisory: !req.NoAdvisories, ReportOnly: req.ReportOnly, Formats: req.Formats},
		Scope: runCtx.Scope, Policy: p, Rules: ruleset, Metrics: metricset, Signals: signal.RunSignals{
			Size:           signal.SizeSignals{FileLOC: facts.FileLOC, FileClassIndex: facts.FileClassIndex},
			Duplication:    signal.DuplicationSignals{Clusters: facts.Clones},
			DynamicImports: signal.DynamicImportSignals{Sites: facts.DynamicImports},
			RuntimeAsync:   signal.RuntimeAsyncSignals{Sites: facts.RuntimeAsyncSites, Confidence: facts.RuntimeConfidence},
			DeprecatedDeps: facts.DeprecatedDeps,
		}, Labels: runCtx.PinnedLabels, BaseMetrics: base.Metrics, Accepted: base, Now: runCtx.Now,
		CaptureRelationships: req.CaptureRelationships,
		ConfigHash:           runCtx.ConfigHash, PrimaryExtractorTools: runCtx.PrimaryExtractorTools}
	acquired := acquiredStage{acquired: acquisition.Result{Graph: facts.Graph, Coverages: facts.Coverage, SCIPSymbols: facts.Symbols, SemanticStrengthOverlay: facts.SemanticStrengthOverlay}, ruleEvidence: ruleEvidence{evidence: evaluation.RuleEvidence{PatternMatches: facts.PatternMatches, SyntaxFacts: facts.SyntaxFacts}}}
	relations := relationshipStageFromAnalysis(in)
	diag, capturedSet := projectAssessment(input, acquired, relations)
	captured = capturedSet
	configSource := runCtx.ConfigSource
	if configSource == "" {
		configSource = a.ConfigPath
	}
	rc := NewRunContext(configSource, a.Root)
	rc.BundleDir = runCtx.BundleDir
	if rc.BundleDir == "" {
		rc.BundleDir = filepath.Dir(configSource)
	}
	run := &preparedRun{input: input, acquired: acquired, relations: relations, base: base, runContext: rc,
		mode: input.Mode, request: req, options: a.PreparedOptions, policy: p, config: a.Config, labels: runCtx.PinnedLabels,
		ownerSource: runCtx.OwnerSource, warnings: runCtx.OwnerWarnings, requireTools: req.RequireTools, captureRelationships: req.CaptureRelationships, captured: &captured}
	return a.finalizePreparedRun(ctx, run, diag)
}

// scoreSnapshotMismatchDetails is kept with the concrete baseline adapter so
// the application package does not depend on baseline persistence types.
func scoreSnapshotMismatchDetails(b baseline.Baseline, mismatches []string) []string {
	out := make([]string, 0, len(mismatches))
	for _, input := range mismatches {
		switch input {
		case baseline.InputScoreVersion:
			out = append(out, fmt.Sprintf("%s %q, current %q", input, b.Score.ScoreVersion, report.ScoreVersion))
		case baseline.InputRubricVersion:
			out = append(out, fmt.Sprintf("%s %d, current %d", input, b.Score.EffectiveRubricVersion(), report.RubricVersion))
		default:
			out = append(out, input)
		}
	}
	return out
}

var _ application.PolicyPreparer = (*Analyzer)(nil)
var _ application.EvidenceStage = (*Analyzer)(nil)
var _ application.RelationshipStage = (*Analyzer)(nil)
var _ application.AssessmentStage = (*Analyzer)(nil)
