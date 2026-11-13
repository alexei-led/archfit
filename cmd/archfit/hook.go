// Package main — `archfit hook run`, the hook entry points that doctor --fix
// installs (design 8.2, 10.2, 11.3).
//
// The hooks run `archfit can` and `archfit check` in process and map the
// answer onto the host's protocol: Claude Code and Codex read exit 2 with
// stderr as feedback for the agent (on a Stop hook it keeps the agent
// working); git pre-commit reads a non-zero exit as "do not commit". The hook
// exit code is the host protocol, not the status of the change. The hooks
// hold no judgement of their own: the record they show is the record the
// command prints. A hook that fails for its own reasons reports and does not
// stop anything.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/application/changecheck"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Events of `archfit hook run`.
const (
	hookEventSessionStart = "session-start"
	hookEventPostToolUse  = "post-tool-use"
	hookEventStop         = "stop"
	hookEventPreCommit    = "pre-commit"
)

// Harnesses doctor --fix writes hooks for.
const (
	harnessClaude = "claude"
	harnessCodex  = "codex"
	harnessGit    = "git"
)

// headRef names the commit the working tree builds on. The pre-commit hook
// judges the staged change against it: the commit it will follow.
const headRef = "HEAD"

// cacheDirName is archfit's cache directory under a repository.
const cacheDirName = ".archfit-cache"

// gitBinary is the git executable the hooks ask for the tree state.
const gitBinary = "git"

// HookCmd groups the host hooks. It is not in the command list (design
// 11.1): doctor --fix installs the hooks, and they call hook run.
type HookCmd struct {
	Install HookInstallCmd `cmd:"" help:"Write the archfit hooks into the configuration of a harness: claude, codex, or git."`
	Run     HookRunCmd     `cmd:"" help:"Run one hook event: session-start, post-tool-use, stop, or pre-commit. The installed hooks call it."`
}

// HookRunCmd runs one hook event.
type HookRunCmd struct {
	Event   string `arg:"" enum:"session-start,post-tool-use,stop,pre-commit" help:"Hook event: session-start, post-tool-use, stop, or pre-commit."`
	Harness string `help:"Agent harness that runs the hook: claude or codex. Both read the same protocol." enum:"claude,codex" default:"claude"`
	// IndexFile is the index git commit hands its hooks: git commit -a and git
	// commit <path> stage into a temporary one. Hidden: git sets it, a user does not.
	IndexFile string `name:"index-file" env:"GIT_INDEX_FILE" hidden:"" help:"pre-commit: index to judge (default: GIT_INDEX_FILE, else the repository's index)."`
}

func (*HookRunCmd) Help() string {
	return `Run one hook event. archfit doctor --fix installs these commands; a harness
without an installer can call them from its own hook system. Every hook reads
.archfit.yaml at the root of the repository, as archfit check does.

session-start, post-tool-use, and stop read the hook event on stdin
(session_id, cwd, hook_event_name, stop_hook_active) and work in the
repository that holds the event cwd. They remember the session in
.archfit-cache/session/<session_id>.json.

  session-start   records HEAD as the session base and prints the five
                  lines for agents that archfit init writes
  post-tool-use   when the working tree changed since its last run, reads
                  the imports the check reads, keeps those on lines added
                  since the session base, and asks archfit can about each:
                    no, not reported earlier in the session
                                    exit 2; stderr names the file and line,
                                    then the record of archfit can; the
                                    first answer is stored for archfit
                                    request
                  The errors an exception covers are left out, as check
                  leaves them out.
                    yes, unknown    exit 0, silent
                  Without a version 3 policy it is silent.
  stop            runs archfit check, as you would before you finish:
                    fail (first stop)           exit 2; the record on stderr
                    fail, with stop_hook_active exit 0 with a systemMessage
                    fail, the same errors it stopped on last time
                                                exit 0 with a systemMessage
                    unknown                     exit 0 with a systemMessage
                    pass                        exit 0, silent
                  A pass or an unknown forgets the last stop, so the same
                  errors coming back stop again. A SubagentStop is stopped
                  once through stop_hook_active and never uses up the stop of
                  the main agent. Nothing changed since the session base (HEAD
                  is the base and the tree is clean, .archfit-cache aside)
                  skips the run.
  pre-commit      runs archfit check on what the commit will contain: the
                  staged content (the index git hands the hook in
                  GIT_INDEX_FILE), never unstaged edits or untracked files,
                  against HEAD, so a commit is stopped only for the errors it
                  adds and for an exception past its date:
                    fail      exit 1; the record on stderr
                    unknown   exit 0; the record on stderr
                    pass      exit 0, silent

A hook that fails for its own reasons (no git repository, a git error) shows
why and exits 0: it never stops the agent or the commit. Malformed stdin
exits 1.`
}

// harnessSubagentStop is the hook_event_name Claude Code gives a subagent's stop.
const harnessSubagentStop = "SubagentStop"

