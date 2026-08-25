package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/scope"
)

type preparedRun struct {
	input                StageInput
	acquired             acquiredStage
	relations            relationshipStage
	base                 baseline.Baseline
	runContext           RunContext
	mode                 Mode
	request              application.AnalysisRequest
	options              RunOptions
	policy               policy.PolicySnapshot
	config               config.Config
	ownerSource          string
	labels               []labels.Label
	warnings             []string
	ownerWarnings        []string
	captured             *relationship.Set
	requireTools         bool
	captureRelationships bool
	previousWarnLabel    string
}

// prepareRun performs scope, cache, adapter, and policy-input setup. It
// does not acquire graph evidence; that is the first visible technical stage.
func (a *Analyzer) prepareRun(ctx context.Context, req application.AnalysisRequest) (*preparedRun, error) {
	if a == nil || a.Deps == nil || a.Deps.Runner == nil {
		return nil, errors.New("analysis runner is required")
	}
	if !a.prepared {
		return nil, errors.New("analysis stage is not prepared")
	}
	deps := a.Deps
	previousWarnLabel := deps.WarnLabel
	if req.WarnLabel != "" {
		deps.WarnLabel = req.WarnLabel
	}
	configPath := a.ConfigPath
	if req.ConfigSource != "" {
		configPath = req.ConfigSource
	}
	bundleDir := filepath.Dir(configPath)
	if req.BundleDir != "" {
		bundleDir = req.BundleDir
	}
	deps.LabelsPath = filepath.Join(bundleDir, ".archfit-labels.yaml")
	base := baseline.Baseline{}
	var err error
	if !req.EmptyBaseline {
		base, err = baseline.Load(ctx, filepath.Join(bundleDir, ".archfit-baseline.json"))
		if err != nil {
			return nil, err
		}
	}
	mode := Mode{Base: req.BaseRef, Full: true, Advisory: !req.NoAdvisories, ReportOnly: req.ReportOnly, Formats: req.Formats}
	root := a.Root
	if req.Root != "" {
		root = req.Root
	}
	rc := NewRunContext(configPath, root)
	rc.BundleDir = bundleDir
	if !req.EvaluatedAt.IsZero() {
		rc.EvaluatedAt = req.EvaluatedAt
	} else {
		rc.EvaluatedAt = time.Now()
	}

	sc := a.PreparedOptions.Scope
	sc.WorkDir, sc.Root, sc.Base, sc.Full = rc.scanDir(), rc.ScanRoot, mode.Base, mode.Full
	deps.reportPhase("Discovering project")
	resolved, err := scope.Resolve(ctx, sc, gitResolver{workDir: rc.scanDir(), runner: deps.Runner})
	if err != nil {
		return nil, err
	}
	deps.ResolvedRoot = resolved.Root

	runPolicy := a.PreparedSnapshot.Clone()
	facts := factcache.NewStore(factsCacheDir(bundleDir))
	facts.RefreshMode = deps.Refresh
	extractors := registry.Build(deps.Runner, a.PreparedOptions.Extractors, facts)
	ruleSet, err := evaluation.NewRuleset(runPolicy.Gates.Rules)
	if err != nil {
		return nil, err
	}
	metricSet := evaluation.NewMetricset(runPolicy.Gates.Metrics)
	var warnings []string
	noteWarning := func(tool string, e error) {
		if e != nil {
			msg := tool + ": " + e.Error()
			warnings = append(warnings, msg)
			deps.warn(msg)
		}
	}
	if abs, absErr := filepath.Abs(bundleDir); absErr == nil {
		if warning := OutputInsideRootWarning(resolved.Root, abs); warning != "" {
			warnings = append(warnings, warning)
			deps.warn(warning)
		}
	}

	deps.reportPhase("Collecting facts")
	factsResult := acquire.Collect(ctx, resolved.Root, a.PreparedOptions.Acquisition, deps.Runner, facts)
	change := signal.RunSignals{
		Size:        signal.SizeSignals{FileLOC: factsResult.FileLOC, FileClassIndex: factsResult.FileClassIndex},
		Duplication: signal.DuplicationSignals{Clusters: factsResult.DuplicationClusters}, ExtraCoverage: factsResult.ExtraCoverage,
		DynamicImports: signal.DynamicImportSignals{Sites: factsResult.DynamicImports},
		RuntimeAsync:   signal.RuntimeAsyncSignals{Sites: factsResult.RuntimeAsyncSites, Confidence: factsResult.RuntimeConfidence},
		DeprecatedDeps: factsResult.DeprecatedDeps,
	}
	noteWarning("loc", factsResult.LOCError)
	// Ownership and deploy-unit resolution runs exactly once per run. The
	// resolved snapshot travels on the analysis context; relationship analysis
	// and assessment read it instead of re-walking repository history.
	ownerSource := "config"
	var ownerWarnings []string
	var resolvedOwners map[string]string
	if runPolicy.NeedsOwnerResolution() {
		owners, source := resolveOwners(ctx, resolved, runPolicy, deps)
		resolvedOwners = owners
		ownerSource = string(source)
		if warning := OwnerDegradationWarning(source); warning != "" {
			ownerWarnings = append(ownerWarnings, warning)
			warnings = append(warnings, warning)
			deps.warn(warning)
		}
	}
	runPolicy = runPolicy.WithResolvedTopology(resolvedOwners, factsResult.DeployUnitsByModule)
	noteWarning("jscpd", factsResult.CloneError)
	lbls, err := deps.loadLabels(bundleDir)
	if err != nil {
		return nil, err
	}
	var captured relationship.Set
	stageInput := StageInput{Mode: mode, Scope: resolved, Policy: runPolicy, Extractors: extractors,
		Patterns: factsResult.Patterns, Resolver: factsResult.Resolver, Syntax: factsResult.Syntax,
		SyntaxCfg: a.PreparedOptions.Syntax, PatternCfg: a.PreparedOptions.Patterns, Rules: ruleSet,
		Metrics: metricSet, BaseMetrics: base.Metrics, Accepted: base, Signals: change, Labels: lbls,
		CaptureRelationships: req.CaptureRelationships,
		Now:                  rc.evaluatedAt(), ConfigHash: effectiveConfigHash(configPath), PrimaryExtractorTools: registry.PrimaryTools()}
	return &preparedRun{input: stageInput, base: base, runContext: rc,
		mode: mode, request: req, options: a.PreparedOptions, policy: runPolicy, config: a.Config, ownerSource: ownerSource,
		labels: lbls, warnings: warnings, ownerWarnings: ownerWarnings, requireTools: req.RequireTools,
		captureRelationships: req.CaptureRelationships, previousWarnLabel: previousWarnLabel, captured: &captured}, nil
}

