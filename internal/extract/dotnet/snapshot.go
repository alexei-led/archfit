package dotnet

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// snapshotSkip names the entries never copied into a snapshot, at any depth:
// VCS metadata (a directory, or the file a worktree or submodule uses) and npm
// package folders. Neither is a C# build input. Everything else is kept,
// including obj/ (supplied project.assets.json, NuGet imports), bin/ (HintPath
// references), and files matched by exclude: removing any of them could change
// the program the helper compiles (D6).
var snapshotSkip = map[string]struct{}{".git": {}, "node_modules": {}}

// makeSnapshot copies root into a new private temp directory and returns it.
// The Roslyn design-time build writes under each project's obj/, so the helper
// runs on the copy and the original is never written. Directories and files are
// made owner-writable so the build can write and the copy can be removed. On
// error nothing is left behind. ctx cancellation stops the copy.
func makeSnapshot(ctx context.Context, root string) (string, error) {
	dir, err := os.MkdirTemp("", "archfit-dotnet-snapshot-*")
	if err != nil {
		return "", fmt.Errorf("create snapshot: %w", err)
	}
	if err := copyTree(ctx, root, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("snapshot %s: %w", root, err)
	}
	return dir, nil
}

func copyTree(ctx context.Context, root, dst string) error {
	return filepath.WalkDir(root, func(src string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, src)
		if err != nil || rel == "." {
			return err
		}
		if _, skip := snapshotSkip[d.Name()]; skip {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.Mkdir(target, 0o700)
		case d.Type()&fs.ModeSymlink != 0:
			return copySymlink(root, dst, src, target)
		case d.Type().IsRegular():
			return copyFile(src, target)
		default:
			return nil // sockets, pipes, devices: not build inputs
		}
	})
}

// copySymlink recreates a link so it names the same file it names in the
// original, with one change: a target inside the root is redirected into the
// snapshot, so a write through the link cannot reach the original tree.
func copySymlink(root, dst, src, target string) error {
	link, err := os.Readlink(src)
	if err != nil {
		return err
	}
	abs := link
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(src), link)
	}
	if rel, inside := relToRoot(root, filepath.Clean(abs)); inside {
		link = filepath.Join(dst, filepath.FromSlash(rel))
	} else {
		link = abs
	}
	return os.Symlink(link, target)
}

func copyFile(src, target string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // walking the scan root by design
	if err != nil {
		return err
	}
	defer in.Close()                                                                             //nolint:errcheck // read-only
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0o600) //nolint:gosec // target is inside the private snapshot
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// snapshotMapper rewrites snapshot paths in helper text (reasons, stderr) back
// to root-relative paths, for every spelling of the snapshot directory.
type snapshotMapper []string

// newSnapshotMapper orders spellings longest first: on macOS the resolved
// /private/var/... spelling contains the /var/... one.
func newSnapshotMapper(spellings ...string) snapshotMapper {
	var m snapshotMapper
	for _, s := range spellings {
		if s != "" {
			m = append(m, filepath.ToSlash(s))
		}
	}
	slices.SortFunc(m, func(a, b string) int { return len(b) - len(a) })
	return m
}

// text replaces "<snapshot>/x" with "x" and a bare "<snapshot>" with ".".
func (m snapshotMapper) text(s string) string {
	for _, prefix := range m {
		s = strings.ReplaceAll(s, prefix+"/", "")
		s = strings.ReplaceAll(s, prefix, ".")
	}
	return s
}

// path maps one helper path: an absolute path under the snapshot becomes
// root-relative; a relative path is already root-relative.
func (m snapshotMapper) path(p string) string {
	p = filepath.ToSlash(p)
	for _, prefix := range m {
		if rel, ok := strings.CutPrefix(p, prefix+"/"); ok {
			return rel
		}
	}
	return p
}
