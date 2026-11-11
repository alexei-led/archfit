package resolved

import "path"

// Covers reports whether glob matches path: the one glob rule ModuleOf and
// CheckTree use. path is repository-relative and slash-separated, a file or a
// directory; a directory is matched as a whole, as CheckTree matches a public
// glob, so pkg/chaos covers the directory pkg/chaos and not the file
// pkg/chaos/chaos.go.
func (p Policy) Covers(glob, path string) bool {
	return matchGlob(glob, path)
}

// IsPublic reports whether target, a file or a directory, is in the public
// part of module. A public glob covers the target itself, or, when the target
// is a file, the directory that holds it: public: [pkg/chaos] names the package
// pkg/chaos, so the file pkg/chaos/chaos.go is public and the package
// pkg/chaos/netem is not. A Go import names a package directory; a TypeScript
// or Python import names a file. target is in module, as ModuleOf or the
// caller decided. p has its defaults (WithDefaults): a module that names no
// public is public as a whole, so every target in it is public, even a package
// directory its file globs (pkg/foo/*.go) do not match. False for a name that
// is not a module.
func (p Policy) IsPublic(module, target string, isFile bool) bool {
	m, ok := p.Modules[module]
	if !ok {
		return false
	}
	if wholeModule(m) {
		return true
	}
	for _, glob := range m.Public {
		if matchGlob(glob, target) || isFile && matchGlob(glob, path.Dir(target)) {
			return true
		}
	}
	return false
}

// IsTest reports whether a glob of the policy's tests covers file. The rules
// skip such a file.
func (p Policy) IsTest(file string) bool {
	return matchesGlobs(p.Tests, file)
}

// IsGenerated reports whether a glob of the policy's generated covers file.
// The rules skip such a file.
func (p Policy) IsGenerated(file string) bool {
	return matchesGlobs(p.Generated, file)
}

func matchesGlobs(globs []string, file string) bool {
	for _, glob := range globs {
		if matchGlob(glob, file) {
			return true
		}
	}
	return false
}
