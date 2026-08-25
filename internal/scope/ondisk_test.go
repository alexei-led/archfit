package scope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnDiskWithinRejectsEscapesAndAcceptsLocalFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	local := filepath.Join(root, "pkg", "file.go")
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	within := OnDiskWithin(root)
	for _, test := range []struct {
		path string
		want bool
	}{
		{path: "pkg/file.go", want: true},
		{path: "../outside.go", want: false},
		{path: local, want: false},
		{path: "/etc/passwd", want: false},
	} {
		path, want := test.path, test.want
		if got := within(path); got != want {
			t.Errorf("OnDiskWithin(%q) = %t, want %t", path, got, want)
		}
	}
}
