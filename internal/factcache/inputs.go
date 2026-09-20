package factcache

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// hashSkipDirs contains only tool metadata shared by every extractor. Source
// exclusions belong to each caller: names such as target or venv (even with
// cache/environment markers) do not prove that its analyzer skips the directory.
var hashSkipDirs = map[string]struct{}{
	".archfit-cache": {},
	".git":           {},
}

// ListInputs enumerates analyzer inputs, following directory symlinks while
// retaining their logical paths. Caller exclusions prune complete subtrees.
// Unreadable inputs, cycles, or excessive enumeration return a directory sentinel
// so HashTree vetoes caching instead of serving incomplete or expensive keys.
func ListInputs(root string, match func(rel string) bool, exclude []string) []string {
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return []string{"."}
	}
	w := inputWalker{match: match, exclude: exclude, active: make(map[string]bool)}
	if err := w.walk(physical, "", 0); err != nil {
		return []string{"."}
	}
	slices.Sort(w.files)
	return w.files
}

const maxInputEntries = 20_000

type inputWalker struct {
	match   func(string) bool
	exclude []string
	active  map[string]bool
	files   []string
	entries int
}

func (w *inputWalker) walk(dir, rel string, depth int) error {
	if depth > 64 || w.active[dir] {
		return fs.ErrInvalid
	}
	w.active[dir] = true
	defer delete(w.active, dir)
	f, err := os.Open(dir) //nolint:gosec // directory resolved from analyzer input scope
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	for {
		entries, err := f.ReadDir(128)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries {
			w.entries++
			if w.entries > maxInputEntries {
				return fs.ErrInvalid
			}
			if err := w.entry(filepath.Join(dir, entry.Name()), filepath.ToSlash(filepath.Join(rel, entry.Name())), entry, depth); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func (w *inputWalker) entry(path, rel string, entry fs.DirEntry, depth int) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		info, err = os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
		}
	}
	if info.IsDir() {
		if _, skip := hashSkipDirs[entry.Name()]; skip || excludesDirectory(rel, w.exclude) {
			return nil
		}
		return w.walk(path, rel, depth+1)
	}
	if info.Mode().IsRegular() && !matchesAny(rel, w.exclude) && w.match(rel) {
		w.files = append(w.files, rel)
	}
	return nil
}

func excludesDirectory(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.HasSuffix(pattern, "/**") {
			if ok, _ := doublestar.Match(pattern, rel+"/"); ok {
				return true
			}
		}
	}
	return false
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

// MatchAll selects every regular file within the caller's traversal scope.
func MatchAll(string) bool { return true }

func matchesAny(rel string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, rel); ok {
			return true
		}
	}
	return false
}
