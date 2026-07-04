# Extractor fact cache — design

Status: approved design (Wave 6 Task 1, plan `docs/plans/20260702-wave6-perf-cache.md`).
Source: `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` §3 (latency): cold ≈ warm on
every corpus repo because no gate-level cache exists; prefect full scan is 8m13s and
`--base` doubles full-scan cost.

Goal: cache the expensive out-of-process extractor work, keyed by content, so a warm
run skips subprocesses. Behavioral contract: **a cached run is byte-identical to an
uncached run on the same tree** — the cache stores facts, never decisions.

## Audit of the existing cache (confirmed 2026-07-04)

`.archfit-cache/` today holds ONLY the LLM response cache:

- `internal/llm/cache.go` — a `Provider` decorator; sha256 content-address over
  `provider|system|user|max_tokens`; atomic tmp+rename writes; corrupt entry ⇒
  drop and refetch; cache-write failure never fails the run.
- The path is built in exactly one place: `llmCacheDir` (`cmd/archfit/enrich.go`)
  → `<configDir>/.archfit-cache/llm`. Imported exclusively from `cmd/archfit/*`.
- `--no-cache` exists only on the five LLM commands (`config init/update/enrich`,
  `explain --llm`, `analyze --llm`) with help text "Bypass the LLM response cache."
- `**/.archfit-cache/**` is already in `scope.DefaultExclusions` (`scope.go`).
  Task 5 re-verifies the loc and gocyclo walkers (they walk independently — see
  project memory `archfit-file-walk-exclusion-fragmentation`) and the
  `config init` gitignore snippet.

There is no extractor-fact cache to extend. The fact cache is new code that shares
only the top-level directory name. `internal/llm.Cache` is NOT reused: extractors
cannot import `internal/llm` (arch_test forbids it), and the LLM cache's key inputs
(prompt content) have nothing in common with fact-cache keys.

## Decisions

### D1 — Layout: `facts/` beside `llm/`, one root

```
.archfit-cache/
  llm/                      # existing LLM response cache (unchanged)
  facts/<analyzer>/<key>.json   # NEW: content-addressed extractor fact blobs
```

Rooted at the same base dir as the LLM cache (the config dir), so "delete
`.archfit-cache/` to reset" stays the single troubleshooting answer.
`<analyzer>` is the analyzer name (`go`, `typescript`, `python`, `rust`,
`cargo_modules`, `scip`, `clones`, `astgrep` — as wired in Task 3);
`<key>` is the hex sha256 key from D3. Writes are
atomic (tmp file + rename in the same directory); same key ⇒ same content, so
concurrent last-writer-wins is safe (plan Technical Details).

### D2 — One flag governs both caches

`--no-cache` grows to mean "bypass ALL archfit caches" (LLM responses + extractor
facts), on every command that runs the pipeline (`analyze`, `baseline`, `explain`,
including both sides of `analyze --base`). No sibling flag.

Rationale: the user intent behind the flag is singular — "give me a fresh,
trust-nothing run." Two flags create a combinatorial matrix nobody wants to
reason about in CI, and correctness must never depend on the flag anyway (stale
entries are prevented by the key, not by bypassing). Help text changes from
"Bypass the LLM response cache." to "Bypass archfit caches (LLM responses and
extractor facts)." `--no-cache` bypasses cache READS and WRITES (a bypassed run
must not poison or refresh entries — it is a control run, per the byte-identical
correctness gate).

### D3 — Cache key

`key = sha256(schema \0 analyzer \0 toolVersion \0 configSliceHash \0 inputTreeHash)`,
hex-encoded. `\0` separators prevent boundary ambiguity (same convention as
`llm.Cache.key`). Components:

- **schema** — `factCacheSchemaVersion` constant (starts at `1`); bumped whenever
  the serialized fact shape changes, so a new binary never misreads old blobs.
- **analyzer** — the analyzer name (also the subdirectory; kept in the hash so a
  renamed directory cannot alias).
- **toolVersion** — from the extractor's existing per-run version probe
  (`detectVersion` in ts/rust, `toolVersion` in py; probes are added in Task 3
  where an analyzer lacks one, e.g. jscpd, ast-grep, scip). NOT from `doctor`:
  `Runner.Detect` deliberately leaves `ToolInfo.Version` empty, and the probe
  must run in the same invocation that fills the cache. For the Go analyzer the
  "tool" is the toolchain: `go version` output. Probe cost (~50 ms) is noise
  next to the subprocess it guards; probes themselves are never cached.
