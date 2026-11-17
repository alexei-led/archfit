package acquisition

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

var errSnapshotSymlink = errors.New("symlink or enumeration limit prevents complete snapshot")

// reportCheckSymlinks stops before the cache enumerator could traverse an
// escaping link. The conservative scope marks all symlinks unknown.
func reportCheckSymlinks(root string) error {
	entries := 0
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errSnapshotSymlink
		}
		if path == root {
			return nil
		}
		entries++
		if entries > 20000 {
			return errSnapshotSymlink
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return errSnapshotSymlink
		}
		if len(strings.Split(filepath.ToSlash(rel), "/")) > 65 {
			return errSnapshotSymlink
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".archfit-cache" || reportSecretPath(rel)) {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return errSnapshotSymlink
		}
		return nil
	})
}
