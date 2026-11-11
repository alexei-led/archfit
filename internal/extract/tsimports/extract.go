// Package tsimports reads TypeScript and JavaScript imports with ast-grep and
// places them on files with a built-in resolver. It reads no node_modules and
// runs no Node or TypeScript compiler. ast-grep runs only through
// toolrun.Runner.
//
// The resolver tries, in this order: relative paths (with extension and index
// probes; as in TypeScript, a ".js" specifier names a ".ts", ".tsx" or ".d.ts"
// file before the ".js" file, and ".jsx", ".mjs" and ".cjs" alike, and a
// ".ts" one its ".d.ts" when the ".ts" file is absent), node: builtins,
// package.json imports ("#x"), tsconfig paths, tsconfig baseUrl when no paths
// pattern matched, and workspace packages (npm, Yarn and pnpm) through
// exports, types, module or main. A query such as "?raw" is dropped. Any other
// bare specifier is a third-party package: KindExternal. A relative path, a
// paths alias or a workspace subpath that names no file is KindUnknown; a bare
// "*" alias alone proves nothing.
//
// No answer depends on installed dependencies: a path or alias under
// node_modules is KindExternal. A tsconfig extends that is not read (a
// package, a file in node_modules or outside the root, a missing file, more
// than 10 nested) is an UnknownItem, and so is a tsconfig that does not parse.
// The paths such a config may set are unknown, so under it a bare specifier
// that nothing places is KindExternal only for a Node built-in or a package
// that a package.json at or above the importer declares; else KindUnknown.
//
// It is a port of scripts/eval/extractors/ts_imports.py, the prototype that
// the extractor-equivalence test measured. Unlike it, nothing here reads
// node_modules or a path outside the root, and a config it cannot read is
// reported, not ignored. Not handled: an extends of a workspace package (it
// is reported as not read), moduleResolution "classic", .vue and .svelte files
// as importers, and template-literal import().
package tsimports

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Kind says where a specifier resolved.
type Kind string

const (
	// KindFile is a first-party file under the root.
	KindFile Kind = "file"
	// KindUnknown is a first-party specifier that names no file.
	KindUnknown Kind = "unknown"
	// KindExternal is a third-party package or a node: builtin.
	KindExternal Kind = "external"
)

// Import is one specifier a source file uses. Type-only imports count.
type Import struct {
	File      string // root-relative, slash-separated importer
	Line      int    // 1-based line of the specifier
	Specifier string // as written, with any query
	Kind      Kind
	Target    string // the root-relative file, for KindFile only
}

// UnknownItem is a tsconfig the resolver does not read: File is the tsconfig
// that names it, or that does not parse; Specifier is the extends value, empty
// for a tsconfig that does not parse.
type UnknownItem struct {
	File      string
	Specifier string
	Reason    string
}

// Facts are the imports of a tree.
type Facts struct {
	Sources []string // every parsed file outside node_modules and .git, sorted
	Imports []Import // sorted by file, line and specifier
	// Unknown lists the unread tsconfigs that govern a source file, sorted.
	Unknown []UnknownItem
}

// ErrNoAstGrep reports that the tree has TypeScript or JavaScript files but
// ast-grep is not installed, so their imports are not read.
var ErrNoAstGrep = errors.New("tsimports: ast-grep is not installed; it reads TypeScript and JavaScript imports")

const astGrep = "ast-grep"

// batchSize is the number of paths per ast-grep call; it keeps the command
// line short on large trees.
const batchSize = 500

// sgNoMatch is the exit code ast-grep gives a scan that matched nothing.
// Any code other than it and 0, including -1 for a process a signal ended,
// is a failed run whose output may be cut short.
const sgNoMatch = 1

//go:embed rules/typescript.yml
var rules string

// parsedExts are the extensions of the files whose imports are read: the
// script extensions that lead probeExts.
var parsedExts = probeExts[:8]

// noIgnore makes ast-grep read every file it is given, whatever ignore files say.
var noIgnore = func() []string {
	kinds := []string{"hidden", "dot", "exclude", "global", "parent", "vcs"}
	args := make([]string, 0, 2*len(kinds))
	for _, kind := range kinds {
		args = append(args, "--no-ignore", kind)
	}
	return args
}()

