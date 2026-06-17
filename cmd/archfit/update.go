package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
)

// UpdateCmd syncs .archfit.yaml with the current project structure.
type UpdateCmd struct {
	Config      string `short:"c" help:"Config file path." default:".archfit.yaml"`
	Root        string `short:"r" help:"Project root directory (default: directory of --config)."`
	LLM         bool   `name:"llm"          help:"Run LLM classification for unclassified modules (off-gate)."`
	Apply       bool   `name:"apply"        help:"Write structural and classification changes live into .archfit.yaml (backup created; existing fields never overwritten)."`
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
	existingByName := indexExisting(existing)

	freshCfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}

	report := initcfg.DiffModules(existing, freshCfg.Modules)
	addedNames := addedSet(report.Added)

	ann, err := c.maybeClassify(ctx, cfg, root, report, addedNames)
	if err != nil {
		return err
	}

	hasEdits := hasActionableEdits(report, ann, existingByName, addedNames, cfg.Layers)

	if !c.Apply {
		_, _ = fmt.Fprint(deps.Stdout, initcfg.RenderUpdateReport(report, ann, cfg.Layers))
		return nil
	}

	if !hasEdits {
		_, _ = fmt.Fprintln(deps.Stdout, "structurally in sync — no changes to apply")
		return nil
	}

	edits := buildUpdateEdits(report, ann, existingByName, cfg.Layers, addedNames)
	edited, err := initcfg.ApplyEdits(originalBytes, edits)
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}
	if err := safeWriteConfig(ctx, deps, c.Config, edited, originalBytes); err != nil {
		return err
	}
	if len(report.PathDrift) > 0 {
		_, _ = fmt.Fprintln(deps.Stdout, "note: module paths replaced with discovered paths")
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

	ann, err := classifyModules(ctx, p, classifyTargets, cfg.Layers)
	if err != nil {
		return nil, &exitError{code: 3, msg: fmt.Sprintf("error: classify failed: %v", err)}
	}
	return ann, nil
}

// buildLLMProvider constructs the LLM provider, honouring the test seam and cache flag.
func (c *UpdateCmd) buildLLMProvider(cfg config.Config) (llm.Provider, error) {
	if c.providerOverride != nil {
		return c.providerOverride, nil
	}

	var llmCfg config.LLMConfig
	if lc, ok := cfg.LLM(); ok {
		llmCfg = lc
	}
	llmCfg.Provider = c.LLMProvider
	llmCfg.Model = c.LLMModel

	p, err := buildProvider(llmCfg)
	if err != nil {
		return nil, &exitError{code: 3, msg: fmt.Sprintf("error: %v (set the key and re-run; see `archfit doctor`)", err)}
	}
	if !c.NoCache {
		cacheDir := filepath.Join(filepath.Dir(c.Config), ".archfit-cache", "llm")
		p = llm.NewCache(p, cacheDir)
	}
	return p, nil
}

// classifyTargetsForUpdate collects ModuleDefs to classify: Added modules from discovery,
// plus existing unclassified modules (excluding those already in addedNames).
func classifyTargetsForUpdate(
	cfg config.Config,
	report initcfg.UpdateReport,
	addedNames map[string]struct{},
) []initcfg.ModuleDef {
	targets := make([]initcfg.ModuleDef, 0, len(report.Added)+len(report.Unclassified))
	targets = append(targets, report.Added...)
	for _, name := range report.Unclassified {
		if _, isAdded := addedNames[name]; isAdded {
			continue
		}
		if def, ok := cfg.Modules[name]; ok {
			targets = append(targets, initcfg.ModuleDef{Name: name, Paths: def.Paths})
		}
	}
	return targets
}

// hasActionableEdits returns true when there is at least one structural change or
// a field-fill that survives the absent+valid filter for an existing unclassified module.
func hasActionableEdits(
	report initcfg.UpdateReport,
	ann map[string]initcfg.ModuleAnnotation,
	existingByName map[string]initcfg.ExistingModule,
	addedNames map[string]struct{},
	layers []string,
) bool {
	if !report.StructuralInSync {
		return true
	}
	if ann == nil {
		return false
	}
	for _, name := range report.Unclassified {
		if _, isAdded := addedNames[name]; isAdded {
			continue
		}
		a, ok := ann[name]
		if !ok {
			continue
		}
		e, eok := existingByName[name]
		if !eok {
			continue
		}
		if fieldFillSurvives(e, a, layers) {
			return true
		}
	}
	return false
}

