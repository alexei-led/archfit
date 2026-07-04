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

- [x] Go: cache `packages.Load` results per workspace member (key includes go.mod/go.sum hashes); test: edit one member file → only that member re-loads — asserted via a fake `packages.Load` loader seam, not Runner counts (go/packages never touches toolrun.Runner; D5 seam 2). Serialized per-member facts (`golang/cache.go`); keys include the member's intra-workspace `require` closure, so editing a dependency re-loads its dependents too (`TestFactCache_DependentMemberInvalidates`); partial loads never cached
- [x] TypeScript: cache dependency-cruiser output (key includes tsconfig.json + package.json + lockfile hashes, resolved tsconfig content, depcruise version); test invalidation on tsconfig change + unresolved-output veto (node_modules state is unkeyed, so partial results are never written)
- [x] Python: cache grimp graph (key includes .py tree + manifests + helper-source hash); Runner `EntryArgs` override absorbs the random temp helper path; test invalidation on a .py file change + unresolved veto
- [x] Rust: cache cargo metadata (manifests-only key — a .rs edit must NOT re-run it) + cargo-modules (.rs tree key) + SCIP reader output (edge/symbol JSON, not the raw index blob — orders of magnitude smaller, so the D4 size cap trivially holds); tests: Cargo.toml change invalidates both, .rs change invalidates only cargo-modules; SCIP warm run does zero index/read subprocess work
- [x] clones (jscpd) + ast-grep: same treatment (clones store-direct — jscpd reports land on disk, not stdout; ast-grep via the Runner decorator incl. Stream); per-analyzer timeout semantics unchanged, timed-out runs NOT cached (`TestFactCache_TimedOutRunNotCached`, astgrep exec-error test)
- [x] `make test && make lint && make archfit`; commit per language if diffs are big — single commit; also plumbed: `--no-cache` on analyze/baseline/explain/enrich governs the fact store (nil store = off), registry passes the store to extractors, warm vs `--no-cache` verified byte-identical on archfit itself (warm full gate ≈2.9 s vs 5.3–8.2 s cold)

### Task 4: Incremental --base

- [x] base side facts keyed by commit SHA (+ config-slice + tool versions): immutable ⇒ cache forever; second `--base <same-ref>` run must skip all base-side subprocess work (assert via Runner call counts) — shipped as a deterministic per-SHA worktree path (`baseWorktreeParent`, `<configDir>/.archfit-cache/worktrees/<sha>`), key formula unchanged: several analyzers fold the scan root into their key because cached output embeds absolute paths (cargo `manifest_path`, Go member facts), so a random temp path missed every run; same SHA ⇒ same root ⇒ the existing content keys hit. Fallback to the old random temp dir on `--no-cache`/unresolvable ref/mkdir failure. fact-cache.md D3 updated
- [x] current side uses the Task 2/3 content cache normally — asserted in the same test (head-side cargo metadata: 0 calls on the warm run)
- [x] test: `--base` twice in a row → identical delta table, base-side extractors invoked zero times on the second run — `TestDiffCmd_BaseSideFactCacheReuse` (fake cargo Runner counts `cargo metadata` WorkDirs: cold 2 → warm 0 → ref moved 1 base-side re-run; byte-identical stdout; fails against the pre-fix random-path behavior, mutation-checked)
- [x] `make test && make lint && make archfit`; commit — also smoke-checked on archfit itself: `--base HEAD~1` twice → byte-identical JSON, no stale worktree registrations

### Task 5: Timing verification (four languages)

- [x] add `make bench-gate`: times cold vs warm gate on the current repo and prints both (reported number, NOT a hard CI assert — machine-dependent) — `scripts/bench-gate.sh`; cold clears ONLY `<configDir>/.archfit-cache/facts`
- [x] measure and record in the PR description, cold → warm: Go archfit self (target warm < 5 s), Python ccgram (target < 10 s), Rust herdr (target < 20 s), TypeScript storybook and Python prefect (record; target ≥ 3× speedup warm) — PR #25 addendum: archfit 8.8→3.0 s, ccgram 29.8→8.7 s, herdr 49.8→10.9 s, storybook 11.2→3.6 s (all targets met); prefect 773.3→720.3 s (1.07×, MISS — grimp/ast-grep facts hit but jscpd+SCIP end partial/timed-out on prefect and are vetoed from caching by design, so warm re-pays both; targeted follow-up per Post-Completion)
- [x] byte-identical corpus check: for archfit + ccgram, diff warm-cached JSON vs `--no-cache` JSON — identical (cmp: archfit 455 KB, ccgram 1.7 MB)
- [x] verify `.archfit-cache/` is in default exclusions for the file walks (scope + loc + gocyclo all three — see project memory `archfit-file-walk-exclusion-fragmentation`) and gitignored in `config init` output — scope glob `internal/scope/scope.go:61`; LOC dot-dir skip pinned by new `loc_test.go` case; gocyclo walker no longer exists (complexity backends removed in v1.0); `config init` prints a gitignore hint when `.gitignore` lacks `.archfit-cache` (table-driven test)
- [x] corpus repos left clean apart from their pre-existing cache dirs; `make all`; PR — ccgram `.archfit.yaml` diff and herdr untracked artifacts predate this wave; prefect/storybook clean; `make all` green; addendum appended to PR #25

### Task 6: [Final] Documentation

- [x] `docs/guide/`: cache behavior, key inputs, `--no-cache`, cache location, eviction; troubleshooting entry (delete `.archfit-cache` to reset) — new `docs/guide/caching.md` (byte-identical contract, per-analyzer key table, `--no-cache` read+write bypass, `--base` per-SHA reuse, 1 GiB/768 MiB LRU eviction, reset), linked from the guide README; `commands.md` `--no-cache` help updated to "extractor facts (and LLM responses with --llm)" + flag added to the analyze flag list + `--base` cache note; `troubleshooting.md` "Suspected stale cache" section
- [x] update findings backlog item 9 with commit refs and the measured before/after table — the wave-6 latency work has no numbered backlog item in 00-FINDINGS.md §4 (items 1–15 are all pre-wave-6; "item 9" there is the wave-3 files[] fix), so recorded as new item 16 under a "Performance (§3 latency, wave 6)" stanza with commit refs `328923e`/`c8fe2d3`/`fbf8179`/`6a8c4eb`/`4299bf4` + the cold→warm table (archfit 8.8→3.0s, ccgram 29.8→8.7s, herdr 49.8→10.9s, storybook 11.2→3.6s, prefect 773.3→720.3s miss); §3 Latency paragraph marked ✅ FIXED and §5 "no cache" answer annotated. reports/ is gitignored — local-only update, same as prior waves

## Technical Details

- The cache stores extractor FACTS (graph JSON, metadata), never scores — config/scoring changes take effect instantly without invalidation complexity.
- Timed-out or partial-status analyzer results are never written to cache (a cached degradation would be sticky and violate coverage honesty from Wave 3).
- Concurrency: analyze may run per-member loads concurrently — writes are atomic rename; last-writer-wins is fine (same key ⇒ same content).

## Post-Completion

- Re-run the corpus timing table and archive next to the eval reports; if prefect warm is still > 60 s, profile where the time goes (likely grimp cold-start dominance) and open a targeted follow-up — do not expand this wave's scope.
