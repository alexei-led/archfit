package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// hookRunCommand starts every command archfit installs: an entry that starts
// with it is archfit's, and installing again replaces it.
const hookRunCommand = "archfit hook run"

// legacyHookCommand is the deleted Stop hook command; install removes it.
const legacyHookCommand = "archfit hook claude"

// gitHookMarker marks a pre-commit hook archfit wrote.
const gitHookMarker = "# Written by archfit hook install --harness git."

// codexTrustNotice is printed after the Codex hooks are written: Codex runs a
// hook only once a user trusts it, and archfit cannot.
const codexTrustNotice = "codex runs these hooks only after you trust them: start codex and approve the hooks, or run it once with --dangerously-bypass-hook-trust"

// harnessHook is one hook archfit installs into a harness.
type harnessHook struct {
	// event is the harness's event name, as its config file keys it.
	event string
	// matcher selects the tools a PostToolUse hook runs after.
	matcher string
	// run is the archfit hook run event the hook calls.
	run string
}

// harnessHooks lists the hooks archfit installs into a harness's config file.
func harnessHooks(harness string) []harnessHook {
	switch harness {
	case harnessClaude:
		return []harnessHook{
			{event: "SessionStart", run: hookEventSessionStart},
			{event: "PostToolUse", matcher: "Edit|Write|MultiEdit|NotebookEdit|Bash", run: hookEventPostToolUse},
			{event: "Stop", run: hookEventStop},
			{event: "SubagentStop", run: hookEventStop},
		}
	case harnessCodex:
		return []harnessHook{
			{event: "SessionStart", run: hookEventSessionStart},
			{event: "PostToolUse", matcher: "^apply_patch$|^Bash$", run: hookEventPostToolUse},
			{event: "Stop", run: hookEventStop},
		}
	default:
		return nil
	}
}

// harnessConfigFile is the project file a harness reads its hooks from,
// relative to the repository. Codex loads hooks from every config layer and
// appends them, so the project file adds to a user's own hooks.
func harnessConfigFile(harness string) string {
	if harness == harnessCodex {
		return filepath.Join(".codex", "hooks.json")
	}
	return filepath.Join(".claude", "settings.json")
}

// HookInstallCmd writes the hook configuration for one harness.
type HookInstallCmd struct {
	Harness string `required:"" enum:"claude,codex,git" help:"Harness to install for: claude (.claude/settings.json), codex (.codex/hooks.json), or git (the pre-commit hook)."`
	Dir     string `help:"Repository to install into." default:"." type:"path"`
	Config  string `short:"c" help:"Config path the installed hooks pass to archfit hook run, relative to the repository root." default:".archfit.yaml"`
}

func (*HookInstallCmd) Help() string {
	return `Write the archfit hooks into the configuration of one harness.

  claude  merges into <dir>/.claude/settings.json: SessionStart, PostToolUse
          (after Edit, Write, MultiEdit, NotebookEdit, Bash), Stop, and
          SubagentStop, each running archfit hook run <event> --harness claude
  codex   merges into <dir>/.codex/hooks.json: SessionStart, PostToolUse
          (after apply_patch and Bash), and Stop. Codex runs a hook only after
          you trust it: start codex and approve the hooks.
  git     writes the pre-commit hook in the hooks directory git uses
          (core.hooksPath when set), running archfit hook run pre-commit

Every other key and hook in a settings file stays. An archfit hook run entry
already there is replaced, so installing twice changes nothing, and an entry of
the removed archfit hook claude command is dropped. A pre-commit hook archfit
did not write is left alone: the command exits 3 and names it.

It prints one line per file it wrote. Exit 0, or 3 on an error.

Examples:
  archfit hook install --harness claude
  archfit hook install --harness codex --dir ../service
  archfit hook install --harness git -c policy/archfit.yaml`
}

func (c *HookInstallCmd) Run(deps *appDeps) error {
	var (
		path string
		err  error
	)
	if c.Harness == harnessGit {
		path, err = installGitHook(deps.runContext(), deps.Runner, c.Dir, c.Config)
	} else {
		path, err = installHarnessHooks(c.Dir, c.Harness, c.Config)
	}
	if err != nil {
		return &exitError{code: 3, msg: "error: " + err.Error()}
	}
	_, _ = fmt.Fprintf(deps.Stdout, "wrote %s\n", path)
	if c.Harness == harnessCodex {
		_, _ = fmt.Fprintln(deps.Stdout, codexTrustNotice)
	}
	return nil
}

// hookRunLine is the command an installed hook runs.
func hookRunLine(run, harness, configPath string) string {
	line := hookRunCommand + " " + run
	if harness != "" {
		line += " --harness " + harness
	}
	if configPath != "" && configPath != defaultConfigPath {
		line += " -c " + shellQuote(configPath)
	}
	return line
}

// shellQuote quotes a word for sh when it holds anything but plain path
// characters.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// installHarnessHooks merges archfit's hooks into a Claude Code or Codex hook
// file and returns its path.
func installHarnessHooks(dir, harness, configPath string) (string, error) {
	path := filepath.Join(dir, harnessConfigFile(harness))
	raw, err := os.ReadFile(path) //nolint:gosec // the harness config file under the repository the user named
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	merged, err := mergeHookConfig(raw, harnessHooks(harness), func(run string) string {
		return hookRunLine(run, harness, configPath)
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	// A settings file kept as a symlink (a dotfiles checkout) is written
	// through the link, with the mode it has.
	target, perm := path, os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			target = resolved
		}
	}
	if err := writeFileAtomic(target, merged, perm); err != nil {
		return "", err
	}
	return path, nil
}