// fieldFillSurvives reports whether at least one absent field has a valid annotation value.
func fieldFillSurvives(e initcfg.ExistingModule, a initcfg.ModuleAnnotation, layers []string) bool {
	return len(absentFields(e, a, layers)) > 0
}

// buildUpdateEdits constructs the ordered Edit slice for an apply pass.
func buildUpdateEdits(
	report initcfg.UpdateReport,
	ann map[string]initcfg.ModuleAnnotation,
	existingByName map[string]initcfg.ExistingModule,
	layers []string,
	addedNames map[string]struct{},
) []initcfg.Edit {
	edits := make([]initcfg.Edit, 0, len(report.Added)+len(report.PathDrift)+len(report.Removed)+len(report.Unclassified))

	for _, def := range report.Added {
		var annPtr *initcfg.ModuleAnnotation
		if ann != nil {
			if a, ok := ann[def.Name]; ok {
				a := a
				annPtr = &a
			}
		}
		edits = append(edits, initcfg.AddModuleEdit{Def: def, Ann: annPtr})
	}

	for _, d := range report.PathDrift {
		edits = append(edits, initcfg.UpdateModulePathsEdit{Module: d.Name, Paths: d.DiscoveredPaths})
	}

	for _, e := range report.Removed {
		edits = append(edits, initcfg.CommentModuleEdit{Module: e.Name, Note: "not found in discovery"})
	}

	edits = append(edits, buildFieldFillEdits(report.Unclassified, ann, existingByName, layers, addedNames)...)
	return edits
}

// buildFieldFillEdits builds SetModuleFields edits for existing unclassified modules.
func buildFieldFillEdits(
	unclassified []string,
	ann map[string]initcfg.ModuleAnnotation,
	existingByName map[string]initcfg.ExistingModule,
	layers []string,
	addedNames map[string]struct{},
) []initcfg.Edit {
	if ann == nil {
		return nil
	}
	edits := make([]initcfg.Edit, 0, len(unclassified))
	for _, name := range unclassified {
		if _, isAdded := addedNames[name]; isAdded {
			continue
		}
		a, ok := ann[name]
		if !ok {
			continue
		}
		e, eok := existingByName[name]
		if !eok {
			continue
		}
		fields := absentFields(e, a, layers)
		if len(fields) == 0 {
			continue
		}
		edits = append(edits, initcfg.SetModuleFieldsEdit{Module: name, Fields: fields})
	}
	return edits
}

// absentFields returns the field map for SetModuleFieldsEdit, containing only fields
// that are absent in the existing module and have a valid annotation value.
func absentFields(e initcfg.ExistingModule, a initcfg.ModuleAnnotation, layers []string) map[initcfg.ModuleField]string {
	fields := make(map[initcfg.ModuleField]string)
	if !e.HasSubdomain && a.Subdomain != "" {
		fields[initcfg.FieldSubdomain] = a.Subdomain
	}
	if !e.HasVolatility && a.Volatility != "" {
		fields[initcfg.FieldVolatility] = a.Volatility
	}
	if !e.HasLayer && a.Layer != "" && layerInSet(a.Layer, layers) {
		fields[initcfg.FieldLayer] = a.Layer
	}
	return fields
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

// indexExisting builds a name→ExistingModule map.
func indexExisting(existing []initcfg.ExistingModule) map[string]initcfg.ExistingModule {
	m := make(map[string]initcfg.ExistingModule, len(existing))
	for _, e := range existing {
		m[e.Name] = e
	}
	return m
}

// addedSet builds a set of Added module names.
func addedSet(added []initcfg.ModuleDef) map[string]struct{} {
	s := make(map[string]struct{}, len(added))
	for _, a := range added {
		s[a.Name] = struct{}{}
	}
	return s
}

// layerInSet reports whether layer is in the allowed layers slice.
func layerInSet(layer string, layers []string) bool {
	for _, l := range layers {
		if l == layer {
			return true
		}
	}
	return false
}
