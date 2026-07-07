package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/deployunit"
	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// UpdateCmd syncs .archfit.yaml with the current project structure.
type UpdateCmd struct {
	Config      string `short:"c" help:"Config file path." default:".archfit.yaml"`
	Root        string `short:"r" help:"Project root directory (default: directory of --config)."`
	LLM         bool   `name:"llm"          help:"Run LLM classification for unclassified modules (off-gate)."`
	Apply       bool   `name:"apply"        help:"Write structural changes live into .archfit.yaml (backup created; LLM semantic proposals remain review-only)."`
	NoCache     bool   `name:"no-cache"     help:"Bypass the LLM response cache."`
	LLMProvider string `name:"llm-provider" help:"LLM provider override."  default:"anthropic"`
	LLMModel    string `name:"llm-model"    help:"LLM model override."     default:"claude-opus-4-8"`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	// It is never a CLI flag (no kong tag).
	providerOverride llm.Provider
}

func (c *UpdateCmd) Run(deps *appDeps) error {
	root, err := c.resolveRoot()
	if err != nil {
		return err
	}

	ctx := context.Background()

	cfg, err := loadConfig(ctx, c.Config, false)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: loading config: %v", err)}
	}

	originalBytes, err := os.ReadFile(c.Config)
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: reading config file: %v", err)}
	}

	existing := configToExisting(cfg.Modules)

	freshCfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}

	report := initcfg.DiffModules(existing, freshCfg.Modules)
	report.DeployUnitSuggestions = deployUnitSuggestions(ctx, root, cfg, deps)
	if c.LLM {
		var synthErr error
		report, synthErr = c.withRustSyntheticSuggestions(ctx, cfg, root, report, deps)
		if synthErr != nil {
			return synthErr
		}
	}
	addedNames := addedSet(report.Added)

	ann, err := c.maybeClassify(ctx, cfg, root, report, addedNames)
	if err != nil {
		return err
	}
	if c.LLM && ann != nil {
		report.RuleSuggestions = ruleSuggestionsFromAnnotations(ann)
		report.ExternalSystemSuggestions = externalSystemSuggestionsFromAnnotations(ann)
	}
	if c.LLM && ann != nil {
		warnTargets := initcfg.BuildClassifyTargets(root, classifyTargetsForUpdate(cfg, report, addedNames))
		warnPartialClassify(deps.Stdout, warnTargets, ann)
	}

	rustConfigNeeded := needsRustDeepAnalysisConfig(cfg, freshCfg.HasRust)
	hasEdits := hasActionableEdits(report) || rustConfigNeeded

	if !c.Apply {
		_, _ = fmt.Fprint(deps.Stdout, initcfg.RenderUpdateReport(report, ann, cfg.Layers))
		return nil
	}

	if !hasEdits {
		if (c.LLM && ann != nil) || hasReviewOnlySuggestions(report) {
			_, _ = fmt.Fprint(deps.Stdout, initcfg.RenderUpdateReport(report, ann, cfg.Layers))
			return nil
		}
		_, _ = fmt.Fprintln(deps.Stdout, "structurally in sync — no changes to apply")
		return nil
	}

	edited := originalBytes
	if hasActionableEdits(report) {
		edits := buildUpdateEdits(report)
		edited, err = initcfg.ApplyEdits(originalBytes, edits)
		if err != nil {
			return fmt.Errorf("applying edits: %w", err)
		}
	}
	if rustConfigNeeded {
		edited = ensureRustDeepAnalysisConfig(edited)
	}
	if err := safeWriteConfig(ctx, deps, c.Config, edited, originalBytes); err != nil {
		return err
	}
	if len(report.PathDrift) > 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "note: module paths replaced with discovered paths")
	}
	if (c.LLM && ann != nil) || hasReviewOnlySuggestions(report) {
		if rendered := initcfg.RenderAppliedLLMReview(report, ann); rendered != "" {
			_, _ = fmt.Fprint(deps.Stdout, rendered)
		}
	}
	return nil
}

// resolveRoot returns the absolute project root, defaulting to the config file's directory.
func (c *UpdateCmd) resolveRoot() (string, error) {
	root := c.Root
	if root == "" {
		root = filepath.Dir(c.Config)
	}
	if !filepath.IsAbs(root) {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolving root: %w", err)
		}
	}
	return root, nil
}

