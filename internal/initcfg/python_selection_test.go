package initcfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPythonPackageSelectionKeepsAllDiscoveredRoots(t *testing.T) {
	t.Parallel()
	const primaryPackage = "primary_pkg"
	for _, tc := range []struct {
		name, want string
		packages   []string
	}{
		{"single flat", primaryPackage, []string{primaryPackage}},
		{"multiple flat", "", []string{primaryPackage, "other"}},
		{"single src", primaryPackage, []string{"src/" + primaryPackage, "stray_pkg"}},
		{"multiple src", "", []string{"src/" + primaryPackage, "src/other", "stray_pkg"}},
		{"private sibling", "", []string{"src/_private", "src/" + primaryPackage}},
		{"single private package", "_private", []string{"src/_private"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, pkg := range tc.packages {
				dir := filepath.Join(root, pkg)
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "__init__.py"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := detectPyPackage(root); got != tc.want {
				t.Fatalf("package=%q want=%q", got, tc.want)
			}
		})
	}
}
