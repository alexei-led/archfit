package factcache

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// Runner is a caching decorator around toolrun.Runner (fact-cache.md D5,
// seam 1). It serves Run/Stream results from the fact store on a key hit and
// records them on a miss. Construct one Runner per analyzer invocation
// scope, immediately before the expensive subprocess call — never around
// version probes: probes are key material, not facts, and are never cached.
//
// Entries are addressed by Key plus a digest of the command name and args.
// WorkDir and Env are deliberately excluded: a --base temp worktree gets a
// fresh absolute path every run, and content identity is already pinned by
// the key's input-tree hash. Corollary: commands that differ ONLY by WorkDir
// or Env MUST use distinct Keys, and args must stay deterministic
// (repo-relative, no temp paths) for entries to survive across runs.
type Runner struct {
	// Inner executes on a cache miss. Required.
	Inner toolrun.Runner
	// Store holds the fact blobs. nil disables the cache entirely — the
	// --no-cache path: no reads, no writes.
	Store *Store
	// Analyzer is the store subdirectory this runner writes under.
	Analyzer string
	// Key is the invocation-scope cache key from Key() — tool version,
	// config-slice hash, and input-tree hash already folded in.
	Key string
	// Cacheable, when non-nil, vetoes storing a completed result (e.g. an
	// extractor refusing to cache output it would report as partial —
	// fact-cache.md D3). nil caches every exec-level success, any exit
	// code: the extractor, not the raw exit code, decides what a fact is.
	Cacheable func(toolrun.Output) bool
	// EntryArgs, when non-nil, replaces cmd Name+Args as the entry-address
	// material — for commands whose argv embeds a nondeterministic temp path
	// (e.g. the py grimp helper file). The caller must pick EntryArgs so that
	// Key+EntryArgs still uniquely identify the invocation's inputs.
	EntryArgs []string
}

var _ toolrun.Runner = (*Runner)(nil)

// cacheEntry is the stored JSON envelope: the full toolrun.Output a warm run
// must reproduce byte-for-byte.
type cacheEntry struct {
	Stdout   []byte `json:"stdout,omitempty"`
	Stderr   []byte `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// Detect delegates to Inner — tool presence probes are never cached.
func (r *Runner) Detect(ctx context.Context, tool string) (toolrun.ToolInfo, bool) {
	return r.Inner.Detect(ctx, tool)
}

// Run returns the cached Output on a hit; on a miss it executes via Inner
// and stores the result. Exec-level failures and timeouts (err != nil) are
// transient and never cached; non-zero exits are cached — the extractor
// decides whether that output is a usable fact.
func (r *Runner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	key := r.entryKey(cmd)
	if e, ok := r.lookup(key); ok {
		return toolrun.Output{Stdout: e.Stdout, Stderr: e.Stderr, ExitCode: e.ExitCode}, nil
	}
	out, err := r.Inner.Run(ctx, cmd)
	if err != nil {
		return out, err
	}
	r.record(key, cacheEntry{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, out)
	return out, nil
}

// Stream serves the cached stdout to consume on a hit; on a miss it tees the
// live stream into the cache. The toolrun contract requires consume to drain
// stdout to EOF on success — bytes it leaves unread are drained by the inner
// runner and would be missing from the cached entry, so that contract is
// what keeps cached and live streams identical.
func (r *Runner) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	key := r.entryKey(cmd)
	if e, ok := r.lookup(key); ok {
		out := toolrun.Output{Stderr: e.Stderr, ExitCode: e.ExitCode}
		if err := consume(bytes.NewReader(e.Stdout)); err != nil {
			return out, err
		}
		return out, nil
	}
	var buf bytes.Buffer
	out, err := r.Inner.Stream(ctx, cmd, func(stdout io.Reader) error {
		return consume(io.TeeReader(stdout, &buf))
	})
	if err != nil {
		return out, err
	}
	r.record(key, cacheEntry{Stdout: buf.Bytes(), Stderr: out.Stderr, ExitCode: out.ExitCode}, out)
	return out, nil
}

// lookup fetches and decodes the entry under key. A corrupted blob is a
// miss: logged at debug and removed so the next successful run heals it —
// never a run failure.
func (r *Runner) lookup(key string) (cacheEntry, bool) {
	data, ok := r.Store.Get(r.Analyzer, key)
	if !ok {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		slog.Debug("factcache: corrupted entry treated as miss", "analyzer", r.Analyzer, "key", key, "err", err)
		_ = os.Remove(r.Store.entryPath(r.Analyzer, key))
		return cacheEntry{}, false
	}
	return e, true
}

// record stores the completed result unless the store is disabled or the
// Cacheable veto rejects it. Best-effort: a failed write never fails the run.
func (r *Runner) record(key string, e cacheEntry, out toolrun.Output) {
	if r.Store == nil {
		return
	}
	if r.Cacheable != nil && !r.Cacheable(out) {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		slog.Debug("factcache: marshal entry failed", "analyzer", r.Analyzer, "err", err)
		return
	}
	r.Store.Put(r.Analyzer, key, data)
}

// entryKey addresses one command under the runner's Key. Name and Args are
// folded in so an analyzer that runs several distinct commands under one Key
// cannot alias entries; WorkDir and Env are excluded (see type doc). A non-nil
// EntryArgs replaces Name+Args (see field doc).
func (r *Runner) entryKey(cmd toolrun.ToolCmd) string {
	addr := r.EntryArgs
	if addr == nil {
		addr = append(append(make([]string, 0, len(cmd.Args)+1), cmd.Name), cmd.Args...)
	}
	return hashParts(append([]string{r.Key}, addr...)...)
}