var referencePath = regexp.MustCompile(`\bpath\s*=\s*["']([^"']+)["']`)

// Extract reads and resolves the imports of every TypeScript and JavaScript
// file under root. A tree without such files needs no ast-grep.
func Extract(ctx context.Context, runner toolrun.Runner, root string) (Facts, error) {
	fsys := os.DirFS(root)
	files, err := sourceFiles(fsys)
	if err != nil || len(files) == 0 {
		return Facts{}, err
	}
	if _, ok := runner.Detect(ctx, astGrep); !ok {
		return Facts{}, ErrNoAstGrep
	}
	specs, err := parse(ctx, runner, root, files)
	if err != nil {
		return Facts{}, err
	}
	r := newResolver(fsys)
	imports := make([]Import, 0, len(specs))
	for _, s := range specs {
		kind, target := r.resolve(s.File, s.Specifier)
		s.Kind, s.Target = kind, target
		imports = append(imports, s)
	}
	slices.SortFunc(imports, func(a, b Import) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Specifier, b.Specifier))
	})
	// Every source file's tsconfig, not only those a bare specifier reads, so
	// the items do not depend on what a file imports.
	var unknown []UnknownItem
	for _, f := range files {
		unknown = append(unknown, r.tsconfig(path.Dir(f)).unread...)
	}
	slices.SortFunc(unknown, func(a, b UnknownItem) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Specifier, b.Specifier),
			cmp.Compare(a.Reason, b.Reason))
	})
	return Facts{Sources: files, Imports: slices.Compact(imports), Unknown: slices.Compact(unknown)}, nil
}

// sourceFiles lists the root-relative files to parse, sorted.
func sourceFiles(fsys fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("tsimports: walk %s: %w", name, err)
		}
		if d.IsDir() {
			if name != "." && (d.Name() == nodeModules || d.Name() == ".git") {
				return fs.SkipDir
			}
			return nil
		}
		if slices.ContainsFunc(parsedExts, func(ext string) bool { return strings.HasSuffix(name, ext) }) {
			files = append(files, name)
		}
		return nil
	})
	slices.Sort(files)
	return files, err
}

// sgMatch is the part of an ast-grep --json=stream match the extractor reads.
type sgMatch struct {
	Text   string `json:"text"`
	File   string `json:"file"`
	RuleID string `json:"ruleId"`
	Range  struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"range"`
}

// parse returns the import specifiers of files, unresolved. A triple-slash
// reference path is relative to its file, as a specifier starting with "./" is.
func parse(ctx context.Context, runner toolrun.Runner, root string, files []string) ([]Import, error) {
	var specs []Import
	for batch := range slices.Chunk(files, batchSize) {
		args := slices.Concat([]string{"scan", "--inline-rules", rules, "--json=stream"}, noIgnore, []string{"--"}, batch)
		out, err := runner.Run(ctx, toolrun.ToolCmd{Name: astGrep, Args: args, WorkDir: root})
		if err != nil {
			return nil, fmt.Errorf("tsimports: run ast-grep: %w", err)
		}
		if out.ExitCode != 0 && out.ExitCode != sgNoMatch {
			return nil, fmt.Errorf("tsimports: ast-grep exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		}
		for line := range bytes.SplitSeq(out.Stdout, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var m sgMatch
			if err := json.Unmarshal(line, &m); err != nil {
				return nil, fmt.Errorf("tsimports: read ast-grep output: %w", err)
			}
			spec, ok := specifier(m)
			if ok {
				specs = append(specs, Import{File: path.Clean(m.File), Line: m.Range.Start.Line + 1, Specifier: spec})
			}
		}
	}
	return specs, nil
}

// specifier returns the specifier a match names: the path of a reference
// directive, else the string literal without its quotes.
func specifier(m sgMatch) (string, bool) {
	if strings.HasSuffix(m.RuleID, "-reference") {
		found := referencePath.FindStringSubmatch(m.Text)
		if found == nil {
			return "", false
		}
		if p := found[1]; strings.HasPrefix(p, ".") || strings.HasPrefix(p, "/") {
			return p, true
		}
		return "./" + found[1], true
	}
	if len(m.Text) < 2 {
		return "", false
	}
	return m.Text[1 : len(m.Text)-1], true
}
