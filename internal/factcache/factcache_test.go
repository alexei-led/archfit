package factcache

import (
	"os"
	"path/filepath"
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
