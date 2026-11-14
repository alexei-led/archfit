package tsimports

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// defaultConditions are the conditions every lookup accepts; a tsconfig adds
// its customConditions.
var defaultConditions = []string{"types", "import", "require", "node", "default"}

// maxExtendsDepth stops an extends cycle.
const maxExtendsDepth = 10

// Reasons of an UnknownItem: why a tsconfig, or one it extends, is not read.
const (
	reasonPackage     = "extends not read: it names a package, which needs node_modules"
	reasonNodeModules = "extends not read: it is in node_modules"
	reasonOutside     = "extends not read: it is outside the root"
	reasonMissing     = "extends not read: no such file"
	reasonDepth       = "extends not read: more than 10 nested extends, or a loop"
	reasonNoParse     = "tsconfig not read: it does not parse as a JSON object"
)

const (
	jsonExt     = ".json"
	nodeModules = "node_modules"
	dtsExt      = ".d.ts"
	tsExt       = ".ts"
	tsxExt      = ".tsx"
	jsExt       = ".js"
	jsxExt      = ".jsx"
	mjsExt      = ".mjs"
	cjsExt      = ".cjs"
	mtsExt      = ".mts"
	ctsExt      = ".cts"
	dmtsExt     = ".d.mts"
	dctsExt     = ".d.cts"
)

// settings are the resolution settings of one tsconfig after its extends
// chain, with TypeScript's defaults.
type settings struct {
	paths *object // pattern to a list of targets, in document order
	// pathsBase is the directory paths targets resolve against: baseUrl when
	// any config in the chain sets it, else the directory of the config that
	// sets paths.
	pathsBase  string
	baseURL    string
	hasBaseURL bool
	conditions []string
	exports    bool // false under node10 module resolution, which ignores exports
	// unread lists the configs of the chain that are not read. Any one of
	// them can set paths, so the chain is incomplete.
	unread []UnknownItem
}

// tsOptions are the options one tsconfig chain sets. A nil field is unset.
type tsOptions struct {
	baseURL    *string
	paths      *object
	pathsDir   string
	hasPaths   bool
	conditions []string
	exports    *bool
	unread     []UnknownItem
}

// override sets every option that o sets over the receiver and keeps the
// configs of both that are not read.
func (t *tsOptions) override(o tsOptions) {
	t.unread = append(t.unread, o.unread...)
	if o.baseURL != nil {
		t.baseURL = o.baseURL
	}
	if o.hasPaths {
		t.paths, t.pathsDir, t.hasPaths = o.paths, o.pathsDir, true
	}
	if o.conditions != nil {
		t.conditions = o.conditions
	}
	if o.exports != nil {
		t.exports = o.exports
	}
}

func (t tsOptions) settings() settings {
	s := settings{paths: t.paths, pathsBase: t.pathsDir, conditions: defaultConditions, exports: true, unread: t.unread}
	if t.baseURL != nil {
		s.baseURL, s.hasBaseURL, s.pathsBase = *t.baseURL, true, *t.baseURL
	}
	if t.conditions != nil {
		s.conditions = t.conditions
	}
	if t.exports != nil {
		s.exports = *t.exports
	}
	return s
}

// tsconfig returns the settings of the nearest tsconfig.json at or above the
// root-relative directory dir, falling back to the root tsconfig.base.json.
// Results are cached per directory.
func (r *resolver) tsconfig(dir string) settings {
	if s, ok := r.configs[dir]; ok {
		return s
	}
	var s settings
	for current := dir; ; current = path.Dir(current) {
		name := path.Join(current, "tsconfig.json")
		if r.isFile(name) {
			s = r.tsconfigOptions(name, 0).settings()
			break
		}
		if current == "." {
			s = tsOptions{}.settings()
			if r.isFile("tsconfig.base.json") {
				s = r.tsconfigOptions("tsconfig.base.json", 0).settings()
			}
			break
		}
	}
	r.configs[dir] = s
	return s
}