// hookEvent is the part of a Claude Code or Codex hook event the hooks read.
type hookEvent struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	StopHookActive bool   `json:"stop_hook_active"`
}

func (c *HookRunCmd) Run(deps *appDeps) error {
	if c.Event == hookEventPreCommit {
		return c.preCommit(deps)
	}
	raw, err := io.ReadAll(deps.stdin())
	if err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: read the hook event: %v", err)}
	}
	var event hookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: malformed hook event: %v", err)}
	}
	ctx := deps.runContext()
	dir := event.Cwd
	if dir == "" {
		dir = "."
	}
	switch c.Event {
	case hookEventSessionStart:
		return sessionStart(ctx, deps, dir, event.SessionID)
	case hookEventPostToolUse:
		return postToolUse(ctx, deps, dir, event.SessionID)
	default:
		return stopHook(ctx, deps, dir, event)
	}
}

// couldNotRun is what a hook shows when it fails for its own reasons. It
// stops nothing.
func couldNotRun(err error) string {
	return "archfit hook could not run: " + err.Error() + ". It stopped nothing."
}

// stopHook runs archfit check and stops the agent once on a change that
// fails. It does not stop again on the same set of errors: a pass or an
// unknown forgets the stop, so the same errors coming back stop again. A
// subagent shares the session but neither records nor applies that: its stop
// is stopped once through stop_hook_active, as the main agent's, and cannot
// use up the stop of the main agent.
func stopHook(ctx context.Context, deps *appDeps, dir string, event hookEvent) error {
	sess, err := openSession(ctx, deps.Runner, dir, event.SessionID)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	untouched, err := sess.untouched(ctx, deps.Runner)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	if untouched {
		return forgetStop(deps, sess, "")
	}
	co, err := openCheckout(ctx, deps, sess.repo)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	out, err := co.changeCheck().Check(ctx, changecheck.Options{})
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	text, err := recordText(out)
	if err != nil {
		return systemMessage(deps, couldNotRun(err))
	}
	switch out.Result.Status {
	case result.StatusFail:
		if event.StopHookActive {
			return systemMessage(deps, "archfit: the change still fails after one attempt to fix it:\n"+text)
		}
		if event.HookEventName == harnessSubagentStop {
			return &exitError{code: 2, msg: strings.TrimSuffix(text, "\n")}
		}
		digest := errorSetDigest(out.Result)
		if digest == sess.StoppedOn {
			return systemMessage(deps, "archfit: the same errors as at the last stop; this stop goes on:\n"+text)
		}
		sess.StoppedOn = digest
		if err := sess.save(); err != nil {
			// The stop still holds; only its repeat suppression is lost.
			text += "archfit: the session was not recorded: " + err.Error() + "\n"
		}
		return &exitError{code: 2, msg: strings.TrimSuffix(text, "\n")}
	case result.StatusPass:
		return forgetStop(deps, sess, "")
	default:
		return forgetStop(deps, sess, text)
	}
}

// forgetStop clears the errors the stop hook recorded, then shows msg as a
// systemMessage when there is one. A save that fails does not stop anything:
// it only leaves the next identical result suppressed.
func forgetStop(deps *appDeps, sess *hookSession, msg string) error {
	if sess.StoppedOn != "" {
		sess.StoppedOn = ""
		if err := sess.save(); err != nil {
			msg += "archfit: the session was not recorded: " + err.Error() + "\n"
		}
	}
	if msg == "" {
		return nil
	}
	return systemMessage(deps, msg)
}

// errorSetDigest identifies the set of errors of a result by their keys:
// equal keys name the same error, wherever its line moved.
func errorSetDigest(r result.Result) string {
	keys := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		keys = append(keys, e.Key)
	}
	slices.Sort(keys)
	sum := sha256.Sum256([]byte(strings.Join(slices.Compact(keys), "\n")))
	return hex.EncodeToString(sum[:])
}

// recordText is the record archfit check prints for out, then its notes, as
// archfit check prints them on stderr.
func recordText(out changecheck.Outcome) (string, error) {
	var b bytes.Buffer
	if err := record.Result(&b, out.Result, record.Options{}); err != nil {
		return "", err
	}
	for _, note := range out.Notes {
		b.WriteString("archfit: " + note + "\n")
	}
	return b.String(), nil
}

// preCommit runs archfit check on the staged tree against HEAD. Only a fail
// stops the commit; a hook error is shown and lets it through.
func (c *HookRunCmd) preCommit(deps *appDeps) error {
	out, err := stagedCheck(deps.runContext(), deps, c.IndexFile)
	if err != nil {
		_, _ = fmt.Fprintln(deps.stderr(), couldNotRun(err))
		return nil
	}
	text, err := recordText(out)
	if err != nil {
		_, _ = fmt.Fprintln(deps.stderr(), couldNotRun(err))
		return nil
	}
	switch out.Result.Status {
	case result.StatusFail:
		return &exitError{code: 1, msg: strings.TrimSuffix(text, "\n")}
	case result.StatusPass:
		return nil
	default:
		_, err := fmt.Fprint(deps.stderr(), text)
		return err
	}
}

