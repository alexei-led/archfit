package syntax

import (
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

// IsTestFile reports whether path is a test file by language convention.
// Used by ClassifyFile (FileClass=Test) and by metrics that exclude test files.
//
// Go:         *_test.go
// Python:     test_*.py, *_test.py, or any path containing a /tests/ segment
// TypeScript/JavaScript: any filename containing .test. or .spec., or a path containing __tests__/
// Rust:       tests.rs, *_test.rs, *_tests.rs, or a tests/, benches/, *_tests/, or *-tests/ dir
//
// Ceiling: Rust classification is file-level. A `#[cfg(test)] mod tests { … }`
// block inside a production file stays Production; only a test module in its own
// sibling file (`mod tests;` → tests.rs) is recognised. Directory suffixes need a
// separator, so contests/ and attests/ stay Production.
func IsTestFile(lang, path string) bool {
	base := filepath.Base(path)
	switch lang {
	case graph.LangGo:
		return strings.HasSuffix(base, "_test.go")
	case graph.LangPython:
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		return strings.HasPrefix(base, "test_") ||
			strings.HasSuffix(stem, "_test") ||
			containsPathSegment(path, "tests")
	case graph.LangTypeScript:
		return strings.Contains(base, ".test.") ||
			strings.Contains(base, ".spec.") ||
			containsPathSegment(path, "__tests__")
	case graph.LangRust:
		stem := strings.TrimSuffix(base, ".rs")
		return base == "tests.rs" ||
			strings.HasSuffix(stem, "_test") || strings.HasSuffix(stem, "_tests") ||
			containsRustTestDir(path)
	default:
		return false
	}
}

// containsRustTestDir reports whether a directory segment of path holds Rust test
// or benchmark code: tests/, benches/, or a test-suite directory such as
// property_tests/. The file name itself is not a directory segment.
func containsRustTestDir(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, dir := range parts[:len(parts)-1] {
		if dir == "tests" || dir == "benches" || strings.HasSuffix(dir, "_tests") || strings.HasSuffix(dir, "-tests") {
			return true
		}
	}
	return false
}

// containsPathSegment reports whether path contains the given directory name segment.
// Handles both forward and back slashes.
func containsPathSegment(path, seg string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == seg {
			return true
		}
	}
	return false
}
