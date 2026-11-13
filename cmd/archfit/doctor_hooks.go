package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// errSharedHooksDir marks a hooks directory outside the repository, such as a
// global core.hooksPath: a hook written there runs in every repository that
// uses it, so doctor --fix does not write one.
var errSharedHooksDir = errors.New("git runs the hooks of this repository from a directory outside it")

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
		case errors.Is(err, errForeignHook), errors.Is(err, errSharedHooksDir):
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
	if _, err := os.Stat(filepath.Join(root, defaultConfigPath)); errors.Is(err, fs.ErrNotExist) {
		_, _ = fmt.Fprintf(deps.Stdout, "  warning: %s does not exist: the hooks read the policy there; run archfit init\n", filepath.Join(root, defaultConfigPath))
	}
	return errors.Join(failed...)
}

// installHarness writes the hooks of one harness under root and returns the
// file. With --dry-run it only names the file.
func (c *DoctorCmd) installHarness(ctx context.Context, deps *appDeps, root, harness string) (string, error) {
	if harness == harnessGit {
		if err := hooksInRepository(ctx, deps, root); err != nil {
			return "", err
		}
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

// hooksInRepository returns errSharedHooksDir when the hooks directory git
// uses for root lies outside the repository and its git directory.
func hooksInRepository(ctx context.Context, deps *appDeps, root string) error {
	hooks, err := gitHooksDir(ctx, deps.Runner, root)
	if err != nil {
		return err
	}
	out, err := deps.Runner.Run(ctx, toolrun.ToolCmd{
		Name: gitBinary, Args: []string{gitRevParse, "--path-format=absolute", "--git-common-dir"}, WorkDir: root,
	})
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return fmt.Errorf("git rev-parse --git-common-dir exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	for _, dir := range []string{root, strings.TrimSpace(string(out.Stdout))} {
		if within(dir, hooks) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s (core.hooksPath); doctor does not write there: add %q to its pre-commit hook",
		errSharedHooksDir, hooks, hookRunLine(hookEventPreCommit, "", defaultConfigPath))
}

// within reports whether path lies in dir, with symlinks resolved on both,
// also in the part of path that exists when the rest does not yet.
func within(dir, path string) bool {
	rel, err := filepath.Rel(resolveExisting(dir), resolveExisting(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExisting resolves the symlinks of the longest part of path that
// exists and keeps the rest as written: a hooks directory not created yet
// still lands where its symlinked parent points.
func resolveExisting(path string) string {
	rest := ""
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, rest)
		}
		if filepath.Dir(p) == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}
