package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/agenttask"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/extract/clones"
	"github.com/alexei-led/archfit/internal/extract/deployunit"
	"github.com/alexei-led/archfit/internal/extract/dynimports"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/manifest"
	runtimedetect "github.com/alexei-led/archfit/internal/extract/runtime"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/model/scan"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/score"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// cloneTestGenGlobs are coarse jscpd --ignore patterns that skip test and
// generated files at scan time. These are additive speed hints; correctness
// is enforced by the clone post-filter in the classify stage.
var cloneTestGenGlobs = []string{
	"**/*_test.go",
	"**/*_test.ts",
	"**/*_test.py",
	"**/mock_*.go",
	"**/*_mock.go",
	"**/*_moq.go",
	"**/*.pb.go",
	"**/*_gen.go",
	"**/mocks/**",
	"**/__mocks__/**",
}

// gitResolver adapts internal/history/git to scope.Resolver. The concrete
// git dependency lives here in the composition root — scope itself stays
// free of process and tool dependencies.
type gitResolver struct {
	workDir string
	runner  toolrun.Runner
}

func (g gitResolver) RepoRoot(ctx context.Context) (string, error) {
	return git.RepoRoot(ctx, g.workDir, g.runner)
}

func (g gitResolver) HeadRef(ctx context.Context) (string, error) {
	return git.HeadRef(ctx, g.workDir, g.runner)
}

func (g gitResolver) Changed(ctx context.Context, base, head string) ([]string, error) {
	cs, err := git.Changed(ctx, g.workDir, base, head, "", g.runner)
	if err != nil {
		return nil, err
	}
	return cs.Files, nil
}

// runContext carries the per-run path and time inputs of one pipeline run.
// Before it existed the config path stood in for four unrelated things, so a
// run could not measure one config file while reading labels and facts from
// another directory. Each field now selects exactly one concern.
type runContext struct {
	// ConfigSource is the config file this run measures. It selects the config
	// hash and the validation command emitted in agent_tasks[]. A comparison
	// run points it at the candidate file while BundleDir stays current.
	ConfigSource string

	// BundleDir is the config-bundle directory: pinned labels, the fact cache,
	// the base-worktree cache, and the output-hygiene warning resolve against
	// it. It is the CURRENT config's directory even when ConfigSource names a
	// candidate file somewhere else.
	BundleDir string

	// ScanRoot is the explicit --root analysis boundary. Empty means "resolve
	// the git root" — scope.Resolve falls through to gitRoot and the emitted
	// validation command omits --root. Never default it to BundleDir: that
	// would move the analysis boundary to the config directory and stamp a
	// spurious --root on every repair task.
	ScanRoot string

	// EvaluatedAt is the single instant waiver and staleness logic evaluate
	// against. Zero samples the current time once inside the pipeline, which
	// is what a normal single run wants; the head and base sides of a --base
	// comparison set it explicitly so both sides age findings identically.
	EvaluatedAt time.Time
}

// newRunContext builds the run context of a normal single-config run: the
// config file is both the measured source and the bundle anchor, and the time
// is sampled later inside the pipeline.
func newRunContext(configPath, root string) runContext {
	return runContext{
		ConfigSource: configPath,
		BundleDir:    filepath.Dir(configPath),
		ScanRoot:     root,
	}
}

// baseRunContext derives the base side of a --base comparison from the head
// run context: same config source and bundle directory (so config drift never
// masquerades as code drift), the base worktree as scan root, and the head
// run's evaluation time so both sides resolve waivers and staleness against
// one instant.
func baseRunContext(head runContext, baseRoot string) runContext {
	head.ScanRoot = baseRoot
	return head
}

// evaluatedAt returns the run's evaluation instant, sampling the current time
// when the caller left EvaluatedAt zero.
func (rc runContext) evaluatedAt() time.Time {
	if rc.EvaluatedAt.IsZero() {
		return time.Now()
	}
	return rc.EvaluatedAt
}

