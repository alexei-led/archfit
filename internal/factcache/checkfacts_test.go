package factcache_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/factcache"
)

func TestTreeFactsKeyChangesWithEveryInput(t *testing.T) {
	t.Parallel()
	key := factcache.TreeFactsKey("v1", "settings", "tree-a")
	if again := factcache.TreeFactsKey("v1", "settings", "tree-a"); again != key {
		t.Fatalf("equal inputs gave %q and %q", key, again)
	}
	for name, other := range map[string]string{
		"another tree":         factcache.TreeFactsKey("v1", "settings", "tree-b"),
		"another tool version": factcache.TreeFactsKey("v2", "settings", "tree-a"),
		"other settings":       factcache.TreeFactsKey("v1", "changed", "tree-a"),
	} {
		if other == key {
			t.Errorf("%s gave the same key", name)
		}
	}
}

func TestContentFactsKeyFollowsTheFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "a.go", "package a\n")
	write(t, root, "b.go", "package b\n")
	key := contentKey(t, root, "a.go", "b.go")

	if got := contentKey(t, root, "b.go", "a.go"); got != key {
		t.Errorf("another path order gave another key")
	}
	write(t, root, "c.go", "package c\n")
	if got := contentKey(t, root, "a.go", "b.go", "c.go"); got == key {
		t.Errorf("a new file gave the same key")
	}
	write(t, root, "a.go", "package a // edited\n")
	if got := contentKey(t, root, "a.go", "b.go"); got == key {
		t.Errorf("an edited file gave the same key")
	}
	if _, err := factcache.ContentFactsKey("v1", "settings", root, []string{"missing.go"}); err == nil {
		t.Error("a missing file gave a key, want an error so the caller runs uncached")
	}
}

// TestTreeAndContentKeysNeverMeet: a base keyed by a tree id and a working
// tree keyed by a content hash never share an entry, even if the two hashes
// were equal.
func TestTreeAndContentKeysNeverMeet(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "a.go", "package a\n")
	hash, err := factcache.HashTree(root, []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if factcache.TreeFactsKey("v1", "settings", hash) == contentKey(t, root, "a.go") {
		t.Error("a tree key equals a content key for the same hash")
	}
}

func TestCheckFactsCacheHitAndMiss(t *testing.T) {
	t.Parallel()
	store := factcache.NewStore(t.TempDir())
	root := t.TempDir()
	write(t, root, "a.go", "package a\n")

	base := factcache.TreeFactsKey("v1", "settings", "tree-a")
	head := contentKey(t, root, "a.go")
	store.Put(factcache.CheckFactsAnalyzer, base, []byte(`{"base":true}`))
	store.Put(factcache.CheckFactsAnalyzer, head, []byte(`{"head":true}`))

	for name, tc := range map[string]struct {
		key  string
		want string
	}{
		"same base tree":       {base, `{"base":true}`},
		"same working tree":    {head, `{"head":true}`},
		"another base tree":    {factcache.TreeFactsKey("v1", "settings", "tree-b"), ""},
		"edited working tree":  {editedKey(t), ""},
		"another tool version": {factcache.TreeFactsKey("v2", "settings", "tree-a"), ""},
	} {
		got, hit := store.Get(factcache.CheckFactsAnalyzer, tc.key)
		if hit != (tc.want != "") || string(got) != tc.want {
			t.Errorf("%s: Get = %q, hit %v; want %q", name, got, hit, tc.want)
		}
	}
}

func editedKey(t *testing.T) string {
	t.Helper()
	edited := t.TempDir()
	write(t, edited, "a.go", "package a // edited\n")
	return contentKey(t, edited, "a.go")
}

func contentKey(t *testing.T, root string, files ...string) string {
	t.Helper()
	key, err := factcache.ContentFactsKey("v1", "settings", root, files)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
