// Package main — `archfit hook install` and `archfit hook run`.
//
// The hooks run the agent result of `check` (archfit.agent-result.v1) or the
// `policy can-import` answer in process and map it onto the host's protocol:
// Claude Code and Codex read exit 2 with stderr as feedback for the agent (on a
// Stop hook it keeps the agent working); git pre-commit reads a non-zero exit
// as "do not commit". The hook exit code is the host protocol, not the engine
// verdict.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/application"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/output/agentout"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Events of `archfit hook run`.
const (
	hookEventSessionStart = "session-start"
	hookEventPostToolUse  = "post-tool-use"
	hookEventStop         = "stop"
	hookEventPreCommit    = "pre-commit"
)

// Harnesses `archfit hook install` writes hooks for.
const (
	harnessClaude = "claude"
	harnessCodex  = "codex"
	harnessGit    = "git"
)

// headRef names the commit the working tree builds on. The pre-commit hook
// scopes the staged change against it: the commit it will follow.
const headRef = "HEAD"

// cacheDirName is archfit's cache directory under a repository.
const cacheDirName = ".archfit-cache"

// gitBinary is the git executable the hooks ask for the tree state.
const gitBinary = "git"

// HookCmd groups the host hooks.
type HookCmd struct {
	Install HookInstallCmd `cmd:"" help:"Write the archfit hooks into the configuration of a harness: claude, codex, or git."`
	Run     HookRunCmd     `cmd:"" help:"Run one hook event: session-start, post-tool-use, stop, or pre-commit. The installed hooks call it."`
}

// HookRunCmd runs one hook event.
type HookRunCmd struct {
	Event   string `arg:"" enum:"session-start,post-tool-use,stop,pre-commit" help:"Hook event: session-start, post-tool-use, stop, or pre-commit."`
	Harness string `help:"Agent harness that runs the hook: claude or codex. Both read the same protocol." enum:"claude,codex" default:"claude"`
	Config  string `short:"c" help:"Path to config file, relative to the repository root (the git toplevel of the event cwd)." default:".archfit.yaml"`
	// IndexFile is the index git commit hands its hooks: git commit -a and git
	// commit <path> stage into a temporary one. Hidden: git sets it, a user does not.
	IndexFile string `name:"index-file" env:"GIT_INDEX_FILE" hidden:"" help:"pre-commit: index to judge (default: GIT_INDEX_FILE, else the repository's index)."`
}

func (*HookRunCmd) Help() string {
	return `Run one hook event. archfit hook install registers these commands; a harness
without an installer can call them from its own hook system.

session-start, post-tool-use, and stop read the hook event on stdin
(session_id, cwd, hook_event_name, stop_hook_active) and resolve --config
against the root of the repository that holds the event cwd, so an agent that
changed directory still finds it. They remember the session in
.archfit-cache/session/<session_id>.json.

  session-start   records HEAD as the session base and prints the bootstrap
                  the harness adds to the agent's context
  post-tool-use   finds the files that differ from the session base with git
                  (never from the tool payload), reads the imports their added
                  lines introduce (Go; relative TypeScript and JavaScript;
                  Python), and judges each with policy can-import:
                    a denial not reported earlier in the session
                                    exit 2; stderr names the file, the target,
                                    the rule, why, and what to do instead
                    anything else   exit 0, silent
  stop            runs check --format agent --base <session base> in process,
                  with the policy guard of check, and maps the change status:
                    fail (first stop)           exit 2; stderr holds the blockers
                    fail, with stop_hook_active exit 0 with a systemMessage
                    fail, the same blockers on the same tree it blocked on
                                                exit 0 with a systemMessage
                    unknown                     exit 0 with a systemMessage
                    pass                        exit 0, silent
                  A pass or an unknown change forgets the block, so the same
                  blockers coming back block again. A SubagentStop is blocked
                  once through stop_hook_active and never suppresses the main
                  agent's stop.
                  Nothing changed since the session base (HEAD is the base and
                  the tree is clean, .archfit-cache aside) skips the run.
  pre-commit      judges what the commit will contain: the staged content (the
                  index git hands the hook in GIT_INDEX_FILE), never unstaged
                  edits or untracked files, against HEAD. Exit 1 with the
                  blockers on stderr when the change fails, 0 otherwise (an
                  unknown change is printed), 3 when archfit cannot run (no git
                  repository, unmerged index entries, a config not in the
                  index). The config comes from the index; the baseline,
                  labels, and fact cache from the config's directory on disk.

Without a session file (no session-start ran), the session starts at the first
hook that runs: its base is HEAD. Malformed stdin exits 1. An archfit error in
post-tool-use or stop fails open: exit 0 with a systemMessage.`
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
		return postToolUse(ctx, deps, dir, c.Config, event.SessionID)
	default:
		return stopHook(ctx, deps, dir, c.Config, event)
	}
}

