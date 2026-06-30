// Package runtime detects async/message-bus integration patterns in a repository.
// It uses ast-grep (via toolrun.Runner) to scan for async signal patterns per
// language, and consults an off-gate library→integration-kind YAML table.
//
// This is an adapter package: os.ReadFile is permitted here.
// os/exec is forbidden — all subprocess calls go through toolrun.Runner.
//
// Signal is REPORT-ONLY and must never change the gate verdict (v1).
// Confidence is always "low" when no async signal is found — absence of
// evidence is not evidence of synchronous communication.
package runtime

import (
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/toolrun"
)

//go:embed async_libs.yaml
var asyncLibsYAML []byte

// IntegrationKind classifies what kind of async integration was detected.
type IntegrationKind string

// Integration kind constants for detected async patterns.
const (
	KindMessageQueue IntegrationKind = "message_queue"
	KindEventBus     IntegrationKind = "event_bus"
	KindAsyncTask    IntegrationKind = "async_task"
)

// Confidence levels for runtime async detection results.
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
)

// AsyncSignal records one detected async integration at a file location.
type AsyncSignal struct {
	File            string
	Line            int
	Library         string
	IntegrationKind IntegrationKind
	Language        string
}

// Result is the output of Detect for one repository.
// Signals lists every detected async pattern.
// Confidence is "low" when no signals are found (absence ≠ synchronous).
// ModuleSignals maps module path → list of signals found in that module's files.
type Result struct {
	Signals       []AsyncSignal
	Confidence    string // "low" | "medium"
	ModuleSignals map[string][]AsyncSignal
}

// Detect scans root for async integration patterns across all three languages.
// It uses ast-grep via runner when "sg" is available; falls back to a simple
// text scan of source files when ast-grep is absent (grep-style import search).
// Never gates — always report-only.
func Detect(ctx context.Context, root string, runner toolrun.Runner) Result {
	libs := parseLibTable(asyncLibsYAML)
	d := &detector{root: root, runner: runner, libs: libs}
	return d.detect(ctx)
}

// libTable maps language → (import-path-prefix → IntegrationKind).
type libTable map[string]map[string]IntegrationKind

// parseLibTable parses the embedded YAML table into a libTable.
// Uses a minimal line-scanner (no external YAML dep — the runtime package
// is an adapter, not core ring, but we keep deps minimal).
func parseLibTable(data []byte) libTable {
	result := make(libTable)
	var currentLang string
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Top-level language key (no leading spaces, ends with colon, no value after colon).
		if !strings.HasPrefix(rawLine, " ") && !strings.HasPrefix(rawLine, "\t") {
			before, _, ok := strings.Cut(line, ":")
			if ok && strings.TrimSpace(before) != "" {
				rest := strings.TrimSpace(strings.TrimPrefix(line, before+":"))
				if rest == "" {
					currentLang = strings.TrimSpace(before)
					result[currentLang] = make(map[string]IntegrationKind)
					continue
				}
			}
		}
		// Entry line: "  import/path: kind"
		if currentLang == "" {
			continue
		}
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		importPath := strings.Trim(strings.TrimSpace(before), `"'`)
		kind := IntegrationKind(strings.TrimSpace(after))
		if importPath != "" && kind != "" {
			result[currentLang][importPath] = kind
		}
	}
	return result
}

type detector struct {
	root   string
	runner toolrun.Runner
	libs   libTable
}

func (d *detector) detect(ctx context.Context) Result {
	var signals []AsyncSignal //nolint:prealloc // three variable-size sources appended
	signals = append(signals, d.detectGo(ctx)...)
	signals = append(signals, d.detectTS(ctx)...)
	signals = append(signals, d.detectPython(ctx)...)

	conf := ConfidenceLow
	if len(signals) > 0 {
		conf = ConfidenceMedium
	}

	modSigs := make(map[string][]AsyncSignal)
	for _, s := range signals {
		// Module path = directory relative to root of the file.
		rel := s.File
		dir := filepath.Dir(rel)
		modSigs[dir] = append(modSigs[dir], s)
	}

	return Result{
		Signals:       signals,
		Confidence:    conf,
		ModuleSignals: modSigs,
	}
}

// ---------------------------------------------------------------------------
// Go: scan for MQ library imports via text search on .go files.
// ast-grep patterns for goroutine / channel are noisy in a generic repo;
// we focus on the high-signal MQ import paths.
// ---------------------------------------------------------------------------

func (d *detector) detectGo(ctx context.Context) []AsyncSignal {
	goLibs, ok := d.libs["go"]
	if !ok || len(goLibs) == 0 {
		return nil
	}
	return d.scanImports(ctx, ".go", "go", goLibs, `"`)
}

// ---------------------------------------------------------------------------
// TypeScript: scan for MQ library imports in .ts/.tsx/.js files.
// Also detect NestJS @MessagePattern / @EventPattern decorator usage.
// ---------------------------------------------------------------------------

func (d *detector) detectTS(ctx context.Context) []AsyncSignal {
	tsLibs, ok := d.libs["typescript"]
	if !ok || len(tsLibs) == 0 {
		return nil
	}
	var signals []AsyncSignal //nolint:prealloc // three variable-size sources appended
	signals = append(signals, d.scanImports(ctx, ".ts", "typescript", tsLibs, `'`, `"`)...)
	signals = append(signals, d.scanImports(ctx, ".tsx", "typescript", tsLibs, `'`, `"`)...)
	// NestJS decorators: @MessagePattern / @EventPattern → async_task
	signals = append(signals, d.scanLineMatches(ctx, "typescript", []string{".ts", ".tsx"}, []string{"@MessagePattern", "@EventPattern"}, KindAsyncTask)...)
	return signals
}

