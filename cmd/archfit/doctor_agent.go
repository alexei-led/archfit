package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/alexei-led/archfit/v3/internal/application"
	historygit "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// doctorSchemaVersion versions the `archfit doctor --format json` document.
const doctorSchemaVersion = "archfit.doctor.v1"

// Hook states doctor reports. A hook that is not installed, not trusted, or
// not archfit's is inactive: the harness will not run it.
const (
	hookActive   = "active"
	hookInactive = "inactive"
)

// Why a hook is inactive, or what its active state rests on.
const (
	reasonNotInstalled    = "not installed"
	reasonNotTrusted      = "not trusted yet"
	reasonHashNotVerified = "hash not verified"
)

// doctorReport is the `archfit doctor --format json` document.
type doctorReport struct {
	SchemaVersion string    `json:"schema_version"`
	Tools         []toolRow `json:"tools"`
	AgentLoop     agentLoop `json:"agent_loop"`
}

// toolRow is one analyzer or helper tool: ok (found), missing, or invalid (a
// configured value it cannot use, which fails doctor as it fails analysis).
type toolRow struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Path        string `json:"path,omitempty"`
	InstallHint string `json:"install_hint,omitempty"`
	Error       string `json:"error,omitempty"`
}

// agentLoop is whether the agent feedback loop runs in this repository: the
// hooks each harness would run, and the scope a check would resolve.
type agentLoop struct {
	Harnesses []harnessStatus `json:"harnesses"`
	Scope     scopeStatus     `json:"scope"`
}

// harnessStatus is one harness: whether it is here and which of archfit's
// hooks it would run.
type harnessStatus struct {
	Name       string       `json:"name"`
	Detected   bool         `json:"detected"`
	DetectedBy string       `json:"detected_by,omitempty"`
	Config     string       `json:"config,omitempty"`
	Hooks      []hookStatus `json:"hooks"`
}