// scanDir returns the directory that anchors scope and git resolution. An
// explicit --root decouples the analyzed repo from where the config bundle
// lives (external-CI use case); without one the bundle directory is the
// anchor. This is only the resolution anchor — the analysis boundary itself
// stays rc.ScanRoot, empty included.
func (rc runContext) scanDir() string {
	if rc.ScanRoot != "" {
		return rc.ScanRoot
	}
	return rc.BundleDir
}

// runPipeline resolves scope, builds extractors/rules/metrics, collects change
// history and optional tool inputs, and executes the engine. check, scan,
// explain, and baseline all run through this single path: baseline snapshots
// must be computed from exactly the same inputs as check verdicts, or every
// post-baseline check reports phantom metric regressions and unmatched finding
// fingerprints. After the engine returns, the scorecard is synthesised and the
// coupling gate applied — INSIDE the pipeline, so an escalated verdict and
// promoted findings are visible to agenttask.Build below and to every renderer
// — then the agent_tasks repair block is attached from the active gate
// findings (deterministic; spec §13).
func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, rc runContext, mode engine.Mode, base baseline.Baseline, extraMetrics ...metrics.Metric) (scan.Diagnostic, score.Scorecard, error) {
	scanDir := rc.scanDir()
	// Merge the built-in artifact/cache excludes into the config exclusions
	// (additive — see scope.MergeExclusions) before projecting any view, so every
	// extractor inherits them. Keeps archfit from measuring its own tool outputs
	// (.gitnexus, reports, .archfit-cache) or vendored/dependency trees.
	cfg.Exclude = scope.MergeExclusions(cfg.Exclude)
	sc := cfg.ForScope()
	sc.WorkDir = scanDir
	// Wire the explicit --root argument so scope.Resolve uses it as the
	// analysis boundary (ScanRoot). When root is empty, cfg.Root="" falls
	// through resolveScanRoot to gitRoot → byte-identical to the pre-flag
	// behaviour. Without this line, --root only sets the gitResolver workDir
	// (which resolves UP to the git toplevel), leaving ScanRoot == gitRoot
	// even when --root points at a subtree of a monorepo.
	sc.Root = rc.ScanRoot
	sc.Base = mode.Base
	sc.Full = mode.Full
	deps.reportPhase("Discovering project")
	s, err := scope.Resolve(ctx, sc, gitResolver{workDir: scanDir, runner: deps.Runner})
	if err != nil {
		return scan.Diagnostic{}, score.Scorecard{}, err
	}

	// Extractor fact cache (fact-cache.md D1/D2): facts/ beside the AI cache
	// under the config bundle dir. Refresh mode bypasses reads but still records
	// fresh facts on success. The cache stores subprocess/loader FACTS keyed by
	// content; classification and scoring below never consult it.
	facts := factcache.NewStore(factsCacheDir(rc.BundleDir))
	facts.RefreshMode = deps.refresh

	extractors := buildExtractors(deps.Runner, cfg, facts)

	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		return scan.Diagnostic{}, score.Scorecard{}, err
	}
	ms := append(metrics.New(cfg.Metrics), extraMetrics...)

	// toolWarnings collects the exceptional, non-nil errors from the optional
	// extractors below. They normally degrade gracefully — encoding absence in
	// their Coverage record (surfaced as a CoverageGap), not an error — so this
	// only fires when a tool ran and failed unexpectedly. noteToolErr records the
	// message for the structured ConfigWarnings block and echoes it to stderr so
	// the failure is never silently discarded (the old `_` behaviour).
	var toolWarnings []string
	noteToolErr := func(tool string, err error) {
		if err == nil {
			return
		}
		msg := tool + ": " + err.Error()
		toolWarnings = append(toolWarnings, msg)
		deps.warn(msg)
	}

	// Output-path hygiene: when the config bundle/output directory sits strictly
	// inside the analyzed root, archfit writes cache/baseline/report artifacts into
	// the scanned tree — a source of non-deterministic repeat scans (OpenAI Sec 7.4).
	// Built-in excludes neutralise the known artifacts, but the directory itself
	// (and any user-redirected report) is still a hazard, so we surface it.
	if absBundleDir, aerr := filepath.Abs(rc.BundleDir); aerr == nil {
		if w := outputInsideRootWarning(s.Root, absBundleDir); w != "" {
			toolWarnings = append(toolWarnings, w)
			deps.warn(w)
		}
	}

	deps.reportPhase("Collecting facts")
	change := signal.RunSignals{}

	// LOC walk — repo-relative path→line-count map + coverage record.
	// ExtraCoverage order: loc, clones.
	var locCov diagnostic.Coverage
	var toolErr error
	fileCfg := cfg.ForFileClass()
	change.Size.FileLOC, change.Size.FileClassIndex, locCov, toolErr = loc.RunWithConfig(s.Root, fileCfg)
	noteToolErr("loc", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, locCov)

	// Dynamic/lazy import detection (deterministic FS scan; always runs): Python
	// non-top-level imports + importlib/__import__, TS require()/dynamic import().
	// Report-only — surfaced as evidence, never modifies the graph, metrics, or gate.
	change.DynamicImports.Sites = dynimports.Detect(s.Root)

	// Manifest deprecation markers (go.mod retract, package.json deprecated).
	// Report-only — never modifies the graph, metric verdict, or gate.
	// Ceiling: cargo yanked and live EOL are not locally declarable — use archfit analyze --ai-summary / config enrich.
	change.DeprecatedDeps = manifest.Scan(s.Root)

	// Runtime async-bridge detection (deterministic FS scan + optional ast-grep).
	// Detects message-queue, event-bus, async-task integration patterns per language.
	// Report-only evidence — never annotates graph edges, never affects distance/score or the gate verdict.
	{
		r := runtimedetect.Detect(ctx, s.Root, deps.Runner)
		for _, sig := range r.Signals {
			change.RuntimeAsync.Sites = append(change.RuntimeAsync.Sites, diagnostic.RuntimeAsyncSite{
				File:            sig.File,
				Line:            sig.Line,
				Library:         sig.Library,
				IntegrationKind: string(sig.IntegrationKind),
				Language:        sig.Language,
			})
		}
		change.RuntimeAsync.Confidence = r.Confidence
	}

	// Ownership resolution: fills module owner gaps from CODEOWNERS or git-author
	// history. Explicit config owner always wins; resolver only fills empty slots.
	// If every path-owning module already declares owner, skip the extra repo scan.
	needsOwnerResolution := false
	for _, def := range cfg.Modules {
		if len(def.Paths) > 0 && def.Owner == "" {
			needsOwnerResolution = true
			break
		}
	}
	// ownerSource records where owners came from for the diagnostic's owner_source
	// (distance-confidence signal). "config" = every path-owning module declared an
	// owner, so no resolution was needed.
	ownerSource := "config"
	if needsOwnerResolution {
		resolved, src := ownership.Resolve(ctx, s.Root, s.GitRoot, s.SubtreePrefix, cfg.ModuleMapView(), deps.Runner)
		cfg.FillMissingOwners(resolved)
		ownerSource = string(src)
		if w := ownerDegradationWarning(src); w != "" {
			toolWarnings = append(toolWarnings, w)
			deps.warn(w)
		}
	}

	// Deploy-unit detection: fills module deploy_unit gaps from static repo
	// analysis (Go main pkgs, Dockerfiles, k8s manifests, package.json workspaces,
	// pyproject.toml). Detect keys results by repo-relative path; KeyByModule
	// remaps those to module names, which is what FillMissingDeployUnits expects
	// (without it, auto-detected units are dropped unless a module's map key
	// equals the path). Config-authored deploy_unit always wins.
	duModules := cfg.ModuleMapView()
	deployUnitsByPath := deployunit.Detect(ctx, s.Root, duModules, deps.Runner)
	deployUnitsByModule := deployunit.KeyByModule(deployUnitsByPath, duModules)
	cfg.FillMissingDeployUnits(deployUnitsByModule)
	change.ExtraCoverage = append(change.ExtraCoverage, diagnostic.Coverage{
		Tool:      toolDeployUnit,
		FilesSeen: len(deployUnitsByPath),
		// Deploy-unit detection is auxiliary distance evidence, not file-scope
		// extractor coverage. Keep the row visible but non-contributing.
		FilesApplicable: 0,
		Status:          diagnostic.StatusOK,
	})

	// Clone detection — opt-in (analyzers.clones.enabled: true). Run returns empty+absent
	// when disabled or the tool is missing; the metric reports n/a in that case.
	// Append coarse test/generated globs to the exclusions so jscpd skips those
	// files at scan time (speed); correctness is enforced by the clone post-filter
	// in the classify stage. These globs are additive to cfg.Exclude.
	clonesExcl := make([]string, len(cfg.Exclude), len(cfg.Exclude)+len(cloneTestGenGlobs))
	copy(clonesExcl, cfg.Exclude)
	clonesExcl = append(clonesExcl, cloneTestGenGlobs...)
	var clonesCov diagnostic.Coverage
	change.Duplication.Clusters, clonesCov, toolErr = clones.Run(ctx, deps.Runner, s.Root, cfg.ClonesEnabled(), cfg.ToolTimeout(config.ToolClones), clonesExcl, facts)
	noteToolErr("jscpd", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, clonesCov)

	// Pinned coupling labels (.archfit-labels.yaml): the human-reviewed output of
	// `archfit enrich`. Optional; a malformed file fails loudly — a half-read
	// labels file must never silently alter the gate.
	lbls, err := labelsio.Load(filepath.Join(rc.BundleDir, defaultLabelsPath))
	if err != nil {
		return scan.Diagnostic{}, score.Scorecard{}, err
	}

	// SCIP symbol-level strength is opt-in (analyzers.scip.enabled: true): the indexer is
	// whole-repo and slow, so it must not run on the default check path, and the
	// decision must live in config (not PATH presence) to keep metrics deterministic.
	// When skipped, append an explicit disabled coverage row so tool_coverage reads
	// "disabled" (not absent/missing) and no spurious install-gap is raised.
	var resolver ports.SymbolResolver = ports.NopSymbolResolver{}
	if cfg.ScipEnabled() {
		scipAdapter := scip.New(deps.Runner, cfg.ToolTimeout(config.ToolScip))
		scipAdapter.Cache = facts
		// scip-go is a Go-toolchain process, so it must honour the same go.work
		// decision the in-process Go loader makes — otherwise it walks up to an
		// out-of-scope go.work, indexes one synthetic package, and writes an empty
		// index. Asking discovery is a handful of stats; deriving it separately
		// here would be a second implementation of the same decision.
		scipAdapter.GoWorkOff = goWorkOff(s.Root, cfg)
		resolver = scipAdapter
	} else {
		change.ExtraCoverage = append(change.ExtraCoverage, diagnostic.Coverage{
			Tool:   toolScip,
			Status: diagnostic.StatusDisabled,
			Reason: reasonScipDisabled,
		})
	}

	// Syntax facts (ast-grep syntax rules) are opt-in (analyzers.syntax.enabled: true):
	// language-specific rules add overhead and the result is report-only.
	// When skipped, append an explicit disabled coverage row for the same reason.
	syntaxCfg := cfg.ForSyntax()
	var syntaxProvider ports.SyntaxProvider = ports.NopSyntaxProvider{}
	if syntaxCfg.Enabled {
		syntaxAdapter := astgrep.New(deps.Runner)
		syntaxAdapter.Cache = facts
		syntaxProvider = syntaxAdapter
	} else {
		change.ExtraCoverage = append(change.ExtraCoverage, diagnostic.Coverage{
			Tool:   toolAstGrepSyntax,
			Status: diagnostic.StatusDisabled,
			Reason: reasonSyntaxDisabled,
		})
	}

	configHash := effectiveConfigHash(rc.ConfigSource)
	// One evaluation instant for waiver expiry and staleness. A --base run
	// hands the same value to both sides so head and base age findings against
	// the same moment rather than two samples taken minutes apart.
	now := rc.evaluatedAt()

	patternCfg := cfg.ForPatterns()
	patternProvider := astgrep.New(deps.Runner)
	patternProvider.Cache = facts
	deps.reportPhase("Analyzing dependencies")
	diag, err := engine.Run(ctx, engine.RunInput{
		Mode:        mode,
		Scope:       s,
		Classify:    cfg.ForClassify(),
		Staleness:   cfg.ForStaleness(),
		Waivers:     cfg.ForWaivers(),
		Extractors:  extractors,
		Patterns:    patternProvider,
		Resolver:    resolver,
		Syntax:      syntaxProvider,
		SyntaxCfg:   syntaxCfg,
		PatternCfg:  patternCfg,
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		// Per-metric gate posture/thresholds: cfg.Metrics is already the
		// name→entry view the verdict wants; missing names read as zero
		// entries (the gate defaults) on lookup.
		MetricGates: cfg.Metrics,
		Labels:      lbls,
		Signals:     change,
		Now:         now,
		ConfigHash:  configHash,
		// Primary dependency-graph analyzer coverage names, in registry order, so
		// score synthesis names them without the core ring hardcoding tool strings.
		PrimaryExtractorTools: primaryExtractorTools(),
	})
	if err != nil {
		return diag, score.Scorecard{}, err
	}
	diag.OwnerSource = ownerSource
	diag.DistanceContext = buildDistanceContext(diag, cfg, len(deployUnitsByModule))
	diag.VolatilityCorroboration = buildVolatilityCorroboration(ctx, s.GitRoot, s.SubtreePrefix, cfg, deps.Runner)

	// TS/Python coverage honesty: unresolved imports must never be a silent
	// gap — surface them on stderr the same way ownerDegradationWarning
	// discloses degraded owner resolution.
	if w := tsUnresolvedWarning(diag.ToolCoverage); w != "" {
		toolWarnings = append(toolWarnings, w)
		deps.warn(w)
	}
	if w := pyUnresolvedWarning(diag.ToolCoverage); w != "" {
		toolWarnings = append(toolWarnings, w)
		deps.warn(w)
	}

	// cargo-modules module-graph coverage: opt-in (analyzers.cargo_modules.enabled: true).
	// The Rust extractor runs cargo-modules during its Extract call (inside engine.Run
	// above) and caches the coverage record. Append it here — BEFORE the scorecard
	// synthesis, which reads ToolCoverage for the partial-module-graph confidence cap —
	// so it appears in ToolCoverage and the CoverageGap block (mirrors the clones pattern).
	if rustEx := rustExtractor(extractors); rustEx != nil {
		diag.ToolCoverage = append(diag.ToolCoverage, rustEx.LastModuleGraphCoverage())
	}

	// Synthesise the scorecard inside the pipeline (not after it, as before) so
	// the coupling gate below can escalate the verdict and promote findings
	// while agenttask.Build and every renderer still see the result.
	deps.reportPhase("Scoring architecture")
	card := score.Synthesize(diag)
	applyCouplingGate(&diag, card, couplingGateView(cfg), base)

	// Attach the structured repair-task block (spec §13) for active gate
	// findings. Deterministic: rule-type templates + module public surfaces +
	// the exact command that re-verifies the gate. Runs after the coupling gate
	// so promoted Balanced-Coupling findings produce repair tasks too.
	ruleTypes := make(map[string]string, len(cfg.Rules))
	for _, def := range cfg.Rules {
		ruleTypes[def.ID] = def.Type
	}
	modulePublic := make(map[string][]string, len(cfg.Modules))
	for name, def := range cfg.Modules {
		if len(def.Public) > 0 {
			modulePublic[name] = def.Public
		}
	}
	validate := validationCommand(rc.ConfigSource, rc.ScanRoot)
	// PathResolver lets agenttask.Build turn a bare config module key, a Rust
	// "crate::mod" id, or a Python dotted module id into a path that actually
	// exists on disk, using facts already gathered above — agenttask itself
	// never touches the filesystem. A nil FileClassIndex (LOC walk did not
	// run) disables resolution, matching the pre-resolver behavior.
	var knownFiles map[string]struct{}
	if change.Size.FileClassIndex != nil {
		knownFiles = make(map[string]struct{}, len(change.Size.FileClassIndex))
		for f := range change.Size.FileClassIndex {
			knownFiles[f] = struct{}{}
		}
	}
	crateRootDirs := map[string]string{}
	if rustEx := rustExtractor(extractors); rustEx != nil {
		for _, cr := range rustEx.LastCrateRoots() {
			crateRootDirs[cr.Name] = cr.Dir
		}
	}
	// onDisk backstops the LOC-walk index: its walk skips dirs (mocks/,
	// target/, venv/) that extractor exclusions do not, so a real edge
	// endpoint there is index-invisible yet must survive resolution.
	pathResolver := agenttask.NewPathResolver(knownFiles, crateRootDirs, module.RootDirs(cfg.Modules), onDiskWithin(s.Root))
	diag.AgentTasks = agenttask.Build(diag.Findings, ruleTypes, modulePublic, []string{validate}, diag.SyntaxFacts, pathResolver)
	diag.AdvisoryTasks = engine.BuildAdvisoryTasks(diag.Findings, []string{validate})

	// Warn-loud coverage reporting: turn the absent tool-coverage records into a
	// machine-readable CoverageGaps block (tool → unlocked metrics → install cmd)
	// and surface config-quality lint plus any swallowed optional-tool errors in
	// ConfigWarnings so they reach md/json/CI instead of being stderr-only.
	// Disclose switched-off primaries as "disabled" BEFORE deriving gaps: an
	// extractor reports ModeOff as "absent", which both comparison paths read as
	// "this language is not in the tree" (see markDisabledPrimaries). s.Root is
	// the presence probe's boundary — a language that is not here stays absent.
	diag.ToolCoverage = markDisabledPrimaries(diag.ToolCoverage, cfg, s.Root)
	diag.CoverageGaps = buildCoverageGaps(diag.ToolCoverage, cfg, s.Root)
	diag.ConfigWarnings = buildConfigWarnings(cfg, toolWarnings)

	// Decision tasks: undeclared judgment inputs that prevent the scorer from
	// placing edges on the book's scale. Appended to ConfigWarnings so they reach
	// the JSON/md output and are actionable for humans and AI agents.
	diag.ConfigWarnings = append(diag.ConfigWarnings,
		buildJudgmentDecisionTasks(cfg, lbls, rc.ConfigSource)...)

	return diag, card, nil
}

