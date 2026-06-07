package git

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// ChangeSet holds the sorted list of files that differ between two refs.
type ChangeSet struct {
	Files []string
	Base  string
	Head  string
}

// ChurnStats is a Phase 1 stub for future git-churn metrics.
type ChurnStats struct{}

const (
	gitTool    = "git"
	gitTimeout = 10 * time.Second
)

// Changed returns the sorted list of files that differ between base and head.
// It runs: git diff --name-only <base>..<head>
func Changed(ctx context.Context, base, head string, runner toolrun.Runner) (ChangeSet, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"diff", "--name-only", base + ".." + head},
		Timeout: gitTimeout,
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
	sort.Strings(files)

	return ChangeSet{Files: files, Base: base, Head: head}, nil
}

// HeadRef returns the SHA of HEAD.
// It runs: git rev-parse HEAD
func HeadRef(ctx context.Context, runner toolrun.Runner) (string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"rev-parse", "HEAD"},
		Timeout: gitTimeout,
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
func RepoRoot(ctx context.Context, runner toolrun.Runner) (string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"rev-parse", "--show-toplevel"},
		Timeout: gitTimeout,
	})
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("git rev-parse --show-toplevel exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	return strings.TrimSpace(string(out.Stdout)), nil
}

// Churn is a Phase 1 stub — returns empty stats and a coverage record.
func Churn(_ context.Context, _ toolrun.Runner) (ChurnStats, diagnostic.Coverage, error) {
	return ChurnStats{}, diagnostic.Coverage{Tool: gitTool, Status: "stub"}, nil
}
