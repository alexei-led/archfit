// Package pyimports reads Python imports with ast-grep and places them on
// modules with a built-in resolver. It runs no Python, uv or grimp, and reads
// no installed dependencies. ast-grep runs only through toolrun.Runner.
//
// The packages are the policy's languages.python.package, else the top-level
// directories that hold __init__.py, with a src layout first. A tree with no
// package gives empty facts, not an error: a repository with loose Python
// scripts is not a Python project to this extractor. The root directory's
// name is never a package, so a tree gives the same facts in any checkout.
//
// Modules are the files of each package as grimp walks them, except that a
// directory without __init__.py inside a package is read too: its files are
// namespace-package modules (PEP 420). "from m import n" uses m.n when that is
// a module, else m. Relative imports resolve as importlib.util.resolve_name
// does. Imports inside "if TYPE_CHECKING:" or "elif x.TYPE_CHECKING:" are
// dropped, as grimp drops them; imports inside functions count.
//
// An import whose top-level name is not a first-party package is
// KindExternal, the standard library included. A first-party module that has
// no file, or a relative import that climbs above its package, is
// KindUnknown.
//
// An excluded file is not a source and its imports are not read, but it stays
// a module, so imports of it still resolve.
//
// It is a port of scripts/eval/extractors/py_imports.py, the prototype that
// the extractor-equivalence test measured (design appendix D.3), with three
// differences: no package from the root directory's name, no first-party
// names from module paths (schema 3 paths are file globs), and a configured
// package without a directory is an error.
package pyimports

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Kind says where an import resolved.
type Kind string

const (
	// KindFile is a first-party module with a file under the root.
	KindFile Kind = "file"
	// KindUnknown is a first-party import the resolver cannot place.
	KindUnknown Kind = "unknown"
	// KindExternal is a module outside the first-party packages, the standard
	// library included.
	KindExternal Kind = "external"
)

// Import is one module a source file uses: one per imported name.
type Import struct {
	File string // root-relative, slash-separated importer
	Line int    // 1-based line of the name that decides the module
	// Module is the dotted module the import uses, after relative
	// resolution; a relative import above its package keeps the name as written.
	Module string
	Kind   Kind
	Target string // the root-relative file, for KindFile only
}

// Facts are the imports of a tree.
type Facts struct {
	Sources []string // the module files read, sorted
	Imports []Import // sorted by file, line and module
}

// Config holds the policy settings the extractor reads.
type Config struct {
	Package string   // languages.python.package; empty: discover
	Exclude []string // globs of paths no extractor reads
}

// ErrNoAstGrep reports that the tree has Python modules but ast-grep is not
// installed, so their imports are not read.
var ErrNoAstGrep = errors.New("pyimports: ast-grep is not installed; it reads Python imports")

const astGrep = "ast-grep"

// batchSize is the number of paths per ast-grep call; it keeps the command
// line short on large trees.
const batchSize = 500

// sgNoMatch is the exit code ast-grep gives a scan that matched nothing.
// Any code other than it and 0, including -1 for a process a signal ended,
// is a failed run whose output may be cut short.
const sgNoMatch = 1

// The rule ids of rules/python.yml.
const (
	ruleImport       = "import-stmt"
	ruleFrom         = "from-stmt"
	ruleFromModule   = "from-module"
	ruleName         = "name"
	ruleTypeChecking = "type-checking"
)

//go:embed rules/python.yml
var rules string

// noIgnore makes ast-grep read every file it is given, whatever ignore files say.
var noIgnore = func() []string {
	kinds := []string{"hidden", "dot", "exclude", "global", "parent", "vcs"}
	args := make([]string, 0, 2*len(kinds))
	for _, kind := range kinds {
		args = append(args, "--no-ignore", kind)
	}
	return args
}()

// Extract reads and resolves the imports of every module of the Python
// packages under root. A tree without modules needs no ast-grep.
func Extract(ctx context.Context, runner toolrun.Runner, root string, cfg Config) (Facts, error) {
	fsys := os.DirFS(root)
	read, discovered, err := packages(fsys, cfg.Package)
	if err != nil {
		return Facts{}, err
	}
	roots := map[string]bool{}
	for _, p := range slices.Concat(read, discovered) {
		if isIdentifier(p) {
			roots[p] = true
		}
	}
	modules := map[string]string{} // module to file
	for _, pkg := range read {
		files, err := sourceFiles(fsys, pkg)
		if err != nil {
			return Facts{}, err
		}
		maps.Copy(modules, files)
	}
	moduleOf := map[string]string{} // source file to module
	for mod, file := range modules {
		if !excludedSource(cfg.Exclude, file) {
			moduleOf[file] = mod
		}
	}
	sources := slices.Sorted(maps.Keys(moduleOf))
	if len(sources) == 0 {
		return Facts{}, nil
	}
	if _, ok := runner.Detect(ctx, astGrep); !ok {
		return Facts{}, ErrNoAstGrep
	}
	parsed, err := parse(ctx, runner, root, sources)
	if err != nil {
		return Facts{}, err
	}
	var imports []Import
	for _, file := range sources {
		mod := moduleOf[file]
		isPackage := path.Base(file) == initFile
		for _, imp := range parsed[file] {
			targets, err := resolve(mod, isPackage, imp, modules)
			if err != nil {
				imports = append(imports, Import{File: file, Line: imp.line, Module: imp.module, Kind: KindUnknown})
				continue
			}
			for _, t := range targets {
				imports = append(imports, place(file, t, roots, modules))
			}
		}
	}
	slices.SortStableFunc(imports, func(a, b Import) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Module, b.Module))
	})
	return Facts{Sources: sources, Imports: imports}, nil
}