// stopHook blocks a stop once on a change that fails, judged against the
// session base: a commit made during the session does not hide what it added.
//
// It does not block twice on the same blockers over the same tree. A pass, or
// a change it cannot judge, forgets the block, so the same blockers coming back
// later block again. A subagent shares the session but neither records nor
// applies that suppression: its stop is blocked once through stop_hook_active,
// as the main agent's, and cannot use up the main agent's block.
func stopHook(ctx context.Context, deps *appDeps, dir, config string, event hookEvent) error {
	sess, err := openSession(ctx, deps.Runner, dir, event.SessionID)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	untouched, err := sess.untouched(ctx, deps.Runner)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	if untouched {
		return forgetBlock(deps, sess, "")
	}
	result, err := sessionResult(ctx, deps, configIn(sess.repo, config), sess.BaseSHA)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	brief := agentout.Brief(result)
	switch outcomeOf(result) {
	case hookBlock:
		if event.StopHookActive {
			return systemMessage(deps, "archfit still reports blockers after one repair attempt:\n"+brief)
		}
		if event.HookEventName == harnessSubagentStop {
			return &exitError{code: 2, msg: strings.TrimSuffix(brief, "\n")}
		}
		digest := blockDigest(result)
		if digest == sess.BlockedDigest {
			return systemMessage(deps, "archfit reports the blockers it already stopped on, on the same tree; it does not stop again:\n"+brief)
		}
		sess.BlockedDigest = digest
		if err := sess.save(); err != nil {
			// The block still stands; only its repeat suppression is lost.
			brief += "archfit: could not record the session: " + err.Error() + "\n"
		}
		return &exitError{code: 2, msg: strings.TrimSuffix(brief, "\n")}
	case hookAllow:
		return forgetBlock(deps, sess, "")
	default:
		return forgetBlock(deps, sess, brief)
	}
}

// forgetBlock clears the block the stop hook recorded, then shows msg as a
// systemMessage when there is one. A save that fails does not block: it only
// leaves the next identical result suppressed.
func forgetBlock(deps *appDeps, sess *hookSession, msg string) error {
	if sess.BlockedDigest != "" {
		sess.BlockedDigest = ""
		if err := sess.save(); err != nil {
			msg += "archfit: could not record the session: " + err.Error() + "\n"
		}
	}
	if msg == "" {
		return nil
	}
	return systemMessage(deps, msg)
}

// blockDigest identifies a blocked result by the tree it measured and its
// blockers: the same digest twice means nothing the stop hook judges changed.
func blockDigest(r agentout.Result) string {
	var ids []string
	for _, rep := range r.Change.Blockers {
		ids = append(ids, rep.FindingIDs...)
	}
	slices.Sort(ids)
	sum := sha256.Sum256([]byte(r.Identity.HeadTreeDigest + "\n" + strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])
}

func (c *HookRunCmd) preCommit(deps *appDeps) error {
	result, err := stagedHookResult(deps.runContext(), deps, c.Config, headRef, c.IndexFile)
	if err != nil {
		return &exitError{code: 3, msg: "archfit hook: " + err.Error()}
	}
	switch outcomeOf(result) {
	case hookBlock:
		return &exitError{code: 1, msg: strings.TrimSuffix(agentout.Brief(result), "\n")}
	case hookReport:
		_, err := fmt.Fprint(deps.stderr(), agentout.Brief(result))
		return err
	default:
		return nil
	}
}

// hookOutcome is what a hook does with a result.
type hookOutcome int

const (
	// hookAllow: the change passes; the hook is silent.
	hookAllow hookOutcome = iota
	// hookReport: the change cannot be judged; the hook shows why and lets it through.
	hookReport
	// hookBlock: the change fails; the hook stops it.
	hookBlock
)

// outcomeOf reads the change status and nothing else: the hook holds no
// judgement of its own, so it cannot disagree with the CLI.
func outcomeOf(r agentout.Result) hookOutcome {
	switch r.Change.Status {
	case agentout.ChangeFail:
		return hookBlock
	case agentout.ChangePass:
		return hookAllow
	default:
		return hookReport
	}
}

// sessionResult runs check --format agent over the files on disk, compared
// with the session base, under the scope and policy guard check resolves: a
// policy file the branch changed makes the change unknown. A session that
// started before the first commit has no base: the whole tree is the change.
func sessionResult(ctx context.Context, deps *appDeps, configPath, base string) (agentout.Result, error) {
	req := scanRequest{configPath: configPath, lint: true}
	if base == "" {
		return runAgentCheck(ctx, deps, req, nil)
	}
	req.baseRef = base
	if !refExists(ctx, deps.Runner, filepath.Dir(configPath), base) {
		// The base commit is gone (a rebase dropped it and gc pruned it): the
		// last commit is the nearest reference left.
		req.baseRef = headRef
	}
	cfg, err := loadAnalysisConfig(ctx, configPath)
	if err != nil {
		return agentout.Result{}, err
	}
	scope, err := resolveCheckScope(ctx, deps, &req, cfg.TargetBranch)
	if err != nil {
		return agentout.Result{}, err
	}
	return runAgentCheck(ctx, deps, req, &scope)
}