- **configSliceHash** — sha256 of the deterministic JSON encoding of the typed
  config view that analyzer consumes (`Config.ForExtract(lang)` plus the
  analyzer's own block, e.g. `analyzers.scip`), per the existing config-views
  convention. Hashing the slice — not the whole config — means editing an
  unrelated rule does not invalidate extractor facts. Go's `encoding/json`
  sorts map keys, so the encoding is deterministic.
- **inputTreeHash** — sha256 over the sorted list of `(relpath \0 sha256(content))`
  pairs for every file in that extractor's input scope (source files matching the
  extractor's language, plus its manifest files: `go.mod`/`go.sum`,
  `tsconfig.json`/`package.json`, `Cargo.toml`/`Cargo.lock`, etc. — per-language
  detail lands in Task 3). The scope must cover the TOOL'S real input set, so
  config `exclude:` globs never filter it: grimp/depcruise/cargo/`packages.Load`
  analyze excluded files anyway, and a file the tool reads but the hash skips is
  a silent-staleness hole (the hash may over-approximate inputs, never
  under-approximate; jscpd is the exception — it receives the exclusions via
  `--ignore`, so its hash faithfully applies them). Content hashes, not
  mtimes: mtime invalidation breaks under git checkout/rebase, and hashing a
  whole repo is sub-second next to multi-minute subprocess runs.

`--base <ref>` side (Task 4, as shipped): the key formula is UNCHANGED — the
commit SHA enters through the worktree PATH instead. Several analyzers fold
the scan-root path into their config-slice hash because the cached subprocess
output embeds absolute paths (cargo metadata's `manifest_path`, Go member
facts), so a random temp worktree would miss every run. `scoreBaseRef` now
checks the base ref out at the deterministic path
`<configDir>/.archfit-cache/worktrees/<sha>` (`baseWorktreeParent`,
`cmd/archfit/worktree.go`): same ref ⇒ same SHA ⇒ same absolute root ⇒ the
existing content keys hit, and the second `--base <same-ref>` run does zero
base-side subprocess work. The checkout is removed after each run (only fact
blobs persist); `--no-cache`, an unresolvable ref, or a cleanup/mkdir failure
falls back to the historical random temp dir — correct, just uncached.

Never written to cache: timed-out runs, partial-status results
(`Coverage.Status != ok`), and exec-level failures — a cached degradation would
be sticky and violate Wave 3 coverage honesty. Legitimate non-zero exits that
extractors treat as usable output (e.g. dependency-cruiser reporting violations)
are cacheable because the extractor, not the raw exit code, decides what a fact
is (see D5). A Go member whose build reaches local source no key covers — a
`replace` to a local directory (in its go.mod, transitively, or in go.work), or
a `require` satisfied by a go.work member outside the loaded set (excluded or
`tools.go.modules`-filtered) — is vetoed per-member: it loads fresh every run
while unaffected siblings still cache (`memberKeys`, `internal/extract/golang/cache.go`).

### D4 — Eviction: size cap + LRU by mtime

Constants (named in `internal/factcache`, values final unless profiling says
otherwise):

- `FactCacheMaxBytes = 1 GiB` — total size cap for `.archfit-cache/facts/`;
  sized for SCIP index blobs, the largest fact class (plan Task 3).
- `FactCacheEvictToBytes = 768 MiB` — eviction target (75% of cap), so eviction
  runs in bursts instead of on every write at the boundary.

Policy: after a write, if the facts tree exceeds `FactCacheMaxBytes`, delete
oldest entries by file mtime across all analyzers until under
`FactCacheEvictToBytes`. A cache HIT touches the entry's mtime (best-effort
`os.Chtimes`), so recently-used entries survive — LRU, not FIFO. Eviction is
best-effort and never fails the run.

Deliberate simplification: eviction stat-walks the whole facts tree (O(n) in
entry count), no index file, no locking (concurrent evictors may double-delete —
harmless, deletes are idempotent and a lost entry is just a miss). Ceiling:
tens of thousands of entries; upgrade trigger: the eviction walk showing up in
`bench-gate` profiles.

### D5 — Placement: adapter package `internal/factcache`, two consumption seams

`internal/factcache` is an adapter (imports `os`; the core ring must NOT import
it — Task 2 extends `internal/arch_test.go`). It exposes a small store
(`Get(analyzer, key) ([]byte, bool)` / `Put(analyzer, key, []byte)`) plus the
key-builder helpers, consumed at two seams:

1. **Runner-shaped analyzers** (dependency-cruiser, grimp, cargo metadata,
   cargo-modules, SCIP, ast-grep): a caching decorator around
   `toolrun.Runner` — the single subprocess choke point — where the extractor
   supplies the key material (tool version, config-slice hash, input-tree hash)
   because the Runner alone cannot see it.
2. **Store-direct consumers**: `packages.Load` (Go) never touches
   `toolrun.Runner` (go/packages shells out internally), so the Go extractor
   uses the store directly, caching the serialized per-workspace-member fact
   set (key includes `go.mod`/`go.sum` hashes) — one entry per member, so
   editing one member re-loads only that member. jscpd also uses the store
   directly (as shipped in Task 3): it writes its report to a temp file, not
   stdout, so the Runner decorator — which caches stdout — cannot see the
   fact payload (`internal/extract/clones/clones.go`).

In both seams the cache stores extractor FACTS (parsed graph/metadata JSON the
extractor would otherwise derive from the subprocess), never scores —
classification and scoring are cheap and re-run every time, so config and
ScoreVersion changes take effect instantly with no invalidation logic.

## Out of scope for this doc

Per-language key details and invalidation tests (Task 3), `--base` wiring
(Task 4), timing targets and corpus verification (Task 5), user-facing docs
(Task 6).
