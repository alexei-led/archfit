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

// Members is the outcome of Go member discovery: the module dirs to load, plus
// the one toolchain fact the dirs alone cannot carry.
type Members struct {
	// Dirs are the absolute, sorted member directories to load.
	Dirs []string
	// GoWorkOff reports that discovery LOCATED a go.work governing scanRoot that
	// names no member inside it, and deliberately fell back to scanRoot's own
	// module(s).
	//
	// The Go toolchain does not know that decision was made. It walks up from the
	// member dir, finds the same go.work, and refuses to load a package the
	// workspace does not `use` ("directory prefix . does not contain modules
	// listed in go.work"). Every Go-toolchain subprocess in the run — the
	// in-process packages.Load AND out-of-process indexers like scip-go — must
	// therefore run with GOWORK=off, or discovery's decision is silently reversed
	// and the analyzer reports absent/empty over a tree it can read perfectly.
	//
	// This is not a corner case: `go help work` recommends not committing
	// go.work, so a repo's go.work is typically gitignored. `analyze --base`
	// checks the base ref out as TRACKED FILES ONLY inside the repo, so the
	// gitignored go.work is missing from the checkout and the repo's own one
	// applies to it — which made the origin delta inert on any such Go repo,
	// archfit's own included.
	GoWorkOff bool
}

// DiscoverMembers returns the Go module members in scope.
//
// Discovery order:
//  1. Locate go.work: check scanRoot/go.work, then walk up parent directories.
//  2. Parse use dirs; keep members under scanRoot that are not exclusion-matched.
//  3. Fallback (no go.work or 0 in-scope members): return [scanRoot] if it has go.mod.
//  4. Fallback: walk scanRoot for go.mod dirs (exclusion-filtered).
//  5. If still empty, return nil Dirs — caller should report absent.
//
// Returned paths are absolute and sorted for determinism.
// Exclusion globs are matched against each member's path relative to scanRoot
// using doublestar semantics. The scanRoot member itself (use ".") is never
// excluded — globs like **/testdata/** target sub-trees, not the root.
//
// Callers consume the absolute paths directly as packages.Load Dir values, and
// must honour Members.GoWorkOff on every Go-toolchain subprocess they run.
func DiscoverMembers(scanRoot string, exclusions []string) (Members, error) {
	// Phase 1: locate and parse go.work.
	members, goWorkFound, err := membersFromGoWork(scanRoot, exclusions)
	if err != nil {
		return Members{}, err
	}
	if goWorkFound && len(members) > 0 {
		return Members{Dirs: members}, nil
	}

	// Reaching here with a go.work located means the workspace governs scanRoot
	// but claims nothing in it. Discovery ignores the workspace from here on, so
	// the toolchain must be told to as well (see Members.GoWorkOff).
	goWorkOff := goWorkFound

	// Phase 2: single go.mod at scanRoot (covers the common single-module case
	// and the go.work-with-0-in-scope-members fallback).
	if hasGoMod(scanRoot) {
		return Members{Dirs: []string{scanRoot}, GoWorkOff: goWorkOff}, nil
	}

	// Phase 3: walk scanRoot for any go.mod dirs (exclusion-filtered).
	members, err = walkGoMods(scanRoot, exclusions)
	if err != nil {
		return Members{}, err
	}
	return Members{Dirs: members, GoWorkOff: goWorkOff}, nil // Dirs may be nil → caller reports absent
}

// AnalysableMembers is the whole member-selection decision in one call:
// DiscoverMembers followed by the tools.go.modules include/exclude filter. The
// returned Members carries the filtered Dirs and the untouched GoWorkOff flag;
// an empty Dirs means the extractor reports absent.
//
// The filter is a deliberate POST-discovery step: DiscoverMembers handles scope
// exclusions (testdata, generated dirs), FilterMembers handles the user knob
// that restricts analysis to a named subset of workspace members for large
// workspaces where a full run exceeds acceptable wall-clock budgets.
//
// Scale ceiling: on a ~178-member workspace (omni), a full NeedTypesInfo load
// takes >5 minutes. Two mitigations are available: languages.go.modules narrows
// the member set; analyzers.<x>.timeout caps the per-analyzer wall-clock budget
// (the watchdog fires before the full pipeline hangs). Use them together for
// large workspaces.
//
// Exported and used by BOTH the extractor and the CLI's Go coverage probe. The
// two must reach the same verdict on "does this scan root have Go in it?": a
// probe that walked for go.mod itself ignored go.work entirely, so a workspace
// that names members the module filter then removes read as a coverage gap
// ("install the Go toolchain") instead of a deliberately empty scope — and the
// reverse shape turned "the extractor never looked" into "there is no Go here",
// which both `analyze --base` and `config compare` treat as safely comparable.
func AnalysableMembers(scanRoot string, exclusions, include, exclude []string) (Members, error) {
	m, err := DiscoverMembers(scanRoot, exclusions)
	if err != nil {
		return Members{}, err
	}
	m.Dirs = FilterMembers(m.Dirs, scanRoot, include, exclude)
	return m, nil
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