// hookStatus is one hook archfit installs for a harness.
type hookStatus struct {
	Event  string `json:"event"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// scopeStatus is what check would compare a change with from here, and
// whether a policy file differs from the target tip.
type scopeStatus struct {
	Mode          string   `json:"mode"`
	Target        string   `json:"target,omitempty"`
	TargetSHA     string   `json:"target_sha,omitempty"`
	Base          string   `json:"base,omitempty"`
	Unresolved    string   `json:"unresolved,omitempty"`
	PolicyChanged []string `json:"policy_changed"`
	PolicyStale   []string `json:"policy_stale"`
	Error         string   `json:"error,omitempty"`
}

// agentLoopStatus reads the agent loop of the repository around the working
// directory. It runs git and reads files; it changes nothing.
func agentLoopStatus(ctx context.Context, deps *appDeps, targetBranch string) agentLoop {
	repo := "."
	if root, err := historygit.RepoRoot(ctx, repo, deps.Runner); err == nil {
		repo = root
	}
	if abs, err := filepath.Abs(repo); err == nil {
		repo = abs
	}
	return agentLoop{
		Harnesses: []harnessStatus{
			harnessStatusOf(ctx, deps, repo, harnessClaude),
			harnessStatusOf(ctx, deps, repo, harnessCodex),
			gitHookStatus(ctx, deps),
		},
		Scope: scopeStatusOf(ctx, deps, targetBranch),
	}
}

// harnessStatusOf reports a Claude Code or Codex harness. A Codex hook counts
// as active only when Codex keeps a trust entry for it; doctor cannot
// recompute the trusted hash, so it says the hash is not verified.
func harnessStatusOf(ctx context.Context, deps *appDeps, repo, harness string) harnessStatus {
	st := harnessStatus{Name: harness, Config: filepath.Join(repo, harnessConfigFile(harness))}
	st.Detected, st.DetectedBy = detectHarness(ctx, deps, repo, harness)
	positions, readErr := installedHooks(st.Config)
	var trusted map[string]bool
	disabledBy := ""
	switch harness {
	case harnessCodex:
		trusted = codexTrustedHooks(filepath.Join(codexHome(), "config.toml"))
	case harnessClaude:
		disabledBy = claudeHooksDisabledBy(repo)
	}
	for _, h := range harnessHooks(harness) {
		hs := hookStatus{Event: h.event, Status: hookInactive}
		pos, installed := positions[h.event+" "+h.run]
		switch {
		case readErr != nil:
			hs.Reason = "unreadable config: " + readErr.Error()
		case !installed:
			hs.Reason = reasonNotInstalled
		case disabledBy != "":
			hs.Reason = "disableAllHooks is set in " + disabledBy
		case harness != harnessCodex:
			hs.Status = hookActive
		case codexTrusts(trusted, st.Config, h.event, pos):
			hs.Status, hs.Reason = hookActive, reasonHashNotVerified
		default:
			hs.Reason = reasonNotTrusted
		}
		st.Hooks = append(st.Hooks, hs)
	}
	return st
}

// claudeHooksDisabledBy names the project settings file whose disableAllHooks
// makes Claude Code run no hook, or "" when none does. settings.local.json
// overrides settings.json, as it does in Claude Code.
func claudeHooksDisabledBy(repo string) string {
	for _, name := range []string{"settings.local.json", "settings.json"} {
		rel := filepath.Join(".claude", name)
		raw, err := os.ReadFile(filepath.Join(repo, rel)) //nolint:gosec // the Claude Code settings of this repository
		if err != nil {
			continue
		}
		var settings struct {
			DisableAllHooks *bool `json:"disableAllHooks"`
		}
		if json.Unmarshal(raw, &settings) != nil || settings.DisableAllHooks == nil {
			continue
		}
		if *settings.DisableAllHooks {
			return rel
		}
		return ""
	}
	return ""
}

// detectHarness reports whether a harness is in use here, and how doctor knows.
func detectHarness(ctx context.Context, deps *appDeps, repo, harness string) (bool, string) {
	if harness == harnessClaude {
		if info, err := os.Stat(filepath.Join(repo, ".claude")); err == nil && info.IsDir() {
			return true, ".claude/ in the repository"
		}
	}
	if _, ok := deps.Runner.Detect(ctx, harness); ok {
		return true, harness + " on PATH"
	}
	if harness == harnessCodex {
		if info, err := os.Stat(codexHome()); err == nil && info.IsDir() {
			return true, codexHome() + " exists"
		}
	}
	return false, ""
}

// codexHome is the directory Codex keeps its config in: CODEX_HOME, else ~/.codex.
func codexHome() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

// hookPosition is where an archfit hook sits in a hook file: the index of its
// group in the event's list and of the hook in the group. Codex keys its
// trust by it.
type hookPosition struct{ group, hook int }

// installedHooks finds archfit's hooks in a Claude Code or Codex hook file,
// keyed by "<event> <hook run event>". A missing file installs nothing.
func installedHooks(path string) (map[string]hookPosition, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the harness config file of this repository
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var file struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	out := map[string]hookPosition{}
	for event, groups := range file.Hooks {
		for g, group := range groups {
			for h, hook := range group.Hooks {
				run, ok := strings.CutPrefix(strings.TrimSpace(hook.Command), hookRunCommand+" ")
				fields := strings.Fields(run)
				if !ok || len(fields) == 0 {
					continue
				}
				key := event + " " + fields[0]
				if _, seen := out[key]; !seen {
					out[key] = hookPosition{group: g, hook: h}
				}
			}
		}
	}
	return out, nil
}

// codexTrustedHooks reads the hook keys Codex trusts from its config.toml: a
// [hooks.state."<key>"] table that holds a trusted_hash. It reads lines, not
// TOML: the tables Codex writes are one header and its keys.
func codexTrustedHooks(path string) map[string]bool {
	raw, err := os.ReadFile(path) //nolint:gosec // the Codex user config
	if err != nil {
		return nil
	}
	trusted := map[string]bool{}
	table := ""
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			table = ""
			if key, ok := strings.CutPrefix(line, `[hooks.state."`); ok {
				table, _ = strings.CutSuffix(key, `"]`)
			}
			continue
		}
		if table != "" && strings.HasPrefix(line, "trusted_hash") {
			trusted[table] = true
		}
	}
	return trusted
}

// codexTrusts reports whether Codex trusts the hook at pos in the hook file:
// the key is "<absolute hooks.json>:<event in snake case>:<group>:<hook>",
// with the path as written or with its symlinks resolved.
func codexTrusts(trusted map[string]bool, hooksFile, event string, pos hookPosition) bool {
	suffix := ":" + snakeCase(event) + ":" + strconv.Itoa(pos.group) + ":" + strconv.Itoa(pos.hook)
	paths := []string{hooksFile}
	if resolved, err := filepath.EvalSymlinks(hooksFile); err == nil {
		paths = append(paths, resolved)
	}
	for _, p := range paths {
		if trusted[p+suffix] {
			return true
		}
	}
	return false
}

