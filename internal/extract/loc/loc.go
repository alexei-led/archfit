// Package loc counts source-file line counts for the structural_weight metric.
// It walks the repository root, skips test files, generated/dependency
// directories, and dot-dirs, and returns a repo-relative slash-normalised map
// of path → line count together with a diagnostic coverage record.
package loc

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

const toolName = "loc"

// sourceExts are the file extensions counted for the structural_weight metric.
var sourceExts = map[string]bool{
	".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
}

// skipDirs are directory names never walked for source LOC.
var skipDirs = map[string]bool{
	"node_modules": true, "dist": true, "vendor": true, "venv": true,
	".venv": true, "build": true, "testdata": true, "mocks": true,
}

// Run walks root and returns a repo-relative path→LOC map plus a coverage
// record. The walk is best-effort: unreadable files contribute nothing.
// FilesSeen and FilesApplicable equal the map length; status is always ok
// because the walk is a pure filesystem operation with no external tool.
func Run(root string) (map[string]int, diagnostic.Coverage, error) {
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
	cov := diagnostic.Coverage{
		Tool:            toolName,
		FilesSeen:       len(out),
		FilesApplicable: len(out),
		Status:          diagnostic.StatusOK,
	}
	return out, cov, nil
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