// tsconfigOptions returns the options a tsconfig sets, merged over its parents
// in extends order. Only relative extends inside the root and outside
// node_modules are followed: a dependency's config depends on the install.
// Every config not read is in the unread list of the result.
func (r *resolver) tsconfigOptions(name string, depth int) tsOptions {
	data := readJSONC(r.fsys, name)
	if data == nil {
		return tsOptions{unread: []UnknownItem{{File: name, Reason: reasonNoParse}}}
	}
	var merged tsOptions
	for _, parent := range stringList(data.get("extends")) {
		target, reason := r.extendsTarget(name, parent, depth)
		if reason != "" {
			merged.unread = append(merged.unread, UnknownItem{File: name, Specifier: parent, Reason: reason})
			continue
		}
		merged.override(r.tsconfigOptions(target, depth+1))
	}
	options := asObject(data.get("compilerOptions"))
	here := path.Dir(name)
	if baseURL, ok := asString(options.get("baseUrl")); ok {
		// An absolute baseUrl stays absolute, so no lookup under it finds a file.
		joined := path.Clean(baseURL)
		if !path.IsAbs(baseURL) {
			joined = path.Join(here, baseURL)
		}
		merged.baseURL = &joined
	}
	if options.has("paths") {
		merged.paths, merged.pathsDir, merged.hasPaths = asObject(options.get("paths")), here, true
	}
	if options.has("customConditions") {
		merged.conditions = slices.Concat(defaultConditions, stringList(options.get("customConditions")))
	}
	if options.has("moduleResolution") {
		mode := strings.ToLower(fmt.Sprint(options.get("moduleResolution")))
		exports := mode != "node" && mode != "node10"
		merged.exports = &exports
	}
	return merged
}

// extendsTarget returns the root-relative file that the extends value parent
// of the tsconfig name names, or the reason it is not read. name is at the
// given depth of the extends chain.
func (r *resolver) extendsTarget(name, parent string, depth int) (target, reason string) {
	switch {
	case depth >= maxExtendsDepth:
		return "", reasonDepth
	case path.IsAbs(parent):
		return "", reasonOutside
	case !strings.HasPrefix(parent, "."):
		return "", reasonPackage
	}
	target = path.Join(path.Dir(name), parent)
	switch {
	case !fs.ValidPath(target):
		return "", reasonOutside
	case inNodeModules(target):
		return "", reasonNodeModules
	}
	if r.isDir(target) {
		target = path.Join(target, "tsconfig.json")
	}
	if path.Ext(target) != jsonExt {
		target += jsonExt
	}
	if !r.isFile(target) {
		return "", reasonMissing
	}
	return target, ""
}

// stringList returns the strings of a string or a list value.
func stringList(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// workspacePackages maps each workspace package name to its root-relative
// directory. Patterns come from package.json workspaces (npm, Yarn) and from
// pnpm-workspace.yaml packages; a "!" pattern excludes.
func workspacePackages(fsys fs.FS) map[string]string {
	workspaces := readJSON(fsys, "package.json").get("workspaces")
	if o := asObject(workspaces); o != nil {
		workspaces = o.get("packages")
	}
	var patterns, excludes []string
	if list, ok := workspaces.([]any); ok {
		patterns = stringList(list)
	}
	patterns = append(patterns, pnpmPatterns(fsys)...)
	var includes []string
	for _, p := range patterns {
		p = strings.TrimPrefix(p, "./")
		if rest, ok := strings.CutPrefix(p, "!"); ok {
			excludes = append(excludes, strings.TrimPrefix(rest, "./"))
		} else if p != "" {
			includes = append(includes, p)
		}
	}
	packages := map[string]string{}
	for _, pattern := range includes {
		// Like pathlib, the walk does not follow symlinks: pnpm links packages
		// into each other's node_modules, and following them can loop.
		matches, err := doublestar.Glob(fsys, pattern, doublestar.WithNoFollow())
		if err != nil {
			continue
		}
		slices.Sort(matches)
		for _, dir := range matches {
			if inNodeModules(dir) || excluded(excludes, dir) {
				continue
			}
			name, ok := asString(readJSON(fsys, path.Join(dir, "package.json")).get("name"))
			if !ok {
				continue
			}
			packages[name] = dir
		}
	}
	return packages
}

func excluded(globs []string, dir string) bool {
	return slices.ContainsFunc(globs, func(g string) bool {
		ok, _ := doublestar.Match(g, dir)
		return ok
	})
}

// pnpmPatterns returns the block-list packages entries of pnpm-workspace.yaml.
// A flow list (packages: [a, b]) is not read; the corpus has none.
func pnpmPatterns(fsys fs.FS) []string {
	data, err := fs.ReadFile(fsys, "pnpm-workspace.yaml")
	if err != nil {
		return nil
	}
	var patterns []string
	inside := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		text, _, _ := strings.Cut(line, " #")
		text = strings.TrimRight(text, " \t")
		trimmed := strings.TrimLeft(text, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		item := strings.HasPrefix(trimmed, "- ")
		topLevel := line[0] != ' ' && line[0] != '\t'
		switch {
		case topLevel && !item:
			inside = text == "packages:"
		case inside && item:
			patterns = append(patterns, strings.Trim(strings.TrimSpace(trimmed[2:]), `'"`))
		}
	}
	return patterns
}
