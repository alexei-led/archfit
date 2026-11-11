package tsimports

import (
	"cmp"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// probeExts are the extensions a specifier without one may omit, in
// resolution order. The first eight are the script files parsedExts reads.
var probeExts = []string{
	".js", ".cjs", ".mjs", ".jsx", tsExt, ".cts", ".mts", tsxExt, dtsExt, jsonExt, ".vue", ".svelte",
}

// jsToTS lists what a specifier extension may name instead: a ".js" specifier
// may name a TypeScript file (TypeScript's ESM convention), and a ".ts" one
// its declaration file.
var jsToTS = map[string][]string{
	".js":  {tsExt, tsxExt, dtsExt},
	tsExt:  {dtsExt},
	".jsx": {tsxExt},
	".mjs": {".mts"},
	".cjs": {".cts"},
}

// resolver places specifiers on root-relative files without node_modules.
// It is not safe for concurrent use.
type resolver struct {
	fsys     fs.FS
	packages map[string]string // workspace package name to its directory
	names    []string          // package names, longest first
	configs  map[string]settings
}

func newResolver(fsys fs.FS) *resolver {
	packages := workspacePackages(fsys)
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), cmp.Compare(a, b))
	})
	return &resolver{fsys: fsys, packages: packages, names: names, configs: map[string]settings{}}
}

// resolve places the specifier spec of the root-relative file importer. It
// returns KindFile and the file, or KindUnknown or KindExternal and "".
func (r *resolver) resolve(importer, spec string) (Kind, string) {
	spec, _, _ = strings.Cut(spec, "?")
	if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
		base := spec // an absolute path is outside the root: file rejects it
		if !path.IsAbs(spec) {
			base = path.Join(path.Dir(importer), spec)
		}
		if inNodeModules(base) {
			return KindExternal, ""
		}
		return r.placed(r.file(base, nil))
	}
	if strings.HasPrefix(spec, "node:") {
		return KindExternal, ""
	}
	config := r.tsconfig(path.Dir(importer))
	if strings.HasPrefix(spec, "#") {
		return r.placed(r.packageImport(importer, spec, config.conditions))
	}
	pattern, matched, targets := aliasTargets(config, spec)
	for _, target := range targets {
		// An alias into node_modules names a dependency, installed or not.
		if inNodeModules(target) {
			return KindExternal, ""
		}
		if found, ok := r.file(target, nil); ok {
			return KindFile, found
		}
	}
	// TypeScript tries baseUrl only when no paths pattern matched.
	if !matched && config.hasBaseURL {
		if found, ok := r.file(path.Join(config.baseURL, spec), nil); ok {
			return KindFile, found
		}
	}
	if name, dir, ok := r.packageOf(spec); ok {
		return r.placed(r.packageEntry(dir, "."+spec[len(name):], config))
	}
	// A configured alias names first-party code; a bare "*" also matches every
	// third-party package, so it alone proves nothing.
	if matched && pattern != "*" {
		return KindUnknown, ""
	}
	return KindExternal, ""
}

func (r *resolver) placed(file string, ok bool) (Kind, string) {
	if ok {
		return KindFile, file
	}
	return KindUnknown, ""
}

// file probes a root-relative base path as a file, with extensions, then as
// a directory with a package.json entry or an index file. It never answers
// with a file under node_modules, so the answer does not depend on whether
// dependencies are installed. seen stops a package.json entry that names its
// own directory.
func (r *resolver) file(base string, seen []string) (string, bool) {
	base = path.Clean(base)
	if !fs.ValidPath(base) || inNodeModules(base) || slices.Contains(seen, base) {
		return "", false
	}
	if r.isFile(base) {
		return base, true
	}
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for _, alt := range jsToTS[ext] {
		if r.isFile(stem + alt) {
			return stem + alt, true
		}
	}
	for _, ext := range probeExts {
		if r.isFile(base + ext) {
			return base + ext, true
		}
	}
	if !r.isDir(base) {
		return "", false
	}
	manifest := readJSON(r.fsys, path.Join(base, "package.json"))
	for _, field := range []string{"types", "module", "main"} {
		if entry, ok := asString(manifest.get(field)); ok {
			if found, ok := r.file(path.Join(base, entry), append(slices.Clip(seen), base)); ok {
				return found, true
			}
		}
	}
	for _, ext := range probeExts {
		if index := path.Join(base, "index"+ext); r.isFile(index) {
			return index, true
		}
	}
	return "", false
}

// inNodeModules reports whether a root-relative path has a node_modules segment.
func inNodeModules(name string) bool {
	return slices.Contains(strings.Split(path.Clean(name), "/"), nodeModules)
}

func (r *resolver) isFile(name string) bool {
	info, err := fs.Stat(r.fsys, name)
	return err == nil && info.Mode().IsRegular()
}

func (r *resolver) isDir(name string) bool {
	info, err := fs.Stat(r.fsys, name)
	return err == nil && info.IsDir()
}

