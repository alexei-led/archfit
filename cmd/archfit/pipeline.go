package main

import (
	"context"
	"errors"
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
	"github.com/alexei-led/archfit/internal/extract/gitnexus"
	"github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/fitness"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/labels"
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
func runPipeline(ctx context.Context, deps *appDeps, cfg config.Config, configPath string, mode engine.Mode, base baseline.Baseline, extraMetrics ...metrics.Metric) (diagnostic.Diagnostic, error) {
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
	// risk_hub reads hand-authored volatility only; ApplyVolatility records
	// churn-derived bands in a separate store, so this call order is free.
	ms := append(metrics.New(cfg), extraMetrics...)

	// Recent git history (cheap; runs by default): per-file churn drives module
	// volatility (unbalanced_edge, BC severity) and the modularity metrics
	// (change_amplification, hidden_coupling). Hand-authored volatility/subdomain
	// config always wins; a non-git repo leaves these signals empty.
	change := signal.ChangeHistory{}
	if churn, coChange, _, herr := git.History(ctx, s.Root, deps.Runner); herr == nil {
		cfg.ApplyVolatility(config.DeriveVolatility(cfg.Modules, churn))
		change.FileChurn, change.CoChange = churn, coChange
	}

	// LOC walk — repo-relative path→line-count map + coverage record.
	// ExtraCoverage order: loc, complexity, clones, gitnexus.
	var locCov diagnostic.Coverage
	change.FileLOC, locCov, _ = loc.Run(s.Root)
	change.ExtraCoverage = append(change.ExtraCoverage, locCov)

	// Architecture-fitness enforcement signals (deterministic FS scan; always runs).
	change.FitnessSignals = fitness.Detect(s.Root)

	// Ownership resolution: fills module owner gaps from CODEOWNERS or git-author
	// history. Explicit config owner always wins; resolver only fills empty slots.
	// Absent CODEOWNERS and non-git repos yield an empty map — no fabrication.
	cfg.FillMissingOwners(ownership.Resolve(ctx, s.Root, cfg.ModuleMapView(), deps.Runner))

	// Cyclomatic complexity via an external multi-language tool (lizard) — opt-in
	// (tools.complexity.enabled: on) like SCIP, since it shells out and adds cost.
	// Coverage carries zero file counts; status only — mirrors clones absent/ok pattern.
	var complexityCov diagnostic.Coverage
	change.Complexity, complexityCov, _ = complexity.Run(ctx, deps.Runner, s.Root, cfg.ComplexityEnabled())
	change.ExtraCoverage = append(change.ExtraCoverage, complexityCov)

	// Clone detection — opt-in (tools.clones.enabled: on). Run returns empty+absent
	// when disabled or the tool is missing; the metric reports n/a in that case.
	var clonesCov diagnostic.Coverage
	change.CloneClusters, clonesCov, _ = clones.Run(ctx, deps.Runner, s.Root, cfg.ClonesEnabled())
	change.ExtraCoverage = append(change.ExtraCoverage, clonesCov)

	// gitnexus optional symbol-impact enrichment — opt-in (tools.gitnexus.enabled: on).
	// Never auto: gitnexus may require network access. Returns empty+absent when
	// disabled or tool absent; risk_hub falls back to surface-breadth-only in that case.
	// Coverage is appended to ExtraCoverage so the engine includes it in the diagnostic.
	var gitnexusCov diagnostic.Coverage
	change.GitnexusImpact, gitnexusCov, _ = gitnexus.Run(ctx, deps.Runner, s.Root, cfg.GitnexusEnabled())
	change.ExtraCoverage = append(change.ExtraCoverage, gitnexusCov)

	// Pinned coupling labels (.archfit-labels.yaml): the human-reviewed output of
	// `archfit enrich`. Optional; a malformed file fails loudly — a half-read
	// labels file must never silently alter the gate.
	lbls, err := labels.Load(filepath.Join(configDir, defaultLabelsPath))
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
		Change:      change,
		Now:         time.Now(),
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
	return diag, nil
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
