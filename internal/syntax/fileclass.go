// Package syntax classifies each source file into a FileClass
// (Production/Test/Generated/Vendor) and exposes file-level helpers
// (LookupFileClass, IsTestFile) used by the LOC walk and by metrics that
// filter on Production files. It is part of the core ring: it decides over
// already-gathered facts and runs no subprocess.
package syntax

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/model/fileclass"
)

// generatedMarker is the standard generated-code header byte sequence.
// Matches both Go (// Code generated …) and most other generators.
var generatedMarker = []byte("Code generated")

// doNotEditMarker is used alongside generatedMarker for the full sentinel.
var doNotEditMarker = []byte("DO NOT EDIT")

// FileClassConfig holds the user-supplied patterns for classifying source files.
// Auto-detection runs first; patterns here extend/override it for custom
// mock frameworks and project-specific conventions. Build this once from
// config and pass it to ClassifyFile.
//
// All glob patterns use forward slashes and are matched against the
// repo-relative path. Both the filename (base) and the full path are checked
// so patterns like "mocks/" (directory segment) and "mock_*.go" (filename
// prefix) both work.
type FileClassConfig struct {
	// GeneratedGlobs are additional glob patterns (matched against the
	// repo-relative slash path) that classify a file as Generated.
	GeneratedGlobs []string
	// TestGlobs are additional glob patterns that classify a file as Test.
	TestGlobs []string
	// MockFrameworks contains filename patterns (prefix or suffix) that
	// identify mock files as Generated; matched against the base filename only.
	MockFrameworks []string
}

// ClassifyFile returns the FileClass for a source file.
//
// Priority (first match wins):
//  1. Generated — header sniff (generatedMarker + doNotEditMarker) in the
//     first 512 bytes of header.
//  2. Generated — filename patterns: *.pb.go, *_gen.go, *.gen.go, *.gen.ts,
//     *_pb2.py, mock_*.go, *_mock.go, *_moq.go.
//  3. Generated — config GeneratedGlobs / MockFrameworks.
//  4. Vendor — path segment "vendor/".
//  5. Test — language-convention path match (reuses IsTestFile).
//  6. Test — config TestGlobs.
//  7. Production — default.
//
// header should be the first ~512 bytes of the file, or nil/empty when the
// content is not available (header sniff is skipped).
func ClassifyFile(lang, path string, header []byte, cfg FileClassConfig) fileclass.FileClass {
	base := filepath.Base(path)
	slash := filepath.ToSlash(path)

	// 1. Header sniff — generated marker (most authoritative).
	if len(header) > 0 && hasGeneratedMarker(header) {
		return fileclass.Generated
	}

	// 2. Built-in filename patterns for generated/mock files.
	if isGeneratedFilename(base) {
		return fileclass.Generated
	}

	// 3. Config-supplied generated/mock patterns.
	if matchesAny(slash, cfg.GeneratedGlobs) {
		return fileclass.Generated
	}
	for _, mf := range cfg.MockFrameworks {
		if mf == "" {
			continue // empty entry matches every filename — skip
		}
		if strings.HasPrefix(base, mf) || strings.HasSuffix(strings.TrimSuffix(base, filepath.Ext(base)), strings.TrimSuffix(mf, "*")) {
			return fileclass.Generated
		}
	}

	// 4a. Well-known generated/mock directory segments.
	//     Files in mocks/ or __mocks__/ are conventionally generated mock code
	//     (gomock, moq, mockery, Jest). loc's walk skips these dirs entirely, so
	//     they never appear in the FileClassIndex; the path-only fallback must
	//     classify them without a header sniff (C6).
	if containsPathSegment(slash, "mocks") || containsPathSegment(slash, "__mocks__") {
		return fileclass.Generated
	}

	// 4b. Vendor directory.
	if containsPathSegment(slash, "vendor") {
		return fileclass.Vendor
	}

	// 5. Language-convention test files.
	if IsTestFile(lang, path) {
		return fileclass.Test
	}

	// 6. Config-supplied test patterns.
	if matchesAny(slash, cfg.TestGlobs) {
		return fileclass.Test
	}

	return fileclass.Production
}

// LookupFileClass resolves the class for a repo-relative slash path by
// consulting the precomputed index first, falling back to path-only
// classification (no header, no config overrides) when the path is absent
// from the index (e.g. files in loc's skipDirs such as mocks/).
//
// This two-phase design lets Tasks 6-9 correctly exclude files that never
// appeared in the loc walk (skipDirs) while still benefiting from the header-
// sniff results for files that did.
func LookupFileClass(path string, index map[string]fileclass.FileClass, lang string, cfg FileClassConfig) fileclass.FileClass {
	if fc, ok := index[path]; ok {
		return fc
	}
	// Fallback: path-only, no header.
	return ClassifyFile(lang, path, nil, cfg)
}

// hasGeneratedMarker reports whether the header bytes contain both the
// standard "Code generated" and "DO NOT EDIT" markers.
func hasGeneratedMarker(header []byte) bool {
	return bytes.Contains(header, generatedMarker) && bytes.Contains(header, doNotEditMarker)
}

// isGeneratedFilename reports whether a base filename matches well-known
// generated-code naming conventions across Go, Python, and TypeScript.
func isGeneratedFilename(base string) bool {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	switch {
	// Go proto / gRPC stubs.
	case strings.HasSuffix(base, ".pb.go"):
		return true
	// Go generated suffix patterns.
	case strings.HasSuffix(stem, "_gen"), strings.HasPrefix(stem, "gen_"):
		return true
	// Go moq / gomock / testify mock naming.
	case strings.HasPrefix(base, "mock_"), strings.HasSuffix(stem, "_mock"),
		strings.HasSuffix(stem, "_moq"):
		return true
	// TypeScript type-declaration files (*.d.ts) — emitted by tsc, never
	// hand-authored production logic.
	case strings.HasSuffix(base, ".d.ts"):
		return true
	// TypeScript/JavaScript generated files.
	case strings.HasSuffix(stem, ".gen"):
		return true
	// Python proto stubs.
	case strings.HasSuffix(stem, "_pb2"):
		return true
	}
	return false
}

// matchesAny reports whether path matches any of the provided glob patterns.
// Patterns use forward slashes and are matched against the full repo-relative
// path or the base filename. Supports ** doublestar semantics (via
// github.com/bmatcuk/doublestar/v4) so user-supplied globs like gen/** or
// **/generated/*.go work as expected.
func matchesAny(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, pat := range patterns {
		// Simple substring for directory-segment patterns (e.g. "mocks/").
		if strings.HasSuffix(pat, "/") {
			if containsPathSegment(path, strings.TrimSuffix(pat, "/")) {
				return true
			}
			continue
		}
		// Match against the base filename (handles patterns like "mock_*.go").
		if ok, _ := doublestar.Match(pat, base); ok {
			return true
		}
		// Match against the full repo-relative path (handles ** and dir-prefixed patterns).
		if ok, _ := doublestar.Match(pat, path); ok {
			return true
		}
	}
	return false
}
