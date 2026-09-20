package factcache

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestListInputs_OnlyCallerExclusionsRemoveSourceDirectories(t *testing.T) {
	root := t.TempDir()
	paths := []string{".venv/input.go", "__pycache__/input.go", "node_modules/input.go", "target/input.go", "venv/input.go"}
	for _, rel := range paths {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package input"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := ListInputs(root, MatchExts([]string{".go"}, nil), nil); !slices.Equal(got, paths) {
		t.Fatalf("source inputs=%v want=%v", got, paths)
	}
	excluded := []string{"**/target/**"}
	files := ListInputs(root, MatchAll, excluded)
	before, err := HashTree(root, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target", "input.go"), []byte("changed output"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := HashTree(root, ListInputs(root, MatchAll, excluded))
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("explicitly excluded analyzer output invalidated the tree hash")
	}
}
