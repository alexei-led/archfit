package toolrun

import (
	"fmt"
	"os"
	"path/filepath"
)

// ContainsPath checks both a path's lexical ancestry and resolved filesystem
// ancestry. Directory identity also handles case aliases on case-insensitive
// filesystems without treating distinct Linux paths as equivalent.
func ContainsPath(root, path string) (bool, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false, err
	}
	if !rootInfo.IsDir() {
		return false, fmt.Errorf("target root is not a directory: %s", root)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	paths := []string{absolute}
	// Resolve the raw path before Clean/Abs can erase a symlink followed by .. .
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return false, err
		}
		paths = append(paths, resolved)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	for _, candidate := range paths {
		for {
			info, err := os.Stat(candidate)
			if err == nil && os.SameFile(rootInfo, info) {
				return true, nil
			}
			if err != nil && !os.IsNotExist(err) {
				return false, err
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break
			}
			candidate = parent
		}
	}
	return false, nil
}
