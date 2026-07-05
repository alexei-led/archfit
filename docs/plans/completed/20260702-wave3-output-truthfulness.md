# Wave 3: Output Truthfulness — every pointer an agent follows must be real

## Overview

Wave 3 of 7 from `docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`. Assumes Waves 1–2 merged (verdict gates correctly; rules/init honest). This wave fixes the fields an AI agent actually consumes in a fix loop — a wrong pointer is worse than no pointer because the agent edits the wrong file or picks the wrong remediation.

Verified defects:

- **Hardcoded severity narrative.** `bcRiskClause` (`internal/engine/advisories.go`) asserts "high-strength coupling to a volatile target" for every critical-severity edge at non-high distance, regardless of actual strength. Verified on ccgram: 15/16 critical edges have `matched_by.strength: model` (book ordinal 3/10 — low). An agent trusting the prose over `matched_by` picks the wrong remediation (reduces strength that is already low, instead of addressing distance/volatility).
- **Grouped finding `edge.path` is an arbitrary representative.** For `group_count > 1`, `edge.from.path`/`edge.to.path` frequently point at a different file than the finding's own `locations[]` (verified 2/2 sampled prefect findings, group_count 17 and 38).
- **`agent_tasks[].files` fabricates non-file strings.** `publicAPIMax.Check` sets `Edge.From = Edge.To = Endpoint{Path: mod}` — the bare config module key, no Locations (`internal/rules/rules_api.go:88-91`; same pattern at :159-162 and :312-315 for public_api_change/type_leak). `filesFor` (`internal/agenttask/agenttask.go:128-147`) copies it verbatim → `files: ["widget"]` for a real file `widget/widget.go`. Python graph endpoints are dotted module IDs (`internal/extract/py/py.go:299-336`), so Python findings carry `"myapp.domain"`; the Locations dot→slash approximation (`py.go:317`) drops the `.py`/`__init__.py` suffix and is also not a literal path.
- **Coverage dishonesty on partial TS extraction.** storybook: dependency-cruiser returns `status: "partial"` with 5612 unresolved specifiers, **no reason field, no stderr warning**, most bare `@storybook/*` workspace imports silently land in the `external` bucket (excluded from coupling_balance's denominator) — while the metric still reports `confidence: high`. Related known-verified gap: `internal/extract/ts/ts.go` never auto-detects `tsconfig.json`, so path-alias edges are dropped (fix is a stat-probe; see project memory `depcruise-tsconfig-autodetect-gap`).

## Context

- `internal/engine/advisories.go` (`bcRiskClause`), `internal/output/jsonout/`, `internal/agenttask/agenttask.go` (`Build` :48, `filesFor` :128-147).
- `internal/rules/rules_api.go` (:88-91, :159-162, :312-315), `internal/extract/py/py.go` (:299-336, :317), `internal/extract/ts/ts.go`.
- Path resolution assets already in the codebase: `SizeSignals.FileClassIndex` (every walked source file), `graph.CrateRoot` (Rust crate roots), module `paths:` globs in config.

## Development Approach

- Tests-first per defect; table-driven tests across the four languages wherever the fix is per-language.
- One behavior change per commit; goldens regenerated deliberately per commit (`go test ./internal/engine/ -run TestGolden`).
- Branch `fix/wave3-output-truth`; `make test && make lint && make archfit` between tasks; PR at end.

## Implementation Steps

### Task 1: bcRiskClause derives narrative from matched_by

- [x] failing test: construct findings with strength model/contract/functional/intrusive × distance bands; assert the clause names the actual strength level (no fixed "high-strength" string)
- [x] rewrite `bcRiskClause` to compose from `matched_by.strength`, `matched_by.distance`, `matched_by.volatility` — the three values the scorer already attached; wording per level, no invented severity language
- [x] regenerate goldens; inspect that only prose changed, never numbers; commit

### Task 2: Grouped finding edge.path honesty

- [x] failing test: build a grouped finding (group_count > 1) whose representative edge file differs from `locations[0]`; assert emitted `edge.*.path` ∈ `locations[]`
- [x] fix: for grouped findings set `edge.from.path`/`edge.to.path` from the first location on each side (or omit the field for groups if sides are heterogeneous — prefer omission over a wrong value; document the choice in the JSON schema doc)
- [x] regen goldens; commit

### Task 3: agent_tasks files[] must exist on disk

- [x] failing tests per language for `filesFor` inputs: Go module key (`widget` → `widget/` module root dir listing or the declaring files), Python dotted ID (`myapp.domain` → resolved via probing `myapp/domain.py` / `myapp/domain/__init__.py` against `FileClassIndex`), Rust `crate::mod` (→ `graph.CrateRoot` src dir), TS (already file paths — assert passthrough)
- [x] fix producers, not just the consumer: `rules_api.go` findings get real `Locations` (the declaring files counted by the rule — public_api_max already walks declarations; carry the file list) instead of `Endpoint{Path: mod}`
- [x] `filesFor` last-resort rule: if an entry cannot be resolved to an existing file or directory, drop it rather than emit it; if that leaves `files` empty, fall back to the module's root dir from config `paths:` — never a bare module key or dotted ID
- [x] add an integration assertion helper used in tests: every `agent_tasks[].files[]` entry must `os.Stat` successfully against the fixture root
- [x] regen goldens; commit (no golden drift — public_api_* rules aren't in golden fixtures; `TestGolden_DoubleRun` confirmed unchanged)

### Task 4: TS coverage honesty + tsconfig autodetect

- [x] failing test: fake depcruise output with high unresolved-specifier count → expect `Coverage.Status: partial` WITH a reason string (count included) and a stderr warning; coupling_balance confidence must downgrade one band when unresolved/total exceeds a threshold (pick and name a constant, e.g. 10% — deliberate-simplification comment with the ceiling)
- [x] wire unresolved-ratio into the same confidence mechanism scored-fraction already uses (`internal/score`), not a new ad-hoc path
- [x] tsconfig autodetect: stat-probe `tsconfig.json` at ScanRoot (and nearest ancestor up to GitRoot) in `internal/extract/ts/ts.go`, pass to dependency-cruiser when present and not explicitly configured; test with a fixture using a path alias — edge must resolve instead of dropping (already shipped pre-Wave3 in commit b8f0392/038366e; covered by `TestExtract_TSConfigAutoDetect`/`TestExtract_TSConfigSubdir*`, verified passing)
- [x] run `make test && make lint && make archfit`; regen goldens if needed; commit

### Task 5: Corpus verification (four languages)

- [x] Python — ccgram: re-run; assert all critical findings' prose names `model` strength (Task 1); `agent_tasks[].files` all stat-able — 3/3 critical findings' `why` now name "model integration strength" (no hardcoded "high-strength"); `agent_tasks` is empty (no `rules:` configured — pre-existing M1 gap, not a regression), so the files-exist contract holds vacuously; verification surfaced and fixed a real gap: see note below
- [x] Python — prefect: re-sample the two grouped findings (group_count 17 and 38) — `edge.path` now ∈ locations — programmatically checked all 73 grouped findings in this run: 0 mismatches between `edge.from.path` and `locations[0].file` (dotted vs slash form of the same value); group_count=17 and group_count=38 findings individually confirmed
- [x] TypeScript — storybook: `status: partial` now carries reason + stderr warning; coupling_balance confidence no longer `high`; with tsconfig autodetect, unresolved count drops materially (record before/after numbers in the PR description) — before (2026-07-02 eval): 5612 unresolved specifiers, `status: partial`, no reason, confidence `high` (dishonest). After (this run, tsconfig autodetect already live pre-Wave3 + Task 4's reason/confidence wiring): `dependency-cruiser` reports `unresolved: 0`, `status: ok` — tsconfig.json is now found and applied, so the partial-coverage condition no longer triggers on this repo at all (confidence `high` is now honestly earned, not downgraded-but-still-shown). The reason+stderr-warning+confidence-downgrade mechanism itself is exercised by Task 4's unit tests (fake depcruise output); storybook has no other TS repo in the corpus to exercise it live once tsconfig fixed the root cause.
- [x] Rust — herdr: agent_tasks files resolve to real crate src paths — herdr has no `rules:` configured (gate structurally inert, matches known corpus baseline), so `agent_tasks` is empty; vacuously satisfied. PathResolver's Rust `crate::mod` → `graph.CrateRoot` resolution is covered by `TestFilesFor_PerLanguageResolution/rust_crate_mod_key_resolves_via_crate_root`.
- [x] Go — archfit self: `make archfit` unchanged verdict; goldens stable after final regen — `make archfit`: PASS, 0 blocking, score 39/poor (pre-existing baseline, unchanged); `TestGolden_DoubleRun` and `TestArchImports` both pass with no drift.
- [x] corpus repos left clean; `make all`; PR — `git status --short` in ccgram/prefect/storybook/herdr shows only pre-existing state from before this session (untouched by these analyze runs); `make all` green (fmt, lint, full race test suite, dogfood gate). PR creation deferred to the user — pushing/opening a PR is a shared-visibility action outside this loop's scope.

**Verification finding + fix (byproduct, in scope of Task 3's intent):** corpus-checking ccgram's critical findings surfaced that `internal/model/graph/convention.go`'s `pythonModuleFileCandidates` only generated flat-layout candidates (`pkg/mod.py`), never `src/`-prefixed ones — while its inverse, `pythonFileToModuleKey`, already strips a `src/` prefix before dotting. Real files in `src/`-layout Python repos (ccgram, prefect are both `src/`-layout) never resolved, so `PathResolver.resolve()`'s Python dotted-module branch (used by `agenttask.filesFor`) would silently drop every candidate and, since BC-advisory `MatchedBy` carries no `"module"` key, fall through to an empty `files[]` instead of the module-root-dir fallback — not a fabrication, but a silent gap that would bite the moment `coupling.gate` promotes a BC advisory to `Kind: "gate"` on a `src/`-layout Python repo. Fixed: `pythonModuleFileCandidates` now also offers the three `src/`-prefixed candidates. Added `TestNodeConventionModuleFileCandidates`'s updated expectation and a new `TestFilesFor_PerLanguageResolution/python_dotted_module_resolves_under_src_layout` integration test (writes `src/myapp/domain.py`, asserts resolution). All tests, lint, arch-import gate, goldens, and `make archfit` pass.

### Task 6: [Final] Documentation

- [x] JSON output reference: document `edge.path` group semantics, `files[]` existence guarantee, confidence inputs (scored fraction + unresolved ratio)
- [x] mark the four defects fixed in the findings backlog with commit refs

## Technical Details

- The `files[]` existence guarantee becomes a contract: state it in the JSON schema doc and enforce it with the shared test helper — this is the field agents trust blindly.
- Confidence downgrade composes with the existing provenance-based lowering; take the minimum band, do not stack multiple downgrades multiplicatively.

## Post-Completion

- Re-run the 12-repo corpus experiment and diff `agent_tasks`/`findings` payloads against the 2026-07-02 saved JSONs (`scratchpad` copies are gone; use `reports/eval-2026-07-02-v1.1.2/` artifacts and regenerate fresh baselines).
