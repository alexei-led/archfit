// Package manifest detects locally-declared deprecation and retraction markers
// in checked-in manifest files: go.mod retract directives and package.json
// top-level "deprecated" fields. No external tool or network access — pure
// os.ReadFile / filepath.Walk.
//
// This is an adapter package: os.ReadFile is permitted here.
//
// REPORT-ONLY: Scan never modifies the dependency graph, any metric verdict,
// or the gate. It surfaces evidence for humans and the off-gate LLM review.
//
// Detection ceiling (named residue — not implemented here):
//   - cargo yanked: registry-side only, not locally declarable; requires
//     crates.io API queries → routed to the LLM path (archfit analyze --llm / enrich).
//   - Live EOL / version-staleness (e.g. urfave/cli v1): irreducible LLM/human
//     residue; no static manifest marker to detect → LLM path.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// Scan walks root looking for go.mod and package.json files and returns any
// locally-declared deprecation/retraction markers. Returns nil when no markers
// are found. Unreadable or malformed files are skipped silently (best-effort).
//
// Ceiling: cargo yanked and live-version EOL require external registry queries
// and are not detected here — they are routed to the LLM review path.
func Scan(root string) []diagnostic.DeprecatedDep {
	var deps []diagnostic.DeprecatedDep
	_ = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			if skipDir(fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		switch fi.Name() {
		case "go.mod":
			deps = append(deps, scanGoMod(root, path)...)
		case "package.json":
			deps = append(deps, scanPackageJSON(root, path)...)
		}
		return nil
	})
	sort.Slice(deps, func(i, j int) bool {
		a, b := deps[i], deps[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Subject < b.Subject
	})
	return deps
}

// scanGoMod parses a go.mod file for retract directives.
//
// Supported forms:
//
//	retract v1.0.0              // single version, no comment
//	retract v1.0.0 // comment   // single version with comment
//	retract [v1.0.0, v1.0.5]    // range
//	retract (                    // block — each inner line is one entry
//	    v1.0.0 // comment
//	    [v1.0.0, v1.0.5]
//	)
func scanGoMod(root, path string) []diagnostic.DeprecatedDep {
	data, err := os.ReadFile(path) //nolint:gosec // path from Walk under repo root
	if err != nil {
		return nil
	}
	rel := relPath(root, path)
	var deps []diagnostic.DeprecatedDep
	lines := strings.Split(string(data), "\n")
	inBlock := false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			// Line inside a retract(...) block: "vX.Y.Z // comment" or "[v..., v...]"
			if d, ok := parseRetractLine(rel, trimmed); ok {
				deps = append(deps, d)
			}
			continue
		}
		// Outside a block: look for "retract" keyword.
		if !strings.HasPrefix(trimmed, "retract") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("retract"):])
		if rest == "(" {
			inBlock = true
			continue
		}
		if d, ok := parseRetractLine(rel, rest); ok {
			deps = append(deps, d)
		}
	}
	return deps
}

// parseRetractLine parses one retract version/range line (with optional comment).
// Returns (dep, true) on success.
func parseRetractLine(file, s string) (diagnostic.DeprecatedDep, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return diagnostic.DeprecatedDep{}, false
	}
	// Split off trailing "// comment".
	subject, note := splitComment(s)
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return diagnostic.DeprecatedDep{}, false
	}
	return diagnostic.DeprecatedDep{
		File:    file,
		Kind:    "retract",
		Subject: subject,
		Note:    note,
	}, true
}

// splitComment splits "text // comment" into ("text", "comment").
// Returns ("text", "") when no comment marker is present.
func splitComment(s string) (string, string) {
	idx := strings.Index(s, "//")
	if idx < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+2:])
}

// scanPackageJSON reads a package.json and returns a DeprecatedDep when the
// top-level "deprecated" field is a non-empty string.
func scanPackageJSON(root, path string) []diagnostic.DeprecatedDep {
	data, err := os.ReadFile(path) //nolint:gosec // path from Walk under repo root
	if err != nil {
		return nil
	}
	rel := relPath(root, path)

	// Decode into a raw map so we can inspect the deprecated field's type.
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil // malformed JSON — skip silently
	}
	raw, ok := pkg["deprecated"]
	if !ok {
		return nil
	}
	note, ok := raw.(string)
	if !ok || note == "" {
		return nil
	}

	// Use the package "name" field as the subject when present.
	subject := rel
	if nameRaw, ok := pkg["name"]; ok {
		if name, ok := nameRaw.(string); ok && name != "" {
			subject = name
		}
	}

	return []diagnostic.DeprecatedDep{
		{
			File:    rel,
			Kind:    "deprecated",
			Subject: subject,
			Note:    note,
		},
	}
}

// relPath returns path relative to root, or path itself on error.
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// skipDir reports whether a directory should be skipped during the walk.
func skipDir(name string) bool {
	return name != "." && (strings.HasPrefix(name, ".") ||
		name == "node_modules" || name == "vendor" || name == "__pycache__")
}
