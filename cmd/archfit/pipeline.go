package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	"github.com/alexei-led/archfit/internal/labels"
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
	if churn, coChange, _, herr := git.History(ctx, histWorkDir, s.SubtreePrefix, deps.Runner); herr == nil {
		change.History.FileChurn, change.History.CoChange = churn, coChange
	}

	// LOC walk — repo-relative path→line-count map + coverage record.
	// ExtraCoverage order: loc, complexity, clones.
	var locCov diagnostic.Coverage
	var toolErr error
	change.Size.FileLOC, locCov, toolErr = loc.Run(s.Root)
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
	var complexityCov diagnostic.Coverage
	change.Complexity.Funcs, complexityCov, toolErr = complexity.Run(ctx, deps.Runner, s.Root, cfg.ComplexityEnabled(), cfg.ComplexityBackend())
	noteToolErr("complexity", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, complexityCov)

	// Clone detection — opt-in (tools.clones.enabled: on). Run returns empty+absent
	// when disabled or the tool is missing; the metric reports n/a in that case.
	var clonesCov diagnostic.Coverage
	change.Duplication.Clusters, clonesCov, toolErr = clones.Run(ctx, deps.Runner, s.Root, cfg.ClonesEnabled())
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
	var resolver ports.SymbolResolver = ports.NopSymbolResolver{}
	if cfg.ScipEnabled() {
		resolver = scip.New(deps.Runner)
	}

	// Syntax facts (ast-grep syntax rules) are opt-in (tools.syntax.enabled: on):
	// language-specific rules add overhead and the result is report-only.
	syntaxCfg := cfg.ForSyntax()
	var syntaxProvider ports.SyntaxProvider = ports.NopSyntaxProvider{}
	if syntaxCfg.Enabled {
		syntaxProvider = astgrep.New(deps.Runner)
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

// gateWarn / gateFail are the coverage-gap gate strings stamped on each gap.
// warn (default) degrades a missing tool's metrics to n/a (never green) and reports
// it, but does not fail the build; fail is the opt-in hard gate (tools.<x>.gate: fail
// / --require-tools). Sourced from config.GateMode so the two never drift.
const (
	gateWarn = string(config.GateWarn)
	gateFail = string(config.GateFail)
)

// coverageToolConfigKey maps a coverage tool name (as it appears in ToolCoverage,
// e.g. "go/packages") to the config Tools map key whose gate: governs it (e.g.
// "go"). Lets a user write tools.go.gate: fail to gate on the go/packages analyzer
// without knowing the internal coverage name. Tools absent here fall back to warn.
// The per-language primary analyzers come from the language registry; the
// cross-language optional tools stay literal. Built once at init.
var coverageToolConfigKey = buildCoverageToolConfigKey()

func buildCoverageToolConfigKey() map[string]string {
	m := map[string]string{
		toolLizard:       config.ToolComplexity,
		toolJscpd:        config.ToolClones,
		toolCargoModules: config.ToolCargoModules,
	}
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = lang.ID
	}
	return m
}

// primaryGraphMetrics are the metrics the dependency-graph extractors
// (go/packages, dependency-cruiser, grimp) unlock; absent any of them, all of
// these drop to n/a. Shared (read-only) across those per-language table entries.
var primaryGraphMetrics = []string{"coverage", "coupling_balance", "encapsulation", "cycle", "blast_radius"}

// affectedMetrics carries an absent analyzer's one-line install hint and the
// metrics its absence leaves unmeasured.
type affectedMetrics struct {
	install string
	metrics []string
}

// toolAffectedMetrics maps an absent analyzer's coverage name to its one-line
// install hint and the metrics its absence leaves unmeasured. Only tools listed
// here produce a CoverageGap — an absent coverage entry with no actionable
// install path is not a gap a user can close. Per-language analyzers come from
// the registry; cross-language optional tools stay literal. Built once at init.
var toolAffectedMetrics = buildToolAffectedMetrics()

func buildToolAffectedMetrics() map[string]affectedMetrics {
	m := map[string]affectedMetrics{
		toolLizard:       {"uv tool install lizard / pip install lizard", []string{"complexity"}},
		toolJscpd:        {"npm install -g jscpd", []string{"functional_candidates"}},
		toolCargoModules: {"cargo install cargo-modules (tools.cargo-modules.enabled: on)", []string{"cycle", "blast_radius", "cohesion", "encapsulation"}},
	}
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = affectedMetrics{lang.InstallHint, primaryGraphMetrics}
	}
	return m
}

// primaryToolLanguage maps a language's primary-tool coverage name back to its
// config language key, so a coverage gap for a disabled language can be suppressed
// (a Rust-only repo should not be told to install go/ts/py analyzers). Built once.
var primaryToolLanguage = buildPrimaryToolLanguage()

func buildPrimaryToolLanguage() map[string]string {
	m := make(map[string]string, len(languageRegistry))
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = lang.ID
	}
	return m
}

// primaryToolProjectMarkers maps a language's primary-tool coverage name to the
// project-marker filenames that signal the language is present in a repo root
// (e.g. "go/packages" → ["go.mod"], "cargo" → ["Cargo.toml"]). Used by
// buildCoverageGaps to suppress gaps for languages whose project is absent from
// the scan root. Built once at init.
var primaryToolProjectMarkers = buildPrimaryToolProjectMarkers()

