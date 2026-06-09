package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// sourceExts are the file extensions counted for the structural_weight metric.
var sourceExts = map[string]bool{
	".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
}

// skipDirs are directory names never walked for source LOC.
var skipDirs = map[string]bool{
	"node_modules": true, "dist": true, "vendor": true, "venv": true,
	".venv": true, "build": true, "testdata": true, "mocks": true,
}

// sourceFileLOC returns repo-relative source file paths to their line counts,
// excluding tests and generated/dependency directories. It is best-effort: an
// unreadable file or root contributes nothing. This feeds the structural_weight
// (size-skew / god-module) metric.
func sourceFileLOC(root string) map[string]int {
	out := make(map[string]int)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExts[filepath.Ext(name)] || isTestFile(name) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if n := countLines(path); n > 0 {
			out[filepath.ToSlash(rel)] = n
		}
		return nil
	})
	return out
}

// isTestFile reports whether a filename is a test/mock file (excluded from size).
func isTestFile(name string) bool {
	switch {
	case strings.HasSuffix(name, "_test.go"), strings.HasPrefix(name, "mock_"):
		return true
	case strings.HasPrefix(name, "test_"), strings.HasSuffix(name, "_test.py"), name == "conftest.py":
		return true
	case strings.Contains(name, ".test."), strings.Contains(name, ".spec."), strings.HasSuffix(name, ".d.ts"):
		return true
	}
	return false
}

// countLines returns the number of lines in a file, or 0 on error.
func countLines(path string) int {
	f, err := os.Open(path) //nolint:gosec // path comes from a directory walk of the project
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	return n
}
