package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	claudePluginID = "archfit@archfit"
	pluginDisabled = "disabled"
	pluginUnknown  = "unknown"
)

// claudePluginStatus reports configuration on disk, not a running session.
func claudePluginStatus(repo string) harnessStatus {
	home := os.Getenv("CLAUDE_CONFIG_DIR")
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".claude")
		}
	}
	st := harnessStatus{Name: "claude plugin", Config: filepath.Join(home, "settings.json"), Hooks: []hookStatus{{Event: claudePluginID, Status: pluginUnknown}}}
	result := &st.Hooks[0]
	if home == "" {
		result.Reason = "Claude config directory unavailable"
		return st
	}
	enabled := false
	present := false
	disabled := false
	for _, path := range []string{st.Config, filepath.Join(repo, ".claude", "settings.json"), filepath.Join(repo, ".claude", "settings.local.json")} {
		raw, err := os.ReadFile(path) //nolint:gosec // Claude settings or installation record
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			result.Reason = "unreadable settings: " + err.Error()
			return st
		}
		var settings struct {
			Enabled map[string]bool `json:"enabledPlugins"`
			Disable *bool           `json:"disableAllHooks"`
		}
		if err := json.Unmarshal(raw, &settings); err != nil {
			result.Reason = "malformed settings: " + err.Error()
			return st
		}
		if value, ok := settings.Enabled[claudePluginID]; ok {
			enabled, present = value, true
		}
		if settings.Disable != nil {
			disabled = *settings.Disable
		}
	}
	if disabled {
		result.Status = pluginDisabled
		result.Reason = "disableAllHooks set"
		return st
	}
	path := filepath.Join(home, "plugins", "installed_plugins.json")
	raw, err := os.ReadFile(path) //nolint:gosec // Claude settings or installation record
	if errors.Is(err, fs.ErrNotExist) {
		switch {
		case present && enabled:
			result.Reason = "enabled but installation record missing"
		case present:
			result.Status = pluginDisabled
		default:
			result.Status = "absent"
		}
		return st
	}
	if err != nil {
		result.Reason = "unreadable installation record: " + err.Error()
		return st
	}
	var record struct {
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &record); err != nil || record.Plugins == nil {
		result.Reason = "malformed installation record"
		return st
	}
	entries := record.Plugins[claudePluginID]
	if !present {
		if len(entries) == 0 {
			result.Status = "absent"
		} else {
			result.Status = pluginUnknown
			result.Reason = "enablement not recorded"
		}
		return st
	}
	if !enabled {
		result.Status = pluginDisabled
		return st
	}
	for _, e := range entries {
		if e.Scope != "user" {
			continue
		}
		if validClaudePlugin(e.InstallPath) {
			st.Detected = true
			st.DetectedBy = "configured and installed; session load not verified"
			result.Status = "active"
			return st
		}
	}
	result.Reason = "enabled but user installation is unavailable"
	return st
}

func writeClaudePlugin(w io.Writer, plugin harnessStatus) {
	status := plugin.Hooks[0]
	_, _ = fmt.Fprintf(w, "  Claude plugin %s: %s", status.Event, status.Status)
	if status.Reason != "" {
		_, _ = fmt.Fprintf(w, " (%s)", status.Reason)
	}
	if status.Status == hookActive {
		_, _ = fmt.Fprint(w, " (configured and installed; session load not verified)")
	}
	_, _ = fmt.Fprintln(w)
}

// validClaudePlugin checks the installed components before calling them active.
func validClaudePlugin(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json")) //nolint:gosec // installed plugin path
	if err != nil {
		return false
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Name != "archfit" {
		return false
	}
	raw, err = os.ReadFile(filepath.Join(dir, "hooks", "hooks.json")) //nolint:gosec // installed plugin path
	if err != nil {
		return false
	}
	var config struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return false
	}
	for _, want := range harnessHooks(harnessClaude) {
		found := false
		for _, group := range config.Hooks[want.event] {
			if group.Matcher != want.matcher {
				continue
			}
			for _, hook := range group.Hooks {
				if hook.Type == "command" && strings.Contains(hook.Command, hookRunLine(want.run, harnessClaude)) {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}
