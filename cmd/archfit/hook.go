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
	"path/filepath"
	"strings"

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

It runs check --format agent --base <ref> in the current directory, over the
files on disk: with pre-commit that is the staged content plus untracked
files (pre-commit stashes unstaged edits); a hook installed directly in
.git/hooks also sees unstaged edits. Exit 1 with the repair on stderr when
the next action is repair or ask_owner and a repair is in scope, 0 otherwise
(other actions are printed), and 3 when archfit cannot run.`
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
	result, err := hookResult(context.Background(), deps, c.Config, c.Base)
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

// hookResult runs check --format agent in process, with the pipeline's own
// warnings kept off the hook's stderr, which the host shows to the agent.
func hookResult(ctx context.Context, deps *appDeps, configPath, base string) (agentout.Result, error) {
	// Before the first commit HEAD names nothing: the whole tree is the
	// change, so every blocker is in scope.
	if base != "" && !refExists(ctx, deps.Runner, filepath.Dir(configPath), base) {
		base = ""
	}
	quiet := *deps
	quiet.Stdout, quiet.Stderr = io.Discard, io.Discard
	resp, _, err := executeScan(ctx, &quiet, scanRequest{
		configPath: configPath, baseRef: base, formats: []string{formatAgent}, progress: "none", quiet: true,
	}, nil)
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