// packageOf returns the workspace package a bare specifier names.
func (r *resolver) packageOf(spec string) (name, dir string, ok bool) {
	for _, name := range r.names {
		if spec == name || strings.HasPrefix(spec, name+"/") {
			return name, r.packages[name], true
		}
	}
	return "", "", false
}

// packageEntry resolves "." or "./sub" inside a workspace package. exports
// decides when present, except under node10 module resolution, which ignores
// it, as TypeScript does.
func (r *resolver) packageEntry(dir, subpath string, config settings) (string, bool) {
	manifest := readJSON(r.fsys, path.Join(dir, "package.json"))
	if exports := manifest.get("exports"); exports != nil && config.exports {
		for _, target := range exportTargets(exports, subpath, config.conditions) {
			if found, ok := r.file(path.Join(dir, target), nil); ok {
				return found, true
			}
		}
		return "", false
	}
	if subpath == "." {
		return r.file(dir, nil)
	}
	return r.file(path.Join(dir, subpath[2:]), nil)
}

// packageImport resolves a "#name" subpath import through the imports field
// of the package.json nearest to the importer.
func (r *resolver) packageImport(importer, spec string, conditions []string) (string, bool) {
	for dir := path.Dir(importer); ; dir = path.Dir(dir) {
		manifest := path.Join(dir, "package.json")
		if r.isFile(manifest) {
			imports := readJSON(r.fsys, manifest).get("imports")
			for _, target := range exportTargets(imports, spec, conditions) {
				if found, ok := r.file(path.Join(dir, target), nil); ok {
					return found, true
				}
			}
			return "", false
		}
		if dir == "." {
			return "", false
		}
	}
}

// aliasTargets returns the tsconfig paths pattern spec matches and its
// root-relative candidates. An exact pattern wins; otherwise the wildcard
// with the longest prefix, the first in document order on a tie.
func aliasTargets(config settings, spec string) (pattern string, matched bool, targets []string) {
	var bestPrefix, star string
	for _, p := range keys(config.paths) {
		prefix, suffix, wildcard := strings.Cut(p, "*")
		if !wildcard {
			if spec == p {
				pattern, matched, star = p, true, ""
				break
			}
			continue
		}
		if strings.HasPrefix(spec, prefix) && strings.HasSuffix(spec, suffix) &&
			len(spec) >= len(prefix)+len(suffix) && (!matched || len(prefix) > len(bestPrefix)) {
			pattern, matched, bestPrefix = p, true, prefix
			star = spec[len(prefix) : len(spec)-len(suffix)]
		}
	}
	if !matched {
		return "", false, nil
	}
	for _, target := range stringList(config.paths.get(pattern)) {
		targets = append(targets, path.Join(config.pathsBase, strings.ReplaceAll(target, "*", star)))
	}
	return pattern, true, targets
}

func keys(o *object) []string {
	if o == nil {
		return nil
	}
	return o.keys
}

// exportTargets returns the targets of an exports or imports map for a
// subpath: every target whose condition is active, in document order. The
// caller takes the first that names a file, so an unbuilt dist target falls
// through. An explicit null blocks the subpath, as in Node.
func exportTargets(exports any, subpath string, conditions []string) []string {
	m := asObject(exports)
	switch exports.(type) {
	case string, []any:
		m = nil
	case *object:
	default:
		return nil
	}
	if m == nil || !slices.ContainsFunc(m.keys, isSubpathKey) {
		m = &object{keys: []string{"."}, vals: map[string]any{".": exports}}
	}
	entry, star := m.get(subpath), ""
	if !m.has(subpath) {
		var wildcards []string
		for _, k := range m.keys {
			if strings.Contains(k, "*") {
				wildcards = append(wildcards, k)
			}
		}
		slices.SortStableFunc(wildcards, func(a, b string) int {
			return cmp.Compare(strings.Index(b, "*"), strings.Index(a, "*"))
		})
		for _, k := range wildcards {
			prefix, suffix, _ := strings.Cut(k, "*")
			if strings.HasPrefix(subpath, prefix) && strings.HasSuffix(subpath, suffix) &&
				len(subpath) >= len(prefix)+len(suffix) {
				entry, star = m.get(k), subpath[len(prefix):len(subpath)-len(suffix)]
				break
			}
		}
	}
	targets := conditionTargets(entry, conditions)
	for i, t := range targets {
		targets[i] = strings.ReplaceAll(t, "*", star)
	}
	return targets
}

func isSubpathKey(k string) bool {
	return strings.HasPrefix(k, ".") || strings.HasPrefix(k, "#")
}

func conditionTargets(entry any, conditions []string) []string {
	switch entry := entry.(type) {
	case string:
		return []string{entry}
	case []any:
		var targets []string
		for _, item := range entry {
			targets = append(targets, conditionTargets(item, conditions)...)
		}
		return targets
	case *object:
		var targets []string
		for _, key := range entry.keys {
			if !slices.Contains(conditions, key) {
				continue
			}
			value := entry.vals[key]
			if value == nil {
				break
			}
			targets = append(targets, conditionTargets(value, conditions)...)
		}
		return targets
	}
	return nil
}