// onDiskWithin returns the PathResolver's onDisk callback: it reports whether
// the repo-relative slash path rel exists under root. Candidates that are not
// local after OS-path conversion (Windows `..\x` or `C:/x`, any escaping or
// absolute path) are rejected before touching the filesystem — the resolver's
// slash-only guard cannot see OS-specific separators, so files[] containment
// is enforced here, after filepath.FromSlash.
func onDiskWithin(root string) func(string) bool {
	return func(rel string) bool {
		osRel := filepath.FromSlash(rel)
		if !filepath.IsLocal(osRel) {
			return false
		}
		_, err := os.Stat(filepath.Join(root, osRel))
		return err == nil
	}
}

func validationCommand(configPath, root string) string {
	args := []string{"archfit", "check", "-c", configPath}
	if root != "" {
		args = append(args, "--root", root)
	}
	for i := range args {
		args[i] = shellQuoteArg(args[i])
	}
	return strings.Join(args, " ")
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$`!#&;()*<>?[\\]^{|}~") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

// ruleIDBCCouplingGate is stamped on the synthetic summary finding that
// applyCouplingGate emits when the gate trips with no promotable BC advisory
// (advisory output off, or coupling.min_severity above every active edge).
// Per-run trip state, never a triageable edge: `archfit baseline` skips it.
const ruleIDBCCouplingGate = "bc/coupling_gate"

// findingIDCouplingGate is the fixed finding ID of that synthetic summary
// finding. It is per-run trip state, not a content fingerprint, so the git
// origin delta always classifies its task as unknown.
const findingIDCouplingGate = "coupling-gate"

// couplingGateView projects the coupling.gate config block into the score
// package's gate view. It lives here in the composition root — config sits
// below score in the layer order, so unlike the other config views this
// projection cannot be a Config method. A nil block is Enabled=false:
// coupling stays advisory-only, byte-identical to pre-gate behavior.
func couplingGateView(cfg config.Config) score.CouplingGate {
	g := cfg.Coupling.Gate
	if g == nil {
		return score.CouplingGate{}
	}
	return score.CouplingGate{
		Enabled: true,
		MinBand: score.Band(g.MinBand),
		MaxDrop: g.MaxDrop,
	}
}

// applyCouplingGate escalates the verdict to fail when the synthesised
// coupling_balance score trips the configured coupling.gate (V2 fix: the
// flagship metric can now block CI). The active Balanced-Coupling advisories —
// the findings whose edges the tripped score aggregates — are promoted to gate
// kind so they flow into agent_tasks through the existing kind filter
// (internal/agenttask) and into the MustFix bucket; baselined/waived edges stay
// triaged and are not promoted. An unmeasured score (band n/a) never trips —
// score.EvaluateCouplingGate owns that rule. No coupling.gate config ⇒ no-op.
// Trip reasons are NOT printed here: runPipeline is shared with baseline,
// enrich, explain, and --base scoring, where an enforcement-sounding stderr
// line is noise — analyze.go re-evaluates the pure gate and echoes them.
//
// The score is synthesised from ClassifiedEdges (before advisory filtering),
// so the gate can trip while no BC advisory exists to promote — advisory
// output off, or coupling.min_severity above every active edge. A fail
// verdict must still carry evidence in the diagnostic itself, so that case
// emits one synthetic summary gate finding (rule bc/coupling_gate) with the
// trip reasons as Why: summary.gate_findings and agent_tasks[] stay
// consistent with the verdict. `archfit baseline` never persists it — the
// trip is per-run state, and a stored fingerprint the engine never
// regenerates would surface as a phantom "fixed" finding.
func applyCouplingGate(diag *scan.Diagnostic, card score.Scorecard, gate score.CouplingGate, base baseline.Baseline) {
	trip := score.EvaluateCouplingGate(card, gate, base.CouplingScore())
	if !trip.Tripped {
		return
	}
	diag.Verdict = diagnostic.VerdictFail
	promoted := 0
	for i := range diag.Findings {
		f := &diag.Findings[i]
		if f.RuleID != engine.RuleIDBCImbalanced || f.Kind != finding.KindAdvisory || !score.IsActiveGateFinding(*f) {
			continue
		}
		f.Kind = finding.KindGate
		promoted++
	}
	// Keep the summary counters consistent with the re-kinded findings: the
	// promoted advisories now count as blocking, not as warnings.
	diag.Summary.GateFindings += promoted
	diag.Summary.Warnings = max(0, diag.Summary.Warnings-promoted)
	if promoted == 0 {
		diag.Findings = append(diag.Findings, finding.Finding{
			ID:       findingIDCouplingGate,
			Kind:     finding.KindGate,
			RuleID:   ruleIDBCCouplingGate,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Why:      strings.Join(trip.Reasons, "; "),
		})
		diag.Summary.GateFindings++
	}
}
