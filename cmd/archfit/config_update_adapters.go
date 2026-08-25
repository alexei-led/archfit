package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"

	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/deployunit"
	"github.com/alexei-led/archfit/internal/extract/dynimports"
	"github.com/alexei-led/archfit/internal/extract/registry"
	runtimedetect "github.com/alexei-led/archfit/internal/extract/runtime"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

// ruleTypeForbiddenLayerDirection is the rule type whose presence makes a
// module's `layer:` field load-bearing.
const ruleTypeForbiddenLayerDirection = "forbidden_layer_direction"

type configUpdateAdapter struct {
	cmd      *UpdateCmd
	deps     *appDeps
	cfg      config.Config
	report   initcfg.UpdateReport
	ann      map[string]initcfg.ModuleAnnotation
	provider llm.Provider
}

func newConfigUpdateAdapter(cmd *UpdateCmd, deps *appDeps) *configUpdateAdapter {
	return &configUpdateAdapter{cmd: cmd, deps: deps}
}

func (a *configUpdateAdapter) LoadConfigUpdate(ctx context.Context, path string) (application.ConfigUpdateConfig, error) {
	cfg, err := loadConfig(ctx, path)
	if err != nil {
		return application.ConfigUpdateConfig{}, &exitError{code: 3, msg: fmt.Sprintf("error: loading config: %v", err)}
	}
	a.cfg = cfg
	out := application.ConfigUpdateConfig{
		Modules: make(map[string]application.ConfigUpdateModule, len(cfg.Modules)),
		Layers:  append([]string(nil), cfg.Layers...),
	}
	for name, mod := range cfg.Modules {
		out.Modules[name] = application.ConfigUpdateModule{
			Name: name, Paths: append([]string(nil), mod.Paths...), Public: append([]string(nil), mod.Public...),
		}
	}
	return out, nil
}

func (a *configUpdateAdapter) ReadConfigUpdateFile(_ context.Context, path string) ([]byte, error) {
	data, err := os.ReadFile(path) //#nosec G304
	if err != nil {
		return nil, &exitError{code: 3, msg: fmt.Sprintf("error: reading config file: %v", err)}
	}
	return data, nil
}

func (a *configUpdateAdapter) DiscoverConfigUpdate(ctx context.Context, req application.ConfigUpdateDiscoveryRequest) error {
	fresh, err := initcfg.Discover(ctx, req.Root, a.deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}
	report := initcfg.DiffModules(configToExisting(a.cfg.Modules), fresh.Modules, requiresLayerClassification(a.cfg))
	candidate := candidateConfigForUpdate(a.cfg, fresh, report.NameDrift)
	report.DeployUnitSuggestions = deployUnitSuggestions(ctx, req.Root, candidate, a.deps)
	report.DistanceConfigCandidates = distanceConfigCandidates(ctx, req.Root, candidate, a.deps)
	if req.AIClassify {
		report, err = a.cmd.withRustSyntheticSuggestions(ctx, a.cfg, req.Root, report, a.deps)
		if err != nil {
			return err
		}
	}
	// The manifest-aware project probe is shared with analysis coverage. Root-only
	// discovery is insufficient when a configured Rust manifest lives in a sub-crate.
	if needsRustDeepAnalysisConfig(a.cfg, registry.ProjectPresent(config.LangRust, req.Root, a.cfg.ForExtract(config.LangRust))) {
		report.Settings = append(report.Settings, initcfg.RustDeepAnalysisSetting())
	}
	a.report = report
	return nil
}

func (a *configUpdateAdapter) ProjectConfigUpdate(context.Context) (application.ConfigUpdatePlan, error) {
	return application.ConfigUpdatePlan{
		Added:               toApplicationUpdateModules(a.report.Added),
		Suggested:           toApplicationUpdateModules(a.report.Suggested),
		Unclassified:        append([]string(nil), a.report.Unclassified...),
		PendingModuleEdits:  initcfg.HasModuleEdits(a.report),
		PendingSettingEdits: len(a.report.Settings) > 0,
		ReviewItems:         initcfg.HasReviewItems(a.report),
		PathDrift:           len(a.report.PathDrift) > 0,
	}, nil
}

