package syntax

import (
	"path/filepath"
	"strings"
)

// IsTestFile reports whether path is a test file by language convention.
// Used by ClassifyFile (FileClass=Test) and by metrics that exclude test files.
//
// Go:         *_test.go
// Python:     test_*.py, *_test.py, or any path containing a /tests/ segment
// TypeScript: *.test.ts, *.test.tsx, *.spec.ts, *.spec.tsx, or path containing __tests__/
// Rust:       any path containing a /tests/ path segment
//
// Ceiling: Rust inline #[cfg(test)] mod blocks are not detected by path.
func IsTestFile(lang, path string) bool {
	base := filepath.Base(path)
	switch lang {
	case "go":
		return strings.HasSuffix(base, "_test.go")
	case "python":
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		return strings.HasPrefix(base, "test_") ||
			strings.HasSuffix(stem, "_test") ||
			containsPathSegment(path, "tests")
	case "typescript":
		return strings.Contains(base, ".test.") ||
			strings.Contains(base, ".spec.") ||
			containsPathSegment(path, "__tests__")
	case "rust":
		return containsPathSegment(path, "tests")
	default:
		return false
	}
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
