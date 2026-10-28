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

// buildInputNames are the files MSBuild, the SDK resolver, and NuGet read from
// a project's ancestor directories, lowercased: NuGet accepts any case.
var buildInputNames = map[string]struct{}{
	"directory.build.props": {}, "directory.build.targets": {}, "directory.build.rsp": {},
	"directory.packages.props": {}, "global.json": {}, "nuget.config": {},
}

// snapshot is a per-run copy of the scan root.
type snapshot struct {
	// dir is the temp directory; removing it removes the snapshot.
	dir string
	// root is the scan root's counterpart: dir plus the root's path below the
	// repository root, so ancestor build inputs keep their relative places.
	root string
	// aboveRepo are build inputs above the repository root. MSBuild would read
	// them in the original tree; the snapshot does not have them.
	aboveRepo []string
}

// makeSnapshot copies root into a new private temp directory, laid out as
// <dir>/<root's path below repoRoot>/, plus the build-input files of every
// ancestor between root and repoRoot (inclusive) at their relative places:
// a subtree scan still sees the Directory.Build.props, global.json, or
// NuGet.config the original tree inherits. The Roslyn design-time build writes
// under each project's obj/, so the helper runs on the copy and the original is
// never written. Directories and files are made owner-writable so the build can
// write and the copy can be removed. On error nothing is left behind. ctx
// cancellation stops the copy.
func makeSnapshot(ctx context.Context, root, repoRoot string) (snapshot, error) {
	dir, err := os.MkdirTemp("", "archfit-dotnet-snapshot-*")
	if err != nil {
		return snapshot{}, fmt.Errorf("create snapshot: %w", err)
	}
	snap, err := fillSnapshot(ctx, dir, root, repoRoot)
	if err != nil {
		_ = os.RemoveAll(dir)
		return snapshot{}, fmt.Errorf("snapshot %s: %w", root, err)
	}
	return snap, nil
}

func fillSnapshot(ctx context.Context, dir, root, repoRoot string) (snapshot, error) {
	snap := snapshot{dir: dir, root: dir}
	prefix, top := "", root
	if repoRoot != "" {
		if rel, inside := relToRoot(repoRoot, root); inside {
			prefix, top = rel, repoRoot
		}
	}
	if prefix != "" {
		snap.root = filepath.Join(dir, filepath.FromSlash(prefix))
		if err := os.MkdirAll(snap.root, 0o700); err != nil {
			return snapshot{}, err
		}
	}
	if err := copyTree(ctx, root, snap.root, dir); err != nil {
		return snapshot{}, err
	}
	levels := 0
	if prefix != "" {
		levels = strings.Count(prefix, "/") + 1
	}
	src, dst := root, snap.root
	for range levels {
		src, dst = filepath.Dir(src), filepath.Dir(dst)
		if err := copyBuildInputs(src, dst); err != nil {
			return snapshot{}, err
		}
	}
	snap.aboveRepo = buildInputsAbove(top)
	return snap, nil
}

// copyBuildInputs copies the build-input files directly in src to dst.
func copyBuildInputs(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, ok := buildInputNames[strings.ToLower(e.Name())]; ok && !e.IsDir() {
			if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildInputsAbove lists build-input files in the ancestors of top, up to the
// filesystem root. MSBuild searches that far; the snapshot stops at top.
func buildInputsAbove(top string) []string {
	var found []string
	for cur := filepath.Dir(top); ; cur = filepath.Dir(cur) {
		entries, _ := os.ReadDir(cur)
		for _, e := range entries {
			if _, ok := buildInputNames[strings.ToLower(e.Name())]; ok && !e.IsDir() {
				found = append(found, filepath.Join(cur, e.Name()))
			}
		}
		if filepath.Dir(cur) == cur {
			return found
		}
	}
}

// copyTree copies root into dst. skip is the snapshot directory: when the temp
// directory lies inside root, the copy must not copy itself.
func copyTree(ctx context.Context, root, dst, skip string) error {
	skipInfo, err := os.Stat(skip)
	if err != nil {
		return err
	}
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
		if _, skipName := snapshotSkip[d.Name()]; skipName {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if info, err := d.Info(); err == nil && os.SameFile(info, skipInfo) {
				return filepath.SkipDir
			}
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

// restoreAssets is the file restore writes into a project's restore directory;
// a project without it was never restored.
const restoreAssets = "project.assets.json"

// borrowRestoreOutputs copies into the snapshot, for every project it has no
// restore outputs for, those of the same project in depRoot: the working tree
// the snapshot's checkout stands in for. A checkout holds tracked files only and
// restore outputs are gitignored, so without them the helper resolves no
// package on the checkout side. Only a project at the same path in both trees
// borrows, and only the files directly in its restore directory (the assets
// file and the NuGet imports), never build output below it. depRoot is read,
// never written, and nothing is written outside the snapshot. The borrowed
// outputs describe the working tree's packages: changedRestoreInputs names the
// projects whose own restore inputs differ. It returns the projects that
// borrowed and those left without restore outputs.
func borrowRestoreOutputs(snapRoot, depRoot string, projects []ProjectEvaluation) (borrowed []ProjectEvaluation, unrestored []string, err error) {
	for _, p := range projects {
		if p.RestoreDir == "" {
			continue
		}
		dir := filepath.FromSlash(p.RestoreDir)
		dst := filepath.Join(snapRoot, dir)
		if fileExists(filepath.Join(dst, restoreAssets)) {
			continue
		}
		src := filepath.Join(depRoot, dir)
		if !fileExists(filepath.Join(depRoot, filepath.FromSlash(p.Path))) || !fileExists(filepath.Join(src, restoreAssets)) ||
			!withinSnapshot(snapRoot, dst) {
			unrestored = append(unrestored, p.Path)
			continue
		}
		if err := copyRestoreDir(src, dst); err != nil {
			return nil, nil, err
		}
		borrowed = append(borrowed, p)
	}
	return borrowed, unrestored, nil
}

// withinSnapshot reports whether p, its existing prefix resolved, lies in the
// snapshot: a link in the checkout must not carry a write outside it.
func withinSnapshot(snapRoot, p string) bool {
	resolved, ok := resolveExisting(p)
	if !ok {
		return false
	}
	_, inside := relToRoot(snapRoot, resolved)
	return inside
}

// copyRestoreDir copies the regular files directly in src into dst. A file the
// snapshot already has is kept.
func copyRestoreDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		target := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), target); err != nil {
			return err
		}
	}
	return nil
}