func toApplicationUpdateModules(modules []initcfg.ModuleDef) []application.ConfigUpdateModule {
	out := make([]application.ConfigUpdateModule, 0, len(modules))
	for _, mod := range modules {
		out = append(out, application.ConfigUpdateModule{
			Name: mod.Name, Paths: append([]string(nil), mod.Paths...), Public: append([]string(nil), mod.Public...),
		})
	}
	return out
}

func (a *configUpdateAdapter) ValidateConfigUpdateClassifier(context.Context) error {
	provider, err := a.cmd.buildAIProvider(a.cfg)
	if err != nil {
		return err
	}
	a.provider = provider
	return nil
}

func (a *configUpdateAdapter) ClassifyConfigUpdate(ctx context.Context, req application.ConfigUpdateClassificationRequest) ([]application.ConfigUpdateClassification, error) {
	modules := make([]initcfg.ModuleDef, 0, len(req.Modules))
	for _, mod := range req.Modules {
		modules = append(modules, initcfg.ModuleDef{Name: mod.Name, Paths: append([]string(nil), mod.Paths...), Public: append([]string(nil), mod.Public...)})
	}
	targets := initcfg.BuildClassifyTargets(req.Root, modules)
	repoEvidence := architectureEvidenceLines(req.Root, modules, a.cmd.Config, updateEvidenceDiagnostics(a.report))
	ann, err := classifyModulesWithEvidence(ctx, a.provider, targets, req.Layers, repoEvidence)
	if err != nil {
		return nil, &exitError{code: 3, msg: fmt.Sprintf("error: classify failed: %v", err)}
	}
	a.ann = ann
	a.report.RuleSuggestions = ruleSuggestionsFromAnnotations(ann)
	a.report.ExternalSystemSuggestions = externalSystemSuggestionsFromAnnotations(ann)
	out := make([]application.ConfigUpdateClassification, 0, len(ann))
	for module, annotation := range ann {
		out = append(out, application.ConfigUpdateClassification{Module: module, Subdomain: annotation.Subdomain, Volatility: annotation.Volatility})
	}
	return out, nil
}

func (a *configUpdateAdapter) RenderConfigUpdateReview(_ context.Context, asJSON bool) ([]byte, error) {
	if asJSON {
		var out bytes.Buffer
		if err := writeConfigReviewJSON(&out, initcfg.BuildConfigReview(a.report)); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	return []byte(initcfg.RenderReviewStatus(initcfg.BuildConfigReview(a.report)) + initcfg.RenderUpdateReport(a.report, a.ann, a.cfg.Layers)), nil
}

func (a *configUpdateAdapter) RenderAppliedConfigUpdateReview(context.Context) ([]byte, error) {
	return []byte(initcfg.RenderAppliedReview(a.report, a.ann)), nil
}

func (a *configUpdateAdapter) EditConfigUpdate(_ context.Context, original []byte) ([]byte, error) {
	edited := original
	var err error
	if initcfg.HasModuleEdits(a.report) {
		edited, err = initcfg.ApplyEdits(original, buildUpdateEdits(a.report))
		if err != nil {
			return nil, fmt.Errorf("applying edits: %w", err)
		}
	}
	if len(a.report.Settings) > 0 {
		edited = ensureRustDeepAnalysisConfig(edited, a.cfg)
	}
	return edited, nil
}

func (a *configUpdateAdapter) WriteConfigUpdate(ctx context.Context, path string, edited, original []byte) error {
	return safeWriteConfig(ctx, a.deps, path, edited, original)
}

// requiresLayerClassification reports whether a module without `layer:` blocks an
// active layer policy. A forbidden_layer_direction rule is live unless its gate
// is "off" — unset and "warn" both still check layers.
func requiresLayerClassification(cfg config.Config) bool {
	for _, r := range cfg.Rules {
		if r.Type == ruleTypeForbiddenLayerDirection && r.Gate != string(config.GateOff) {
			return true
		}
	}
	return false
}

// writeConfigReviewJSON emits the versioned review document.
func writeConfigReviewJSON(w io.Writer, review initcfg.ConfigReview) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(review); err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: encoding config review: %v", err)}
	}
	return nil
}

