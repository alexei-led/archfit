package toolrun_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestContainsPathUsesLexicalAndFilesystemAncestry(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "target")
	outside := filepath.Join(base, "owned")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "program"), []byte("program"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	insideLink := filepath.Join(root, "external-link")
	outsideLink := filepath.Join(outside, "target-link")
	if err := os.Symlink(filepath.Join(outside, "program"), insideLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "program"), outsideLink); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want bool
	}{
		{root, true},
		{filepath.Join(root, "program"), true},
		{filepath.Join(root, "not-created", "program"), true},
		{filepath.Join(outside, "program"), false},
		{insideLink, true},
		{outsideLink, true},
	} {
		inside, err := toolrun.ContainsPath(root, tc.path)
		if err != nil || inside != tc.want {
			t.Errorf("ContainsPath(%q) = %v, %v; want %v", tc.path, inside, err, tc.want)
		}
	}
	alias := strings.Replace(filepath.Join(root, "program"), string(filepath.Separator)+"target"+string(filepath.Separator), string(filepath.Separator)+"TARGET"+string(filepath.Separator), 1)
	if _, err := os.Stat(alias); err == nil {
		inside, err := toolrun.ContainsPath(root, alias)
		if err != nil || !inside {
			t.Fatalf("case alias escaped filesystem boundary: inside=%v error=%v", inside, err)
		}
	}
}
