// Package main — `archfit hook claude` and `archfit hook git`.
//
// The hooks run the agent result of `check` (archfit.agent-result.v1) in
// process and map its next action onto the host's protocol: Claude Code reads
// exit 2 on a Stop hook as "do not stop yet" and feeds stderr to the agent; git
// pre-commit reads a non-zero exit as "do not commit". The hook exit code is
// the host protocol, not the engine verdict.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	historygit "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/output/agentout"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Claude Code hook events the Stop hook acts on.
const (
	hookEventStop         = "Stop"
	hookEventSubagentStop = "SubagentStop"
)

// cacheDirName is archfit's cache directory under a repository.
const cacheDirName = ".archfit-cache"

// gitBinary is the git executable the Stop hook asks for the tree state.
const gitBinary = "git"

// HookCmd groups the host hooks.
type HookCmd struct {
	Claude HookClaudeCmd `cmd:"" help:"Claude Code Stop/SubagentStop hook: reads the event on stdin; exit 2 with the repair on stderr blocks the stop once."`
	Git    HookGitCmd    `cmd:"" help:"git pre-commit hook: exit 1 on a repair or an owner decision, 3 on an error."`
}

// hookFlags are the flags both hooks share.
type hookFlags struct {
	Config string `short:"c" help:"Path to config file, relative to the hook's working directory." default:".archfit.yaml"`
	Base   string `help:"Git ref the result is scoped against: a repair whose findings all exist at this ref is outside the scope. Empty scopes every blocker in." default:"HEAD"`
}

// HookClaudeCmd is the Claude Code Stop hook.
type HookClaudeCmd struct {
	hookFlags
}

// HookGitCmd is the git pre-commit hook.
type HookGitCmd struct {
	hookFlags
	// IndexFile is the index git commit hands its hooks: git commit -a and git
	// commit <path> stage into a temporary one. Hidden: git sets it, a user does not.
	IndexFile string `name:"index-file" env:"GIT_INDEX_FILE" hidden:"" help:"Index to judge (default: GIT_INDEX_FILE, else the repository's index)."`
}

func (*HookClaudeCmd) Help() string {
	return `Run as a Claude Code Stop and SubagentStop hook. Register it in
.claude/settings.json:

  {"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "archfit hook claude"}]}],
             "SubagentStop": [{"hooks": [{"type": "command", "command": "archfit hook claude"}]}]}}

It reads the hook event on stdin and resolves --config against the event cwd.
A clean working tree (git status --porcelain lists nothing but archfit's own
.archfit-cache) skips the run. Otherwise it runs check --format agent
--base <ref> in process and maps the next action:

  repair, ask_owner with an in-scope repair (first stop)
                                    exit 2; stderr holds the repair for the agent
  the same, with stop_hook_active   exit 0 with a systemMessage: it blocks once
  any other action but none         exit 0 with a systemMessage (a dead selector
                                    or a metric ratchet is not scoped to the change)
  none                              exit 0, silent
  archfit error                     exit 0 with a systemMessage (fails open)
  malformed stdin                   exit 1
  any other event                   exit 0`
}

func (*HookGitCmd) Help() string {
	return `Run as a git pre-commit hook (see .pre-commit-hooks.yaml):

  - repo: https://github.com/alexei-led/archfit
    rev: <tag>
    hooks:
      - id: archfit

It judges what the commit will contain: the staged content (the index git
hands the hook in GIT_INDEX_FILE), never unstaged edits or untracked files.
It records the index as an unreachable commit object, checks it out in a
temporary worktree under .archfit-cache/worktrees, and runs check --format
agent --base <ref> there; the working tree, the index, and every ref stay as
they are. The config comes from the index; the baseline, labels, and fact
cache come from the config's directory on disk.

Exit 1 with the repair on stderr when the next action is repair or ask_owner
and a repair is in scope, 0 otherwise (other actions are printed), and 3 when
archfit cannot run (no git repository, unmerged index entries, or a config
that is not in the index).`
}

// claudeHookEvent is the part of the Claude Code hook event the hook reads.
type claudeHookEvent struct {
	HookEventName  string `json:"hook_event_name"`
	Cwd            string `json:"cwd"`
	StopHookActive bool   `json:"stop_hook_active"`
}

func (c *HookClaudeCmd) Run(deps *appDeps) error {
	raw, err := io.ReadAll(deps.stdin())
	if err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: read the hook event: %v", err)}
	}
	var event claudeHookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: malformed hook event: %v", err)}
	}
	if event.HookEventName != hookEventStop && event.HookEventName != hookEventSubagentStop {
		return nil
	}
	ctx := context.Background()
	dir := event.Cwd
	if dir == "" {
		dir = "."
	}
	dirty, err := worktreeDirty(ctx, deps.Runner, dir)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	if !dirty {
		return nil
	}
	result, err := hookResult(ctx, deps, configIn(dir, c.Config), c.Base)
	if err != nil {
		return systemMessage(deps, "archfit hook failed open: "+err.Error())
	}
	brief := agentout.Brief(result)
	switch {
	case blocksChange(result):
		if event.StopHookActive {
			return systemMessage(deps, "archfit still reports blockers after one repair attempt:\n"+brief)
		}
		return &exitError{code: 2, msg: strings.TrimSuffix(brief, "\n")}
	case result.NextAction == agentout.ActionNone:
		return nil
	default:
		return systemMessage(deps, brief)
	}
}