// stagedHookResult runs check --format agent over the index rather than the
// files on disk: the index is recorded as an unreachable commit
// (historygit.SnapshotIndex) and checked out in a temporary worktree, which is
// the head side of the run. The baseline, labels, and fact cache stay in the
// config's directory on disk, as on the --base side.
//
// The checkout holds tracked files only. Gitignored inputs an analyzer
// resolves through (node_modules, generated code) come from the surrounding
// repository, as for --base, so a node_modules below the repository root is
// not seen.
func stagedHookResult(ctx context.Context, deps *appDeps, configPath, base, indexFile string) (agentout.Result, error) {
	configAbs, err := filepath.Abs(configPath)
	if err != nil {
		return agentout.Result{}, err
	}
	configDir := filepath.Dir(configAbs)
	gitRoot, err := historygit.RepoRoot(ctx, configDir, deps.Runner)
	if err != nil {
		return agentout.Result{}, fmt.Errorf("the pre-commit hook needs a git repository: %w", err)
	}
	if indexFile != "" {
		// git hands a hook a path relative to the hook's working directory.
		if indexFile, err = filepath.Abs(indexFile); err != nil {
			return agentout.Result{}, err
		}
	}
	configRel, err := pathInRepo(gitRoot, configAbs)
	if err != nil {
		return agentout.Result{}, err
	}
	snapshot, err := historygit.SnapshotIndex(ctx, deps.Runner, gitRoot, indexFile)
	if err != nil {
		return agentout.Result{}, fmt.Errorf("record the index: %w", err)
	}
	// The base ref is resolved here, in the repository: inside the snapshot
	// worktree HEAD names the snapshot itself, and every finding would read
	// as pre-existing.
	baseSHA := ""
	if base != "" {
		// Before the first commit HEAD names nothing: the whole tree is the
		// change, so every blocker is in scope.
		if sha, rerr := historygit.ResolveCommit(ctx, gitRoot, base, deps.Runner); rerr == nil {
			baseSHA = sha
		}
	}
	// The scan boundary is the whole repository, as for check -c <config>
	// with no --root: the snapshot worktree's root, never the config's directory.
	root, cleanup, err := historygit.Worktree{Runner: deps.Runner}.Checkout(ctx, snapshot, gitRoot, gitRoot)
	defer cleanup()
	if err != nil {
		return agentout.Result{}, fmt.Errorf("check out the index: %w", err)
	}
	stagedConfig := filepath.Join(root, configRel)
	if _, err := os.Stat(stagedConfig); err != nil {
		return agentout.Result{}, fmt.Errorf("config %s is not in the index: stage it first", configPath)
	}
	req := scanRequest{
		configPath: stagedConfig, root: root, bundleDir: configDir, baseRef: baseSHA,
		// The repair's validation command must not name the snapshot, which
		// cleanup removes: it names the repository and the ref as written.
		validationConfig: filepath.Join(gitRoot, configRel), validationRoot: gitRoot,
	}
	if baseSHA != "" {
		req.validationBase = base
	}
	return runAgentCheck(ctx, deps, req, nil)
}

// pathInRepo returns configAbs relative to gitRoot, through symlinks and a
// case-variant spelling, or an error when the config lies outside the
// repository: the pre-commit hook reads the config from the index.
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

// runAgentCheck runs check --format agent in process, with the pipeline's own
// warnings kept off the hook's stderr, which the host shows to the agent. A
// resolved scope is attached as check attaches it, with the result under the
// target's policy when a policy file differs from the target tip.
func runAgentCheck(ctx context.Context, deps *appDeps, req scanRequest, scope *application.ScopeResolution) (agentout.Result, error) {
	quiet := *deps
	quiet.Stdout, quiet.Stderr = io.Discard, io.Discard
	req.formats, req.progress, req.quiet = []string{formatAgent}, progressNone, true
	resp, _, err := executeScan(ctx, &quiet, req, nil)
	if err != nil {
		return agentout.Result{}, err
	}
	doc := resp.Document
	if scope != nil {
		underTarget, err := measureUnderTargetPolicy(ctx, &quiet, req, *scope)
		if err != nil {
			return agentout.Result{}, err
		}
		doc = application.AttachScope(doc, *scope, underTarget)
	}
	return agentout.Build(doc), nil
}

// refExists reports whether ref names a commit in the repository at dir.
func refExists(ctx context.Context, runner toolrun.Runner, dir, ref string) bool {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: []string{"rev-parse", "--verify", "--quiet", ref + "^{commit}"}, WorkDir: dir})
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

// configIn resolves a relative config path against the hook's working
// directory: the event cwd, not the hook process's own.
func configIn(dir, configPath string) string {
	if filepath.IsAbs(configPath) {
		return configPath
	}
	return filepath.Join(dir, configPath)
}

// systemMessage writes the hook output that shows a message to the user
// without blocking: exit 0 with {"systemMessage": ...} on stdout.
func systemMessage(deps *appDeps, msg string) error {
	if err := json.NewEncoder(deps.Stdout).Encode(map[string]string{"systemMessage": strings.TrimSuffix(msg, "\n")}); err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: write the system message: %v", err)}
	}
	return nil
}
