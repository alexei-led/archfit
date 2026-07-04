package factcache

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestStore_PutGetRoundtrip(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	s.Put("ts", "key1", []byte(`{"fact":1}`))

	got, ok := s.Get("ts", "key1")
	if !ok {
		t.Fatal("expected hit after Put")
	}
	if string(got) != `{"fact":1}` {
		t.Errorf("got %q, want %q", got, `{"fact":1}`)
	}
}

func TestStore_GetMiss(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	if _, ok := s.Get("ts", "absent"); ok {
		t.Error("expected miss for absent entry")
	}
}

func TestStore_NilStoreDisablesCache(t *testing.T) {
	t.Parallel()
	var s *Store
	s.Put("ts", "key1", []byte("data")) // must not panic
	if _, ok := s.Get("ts", "key1"); ok {
		t.Error("nil store must always miss")
	}
}

func TestStore_AtomicWriteLeavesNoTempFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)
	s.Put("ts", "key1", []byte("data"))

	entries, err := os.ReadDir(filepath.Join(dir, "ts"))
	if err != nil {
		t.Fatalf("read analyzer dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("want exactly 1 entry, got %d", len(entries))
	}
}

func TestStore_EvictOldestByMtime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Small caps so three 40-byte entries exceed the max on the third write.
	s := &Store{dir: dir, maxBytes: 100, evictToBytes: 60}
	blob := []byte(strings.Repeat("x", 40))

	s.Put("ts", "oldest", blob)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(s.entryPath("ts", "oldest"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	s.Put("ts", "middle", blob)
	mid := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(s.entryPath("ts", "middle"), mid, mid); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	// Third write pushes the tree to 120 > 100: eviction must delete oldest
	// and middle to get back under the 60-byte target, keeping the newest.
	s.Put("ts", "newest", blob)

	if _, ok := s.Get("ts", "oldest"); ok {
		t.Error("oldest entry should be evicted")
	}
	if _, ok := s.Get("ts", "middle"); ok {
		t.Error("middle entry should be evicted (burst eviction to target)")
	}
	if _, ok := s.Get("ts", "newest"); !ok {
		t.Error("newest entry should survive eviction")
	}
}

// TestStore_GetTouchesMtime pins the LRU read-touch: a hit refreshes the
// entry's mtime so recently-used entries survive mtime-ordered eviction.
func TestStore_GetTouchesMtime(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	s.Put("ts", "hot", []byte(`{"fact":1}`))
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(s.entryPath("ts", "hot"), old, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("ts", "hot"); !ok {
		t.Fatal("expected hit")
	}
	fi, err := os.Stat(s.entryPath("ts", "hot"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.ModTime().Sub(old) < time.Hour {
		t.Errorf("Get must touch mtime for LRU: still %v", fi.ModTime())
	}
}

func TestKey_ComponentChangeInvalidates(t *testing.T) {
	t.Parallel()
	base := Key("ts", "v1", "cfg-a", "tree-a")
	cases := map[string]string{
		"same components ⇒ same key":    Key("ts", "v1", "cfg-a", "tree-a"),
		"analyzer change ⇒ new key":     Key("py", "v1", "cfg-a", "tree-a"),
		"tool version change ⇒ new key": Key("ts", "v2", "cfg-a", "tree-a"),
		"config slice change ⇒ new key": Key("ts", "v1", "cfg-b", "tree-a"),
		"input tree change ⇒ new key":   Key("ts", "v1", "cfg-a", "tree-b"),
	}
	if got := cases["same components ⇒ same key"]; got != base {
		t.Errorf("identical inputs must produce identical keys: %s != %s", got, base)
	}
	for name, key := range cases {
		if name == "same components ⇒ same key" {
			continue
		}
		if key == base {
			t.Errorf("%s: key did not change", name)
		}
	}
}

func TestHashJSON_Deterministic(t *testing.T) {
	t.Parallel()
	const keyEnabled, keyInclude, glob = "enabled", "include", "src/**"
	view := map[string]any{keyInclude: []string{glob}, keyEnabled: true}
	h1, err := HashJSON(view)
	if err != nil {
		t.Fatalf("HashJSON: %v", err)
	}
	h2, err := HashJSON(map[string]any{keyEnabled: true, keyInclude: []string{glob}})
	if err != nil {
		t.Fatalf("HashJSON: %v", err)
	}
	if h1 != h2 {
		t.Error("same content in different map order must hash identically")
	}
	h3, err := HashJSON(map[string]any{keyInclude: []string{glob}, keyEnabled: false})
	if err != nil {
		t.Fatalf("HashJSON: %v", err)
	}
	if h1 == h3 {
		t.Error("different content must hash differently")
	}
}

func TestHashTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	const fileA, fileB = "a.go", "sub/b.go"
	write(fileA, "package a")
	write(fileB, "package b")

	base, err := HashTree(root, []string{fileA, fileB})
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}

	reordered, err := HashTree(root, []string{fileB, fileA})
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}
	if reordered != base {
		t.Error("enumeration order must not affect the hash")
	}

	write(fileA, "package a // changed")
	changed, err := HashTree(root, []string{fileA, fileB})
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}
	if changed == base {
		t.Error("file content change must change the hash")
	}

	write("renamed.go", "package a // changed")
	renamed, err := HashTree(root, []string{"renamed.go", fileB})
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}
	if renamed == changed {
		t.Error("same content under a different relpath must change the hash")
	}

	if _, err := HashTree(root, []string{"missing.go"}); err == nil {
		t.Error("missing file must be an error, not a silent skip")
	}
}

