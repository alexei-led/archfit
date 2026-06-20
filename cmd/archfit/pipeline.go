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
	"github.com/alexei-led/archfit/internal/extract/gitnexus"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/extract/ts"
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
	cs, err := git.Changed(ctx, g.workDir, base, head, g.runner)
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
func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, configPath string, noConfig bool, mode engine.Mode, base baseline.Baseline, extraMetrics ...metrics.Metric) (diagnostic.Diagnostic, error) {
	configDir := filepath.Dir(configPath)
	sc := cfg.ForScope()
	sc.WorkDir = configDir
	sc.Base = mode.Base
	sc.Full = mode.Full
	s, err := scope.Resolve(ctx, sc, gitResolver{workDir: configDir, runner: deps.Runner})
	if err != nil {
		return diagnostic.Diagnostic{}, err
	}

	extractors := []ports.Extractor{
		golang.New(cfg.ForExtract("go")),
		ts.New(deps.Runner, cfg.ForExtract("typescript")),
		py.New(deps.Runner, cfg.ForExtract("python")),
	}

	rs := rules.New(cfg.ForRules())
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

	// Recent git history (cheap; runs by default): per-file churn drives module
	// volatility (unbalanced_edge, BC severity) and the modularity metrics
	// (change_amplification, hidden_coupling). Hand-authored volatility/subdomain
	// config always wins; a non-git repo leaves these signals empty.
	change := signal.RunSignals{}
	if churn, coChange, _, herr := git.History(ctx, s.Root, deps.Runner); herr == nil {
		change.History.FileChurn, change.History.CoChange = churn, coChange
	}

	// LOC walk — repo-relative path→line-count map + coverage record.
	// ExtraCoverage order: loc, complexity, clones, gitnexus.
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

	// Ownership resolution: fills module owner gaps from CODEOWNERS or git-author
	// history. Explicit config owner always wins; resolver only fills empty slots.
	// Absent CODEOWNERS and non-git repos yield an empty map — no fabrication.
	cfg.FillMissingOwners(ownership.Resolve(ctx, s.Root, cfg.ModuleMapView(), deps.Runner))

	// Deploy-unit detection: fills module deploy_unit gaps from static repo
	// analysis (Go main pkgs, Dockerfiles, k8s manifests, package.json workspaces,
	// pyproject.toml). Detect keys results by repo-relative path; KeyByModule
	// remaps those to module names, which is what FillMissingDeployUnits expects
	// (without it, auto-detected units are dropped unless a module's map key
	// equals the path). Config-authored deploy_unit always wins.
	duModules := cfg.ModuleMapView()
	cfg.FillMissingDeployUnits(deployunit.KeyByModule(deployunit.Detect(ctx, s.Root, duModules, deps.Runner), duModules))

	// Cyclomatic complexity via an external multi-language tool (lizard) — opt-in
	// (tools.complexity.enabled: on) like SCIP, since it shells out and adds cost.
	// Coverage carries zero file counts; status only — mirrors clones absent/ok pattern.
	var complexityCov diagnostic.Coverage
	change.Complexity.Funcs, complexityCov, toolErr = complexity.Run(ctx, deps.Runner, s.Root, cfg.ComplexityEnabled())
	noteToolErr("lizard", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, complexityCov)

	// Clone detection — opt-in (tools.clones.enabled: on). Run returns empty+absent
	// when disabled or the tool is missing; the metric reports n/a in that case.
	var clonesCov diagnostic.Coverage
	change.Duplication.Clusters, clonesCov, toolErr = clones.Run(ctx, deps.Runner, s.Root, cfg.ClonesEnabled())
	noteToolErr("jscpd", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, clonesCov)

	// gitnexus optional symbol-impact enrichment. tools.gitnexus.enabled is
	// three-state: on always attempts, off respects the opt-out (but reports a
	// present index), and auto/unset auto-detects — a present .gitnexus/.codegraph
	// index is used automatically. Returns empty+absent when not used or the CLI is
	// absent; risk_hub falls back to surface-breadth-only in that case.
	// Coverage is appended to ExtraCoverage so the engine includes it in the diagnostic.
	var gitnexusCov diagnostic.Coverage
	change.GitnexusImpact, gitnexusCov, toolErr = gitnexus.Run(ctx, deps.Runner, s.Root, cfg.GitnexusEnabled(), cfg.GitnexusExplicitlyDisabled())
	noteToolErr("gitnexus", toolErr)
	change.ExtraCoverage = append(change.ExtraCoverage, gitnexusCov)

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
		PatternCfg:  patternCfg,
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      lbls,
		Signals:     change,
		Now:         time.Now(),
		ConfigHash:  configHash,
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
	diag.AgentTasks = agenttask.Build(diag.Findings, ruleTypes, modulePublic, []string{validate})

	// Warn-loud coverage reporting: turn the absent tool-coverage records into a
	// machine-readable CoverageGaps block (tool → unlocked metrics → install cmd)
	// and surface config-quality lint plus any swallowed optional-tool errors in
	// ConfigWarnings so they reach md/json/CI instead of being stderr-only.
	diag.CoverageGaps = buildCoverageGaps(diag.ToolCoverage)
	diag.ConfigWarnings = buildConfigWarnings(cfg, toolWarnings)
	return diag, nil
}

