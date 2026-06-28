package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/internal/agenttask"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/extract/astgrep"
	"github.com/alexei-led/archfit/internal/extract/clones"
	"github.com/alexei-led/archfit/internal/extract/complexity"
	"github.com/alexei-led/archfit/internal/extract/deployunit"
	"github.com/alexei-led/archfit/internal/extract/dynimports"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/manifest"
	runtimedetect "github.com/alexei-led/archfit/internal/extract/runtime"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/fitness"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// cloneTestGenGlobs are coarse jscpd --ignore patterns that skip test and
// generated files at scan time. These are additive speed hints; the post-filter
// in functional_candidates.go is the authoritative correctness gate.
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

// runPipeline resolves scope, builds extractors/rules/metrics, collects change
// history and optional tool inputs, and executes the engine. check, scan,
// explain, and baseline all run through this single path: baseline snapshots
// must be computed from exactly the same inputs as check verdicts, or every
// post-baseline check reports phantom metric regressions and unmatched finding
// fingerprints. After the engine returns, the agent_tasks repair block is
// attached from the active gate findings (deterministic; spec §13).
func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, configPath, root string, noConfig bool, mode engine.Mode, base baseline.Baseline, extraMetrics ...metrics.Metric) (diagnostic.Diagnostic, error) {
	configDir := filepath.Dir(configPath)
	// scanDir anchors scope/git resolution. An explicit --root decouples the
	// analyzed repo from where the config lives (external-CI use case); when it is
	// empty the config directory is the root, identical to the historical
	// behaviour. Baseline, labels, and the config hash stay config-adjacent
	// regardless — they are part of the config bundle, not the scanned tree.
	scanDir := root
	if scanDir == "" {
		scanDir = configDir
	}
	// Merge the built-in artifact/cache excludes into the config exclusions
	// (additive — see scope.MergeExclusions) before projecting any view, so every
	// extractor inherits them. Keeps archfit from measuring its own tool outputs
	// (.gitnexus, reports, .archfit-cache) or vendored/dependency trees.
	cfg.Exclusions = scope.MergeExclusions(cfg.Exclusions)
	sc := cfg.ForScope()
	sc.WorkDir = scanDir
	// Wire the explicit --root argument so scope.Resolve uses it as the
	// analysis boundary (ScanRoot). When root is empty, cfg.Root="" falls
	// through resolveScanRoot to gitRoot → byte-identical to the pre-flag
	// behaviour. Without this line, --root only sets the gitResolver workDir
	// (which resolves UP to the git toplevel), leaving ScanRoot == gitRoot
	// even when --root points at a subtree of a monorepo.
	sc.Root = root
	sc.Base = mode.Base
	sc.Full = mode.Full
	s, err := scope.Resolve(ctx, sc, gitResolver{workDir: scanDir, runner: deps.Runner})
	if err != nil {
		return diagnostic.Diagnostic{}, err
	}

	extractors := buildExtractors(deps.Runner, cfg)

	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		return diagnostic.Diagnostic{}, err
	}
	// risk_hub reads hand-authored volatility only (never git churn).
	ms := append(metrics.New(cfg), extraMetrics...)

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
		_, _ = fmt.Fprintln(os.Stderr, "warning: "+msg)
	}

	// Output-path hygiene: when the config/output directory sits strictly inside
	// the analyzed root, archfit writes cache/baseline/report artifacts into the
	// scanned tree — a source of non-deterministic repeat scans (OpenAI Sec 7.4).
	// Built-in excludes neutralise the known artifacts, but the directory itself
	// (and any user-redirected report) is still a hazard, so we surface it.
	if absConfigDir, aerr := filepath.Abs(configDir); aerr == nil {
		if w := outputInsideRootWarning(s.Root, absConfigDir); w != "" {
			toolWarnings = append(toolWarnings, w)
			_, _ = fmt.Fprintln(os.Stderr, "warning: "+w)
		}
	}

	// Recent git history (cheap; runs by default): per-file churn drives module
	// volatility (unbalanced_edge, BC severity) and the modularity metrics
	// (change_amplification, hidden_coupling). Hand-authored volatility/subdomain
	// config always wins; a non-git repo leaves these signals empty.
	// Run history at the git toplevel (GitRoot) scoped to the analysis subtree
	// (SubtreePrefix), so returned paths are ScanRoot-relative. Falls back to
	// s.Root when GitRoot is empty (non-git run: History returns absent).
	change := signal.RunSignals{}
	histWorkDir := s.GitRoot
	if histWorkDir == "" {
		histWorkDir = s.Root
	}
	if churn, coChange, commitCount, _, herr := git.History(ctx, histWorkDir, s.SubtreePrefix, deps.Runner); herr == nil {
		change.History.FileChurn, change.History.CoChange = churn, coChange
		change.History.CommitCount = commitCount
	}

	// LOC walk — repo-relative path→line-count map + coverage record.
	// ExtraCoverage order: loc, complexity, clones.
	var locCov diagnostic.Coverage
	var toolErr error
	change.Size.FileLOC, change.Size.FileClassIndex, locCov, toolErr = loc.Run(s.Root)
	noteToolErr("loc", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, locCov)

	// Architecture-fitness enforcement signals (deterministic FS scan; always runs).
	change.Fitness = fitness.Detect(s.Root)

	// Dynamic/lazy import detection (deterministic FS scan; always runs): Python
	// non-top-level imports + importlib/__import__, TS require()/dynamic import().
	// Report-only — surfaced as evidence, never modifies the graph, metrics, or gate.
	change.DynamicImports.Sites = dynimports.Detect(s.Root)

	// Manifest deprecation markers (go.mod retract, package.json deprecated).
	// Report-only — never modifies the graph, metric verdict, or gate.
	// Ceiling: cargo yanked and live EOL are not locally declarable — use archfit review/enrich.
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
	if needsOwnerResolution {
		cfg.FillMissingOwners(ownership.Resolve(ctx, s.Root, cfg.ModuleMapView(), deps.Runner))
	}

	// Deploy-unit detection: fills module deploy_unit gaps from static repo
	// analysis (Go main pkgs, Dockerfiles, k8s manifests, package.json workspaces,
	// pyproject.toml). Detect keys results by repo-relative path; KeyByModule
	// remaps those to module names, which is what FillMissingDeployUnits expects
	// (without it, auto-detected units are dropped unless a module's map key
	// equals the path). Config-authored deploy_unit always wins.
	duModules := cfg.ModuleMapView()
	cfg.FillMissingDeployUnits(deployunit.KeyByModule(deployunit.Detect(ctx, s.Root, duModules, deps.Runner), duModules))

	// Cyclomatic complexity — opt-in (tools.complexity.enabled: on).
	// Backend: auto (default) = gocyclo(Go) + ast-grep proxy(TS/Py/Rust); lizard =
	// exact multi-language CCN (re-pins Python). Coverage carries zero file counts.
	// Config excludes + scope defaults are forwarded to lizard's -x flags so it
	// skips the same paths that all other extractors skip.
	complexityExcl := scope.MergeExclusions(cfg.Exclusions)
	var complexityCov diagnostic.Coverage
	change.Complexity.Funcs, complexityCov, toolErr = complexity.Run(ctx, deps.Runner, s.Root, cfg.ComplexityEnabled(), cfg.ComplexityBackend(), cfg.ToolTimeout(config.ToolComplexity), complexityExcl)
	noteToolErr("complexity", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, complexityCov)

	// Clone detection — opt-in (tools.clones.enabled: on). Run returns empty+absent
	// when disabled or the tool is missing; the metric reports n/a in that case.
	// Append coarse test/generated globs to the exclusions so jscpd skips those
	// files at scan time (speed). The post-filter in functional_candidates.go is
	// the source-of-truth for correctness; these globs are additive.
	clonesExcl := make([]string, len(cfg.Exclusions), len(cfg.Exclusions)+len(cloneTestGenGlobs))
	copy(clonesExcl, cfg.Exclusions)
	clonesExcl = append(clonesExcl, cloneTestGenGlobs...)
	var clonesCov diagnostic.Coverage
	change.Duplication.Clusters, clonesCov, toolErr = clones.Run(ctx, deps.Runner, s.Root, cfg.ClonesEnabled(), cfg.ToolTimeout(config.ToolClones), clonesExcl)
	noteToolErr("jscpd", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, clonesCov)

	// Pinned coupling labels (.archfit-labels.yaml): the human-reviewed output of
	// `archfit enrich`. Optional; a malformed file fails loudly — a half-read
	// labels file must never silently alter the gate.
	lbls, err := labelsio.Load(filepath.Join(configDir, defaultLabelsPath))
	if err != nil {
		return diagnostic.Diagnostic{}, err
	}

	// SCIP symbol-level strength is opt-in (tools.scip.enabled: on): the indexer is
	// whole-repo and slow, so it must not run on the default check path, and the
	// decision must live in config (not PATH presence) to keep metrics deterministic.
	// When skipped, append an explicit disabled coverage row so tool_coverage reads
	// "disabled" (not absent/missing) and no spurious install-gap is raised.
	var resolver ports.SymbolResolver = ports.NopSymbolResolver{}
	if cfg.ScipEnabled() {
		resolver = scip.New(deps.Runner, cfg.ToolTimeout(config.ToolScip))
	} else {
		change.ExtraCoverage = append(change.ExtraCoverage, diagnostic.Coverage{
			Tool:   toolScip,
			Status: diagnostic.StatusDisabled,
			Reason: reasonScipDisabled,
		})
	}

	// Syntax facts (ast-grep syntax rules) are opt-in (tools.syntax.enabled: on):
	// language-specific rules add overhead and the result is report-only.
	// When skipped, append an explicit disabled coverage row for the same reason.
	syntaxCfg := cfg.ForSyntax()
	var syntaxProvider ports.SyntaxProvider = ports.NopSyntaxProvider{}
	if syntaxCfg.Enabled {
		syntaxProvider = astgrep.New(deps.Runner)
	} else {
		change.ExtraCoverage = append(change.ExtraCoverage, diagnostic.Coverage{
			Tool:   toolAstGrepSyntax,
			Status: diagnostic.StatusDisabled,
			Reason: reasonSyntaxDisabled,
		})
	}

	// Config hash for reproducibility — empty when --no-config ignored the file.
	configHash := effectiveConfigHash(configPath, noConfig)

	patternCfg := cfg.ForPatterns()
	diag, err := engine.Run(ctx, engine.RunInput{
		Mode:        mode,
		Scope:       s,
		Classify:    cfg.ForClassify(),
		Staleness:   cfg.ForStaleness(),
		Exceptions:  cfg.ForStatus(),
		Extractors:  extractors,
		Patterns:    astgrep.New(deps.Runner),
		Resolver:    resolver,
		Syntax:      syntaxProvider,
		SyntaxCfg:   syntaxCfg,
		PatternCfg:  patternCfg,
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      lbls,
		Signals:     change,
		Now:         time.Now(),
		ConfigHash:  configHash,
		// Primary dependency-graph analyzer coverage names, in registry order, so
		// score synthesis names them without the core ring hardcoding tool strings.
		PrimaryExtractorTools: primaryExtractorTools(),
	})
	if err != nil {
		return diag, err
	}

	// Attach the structured repair-task block (spec §13) for active gate
	// findings. Deterministic: rule-type templates + module public surfaces +
	// the exact command that re-verifies the gate.
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
	validate := "archfit check -c " + configPath
	if mode.Full {
		validate += " --full"
	}
	diag.AgentTasks = agenttask.Build(diag.Findings, ruleTypes, modulePublic, []string{validate}, diag.SyntaxFacts)

	// cargo-modules module-graph coverage: opt-in (tools.cargo-modules.enabled: on).
	// The Rust extractor runs cargo-modules during its Extract call (inside engine.Run
	// above) and caches the coverage record. Append it here so it appears in
	// ToolCoverage and the CoverageGap block — mirrors the complexity/clones pattern.
	if rustEx := rustExtractor(extractors); rustEx != nil {
		diag.ToolCoverage = append(diag.ToolCoverage, rustEx.LastModuleGraphCoverage())
	}

	// Warn-loud coverage reporting: turn the absent tool-coverage records into a
	// machine-readable CoverageGaps block (tool → unlocked metrics → install cmd)
	// and surface config-quality lint plus any swallowed optional-tool errors in
	// ConfigWarnings so they reach md/json/CI instead of being stderr-only.
	diag.CoverageGaps = buildCoverageGaps(diag.ToolCoverage, cfg, s.Root)
	diag.ConfigWarnings = buildConfigWarnings(cfg, toolWarnings)

	// Decision tasks: undeclared judgment inputs that prevent the scorer from
	// placing edges on the book's scale. Appended to ConfigWarnings so they reach
	// the JSON/md output and are actionable for humans and AI agents.
	diag.ConfigWarnings = append(diag.ConfigWarnings,
		buildJudgmentDecisionTasks(cfg, lbls, configPath)...)

	return diag, nil
}