// stagedCheck runs archfit check over the index rather than the files on
// disk: the index is recorded as an unreachable commit
// (historygit.SnapshotIndex), checked out in a temporary worktree, and checked
// there against the HEAD commit of the repository, whose policy is the one the
// staged policy is compared with. The result is not stored: the last result
// that archfit request reads stays the one of the working tree.
//
// The checkout holds tracked files only: gitignored inputs are not seen.
func stagedCheck(ctx context.Context, deps *appDeps, indexFile string) (changecheck.Outcome, error) {
	gitRoot, err := historygit.RepoRoot(ctx, ".", deps.Runner)
	if err != nil {
		return changecheck.Outcome{}, fmt.Errorf("the pre-commit hook needs a git repository: %w", err)
	}
	if indexFile != "" {
		// git hands a hook a path relative to the hook's working directory.
		if indexFile, err = filepath.Abs(indexFile); err != nil {
			return changecheck.Outcome{}, err
		}
	}
	snapshot, err := historygit.SnapshotIndex(ctx, deps.Runner, gitRoot, indexFile)
	if err != nil {
		return changecheck.Outcome{}, fmt.Errorf("record the index: %w", err)
	}
	// HEAD is resolved here, in the repository: inside the snapshot worktree
	// HEAD names the snapshot itself. Before the first commit there is none,
	// and the check looks for the target branch.
	base, err := historygit.ResolveCommit(ctx, gitRoot, headRef, deps.Runner)
	if err != nil {
		base = ""
	}
	root, cleanup, err := historygit.Worktree{Runner: deps.Runner}.Checkout(ctx, snapshot, gitRoot, gitRoot)
	defer cleanup()
	if err != nil {
		return changecheck.Outcome{}, fmt.Errorf("check out the index: %w", err)
	}
	co, err := openCheckout(ctx, deps, root)
	if err != nil {
		return changecheck.Outcome{}, err
	}
	svc := co.changeCheck()
	svc.Store = noStore{}
	return svc.Check(ctx, changecheck.Options{Base: base})
}

// noStore is a changecheck.Store that keeps nothing.
type noStore struct{}

func (noStore) Save(context.Context, string, result.Result) error { return nil }

// pathInRepo returns configAbs relative to gitRoot, through symlinks and a
// case-variant spelling, or an error when the config lies outside the
// repository.
func pathInRepo(gitRoot, configAbs string) (string, error) {
	resolved := configAbs
	if dir, err := filepath.EvalSymlinks(filepath.Dir(configAbs)); err == nil {
		resolved = filepath.Join(dir, filepath.Base(configAbs))
	}
	rel, err := filepath.Rel(gitRoot, historygit.SnapToRoot(gitRoot, resolved))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("config %s is outside the repository %s: the pre-commit hook reads it from the index", configAbs, gitRoot)
	}
	return rel, nil
}

// refExists reports whether ref names a commit in the repository at dir.
func refExists(ctx context.Context, runner toolrun.Runner, dir, ref string) bool {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: []string{gitRevParse, "--verify", "--quiet", ref + "^{commit}"}, WorkDir: dir})
	return err == nil && out.ExitCode == 0
}

// worktreeDirty reports whether git status --porcelain prints anything in dir.
func worktreeDirty(ctx context.Context, runner toolrun.Runner, dir string) (bool, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: []string{"status", "--porcelain"}, WorkDir: dir})
	if err != nil {
		return false, err
	}
	if out.ExitCode != 0 {
		return false, fmt.Errorf("git status exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	for _, line := range strings.Split(string(out.Stdout), "\n") {
		if strings.TrimSpace(line) != "" && !archfitCache(line) {
			return true, nil
		}
	}
	return false, nil
}

// archfitCache reports whether a git status --porcelain line ("XY path") is
// archfit's own cache, which a hook run writes (fact cache, base worktrees,
// session files) and which is not a change of the user's.
func archfitCache(statusLine string) bool {
	return inArchfitCache(strings.Trim(statusLine[min(3, len(statusLine)):], `"`))
}

// inArchfitCache reports whether a repository-relative path lies in an
// archfit cache directory, at the root or below it.
func inArchfitCache(path string) bool {
	return strings.HasPrefix(path, cacheDirName+"/") || strings.Contains(path, "/"+cacheDirName+"/")
}

// systemMessage writes the hook output that shows a message to the user
// without blocking: exit 0 with {"systemMessage": ...} on stdout.
func systemMessage(deps *appDeps, msg string) error {
	if err := json.NewEncoder(deps.Stdout).Encode(map[string]string{"systemMessage": strings.TrimSuffix(msg, "\n")}); err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: write the system message: %v", err)}
	}
	return nil
}