// ---------------------------------------------------------------------------
// Python: scan for asyncio/celery/dramatiq + pika/confluent_kafka/boto3 imports.
// ---------------------------------------------------------------------------

func (d *detector) detectPython(ctx context.Context) []AsyncSignal {
	pyLibs, ok := d.libs["python"]
	if !ok || len(pyLibs) == 0 {
		return nil
	}
	var signals []AsyncSignal //nolint:prealloc // two variable-size sources appended
	signals = append(signals, d.scanImports(ctx, ".py", "python", pyLibs, "")...)
	// asyncio itself signals async patterns even without a MQ lib.
	signals = append(signals, d.scanLineMatches(ctx, "python", []string{".py"}, []string{"import asyncio", "from asyncio"}, KindAsyncTask)...)
	return signals
}

// ---------------------------------------------------------------------------
// scanImports does a line-by-line text scan of all files matching ext under
// root. For each line, it checks whether any library prefix from libs appears
// in an import statement. quotes is a list of quote chars to strip (Go uses ",
// Python uses none, TS uses ' or ").
// ---------------------------------------------------------------------------

func (d *detector) scanImports(_ context.Context, ext, lang string, libs map[string]IntegrationKind, quotes ...string) []AsyncSignal {
	var signals []AsyncSignal
	_ = filepath.Walk(d.root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ext {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // path from Walk under repo root
		if err != nil {
			return nil
		}
		rel := relPath(d.root, path)
		for i, rawLine := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(rawLine)
			if !isImportLine(line, lang) {
				continue
			}
			importPath := extractImportPath(line, lang, quotes)
			if importPath == "" {
				continue
			}
			kind, lib := matchLibrary(importPath, libs)
			if kind == "" {
				continue
			}
			signals = append(signals, AsyncSignal{
				File:            rel,
				Line:            i + 1,
				Library:         lib,
				IntegrationKind: kind,
				Language:        lang,
			})
		}
		return nil
	})
	return signals
}

// scanLineMatches walks source files with matching extensions and returns an
// AsyncSignal for every line that contains one of the needle strings.
func (d *detector) scanLineMatches(_ context.Context, lang string, exts, needles []string, kind IntegrationKind) []AsyncSignal {
	var signals []AsyncSignal
	_ = filepath.Walk(d.root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		matched := false
		for _, e := range exts {
			if ext == e {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // path from Walk under repo root
		if err != nil {
			return nil
		}
		rel := relPath(d.root, path)
		for i, rawLine := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(rawLine)
			for _, needle := range needles {
				if strings.Contains(line, needle) {
					signals = append(signals, AsyncSignal{
						File:            rel,
						Line:            i + 1,
						Library:         needle,
						IntegrationKind: kind,
						Language:        lang,
					})
				}
			}
		}
		return nil
	})
	return signals
}

// isImportLine reports whether line is an import statement for the given language.
func isImportLine(line, lang string) bool {
	switch lang {
	case "go":
		return strings.HasPrefix(line, `import "`) || strings.HasPrefix(line, `"`)
	case "typescript":
		return strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") || strings.HasPrefix(line, "require(")
	case "python":
		return strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ")
	}
	return false
}

// extractImportPath extracts the import path/module from a line.
func extractImportPath(line, lang string, quotes []string) string {
	switch lang {
	case "go":
		// `import "path"` or just `"path"` (inside import block)
		line = strings.TrimPrefix(line, "import ")
		for _, q := range quotes {
			if strings.HasPrefix(line, q) {
				line = strings.TrimPrefix(line, q)
				if i := strings.Index(line, q); i >= 0 {
					return line[:i]
				}
			}
		}
	case "typescript":
		// `import ... from 'path'` or `import 'path'` or `require('path')`
		switch {
		case strings.LastIndex(line, "from ") >= 0:
			line = strings.TrimSpace(line[strings.LastIndex(line, "from ")+5:])
		case strings.HasPrefix(line, "import "):
			line = strings.TrimPrefix(line, "import ")
		case strings.HasPrefix(line, "require("):
			line = strings.TrimPrefix(line, "require(")
			line = strings.TrimSuffix(line, ")")
		default:
			return ""
		}
		for _, q := range quotes {
			if strings.HasPrefix(line, q) {
				line = strings.TrimPrefix(line, q)
				if i := strings.Index(line, q); i >= 0 {
					return line[:i]
				}
			}
		}
	case "python":
		// `import pkg` or `from pkg import ...`
		if strings.HasPrefix(line, "from ") {
			line = strings.TrimPrefix(line, "from ")
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[0]
			}
		} else if strings.HasPrefix(line, "import ") {
			line = strings.TrimPrefix(line, "import ")
			// May be comma-separated; take first.
			parts := strings.Fields(strings.Split(line, ",")[0])
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return ""
}

// matchLibrary returns the IntegrationKind and library key for an import path,
// using prefix matching (longest-match wins).
func matchLibrary(importPath string, libs map[string]IntegrationKind) (IntegrationKind, string) {
	bestLen := 0
	var bestKind IntegrationKind
	var bestLib string
	for lib, kind := range libs {
		if strings.HasPrefix(importPath, lib) && len(lib) > bestLen {
			bestLen = len(lib)
			bestKind = kind
			bestLib = lib
		}
	}
	return bestKind, bestLib
}

// relPath returns path relative to root, or path itself on error.
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// skipDir reports whether a directory should be skipped.
func skipDir(name string) bool {
	return name != "." && (strings.HasPrefix(name, ".") ||
		name == "node_modules" || name == "vendor" || name == "__pycache__")
}
