package initcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Python source layout constants.
const (
	pyInitFile = "__init__.py"
	pySrcDir   = "src"
)

// DiscoverPy reads Python packages from root and root/src (src-layout).
// A Python project is detected by pyproject.toml or setup.py at root.
// For each top-level package found, sub-packages are returned as individual
// modules. If a top-level package has no sub-packages it is returned itself.
func DiscoverPy(root string) ([]ModuleDef, error) {
	hasPyProject := fileExists(filepath.Join(root, "pyproject.toml"))
	hasSetupPy := fileExists(filepath.Join(root, "setup.py"))
	if !hasPyProject && !hasSetupPy {
		return nil, nil
	}

	type scanTarget struct {
		dir    string
		prefix string
	}
	targets := []scanTarget{
		{dir: root, prefix: ""},
		{dir: filepath.Join(root, pySrcDir), prefix: pySrcDir + "/"},
	}

	var mods []ModuleDef
	for _, t := range targets {
		entries, err := os.ReadDir(t.dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("initcfg: read dir %s: %w", t.dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			pkgDir := filepath.Join(t.dir, e.Name())
			if !fileExists(filepath.Join(pkgDir, pyInitFile)) {
				continue
			}
			// Found a top-level Python package — enumerate sub-packages.
			subMods := discoverPySubpackages(pkgDir, t.prefix+e.Name()+"/")
			if len(subMods) > 0 {
				mods = append(mods, subMods...)
			} else {
				// No sub-packages: return the top-level package as a single module.
				mod := pyDottedModule(t.prefix + e.Name())
				mods = append(mods, ModuleDef{
					Name:  e.Name(),
					Paths: pyModulePaths(mod),
					Layer: layerCore,
				})
			}
		}
	}
	return mods, nil
}

// discoverPySubpackages scans immediate subdirectories of pkgDir that contain
// __init__.py and returns one ModuleDef per sub-package.
// pathPrefix is the path prefix for glob construction, e.g. "src/ccgram/".
func discoverPySubpackages(pkgDir, pathPrefix string) []ModuleDef {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	var mods []ModuleDef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		if !fileExists(filepath.Join(pkgDir, e.Name(), pyInitFile)) {
			continue
		}
		mod := pyDottedModule(pathPrefix + e.Name())
		mods = append(mods, ModuleDef{
			Name:  e.Name(),
			Paths: pyModulePaths(mod),
			Layer: inferPyLayer(e.Name()),
		})
	}
	return mods
}

// pyDottedModule converts a slash path (e.g. "src/ccgram/handlers" or
// "ccgram/handlers") to a dotted Python module path ("ccgram.handlers"). Python
// graph nodes are dotted module names, so paths: globs must be dotted too.
func pyDottedModule(slashPath string) string {
	s := strings.TrimPrefix(slashPath, pySrcDir+"/")
	return strings.ReplaceAll(strings.Trim(s, "/"), "/", ".")
}

// pyModulePaths returns the paths globs for a dotted Python module: the package
// itself and its submodules ("ccgram.handlers" + "ccgram.handlers.*").
func pyModulePaths(mod string) []string {
	return []string{mod, mod + ".*"}
}

// inferPyLayer maps common Python sub-package names to architectural layers.
func inferPyLayer(name string) string {
	switch name {
	case "handlers", "api", "routes", "views", "providers":
		return layerAdapter
	case "model", "models", "types", "schema":
		return layerModel
	case layerCmd, "cli":
		return layerCmd
	default:
		return layerCore
	}
}

// detectPyPackage scans root then root/src for the first directory that
// contains __init__.py (not starting with '.' or '_'). Returns its name.
func detectPyPackage(root string) string {
	for _, dir := range []string{root, filepath.Join(root, pySrcDir)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			if fileExists(filepath.Join(dir, e.Name(), pyInitFile)) {
				return e.Name()
			}
		}
	}
	return ""
}