// snakeCase spells a hook event as Codex keys its trust: PostToolUse is
// post_tool_use.
func snakeCase(event string) string {
	var b strings.Builder
	for i, r := range event {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// gitHookStatus reports the pre-commit hook in the hooks directory git uses.
func gitHookStatus(ctx context.Context, deps *appDeps) harnessStatus {
	st := harnessStatus{Name: harnessGit}
	hs := hookStatus{Event: hookEventPreCommit, Status: hookInactive}
	hooksDir, err := gitHooksDir(ctx, deps.Runner, ".")
	if err != nil {
		hs.Reason = "not a git repository"
		st.Hooks = []hookStatus{hs}
		return st
	}
	st.Detected, st.DetectedBy = true, "a git repository"
	st.Config = filepath.Join(hooksDir, hookEventPreCommit)
	if abs, err := filepath.Abs(st.Config); err == nil {
		st.Config = abs
	}
	content, readErr := os.ReadFile(st.Config)
	info, statErr := os.Stat(st.Config)
	switch {
	case readErr != nil || statErr != nil:
		hs.Reason = reasonNotInstalled
	case !bytes.Contains(content, []byte(gitHookMarker)):
		hs.Reason = "the pre-commit hook is not archfit's"
	case info.Mode().Perm()&0o111 == 0:
		hs.Reason = "not executable"
	default:
		hs.Status = hookActive
	}
	st.Hooks = []hookStatus{hs}
	return st
}

// scopeStatusOf resolves the scope check would use here, with its defaults:
// --base auto and the default target.
func scopeStatusOf(ctx context.Context, deps *appDeps, targetBranch string) scopeStatus {
	res, err := resolveScope(ctx, deps.Runner, ".", defaultConfigPath, application.ScopeRequest{ConfigTarget: targetBranch})
	st := scopeStatus{
		Mode: res.Mode, Target: res.Target, TargetSHA: res.TargetSHA, Base: res.Base, Unresolved: res.Unresolved,
		PolicyChanged: append([]string{}, res.PolicyChanged...), PolicyStale: append([]string{}, res.PolicyStale...),
	}
	if err != nil {
		st.Error = err.Error()
	}
	return st
}

// writeAgentLoop prints the agent loop section of the text report.
func writeAgentLoop(w io.Writer, loop agentLoop) {
	_, _ = fmt.Fprintln(w, "\nAgent loop (install hooks: archfit hook install --harness claude|codex|git):")
	for _, h := range loop.Harnesses {
		detected := "not detected"
		if h.Detected {
			detected = "detected (" + h.DetectedBy + ")"
		}
		_, _ = fmt.Fprintf(w, "  %s: %s", h.Name, detected)
		if h.Config != "" {
			_, _ = fmt.Fprintf(w, ", %s", h.Config)
		}
		_, _ = fmt.Fprintln(w)
		for _, hook := range h.Hooks {
			status := hook.Status
			if hook.Reason != "" {
				status += " (" + hook.Reason + ")"
			}
			_, _ = fmt.Fprintf(w, "    %s: %s\n", hook.Event, status)
		}
	}
	s := loop.Scope
	switch {
	case s.Error != "":
		_, _ = fmt.Fprintf(w, "  target branch: not resolved: %s\n", s.Error)
	case s.Unresolved != "":
		_, _ = fmt.Fprintf(w, "  target branch: not resolved: %s\n", s.Unresolved)
	case s.Target != "":
		_, _ = fmt.Fprintf(w, "  target branch: %s (%s)\n", s.Target, shortSHA(s.TargetSHA))
	}
	scope := s.Mode
	if s.Base != "" {
		scope += ", base " + shortSHA(s.Base)
	} else if s.Mode == report.ScopeModeNone || s.Mode == report.ScopeModeTargetBranch {
		scope += ", the whole tree is the change"
	}
	_, _ = fmt.Fprintf(w, "  scope: %s\n", scope)
	switch {
	case len(s.PolicyChanged) > 0:
		_, _ = fmt.Fprintf(w, "  policy guard: changed on this branch: %s (an owner approves it)\n", strings.Join(s.PolicyChanged, ", "))
	case len(s.PolicyStale) > 0:
		_, _ = fmt.Fprintf(w, "  policy guard: changed on the target since the fork: %s (merge or rebase)\n", strings.Join(s.PolicyStale, ", "))
	default:
		_, _ = fmt.Fprintln(w, "  policy guard: no policy file differs from the target")
	}
}

// shortSHA shortens a commit SHA for the text report; a ref name stays whole.
func shortSHA(sha string) string {
	if len(sha) == 40 || len(sha) == 64 {
		return sha[:12]
	}
	return sha
}