// gateWarn is the default coverage-gap posture: a missing tool degrades metrics
// to n/a (never green) and is reported, but does not fail the build. An opt-in
// hard gate (tools.<x>.gate: fail / --require-tools) raises this to "fail".
const gateWarn = "warn"

// primaryGraphMetrics are the metrics the dependency-graph extractors
// (go/packages, dependency-cruiser, grimp) unlock; absent any of them, all of
// these drop to n/a. Shared (read-only) across those three table entries.
var primaryGraphMetrics = []string{"coverage", "coupling_balance", "encapsulation", "cycle", "blast_radius"}

// toolAffectedMetrics maps an absent analyzer's coverage name to its one-line
// install hint and the metrics its absence leaves unmeasured. Only tools listed
// here produce a CoverageGap — an absent coverage entry with no actionable
// install path is not a gap a user can close. Static and deterministic.
var toolAffectedMetrics = map[string]struct {
	install string
	metrics []string
}{
	"go/packages":        {"https://go.dev/dl (bundled with the Go toolchain)", primaryGraphMetrics},
	"dependency-cruiser": {"npm install -g dependency-cruiser", primaryGraphMetrics},
	"grimp":              {"uv tool install grimp / pip install grimp", primaryGraphMetrics},
	toolLizard:           {"uv tool install lizard / pip install lizard", []string{"complexity"}},
	toolJscpd:            {"npm install -g jscpd", []string{"functional_candidates"}},
	toolGitnexus:         {"see docs/guide — git-history change-coupling index", []string{"risk_hub"}},
}

// buildCoverageGaps derives the CoverageGaps block from the absent tool-coverage
// records. Gaps are sorted by tool name so a double-run stays byte-identical
// regardless of upstream coverage order. Returns nil when no known tool is
// absent (omitempty keeps the field out of clean output).
func buildCoverageGaps(cov []diagnostic.Coverage) []diagnostic.CoverageGap {
	var gaps []diagnostic.CoverageGap
	for _, c := range cov {
		if c.Status != diagnostic.StatusAbsent {
			continue
		}
		info, ok := toolAffectedMetrics[c.Tool]
		if !ok {
			continue
		}
		gaps = append(gaps, diagnostic.CoverageGap{
			Tool:            c.Tool,
			InstallCmd:      info.install,
			AffectedMetrics: info.metrics,
			Gate:            gateWarn,
		})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Tool < gaps[j].Tool })
	return gaps
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

// canonicalLang maps a --lang key (go, ts, py, or full name) to the config
// language name. Returns "" for unknown keys.
func canonicalLang(key string) string {
	switch key {
	case "go":
		return config.LangGo
	case "ts", config.LangTypeScript:
		return config.LangTypeScript
	case "py", config.LangPython:
		return config.LangPython
	default:
		return ""
	}
}

// applyFlagOverrides applies non-empty CLI flag values onto cfg, overriding
// whatever the config file (or Default) provided.
func applyFlagOverrides(cfg *config.Config, severity string, lang []string) error {
	if severity != "" {
		cfg.BCAdvisoryMinSeverity = severity
	}
	for _, key := range lang {
		canonical := canonicalLang(key)
		if canonical == "" {
			return fmt.Errorf("--lang: unknown language %q; use go, ts, or py", key)
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
