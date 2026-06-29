package scope

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const osWindows = "windows"

// TestCanonicalPath_Symlink verifies that canonicalPath resolves a symlinked
// directory to its real path (Linux/macOS), documenting the macOS case-sensitivity
// scenario where the OS mounts a case-insensitive FS with canonical casing in
// EvalSymlinks output.
func TestCanonicalPath_Symlink(t *testing.T) {
	if runtime.GOOS == osWindows {
		t.Skip("symlink test skipped on Windows")
	}

	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	got := canonicalPath(linkDir)
	// EvalSymlinks must resolve the symlink to the real directory.
	want, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatalf("EvalSymlinks on realDir: %v", err)
	}
	if got != want {
		t.Errorf("canonicalPath(%q) = %q; want %q (real path)", linkDir, got, want)
	}
}

// TestCanonicalPath_Empty verifies the empty-string guard: canonicalPath("") == ""
// so that the GitRoot=="" invariant in non-git full mode is never corrupted.
func TestCanonicalPath_Empty(t *testing.T) {
	if got := canonicalPath(""); got != "" {
		t.Errorf("canonicalPath(%q) = %q; want %q", "", got, "")
	}
}

// TestSubtreePrefix_SymlinkScanRoot verifies that when the scan root is supplied
// as a symlink pointing into the git root, subtreePrefix still resolves the
// correct non-empty prefix rather than returning "".
//
// This is the core macOS scenario: the user passes --root /var/folders/x/…
// while git rev-parse --show-toplevel returns /private/var/folders/x/… (the
// canonical form after symlink resolution). Without canonicalPath both sides of
// filepath.Rel are different strings even though they describe the same directory,
// so Rel returns a ".." path and subtreePrefix falls back to "".
func TestSubtreePrefix_SymlinkScanRoot(t *testing.T) {
	if runtime.GOOS == osWindows {
		t.Skip("symlink test skipped on Windows")
	}

	// Create a real directory tree: gitRoot/services/api
	gitRoot := t.TempDir()
	serviceDir := filepath.Join(gitRoot, "services", "api")
	if err := os.MkdirAll(serviceDir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Symlink pointing to gitRoot (simulates /var → /private/var on macOS).
	symlinkGitRoot := filepath.Join(t.TempDir(), "gitlink")
	if err := os.Symlink(gitRoot, symlinkGitRoot); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	// The Resolver returns the canonical gitRoot; cfg.Root uses the symlinked variant.
	symlinkScanRoot := filepath.Join(symlinkGitRoot, "services", "api")

	// Both sides go through canonicalPath in Resolve/resolveScanRoot.
	canonGitRoot := canonicalPath(gitRoot)          // resolves to realDir
	canonScanRoot := canonicalPath(symlinkScanRoot) // resolves through symlink

	prefix := subtreePrefix(canonGitRoot, canonScanRoot)

	// filepath.ToSlash so the test passes on any OS.
	got := filepath.ToSlash(prefix)
	const want = "services/api"
	if got != want {
		t.Errorf("subtreePrefix = %q; want %q (symlink scan root must resolve to real path)", got, want)
	}
}

// TestSnapScanRoot verifies that snapScanRoot snaps to gitRoot when scanRoot
// names the same directory via a different path (symlink here; on macOS the
// same logic fires for case-variant APFS paths like /workspace vs /Workspace).
func TestSnapScanRoot_SameDir(t *testing.T) {
	if runtime.GOOS == osWindows {
		t.Skip("symlink test skipped on Windows")
	}

	gitRoot := t.TempDir()
	// linkRoot points to the same directory as gitRoot via a symlink — same inode.
	linkRoot := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(gitRoot, linkRoot); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	got := snapScanRoot(gitRoot, linkRoot)
	if got != gitRoot {
		t.Errorf("snapScanRoot(%q, %q) = %q; want %q (same-inode path should snap to gitRoot)", gitRoot, linkRoot, got, gitRoot)
	}
}

func TestSnapScanRoot_NoOp(t *testing.T) {
	dir := t.TempDir()
	// Same string → short-circuits without Stat.
	if got := snapScanRoot(dir, dir); got != dir {
		t.Errorf("same-string: got %q; want %q", got, dir)
	}
	// Empty gitRoot → always returns scanRoot.
	if got := snapScanRoot("", dir); got != dir {
		t.Errorf("empty gitRoot: got %q; want %q", got, dir)
	}
}
