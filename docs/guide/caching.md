# Caching

archfit caches the expensive out-of-process extractor work (`go list`/packages.Load,
dependency-cruiser, grimp, cargo metadata, cargo-modules, SCIP, jscpd, ast-grep) so
a warm run skips subprocesses. Measured on real repos, a warm gate is typically
3–5× faster than cold (archfit itself: ~8 s → ~3 s). Compare on your repo with
`make bench-gate` (in the archfit source tree) or by timing a run before and after
deleting the cache.

**Correctness contract:** a cached run is byte-identical to an uncached run on the
same tree. The cache stores extractor **facts** (dependency graphs, metadata), never
scores or decisions — classification and scoring re-run every time, so editing
`.archfit.yaml` rules or upgrading archfit's scoring takes effect instantly with no
cache invalidation step.

## Location

```
<config dir>/.archfit-cache/
  facts/<analyzer>/<key>.json   # extractor fact blobs (content-addressed)
  llm/                          # AI response cache (enrich/explain/analyze --ai-summary)
```

The directory sits next to `.archfit.yaml`. Add `.archfit-cache/` to `.gitignore`
(`archfit config init` prints a hint when it is missing). It is already in
archfit's built-in scan exclusions, so the cache is never measured back into the
analysis.

## What invalidates an entry

Cache keys are content hashes — there is no time-based expiry. An entry is reused
only when **all** of these are unchanged:

- the analyzer's tool version (probed each run: `go version`, depcruise, grimp,
  cargo, ast-grep, jscpd, SCIP indexer);
- the slice of `.archfit.yaml` that analyzer consumes (editing an unrelated rule
  does not invalidate extractor facts);
- the analyzer's input files, by content hash:

| Analyzer                  | Keyed on                                                                                                                                          |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go                        | per workspace member: member sources + `go.mod`/`go.sum` + intra-workspace deps — editing one member re-loads only that member and its dependents |
| TypeScript (depcruise)    | source tree + `tsconfig.json` (resolved) + `package.json` + lockfile                                                                              |
| Python (grimp)            | `.py` tree + manifests                                                                                                                            |
| Rust `cargo metadata`     | manifests only (`Cargo.toml`/`Cargo.lock`) — a `.rs` edit does not re-run it                                                                      |
| Rust cargo-modules / SCIP | `.rs` tree (+ manifests); SCIP caches the parsed edge/symbol output, not the raw index                                                            |
| clones (jscpd), ast-grep  | their source-file scope                                                                                                                           |

Never cached: timed-out runs, partial-status results, and tool failures — a cached
degradation would be sticky. A corrupted cache entry is treated as a miss, never an
error. A Go workspace member whose build reaches source the key cannot see — a
`replace` pointing at a local directory, or a dependency on a go.work member
filtered out by exclusions or `tools.go.modules` — always runs fresh; unaffected
members still cache. Config `exclude:` globs never shrink the key's input hash:
the underlying tools analyze excluded files anyway, so their edits still
invalidate.

## `--refresh`

`--refresh` on `analyze`, `baseline`, `explain`, and `config enrich` bypasses
cache reads, re-runs extractors, and writes fresh results back to cache. On the
AI commands the same flag also bypasses the AI response cache.

`--refresh` re-runs extractors and writes fresh results to cache (unlike the old
`--no-cache` which also disabled cache writes). Correctness never depends on the
flag: stale entries are prevented by the key, not by bypassing.

## `--base` and the cache

`analyze --base <ref>` checks the base ref out at a deterministic per-commit path
under `.archfit-cache/worktrees/<sha>`, so base-side facts are keyed by commit and
reused forever: the second run against the same ref does zero base-side subprocess
work. The checkout itself is removed after each run; only fact blobs persist.

## Eviction

The facts tree is capped at 1 GiB; when a write exceeds the cap, the oldest entries
(LRU by mtime — hits refresh mtime) are evicted down to 768 MiB. Eviction is
best-effort and never fails a run.

## Reset

Delete the directory:

```sh
rm -rf .archfit-cache
```

Safe at any time — the next run is a cold run that repopulates it.
