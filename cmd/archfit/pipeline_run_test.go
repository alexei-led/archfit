package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOnDiskWithin_RejectsNonLocalPaths pins the files[] containment contract
// at the I/O boundary: the resolver's slash-only guard cannot see OS-specific
// separators, so the onDisk closure itself must reject candidates that are
// absolute or escape the scan root after OS-path conversion — even when the
// escaped target really exists on disk.
func TestOnDiskWithin_RejectsNonLocalPaths(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "real.go"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	onDisk := onDiskWithin(root)
	cases := []struct {
		rel  string
		want bool
	}{
		{"pkg/real.go", true},
		{"pkg", true},
		{"../outside.txt", false},        // exists, but escapes the root
		{"sub/../../outside.txt", false}, // cleans to an escape
		{"/etc", false},                  // absolute
		{"", false},
		{"pkg/missing.go", false},
	}
	for _, tc := range cases {
		if got := onDisk(tc.rel); got != tc.want {
			t.Errorf("onDisk(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}
