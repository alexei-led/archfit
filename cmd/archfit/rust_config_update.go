package main

import (
	"strings"

	"github.com/alexei-led/archfit/internal/config"
)

const (
	rustDeepLangSection = "languages:\n  rust:\n    enabled: auto\n\n"
	rustDeepAnalyzers   = "analyzers:\n  cargo_modules:\n    enabled: true\n  scip:\n    enabled: true\n\n"
)

func needsRustDeepAnalysisConfig(cfg config.Config, hasRust bool) bool {
	if !hasRust || cfg.Languages.Rust.Enabled == config.ModeOff {
		return false
	}
	return cfg.Languages.Rust.Enabled != config.ModeAuto ||
		cfg.Analyzers.CargoModules.Enabled != config.ModeOn ||
		cfg.Analyzers.Scip.Enabled != config.ModeOn
}

func ensureRustDeepAnalysisConfig(src []byte) []byte {
	lines := strings.Split(string(src), "\n")
	trailingNewline := len(lines) > 0 && lines[len(lines)-1] == ""

	lines = ensureRustLanguageAuto(lines)
	lines = ensureAnalyzerEnabled(lines, "cargo_modules")
	lines = ensureAnalyzerEnabled(lines, "scip")

	out := strings.Join(lines, "\n")
	if trailingNewline && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out)
}

func ensureRustLanguageAuto(lines []string) []string {
	start, end := topSection(lines, "languages")
	if start < 0 {
		return insertLines(lines, topInsertAfter(lines, "coupling"), strings.Split(strings.TrimSuffix(rustDeepLangSection, "\n"), "\n")...)
	}

	rustStart, rustEnd := childSection(lines, start+1, end, 2, "rust")
	if rustStart < 0 {
		return insertLines(lines, end, "  rust:", "    enabled: auto")
	}

	if enabled := childScalarLine(lines, rustStart+1, rustEnd, 4, "enabled"); enabled >= 0 {
		lines[enabled] = "    enabled: auto"
		return lines
	}
	return insertLines(lines, rustStart+1, "    enabled: auto")
}

func ensureAnalyzerEnabled(lines []string, name string) []string {
	start, end := topSection(lines, "analyzers")
	if start < 0 {
		return insertLines(lines, topInsertAfter(lines, "languages"), strings.Split(strings.TrimSuffix(rustDeepAnalyzers, "\n"), "\n")...)
	}

	analyzerStart, analyzerEnd := childSection(lines, start+1, end, 2, name)
	if analyzerStart < 0 {
		return insertLines(lines, end, "  "+name+":", "    enabled: true")
	}

	if enabled := childScalarLine(lines, analyzerStart+1, analyzerEnd, 4, "enabled"); enabled >= 0 {
		lines[enabled] = "    enabled: true"
		return lines
	}
	return insertLines(lines, analyzerStart+1, "    enabled: true")
}

func topInsertAfter(lines []string, key string) int {
	_, end := topSection(lines, key)
	if end >= 0 {
		return end
	}
	_, end = topSection(lines, "version")
	if end >= 0 {
		return end
	}
	return len(lines)
}

func topSection(lines []string, key string) (int, int) {
	start := -1
	prefix := key + ":"
	for i, line := range lines {
		if isTopKey(line, prefix) {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if isAnyTopKey(lines[i]) {
			end = i
			break
		}
	}
	return start, end
}

func childSection(lines []string, start, end, indent int, key string) (int, int) {
	sectionStart := -1
	prefix := strings.Repeat(" ", indent) + key + ":"
	for i := start; i < end; i++ {
		if hasIndentedKey(lines[i], indent, prefix) {
			sectionStart = i
			break
		}
	}
	if sectionStart < 0 {
		return -1, -1
	}
	sectionEnd := end
	for i := sectionStart + 1; i < end; i++ {
		if countLeadingSpaces(lines[i]) == indent && strings.Contains(strings.TrimSpace(lines[i]), ":") {
			sectionEnd = i
			break
		}
	}
	return sectionStart, sectionEnd
}

func childScalarLine(lines []string, start, end, indent int, key string) int {
	prefix := strings.Repeat(" ", indent) + key + ":"
	for i := start; i < end; i++ {
		if hasIndentedKey(lines[i], indent, prefix) {
			return i
		}
	}
	return -1
}

func hasIndentedKey(line string, indent int, prefix string) bool {
	return len(line) >= indent && strings.TrimSpace(line[:indent]) == "" && strings.HasPrefix(line, prefix)
}

func insertLines(lines []string, at int, add ...string) []string {
	if at < 0 {
		at = 0
	}
	if at > len(lines) {
		at = len(lines)
	}
	out := make([]string, 0, len(lines)+len(add))
	out = append(out, lines[:at]...)
	out = append(out, add...)
	out = append(out, lines[at:]...)
	return out
}

func isTopKey(line, prefix string) bool {
	return line == prefix || strings.HasPrefix(line, prefix+" ") || strings.HasPrefix(line, prefix+" #")
}

func isAnyTopKey(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return false
	}
	return strings.Contains(line, ":")
}

func countLeadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}