// maybeClassify runs the LLM classification pass when --llm is set.
func (c *UpdateCmd) maybeClassify(
	ctx context.Context,
	cfg config.Config,
	root string,
	report initcfg.UpdateReport,
	addedNames map[string]struct{},
) (map[string]initcfg.ModuleAnnotation, error) {
	if !c.LLM {
		return nil, nil
	}

	p, err := c.buildLLMProvider(cfg)
	if err != nil {
		return nil, err
	}

	targets := classifyTargetsForUpdate(cfg, report, addedNames)
	classifyTargets := initcfg.BuildClassifyTargets(root, targets)
	if len(classifyTargets) == 0 {
		return nil, nil
	}

	repoEvidence := architectureEvidenceLines(root, targets, c.Config, updateEvidenceDiagnostics(report))
	ann, err := classifyModulesWithEvidence(ctx, p, classifyTargets, cfg.Layers, repoEvidence)
	if err != nil {
		return nil, &exitError{code: 3, msg: fmt.Sprintf("error: classify failed: %v", err)}
	}
	return ann, nil
}

// buildLLMProvider constructs the LLM provider, honouring the test seam and cache flag.
func (c *UpdateCmd) buildLLMProvider(cfg config.Config) (llm.Provider, error) {
	var llmCfg config.LLMConfig
	if lc, ok := cfg.LLM(); ok {
		llmCfg = lc
	}
	llmCfg.Provider = c.LLMProvider
	llmCfg.Model = c.LLMModel

	cacheDir := llmCacheDir(filepath.Dir(c.Config))
	p, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.NoCache)
	if err != nil {
		return nil, &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}
	return p, nil
}

func (c *UpdateCmd) withRustSyntheticSuggestions(
	ctx context.Context,
	cfg config.Config,
	root string,
	report initcfg.UpdateReport,
	deps *appDeps,
) (initcfg.UpdateReport, error) {
	extractCfg := cfg.ForExtract(config.LangRust)
	if extractCfg.Mode == config.ModeOff || !extractCfg.ModuleGraph {
		return report, nil
	}

	var facts *factcache.Store
	if !c.NoCache {
		facts = factcache.NewStore(factsCacheDir(filepath.Dir(c.Config)))
	}
	ex := rust.New(deps.Runner, extractCfg)
	ex.Cache = facts
	rustFacts, _, err := ex.Extract(ctx, scope.Scope{Root: root})
	if err != nil {
		return report, &exitError{code: 3, msg: fmt.Sprintf("error: discovering Rust synthetic modules: %v", err)}
	}

	g := graph.Build([]graph.Facts{rustFacts})
	augmented := classify.AugmentModulesFromGraph(g, cfg.ForClassify().Modules)
	if len(augmented) == len(cfg.Modules) {
		return report, nil
	}

	existingNames := make(map[string]struct{}, len(cfg.Modules)+len(report.Added)+len(report.Suggested))
	for name := range cfg.Modules {
		existingNames[name] = struct{}{}
	}
	for _, m := range report.Added {
		existingNames[m.Name] = struct{}{}
	}
	for _, m := range report.Suggested {
		existingNames[m.Name] = struct{}{}
	}

	var paths []string
	for path := range augmented {
		if _, configured := cfg.Modules[path]; configured || !strings.Contains(path, "::") {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		def := augmented[path]
		if len(def.Paths) == 0 {
			def.Paths = []string{path}
		}
		report.Suggested = append(report.Suggested, initcfg.ModuleDef{
			Name:  uniqueSyntheticModuleName(path, existingNames),
			Paths: def.Paths,
			Layer: def.Layer,
		})
	}
	return report, nil
}

func uniqueSyntheticModuleName(path string, used map[string]struct{}) string {
	base := strings.ReplaceAll(path, "::", "-")
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

// classifyTargetsForUpdate collects ModuleDefs to classify: Added modules from discovery,
// plus existing unclassified modules (excluding those already in addedNames).
func classifyTargetsForUpdate(
	cfg config.Config,
	report initcfg.UpdateReport,
	addedNames map[string]struct{},
) []initcfg.ModuleDef {
	targets := make([]initcfg.ModuleDef, 0, len(report.Added)+len(report.Suggested)+len(report.Unclassified))
	targets = append(targets, report.Added...)
	targets = append(targets, report.Suggested...)
	for _, name := range report.Unclassified {
		if _, isAdded := addedNames[name]; isAdded {
			continue
		}
		if def, ok := cfg.Modules[name]; ok {
			targets = append(targets, initcfg.ModuleDef{Name: name, Paths: def.Paths, Public: def.Public})
		} // name came from DiffModules over cfg.Modules, so absent is purely defensive
	}
	return targets
}

// hasActionableEdits returns true when there is at least one structural change.
// LLM role/volatility output is review-only: it is rendered as a diff but never
// written by config update --apply.
func hasActionableEdits(report initcfg.UpdateReport) bool {
	return len(report.Added) > 0 || len(report.Removed) > 0 || len(report.PathDrift) > 0
}

func hasReviewOnlySuggestions(report initcfg.UpdateReport) bool {
	return len(report.Suggested) > 0 || len(report.DeployUnitSuggestions) > 0 || len(report.RuleSuggestions) > 0 || len(report.ExternalSystemSuggestions) > 0
}

func deployUnitSuggestions(ctx context.Context, root string, cfg config.Config, deps *appDeps) []initcfg.DeployUnitSuggestion {
	mm := cfg.ModuleMapView()
	detected := deployunit.Detect(ctx, root, mm, deps.Runner)
	if len(detected) == 0 {
		return nil
	}
	paths := make([]string, 0, len(detected))
	for path := range detected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	seen := make(map[string]struct{}, len(paths))
	out := make([]initcfg.DeployUnitSuggestion, 0, len(paths))
	for _, path := range paths {
		mod, ok := mm.ModuleForFile(path)
		if !ok {
			if !mm.Has(path) {
				continue
			}
			mod = path
		}
		if _, exists := seen[mod]; exists {
			continue
		}
		def, ok := cfg.Modules[mod]
		if !ok || def.DeployUnit != "" {
			continue
		}
		seen[mod] = struct{}{}
		out = append(out, initcfg.DeployUnitSuggestion{Module: mod, Unit: detected[path], Source: path})
	}
	return out
}

func ruleSuggestionsFromAnnotations(ann map[string]initcfg.ModuleAnnotation) []initcfg.RuleSuggestion {
	if len(ann) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]initcfg.RuleSuggestion, 0)
	for module, a := range ann {
		for _, suggestion := range a.RuleSuggestions {
			if suggestion.SourceModule == "" {
				suggestion.SourceModule = module
			}
			key := strings.Join([]string{suggestion.Type, suggestion.ID, suggestion.From, suggestion.To, suggestion.SourceModule}, "\x00")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, suggestion)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].SourceModule < out[j].SourceModule
	})
	return out
}