// buildAIProvider constructs the AI provider, honouring the test seam and cache flag.
func (c *UpdateCmd) buildAIProvider(cfg config.Config) (llm.Provider, error) {
	var llmCfg config.LLMConfig
	if lc, ok := cfg.LLM(); ok {
		llmCfg = lc
	}
	llmCfg.Provider = c.AIProvider
	llmCfg.Model = c.AIModel

	cacheDir := llmCacheDir(filepath.Dir(c.Config))
	p, err := buildCachedProvider(c.providerOverride, llmCfg, cacheDir, c.Refresh)
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

	facts := factcache.NewStore(factsCacheDir(filepath.Dir(c.Config)))
	facts.RefreshMode = c.Refresh
	ex := registry.New(config.LangRust, deps.Runner, cfg.ForExtract(config.LangRust), facts)
	rustFacts, _, err := ex.Extract(ctx, scope.Scope{Root: root})
	if err != nil {
		return report, &exitError{code: 3, msg: fmt.Sprintf("error: discovering Rust synthetic modules: %v", err)}
	}

	g := graph.Build([]graph.Facts{rustFacts})
	augmentedCfg, _ := apppipeline.ClassifyGraph(g, cfg.ForClassify())
	augmented := augmentedCfg.Modules
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

// candidateConfigForUpdate projects the config as it stands AFTER `config update
// --apply`: discovery's module set, with every preserved stanza's hand-authored
// metadata carried across. The deploy-unit and distance suggestion builders read
// this projection, so a stanza whose metadata goes missing here yields a
// suggestion to set a field the real config already sets.
//
// drift is why the lookup cannot key on the discovered name alone. When
// discovery re-emits `internal/foo` as `foo` over the SAME path set, --apply
// writes nothing at all: the stanza keeps its name and every field it carries
// (name drift is review-only precisely because rewriting it would discard them).
// So a drifted module enters the candidate under its CONFIG name, which is also
// the only name the metadata lookup can find. Resolving the name BEFORE the
// lookup keeps one copy of the field list — a second branch is a second place to
// forget a field.
//
// The config name cannot collide with another discovered module: it comes from
// the report's Removed bucket, which by construction holds only names discovery
// did not emit, and each drift pairing is unique.
func candidateConfigForUpdate(cfg config.Config, discovered initcfg.DiscoveredConfig, drift []initcfg.NameDrift) config.Config {
	if len(discovered.Modules) == 0 {
		return cfg
	}
	configNameFor := make(map[string]string, len(drift))
	for _, d := range drift {
		configNameFor[d.DiscoveredName] = d.ConfigName
	}
	out := cfg
	out.Modules = make(map[string]config.ModuleDef, len(discovered.Modules))
	for _, mod := range discovered.Modules {
		name := mod.Name
		if configName, drifted := configNameFor[name]; drifted {
			name = configName
		}
		def := config.ModuleDef{
			Paths:    append([]string(nil), mod.Paths...),
			Public:   append([]string(nil), mod.Public...),
			Internal: append([]string(nil), mod.Internal...),
			Layer:    mod.Layer,
		}
		if existing, ok := cfg.Modules[name]; ok {
			def.Public = append([]string(nil), existing.Public...)
			def.Internal = append([]string(nil), existing.Internal...)
			def.Layer = existing.Layer
			def.Subdomain = existing.Subdomain
			def.Volatility = existing.Volatility
			def.Owner = existing.Owner
			def.DeployUnit = existing.DeployUnit
			def.Role = existing.Role
			def.ReviewedAt = existing.ReviewedAt
			def.ReviewedBy = existing.ReviewedBy
		}
		out.Modules[name] = def
	}
	return out
}

func distanceConfigCandidates(ctx context.Context, root string, cfg config.Config, deps *appDeps) []initcfg.DistanceConfigCandidate {
	mm := cfg.ModuleMapView()
	dynamicImports := apppipeline.BuildDynamicImports(dynimports.Detect(root), mm)
	runtimeResult := runtimedetect.Detect(ctx, root, deps.Runner)
	runtimeSites := make([]evidence.RuntimeAsyncSite, 0, len(runtimeResult.Signals))
	for _, sig := range runtimeResult.Signals {
		runtimeSites = append(runtimeSites, evidence.RuntimeAsyncSite{
			File:            sig.File,
			Line:            sig.Line,
			Library:         sig.Library,
			IntegrationKind: string(sig.IntegrationKind),
			Language:        sig.Language,
		})
	}
	runtimeEdges := apppipeline.BuildRuntimeAsyncEdges(runtimeSites, runtimeResult.Confidence, mm)
	dynamicSignals := apppipeline.BuildDynamicConnascenceSignals(dynamicImports, runtimeEdges, nil)
	candidates := append(
		staticExternalDistanceConfigCandidates(ctx, root, cfg, deps),
		apppipeline.BuildDistanceConfigCandidates(dynamicImports, runtimeEdges, dynamicSignals)...,
	)
	out := make([]initcfg.DistanceConfigCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, toInitcfgDistanceConfigCandidate(c))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceBlock != out[j].SourceBlock {
			return out[i].SourceBlock < out[j].SourceBlock
		}
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].IntegrationKind < out[j].IntegrationKind
	})
	return out
}

