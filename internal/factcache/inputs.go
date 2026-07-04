package factcache

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// hashSkipDirs are directory names never part of any extractor's input scope:
// dependency trees, build output, and interpreter caches. Pruning them by name
// keeps the input-tree walk fast on large repos; correctness still comes from
// the caller's exclusion globs, which are matched per file below. Dot-dirs
// (.git, .archfit-cache, .venv, …) are pruned unconditionally.
var hashSkipDirs = map[string]struct{}{
	"node_modules": {},
	"target":       {},
	"__pycache__":  {},
	"venv":         {},
}

// ListInputs walks root and returns the sorted slash-relative paths of the
// regular files that form one analyzer's input scope: match(rel) selects by
// path (extension, basename, …) and exclude globs (doublestar, matched against
// the slash-relative path) drop the rest. Feed the result to HashTree for the
// key's input-tree component. The walk is best-effort: unreadable entries are
// skipped, never an error — a file that vanishes between walk and hash is
// HashTree's error to report.
func ListInputs(root string, match func(rel string) bool, exclude []string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // best-effort: skip unreadable entries
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, skip := hashSkipDirs[name]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		isDirLink := false
		if !d.Type().IsRegular() {
			if d.Type()&fs.ModeSymlink == 0 {
				return nil // sockets, devices, … — never tool inputs
			}
			info, serr := os.Stat(path) // follow the link
			if serr != nil {
				return nil // dangling: no content the tool could read either
			}
			// A symlink to a regular file is an input like any other — HashTree's
			// ReadFile follows it, so target edits invalidate. A symlink to a
			// directory cannot be descended by WalkDir while the real tool follows
			// it: include the path so HashTree errors and the caller vetoes
			// caching — a miss, never a stale hit (over-hash, never under-hash).
			isDirLink = info.IsDir()
			if !isDirLink && !info.Mode().IsRegular() {
				return nil
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if matchesAny(rel, exclude) || (!isDirLink && !match(rel)) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out
}

// MatchExts returns a match func for ListInputs that selects files by
// extension (e.g. ".go") or exact basename (e.g. "go.mod", "tsconfig.json").
func MatchExts(exts []string, basenames []string) func(rel string) bool {
	extSet := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		extSet[e] = struct{}{}
	}
	baseSet := make(map[string]struct{}, len(basenames))
	for _, b := range basenames {
		baseSet[b] = struct{}{}
	}
	return func(rel string) bool {
		if _, ok := extSet[filepath.Ext(rel)]; ok {
			return true
		}
		_, ok := baseSet[filepath.Base(rel)]
		return ok
	}
}

// MatchAll selects every regular file — for analyzers whose input scope is the
// whole tree (jscpd, ast-grep).
func MatchAll(string) bool { return true }

func matchesAny(rel string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, rel); ok {
			return true
		}
	}
	return false
}
