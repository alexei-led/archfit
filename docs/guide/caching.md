# Caching

archfit caches expensive extractor work when its inputs can be identified safely
and within a bounded cost. A warm run can skip extraction, while version probes
and producers without a reliable cache identity still run. Measure your workload
with `make bench-gate` in the archfit source tree.

Python/grimp currently runs fresh: the `uv` launcher version does not identify
the transient Python/grimp environment. Replaying cached helper output could
replay both stale facts and their old version. This does not disable uv's own
package cache.

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

- the analyzer's tool version (probed each run: `go version`, depcruise,
  cargo, ast-grep, jscpd, SCIP indexer, and `go env` for the standard-library
  list rule selectors are judged against);
- the slice of `.archfit.yaml` that analyzer consumes (editing an unrelated rule
  does not invalidate extractor facts);
- the analyzer's input files, by content hash:

| Analyzer                  | Keyed on                                                                                                                                                                       |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Go                        | per workspace member: every source file the member load can reach + `go.mod`/`go.sum` + intra-workspace deps — editing one member re-loads only that member and its dependents |
| TypeScript (depcruise)    | source tree + `tsconfig.json` and its complete supported `extends` chain (including files outside `--root`) + `package.json` + lockfile                                        |
| Python (grimp)            | Not fact-cached; the helper runs on every analysis                                                                                                                             |
| Rust `cargo metadata`     | manifests only (`Cargo.toml`/`Cargo.lock`) — a `.rs` edit does not re-run it                                                                                                   |
| Rust cargo-modules / SCIP | `.rs` tree (+ manifests); SCIP caches the parsed edge/symbol output, not the raw index                                                                                         |
| clones (jscpd), ast-grep  | their source-file scope                                                                                                                                                        |
| Go standard library list  | the toolchain only: `go env GOVERSION GOROOT GOOS GOARCH GOFLAGS GOEXPERIMENT CGO_ENABLED`, probed in the repository so a `go.mod` `toolchain` line counts — no source input  |

Never cached: timed-out runs, partial-status results, and tool failures — a cached
degradation would be sticky. A corrupted cache entry is treated as a miss, never an
error. A Go workspace member whose build reaches source the key cannot see — a
`replace` pointing at a local directory, or a dependency on a go.work member
filtered out by exclusions or `languages.go.modules` — always runs fresh;
unaffected members still cache. Config `exclude:` globs never shrink the key's
input hash: the underlying tools analyze excluded files anyway, so their edits
still invalidate.

The shared input walker skips only Archfit metadata (`.archfit-cache` and `.git`).
Names such as `target`, `venv`, `node_modules`, and `__pycache__` do not prove
that an analyzer ignores source there; an analyzer-specific input set must opt
out explicitly. This conservative rule keeps a source file in a tool-readable
directory from becoming an invisible cache dependency. For TypeScript, Archfit
hashes the complete supported `extends` chain; if an inherited config cannot be
resolved, it bypasses the fact cache for that run instead of reusing an
unverifiable entry. Dynamic dependency-cruiser configurations do not use fact
caching. For supported JS configurations, the native dependency-cruiser loader
evaluates the configuration once and the graph invocation consumes that frozen
snapshot, so measurement identity records the actual settings. Unsupported
integrations (such as opaque webpack/Babel/plugin settings) and unsupported
compiler-config resolution retain facts but have an unknown identity. Unknown
inputs prevent numerical comparisons; repeating the same warning does not make
them trustworthy.

Cache-key work is bounded: at most 20,000 enumerated entries, 64 directory levels,
and 32 MiB of hashed file content. Exhausting a budget produces no usable key
and the analyzer runs fresh. Directory symlinks are followed within these
bounds; cycles and read failures also bypass caching. No partial hash is reused.

TypeScript excludes root `node_modules` source bytes, matching the tool's
exclusion, but includes a bounded inventory of dependency filenames and resolver
metadata so file-presence and package-resolution changes invalidate entries.
AST keys use the requested language's source extensions and ignore/config
inputs; a build binary is not parsed as source. Rust `.rs` files under `target`
remain eligible inputs because generated or explicitly configured source may
be analyzed there. Large or unsupported scopes run fresh.

## `--refresh`

`--refresh` on `analyze`, `baseline`, `explain`, and `config enrich` bypasses
cache reads, re-runs extractors, and writes fresh results back to cache. On the
AI commands the same flag also bypasses the AI response cache.

`--refresh` re-runs extractors and writes fresh results to cache (unlike the old
`--no-cache` which also disabled cache writes). Correctness never depends on the
flag: stale entries are prevented by the key, not by bypassing.

`config compare` runs two full pipelines and does normal cached reads and writes,
but has no `--refresh` flag of its own. Both sides share the current config's
bundle directory, so to force fresh facts for a comparison, refresh that cache
first with `archfit analyze --refresh -c <current>`, or delete
`.archfit-cache/facts`.

## `--base` and the cache

`analyze --base <ref>` checks the base ref out at a deterministic per-commit path
under `.archfit-cache/worktrees/<sha>`. Repeated runs can reuse base-side facts
when their tool and input identities match. Version probes and uncached
producers still run. The checkout itself is removed after each run; only fact
blobs persist.

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
