// Package main — worktree helper for `analyze --base`.
//
// scoreBaseRef checks <base-ref> out into a clean detached git worktree, scores
// it with the full pipeline, and returns the base Scorecard plus the narrow
// baseEvidence the git-origin delta needs. analyze attaches the resulting
// before/after delta as a section of the HEAD decision report, so --base renders
// through the SAME pipeline and output format as a normal run (consistent JSON
// schema; advisory output and required-tool enforcement are honoured exactly as
// the caller set them).
//
// Invariants:
//   - The user's working tree is never mutated.
//   - Cleanup runs even on error paths (deferred).
//   - Non-git directory or missing/bad ref → exit 3.
//   - The base side measures the caller's EFFECTIVE head config (--lang and
//     --min-severity overrides included) and never reparses the file, so config
//     drift can never masquerade as code drift.
//   - The base Diagnostic never leaves this file: runScoreSide projects it to
//     baseEvidence at the source. Its agent tasks carry a validation command and
//     paths rooted in the temporary worktree, which is deleted on return.
//   - All git subprocesses go through deps.Runner (toolrun) — no os/exec here.
package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/score"
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

// cleanGitEnv returns os.Environ() with git-redirect variables removed.
func cleanGitEnv() []string {
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

// baseEvidence is the ONLY data allowed to cross from the base sub-run into
// head output: observed finding IDs, analyzer coverage, and the config hash.
// Base task paths, locations, validation commands, and declarations are dropped
// at the source — the base run is scored inside a temporary worktree that no
// longer exists when the head report is read, so any path from it is a lie.
type baseEvidence struct {
	// FindingIDs are the base run's observed finding IDs, sorted, with fixed
	// entries removed.
	FindingIDs []string
	// Coverage and CoverageGaps are the base run's analyzer coverage evidence.
	Coverage     []diagnostic.Coverage
	CoverageGaps []diagnostic.CoverageGap
	// ConfigHash is the base run's effective config hash.
	ConfigHash string
}

// withIndependentModules returns cfg with a private copy of its Modules map.
// config.Config is passed by value but the map is shared, and runPipeline fills
// missing owners and deploy units through it. The base sub-run must resolve both
// from its OWN tree, so callers snapshot the effective config before the head
// run mutates the shared map. Everything else in Config is read-only during a
// run and is safe to share.
func withIndependentModules(cfg config.Config) config.Config {
	modules := make(map[string]module.ModuleDef, len(cfg.Modules))
	maps.Copy(modules, cfg.Modules)
	cfg.Modules = modules
	return cfg
}

// scoreBaseRef checks baseRef out into a temporary detached worktree, scores it
// with the full pipeline (advisory per the caller), and returns the base
// Scorecard plus its baseEvidence. The worktree is always removed. advisory
// mirrors the HEAD side so the delta compares like with like. head is the HEAD
// run context: its config source, bundle directory, and evaluation time carry
// over unchanged; only the scan root is swapped for the base worktree. cfg is
// the caller's EFFECTIVE head config — the base side never reparses the file.
func scoreBaseRef(ctx context.Context, deps *appDeps, baseRef string, head runContext, cfg config.Config, advisory bool) (score.Scorecard, baseEvidence, error) {
	// A leading-dash ref would be parsed as a flag by rev-parse/worktree-add;
	// `git worktree add --detach <dir> --force` silently checks out HEAD and the
	// delta becomes HEAD-vs-HEAD. Reject rather than pass through.
	if strings.HasPrefix(baseRef, "-") {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: invalid --base ref %q", baseRef)}
	}
	// Resolve the git root. Use --root when given (absolutized); otherwise the
	// config bundle directory — both are inside the repo and yield the same gitRoot.
	gitAnchor := head.scanDir()
	if abs, aerr := filepath.Abs(gitAnchor); aerr == nil {
		gitAnchor = abs
	}
	gitRoot, err := git.RepoRoot(ctx, gitAnchor, deps.Runner)
	if err != nil {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: --base requires a git repository: %v", err)}
	}

	// HEAD-side analysis boundary: --root when given, else the whole repo.
	headScanRoot := head.ScanRoot
	if headScanRoot == "" {
		headScanRoot = gitRoot
	} else if abs, aerr := filepath.Abs(headScanRoot); aerr == nil {
		headScanRoot = abs
	}
	// Canonicalize symlinks so filepath.Rel works on macOS (/var vs /private/var).
	if canon, cerr := filepath.EvalSymlinks(headScanRoot); cerr == nil {
		headScanRoot = canon
	}
	// Snap a case-variant --root to gitRoot's canonical casing so the subtree
	// mapping (filepath.Rel) works on case-insensitive filesystems — the main scan
	// path does this via scope.snapScanRoot/os.SameFile; the delta path must too (F4).
	headScanRoot = snapToGitRoot(gitRoot, headScanRoot)

	tmpBase, releaseWorktreeParent, err := baseWorktreeParent(ctx, deps, gitRoot, baseRef)
	if err != nil {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: create temp dir: %v", err)}
	}
	wtDir := filepath.Join(tmpBase, "wt")
	defer func() {
		removeWorktree(ctx, deps.Runner, gitRoot, wtDir)
		_ = os.RemoveAll(tmpBase)
		releaseWorktreeParent()
	}()

	if aerr := addWorktree(ctx, deps.Runner, gitRoot, wtDir, baseRef); aerr != nil {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: cannot create worktree for ref %q: %v", baseRef, aerr)}
	}
	wtCanon, err := filepath.EvalSymlinks(wtDir)
	if err != nil {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: eval worktree symlinks: %v", err)}
	}
	baseRoot, err := subtreeInWorktree(gitRoot, headScanRoot, wtCanon)
	if err != nil {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: map subtree into worktree: %v", err)}
	}

	sc, ev, err := runScoreSide(ctx, deps, cfg, baseRunContext(head, baseRoot), advisory)
	if err != nil {
		return score.Scorecard{}, baseEvidence{}, &exitError{code: 3, msg: fmt.Sprintf("error: score base (%s): %v", baseRef, err)}
	}
	return sc, ev, nil
}

// runScoreSide runs the full pipeline over rc with the caller's already-parsed
// config and returns the synthesised Scorecard plus the base evidence. advisory
// mirrors the caller's advisory setting (`--no-advisories`) so the base and HEAD
// sides are scored identically.
//
// The Diagnostic is projected to baseEvidence here and never returned: that is
// the structural guarantee that no base path, location, or validation command
// reaches head output.
func runScoreSide(ctx context.Context, deps *appDeps, cfg config.Config, rc runContext, advisory bool) (score.Scorecard, baseEvidence, error) {
	// Silence phase progress for the base sub-scan: the head run already announced
	// "Comparing against base", and re-emitting discover/facts/analyze through the
	// same reporter overflows the head phase counter (e.g. [7/6], F6). Label its
	// stderr warnings so a base-side owner/TS-coverage degradation isn't misread
	// as a head-side regression on the shared stream.
	quiet := *deps
	quiet.progress = nil
	quiet.warnLabel = "[base] "
	mode := engine.Mode{Full: true, Advisory: advisory, ReportOnly: true}
	diag, sc, err := runPipeline(ctx, &quiet, cfg, rc, mode, baseline.Baseline{})
	if err != nil {
		return score.Scorecard{}, baseEvidence{}, err
	}
	return sc, baseEvidence{
		FindingIDs:   baseFindingIDs(diag.Findings),
		Coverage:     diag.ToolCoverage,
		CoverageGaps: diag.CoverageGaps,
		ConfigHash:   diag.ConfigHash,
	}, nil
}

// baseWorktreeParent picks the parent directory for the base-side worktree
// (wt/ is created inside it and the whole directory is removed after the run).
// The returned release function must be called after cleanup.
//
// With the fact cache on, the path is a deterministic function of the resolved
// base commit SHA: `<gitRoot>/.archfit-cache/worktrees/<sha>`. The base tree
// is immutable, and the per-checkout fact-cache keys fold the scan-root path in
// (cached subprocess output embeds absolute paths — see rust.cachedRunner /
// golang.memberKeys), so a repeat `--base <same-ref>` run reuses the same
// absolute root and every base-side extractor subprocess becomes a cache hit
// (Wave 6 Task 4). A leftover checkout from a crashed run is removed and the
// path reused. Concurrent runs for the same SHA take an inter-process lock
// before removing/recreating the checkout, so one process cannot delete another
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
func baseWorktreeParent(ctx context.Context, deps *appDeps, gitRoot, baseRef string) (string, func(), error) {
	releaseNoop := func() {}
	sha, err := git.ResolveCommit(ctx, gitRoot, baseRef, deps.Runner)
	// gitRoot is already absolute (git rev-parse --show-toplevel); absolutize
	// anyway so the git subprocesses below (WorkDir: gitRoot) and the os calls
	// that create/remove the directory can never disagree about the path.
	parent, aerr := filepath.Abs(baseWorktreesDir(gitRoot))
	if err == nil && aerr == nil {
		dir := filepath.Join(parent, sha)
		release, lerr := lockBaseWorktree(ctx, dir+".lock")
		if lerr == nil {
			removeWorktree(ctx, deps.Runner, gitRoot, filepath.Join(dir, "wt"))
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

const baseWorktreeLockPoll = 50 * time.Millisecond

func lockBaseWorktree(ctx context.Context, lockPath string) (func(), error) {
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
		case <-time.After(baseWorktreeLockPoll):
		}
	}
}

// snapToGitRoot rewrites headRoot so its gitRoot prefix uses gitRoot's canonical
// casing, so filepath.Rel works on case-insensitive filesystems (macOS APFS),
// mirroring scope.snapScanRoot. It walks headRoot upward and, when an ancestor is
// the same directory as gitRoot (device+inode via os.SameFile), rebuilds gitRoot
// plus the collected suffix. Returns headRoot unchanged when no ancestor matches.
func snapToGitRoot(gitRoot, headRoot string) string {
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

// subtreeInWorktree maps headRoot (absolute, inside gitRoot) to its mirror
// inside wtDir. When headRoot == gitRoot the result is wtDir itself.
// Returns an error when headRoot is not under gitRoot (e.g. a ../ path).
func subtreeInWorktree(gitRoot, headRoot, wtDir string) (string, error) {
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

// addWorktree runs `git worktree add --detach <dir> <ref>` from gitRoot via the
// Runner. GIT_DIR/GIT_WORK_TREE/etc. are scrubbed so CI/hook-set vars do not
// redirect the command to the wrong repository.
func addWorktree(ctx context.Context, runner toolrun.Runner, gitRoot, dir, ref string) error {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitBinary,
		Args:    []string{gitWorktree, "add", "--detach", dir, ref},
		Env:     cleanGitEnv(),
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

// removeWorktree removes the worktree registration cleanly via the Runner.
// Best-effort — errors are ignored; os.RemoveAll on the temp dir follows in the
// caller's defer.
func removeWorktree(ctx context.Context, runner toolrun.Runner, gitRoot, dir string) {
	env := cleanGitEnv()
	_, _ = runner.Run(ctx, toolrun.ToolCmd{
		Name: gitBinary, Args: []string{"worktree", "remove", "--force", dir},
		Env: env, WorkDir: gitRoot,
	})
	// Prune in case RemoveAll ran before the remove command.
	_, _ = runner.Run(ctx, toolrun.ToolCmd{
		Name: gitBinary, Args: []string{"worktree", "prune"},
		Env: env, WorkDir: gitRoot,
	})
}
