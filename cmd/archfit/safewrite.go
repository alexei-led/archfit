package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/internal/config"
)

// safeWriteConfig writes edited bytes to path using the strict write protocol:
//  1. Write to a temp file in the same directory.
//  2. Validate via config.Load.
//  3. Concurrency guard: if original != nil, re-read path and abort if changed;
//     if original == nil (new file), abort if path now exists.
//  4. Back up any existing file (non-clobbering).
//  5. Atomic os.Rename.
//
// Notices are printed to deps.Stdout.
func safeWriteConfig(ctx context.Context, deps *appDeps, path string, edited, original []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".archfit-init-*.yaml")
	if err != nil {
		return fmt.Errorf("safeWriteConfig: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	if _, err := tmp.Write(edited); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("safeWriteConfig: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("safeWriteConfig: close temp: %w", err)
	}

	// Validate: the edited YAML must parse cleanly.
	if _, err := config.Load(ctx, tmpName); err != nil {
		return fmt.Errorf("safeWriteConfig: validation failed: %w", err)
	}

	// Concurrency guard.
	if original != nil {
		cur, readErr := os.ReadFile(path) //#nosec G304
		if readErr != nil || !bytesEqual(cur, original) {
			return fmt.Errorf("safeWriteConfig: %s changed since read — aborting to avoid overwrite", path)
		}
	} else {
		if _, statErr := os.Stat(path); statErr == nil {
			return fmt.Errorf("safeWriteConfig: %s appeared concurrently — aborting", path)
		}
	}

	// Backup existing file (non-clobbering).
	if _, statErr := os.Stat(path); statErr == nil {
		bakPath := path + ".bak"
		if _, err := os.Stat(bakPath); err == nil {
			// .bak exists — use a timestamped name.
			bakPath = path + "." + time.Now().Format("20060102-150405") + ".bak"
		}
		if err := copyFile(path, bakPath); err != nil {
			return fmt.Errorf("safeWriteConfig: backup failed: %w", err)
		}
		_, _ = fmt.Fprintf(deps.Stdout, "backed up %s → %s\n", filepath.Base(path), filepath.Base(bakPath))
	}

	// Atomic rename.
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("safeWriteConfig: rename failed: %w", err)
	}
	_, _ = fmt.Fprintf(deps.Stdout, "wrote %s\n", path)
	return nil
}

// bytesEqual reports whether a and b have identical content.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// copyFile copies src to dst, creating dst with mode 0o600.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //#nosec G304
	if err != nil {
		return err
	}
	defer in.Close()                                                       //nolint:errcheck
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //#nosec G304
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
