package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Cache is a content-addressed Provider decorator. A hit skips the network
// entirely, which is what makes `enrich` replay deterministic: same provider,
// model, prompts → same cache key → byte-identical response. The cache
// directory is committable — a committed cache replays identically across
// machines. Corrupt entries are refetched, never fatal.
type Cache struct {
	inner Provider
	dir   string

	// RefreshMode bypasses cache reads but still writes the fresh response.
	RefreshMode bool
}

// NewCache wraps inner with a file cache rooted at dir
// (e.g. <repo>/.archfit-cache/llm). The directory is created lazily.
func NewCache(inner Provider, dir string) *Cache {
	return &Cache{inner: inner, dir: dir}
}

// Name returns the inner provider's name (the cache is transparent).
func (c *Cache) Name() string { return c.inner.Name() }

// Complete returns the cached response when present, otherwise calls the
// inner provider and stores the result (atomic temp+rename write).
func (c *Cache) Complete(ctx context.Context, req Request) (Response, error) {
	path := filepath.Join(c.dir, c.key(req)+".json")

	if !c.RefreshMode {
		if data, err := os.ReadFile(path); err == nil { //nolint:gosec // path is derived from a content hash under our cache dir
			var resp Response
			if json.Unmarshal(data, &resp) == nil && resp.Text != "" {
				return resp, nil
			}
			// Corrupt or empty entry: drop it and refetch.
			_ = os.Remove(path)
		}
	}

	resp, err := c.inner.Complete(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if werr := c.store(path, resp); werr != nil {
		// A cache-write failure must not fail the run; the response is valid.
		return resp, nil //nolint:nilerr // intentional: cache is best-effort
	}
	return resp, nil
}

// key derives the content address: provider/model + full request content.
func (c *Cache) key(req Request) string {
	h := sha256.New()
	for _, part := range []string{c.inner.Name(), req.System, req.User, strconv.FormatInt(maxTokensOrDefault(req), 10)} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// store writes the response atomically (temp file + rename).
func (c *Cache) store(path string, resp Response) error {
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup on the error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
