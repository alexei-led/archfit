package main

import (
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/mod/modfile"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
)

// member is one Go module the run reads: its module path and its dir
// relative to the root ("." for the root).
type member struct {
	path, relDir string
}

// goparse is the parser-based Go extractor: it reads the imports of every
// non-test Go file the host build context selects, in the members today's
// extractor loads, and places a first-party import path on its package dir.
// It runs no Go tool.
func goparse(root string, cfg config.Config) (facts, error) {
	cfg = mergedExclusions(cfg)
	ec := cfg.ForExtract(config.LangGo)
	dirs, _ := registry.GoMembers(root, ec)
	members, err := readMembers(root, dirs)
	if err != nil {
		return facts{}, err
	}
	ctxt := buildContext(os.Getenv, ec.BuildFlags)
	out := facts{Language: config.LangGo, Side: "new"}
	type fileImports struct {
		rel     string
		imports []string
	}
	var files []fileImports
	packages := make(map[string]bool)
	for _, m := range members {
		memberDir := filepath.Join(root, filepath.FromSlash(m.relDir))
		// #nosec G703 -- memberDir is a member the operator's root holds.
		walkErr := filepath.WalkDir(memberDir, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, name)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				if name != memberDir && skipDir(name, entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isGoSource(entry.Name()) || excluded(ec.Exclusions, rel) {
				return nil
			}
			if ok, matchErr := ctxt.MatchFile(filepath.Dir(name), entry.Name()); matchErr != nil || !ok {
				return nil //nolint:nilerr // an unreadable or unselected file is not a source, as for the go command
			}
			imports, parseErr := parseImports(name)
			if parseErr != nil {
				return nil //nolint:nilerr // a file that does not parse has no imports to compare
			}
			if !ctxt.CgoEnabled && slices.Contains(imports, "C") {
				return nil // the go command ignores a cgo file when cgo is off
			}
			packages[packageDir(rel)] = true
			files = append(files, fileImports{rel: rel, imports: imports})
			return nil
		})
		if walkErr != nil {
			return facts{}, walkErr
		}
	}
	for _, f := range files {
		out.Sources = append(out.Sources, f.rel)
		for _, imp := range f.imports {
			dir, firstParty := placeImport(members, imp)
			switch {
			case !firstParty:
				out.External++
			case excluded(ec.Exclusions, dir):
				// Today's extractor drops an import of an excluded package.
			case packages[dir]:
				out.Edges = append(out.Edges, edge{From: f.rel, To: dir})
			default:
				out.Unknown = append(out.Unknown, unresolved{From: f.rel, Specifier: dir})
			}
		}
	}
	out.finish()
	return out, nil
}

// readMembers reads the module path of each member dir from its go.mod.
func readMembers(root string, dirs []string) ([]member, error) {
	members := make([]member, 0, len(dirs))
	for _, dir := range dirs {
		goMod := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(goMod) // #nosec G304 G703 -- a member's go.mod under the root
		if err != nil {
			return nil, err
		}
		modPath := modfile.ModulePath(data)
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return nil, err
		}
		members = append(members, member{path: modPath, relDir: filepath.ToSlash(rel)})
	}
	return members, nil
}

// placeImport strips the longest member module path from an import path and
// returns the package dir relative to the root, spelled as the engine spells
// a package node: "" for the root package. Other import paths are external.
func placeImport(members []member, importPath string) (string, bool) {
	best := -1
	for i, m := range members {
		if (importPath == m.path || strings.HasPrefix(importPath, m.path+"/")) &&
			(best < 0 || len(m.path) > len(members[best].path)) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	m := members[best]
	return packageDir(path.Join(m.relDir, strings.TrimPrefix(importPath, m.path), "x")), true
}

// packageDir is the package node of a root-relative file path.
func packageDir(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return ""
	}
	return dir
}

// skipDir reports whether the go command skips the directory for "./...":
// hidden and underscore dirs, testdata, vendor, and a nested module.
func skipDir(name, base string) bool {
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "testdata" || base == "vendor" {
		return true
	}
	_, err := os.Stat(filepath.Join(name, "go.mod"))
	return err == nil
}

// buildContext is the host build context with the run's -tags, read from
// GOFLAGS and the configured build flags, as today's extractor reads them, and
// with cgo as CGO_ENABLED sets it.
func buildContext(getenv func(string) string, buildFlags []string) build.Context {
	ctxt := build.Default
	switch getenv("CGO_ENABLED") {
	case "0":
		ctxt.CgoEnabled = false
	case "1":
		ctxt.CgoEnabled = true
	}
	flags := append(strings.Fields(getenv("GOFLAGS")), buildFlags...)
	for i, flag := range flags {
		value, ok := strings.CutPrefix(flag, "-tags=")
		if flag == "-tags" && i+1 < len(flags) {
			value, ok = flags[i+1], true
		}
		if ok {
			ctxt.BuildTags = strings.FieldsFunc(strings.Trim(value, "\"'"), func(r rune) bool { return r == ',' || r == ' ' })
		}
	}
	return ctxt
}

// parseImports returns the import paths of one Go file.
func parseImports(name string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	imports := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		p, unquoteErr := strconv.Unquote(imp.Path.Value)
		if unquoteErr != nil {
			continue
		}
		imports = append(imports, p)
	}
	return imports, nil
}

// isGoSource reports whether name is a Go file the go command reads for a
// package: a .go file that is not a test and does not start with "." or "_".
func isGoSource(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") &&
		!strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_")
}

// excluded reports whether a root-relative path matches an exclusion glob.
func excluded(exclusions []string, p string) bool {
	for _, pattern := range exclusions {
		if matched, _ := doublestar.Match(pattern, p); matched {
			return true
		}
	}
	return false
}
