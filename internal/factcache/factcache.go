// Package factcache is the extractor-fact cache adapter: content-addressed
// JSON blobs under .archfit-cache/facts/<analyzer>/<key>.json that let a warm
// run skip expensive out-of-process extractor work (docs/design/fact-cache.md).
// It stores FACTS — the subprocess output extractors derive facts from —
// never scores: classification and scoring re-run every time, so config and
// ScoreVersion changes take effect instantly with no invalidation logic.
//
// The cache is best-effort by contract: a corrupted or unreadable entry is a
// miss (logged at debug), a failed write never fails the run, and a nil
// *Store disables reads AND writes (the --no-cache path — a bypassed run must
// not poison or refresh entries).
package factcache

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// MaxBytes caps the total size of the facts tree (fact-cache.md D4);
	// sized for SCIP index blobs, the largest fact class.
	MaxBytes int64 = 1 << 30 // 1 GiB
	// EvictToBytes is the eviction target (75% of MaxBytes), so eviction
	// runs in bursts instead of on every write at the boundary.
	EvictToBytes int64 = 768 << 20 // 768 MiB

	// schemaVersion participates in every cache key (fact-cache.md D3);
	// bump it whenever the serialized fact shape changes so a new binary
	// never misreads old blobs.
	schemaVersion = "2"

	entryExt = ".json"
)

// Store is the content-addressed fact-blob store rooted at
// .archfit-cache/facts. A nil *Store is valid and disables the cache: Get
// always misses and Put is a no-op.
type Store struct {
	dir          string
	maxBytes     int64
	evictToBytes int64
}

// NewStore returns a Store rooted at dir (e.g. <configDir>/.archfit-cache/facts)
// with the default size cap. The directory is created lazily on first Put.
func NewStore(dir string) *Store {
	return &Store{dir: dir, maxBytes: MaxBytes, evictToBytes: EvictToBytes}
}

// Get returns the blob stored under analyzer/key. Any miss — absent entry,
// unreadable file, nil store — is (nil, false); the caller re-derives. A hit
// touches the entry mtime (best-effort) so LRU eviction spares it.
func (s *Store) Get(analyzer, key string) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	path := s.entryPath(analyzer, key)
	data, err := os.ReadFile(path) //nolint:gosec // path is a content hash under our cache dir
	if err != nil {
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return data, true
}

// Put stores data under analyzer/key atomically (tmp file + rename in the
// same directory — same key ⇒ same content, so concurrent last-writer-wins
// is safe). Best-effort: any failure is logged at debug and never fails the
// run. A successful write triggers the size-cap eviction pass.
func (s *Store) Put(analyzer, key string, data []byte) {
	if s == nil {
		return
	}
	dir := filepath.Join(s.dir, analyzer)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		slog.Debug("factcache: mkdir failed", "dir", dir, "err", err)
		return
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		slog.Debug("factcache: create temp failed", "dir", dir, "err", err)
		return
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup on the error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		slog.Debug("factcache: write failed", "path", tmp.Name(), "err", err)
		return
	}
	if err := tmp.Close(); err != nil {
		slog.Debug("factcache: close failed", "path", tmp.Name(), "err", err)
		return
	}
	if err := os.Rename(tmp.Name(), s.entryPath(analyzer, key)); err != nil {
		slog.Debug("factcache: rename failed", "key", key, "err", err)
		return
	}
	s.evict()
}

func (s *Store) entryPath(analyzer, key string) string {
	return filepath.Join(s.dir, analyzer, key+entryExt)
}

// evict deletes oldest-by-mtime entries across all analyzers until the facts
// tree is back under evictToBytes — but only once total size exceeds
// maxBytes. Best-effort: walk and delete errors are ignored.
//
// Deliberate simplification: a full stat-walk of the facts tree on every
// triggering write, no index file, no locking (concurrent evictors may
// double-delete — harmless: deletes are idempotent and a lost entry is just
// a miss). Ceiling: tens of thousands of entries; upgrade trigger: this walk
// showing up in bench-gate profiles.
func (s *Store) evict() {
	type entry struct {
		path  string
		mtime time.Time
		size  int64
	}
	var entries []entry
	var total int64
	_ = filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort: skip unreadable entries
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil //nolint:nilerr // best-effort: skip unstatable entries
		}
		entries = append(entries, entry{path: path, mtime: info.ModTime(), size: info.Size()})
		total += info.Size()
		return nil
	})
	if total <= s.maxBytes {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].mtime.Before(entries[j].mtime) })
	for _, e := range entries {
		if total <= s.evictToBytes {
			return
		}
		if err := os.Remove(e.path); err == nil {
			total -= e.size
		}
	}
}
