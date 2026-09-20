package factcache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestListInputs_ExpansionBudgetNeverReturnsPartialHash(t *testing.T) {
	root, shared := t.TempDir(), t.TempDir()
	for i := range 150 {
		if err := os.WriteFile(filepath.Join(shared, fmt.Sprintf("%d.go", i)), []byte("package input"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, filepath.Join(root, fmt.Sprintf("dependency-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	if hash, err := HashTree(root, ListInputs(root, MatchAll, nil)); err == nil || hash != "" {
		t.Fatalf("over-budget inventory must veto caching: hash=%q error=%v", hash, err)
	}
}

func TestListInputs_SymlinkCycleVetoesCache(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(root, filepath.Join(root, "cycle")); err != nil {
		t.Fatal(err)
	}
	if hash, err := HashTree(root, ListInputs(root, MatchAll, nil)); err == nil || hash != "" {
		t.Fatalf("cyclic inputs must veto caching: hash=%q error=%v", hash, err)
	}
}

func TestHashTree_ContentBudgetNeverReturnsPartialHash(t *testing.T) {
	root := t.TempDir()
	f, err := os.Create(filepath.Join(root, "large.bin")) //nolint:gosec // isolated test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxInputBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if hash, err := HashTree(root, []string{"large.bin"}); err == nil || hash != "" {
		t.Fatalf("over-budget content must veto caching: hash=%q error=%v", hash, err)
	}
}