// copySymlink recreates a link so it names the same file it names in the
// original, with one change: a target inside the root is redirected into the
// snapshot, so a write through the link cannot reach the original tree. Inside
// is decided on the lexical target and on its resolved form, so an alias of an
// ancestor (macOS /var for /private/var) cannot hide the original.
func copySymlink(root, dst, src, target string) error {
	link, err := os.Readlink(src)
	if err != nil {
		return err
	}
	abs := link
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(src), link)
	}
	abs = filepath.Clean(abs)
	if rel, inside := insideRoot(root, abs); inside {
		link = filepath.Join(dst, filepath.FromSlash(rel))
	} else {
		link = abs
	}
	return os.Symlink(link, target)
}

// insideRoot reports p as root-relative when it lies in root, lexically or
// once resolved.
func insideRoot(root, p string) (string, bool) {
	if rel, ok := relToRoot(root, p); ok {
		return rel, true
	}
	if resolved, ok := resolveExisting(p); ok {
		return relToRoot(root, resolved)
	}
	return "", false
}

// resolveExisting resolves symlinks in the longest existing prefix of p and
// appends the rest: a link target need not exist yet (obj/ before a build).
func resolveExisting(p string) (string, bool) {
	rest := ""
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest), true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
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

// snapshotMapper rewrites snapshot paths in helper text (reasons, stderr) and
// paths back to the original tree, for every spelling of the snapshot.
type snapshotMapper []snapshotPrefix

// snapshotPrefix maps one snapshot spelling: to "" means root-relative (the
// scan root's counterpart); otherwise to is the original directory (an
// ancestor's counterpart maps to the repository root).
type snapshotPrefix struct{ from, to string }

// newSnapshotMapper maps the snapshot's root counterpart to root-relative
// paths and the snapshot directory to repoRoot, for each spelling, longest
// first: on macOS the resolved /private/var/... spelling contains /var/....
func newSnapshotMapper(snap snapshot, repoRoot string) snapshotMapper {
	var m snapshotMapper
	add := func(from, to string) {
		m = append(m, snapshotPrefix{from: filepath.ToSlash(from), to: filepath.ToSlash(to)})
		if resolved, err := filepath.EvalSymlinks(from); err == nil && resolved != from {
			m = append(m, snapshotPrefix{from: filepath.ToSlash(resolved), to: filepath.ToSlash(to)})
		}
	}
	add(snap.root, "")
	if snap.dir != snap.root && repoRoot != "" {
		add(snap.dir, repoRoot)
	}
	slices.SortFunc(m, func(a, b snapshotPrefix) int { return len(b.from) - len(a.from) })
	return m
}

// addPrefix returns m with one more path spelling mapped the same way, still
// longest first so a longer spelling is never rewritten by a shorter one it
// contains.
func (m snapshotMapper) addPrefix(from, to string) snapshotMapper {
	out := append(slices.Clone(m), snapshotPrefix{from: filepath.ToSlash(from), to: filepath.ToSlash(to)})
	slices.SortFunc(out, func(a, b snapshotPrefix) int { return len(b.from) - len(a.from) })
	return out
}

// text replaces "<root counterpart>/x" with "x" and a bare one with ".", and
// "<snapshot dir>/x" with "<repo root>/x".
func (m snapshotMapper) text(s string) string {
	for _, p := range m {
		if p.to == "" {
			s = strings.ReplaceAll(s, p.from+"/", "")
			s = strings.ReplaceAll(s, p.from, ".")
		} else {
			s = strings.ReplaceAll(s, p.from, p.to)
		}
	}
	return s
}

// path maps one helper path: an absolute path under the root counterpart
// becomes root-relative, one elsewhere in the snapshot names the original; a
// relative path is already root-relative.
func (m snapshotMapper) path(p string) string {
	p = filepath.ToSlash(p)
	for _, prefix := range m {
		if rel, ok := strings.CutPrefix(p, prefix.from+"/"); ok {
			if prefix.to == "" {
				return rel
			}
			return prefix.to + "/" + rel
		}
	}
	return p
}
