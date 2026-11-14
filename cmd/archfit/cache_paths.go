package main

import "path/filepath"

// factsCacheDir returns the extractor fact-cache directory under baseDir —
// facts/ in .archfit-cache, so
// "delete .archfit-cache to reset" stays the single troubleshooting answer.
func factsCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "facts")
}
