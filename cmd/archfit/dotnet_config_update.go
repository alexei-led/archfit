package main

import (
	"strings"

	"github.com/alexei-led/archfit/v3/internal/config"
)

const dotnetTypeGraphAnalyzers = "analyzers:\n  roslyn:\n    enabled: true\n\n"

// needsDotNetTypeGraphConfig reports whether a present .NET project leaves the
// Roslyn analyzer at its implicit default. An explicit true or false is kept.
func needsDotNetTypeGraphConfig(cfg config.Config, hasDotNet bool) bool {
	if !hasDotNet || cfg.Languages.DotNet.Enabled == config.ModeOff {
		return false
	}
	return cfg.Analyzers.Roslyn.Enabled != config.ModeOn && cfg.Analyzers.Roslyn.Enabled != config.ModeOff
}

// ensureDotNetTypeGraphConfig writes `analyzers.roslyn.enabled: true`, adding
// the analyzers section when the config has none.
func ensureDotNetTypeGraphConfig(src []byte, cfg config.Config) []byte {
	lines := strings.Split(string(src), "\n")
	trailingNewline := len(lines) > 0 && len(lines[len(lines)-1]) == 0
	if start, _ := topSection(lines, "analyzers"); start < 0 {
		if cfg.Analyzers.Roslyn.Enabled != config.ModeOn && cfg.Analyzers.Roslyn.Enabled != config.ModeOff {
			lines = insertLines(lines, topInsertAfter(lines, "languages"), strings.Split(strings.TrimSuffix(dotnetTypeGraphAnalyzers, "\n"), "\n")...)
		}
	} else {
		lines = ensureAnalyzerEnabled(lines, "roslyn", cfg.Analyzers.Roslyn.Enabled)
	}
	out := strings.Join(lines, "\n")
	if trailingNewline && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out)
}