func (c *HookGitCmd) Run(deps *appDeps) error {
	result, err := stagedHookResult(context.Background(), deps, c.Config, c.Base, c.IndexFile)
	if err != nil {
		return &exitError{code: 3, msg: "archfit hook: " + err.Error()}
	}
	switch {
	case blocksChange(result):
		return &exitError{code: 1, msg: strings.TrimSuffix(agentout.Brief(result), "\n")}
	case result.NextAction != agentout.ActionNone:
		_, err := fmt.Fprint(deps.stderr(), agentout.Brief(result))
		return err
	default:
		return nil
	}
}

// blocksChange reports whether a hook blocks: the next action is repair or
// ask_owner and an in-scope repair exists. A dead selector or a metric
// ratchet also leads to those actions, but neither is scoped to the change:
// both may predate it, so a hook reports them and lets the change through.
func blocksChange(r agentout.Result) bool {
	if r.NextAction != agentout.ActionRepair && r.NextAction != agentout.ActionAskOwner {
		return false
	}
	for _, rep := range r.Repairs {
		if rep.InScope {
			return true
		}
	}
	return r.Omitted.Repairs > 0
}

// hookResult runs check --format agent over the files on disk: the Claude
// hook judges the agent's edits, which are not staged.
func hookResult(ctx context.Context, deps *appDeps, configPath, base string) (agentout.Result, error) {
	// Before the first commit HEAD names nothing: the whole tree is the
	// change, so every blocker is in scope.
	if base != "" && !refExists(ctx, deps.Runner, filepath.Dir(configPath), base) {
		base = ""
	}
	return runAgentCheck(ctx, deps, scanRequest{configPath: configPath, baseRef: base})
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
		return agentout.Result{}, fmt.Errorf("hook git needs a git repository: %w", err)
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
	result, err := runAgentCheck(ctx, deps, scanRequest{configPath: stagedConfig, root: root, bundleDir: configDir, baseRef: baseSHA})
	if err != nil {
		return agentout.Result{}, err
	}
	// The run's validation command names the snapshot, which cleanup removes.
	// Point it at the repository, with the ref as the caller wrote it.
	result.Validate = strings.ReplaceAll(result.Validate, root, gitRoot)
	// A ref that needs shell quoting (HEAD~1) keeps its resolved SHA.
	if baseSHA != "" && !strings.ContainsAny(base, " \t\n'\"\\$`!#&;()*<>?[]^{|}~") {
		result.Validate = strings.ReplaceAll(result.Validate, "--base "+baseSHA, "--base "+base)
	}
	return result, nil
}

// pathInRepo returns configAbs relative to gitRoot, through symlinks and a
// case-variant spelling, or an error when the config lies outside the
// repository: hook git reads the config from the index.
func pathInRepo(gitRoot, configAbs string) (string, error) {
	resolved := configAbs
	if dir, err := filepath.EvalSymlinks(filepath.Dir(configAbs)); err == nil {
		resolved = filepath.Join(dir, filepath.Base(configAbs))
	}
	rel, err := filepath.Rel(gitRoot, historygit.SnapToRoot(gitRoot, resolved))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("config %s is outside the repository %s: hook git reads it from the index", configAbs, gitRoot)
	}
	return rel, nil
}

// runAgentCheck runs check --format agent in process, with the pipeline's own
// warnings kept off the hook's stderr, which the host shows to the agent.
func runAgentCheck(ctx context.Context, deps *appDeps, req scanRequest) (agentout.Result, error) {
	quiet := *deps
	quiet.Stdout, quiet.Stderr = io.Discard, io.Discard
	req.formats, req.progress, req.quiet = []string{formatAgent}, "none", true
	resp, _, err := executeScan(ctx, &quiet, req, nil)
	if err != nil {
		return agentout.Result{}, err
	}
	return agentout.Build(resp.Document), nil
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
// archfit's own cache,
// which a hook run writes (fact cache, base worktrees) and which is not a
// change of the user's.
func archfitCache(statusLine string) bool {
	entry := strings.Trim(statusLine[min(3, len(statusLine)):], `"`)
	return entry == cacheDirName+"/" || strings.HasPrefix(entry, cacheDirName+"/") || strings.Contains(entry, "/"+cacheDirName+"/")
}

// configIn resolves a relative config path against the hook's working
// directory: the event cwd, not the hook process's own.
func configIn(dir, configPath string) string {
	if filepath.IsAbs(configPath) {
		return configPath
	}
	return filepath.Join(dir, configPath)
}

// systemMessage writes the Claude Code hook output that shows a message to
// the user without blocking: exit 0 with {"systemMessage": ...} on stdout.
func systemMessage(deps *appDeps, msg string) error {
	if err := json.NewEncoder(deps.Stdout).Encode(map[string]string{"systemMessage": strings.TrimSuffix(msg, "\n")}); err != nil {
		return &exitError{code: 1, msg: fmt.Sprintf("archfit hook: write the system message: %v", err)}
	}
	return nil
}
