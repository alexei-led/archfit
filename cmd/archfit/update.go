package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	if c.LLM && ann != nil {
		warnTargets := initcfg.BuildClassifyTargets(root, classifyTargetsForUpdate(cfg, report, addedNames))
		warnPartialClassify(deps.Stdout, warnTargets, ann)
	}

	hasEdits := hasActionableEdits(report)

	if !c.Apply {
		_, _ = fmt.Fprint(deps.Stdout, initcfg.RenderUpdateReport(report, ann, cfg.Layers))
		return nil
	}

	if !hasEdits {
		if c.LLM && ann != nil {
			_, _ = fmt.Fprint(deps.Stdout, initcfg.RenderUpdateReport(report, ann, cfg.Layers))
			return nil
		}
		_, _ = fmt.Fprintln(deps.Stdout, "structurally in sync — no changes to apply")
		return nil
	}

	edits := buildUpdateEdits(report)
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

	ann, err := classifyModulesWithEvidence(ctx, p, classifyTargets, cfg.Layers, collectUpdateRepoEvidence(root))
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
			targets = append(targets, initcfg.ModuleDef{Name: name, Paths: def.Paths, Public: def.Public})
		} // name came from DiffModules over cfg.Modules, so absent is purely defensive
	}
	return targets
}

// hasActionableEdits returns true when there is at least one structural change.
// LLM role/volatility output is review-only: it is rendered as a diff but never
// written by config update --apply.
func hasActionableEdits(report initcfg.UpdateReport) bool {
	return !report.StructuralInSync
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

const maxUpdateRepoEvidence = 20

// collectUpdateRepoEvidence gathers lightweight review evidence for the update
// LLM prompt. Failures are ignored; module names and paths still carry the prompt.
func collectUpdateRepoEvidence(root string) []string {
	var evidence []string
	addHeadings := func(label, path string) {
		if len(evidence) >= maxUpdateRepoEvidence {
			return
		}
		data, err := os.ReadFile(path) //nolint:gosec // local target repo evidence
		if err != nil {
			return
		}
		for _, h := range markdownHeadings(string(data), maxUpdateRepoEvidence-len(evidence)) {
			evidence = append(evidence, label+": "+h)
		}
	}

	for _, name := range []string{"README.md", "README"} {
		addHeadings(name, filepath.Join(root, name))
		if len(evidence) > 0 {
			break
		}
	}

	docsDir := filepath.Join(root, "docs")
	_ = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || len(evidence) >= maxUpdateRepoEvidence {
			return filepath.SkipAll
		}
		if d.IsDir() {
			if path != docsDir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			addHeadings(filepath.ToSlash(rel), path)
		}
		return nil
	})

	sort.Strings(evidence)
	if len(evidence) > maxUpdateRepoEvidence {
		evidence = evidence[:maxUpdateRepoEvidence]
	}
	return evidence
}

func markdownHeadings(text string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var headings []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if heading == "" {
			continue
		}
		headings = append(headings, heading)
		if len(headings) >= limit {
			break
		}
	}
	return headings
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