// hookGroup is one matcher group of a Claude Code or Codex hook file.
type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// hookCommand is one command hook.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// mergeHookConfig adds archfit's hooks to a Claude Code or Codex hook file.
// Every other key, event, group, and hook keeps its value. A group that holds
// only archfit hook run entries is replaced in place, so installing again
// changes nothing and keeps each hook's position, which Codex keys its trust
// by. An archfit hook run entry in a group shared with other hooks is moved
// into archfit's own group; an archfit hook claude entry is dropped.
func mergeHookConfig(raw []byte, hooks []harnessHook, command func(run string) string) ([]byte, error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &top); err != nil {
			return nil, fmt.Errorf("not a JSON object: %w", err)
		}
	}
	events := map[string][]json.RawMessage{}
	if h, ok := top["hooks"]; ok {
		if err := json.Unmarshal(h, &events); err != nil {
			return nil, fmt.Errorf(`"hooks" is not an object of event lists: %w`, err)
		}
	}
	if events == nil {
		events = map[string][]json.RawMessage{}
	}
	for name, groups := range events {
		merged := mergeEvent(groups, nil, command)
		if len(groups) > 0 && len(merged) == 0 {
			// The event held only the removed archfit hook claude.
			delete(events, name)
			continue
		}
		events[name] = merged
	}
	for i := range hooks {
		events[hooks[i].event] = mergeEvent(events[hooks[i].event], &hooks[i], command)
	}
	encodedEvents, err := encodeJSON(events, false)
	if err != nil {
		return nil, err
	}
	top["hooks"] = encodedEvents
	return encodeJSON(top, true)
}

// mergeEvent rewrites one event's groups. want is the hook archfit installs
// for the event, or nil when it installs none there: then only legacy entries
// go.
func mergeEvent(groups []json.RawMessage, want *harnessHook, command func(run string) string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(groups)+1)
	placed := false
	for _, raw := range groups {
		var group map[string]json.RawMessage
		var entries []json.RawMessage
		if json.Unmarshal(raw, &group) != nil || json.Unmarshal(group["hooks"], &entries) != nil {
			out = append(out, raw)
			continue
		}
		rest := make([]json.RawMessage, 0, len(entries))
		ours := false
		for _, entry := range entries {
			switch cmd := entryCommand(entry); {
			case strings.HasPrefix(cmd, legacyHookCommand):
			case want != nil && strings.HasPrefix(cmd, hookRunCommand):
				ours = true
			default:
				rest = append(rest, entry)
			}
		}
		switch {
		case len(rest) == len(entries):
			out = append(out, raw)
		case len(rest) > 0:
			group["hooks"], _ = encodeJSON(rest, false)
			encoded, _ := encodeJSON(group, false)
			out = append(out, encoded)
		case ours && !placed:
			out = append(out, archfitGroup(want, command))
			placed = true
		}
	}
	if want != nil && !placed {
		out = append(out, archfitGroup(want, command))
	}
	return out
}

// entryCommand reads the command of one hook entry; "" when it has none.
func entryCommand(entry json.RawMessage) string {
	var hook struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(entry, &hook)
	return strings.TrimSpace(hook.Command)
}

// archfitGroup is the group archfit installs for one hook.
func archfitGroup(h *harnessHook, command func(run string) string) json.RawMessage {
	raw, _ := encodeJSON(hookGroup{Matcher: h.matcher, Hooks: []hookCommand{{Type: "command", Command: command(h.run)}}}, false)
	return raw
}

// encodeJSON encodes v without HTML escaping, indented when asked: a hook
// command holds &&, < and >, and a person reads the file.
func encodeJSON(v any, indent bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if !indent {
		return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
	}
	return buf.Bytes(), nil
}

// gitHooksDir is the hooks directory git runs hooks from in dir: core.hooksPath
// when set, else the repository's hooks directory.
func gitHooksDir(ctx context.Context, runner toolrun.Runner, dir string) (string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: []string{"rev-parse", "--git-path", "hooks"}, WorkDir: dir})
	if err != nil {
		return "", err
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("%s is not in a git repository: %s", dir, strings.TrimSpace(string(out.Stderr)))
	}
	hooks := strings.TrimSpace(string(out.Stdout))
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(dir, hooks)
	}
	return hooks, nil
}

// gitPreCommitScript is the pre-commit hook archfit writes.
func gitPreCommitScript(configPath string) []byte {
	return []byte("#!/bin/sh\n" + gitHookMarker + "\n" +
		"# It judges the staged content and blocks a commit whose change fails (archfit hook run --help).\n" +
		"exec " + hookRunLine(hookEventPreCommit, "", configPath) + "\n")
}

// installGitHook writes the pre-commit hook and returns its path. A hook
// archfit did not write is never replaced.
func installGitHook(ctx context.Context, runner toolrun.Runner, dir, configPath string) (string, error) {
	hooksDir, err := gitHooksDir(ctx, runner, dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(hooksDir, hookEventPreCommit)
	current, err := os.ReadFile(path) //nolint:gosec // the hook file of the repository the user named
	switch {
	case err == nil && !bytes.Contains(current, []byte(gitHookMarker)):
		return "", fmt.Errorf("%s exists and archfit did not write it: add %q to it, or remove it and run this again", path, hookRunLine(hookEventPreCommit, "", configPath))
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	if err := writeFileAtomic(path, gitPreCommitScript(configPath), 0o755); err != nil {
		return "", err
	}
	return path, nil
}
