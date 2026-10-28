package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Exit code git rev-parse, merge-base and cat-file use for "no such object" or
// "no common ancestor", as opposed to a failure to run.
const gitNotFound = 1

// run executes git in dir and returns its output. A failure to start is an
// error; a non-zero exit code is returned for the caller to judge.
func run(ctx context.Context, dir string, runner toolrun.Runner, args ...string) (toolrun.Output, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitTool, Args: args, Timeout: gitTimeout, WorkDir: dir})
	if err != nil {
		return toolrun.Output{}, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// InRepository reports whether dir is inside a git work tree.
// It runs: git rev-parse --is-inside-work-tree
func InRepository(ctx context.Context, dir string, runner toolrun.Runner) bool {
	out, err := run(ctx, dir, runner, "rev-parse", "--is-inside-work-tree")
	return err == nil && out.ExitCode == 0 && strings.TrimSpace(string(out.Stdout)) == "true"
}

// Lookup resolves ref to a commit SHA. ok is false when ref names no commit
// here (an unborn HEAD, a branch that was not fetched); err is set only when git
// cannot run.
// It runs: git rev-parse --verify --quiet <ref>^{commit}
func Lookup(ctx context.Context, dir, ref string, runner toolrun.Runner) (sha string, ok bool, err error) {
	out, err := run(ctx, dir, runner, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", false, err
	}
	if out.ExitCode != 0 {
		return "", false, nil
	}
	return strings.TrimSpace(string(out.Stdout)), true, nil
}

// OriginHead returns the remote's default branch as a remote-tracking ref such
// as "origin/main", when the clone recorded one.
// It runs: git symbolic-ref --quiet --short refs/remotes/origin/HEAD
func OriginHead(ctx context.Context, dir string, runner toolrun.Runner) (string, bool) {
	out, err := run(ctx, dir, runner, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil || out.ExitCode != 0 {
		return "", false
	}
	ref := strings.TrimSpace(string(out.Stdout))
	return ref, ref != ""
}

// MergeBase returns the best common ancestor of a and b. ok is false when the
// two share no history this clone holds, for example in a shallow clone.
// It runs: git merge-base <a> <b>
func MergeBase(ctx context.Context, dir, a, b string, runner toolrun.Runner) (sha string, ok bool, err error) {
	out, err := run(ctx, dir, runner, "merge-base", a, b)
	if err != nil {
		return "", false, err
	}
	if out.ExitCode == gitNotFound {
		return "", false, nil
	}
	if out.ExitCode != 0 {
		return "", false, fmt.Errorf("git merge-base exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return strings.TrimSpace(string(out.Stdout)), true, nil
}

// IsAncestor reports whether a is an ancestor of b, or equal to it.
// It runs: git merge-base --is-ancestor <a> <b>
func IsAncestor(ctx context.Context, dir, a, b string, runner toolrun.Runner) (bool, error) {
	out, err := run(ctx, dir, runner, "merge-base", "--is-ancestor", a, b)
	if err != nil {
		return false, err
	}
	switch out.ExitCode {
	case 0:
		return true, nil
	case gitNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("git merge-base --is-ancestor exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
}

// FileAt returns the bytes of path at rev, read as a git object. exists is
// false when rev has no such file. path is relative to the repository root.
// It runs: git cat-file blob <rev>:<path>
func FileAt(ctx context.Context, dir, rev, path string, runner toolrun.Runner) (content []byte, exists bool, err error) {
	spec := rev + ":" + path
	probe, err := run(ctx, dir, runner, "cat-file", "-e", spec)
	if err != nil {
		return nil, false, err
	}
	if probe.ExitCode != 0 {
		return nil, false, nil
	}
	out, err := run(ctx, dir, runner, "cat-file", "blob", spec)
	if err != nil {
		return nil, false, err
	}
	if out.ExitCode != 0 {
		return nil, false, fmt.Errorf("git cat-file blob %s exited %d: %s", spec, out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return out.Stdout, true, nil
}
