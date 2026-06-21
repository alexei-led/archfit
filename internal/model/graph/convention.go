package graph

import "strings"

// NodeConvention captures the language-specific, filesystem-free heuristics the
// core decision ring needs to reason about module names and source files
// without importing adapters or switching on language strings inline. It is
// pure data plus pure functions, so it lives in the stdlib-only model ring and
// is importable by classify / metrics / score.
//
// Each builtin entry is an exact copy of a heuristic that previously lived
// inline in a core package (see BuiltinConventions). Lookup returns a safe
// default for unknown languages.
type NodeConvention struct {
	// Language is the canonical language id ("go", "typescript", "python", "rust").
	Language string
	// ModuleSegmentSep separates a hierarchical module name into path segments:
	// "/" for path modules (Go/TS/Rust), "." for dotted modules (Python).
	ModuleSegmentSep string
	// FileExtensions lists this language's source-file extensions.
	FileExtensions []string
	// Priority orders multi-source edge dedup; lower wins (Go 0 … Rust 3).
	Priority int

	// moduleFileCandidatesFn maps a module-node path to the repo-relative source
	// files it could have been built from. Nil for file-node languages (Go, TS,
	// Rust), whose file nodes already carry the path.
	moduleFileCandidatesFn func(modulePath string) []string
	// fileToModuleKeyFn maps a git file path to this language's module/package
	// key, or "" when the file is not one of this language's source files. Nil
	// means pass the path through unchanged (the prior modgraph default branch).
	fileToModuleKeyFn func(file string) string
}

// ModuleSegments splits a hierarchical module name into its path segments using
// the convention's separator (defaulting to "/" when unset).
func (c NodeConvention) ModuleSegments(module string) []string {
	sep := c.ModuleSegmentSep
	if sep == "" {
		sep = "/"
	}
	return strings.Split(module, sep)
}

// ModuleFileCandidates returns the repo-relative source-file path(s) a module
// node could have been built from, or nil for languages whose graph nodes are
// files (Go, TS, Rust) and therefore already carry the path.
func (c NodeConvention) ModuleFileCandidates(modulePath string) []string {
	if c.moduleFileCandidatesFn == nil {
		return nil
	}
	return c.moduleFileCandidatesFn(modulePath)
}

// FileToModuleKey maps a git file path to this language's module/package key,
// returning "" when the file is not one of this language's source files. A
// convention without a mapping function passes the path through unchanged,
// matching the prior modgraph default branch for non-Go/Python languages.
func (c NodeConvention) FileToModuleKey(file string) string {
	if c.fileToModuleKeyFn == nil {
		return file
	}
	return c.fileToModuleKeyFn(file)
}

// ConventionRegistry maps a canonical language id to its NodeConvention.
type ConventionRegistry map[string]NodeConvention

// defaultLanguagePriority is the dedup priority for an unknown language, matching
// the previous languagePriority default branch (lowest of the known languages).
const defaultLanguagePriority = 3

// Lookup returns the convention for lang, or a safe default (slash-separated,
// passthrough file→module key, lowest priority) for an unknown language. The
// default mirrors the previous inline behavior for languages the core ring did
// not special-case.
func (r ConventionRegistry) Lookup(lang string) NodeConvention {
	if c, ok := r[lang]; ok {
		return c
	}
	return NodeConvention{Language: lang, ModuleSegmentSep: "/", Priority: defaultLanguagePriority}
}

// BuiltinConventions holds the conventions for archfit's supported languages.
// Go/TypeScript/Python entries are exact copies of heuristics that previously
// lived inline in classify, metrics, and the graph model; rust is new.
var BuiltinConventions = ConventionRegistry{
	LangGo: {
		Language:          LangGo,
		ModuleSegmentSep:  "/",
		FileExtensions:    []string{".go"},
		Priority:          0,
		fileToModuleKeyFn: goFileToModuleKey,
	},
	LangTypeScript: {
		Language:         LangTypeScript,
		ModuleSegmentSep: "/",
		FileExtensions:   []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"},
		Priority:         1,
		// fileToModuleKeyFn nil → passthrough (file == node), matching the prior
		// modgraph default branch for TypeScript/JavaScript.
	},
	LangPython: {
		Language:               LangPython,
		ModuleSegmentSep:       ".",
		FileExtensions:         []string{".py", ".pyi"},
		Priority:               2,
		moduleFileCandidatesFn: pythonModuleFileCandidates,
		fileToModuleKeyFn:      pythonFileToModuleKey,
	},
	LangRust: {
		Language:          LangRust,
		ModuleSegmentSep:  "/",
		FileExtensions:    []string{".rs"},
		Priority:          3,
		fileToModuleKeyFn: rustFileToModuleKey,
	},
}

// goFileToModuleKey collapses a Go source file to its package directory
// (exact copy of the modgraph "go" branch).
func goFileToModuleKey(file string) string {
	if !strings.HasSuffix(file, ".go") {
		return ""
	}
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		return file[:i]
	}
	return ""
}

// pythonFileToModuleKey maps a Python source file to its dotted module key
// (exact copy of the modgraph "python" branch).
func pythonFileToModuleKey(file string) string {
	if !strings.HasSuffix(file, ".py") {
		return ""
	}
	p := strings.TrimSuffix(file, ".py")
	p = strings.TrimPrefix(p, "src/")
	p = strings.TrimSuffix(p, "/__init__")
	return strings.ReplaceAll(p, "/", ".")
}

// pythonModuleFileCandidates maps a dotted Python module path to its candidate
// source files (exact copy of change_locality's module-node branch).
func pythonModuleFileCandidates(modulePath string) []string {
	slashed := strings.ReplaceAll(modulePath, ".", "/")
	return []string{slashed + ".py", slashed + ".pyi", slashed + "/__init__.py"}
}

// rustFileToModuleKey maps a Rust source file to its crate via the cargo
// workspace directory convention (crates/<name>/…). Precise crate boundaries
// need member paths the filesystem-free core ring lacks, so files outside
// crates/ map to "" — a documented ceiling; change-coupling is a secondary
// metric and crate-level granularity is the accepted tradeoff.
func rustFileToModuleKey(file string) string {
	if !strings.HasSuffix(file, ".rs") {
		return ""
	}
	rest, ok := strings.CutPrefix(file, "crates/")
	if !ok {
		return ""
	}
	if name, _, found := strings.Cut(rest, "/"); found {
		return name
	}
	return strings.TrimSuffix(rest, ".rs")
}