// place classifies one target module of an import in file.
func place(file string, t target, roots map[string]bool, modules map[string]string) Import {
	imp := Import{File: file, Line: t.line, Module: t.module}
	top, _, _ := strings.Cut(t.module, ".")
	switch {
	case !roots[top]:
		imp.Kind = KindExternal
	case modules[t.module] != "":
		imp.Kind, imp.Target = KindFile, modules[t.module]
	default:
		imp.Kind = KindUnknown
	}
	return imp
}

// name is one dotted name an import statement lists.
type name struct {
	text string
	line int
}

// pyImport is one import as written: "import a.b" or "from m import names".
type pyImport struct {
	module string
	names  []name
	isFrom bool
	line   int // the line of module
}

// span is the byte range of a match.
type span struct{ start, end int }

func (s span) inside(outer span) bool { return outer.start <= s.start && s.end <= outer.end }

// sgMatch is the part of an ast-grep --json=stream match the extractor reads.
type sgMatch struct {
	Text   string `json:"text"`
	File   string `json:"file"`
	RuleID string `json:"ruleId"`
	Range  struct {
		ByteOffset struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"byteOffset"`
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"range"`
}

func (m sgMatch) span() span { return span{m.Range.ByteOffset.Start, m.Range.ByteOffset.End} }

func (m sgMatch) name() name { return name{m.Text, m.Range.Start.Line + 1} }

// parse runs ast-grep over files and returns the imports of each file, without
// those inside a TYPE_CHECKING block.
func parse(ctx context.Context, runner toolrun.Runner, root string, files []string) (map[string][]pyImport, error) {
	byFile := map[string]map[string][]sgMatch{}
	for batch := range slices.Chunk(files, batchSize) {
		args := slices.Concat([]string{"scan", "--inline-rules", rules, "--json=stream"}, noIgnore, []string{"--"}, batch)
		out, err := runner.Run(ctx, toolrun.ToolCmd{Name: astGrep, Args: args, WorkDir: root})
		if err != nil {
			return nil, fmt.Errorf("pyimports: run ast-grep: %w", err)
		}
		if out.ExitCode != 0 && out.ExitCode != sgNoMatch {
			return nil, fmt.Errorf("pyimports: ast-grep exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		}
		for line := range bytes.SplitSeq(out.Stdout, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var m sgMatch
			if err := json.Unmarshal(line, &m); err != nil {
				return nil, fmt.Errorf("pyimports: read ast-grep output: %w", err)
			}
			file := path.Clean(m.File)
			if byFile[file] == nil {
				byFile[file] = map[string][]sgMatch{}
			}
			byFile[file][m.RuleID] = append(byFile[file][m.RuleID], m)
		}
	}
	out := make(map[string][]pyImport, len(byFile))
	for file, byRule := range byFile {
		imports, err := fileImports(byRule)
		if err != nil {
			return nil, fmt.Errorf("pyimports: %s: %w", file, err)
		}
		out[file] = imports
	}
	return out, nil
}

// fileImports builds the imports of one file from its matches by rule.
func fileImports(byRule map[string][]sgMatch) ([]pyImport, error) {
	for _, ms := range byRule {
		slices.SortFunc(ms, func(a, b sgMatch) int { return cmp.Compare(a.span().start, b.span().start) })
	}
	skip := make([]span, 0, len(byRule[ruleTypeChecking]))
	for _, b := range byRule[ruleTypeChecking] {
		skip = append(skip, b.span())
	}
	skipped := func(s span) bool { return slices.ContainsFunc(skip, s.inside) }
	modules := byRule[ruleFromModule]
	isModule := func(s span) bool {
		return slices.ContainsFunc(modules, func(m sgMatch) bool { return m.span() == s })
	}
	var out []pyImport
	for _, stmt := range byRule[ruleImport] {
		if s := stmt.span(); !skipped(s) {
			for _, n := range byRule[ruleName] {
				if n.span().inside(s) {
					out = append(out, pyImport{module: n.Text, line: n.name().line})
				}
			}
		}
	}
	for _, stmt := range byRule[ruleFrom] {
		s := stmt.span()
		if skipped(s) {
			continue
		}
		i := slices.IndexFunc(modules, func(m sgMatch) bool { return m.span().inside(s) })
		if i < 0 {
			return nil, fmt.Errorf("from-import on line %d names no module", stmt.name().line)
		}
		imp := pyImport{module: modules[i].Text, isFrom: true, line: modules[i].name().line}
		for _, n := range byRule[ruleName] {
			if n.span().inside(s) && !isModule(n.span()) {
				imp.names = append(imp.names, n.name())
			}
		}
		out = append(out, imp)
	}
	return out, nil
}
