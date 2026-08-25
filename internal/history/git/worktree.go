package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// git command + subcommand names used by the worktree helpers.
const (
	gitBinary   = "git"
	gitWorktree = "worktree"
)

// gitEnvVars lists environment variables that redirect git's internal state.
// When archfit is invoked by a CI system or git hook that sets these, inheriting
// them into worktree add/remove commands would make git operate on the wrong
// repository. Scrub all of them from the subprocess environment.
var gitEnvVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
}

// CleanEnv returns os.Environ() with git-redirect variables removed.
func CleanEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	scrub := make(map[string]bool, len(gitEnvVars))
	for _, k := range gitEnvVars {
		scrub[k] = true
	}
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !scrub[key] {
			out = append(out, kv)
		}
	}
	return out
}

// WorktreesDir returns the --base worktree parent directory under baseDir.
func WorktreesDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "worktrees")
}

// WorktreeParent picks the parent directory for the base-side worktree
// (wt/ is created inside it and the whole directory is removed after the run).
// The returned release function must be called after cleanup.
//
// With the fact cache on, the path is a deterministic function of the resolved
// base commit SHA: `<gitRoot>/.archfit-cache/worktrees/<sha>`. The base tree
// is immutable, and the per-checkout fact-cache keys fold the scan-root path in
// (cached subprocess output embeds absolute paths — see rust.cachedRunner /
// golang.memberKeys), so a repeat `--base <same-ref>` run reuses the same
// absolute root and every base-side extractor subprocess becomes a cache hit.
// A leftover checkout from a crashed run is removed and the path reused.
// Concurrent runs for the same SHA take an inter-process lock before
// removing/recreating the checkout, so one process cannot delete another
// process's live base tree. An unresolvable ref, or any cleanup/mkdir failure
// falls back to the historical random temp dir — correct, just uncached.
//
// The parent is derived from gitRoot, NOT from the config directory. A git
// worktree holds TRACKED files only, so every gitignored input an analyzer
// resolves through — node_modules, vendored or generated code — must come from
// the surrounding repo, which only works while the checkout sits inside it.
// Deriving from the config directory put the checkout outside the repo whenever
// the policy config lived elsewhere — exactly the CI layout `--root` advertises
// — and dependency-cruiser then reported the base side partial-unresolved
// against an ok head, filing genuinely introduced violations as unknown-origin.
func WorktreeParent(ctx context.Context, runner toolrun.Runner, gitRoot, baseRef string) (string, func(), error) {
	releaseNoop := func() {}
	sha, err := ResolveCommit(ctx, gitRoot, baseRef, runner)
	// gitRoot is already absolute (git rev-parse --show-toplevel); absolutize
	// anyway so the git subprocesses below (WorkDir: gitRoot) and the os calls
	// that create/remove the directory can never disagree about the path.
	parent, aerr := filepath.Abs(WorktreesDir(gitRoot))
	if err == nil && aerr == nil {
		dir := filepath.Join(parent, sha)
		release, lerr := lockWorktree(ctx, dir+".lock")
		if lerr == nil {
			RemoveWorktree(ctx, runner, gitRoot, filepath.Join(dir, "wt"))
			if rerr := os.RemoveAll(dir); rerr == nil {
				if merr := os.MkdirAll(dir, 0o750); merr == nil {
					return dir, release, nil
				}
			}
			release()
		} else if errors.Is(lerr, context.Canceled) || errors.Is(lerr, context.DeadlineExceeded) {
			return "", releaseNoop, lerr
		}
	}
	dir, err := os.MkdirTemp("", "archfit-base-*")
	return dir, releaseNoop, err
}

const worktreeLockPoll = 50 * time.Millisecond

func lockWorktree(ctx context.Context, lockPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //#nosec G304 -- internal cache lock path derived from gitRoot + resolved commit SHA
	if err != nil {
		return nil, err
	}
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(worktreeLockPoll):
		}
	}
}

// SnapToRoot rewrites headRoot so its gitRoot prefix uses gitRoot's canonical
// casing, so filepath.Rel works on case-insensitive filesystems (macOS APFS),
// mirroring scope.snapScanRoot. It walks headRoot upward and, when an ancestor is
// the same directory as gitRoot (device+inode via os.SameFile), rebuilds gitRoot
// plus the collected suffix. Returns headRoot unchanged when no ancestor matches.
func SnapToRoot(gitRoot, headRoot string) string {
	gitInfo, err := os.Stat(gitRoot)
	if err != nil {
		return headRoot
	}
	suffix := ""
	for cur := headRoot; ; {
		if info, statErr := os.Stat(cur); statErr == nil && os.SameFile(gitInfo, info) {
			return filepath.Join(gitRoot, suffix)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return headRoot // reached filesystem root without matching gitRoot
		}
		suffix = filepath.Join(filepath.Base(cur), suffix)
		cur = parent
	}
}