// ListInputs must hash THROUGH file symlinks (the tools follow them, so target
// edits must invalidate) and surface directory symlinks (WalkDir cannot descend
// them — including the path makes HashTree error and the caller veto caching).
// Dangling links carry no readable content and are skipped.
func TestListInputs_Symlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	write := func(dir, rel, content string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	link := func(target, rel string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(root, rel)); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	const plainFile, linkedFile, realFile = "plain.go", "linked.go", "real.go"
	write(root, plainFile, "package a")
	write(outside, realFile, "package b")
	if err := os.MkdirAll(filepath.Join(outside, "pkg"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link(filepath.Join(outside, realFile), linkedFile)
	link(filepath.Join(outside, "pkg"), "linkeddir")
	link(filepath.Join(outside, "gone.go"), "dangling.go")

	got := ListInputs(root, MatchExts([]string{".go"}, nil), nil)
	want := []string{linkedFile, "linkeddir", plainFile}
	if !slices.Equal(got, want) {
		t.Fatalf("ListInputs = %v, want %v", got, want)
	}

	// The file symlink hashes its target's content: edits behind the link
	// must change the tree hash.
	before, err := HashTree(root, []string{plainFile, linkedFile})
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}
	write(outside, realFile, "package b // changed")
	after, err := HashTree(root, []string{plainFile, linkedFile})
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}
	if before == after {
		t.Error("edit behind a file symlink must change the hash")
	}

	// A directory symlink in the list must be a HashTree error (cache veto),
	// never a silent skip.
	if _, err := HashTree(root, got); err == nil {
		t.Error("directory symlink in inputs must veto hashing, not skip")
	}
}

func TestListInputs_IncludesProjectDotDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	write(".storybook/main.ts", "export default {}")
	write(".cargo/config.toml", "[build]\n")
	write(".git/config", "[core]\n")
	write(".archfit-cache/facts/ts/blob.json", "{}")
	write(".venv/site.py", "cache")
	write("src/app.ts", "export const app = 1")

	got := ListInputs(root, MatchExts([]string{".ts", ".toml"}, nil), nil)
	want := []string{".cargo/config.toml", ".storybook/main.ts", "src/app.ts"}
	if !slices.Equal(got, want) {
		t.Fatalf("ListInputs = %v, want %v", got, want)
	}
}
