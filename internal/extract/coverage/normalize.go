// Package coverage ingests coverage artifacts supplied by the caller. It never
// executes a target repository's tests or starts a subprocess.
package coverage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

// ErrUnresolvedPath means an artifact path could not be reduced to a regular
// file contained by the scan root. Callers count this error; they never discard
// the path silently.
var ErrUnresolvedPath = errors.New("unresolved coverage path")

type modulePrefix struct {
	path string
	dir  string
}

// Normalizer reduces parser-emitted paths to scan-root-relative slash paths.
// It resolves the scan root and candidate through symlinks before containment
// checking, so neither a case-preserving symlinked root nor an escaping file
// symlink can produce a false attribution.
type Normalizer struct {
	root           string
	modulePrefixes []modulePrefix
}

// NewNormalizer prepares a normalizer for one resolved scan root. Go module
// prefixes are discovered from go.mod files so a profile may mix local absolute
// paths and import-path-prefixed paths (golang/go#40251).
func NewNormalizer(scanRoot string) (*Normalizer, error) {
	abs, err := filepath.Abs(scanRoot)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Normalizer{root: root, modulePrefixes: discoverModulePrefixes(root)}, nil
}

// Normalize returns a scan-root-relative slash path. The target must exist as a
// regular file; outside, missing, directory, and escaping-symlink paths all
// return ErrUnresolvedPath.
func (n *Normalizer) Normalize(raw string) (string, error) {
	if n == nil || n.root == "" || raw == "" || strings.IndexByte(raw, 0) >= 0 {
		return "", ErrUnresolvedPath
	}

	// Coverage files use slash syntax even on another operating system. Accept
	// Windows separators before filepath performs host containment checks.
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/")
	if cleaned == "" {
		return "", ErrUnresolvedPath
	}

	if filepath.IsAbs(filepath.FromSlash(cleaned)) {
		return n.normalizeCandidate(filepath.FromSlash(cleaned))
	}
	// A drive-qualified path cannot be interpreted safely on a non-Windows host.
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		return "", ErrUnresolvedPath
	}

	cleaned = strings.TrimPrefix(cleaned, "./")
	if rel, err := n.normalizeCandidate(filepath.Join(n.root, filepath.FromSlash(cleaned))); err == nil {
		return rel, nil
	}

	for _, module := range n.modulePrefixes {
		if cleaned != module.path && !strings.HasPrefix(cleaned, module.path+"/") {
			continue
		}
		remainder := strings.TrimPrefix(cleaned, module.path)
		remainder = strings.TrimPrefix(remainder, "/")
		candidate := filepath.Join(n.root, filepath.FromSlash(module.dir), filepath.FromSlash(remainder))
		if rel, err := n.normalizeCandidate(candidate); err == nil {
			return rel, nil
		}
	}
	return "", ErrUnresolvedPath
}

func (n *Normalizer) normalizeCandidate(candidate string) (string, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Clean(candidate))
	if err != nil {
		return "", ErrUnresolvedPath
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrUnresolvedPath
	}
	rel, err := filepath.Rel(n.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrUnresolvedPath
	}
	return filepath.ToSlash(filepath.Clean(rel)), nil
}

func discoverModulePrefixes(root string) []modulePrefix {
	var out []modulePrefix
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // best effort: an unreadable module cannot help normalization
		}
		if entry.IsDir() {
			if path != root && skipNormalizationDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // path came from the contained root walk
		if err != nil {
			return nil
		}
		modulePath := modfile.ModulePath(data)
		if modulePath == "" {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return nil
		}
		if dir == "." {
			dir = ""
		}
		out = append(out, modulePrefix{path: filepath.ToSlash(modulePath), dir: filepath.ToSlash(dir)})
		return nil
	})
	// Longest prefix first: nested modules must win over their parent module.
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].path) != len(out[j].path) {
			return len(out[i].path) > len(out[j].path)
		}
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		return out[i].dir < out[j].dir
	})
	return out
}

func skipNormalizationDir(name string) bool {
	switch name {
	case ".archfit-cache", ".git", ".venv", "node_modules", "target", "venv", "__pycache__":
		return true
	default:
		return false
	}
}
