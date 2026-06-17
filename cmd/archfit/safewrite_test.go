package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalValidYAML is a minimal .archfit.yaml that passes config.Load validation.
const minimalValidYAML = "version: 1\n"

func newTestDeps(t *testing.T) *appDeps {
	t.Helper()
	var buf bytes.Buffer
	return &appDeps{Stdout: &buf}
}

func TestSafeWriteConfig_OriginalNil_ConcurrentAppearance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".archfit.yaml")

	// Pre-create the file — simulates concurrent appearance.
	if err := os.WriteFile(path, []byte(minimalValidYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	deps := newTestDeps(t)
	err := safeWriteConfig(context.Background(), deps, path, []byte(minimalValidYAML), nil)
	if err == nil {
		t.Fatal("expected error when file appeared concurrently (original==nil)")
	}
	if !strings.Contains(err.Error(), "appeared concurrently") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSafeWriteConfig_BackupClobber_TimestampedFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".archfit.yaml")

	// Write an initial config.
	original := []byte(minimalValidYAML)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	// Pre-create the .bak so safeWriteConfig must use a timestamped name.
	bakPath := path + ".bak"
	existingBak := []byte("# existing backup\n")
	if err := os.WriteFile(bakPath, existingBak, 0o600); err != nil {
		t.Fatal(err)
	}

	deps := newTestDeps(t)
	edited := []byte("version: 1\n# edited\n")
	if err := safeWriteConfig(context.Background(), deps, path, edited, original); err != nil {
		t.Fatalf("safeWriteConfig failed: %v", err)
	}

	// The original .bak must be preserved (not overwritten).
	gotBak, err := os.ReadFile(bakPath) //nolint:gosec
	if err != nil {
		t.Fatalf("original .bak missing after apply: %v", err)
	}
	if !bytes.Equal(gotBak, existingBak) {
		t.Errorf("original .bak was clobbered: got %q, want %q", gotBak, existingBak)
	}

	// A timestamped backup must exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var timestamped []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".bak") && name != filepath.Base(bakPath) {
			timestamped = append(timestamped, name)
		}
	}
	if len(timestamped) == 0 {
		t.Error("expected a timestamped .bak file to be created alongside the original .bak")
	}
}
