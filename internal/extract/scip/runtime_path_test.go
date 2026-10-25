package scip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeRejectsSymlinkParentSegments(t *testing.T) {
	base := t.TempDir()
	root, owned := filepath.Join(base, "target"), filepath.Join(base, "owned")
	for _, dir := range []string{filepath.Join(root, "child"), filepath.Join(root, "bin"), owned} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	program := filepath.Join(root, "bin", "python")
	if err := os.WriteFile(program, []byte("target interpreter"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "child"), filepath.Join(owned, "link")); err != nil {
		t.Fatal(err)
	}
	raw := owned + string(filepath.Separator) + "link" + string(filepath.Separator) + ".." + string(filepath.Separator) + "bin" + string(filepath.Separator) + "python"
	err := validateReaderPython(root, raw)
	if err == nil || !strings.Contains(err.Error(), "must not select a target interpreter") {
		t.Fatalf("raw target path not rejected at boundary: %v", err)
	}
}
