# Wave 6: Performance — fact cache and incremental --base

## Overview

Wave 6 of 7 from `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` (§3 latency). Assumes Waves 1–5 merged; behaviorally this wave must be a pure no-op on results — same facts, same scores, byte-identical reports — only faster.

Measured on the corpus (v1.1.2): cold ≈ warm on every repo (no gate-level cache); archfit self 5.3–8.2 s, ccgram 27 s, herdr 79 s, prometheus 87 s, prefect 493 s (8 m13 s). `--base` re-runs the full pipeline on BOTH worktree sides (≈2× full-scan cost), so the one delta-shaped flag does not shortcut anything. For the goal "fast feedback loop for AI agents," per-edit use is only viable today on small repos.

Strategy: cache **extractor facts** (the expensive out-of-process work: `go list`/packages.Load, dependency-cruiser, grimp, cargo metadata, ast-grep, jscpd, SCIP), keyed by content, and reuse the immutable base side of `--base` keyed by commit SHA. Do NOT cache classification/scoring — it is cheap, and caching it would couple the cache to config/ScoreVersion churn.

## Context

- Every subprocess goes through `toolrun.Runner` (defined in `internal/toolrun`, `toolrun.go:52` — CLAUDE.md's "internal/ports" wording is stale) — the single choke point to instrument; extractors in `internal/extract/{go,ts,py,rust}`.
- IMPORTANT: `.archfit-cache/` and the `--no-cache` flag today serve ONLY the off-gate LLM response cache (`internal/llm/cache.go`, imported exclusively from `cmd/archfit/*`). There is NO existing extractor-fact cache to extend. The fact cache is NEW code that shares the top-level directory name; do NOT reuse `internal/llm.Cache` — extractors cannot import `internal/llm` (the arch_test import ring forbids it).
- Scope walking: `internal/scope` (ScanRoot, exclusions); `internal/syntax` FileClassIndex from the LOC walk.
- Timing evidence: `reports/eval-2026-07-02-v1.1.2/corpus-experiments.md` timing table (cold/warm per repo).

## Development Approach

- Correctness gate for every task: cached run output must be byte-identical to uncached (`--no-cache`) on the same tree — add this as an automated test and a corpus check.
- Tests-first for invalidation logic (the dangerous part); stale facts are worse than slow facts.
- Branch `feat/wave6-fact-cache`; `make test && make lint && make archfit` between tasks; PR per task group if large.

## Implementation Steps

### Task 1: Audit existing cache + design note

- [x] confirm `.archfit-cache/` holds only the LLM response cache today (`internal/llm/cache.go`); decide the fact-cache layout beside it (`facts/` subdir) and whether `--no-cache` governs both caches or gains a sibling flag (recommend: one flag governs both); write the decision into a short design doc `docs/design/fact-cache.md`
- [x] define the cache key: analyzer name + tool version (from `doctor` probes) + config-slice hash (only the fields that analyzer consumes — the typed view, per the existing config-views convention) + input-tree hash (file list + per-file content hash of files matching that extractor's scope; reuse FileClassIndex walk data where possible) — note: versions come from the extractors' own probes, not `doctor` (`Runner.Detect` leaves Version empty); see fact-cache.md D3
- [x] define eviction: size cap + LRU by directory mtime (name the constants; deliberate-simplification comment)
- [x] no code behavior change; commit the doc

### Task 2: Cache layer at the Runner seam

- [x] implement a caching decorator around `toolrun.Runner` (core ring stays pure — the cache is an adapter; verify `internal/arch_test.go` import ring still passes and extend it to forbid cache imports from core) — `internal/factcache.Runner`; entry address = Key + cmd Name/Args digest (WorkDir/Env excluded so --base worktrees can share entries); `Cacheable` veto seam for partial-status results (D3)
- [x] store: content-addressed JSON fact blobs under `.archfit-cache/facts/<analyzer>/<key>`; atomic write (tmp+rename) — plus D4 LRU eviction (MaxBytes/EvictToBytes) and an adapter-layer stanza in `.archfit.yaml` (catch-all would have classed it support ⇒ layer inversion)
- [x] `--no-cache` bypasses read AND write; corrupted/unreadable blob ⇒ treat as miss, log at debug, never fail the run — layer contract: nil `*Store` disables reads AND writes (tested); cmd flag plumbing lands with the first consumer (Task 3)
- [x] tests: hit, miss, corrupted blob, config-slice change invalidates, tool-version change invalidates, file-content change invalidates (table-driven)
- [x] byte-identical test: run a fixture analyze twice (cold, warm) and diff full JSON output — must be identical; run once with `--no-cache` — identical again (`TestByteIdentical_ColdWarmNoCache`; trivially green until Task 3 wires an analyzer — contract pinned first)
- [x] `make test && make lint && make archfit`; commit

### Task 3: Per-language wiring + invalidation tests

- [ ] Go: cache `packages.Load` results per workspace member (key includes go.mod/go.sum hashes); test: edit one member file → only that member re-loads (assert via fake Runner call counts)
- [ ] TypeScript: cache dependency-cruiser output (key includes tsconfig.json + package.json hashes); test invalidation on tsconfig change
- [ ] Python: cache grimp graph (key includes the package dir tree hash); test invalidation on a .py file change
- [ ] Rust: cache cargo metadata + cargo-modules + SCIP index outputs (key includes Cargo.toml/Cargo.lock hashes; SCIP blob is large — verify size cap handles it); test invalidation on Cargo.toml change
- [ ] clones (jscpd) + ast-grep: same treatment; per-analyzer timeout semantics unchanged (a timed-out run must NOT be cached)
- [ ] `make test && make lint && make archfit`; commit per language if diffs are big

### Task 4: Incremental --base

- [ ] base side facts keyed by commit SHA (+ config-slice + tool versions): immutable ⇒ cache forever; second `--base <same-ref>` run must skip all base-side subprocess work (assert via Runner call counts)
- [ ] current side uses the Task 2/3 content cache normally
- [ ] test: `--base` twice in a row → identical delta table, base-side extractors invoked zero times on the second run
- [ ] `make test && make lint && make archfit`; commit

### Task 5: Timing verification (four languages)

- [ ] add `make bench-gate`: times cold vs warm gate on the current repo and prints both (reported number, NOT a hard CI assert — machine-dependent)
- [ ] measure and record in the PR description, cold → warm: Go archfit self (target warm < 5 s), Python ccgram (target < 10 s), Rust herdr (target < 20 s), TypeScript storybook and Python prefect (record; target ≥ 3× speedup warm)
- [ ] byte-identical corpus check: for archfit + ccgram, diff warm-cached JSON vs `--no-cache` JSON — identical
- [ ] verify `.archfit-cache/` is in default exclusions for the file walks (scope + loc + gocyclo all three — see project memory `archfit-file-walk-exclusion-fragmentation`) and gitignored in `config init` output
- [ ] corpus repos left clean apart from their pre-existing cache dirs; `make all`; PR

### Task 6: [Final] Documentation

- [ ] `docs/guide/`: cache behavior, key inputs, `--no-cache`, cache location, eviction; troubleshooting entry (delete `.archfit-cache` to reset)
- [ ] update findings backlog item 9 with commit refs and the measured before/after table

## Technical Details

- The cache stores extractor FACTS (graph JSON, metadata), never scores — config/scoring changes take effect instantly without invalidation complexity.
- Timed-out or partial-status analyzer results are never written to cache (a cached degradation would be sticky and violate coverage honesty from Wave 3).
- Concurrency: analyze may run per-member loads concurrently — writes are atomic rename; last-writer-wins is fine (same key ⇒ same content).

## Post-Completion

- Re-run the corpus timing table and archive next to the eval reports; if prefect warm is still > 60 s, profile where the time goes (likely grimp cold-start dominance) and open a targeted follow-up — do not expand this wave's scope.
