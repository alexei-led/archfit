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

// CoChange maps a sorted file pair {a,b} (a<b) to the number of commits that
// touched both — the logical/temporal coupling signal.
type CoChange map[[2]string]int

// coChangeMaxFiles caps the commit size considered for co-change: a sweeping
// refactor that touches hundreds of files is not evidence of pairwise coupling.
const coChangeMaxFiles = 50

// Churn returns per-file commit counts over the recent window (a volatility proxy).
func Churn(ctx context.Context, workDir string, runner toolrun.Runner) (FileChurn, diagnostic.Coverage, error) {
	churn, _, _, cov, err := History(ctx, workDir, "", runner)
	return churn, cov, err
}

// History returns per-file churn, pairwise co-change, and commit count over the
// recent commit window in a single `git log` pass. A non-git directory or any
// failure yields empty maps with absent coverage, never an error — this signal
// is best-effort. The commit window is bounded by count (deterministic for a
// fixed HEAD).
//
// When prefix is non-empty, the git log is scoped to that subtree path and only
// matching files are collected; their prefix component is stripped so returned
// paths are ScanRoot-relative. When prefix is "", behavior is byte-identical to
// the pre-prefix version.
func History(ctx context.Context, workDir, prefix string, runner toolrun.Runner) (FileChurn, CoChange, int, diagnostic.Coverage, error) {
	args := []string{"log", fmt.Sprintf("-n%d", churnCommits), "--name-only", "--pretty=format:@"}
	if prefix != "" {
		args = append(args, "--", prefix)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    args,
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if err != nil || out.ExitCode != 0 {
		return nil, nil, 0, diagnostic.Coverage{Tool: gitTool, Status: "absent"}, nil
	}

	subtree := ""
	if prefix != "" {
		subtree = prefix + "/"
	}

	churn := FileChurn{}
	coChange := CoChange{}
	totalCommits := 0
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
			totalCommits++
			continue
		}
		if f := strings.TrimSpace(line); f != "" {
			if subtree == "" {
				commit = append(commit, f)
			} else if rel, ok := strings.CutPrefix(f, subtree); ok {
				commit = append(commit, rel)
			}
		}
	}
	flush()

	status := "ok"
	if len(churn) == 0 {
		status = "absent"
	}
	return churn, coChange, totalCommits, diagnostic.Coverage{Tool: gitTool, Status: status, FilesSeen: len(churn)}, nil
}
