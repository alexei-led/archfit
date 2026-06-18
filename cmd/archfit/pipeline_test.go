package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEffectiveConfigHash verifies that --no-config never hashes the on-disk
// config file: a run that ignored the file must report no hash, even when the
// file exists, so the hash never reflects (or changes with) an ignored file.
func TestEffectiveConfigHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// noConfig=true: file present but ignored → empty hash.
	if got := effectiveConfigHash(path, true); got != "" {
		t.Errorf("effectiveConfigHash(existing, noConfig=true) = %q, want \"\"", got)
	}

	// noConfig=false: file present and read → non-empty hash.
	withCfg := effectiveConfigHash(path, false)
	if withCfg == "" {
		t.Error("effectiveConfigHash(existing, noConfig=false) = \"\", want a hash")
	}

	// noConfig=false but file absent → empty hash (never fails on missing config).
	if got := effectiveConfigHash(filepath.Join(dir, "absent.yaml"), false); got != "" {
		t.Errorf("effectiveConfigHash(absent, noConfig=false) = %q, want \"\"", got)
	}

	// Mutating the ignored file must NOT change the no-config result (stays empty).
	if err := os.WriteFile(path, []byte("version: 1\nmodules: {}\n"), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if got := effectiveConfigHash(path, true); got != "" {
		t.Errorf("effectiveConfigHash after mutating ignored file = %q, want \"\"", got)
	}
}
