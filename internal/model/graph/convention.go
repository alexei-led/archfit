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
	// "/" for path modules (Go/TS), "::" for Rust, "." for dotted modules (Python).
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
		Language: LangRust,
		// Module NODE paths are crate-relative Rust paths ("crate::mod::sub"); the
		// segment separator is "::" so code-structure distance sees siblings as
		// same-owner. (File paths are mapped separately by fileToModuleKeyFn.)
		ModuleSegmentSep:  "::",
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
// source files (mirrors the module-node convention used by the graph extractor).
func pythonModuleFileCandidates(modulePath string) []string {
	slashed := strings.ReplaceAll(modulePath, ".", "/")
	return []string{slashed + ".py", slashed + ".pyi", slashed + "/__init__.py"}
}

// rustCrateSubdirs maps a Cargo crate's conventional source subdir to the module
// namespace prefix for files under it. "src" → "" (lib/bin modules map straight to
// the crate path); tests/benches/examples are separate cargo targets, so their files
// are namespaced (e.g. crate::tests::…) to stay distinct from lib modules. Membership
// also marks the first path segment as a crate dir in the root-level workspace layout
// (<crate>/src/…).
var rustCrateSubdirs = map[string]string{"src": "", "tests": "tests", "benches": "benches", "examples": "examples"}

// rustFileToModuleKey maps a Rust source file to its crate. It handles the two
// dominant Cargo workspace layouts: members nested under crates/ (crates/<name>/…)
// and members at the workspace root (<name>/src/…), the latter identified by a
// conventional crate subdir as the second segment. Precise crate boundaries need
// the cargo member-path map the filesystem-free core ring lacks, so the root crate's
// own files (e.g. "src/main.rs") and unrecognised layouts map to "", and the
// directory name is assumed to equal the crate name — documented ceilings; crate-level
// granularity is the accepted tradeoff for the secondary size/change-coupling metrics.
func rustFileToModuleKey(file string) string {
	if !strings.HasSuffix(file, ".rs") {
		return ""
	}
	// crates/<name>/… layout.
	if rest, ok := strings.CutPrefix(file, "crates/"); ok {
		if name, _, found := strings.Cut(rest, "/"); found {
			return name
		}
		return strings.TrimSuffix(rest, ".rs")
	}
	// Root-level workspace member: <crate>/{src,tests,benches,examples}/…
	if dir, rest, found := strings.Cut(file, "/"); found {
		if sub, _, ok := strings.Cut(rest, "/"); ok {
			if _, isSubdir := rustCrateSubdirs[sub]; isSubdir {
				return dir
			}
		}
	}
	return ""
}

// RustFileToModuleKey maps a Rust source file to its module key ("<crate>::<mod>"),
// using the crate roots the extractor discovered from cargo metadata. Unlike the
// crate-only rustFileToModuleKey convention, this resolves to module granularity so
// size/cohesion metrics see god *modules*, not just god crates. The crate name is
// supplied (not guessed from the directory), and the within-crate module path is the
// standard Rust file layout: src/a/b.rs → crate::a::b, src/a/mod.rs → crate::a,
// src/{lib,main}.rs → crate. Files under tests/benches/examples are separate crates
// in cargo's model, so they are namespaced (crate::tests::…) to stay distinct from
// the lib modules and from the cargo-modules/SCIP node ids (which cover lib/bin only).
//
// Accepted ceiling: this is filesystem convention, so #[path="…"], inline `mod foo {}`
// submodules, and include! diverge from rust-analyzer's semantic module tree. For
// size-skew (god-file) detection a big file is still attributed to one real module,
// which is sufficient; SCIP-derived mapping would be the precision upgrade. Returns
// "" for non-.rs files or files outside every known crate root.
func RustFileToModuleKey(file string, crates []CrateRoot) string {
	if !strings.HasSuffix(file, ".rs") {
		return ""
	}
	crate, rel, ok := matchCrateRoot(file, crates)
	if !ok {
		return ""
	}
	// rel is the crate-relative path (e.g. "src/a/b.rs"). Split off the conventional
	// target subdir; src maps straight to the module path, the rest is namespaced by
	// the prefix the subdir map carries.
	sub, within, hasSub := strings.Cut(rel, "/")
	ns, isSubdir := rustCrateSubdirs[sub]
	if !hasSub || !isSubdir {
		return crate.Name // crate-root file outside a recognised subdir (e.g. build.rs)
	}
	key := crate.Name
	if ns != "" {
		key += "::" + ns
	}
	if mod := rustModulePathFromWithin(within); mod != "" {
		key += "::" + mod
	}
	return key
}

// matchCrateRoot returns the crate whose Dir is the longest path-prefix of file and
// the crate-relative remainder. A root crate (Dir "") contains every file but at
// lowest priority, so a nested workspace member always wins. ok=false when no crate
// contains the file.
func matchCrateRoot(file string, crates []CrateRoot) (CrateRoot, string, bool) {
	best, bestLen := -1, -1
	for i, c := range crates {
		var pfxLen int
		switch {
		case c.Dir == "":
			pfxLen = 0 // root crate: matches everything, lowest priority
		case strings.HasPrefix(file, c.Dir+"/"):
			pfxLen = len(c.Dir)
		default:
			continue
		}
		if best < 0 || pfxLen > bestLen {
			best, bestLen = i, pfxLen
		}
	}
	if best < 0 {
		return CrateRoot{}, "", false
	}
	c := crates[best]
	if c.Dir == "" {
		return c, file, true
	}
	return c, strings.TrimPrefix(file, c.Dir+"/"), true
}

// rustModulePathFromWithin converts a crate-subdir-relative file path to a "::" module
// path: "a/b.rs" → "a::b", "a/mod.rs" → "a", "lib.rs"/"main.rs"/"mod.rs" → "".
func rustModulePathFromWithin(within string) string {
	p := strings.TrimSuffix(within, ".rs")
	switch p {
	case "lib", "main", "mod", "":
		return ""
	}
	p = strings.TrimSuffix(p, "/mod") // a/mod.rs → a
	return strings.ReplaceAll(p, "/", "::")
}