func resolveOwners(ctx context.Context, s scope.Scope, p policy.PolicySnapshot, deps *Deps) (map[string]string, ownership.Source) {
	return ownership.Resolve(ctx, s.Root, s.GitRoot, s.SubtreePrefix, p.Topology.ModuleMap, deps.Runner)
}

// finalizePreparedRun performs score, gate, report-only evidence, and repair-task
// attachment after assessment. It does not acquire or classify relationships.
func (a *Analyzer) finalizePreparedRun(ctx context.Context, run *preparedRun, diag result.Result) (application.AnalysisResult, error) {
	deps := a.Deps
	defer func() { deps.WarnLabel = run.previousWarnLabel }()
	diag.OwnerSource = run.ownerSource
	diag.DistanceContext = BuildDistanceContext(diag, run.policy, len(run.policy.DeployUnits))
	diag.VolatilityCorroboration = BuildVolatilityCorroboration(ctx, run.input.Scope.GitRoot, run.input.Scope.SubtreePrefix, run.policy, deps.Runner)
	if warning := TSUnresolvedWarning(diag.ToolCoverage); warning != "" {
		run.warnings = append(run.warnings, warning)
		deps.warn(warning)
	}
	if warning := PyUnresolvedWarning(diag.ToolCoverage); warning != "" {
		run.warnings = append(run.warnings, warning)
		deps.warn(warning)
	}
	if ex := registry.RustExtractor(run.input.Extractors); ex != nil {
		diag.ToolCoverage = append(diag.ToolCoverage, ex.LastModuleGraphCoverage())
	}
	EmitHealthWarnings(deps, diag, run.policy.Topology.Modules, run.input.Scope.Root, run.runContext.ConfigSource)
	deps.reportPhase("Scoring architecture")
	ruleTypes := make(map[string]string, len(run.policy.Gates.Rules.Rules))
	for _, def := range run.policy.Gates.Rules.Rules {
		ruleTypes[def.ID] = def.Type
	}
	modulePublic := make(map[string][]string, len(run.policy.Topology.Modules))
	for name, def := range run.policy.Topology.Modules {
		if len(def.Public) > 0 {
			modulePublic[name] = def.Public
		}
	}
	knownFiles := make(map[string]struct{}, len(run.input.Signals.Size.FileClassIndex))
	for file := range run.input.Signals.Size.FileClassIndex {
		knownFiles[file] = struct{}{}
	}
	crateRootDirs := map[string]string{}
	if ex := registry.RustExtractor(run.input.Extractors); ex != nil {
		for _, cr := range ex.LastCrateRoots() {
			crateRootDirs[cr.Name] = cr.Dir
		}
	}
	anchor := evaluation.BaselineAnchor{CouplingScore: run.base.CouplingScore(), SnapshotMismatches: run.base.ScoreSnapshotMismatches()}
	gate := run.policy.Gates.Coupling
	finalized := evaluation.Finalize(&diag, evaluation.FinalizeInput{
		Gate: gate, Baseline: anchor, RuleTypes: ruleTypes, ModulePublic: modulePublic,
		ValidationCommands: []string{ValidationCommand(run.runContext.ConfigSource, run.runContext.ScanRoot)},
		KnownFiles:         knownFiles, CrateRootDirs: crateRootDirs,
		ModuleRootDirs: policy.ModuleRootDirs(run.policy.Topology.Modules),
		OnDisk:         OnDiskWithin(run.input.Scope.Root),
	})
	card := finalized.Score
	if !run.request.SuppressGateReasons {
		for _, reason := range finalized.GateReasons {
			_, _ = fmt.Fprintln(deps.stderr(), "coupling gate: "+reason)
		}
		if evaluation.CouplingGateAnchorStale(gate, anchor) {
			_, _ = fmt.Fprintf(deps.stderr(), "coupling gate: max_drop skipped — baseline score snapshot is incompatible (%s); run `archfit baseline` to re-anchor\n", strings.Join(scoreSnapshotMismatchDetails(run.base, anchor.SnapshotMismatches), ", "))
		}
	}
	diag.ToolCoverage = MarkDisabledPrimaries(diag.ToolCoverage, Coverage(run.config), run.input.Scope.Root)
	diag.CoverageGaps = BuildCoverageGaps(diag.ToolCoverage, Coverage(run.config), run.input.Scope.Root)
	diag.ConfigWarnings = BuildConfigWarnings(run.options.LintWarnings, run.warnings)
	diag.ConfigWarnings = append(diag.ConfigWarnings, BuildJudgmentDecisionTasks(run.policy.Topology.Modules, run.labels, run.runContext.ConfigSource)...)
	out := application.AnalysisResult{Score: card, HardGate: ApplyToolGate(&diag, runInputRequireTools(run))}
	if run.mode.Base != "" {
		if deps.Progress != nil {
			deps.reportPhase("Comparing against base")
		}
		baseCfg := WithIndependentModules(a.Config)
		bsc, bev, err := ScoreBaseRef(ctx, deps, run.mode.Base, run.runContext, Options(baseCfg), PolicySnapshot(baseCfg), baseCfg, run.mode.Advisory)
		if err != nil {
			return application.AnalysisResult{}, err
		}
		out.BaseScore = &bsc.Score
		settings := AnalyzerSettings(a.Config)
		evaluation.AttachGitOrigin(&diag, evaluation.GitOriginInput{
			BaseRef: run.mode.Base, BaseFindingIDs: bev.FindingIDs,
			HeadCoverage: diag.ToolCoverage, HeadGaps: diag.CoverageGaps, HeadConfigHash: diag.ConfigHash,
			BaseCoverage: bev.Coverage, BaseGaps: bev.CoverageGaps, BaseConfigHash: bev.ConfigHash,
			PrimaryTools: registry.PrimaryTools(), Patterns: settings.Patterns, Syntax: settings.Syntax,
			SCIP: settings.SCIP, Clones: settings.Clones, CargoModules: settings.CargoModules,
		})
	}
	out.Diagnostic = diag
	if runInputCapture(run) && run.captured != nil {
		out.EnrichmentEvidence = projectEnrichmentEvidence(*run.captured)
	}
	return out, nil
}

func runInputRequireTools(run *preparedRun) bool { return run.requireTools }
func runInputCapture(run *preparedRun) bool      { return run.captureRelationships }