func buildPrimaryToolProjectMarkers() map[string][]string {
	m := make(map[string][]string, len(languageRegistry))
	for _, lang := range languageRegistry {
		m[lang.PrimaryTool] = lang.ProjectMarkers
	}
	return m
}

// projectMarkerPresent reports whether any of the given project-marker filenames
// exist in root. Checks only the root dir (not recursive) — markers like go.mod
// and Cargo.toml are always at the repo root. Returns true when markers is empty
// (no marker = cannot determine absence, so don't suppress).
func projectMarkerPresent(root string, markers []string) bool {
	if len(markers) == 0 {
		return true
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(root, m)); err == nil {
			return true
		}
	}
	return false
}

// buildCoverageGaps derives the CoverageGaps block from the absent tool-coverage
// records. Each gap's Gate is the configured posture for that tool (tools.<x>.gate,
// default warn) — the --require-tools override is applied later by applyToolGate so
// the non-check callers of runPipeline are unaffected. Gaps are sorted by tool name
// so a double-run stays byte-identical regardless of upstream coverage order.
// Returns nil when no known tool is absent (omitempty keeps clean output unchanged).
//
// StatusDisabled entries (tools present but turned off in config) are intentionally
// excluded — the user does not need an "install" prompt for a deliberate opt-out.
// The tool_coverage block already carries the reason for any reader who wants it.
//
// Gaps are also suppressed when the language's project marker is absent from root
// (e.g. no Cargo.toml → Rust is not present → cargo gap is noise). An explicit
// gate on that tool overrides the suppression — it is an intentional "require it"
// even in repos that don't currently use that language.
func buildCoverageGaps(cov []diagnostic.Coverage, cfg config.Config, root string) []diagnostic.CoverageGap {
	var gaps []diagnostic.CoverageGap
	for _, c := range cov {
		// Only truly absent tools produce a gap. Disabled-by-config tools are an
		// intentional opt-out; partial coverage is informational (not actionable).
		if c.Status != diagnostic.StatusAbsent {
			continue
		}
		// A disabled language's primary tool is not a gap the user needs to close —
		// don't tell a Rust-only repo to install dependency-cruiser/grimp/go-packages.
		// An explicit gate on that tool (tools.<lang>.gate) is an intentional
		// "require it anyway" override and is preserved.
		if lang, isPrimary := primaryToolLanguage[c.Tool]; isPrimary &&
			cfg.Tools[lang].Enabled == config.ModeOff && cfg.Tools[lang].Gate == "" {
			continue
		}
		// Suppress the gap when the language's project marker is absent from the
		// scan root — the language simply isn't present in this repo, so the missing
		// tool is not actionable. An explicit gate overrides this (same carve-out as
		// the disabled-language check above). cargo-modules (opt-in intra-crate tool,
		// not a language primary) is also suppressed when no Cargo.toml is present.
		if root != "" {
			switch c.Tool {
			case toolCargoModules:
				// cargo-modules is Rust-specific but not a primary tool; use Cargo.toml.
				if configToolGate(cfg, c.Tool) == gateWarn && !projectMarkerPresent(root, []string{markerCargoToml}) {
					continue
				}
			default:
				if markers, ok := primaryToolProjectMarkers[c.Tool]; ok {
					lang := primaryToolLanguage[c.Tool]
					if cfg.Tools[lang].Gate == "" && !projectMarkerPresent(root, markers) {
						continue
					}
				}
			}
		}
		info, ok := toolAffectedMetrics[c.Tool]
		if !ok {
			continue
		}
		gaps = append(gaps, diagnostic.CoverageGap{
			Tool:            c.Tool,
			InstallCmd:      info.install,
			AffectedMetrics: info.metrics,
			Gate:            configToolGate(cfg, c.Tool),
		})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Tool < gaps[j].Tool })
	return gaps
}

// configToolGate resolves the configured gate for the analyzer behind a coverage
// tool name, defaulting to warn. An unmapped tool or an empty gate: yields warn.
func configToolGate(cfg config.Config, tool string) string {
	key, ok := coverageToolConfigKey[tool]
	if !ok {
		return gateWarn
	}
	if g := cfg.Tools[key].Gate; g != "" {
		return string(g)
	}
	return gateWarn
}

// applyToolGate finalises the hard-gate decision for a check/scan run: --require-tools
// raises every coverage gap to fail, and any gap that gates fail stamps the verdict
// fail so the rendered output reflects the policy failure. Returns true when the run
// must exit 1. The policy decision lives here in cmd/ (the layering invariant) — the
// core ring never sees tool names or gate config. Idempotent and render-order safe:
// callers invoke it before rendering so the output shows the effective gate.
func applyToolGate(diag *diagnostic.Diagnostic, requireTools bool) bool {
	failed := false
	for i := range diag.CoverageGaps {
		if requireTools {
			diag.CoverageGaps[i].Gate = gateFail
		}
		if diag.CoverageGaps[i].Gate == gateFail {
			failed = true
		}
	}
	if failed {
		diag.Verdict = diagnostic.VerdictFail
	}
	return failed
}