// SubtreeInWorktree maps headRoot (absolute, inside gitRoot) to its mirror
// inside wtDir. When headRoot == gitRoot the result is wtDir itself.
// Returns an error when headRoot is not under gitRoot (e.g. a ../ path).
func SubtreeInWorktree(gitRoot, headRoot, wtDir string) (string, error) {
	rel, err := filepath.Rel(gitRoot, headRoot)
	if err != nil {
		return "", fmt.Errorf("rel(%s, %s): %w", gitRoot, headRoot, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("--root %s is not under git root %s", headRoot, gitRoot)
	}
	if rel == "." {
		return wtDir, nil
	}
	return filepath.Join(wtDir, rel), nil
}

// AddWorktree runs `git worktree add --detach <dir> <ref>` from gitRoot via the
// Runner. GIT_DIR/GIT_WORK_TREE/etc. are scrubbed so CI/hook-set vars do not
// redirect the command to the wrong repository.
func AddWorktree(ctx context.Context, runner toolrun.Runner, gitRoot, dir, ref string) error {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitBinary,
		Args:    []string{gitWorktree, "add", "--detach", dir, ref},
		Env:     CleanEnv(),
		WorkDir: gitRoot,
	})
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		if msg := strings.TrimSpace(string(out.Stderr)); msg != "" {
			return fmt.Errorf("git worktree add: %s", msg)
		}
		return fmt.Errorf("git worktree add exited %d", out.ExitCode)
	}
	return nil
}

// RemoveWorktree removes the worktree registration cleanly via the Runner.
// Best-effort — errors are ignored; os.RemoveAll on the temp dir follows in the
// caller's defer.
func RemoveWorktree(ctx context.Context, runner toolrun.Runner, gitRoot, dir string) {
	env := CleanEnv()
	_, _ = runner.Run(ctx, toolrun.ToolCmd{
		Name: gitBinary, Args: []string{gitWorktree, "remove", "--force", dir},
		Env: env, WorkDir: gitRoot,
	})
	// Prune in case RemoveAll ran before the remove command.
	_, _ = runner.Run(ctx, toolrun.ToolCmd{
		Name: gitBinary, Args: []string{gitWorktree, "prune"},
		Env: env, WorkDir: gitRoot,
	})
}

// Worktree materialises a base ref in a clean detached worktree. It is the VCS
// adapter behind the application's base-tree comparison: the user's working
// tree is never mutated and every git subprocess goes through the Runner.
type Worktree struct{ Runner toolrun.Runner }

// Checkout resolves the repository from anchorDir, checks baseRef out into a
// temporary detached worktree, and returns the directory inside it that mirrors
// headRoot. cleanup is never nil once a temporary directory exists and must run
// on every path; it removes the worktree registration, the directory, and the
// inter-process lock.
func (w Worktree) Checkout(ctx context.Context, baseRef, anchorDir, headRoot string) (string, func(), error) {
	noop := func() {}
	gitAnchor := anchorDir
	if abs, err := filepath.Abs(gitAnchor); err == nil {
		gitAnchor = abs
	}
	gitRoot, err := RepoRoot(ctx, gitAnchor, w.Runner)
	if err != nil {
		return "", noop, fmt.Errorf("--base requires a git repository: %w", err)
	}

	// HEAD-side analysis boundary: --root when given, else the whole repo.
	headScanRoot := headRoot
	if headScanRoot == "" {
		headScanRoot = gitRoot
	} else if abs, aerr := filepath.Abs(headScanRoot); aerr == nil {
		headScanRoot = abs
	}
	// Canonicalize symlinks so filepath.Rel works on macOS (/var vs /private/var),
	// then snap a case-variant --root to gitRoot's canonical casing so the subtree
	// mapping works on case-insensitive filesystems.
	if canon, cerr := filepath.EvalSymlinks(headScanRoot); cerr == nil {
		headScanRoot = canon
	}
	headScanRoot = SnapToRoot(gitRoot, headScanRoot)

	tmpBase, release, err := WorktreeParent(ctx, w.Runner, gitRoot, baseRef)
	if err != nil {
		return "", noop, fmt.Errorf("create temp dir: %w", err)
	}
	wtDir := filepath.Join(tmpBase, "wt")
	cleanup := func() {
		RemoveWorktree(ctx, w.Runner, gitRoot, wtDir)
		_ = os.RemoveAll(tmpBase)
		release()
	}

	if aerr := AddWorktree(ctx, w.Runner, gitRoot, wtDir, baseRef); aerr != nil {
		return "", cleanup, fmt.Errorf("cannot create worktree for ref %q: %w", baseRef, aerr)
	}
	wtCanon, err := filepath.EvalSymlinks(wtDir)
	if err != nil {
		return "", cleanup, fmt.Errorf("eval worktree symlinks: %w", err)
	}
	baseRoot, err := SubtreeInWorktree(gitRoot, headScanRoot, wtCanon)
	if err != nil {
		return "", cleanup, fmt.Errorf("map subtree into worktree: %w", err)
	}
	return baseRoot, cleanup, nil
}
