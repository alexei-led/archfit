package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/llm"
)

// UpdateCmd syncs .archfit.yaml with the current project structure.
type UpdateCmd struct {
	Config     string `short:"c" help:"Config file path." default:".archfit.yaml"`
	Root       string `short:"r" help:"Project root directory (default: directory of --config)."`
	AIClassify bool   `name:"ai-classify" help:"Run AI classification for unclassified modules (off-gate)."`
	Apply      bool   `name:"apply" help:"Write structural changes live into .archfit.yaml (backup created; AI semantic proposals remain review-only)."`
	JSON       bool   `name:"json" help:"Emit the review as a JSON document (report-only; not combinable with --apply, --ai-classify, or --refresh)."`
	Refresh    bool   `name:"refresh" help:"Re-run the AI calls and refresh the cache."`
	AIProvider string `name:"ai-provider" help:"AI provider override." default:"anthropic"`
	AIModel    string `name:"ai-model" help:"AI model override." default:"claude-opus-4-8"`

	// providerOverride is a test seam — set directly on the struct to inject a fake provider.
	providerOverride llm.Provider
}

func (c *UpdateCmd) Run(deps *appDeps) error {
	root, err := c.resolveRoot()
	if err != nil {
		return err
	}
	adapter := newConfigUpdateAdapter(c, deps)
	service := application.ConfigUpdateService{
		Configs: adapter, Files: adapter, Discovery: adapter, Projection: adapter,
		Classifier: adapter, Reviewer: adapter, Editor: adapter, Writer: adapter,
	}
	out, err := service.Execute(context.Background(), application.ConfigUpdateRequest{
		ConfigPath: c.Config, Root: root, AIClassify: c.AIClassify,
		Apply: c.Apply, JSON: c.JSON, Refresh: c.Refresh,
	})
	if err != nil {
		var invalid *application.InvalidConfigUpdateRequestError
		if errors.As(err, &invalid) {
			return &exitError{code: 3, msg: invalid.Error()}
		}
		return err
	}
	if len(out.MissingClassifications) > 0 {
		_, _ = fmt.Fprintf(deps.Stdout, "warning: LLM did not classify %d module(s): %s — they were left unclassified\n",
			len(out.MissingClassifications), strings.Join(out.MissingClassifications, ", "))
	}
	switch out.Action {
	case application.ConfigUpdateNoChanges:
		_, _ = fmt.Fprintln(deps.Stdout, "structurally in sync — no changes to apply")
	case application.ConfigUpdateApplied:
		if out.PathDrift {
			_, _ = fmt.Fprintln(deps.Stdout, "note: module paths replaced with discovered paths")
		}
		_, _ = deps.Stdout.Write(out.Output)
	default:
		if _, writeErr := deps.Stdout.Write(out.Output); writeErr != nil && c.JSON {
			return &exitError{code: 3, msg: fmt.Sprintf("error: encoding config review: %v", writeErr)}
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
