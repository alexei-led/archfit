package factcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Key derives the cache key for one analyzer invocation scope
// (fact-cache.md D3): sha256 over the schema version, analyzer name, tool
// version, config-slice hash, and input-tree hash, NUL-separated to prevent
// boundary ambiguity. Any component change ⇒ new key ⇒ miss — stale entries
// are never served, only orphaned (eviction reclaims them).
func Key(analyzer, toolVersion, configSliceHash, inputTreeHash string) string {
	return hashParts(schemaVersion, analyzer, toolVersion, configSliceHash, inputTreeHash)
}

// HashJSON returns the sha256 hex of v's JSON encoding. encoding/json sorts
// map keys, so the encoding — and the hash — is deterministic. Used for the
// config-slice component of Key: hash the typed view the analyzer consumes,
// not the whole config, so an unrelated config edit does not invalidate facts.
func HashJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("factcache: hash config slice: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// HashTree returns the input-tree hash for Key: sha256 over the sorted
// (relpath, content-hash) pairs of relPaths under root. Content hashes, not
// mtimes — mtime invalidation breaks under git checkout/rebase, and hashing
// a whole repo is sub-second next to the subprocess runs the cache guards.
// Enumeration order does not matter; a missing or unreadable file is an
// error and the caller falls back to an uncached run.
func HashTree(root string, relPaths []string) (string, error) {
	sorted := slices.Clone(relPaths)
	slices.Sort(sorted)
	h := sha256.New()
	for _, rel := range sorted {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // callers enumerate rel from their own scope walk
		if err != nil {
			return "", fmt.Errorf("factcache: hash tree: %w", err)
		}
		sum := sha256.Sum256(data)
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(sum[:])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashParts is the shared NUL-separated sha256 (same convention as the LLM
// cache key builder).
func hashParts(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