func externalSystemSuggestionsFromAnnotations(ann map[string]initcfg.ModuleAnnotation) []initcfg.ExternalSystemSuggestion {
	if len(ann) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]initcfg.ExternalSystemSuggestion, 0)
	for module, a := range ann {
		for _, suggestion := range a.ExternalSystemSuggestions {
			if suggestion.SourceModule == "" {
				suggestion.SourceModule = module
			}
			key := strings.Join([]string{suggestion.Name, strings.Join(suggestion.Targets, ","), suggestion.SourceModule}, "\x00")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, suggestion)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].SourceModule < out[j].SourceModule
	})
	return out
}

// buildUpdateEdits constructs the ordered Edit slice for an apply pass.
func buildUpdateEdits(report initcfg.UpdateReport) []initcfg.Edit {
	edits := make([]initcfg.Edit, 0, len(report.Added)+len(report.PathDrift)+len(report.Removed)+len(report.Unclassified))

	for _, def := range report.Added {
		edits = append(edits, initcfg.AddModuleEdit{Def: def})
	}

	for _, d := range report.PathDrift {
		edits = append(edits, initcfg.UpdateModulePathsEdit{Module: d.Name, Paths: d.DiscoveredPaths})
	}

	for _, e := range report.Removed {
		edits = append(edits, initcfg.CommentModuleEdit{Module: e.Name, Note: "not found in discovery"})
	}

	return edits
}

// collectUpdateRepoEvidence is kept for older prompt tests; update --llm uses
// the shared architecture evidence pack directly.
func collectUpdateRepoEvidence(root string) []string {
	return architectureEvidenceLines(root, nil, "", nil)
}

// configToExisting projects config.Modules into []initcfg.ExistingModule.
func configToExisting(modules map[string]config.ModuleDef) []initcfg.ExistingModule {
	out := make([]initcfg.ExistingModule, 0, len(modules))
	for name, def := range modules {
		out = append(out, initcfg.ExistingModule{
			Name:          name,
			Paths:         def.Paths,
			HasSubdomain:  def.Subdomain != "",
			HasVolatility: def.Volatility != "",
			HasLayer:      def.Layer != "",
		})
	}
	return out
}

// addedSet builds a set of Added module names.
func addedSet(added []initcfg.ModuleDef) map[string]struct{} {
	s := make(map[string]struct{}, len(added))
	for _, a := range added {
		s[a.Name] = struct{}{}
	}
	return s
}
