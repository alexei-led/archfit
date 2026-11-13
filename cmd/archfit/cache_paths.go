package main

import "path/filepath"

func llmCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "llm")
}

// factsCacheDir returns the extractor fact-cache directory under baseDir —
// facts/ beside llm/ in the same .archfit-cache root (fact-cache.md D1), so
// "delete .archfit-cache to reset" stays the single troubleshooting answer.
func factsCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "facts")
}
