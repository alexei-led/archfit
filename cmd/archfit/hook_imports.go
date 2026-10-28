package main

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/config"
)

// The import forms post-tool-use reads from a source line. They read text,
// not a syntax tree: a hook must answer in well under a second, and
// can-import judges the target, so a false read costs one query.
var (
	// goImportOpen starts a parenthesised Go import block.
	goImportOpen = regexp.MustCompile(`^\s*import\s*\(`)
	// goImportSingle is `import "x"`, with an optional name, dot, or blank.
	goImportSingle = regexp.MustCompile(`^\s*import\s+(?:[\p{L}_][\p{L}\p{N}_]*\s+|[._]\s+)?"([^"]+)"`)
	// goImportSpec is one spec inside an import block.
	goImportSpec = regexp.MustCompile(`^\s*(?:[\p{L}_][\p{L}\p{N}_]*\s+|[._]\s*)?"([^"]+)"`)
	// tsImport is from '...', import '...', require('...'), or import('...').
	tsImport = regexp.MustCompile(`(?:\bfrom|\bimport|\brequire\s*\(|\bimport\s*\()\s*['"]([^'"]+)['"]`)
	// pyFrom is from a.b import c; a relative from (.a) is not read.
	pyFrom = regexp.MustCompile(`^\s*from\s+([A-Za-z_][\w.]*)\s+import\b`)
	// pyImport is import a.b, c as d.
	pyImport = regexp.MustCompile(`^\s*import\s+(.+)$`)
	// pyModule is a dotted module name.
	pyModule = regexp.MustCompile(`^[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*$`)
)

// tsResolveSuffixes are tried, in order, after a relative specifier.
var tsResolveSuffixes = []string{"", ".ts", ".tsx", ".js", ".jsx", "/index.ts", "/index.tsx", "/index.js"}

// importTargets returns the imports that the added lines of a source file
// introduce, spelled as policy can-import takes a target: a Go import path, a
// repository-relative TypeScript or JavaScript file, a dotted Python module.
// added holds the 1-based added line numbers; nil means every line is new.
// isFile reports whether a repository-relative path is a file.
func importTargets(lang, file string, content []byte, added map[int]bool, isFile func(string) bool) []string {
	isAdded := func(n int) bool { return added == nil || added[n] }
	lines := strings.Split(string(content), "\n")
	var out []string
	switch lang {
	case config.LangGo:
		out = goImports(lines, isAdded)
	case config.LangTypeScript:
		for i, line := range lines {
			if !isAdded(i + 1) {
				continue
			}
			for _, m := range tsImport.FindAllStringSubmatch(line, -1) {
				if target, ok := resolveRelative(file, m[1], isFile); ok {
					out = append(out, target)
				}
			}
		}
	case config.LangPython:
		for i, line := range lines {
			if isAdded(i + 1) {
				out = append(out, pyImports(line)...)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// goImports reads the import paths on added lines, inside an import block or
// in a single import declaration.
func goImports(lines []string, isAdded func(int) bool) []string {
	var out []string
	inBlock := false
	for i, line := range lines {
		rest := line
		switch {
		case inBlock:
		case goImportOpen.MatchString(line):
			inBlock = true
			rest = line[goImportOpen.FindStringIndex(line)[1]:]
		default:
			if m := goImportSingle.FindStringSubmatch(line); m != nil && isAdded(i+1) {
				out = append(out, m[1])
			}
			continue
		}
		if m := goImportSpec.FindStringSubmatch(rest); m != nil {
			if isAdded(i + 1) {
				out = append(out, m[1])
			}
			rest = rest[len(m[0]):]
		}
		if code, _, _ := strings.Cut(rest, "//"); strings.Contains(code, ")") {
			inBlock = false
		}
	}
	return out
}

// pyImports reads the dotted modules one Python line imports.
func pyImports(line string) []string {
	line, _, _ = strings.Cut(line, "#")
	if m := pyFrom.FindStringSubmatch(line); m != nil {
		return []string{m[1]}
	}
	m := pyImport.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	var out []string
	for _, part := range strings.Split(m[1], ",") {
		// "a.b as c" imports a.b.
		if fields := strings.Fields(part); len(fields) > 0 && pyModule.MatchString(fields[0]) {
			out = append(out, fields[0])
		}
	}
	return out
}

// resolveRelative resolves a relative specifier against the importing file,
// as a bundler would for the common extensions. A bare specifier, one that
// leaves the repository, or one that names no file is not resolved.
func resolveRelative(file, spec string, isFile func(string) bool) (string, bool) {
	if !strings.HasPrefix(spec, "./") && !strings.HasPrefix(spec, "../") {
		return "", false
	}
	base := path.Join(path.Dir(file), spec)
	if base == ".." || strings.HasPrefix(base, "../") {
		return "", false
	}
	for _, suffix := range tsResolveSuffixes {
		if candidate := base + suffix; isFile(candidate) {
			return candidate, true
		}
	}
	return "", false
}
