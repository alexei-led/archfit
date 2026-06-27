package golang

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/mod/modfile"
)

// DiscoverMembers returns the absolute paths of Go module members in scope.
//
// Discovery order:
//  1. Locate go.work: check scanRoot/go.work, then walk up parent directories.
//  2. Parse use dirs; keep members under scanRoot that are not exclusion-matched.
//  3. Fallback (no go.work or 0 in-scope members): return [scanRoot] if it has go.mod.
//  4. Fallback: walk scanRoot for go.mod dirs (exclusion-filtered).
//  5. If still empty, return nil — caller should report absent.
//
// Returned paths are absolute and sorted for determinism.
// Exclusion globs are matched against each member's path relative to scanRoot
// using doublestar semantics. The scanRoot member itself (use ".") is never
// excluded — globs like **/testdata/** target sub-trees, not the root.
//
// Task 6 consumes the absolute paths directly as packages.Load Dir values.
func DiscoverMembers(scanRoot string, exclusions []string) ([]string, error) {
	// Phase 1: locate and parse go.work.
	members, goWorkFound, err := membersFromGoWork(scanRoot, exclusions)
	if err != nil {
		return nil, err
	}
	if goWorkFound && len(members) > 0 {
		return members, nil
	}

	// Phase 2: single go.mod at scanRoot (covers the common single-module case
	// and the go.work-with-0-in-scope-members fallback).
	if hasGoMod(scanRoot) {
		return []string{scanRoot}, nil
	}

	// Phase 3: walk scanRoot for any go.mod dirs (exclusion-filtered).
	members, err = walkGoMods(scanRoot, exclusions)
	if err != nil {
		return nil, err
	}
	return members, nil // may be nil → caller reports absent
}

// membersFromGoWork locates a go.work file (at scanRoot or in a parent), parses
// its use directives, and returns the subset of member dirs that are under
// scanRoot and not exclusion-matched.
//
// Returns (members, found, error). found is true when a go.work was located
// (even if 0 members passed the scope filter).
func membersFromGoWork(scanRoot string, exclusions []string) ([]string, bool, error) {
	goWorkPath, found := findGoWork(scanRoot)
	if !found {
		return nil, false, nil
	}

	data, err := os.ReadFile(goWorkPath) //nolint:gosec // path is derived from scanRoot walk, not user input
	if err != nil {
		return nil, true, fmt.Errorf("read go.work %s: %w", goWorkPath, err)
	}

	wf, err := modfile.ParseWork(goWorkPath, data, nil)
	if err != nil {
		return nil, true, fmt.Errorf("parse go.work %s: %w", goWorkPath, err)
	}

	goWorkDir := filepath.Dir(goWorkPath)
	var members []string
	for _, u := range wf.Use {
		memberAbs := filepath.Clean(filepath.Join(goWorkDir, filepath.FromSlash(u.Path)))
		// Keep only members that are under scanRoot.
		rel, err := filepath.Rel(scanRoot, memberAbs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		// Exclusion check: use the ScanRoot-relative path.
		// rel == "." means the scanRoot itself; skip the exclusion test for it
		// because patterns like **/testdata/** target sub-trees, not the root.
		if rel != "." && isMemberExcluded(rel, exclusions) {
			continue
		}
		members = append(members, memberAbs)
	}
	sort.Strings(members)
	return members, true, nil
}

// findGoWork walks up from dir looking for a go.work file.
// Returns (absolute path, true) on success, ("", false) if none found.
func findGoWork(dir string) (string, bool) {
	dir = filepath.Clean(dir)
	for {
		p := filepath.Join(dir, "go.work")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root.
			return "", false
		}
		dir = parent
	}
}

// hasGoMod reports whether dir contains a go.mod file.
func hasGoMod(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}

// walkGoMods walks scanRoot and returns the absolute paths of dirs containing a
// go.mod, filtered by exclusion globs. Excluded directories are skipped entirely
// (filepath.SkipDir) to avoid unnecessary traversal.
func walkGoMods(scanRoot string, exclusions []string) ([]string, error) {
	var members []string
	err := filepath.WalkDir(scanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries; don't abort the walk
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(scanRoot, path)
		if err != nil {
			return nil
		}
		// Skip and prune excluded dirs (except the root itself).
		if rel != "." && isMemberExcluded(rel, exclusions) {
			return filepath.SkipDir
		}
		if hasGoMod(path) {
			members = append(members, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk go.mod dirs under %s: %w", scanRoot, err)
	}
	sort.Strings(members)
	return members, nil
}

// isMemberExcluded reports whether relPath (relative to scanRoot) matches any
// of the exclusion globs using doublestar semantics.
func isMemberExcluded(relPath string, exclusions []string) bool {
	for _, pattern := range exclusions {
		if matched, _ := doublestar.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

// FilterMembers applies tools.go.modules include/exclude globs to a list of
// discovered member absolute paths, returning only the subset that passes.
//
// Globs are matched against each member's path relative to scanRoot using
// doublestar semantics — the same matcher as scope exclusions. An empty include
// list accepts all members; exclude is applied after include. The order of the
// input slice is preserved in the output.
//
// Returns nil when no members survive (caller should report absent).
// This is a pure filtering step; it does not re-walk the filesystem.
func FilterMembers(members []string, scanRoot string, include, exclude []string) []string {
	if len(include) == 0 && len(exclude) == 0 {
		return members // fast path: no filter configured
	}
	var out []string
	for _, abs := range members {
		rel, err := filepath.Rel(scanRoot, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !memberMatchesInclude(rel, include) || isMemberExcluded(rel, exclude) {
			continue
		}
		out = append(out, abs)
	}
	return out // nil when empty — caller treats len==0 as absent
}

// memberMatchesInclude reports whether relPath matches any include glob.
// Returns true when include is empty (accept-all default).
func memberMatchesInclude(relPath string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	for _, pat := range include {
		if matched, _ := doublestar.Match(pat, relPath); matched {
			return true
		}
	}
	return false
}
