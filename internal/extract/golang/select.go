package golang

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// selection is whether the go command reads a Go file for its package.
type selection int

const (
	selectNo selection = iota
	selectYes
	selectUndecided
)

// The decisions in this file are the ones ParseImports and QueryEdge share,
// so `policy can-import` and the check read the same files and place an
// import on the same package.

// placeImport places a Go import path on the loaded member whose module path
// is its longest prefix and returns the package dir relative to the scan root,
// spelled as a package node: "" for the root package. An import path no member
// prefixes is not first-party: the standard library or another module. So the
// standard library `log` never gets the dir of a first-party `<root>/log`.
// members must be sorted by sortModEntries.
func placeImport(members []modEntry, importPath string) (dir string, firstParty bool) {
	for _, m := range members {
		if importPath == m.path || strings.HasPrefix(importPath, m.path+"/") {
			return packageDir(path.Join(m.relDir, strings.TrimPrefix(importPath, m.path), "x")), true
		}
	}
	return "", false
}

// packageDir is the package node of a scan-root-relative file path.
func packageDir(rel string) string {
	if dir := path.Dir(rel); dir != "." {
		return dir
	}
	return ""
}

// fileReason names why the go command never reads the scan-root-relative Go
// file rel for a package of a loaded member, or returns "" when it may. The
// build constraints are buildSelects' decision.
func fileReason(scanRoot string, exclusions []string, members []modEntry, rel string) string {
	name := path.Base(rel)
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return rel + " is a test file, which the load does not read"
	case strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_"):
		return rel + " starts with \".\" or \"_\", which the go command ignores"
	case excluded(exclusions, rel):
		return rel + " matches an exclude: glob"
	}
	member, ok := memberOf(scanRoot, members, rel)
	if !ok {
		return rel + " belongs to no loaded Go module"
	}
	if dir := skippedDir(member.relDir, rel); dir != "" {
		return rel + " is under " + dir + ", which the go command skips"
	}
	return ""
}

// memberOf returns the loaded member whose go.mod is the nearest above rel. A
// file whose nearest go.mod is a nested module the run does not load has none.
func memberOf(scanRoot string, members []modEntry, rel string) (modEntry, bool) {
	for dir := path.Dir(rel); ; dir = path.Dir(dir) {
		if hasGoMod(filepath.Join(scanRoot, filepath.FromSlash(dir))) {
			for _, m := range members {
				if m.relDir == dir {
					return m, true
				}
			}
			return modEntry{}, false
		}
		if dir == "." || dir == "/" {
			return modEntry{}, false
		}
	}
}

// skippedDir returns the dir of rel below its member dir that the go command
// skips for "./...", or "".
func skippedDir(memberDir, rel string) string {
	below := rel
	if memberDir != "." {
		below = strings.TrimPrefix(rel, memberDir+"/")
	}
	segments := strings.Split(path.Dir(below), "/")
	for i, segment := range segments {
		if segment != "." && skipDirName(segment) {
			return path.Join(memberDir, path.Join(segments[:i+1]...))
		}
	}
	return ""
}

// skipDirName reports whether the go command skips a directory of this name
// for "./...".
func skipDirName(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor"
}

// buildSelects decides whether the go toolchain the run would start reads the
// scan-root-relative Go file rel: the build context toolchainContext gives the
// extractor (GOOS, GOARCH, -tags of GOFLAGS, the go env file, the build
// flags), and a file that imports "C" is ignored when cgo is off. With
// CGO_ENABLED unset, cgo depends on a C compiler this binary cannot see, so
// both settings are tried: when they disagree the answer is undecided. A file
// that does not exist yet is judged by its name. The reason names a no or an
// undecided answer. A build constraint MatchFile cannot read counts as a match
// and comes back as the error, for the caller to decide.
func buildSelects(scanRoot string, buildFlags []string, rel string, importsC bool) (selection, string, error) {
	settings := []bool{true, false}
	switch goEnv("CGO_ENABLED") {
	case "1":
		settings = []bool{true}
	case "0":
		settings = []bool{false}
	}
	dir := filepath.Join(scanRoot, filepath.FromSlash(path.Dir(rel)))
	matches := make([]bool, 0, len(settings))
	var matchErr error
	for _, cgo := range settings {
		ctxt := toolchainContext(goEnv, buildFlags)
		ctxt.CgoEnabled = cgo
		ctxt.OpenFile = openOrEmpty
		ok, err := ctxt.MatchFile(dir, path.Base(rel))
		if err != nil && matchErr == nil {
			matchErr = err
		}
		matches = append(matches, err != nil || ok)
	}
	cgoOff := len(settings) == 1 && !settings[0]
	switch {
	case cgoOff && importsC:
		return selectNo, rel + " imports \"C\" and cgo is off, so the load ignores it", nil
	case len(matches) == 2 && matches[0] != matches[1]:
		return selectUndecided, fmt.Sprintf("whether the load reads %s depends on cgo, and CGO_ENABLED is not set", rel), nil
	case !matches[0]:
		return selectNo, rel + " is excluded by the build constraints", nil
	case len(settings) == 2 && importsC:
		return selectUndecided, rel + " imports \"C\": whether the load reads it depends on whether cgo is on, and CGO_ENABLED is not set", nil
	}
	return selectYes, "", matchErr
}

// openOrEmpty opens a Go source for MatchFile; a file that does not exist yet
// reads as a bare package clause, so only its name decides.
func openOrEmpty(p string) (io.ReadCloser, error) {
	f, err := os.Open(p) // #nosec G304 -- a Go source file under the scan root
	if os.IsNotExist(err) {
		return io.NopCloser(strings.NewReader("package p\n")), nil
	}
	return f, err
}
