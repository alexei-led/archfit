package git

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// ChangeSet holds the sorted list of files that differ between two refs.
type ChangeSet struct {
	Files []string
	Base  string
	Head  string
}

const (
	gitTool    = "git"
	gitTimeout = 10 * time.Second
)

// Changed returns the sorted list of files that differ between base and head.
// It runs: git diff --name-only <base>..<head> [-- <prefix>]
// When prefix is non-empty, only paths under that subtree are returned and the
// prefix component is stripped, so returned paths are ScanRoot-relative.
// When prefix is empty, behavior is byte-identical to the pre-prefix version.
func Changed(ctx context.Context, workDir, base, head, prefix string, runner toolrun.Runner) (ChangeSet, error) {
	args := []string{"diff", "--name-only", base + ".." + head}
	if prefix != "" {
		args = append(args, "--", prefix)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    args,
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if err != nil {
		return ChangeSet{}, fmt.Errorf("git diff: %w", err)
	}
	if out.ExitCode != 0 {
		return ChangeSet{}, fmt.Errorf("git diff exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	raw := strings.TrimRight(string(out.Stdout), "\n")
	var files []string
	if raw != "" {
		files = strings.Split(raw, "\n")
	}
	files = rebaseToSubtree(prefix, files)
	sort.Strings(files)

	return ChangeSet{Files: files, Base: base, Head: head}, nil
}

// rebaseToSubtree keeps only paths starting with prefix+"/" and strips that
// prefix so the returned paths are ScanRoot-relative.
// Returns paths unchanged when prefix is "".
func rebaseToSubtree(prefix string, paths []string) []string {
	if prefix == "" {
		return paths
	}
	sep := prefix + "/"
	var out []string
	for _, p := range paths {
		if rel, ok := strings.CutPrefix(p, sep); ok {
			out = append(out, rel)
		}
	}
	return out
}

// HeadRef returns the SHA of HEAD.
// It runs: git rev-parse HEAD
func HeadRef(ctx context.Context, workDir string, runner toolrun.Runner) (string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"rev-parse", "HEAD"},
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("git rev-parse HEAD exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	return strings.TrimSpace(string(out.Stdout)), nil
}

// RepoRoot returns the absolute path to the repository root.
// It runs: git rev-parse --show-toplevel
func RepoRoot(ctx context.Context, workDir string, runner toolrun.Runner) (string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"rev-parse", "--show-toplevel"},
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("git rev-parse --show-toplevel exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	return strings.TrimSpace(string(out.Stdout)), nil
}
