package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// The check compares the working tree with the merge base of HEAD and the
// target. These errors say why there is no base to compare with; the caller
// turns them into a status of unknown, never into a guess.
var (
	// ErrShallowClone: the clone's history is cut, so git can return a merge
	// base that is not the real one.
	ErrShallowClone = errors.New("shallow clone")
	// ErrNoMergeBase: HEAD and the target share no commit this clone holds.
	ErrNoMergeBase = errors.New("no merge base")
)

// BaseCommit returns the merge base of HEAD and target: the commit the check
// compares the working tree with. A shallow clone is refused even when git
// finds a merge base, because the base it finds may be the cut, not the fork.
// It runs: git rev-parse --is-shallow-repository and git merge-base HEAD <target>
func BaseCommit(ctx context.Context, dir, target string, runner toolrun.Runner) (string, error) {
	if err := checkRef("target", target); err != nil {
		return "", err
	}
	out, err := run(ctx, dir, runner, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return "", err
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("git rev-parse --is-shallow-repository exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	if strings.TrimSpace(string(out.Stdout)) == "true" {
		return "", fmt.Errorf("%w: the merge base with %s needs the full history (git fetch --unshallow)", ErrShallowClone, target)
	}
	sha, ok, err := MergeBase(ctx, dir, "HEAD", target, runner)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%w of HEAD and %s", ErrNoMergeBase, target)
	}
	return sha, nil
}

// TreeOf returns the id of the tree commit points to. Equal tree ids mean
// equal content, so the id keys the facts read from that tree.
// It runs: git rev-parse <commit>^{tree}
func TreeOf(ctx context.Context, dir, commit string, runner toolrun.Runner) (string, error) {
	// rev-parse prints an argument it reads as an option, and exits 0.
	if err := checkRef("commit", commit); err != nil {
		return "", err
	}
	return runGitRevParse(ctx, dir, runner, []string{commit + "^{tree}"}, "rev-parse "+commit+"^{tree}")
}

// SnapshotTree writes the files of commit's tree into dest, which must be
// empty or absent. The repository's index, HEAD and working tree stay as they
// are: git reads the tree into a private temporary index and checks that index
// out under dest, so git itself checks every path and writes symlinks and the
// executable bit. Checkout filters (end of line, smudge) apply as they would
// in a working tree; export attributes (export-ignore, export-subst) do not,
// because they shape an archive, not the code.
// It runs: git read-tree <commit> and git checkout-index --all --prefix=<dest>/
func SnapshotTree(ctx context.Context, dir, commit, dest string, runner toolrun.Runner) error {
	// git read-tree --empty succeeds: an option would give an empty base.
	if err := checkRef("commit", commit); err != nil {
		return err
	}
	root, err := RepoRoot(ctx, dir, runner)
	if err != nil {
		return err
	}
	prefix, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("snapshot directory: %w", err)
	}
	tmp, err := os.MkdirTemp("", "archfit-tree-index-*")
	if err != nil {
		return fmt.Errorf("snapshot index: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := runTree(ctx, root, runner, env, "read-tree", commit); err != nil {
		return err
	}
	_, err = runTree(ctx, root, runner, env, "checkout-index", "--all", "--prefix="+prefix+string(filepath.Separator))
	return err
}

// checkRef refuses a ref that starts with a dash: git reads it as an option.
func checkRef(kind, ref string) error {
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid %s %q: a ref may not start with '-'", kind, ref)
	}
	return nil
}

// runTree runs a git command that reads or writes a whole tree, with env
// appended, and returns its stdout untrimmed; a non-zero exit is an error. It
// has no timeout of its own, as AddWorktree has none: its time grows with the
// repository, so the caller's context bounds it.
func runTree(ctx context.Context, dir string, runner toolrun.Runner, env []string, args ...string) ([]byte, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitTool, Args: args, Env: env, WorkDir: dir})
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if out.ExitCode != 0 {
		return nil, fmt.Errorf("git %s exited %d: %s", strings.Join(args, " "), out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return out.Stdout, nil
}
