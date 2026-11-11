package golang

import (
	"cmp"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
)

// ImportKind says where an import of a Go file lands.
type ImportKind string

const (
	// ImportFirstParty is an import of a package of a loaded member that the
	// load reads.
	ImportFirstParty ImportKind = "first_party"
	// ImportExternal is an import of the standard library or of a module the
	// run does not load.
	ImportExternal ImportKind = "external"
	// ImportUnresolved is an import under a loaded member's module path where
	// the load reads no package.
	ImportUnresolved ImportKind = "unresolved"
)

// FileImport is one import spec of a Go source file.
type FileImport struct {
	// File is the importing file, relative to the scan root.
	File string
	// Line is the line of the import path in File.
	Line int
	// Path is the import path as written.
	Path string
	// Package is the scan-root-relative package dir of a first-party or
	// unresolved import ("" for a member's root package); empty for an
	// external import.
	Package string
	Kind    ImportKind
}

// FileUnknown is a Go source file whose imports the parser cannot give: the
// file is not dropped and its imports are not guessed.
type FileUnknown struct {
	File   string
	Reason string
}

// ParsedImports is the parser path's account of the Go sources under a scan
// root, sorted so two runs compare byte for byte.
type ParsedImports struct {
	// Sources are the files the go command reads, relative to the scan root.
	Sources []string
	// Imports are the import specs of Sources. An import whose package dir,
	// or external import path, an exclusion matches is left out, as the typed
	// loader leaves it out.
	Imports []FileImport
	Unknown []FileUnknown
}

// ParseImports reads the imports of every Go source file the go command would
// read in the loaded members (go.work, go.mod, the languages.go.modules
// filter), with go/parser and go/build. It runs no Go tool and needs no module
// dependencies.
func ParseImports(scanRoot string, cfg evidenceports.ExtractConfig) (ParsedImports, error) {
	members, err := memberModules(scanRoot, cfg)
	if err != nil {
		return ParsedImports{}, err
	}
	var out ParsedImports
	var specs []FileImport
	for _, m := range members {
		memberDir := filepath.Join(scanRoot, filepath.FromSlash(m.relDir))
		walkErr := filepath.WalkDir(memberDir, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if name != memberDir && (skipDirName(entry.Name()) || hasGoMod(name)) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(scanRoot, name)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if filepath.Ext(rel) != ".go" || fileReason(scanRoot, cfg.Exclusions, members, rel) != "" {
				return nil
			}
			imports, parseErr := parseFileImports(name, rel)
			sel, reason, matchErr := buildSelects(scanRoot, cfg.BuildFlags, rel, slices.ContainsFunc(imports, isImportC))
			switch {
			case sel == selectNo:
			case parseErr != nil:
				out.Unknown = append(out.Unknown, FileUnknown{File: rel, Reason: "does not parse: " + strings.ReplaceAll(parseErr.Error(), name, rel)})
			case matchErr != nil:
				out.Unknown = append(out.Unknown, FileUnknown{File: rel, Reason: "build constraints do not parse: " + strings.ReplaceAll(matchErr.Error(), name, rel)})
			case sel == selectUndecided:
				out.Unknown = append(out.Unknown, FileUnknown{File: rel, Reason: reason})
			default:
				out.Sources = append(out.Sources, rel)
				specs = append(specs, imports...)
			}
			return nil
		})
		if walkErr != nil {
			return ParsedImports{}, walkErr
		}
	}
	out.Imports = placeImports(members, cfg.Exclusions, out.Sources, specs)
	out.sort()
	return out, nil
}

// placeImports gives each import spec its package and kind. A first-party
// package is one the sources hold.
func placeImports(members []modEntry, exclusions, sources []string, specs []FileImport) []FileImport {
	packages := make(map[string]bool, len(sources))
	for _, src := range sources {
		packages[packageDir(src)] = true
	}
	placed := make([]FileImport, 0, len(specs))
	for _, spec := range specs {
		dir, firstParty := placeImport(members, spec.Path)
		if !firstParty {
			dir = spec.Path
		}
		switch {
		case excluded(exclusions, dir):
			continue
		case !firstParty:
			spec.Kind = ImportExternal
		case packages[dir]:
			spec.Package, spec.Kind = dir, ImportFirstParty
		default:
			spec.Package, spec.Kind = dir, ImportUnresolved
		}
		placed = append(placed, spec)
	}
	return placed
}

// parseFileImports reads the import specs of the Go file at name with their
// lines. A file whose header does not parse gives the error.
func parseFileImports(name, rel string) ([]FileImport, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	imports := make([]FileImport, 0, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil {
			continue
		}
		imports = append(imports, FileImport{File: rel, Line: fset.Position(spec.Path.Pos()).Line, Path: importPath})
	}
	return imports, nil
}

func isImportC(spec FileImport) bool { return spec.Path == "C" }

func (p *ParsedImports) sort() {
	slices.Sort(p.Sources)
	slices.SortFunc(p.Imports, func(a, b FileImport) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Path, b.Path))
	})
	slices.SortFunc(p.Unknown, func(a, b FileUnknown) int { return cmp.Compare(a.File, b.File) })
}