// buildConfigWarnings assembles the advisory ConfigWarnings block: under-specified
// modules from cfg.Lint() (deterministic order) followed by any swallowed
// optional-tool errors. Returns nil when both are empty.
func buildConfigWarnings(cfg config.Config, toolWarnings []string) []string {
	lint := cfg.Lint()
	out := make([]string, 0, len(lint)+len(toolWarnings))
	for _, w := range lint {
		out = append(out, w.String())
	}
	out = append(out, toolWarnings...)
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildJudgmentDecisionTasks returns actionable decision-task strings for
// undeclared judgment inputs that force the scorer to abstain:
//
//  1. Modules with no subdomain AND no volatility declared — the scorer cannot
//     place their edges on the book's volatility scale. Tells the user to edit
//     .archfit.yaml and add subdomain: or volatility:.
//  2. Approved labels whose strength came from an LLM (provenance: llm) —
//     notifies the user they can upgrade provenance to "human" after code review.
//
// These are advisory strings appended to ConfigWarnings, not gate findings.
// Sorted for deterministic output.
func buildJudgmentDecisionTasks(cfg config.Config, lbls []labels.Label, configPath string) []string {
	var out []string

	// 1. Modules missing subdomain and volatility — scorer abstains on volatility.
	names := make([]string, 0, len(cfg.Modules))
	for name := range cfg.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := cfg.Modules[name]
		if def.Subdomain == "" && def.Volatility == "" {
			out = append(out,
				"decision needed: module "+name+" has no subdomain or volatility declared — "+
					"scorer abstains on volatility for its edges; "+
					"add `subdomain: core|supporting|generic` or `volatility: high|medium|low` "+
					"to modules."+name+" in "+configPath)
		}
	}

	// 2. LLM-provenance approved labels — inform the user they can promote to human.
	for _, l := range lbls {
		if l.Status == labels.StatusApproved && l.Provenance == labels.ProvenanceLLM {
			out = append(out,
				"decision needed: label "+l.From+" → "+l.To+
					" approved but provenance is llm — "+
					"if you have reviewed the code, set `provenance: human` in .archfit-labels.yaml "+
					"to restore full confidence in coupling_balance")
		}
	}

	return out
}

// outputInsideRootWarning reports whether dir (an absolute config/output
// directory) resolves strictly inside the absolute analyzed root. Returns a
// warning string in that case, "" when dir is the root itself or lies outside
// it. Path-only — no filesystem access — so it stays deterministic and testable.
func outputInsideRootWarning(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return "output written inside analyzed root (" + rel + ") — exclude it or " +
		"use a path outside --root to keep scans deterministic"
}

// loadConfig loads the config file at path. When path equals the default
// ".archfit.yaml" and the file is absent, it returns config.Default() so the
// tool works without a config file. An explicit --config path that is missing
// always returns an error. noConfig=true skips file loading entirely.
func loadConfig(ctx context.Context, path string, noConfig bool) (config.Config, error) {
	if noConfig {
		return config.Default(), nil
	}
	cfg, err := config.Load(ctx, path)
	if err == nil {
		return cfg, nil
	}
	if path == defaultConfigPath && errors.Is(err, os.ErrNotExist) {
		return config.Default(), nil
	}
	return config.Config{}, err
}

// applyFlagOverrides applies non-empty CLI flag values onto cfg, overriding
// whatever the config file (or Default) provided.
func applyFlagOverrides(cfg *config.Config, severity string, lang []string) error {
	if severity != "" {
		cfg.BCAdvisoryMinSeverity = severity
	}
	for _, key := range lang {
		canonical := languageByAlias(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown analyzer %q; see %s", key, languagesDocsURL)
		}
		if cfg.Tools == nil {
			cfg.Tools = make(map[string]config.ToolConfig)
		}
		cfg.Tools[canonical] = config.ToolConfig{Enabled: config.ModeOn}
	}
	return nil
}

// effectiveConfigHash returns the config hash that governed this run: the
// sha256 of the on-disk config file, or "" when --no-config ignored that file
// (built-in defaults were used). Hashing an ignored file would make the reported
// hash misleading and non-reproducible — it would change when a file the run
// never read changes.
func effectiveConfigHash(path string, noConfig bool) string {
	if noConfig {
		return ""
	}
	return computeConfigHash(path)
}

// computeConfigHash returns the sha256 hex digest of the raw config file bytes
// at path. Returns "" when the file cannot be read (absent, --no-config, etc.)
// so callers never fail on a missing config; they just get no hash.
func computeConfigHash(path string) string {
	b, err := os.ReadFile(path) //#nosec G304 -- path comes from the --config CLI flag, not arbitrary user input
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// verdictToError maps a diagnostic verdict to an exit error (nil = exit 0).
func verdictToError(v diagnostic.Verdict) error {
	switch v {
	case diagnostic.VerdictFail:
		return &exitError{code: 1}
	case diagnostic.VerdictWarn:
		return &exitError{code: 2}
	default:
		return nil
	}
}