func staticExternalDistanceConfigCandidates(ctx context.Context, root string, cfg config.Config, deps *appDeps) []evidence.DistanceConfigCandidate {
	g := buildUpdateCandidateGraph(ctx, root, cfg, deps)
	if g == nil {
		return nil
	}
	return staticExternalDistanceConfigCandidatesFromGraph(g, cfg)
}

func staticExternalDistanceConfigCandidatesFromGraph(g *graph.Graph, cfg config.Config) []evidence.DistanceConfigCandidate {
	classifyCfg, idx := apppipeline.ClassifyGraph(g, cfg.ForClassify())
	return apppipeline.BuildStaticExternalDistanceCandidates(g, idx, classifyCfg.ModuleMap)
}

func buildUpdateCandidateGraph(ctx context.Context, root string, cfg config.Config, deps *appDeps) *graph.Graph {
	extractors := registry.Build(deps.Runner, extractConfigs(cfg), nil)
	allFacts := make([]graph.Facts, 0, len(extractors))
	for _, ex := range extractors {
		facts, _, err := ex.Extract(ctx, scope.Scope{Root: root})
		if err != nil {
			continue
		}
		if len(facts.Nodes) == 0 && len(facts.Edges) == 0 {
			continue
		}
		allFacts = append(allFacts, facts)
	}
	if len(allFacts) == 0 {
		return nil
	}
	return graph.Build(allFacts)
}

func toInitcfgDistanceConfigCandidate(c evidence.DistanceConfigCandidate) initcfg.DistanceConfigCandidate {
	return initcfg.DistanceConfigCandidate{
		SourceBlock:           c.SourceBlock,
		Module:                c.Module,
		Target:                c.Target,
		IntegrationKind:       c.IntegrationKind,
		Count:                 c.Count,
		EvidenceRefs:          distanceConfigEvidenceRefs(c.EvidenceSites),
		SuggestedReviewAction: c.SuggestedReviewAction,
	}
}

func distanceConfigEvidenceRefs(sites []evidence.DistanceConfigEvidenceSite) []string {
	refs := make([]string, 0, len(sites))
	for _, s := range sites {
		if s.File == "" {
			continue
		}
		ref := s.File
		if s.Line > 0 {
			ref = fmt.Sprintf("%s:%d", s.File, s.Line)
		}
		refs = append(refs, ref)
	}
	return refs
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
	edits := make([]initcfg.Edit, 0, len(report.Added)+len(report.PathDrift))

	for _, def := range report.Added {
		edits = append(edits, initcfg.AddModuleEdit{Def: def})
	}

	for _, d := range report.PathDrift {
		edits = append(edits, initcfg.UpdateModulePathsEdit{Module: d.Name, Paths: d.DiscoveredPaths})
	}

	// No edit is built for report.Removed: see hasActionableEdits. Removing a
	// configured stanza stays a human decision, and the report says so.

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
			HasOwner:      def.Owner != "",
		})
	}
	return out
}
