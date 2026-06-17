package initcfg

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ClassifyTarget bundles the information sent to the LLM classifier for one
// module. Paths mirrors ModuleDef.Paths verbatim. Files holds up to 20
// first-level file basenames resolved from filesystem-glob paths (Go/TS style,
// e.g. "internal/x/**") or from a Python dotted path converted to a directory
// (e.g. "ccgram.handlers" → <root>/ccgram/handlers). Files is empty when the
// directory does not exist or when paths are purely dotted with no corresponding
// directory.
type ClassifyTarget struct {
	Name  string
	Paths []string
	Files []string
}

// maxClassifyFiles caps the number of basenames collected per target.
const maxClassifyFiles = 20

// BuildClassifyTargets produces one ClassifyTarget per ModuleDef, in the same
// order as mods. For each module it attempts to collect up to 20 first-level
// regular-file basenames:
//
//   - Filesystem-glob paths (contain "/" — Go "internal/x/**" or TS "src/x/**"):
//     strip a trailing "/**" to get the directory, join under root, read first-level
//     regular files, skip dot- and underscore-prefixed names.
//   - Python dotted paths (contain ".", no "/"): convert dots to slashes and look
//     for the directory under root. If absent, Files is left empty (explicit degrade).
//
// Files are de-duplicated, sorted, and capped at 20. Output is deterministic:
// target order matches mods; Files within each target are sorted.
func BuildClassifyTargets(root string, mods []ModuleDef) []ClassifyTarget {
	targets := make([]ClassifyTarget, 0, len(mods))
	for _, m := range mods {
		ct := ClassifyTarget{
			Name:  m.Name,
			Paths: m.Paths,
		}
		ct.Files = resolveFiles(root, m.Paths)
		targets = append(targets, ct)
	}
	return targets
}

// resolveFiles collects up to maxClassifyFiles first-level regular-file
// basenames from the directories implied by paths. Results are de-duplicated
// and sorted.
func resolveFiles(root string, paths []string) []string {
	seen := make(map[string]bool)
	for _, p := range paths {
		dir, ok := resolveDir(root, p)
		if !ok {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				continue
			}
			seen[name] = true
			if len(seen) >= maxClassifyFiles {
				break
			}
		}
		if len(seen) >= maxClassifyFiles {
			break
		}
	}

	if len(seen) == 0 {
		return nil
	}

	files := make([]string, 0, len(seen))
	for name := range seen {
		files = append(files, name)
	}
	sort.Strings(files)
	return files
}

// resolveDir derives the filesystem directory from a single path element.
// Returns (dir, true) when a strategy applies, (_, false) to skip.
//
// Strategy:
//  1. Filesystem-glob path (contains "/"): strip trailing "/**", join under root.
//  2. Python dotted path (contains ".", no "/") or bare single-segment package
//     name (no "/" and no "."): convert dots to slashes and try the candidate
//     directory under both <root> and <root>/src, returning the first that
//     exists and is a directory. Degrades to (_, false) when neither exists.
func resolveDir(root, path string) (string, bool) {
	if strings.Contains(path, "/") {
		// Filesystem glob: strip trailing "/**" if present.
		dir := strings.TrimSuffix(path, "/**")
		return filepath.Join(root, filepath.FromSlash(dir)), true
	}

	// Dotted Python path or bare single-segment package name.
	// Convert dots to slashes to get a candidate subdirectory.
	candidate := strings.ReplaceAll(path, ".", string(filepath.Separator))
	for _, base := range []string{root, filepath.Join(root, pySrcDir)} {
		dir := filepath.Join(base, candidate)
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			return dir, true
		}
	}
	return "", false
}
