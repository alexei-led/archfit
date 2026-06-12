package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/initcfg"
)

// InitCmd discovers project structure and writes a starter archfit.yaml.
type InitCmd struct {
	Root   string `short:"r" help:"Project root directory." default:"."`
	Output string `short:"o" help:"Output file (use '-' for stdout)." default:".archfit.yaml"`
}

func (c *InitCmd) Run(deps *appDeps) error {
	root := c.Root
	if !filepath.IsAbs(root) {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolving root: %w", err)
		}
	}
	ctx := context.Background()
	cfg, err := initcfg.Discover(ctx, root, deps.Runner)
	if err != nil {
		return fmt.Errorf("discovering project structure: %w", err)
	}
	yaml := initcfg.Render(cfg)
	if c.Output == "-" {
		_, _ = fmt.Fprint(deps.Stdout, yaml)
		return nil
	}
	if err := os.WriteFile(c.Output, []byte(yaml), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.Output, err)
	}
	_, _ = fmt.Fprintf(deps.Stdout, "wrote %s\n", c.Output)
	return nil
}
