package pyimports

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	srcDir   = "src"
	initFile = "__init__.py"
	pyExt    = ".py"
)

// errAboveTopLevel is a relative import that climbs above its top-level package.
var errAboveTopLevel = errors.New("relative import beyond top-level package")

// packages returns the packages to read: the configured one, else the
// discovered ones. It also returns every discovered package, which the
// first-party roots include even when one package is configured.
func packages(fsys fs.FS, configured string) (read, discovered []string, err error) {
	discovered, err = discoverPackages(fsys)
	if err != nil {
		return nil, nil, err
	}
	if configured == "" {
		return discovered, discovered, nil
	}
	if !isDir(fsys, path.Join(srcDir, configured)) && !isDir(fsys, configured) {
		return nil, nil, fmt.Errorf("pyimports: package %q: no directory %s/%s or %s", configured, srcDir, configured, configured)
	}
	return []string{configured}, discovered, nil
}

// discoverPackages returns the top-level directories that hold __init__.py,
// sorted. A src layout wins over the root.
func discoverPackages(fsys fs.FS) ([]string, error) {
	for _, base := range []string{srcDir, "."} {
		if base == srcDir && !isDir(fsys, srcDir) {
			continue
		}
		entries, err := fs.ReadDir(fsys, base)
		if err != nil {
			return nil, fmt.Errorf("pyimports: read %s: %w", base, err)
		}
		var found []string
		for _, e := range entries {
			dir := path.Join(base, e.Name())
			if isDir(fsys, dir) && isFile(fsys, path.Join(dir, initFile)) {
				found = append(found, e.Name())
			}
		}
		if len(found) > 0 {
			slices.Sort(found)
			return found, nil
		}
	}
	return nil, nil
}

// packageDir is the source directory of a package: src/<package> when it
// exists, else <package>.
func packageDir(fsys fs.FS, pkg string) string {
	if src := path.Join(srcDir, pkg); isDir(fsys, src) {
		return src
	}
	return pkg
}

// sourceFiles maps each module of a package to its root-relative file, as
// grimp walks it. Directories with non-identifier names are skipped. A
// directory without __init__.py is still walked: its files are namespace
// package (PEP 420) modules.
func sourceFiles(fsys fs.FS, pkg string) (map[string]string, error) {
	base := packageDir(fsys, pkg)
	modules := map[string]string{}
	err := fs.WalkDir(fsys, base, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("pyimports: walk %s: %w", name, err)
		}
		if d.IsDir() {
			if name != base && !isIdentifier(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		file := d.Name()
		if !strings.HasSuffix(file, pyExt) || strings.HasPrefix(file, ".") || strings.Count(file, ".") != 1 {
			return nil
		}
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(name, base+"/"), pyExt), "/")
		if parts[len(parts)-1] == "__init__" {
			parts = parts[:len(parts)-1]
		}
		modules[strings.Join(append([]string{pkg}, parts...), ".")] = name
		return nil
	})
	return modules, err
}

// target is one module an import uses, with the line of the name that names it.
type target struct {
	module string
	line   int
}

// resolve returns the dotted modules one import uses, as grimp records them.
// "from m import n" uses m.n when that is a module, else m. A target may be
// absent from modules; the caller reports it as unknown. A relative import
// that climbs above its top-level package is an error.
func resolve(importer string, isPackage bool, imp pyImport, modules map[string]string) ([]target, error) {
	mod := imp.module
	if imp.isFrom && strings.HasPrefix(mod, ".") {
		parent := importer
		if !isPackage {
			parent = importer[:max(strings.LastIndex(importer, "."), 0)]
		}
		var err error
		if mod, err = resolveRelative(mod, parent); err != nil {
			return nil, err
		}
	}
	if !imp.isFrom || len(imp.names) == 0 {
		return []target{{mod, imp.line}}, nil
	}
	out := make([]target, 0, len(imp.names))
	for _, n := range imp.names {
		if sub := mod + "." + n.text; modules[sub] != "" {
			out = append(out, target{sub, n.line})
		} else {
			out = append(out, target{mod, n.line})
		}
	}
	return out, nil
}

// resolveRelative turns a relative name such as "..x" into an absolute one
// from package parent, as importlib.util.resolve_name does.
func resolveRelative(name, parent string) (string, error) {
	rest := strings.TrimLeft(name, ".")
	level := len(name) - len(rest)
	if parent == "" {
		return "", errAboveTopLevel
	}
	parts := strings.Split(parent, ".")
	if len(parts) < level {
		return "", errAboveTopLevel
	}
	base := strings.Join(parts[:len(parts)-(level-1)], ".")
	if rest == "" {
		return base, nil
	}
	return base + "." + rest, nil
}

// excludedSource reports whether a file, or any directory above it, matches
// an exclusion glob.
func excludedSource(exclusions []string, file string) bool {
	for p := file; p != "." && p != "/"; p = path.Dir(p) {
		for _, pattern := range exclusions {
			if ok, _ := doublestar.Match(pattern, p); ok {
				return true
			}
		}
	}
	return false
}

// isIdentifier reports whether s is a Python identifier, as str.isidentifier
// answers for the names directories use.
func isIdentifier(s string) bool {
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i > 0 && (unicode.IsDigit(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc)):
		default:
			return false
		}
	}
	return s != ""
}

func isDir(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && info.IsDir()
}

func isFile(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && info.Mode().IsRegular()
}
