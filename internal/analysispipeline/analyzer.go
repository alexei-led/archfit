package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/assessment/score"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	relationshipanalysis "github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
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

// Acquire implements application.EvidenceStage. It returns neutral facts and
// keeps no run result on the analyzer.
func (a *Analyzer) Acquire(ctx context.Context, req application.AnalysisRequest) (evidence.Snapshot, error) {
	if a == nil {
		return evidence.Snapshot{}, errors.New("analysis runner is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	run, err := a.prepareRun(ctx, req)
	if err != nil {
		return evidence.Snapshot{}, err
	}
	acquired, err := acquireStage(ctx, run.input)
	if err != nil {
		return evidence.Snapshot{}, err
	}
	coverage := append([]evidence.Coverage(nil), acquired.acquired.Coverages...)
	if ex := registry.RustExtractor(run.input.Extractors); ex != nil {
		coverage = append(coverage, ex.LastModuleGraphCoverage())
	}
	return evidence.Snapshot{
		Graph: acquired.acquired.Graph, Coverage: coverage,
		Symbols: acquired.acquired.SCIPSymbols, PatternMatches: acquired.ruleEvidence.evidence.PatternMatches,
		SyntaxFacts: acquired.ruleEvidence.evidence.SyntaxFacts, FileLOC: run.input.Signals.Size.FileLOC,
		FileClassIndex: run.input.Signals.Size.FileClassIndex, Clones: run.input.Signals.Duplication.Clusters,
		DynamicImports: run.input.Signals.DynamicImports.Sites, RuntimeAsyncSites: run.input.Signals.RuntimeAsync.Sites,
		RuntimeConfidence: run.input.Signals.RuntimeAsync.Confidence, DeprecatedDeps: run.input.Signals.DeprecatedDeps,
		DeployUnitsByModule: run.input.Policy.DeployUnits, SemanticStrengthOverlay: acquired.acquired.SemanticStrengthOverlay, Scope: run.input.Scope,
		BaseRef: req.BaseRef, Full: run.input.Mode.Full, PinnedLabels: run.policy.Relationship.PinnedLabels,
		Now: run.input.Now, ConfigHash: run.input.ConfigHash, PrimaryExtractorTools: run.input.PrimaryExtractorTools,
		ConfigSource: run.runContext.ConfigSource, BundleDir: run.runContext.BundleDir,
	}, nil
}

// Relate implements application.RelationshipStage. It consumes only the
// immutable snapshot and prepared configuration.
func (a *Analyzer) Relate(ctx context.Context, in evidence.Snapshot) (relationship.AnalysisResult, error) {
	if a == nil {
		return relationship.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.prepared {
		return relationship.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	p := a.PreparedSnapshot.Clone()
	for name, unit := range in.DeployUnitsByModule {
		if def, ok := p.Topology.Modules[name]; ok && def.DeployUnit == "" {
			def.DeployUnit = unit
			p.Topology.Modules[name] = def
		}
		p.DeployUnits[name] = unit
	}
	if resolvedOwners, _ := resolveOwners(ctx, in.Scope, p, a.Deps); len(resolvedOwners) > 0 {
		for name, owner := range resolvedOwners {
			if def, ok := p.Topology.Modules[name]; ok && def.Owner == "" {
				def.Owner = owner
				p.Topology.Modules[name] = def
				p.Ownership[name] = owner
			}
		}
	}
	p.Topology.ModuleMap = module.BuildMap(p.Topology.Modules)
	p.Relationship.Topology, p.Assessment.Topology = p.Topology, p.Topology
	p.Relationship.PinnedLabels = append([]labels.Label(nil), in.PinnedLabels...)
	return relationshipanalysis.Analyze(relationshipanalysis.Input{
		Graph: in.Graph, Policy: p.Relationship,
		Mode: relationshipanalysis.Mode{Base: in.BaseRef, Full: in.Full}, Labels: in.PinnedLabels,
		CloneClusters: in.Clones, FileClassIndex: in.FileClassIndex,
		RuntimeSites: in.RuntimeAsyncSites, RuntimeConfidence: in.RuntimeConfidence,
	}), nil
}

// Assess implements application.AssessmentStage. It consumes explicit values
// and reconstructs assessment inputs without prior-run analyzer state.
func (a *Analyzer) Assess(ctx context.Context, req application.AnalysisRequest, view evidence.AssessmentView, in relationship.AnalysisResult) (application.AnalysisResult, error) {
	if a == nil {
		return application.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.prepared {
		return application.AnalysisResult{}, errors.New("analysis stage is not prepared")
	}
	p := a.PreparedSnapshot.Clone()
	ownerSource := "config"
	var warnings []string
	for name, unit := range view.DeployUnitsByModule {
		p.DeployUnits[name] = unit
	}
	needOwners := false
	for _, def := range a.PreparedSnapshot.Topology.Modules {
		if len(def.Paths) > 0 && def.Owner == "" {
			needOwners = true
			break
		}
	}
	if needOwners {
		resolvedOwners, source := resolveOwners(ctx, view.Scope, p, a.Deps)
		ownerSource = string(source)
		if warning := OwnerDegradationWarning(source); warning != "" {
			warnings = append(warnings, warning)
			a.Deps.warn(warning)
		}
		for name, owner := range resolvedOwners {
			if def, ok := p.Topology.Modules[name]; ok && def.Owner == "" {
				def.Owner = owner
				p.Topology.Modules[name] = def
				p.Ownership[name] = owner
			}
		}
		p.Topology.ModuleMap = module.BuildMap(p.Topology.Modules)
		p.Relationship.Topology, p.Assessment.Topology = p.Topology, p.Topology
	}
	ruleset, err := rules.New(p.Gates.Rules)
	if err != nil {
		return application.AnalysisResult{}, err
	}
	metricset := metrics.New(p.Gates.Metrics)
	var captured signal.CommonInput
	if req.CaptureRelationships {
		metricset = append(metricset, &relationshipCapture{in: &captured})
	}
	base := baseline.Baseline{}
	if !req.EmptyBaseline {
		bundleDir := view.BundleDir
		if bundleDir == "" {
			bundleDir = filepath.Dir(view.ConfigSource)
		}
		base, _ = baseline.Load(ctx, filepath.Join(bundleDir, ".archfit-baseline.json"))
	}
	input := StageInput{Mode: Mode{Base: req.BaseRef, Full: true, Advisory: !req.NoAdvisories, ReportOnly: req.ReportOnly, Formats: req.Formats},
		Scope: view.Scope, Policy: p, Rules: ruleset, Metrics: metricset, Signals: signal.RunSignals{
			Size:           signal.SizeSignals{FileLOC: view.FileLOC, FileClassIndex: view.FileClassIndex},
			Duplication:    signal.DuplicationSignals{Clusters: view.Clones},
			DynamicImports: signal.DynamicImportSignals{Sites: view.DynamicImports},
			RuntimeAsync:   signal.RuntimeAsyncSignals{Sites: view.RuntimeAsyncSites, Confidence: view.RuntimeConfidence},
			DeprecatedDeps: view.DeprecatedDeps,
		}, BaseMetrics: base.Metrics, Accepted: base, Now: view.Now, ConfigHash: view.ConfigHash, PrimaryExtractorTools: view.PrimaryExtractorTools}
	acquired := acquiredStage{acquired: acquisition.Result{Graph: view.Graph, Coverages: view.Coverage, SCIPSymbols: view.Symbols, SemanticStrengthOverlay: view.SemanticStrengthOverlay}, ruleEvidence: ruleEvidence{evidence: rules.Evidence{PatternMatches: view.PatternMatches, SyntaxFacts: view.SyntaxFacts}}}
	relations := relationshipStageFromAnalysis(in)
	diag, err := projectAssessment(input, acquired, relations)
	if err != nil {
		return application.AnalysisResult{}, err
	}
	configSource := view.ConfigSource
	if configSource == "" {
		configSource = a.ConfigPath
	}
	rc := NewRunContext(configSource, a.Root)
	rc.BundleDir = view.BundleDir
	if rc.BundleDir == "" {
		rc.BundleDir = filepath.Dir(configSource)
	}
	run := &preparedRun{input: input, acquired: acquired, relations: relations, base: base, runContext: rc,
		mode: input.Mode, request: req, options: a.PreparedOptions, policy: p, config: a.Config,
		ownerSource: ownerSource, warnings: warnings, requireTools: req.RequireTools, captureRelationships: req.CaptureRelationships, captured: &captured}
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
			out = append(out, fmt.Sprintf("%s %d, current %d", input, b.Score.EffectiveRubricVersion(), score.RubricVersion))
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
