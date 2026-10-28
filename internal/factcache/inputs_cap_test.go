package factcache

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCapSkipNote pins the disclosure that a run prints when the entry cap is
// what stopped input enumeration: the cache never engages, and nothing else
// reports it.
func TestCapSkipNote(t *testing.T) {
	tests := []struct {
		name    string
		entries int
		cap     int
		want    string
	}{
		{name: "within the cap", entries: 10, cap: 100, want: ""},
		{name: "exactly at the cap", entries: 100, cap: 100, want: ""},
		{name: "one entry over", entries: 101, cap: 100, want: "fact cache skipped: input tree has more than 100 filesystem entries"},
		{name: "default cap", entries: maxInputEntries + 1, cap: maxInputEntries,
			want: "fact cache skipped: input tree has more than 20000 filesystem entries"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := capSkipNote(tc.entries, tc.cap); got != tc.want {
				t.Errorf("capSkipNote(%d, %d) = %q, want %q", tc.entries, tc.cap, got, tc.want)
			}
		})
	}
}

// TestListInputsDisclosesTheEntryCap drives the real walk over a handful of
// files with a tiny cap, so the disclosure is proven on the path a 20,000-entry
// tree takes without building one.
func TestListInputsDisclosesTheEntryCap(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package x\n"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	t.Run("over the cap", func(t *testing.T) {
		files, skip := listInputs(root, MatchAll, nil, 2)
		if !slices.Equal(files, []string{"."}) {
			t.Errorf("files = %v, want the directory sentinel", files)
		}
		if !strings.Contains(skip, "more than 2 filesystem entries") {
			t.Errorf("skip = %q, want the entry-cap disclosure", skip)
		}
	})

	t.Run("at the cap", func(t *testing.T) {
		files, skip := listInputs(root, MatchAll, nil, 3)
		if skip != "" {
			t.Errorf("skip = %q, want no disclosure for a tree within the cap", skip)
		}
		if len(files) != 3 {
			t.Errorf("files = %v, want the three fixture files", files)
		}
	})

	t.Run("other failures stay silent", func(t *testing.T) {
		// An unusable root is not a size problem: no cap disclosure.
		files, skip := listInputs(filepath.Join(root, "missing"), MatchAll, nil, maxInputEntries)
		if !slices.Equal(files, []string{"."}) || skip != "" {
			t.Errorf("files = %v, skip = %q; want the sentinel with no disclosure", files, skip)
		}
	})
}
