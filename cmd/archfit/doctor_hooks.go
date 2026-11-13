package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
)

// noHarness is what doctor --fix prints when it finds no harness to install
// hooks for. It writes nothing then.
const noHarness = "no harness detected: no hook installed (the hooks need a git repository)"

// installHooks is the hook part of doctor --fix (design 10.1, 11.3): it
// writes the hooks of every harness detected in the repository that holds the
// working directory, at its root. A pre-commit hook archfit did not write
// stays; doctor says what to add to it. Outside a git repository there is no
// harness, and nothing is written.
func (c *DoctorCmd) installHooks(ctx context.Context, deps *appDeps) error {
	_, _ = fmt.Fprintln(deps.Stdout, "\ninstalling hooks:")
	root, err := historygit.RepoRoot(ctx, ".", deps.Runner)
	if err != nil {
		_, _ = fmt.Fprintln(deps.Stdout, "  "+noHarness)
		return nil
	}
	var failed []error
	for _, harness := range []string{harnessClaude, harnessCodex, harnessGit} {
		if harness != harnessGit {
			if detected, _ := detectHarness(ctx, deps, root, harness); !detected {
				continue
			}
		}
		path, err := c.installHarness(ctx, deps, root, harness)
		switch {
		case errors.Is(err, errForeignHook):
			_, _ = fmt.Fprintf(deps.Stdout, "  warning: %v\n", err)
			continue
		case err != nil:
			_, _ = fmt.Fprintf(deps.Stdout, "  %s: failed: %v\n", harness, err)
			failed = append(failed, fmt.Errorf("%s hooks: %w", harness, err))
			continue
		}
		verb := "wrote"
		if c.DryRun {
			verb = "would write"
		}
		_, _ = fmt.Fprintf(deps.Stdout, "  %s %s\n", verb, path)
		if harness == harnessGit {
			continue
		}
		if gitIgnores(ctx, deps, root, harnessConfigFile(harness)) {
			_, _ = fmt.Fprintln(deps.Stdout, "  "+ignoredWarning(path))
		}
		if harness == harnessCodex {
			_, _ = fmt.Fprintln(deps.Stdout, "  "+codexTrustNotice)
		}
	}
	return errors.Join(failed...)
}

// installHarness writes the hooks of one harness under root and returns the
// file. With --dry-run it only names the file.
func (c *DoctorCmd) installHarness(ctx context.Context, deps *appDeps, root, harness string) (string, error) {
	if harness == harnessGit {
		if c.DryRun {
			return gitHookTarget(ctx, deps.Runner, root, defaultConfigPath)
		}
		return installGitHook(ctx, deps.Runner, root, defaultConfigPath)
	}
	if c.DryRun {
		return filepath.Join(root, harnessConfigFile(harness)), nil
	}
	return installHarnessHooks(root, harness, defaultConfigPath)
}
