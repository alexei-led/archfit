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

// FileChurn maps a repo-relative file path to the number of commits that touched
// it in the analysed window — a proxy for volatility (how often it changes).
type FileChurn map[string]int

const (
	gitTool    = "git"
	gitTimeout = 10 * time.Second
	// churnCommits bounds the churn window to the most recent N commits. A commit
	// count (not a wall-clock --since) keeps the result deterministic for a fixed
	// HEAD while still capturing recent change activity, and keeps git log cheap.
	churnCommits = 1000
)

// Changed returns the sorted list of files that differ between base and head.
// It runs: git diff --name-only <base>..<head>
func Changed(ctx context.Context, workDir, base, head string, runner toolrun.Runner) (ChangeSet, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"diff", "--name-only", base + ".." + head},
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
	sort.Strings(files)

	return ChangeSet{Files: files, Base: base, Head: head}, nil
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

// CoChange maps a sorted file pair {a,b} (a<b) to the number of commits that
// touched both — the logical/temporal coupling signal.
type CoChange map[[2]string]int

// coChangeMaxFiles caps the commit size considered for co-change: a sweeping
// refactor that touches hundreds of files is not evidence of pairwise coupling.
const coChangeMaxFiles = 50

// Churn returns per-file commit counts over the recent window (a volatility proxy).
func Churn(ctx context.Context, workDir string, runner toolrun.Runner) (FileChurn, diagnostic.Coverage, error) {
	churn, _, cov, err := History(ctx, workDir, runner)
	return churn, cov, err
}

// History returns per-file churn and pairwise co-change over the recent commit
// window in a single `git log` pass. A non-git directory or any failure yields
// empty maps with absent coverage, never an error — this signal is best-effort.
// The commit window is bounded by count (deterministic for a fixed HEAD).
func History(ctx context.Context, workDir string, runner toolrun.Runner) (FileChurn, CoChange, diagnostic.Coverage, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    []string{"log", fmt.Sprintf("-n%d", churnCommits), "--name-only", "--pretty=format:@"},
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if err != nil || out.ExitCode != 0 {
		return nil, nil, diagnostic.Coverage{Tool: gitTool, Status: "absent"}, nil
	}

	churn := FileChurn{}
	coChange := CoChange{}
	var commit []string
	flush := func() {
		for _, f := range commit {
			churn[f]++
		}
		if len(commit) > 1 && len(commit) <= coChangeMaxFiles {
			sort.Strings(commit)
			for i := 0; i < len(commit); i++ {
				for j := i + 1; j < len(commit); j++ {
					coChange[[2]string{commit[i], commit[j]}]++
				}
			}
		}
		commit = commit[:0]
	}
	for _, line := range strings.Split(string(out.Stdout), "\n") {
		if line == "@" {
			flush()
			continue
		}
		if f := strings.TrimSpace(line); f != "" {
			commit = append(commit, f)
		}
	}
	flush()

	status := "ok"
	if len(churn) == 0 {
		status = "absent"
	}
	return churn, coChange, diagnostic.Coverage{Tool: gitTool, Status: status, FilesSeen: len(churn)}, nil
}
