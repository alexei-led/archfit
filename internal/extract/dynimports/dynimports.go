// Package dynimports detects dynamic/lazy imports that are invisible to the
// static dependency graph: Python non-top-level (in-function) `import`/`from`,
// `importlib.import_module` / `__import__`, and TypeScript `require()` / dynamic
// `import()`. These deferred imports hide cycles and undercount coupling, so the
// signal flags them as report-only evidence (count per module + sample sites).
//
// This is an adapter package: os.ReadFile / filepath.Walk are permitted here.
// The scan is a deterministic structural pass over source files — no external
// tool — so the signal is byte-stable across environments and never depends on
// `sg` (ast-grep) being installed. Package shape mirrors internal/extract/runtime.
//
// REPORT-ONLY: Detect never modifies the dependency graph, any metric, or the
// verdict. It only surfaces evidence for humans and the off-gate LLM review.
//
// Detection ceiling (accepted tradeoff): Python in-function detection uses a
// def-indentation stack rather than a full parser, so an import inside a function
// body is flagged; class-body and module-top-level imports are not. Upgrade to an
// ast-grep relational rule only if false positives (e.g. in-function
// `if TYPE_CHECKING:` imports) prove noisy in practice.
package dynimports

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/syntax"
)

// Dynamic-import kind labels. These are coverage/evidence terms, not Balanced
// Coupling vocabulary — the signal is a supporting risk hint, not a BC verdict.
const (
	kindLazyImport    = evidence.DynamicImportKindLazyImport    // python non-top-level import / from-import
	kindImportlib     = evidence.DynamicImportKindImportlib     // python importlib.import_module / __import__
	kindRequire       = evidence.DynamicImportKindRequire       // ts/js require(...)
	kindDynamicImport = evidence.DynamicImportKindDynamicImport // ts/js dynamic import(...)

	langPython     = "python"
	langTypeScript = "typescript"
)

// tsRequire matches a require( call at a word boundary so a substring like
// myRequire( is not flagged.
var tsRequire = regexp.MustCompile(`\brequire\s*\(`)

// tsDynImport matches a dynamic import( call. The static `import {x} from` /
// `import 'x'` forms are followed by `{`, an identifier, or a quote — never `(` —
// so this never matches a static import.
var tsDynImport = regexp.MustCompile(`\bimport\s*\(`)

// tsSourceExtensions is copied once at init time; Detect checks it for every
// walked file, so avoid calling graph.TypeScriptSourceExtensions() per file.
var tsSourceExtensions = graph.TypeScriptSourceExtensions()

// pyImportlib matches importlib.import_module(...) or __import__(...) anywhere on
// a line (these are calls, not statements, so indentation is irrelevant).
var pyImportlib = regexp.MustCompile(`\b(?:importlib\.import_module|__import__)\s*\(`)

// Detect walks root and returns every dynamic/lazy import site, sorted by
// (file, line, kind) for determinism. Unreadable files are skipped (best-effort
// evidence) — Detect never returns an error.
func Detect(root string) []evidence.DynamicImportSite {
	var sites []evidence.DynamicImportSite
	_ = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel := relPath(root, path)
		switch filepath.Ext(path) {
		case ".py":
			if dynamicImportEvidenceSourceFile(langPython, rel) {
				sites = append(sites, scanPython(root, path)...)
			}
		default:
			if tsSourceExt(filepath.Ext(path)) && dynamicImportEvidenceSourceFile(langTypeScript, rel) {
				sites = append(sites, scanTS(root, path)...)
			}
		}
		return nil
	})
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].File != sites[j].File {
			return sites[i].File < sites[j].File
		}
		if sites[i].Line != sites[j].Line {
			return sites[i].Line < sites[j].Line
		}
		return sites[i].Kind < sites[j].Kind
	})
	return sites
}

func tsSourceExt(ext string) bool {
	for _, candidate := range tsSourceExtensions {
		if ext == candidate {
			return true
		}
	}
	return false
}

// scanPython flags imports that are NOT at module top-level (sitting inside a
// function body) plus importlib/__import__ calls anywhere. A def-indentation
// stack tracks whether the current line is inside a function: non-top-level
// imports are the ones the static graph misses.
func scanPython(root, path string) []evidence.DynamicImportSite {
	data, err := os.ReadFile(path) //nolint:gosec // path from Walk under repo root
	if err != nil {
		return nil
	}
	rel := relPath(root, path)
	var sites []evidence.DynamicImportSite
	var defStack []int // indentation columns of currently-open def/async-def blocks
	for i, raw := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimLeft(raw, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(trimmed)
		// A code line at indent <= an open def's indent closes that def's body.
		for len(defStack) > 0 && indent <= defStack[len(defStack)-1] {
			defStack = defStack[:len(defStack)-1]
		}
		inFunc := len(defStack) > 0
		switch {
		case pyImportlib.MatchString(trimmed):
			sites = append(sites, evidence.DynamicImportSite{File: rel, Line: i + 1, Kind: kindImportlib, Language: langPython})
		case inFunc && isPyImport(trimmed):
			sites = append(sites, evidence.DynamicImportSite{File: rel, Line: i + 1, Kind: kindLazyImport, Language: langPython})
		}
		// Push after this line's checks so the `def` line itself is not "in func".
		if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ") {
			defStack = append(defStack, indent)
		}
	}
	return sites
}

// scanTS flags require(...) and dynamic import(...) calls. Static `import ... from`
// / `export` statements never match (both regexes require a `(` after the
// keyword). Comment lines — including multi-line `/* ... */` blocks — are skipped
// so a commented-out require()/import() does not inflate the count.
func scanTS(root, path string) []evidence.DynamicImportSite {
	data, err := os.ReadFile(path) //nolint:gosec // path from Walk under repo root
	if err != nil {
		return nil
	}
	rel := relPath(root, path)
	var sites []evidence.DynamicImportSite
	inBlock := false
	for i, raw := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(raw)
		if inBlock {
			// Still inside a /* ... */ block: it ends on the line with the */.
			if strings.Contains(trimmed, "*/") {
				inBlock = false
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			// A block comment opens here; skip its body unless it also closes on
			// this same line (single-line /* ... */).
			if !strings.Contains(trimmed[len("/*"):], "*/") {
				inBlock = true
			}
			continue
		}
		if tsRequire.MatchString(trimmed) {
			sites = append(sites, evidence.DynamicImportSite{File: rel, Line: i + 1, Kind: kindRequire, Language: langTypeScript})
		}
		if tsDynImport.MatchString(trimmed) {
			sites = append(sites, evidence.DynamicImportSite{File: rel, Line: i + 1, Kind: kindDynamicImport, Language: langTypeScript})
		}
	}
	return sites
}

// isPyImport reports whether a trimmed line begins a Python import statement.
func isPyImport(trimmed string) bool {
	return strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ")
}

// relPath returns path relative to root, or path itself on error.
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// skipDir reports whether a directory should be skipped during the walk.
func skipDir(name string) bool {
	return name != "." && (strings.HasPrefix(name, ".") ||
		name == "node_modules" || name == "vendor" || name == "__pycache__" || name == "testdata")
}

func dynamicImportEvidenceSourceFile(lang, path string) bool {
	return !syntax.IsTestFile(lang, path) && !containsPathSegment(path, "testdata")
}

func containsPathSegment(path, seg string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == seg {
			return true
		}
	}
	return false
}
