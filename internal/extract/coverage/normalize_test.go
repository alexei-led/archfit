package coverage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizer_PathContract(t *testing.T) {
	root := t.TempDir()
	writeNormalizeFile(t, root, "go.mod", "module example.com/acme/repo\n")
	writeNormalizeFile(t, root, "pkg/a.go", "package pkg\n")
	writeNormalizeFile(t, root, "web/app.ts", "export const app = 1\n")

	normalizer, err := NewNormalizer(root)
	if err != nil {
		t.Fatal(err)
	}
	absoluteGo := filepath.Join(root, "pkg", "a.go")
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "absolute POSIX/local Go package", raw: filepath.ToSlash(absoluteGo), want: "pkg/a.go"},
		{name: "Go import path prefix", raw: "example.com/acme/repo/pkg/a.go", want: "pkg/a.go"},
		{name: "relative", raw: "./web/app.ts", want: "web/app.ts"},
		{name: "Windows separators in LCOV", raw: `web\app.ts`, want: "web/app.ts"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizer.Normalize(tc.raw)
			if err != nil {
				t.Fatalf("Normalize(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizer_GoModuleAboveSubtree(t *testing.T) {
	repositoryRoot := t.TempDir()
	writeNormalizeFile(t, repositoryRoot, "go.mod", "module example.com/mod\n")
	writeNormalizeFile(t, repositoryRoot, "services/api/api.go", "package api\n")
	scanRoot := filepath.Join(repositoryRoot, "services", "api")

	normalizer, err := NewNormalizer(scanRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"example.com/mod/services/api/api.go",
		"services/api/api.go",
		"api.go",
	} {
		got, err := normalizer.Normalize(raw)
		if err != nil {
			t.Errorf("Normalize(%q): %v", raw, err)
			continue
		}
		if got != "api.go" {
			t.Errorf("Normalize(%q) = %q, want api.go", raw, got)
		}
	}
}

func TestNormalizer_ContainmentAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeNormalizeFile(t, root, "inside.go", "package inside\n")
	writeNormalizeFile(t, outside, "outside.go", "package outside\n")

	linkRoot := filepath.Join(t.TempDir(), "scan-link")
	if err := os.Symlink(root, linkRoot); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	normalizer, err := NewNormalizer(linkRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := normalizer.Normalize(filepath.Join(linkRoot, "inside.go")); err != nil || got != "inside.go" {
		t.Fatalf("symlinked root normalize = %q, %v", got, err)
	}

	for _, raw := range []string{filepath.Join(outside, "outside.go"), "../outside.go", "missing.go"} {
		if got, err := normalizer.Normalize(raw); err == nil {
			t.Errorf("Normalize(%q) = %q, want ErrUnresolvedPath", raw, got)
		}
	}

	escapingLink := filepath.Join(root, "escaping.go")
	if err := os.Symlink(filepath.Join(outside, "outside.go"), escapingLink); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if got, err := normalizer.Normalize("escaping.go"); err == nil {
		t.Fatalf("escaping symlink normalized to %q", got)
	}
}

func writeNormalizeFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
