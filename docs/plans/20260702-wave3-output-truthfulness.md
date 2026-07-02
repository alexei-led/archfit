# Wave 3: Output Truthfulness — every pointer an agent follows must be real

## Overview

Wave 3 of 7 from `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`. Assumes Waves 1–2 merged (verdict gates correctly; rules/init honest). This wave fixes the fields an AI agent actually consumes in a fix loop — a wrong pointer is worse than no pointer because the agent edits the wrong file or picks the wrong remediation.

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

- [ ] failing test: construct findings with strength model/contract/functional/intrusive × distance bands; assert the clause names the actual strength level (no fixed "high-strength" string)
- [ ] rewrite `bcRiskClause` to compose from `matched_by.strength`, `matched_by.distance`, `matched_by.volatility` — the three values the scorer already attached; wording per level, no invented severity language
- [ ] regenerate goldens; inspect that only prose changed, never numbers; commit

### Task 2: Grouped finding edge.path honesty

- [ ] failing test: build a grouped finding (group_count > 1) whose representative edge file differs from `locations[0]`; assert emitted `edge.*.path` ∈ `locations[]`
- [ ] fix: for grouped findings set `edge.from.path`/`edge.to.path` from the first location on each side (or omit the field for groups if sides are heterogeneous — prefer omission over a wrong value; document the choice in the JSON schema doc)
- [ ] regen goldens; commit

### Task 3: agent_tasks files[] must exist on disk

- [ ] failing tests per language for `filesFor` inputs: Go module key (`widget` → `widget/` module root dir listing or the declaring files), Python dotted ID (`myapp.domain` → resolved via probing `myapp/domain.py` / `myapp/domain/__init__.py` against `FileClassIndex`), Rust `crate::mod` (→ `graph.CrateRoot` src dir), TS (already file paths — assert passthrough)
- [ ] fix producers, not just the consumer: `rules_api.go` findings get real `Locations` (the declaring files counted by the rule — public_api_max already walks declarations; carry the file list) instead of `Endpoint{Path: mod}`
- [ ] `filesFor` last-resort rule: if an entry cannot be resolved to an existing file or directory, drop it rather than emit it; if that leaves `files` empty, fall back to the module's root dir from config `paths:` — never a bare module key or dotted ID
- [ ] add an integration assertion helper used in tests: every `agent_tasks[].files[]` entry must `os.Stat` successfully against the fixture root
- [ ] regen goldens; commit

### Task 4: TS coverage honesty + tsconfig autodetect

- [ ] failing test: fake depcruise output with high unresolved-specifier count → expect `Coverage.Status: partial` WITH a reason string (count included) and a stderr warning; coupling_balance confidence must downgrade one band when unresolved/total exceeds a threshold (pick and name a constant, e.g. 10% — deliberate-simplification comment with the ceiling)
- [ ] wire unresolved-ratio into the same confidence mechanism scored-fraction already uses (`internal/score`), not a new ad-hoc path
- [ ] tsconfig autodetect: stat-probe `tsconfig.json` at ScanRoot (and nearest ancestor up to GitRoot) in `internal/extract/ts/ts.go`, pass to dependency-cruiser when present and not explicitly configured; test with a fixture using a path alias — edge must resolve instead of dropping
- [ ] run `make test && make lint && make archfit`; regen goldens if needed; commit

### Task 5: Corpus verification (four languages)

- [ ] Python — ccgram: re-run; assert all critical findings' prose names `model` strength (Task 1); `agent_tasks[].files` all stat-able
- [ ] Python — prefect (configs saved under `reports/eval-2026-07-02-v1.1.2/`): re-sample the two grouped findings (group_count 17 and 38) — `edge.path` now ∈ locations
- [ ] TypeScript — storybook: `status: partial` now carries reason + stderr warning; coupling_balance confidence no longer `high`; with tsconfig autodetect, unresolved count drops materially (record before/after numbers in the PR description)
- [ ] Rust — herdr: agent_tasks files resolve to real crate src paths
- [ ] Go — archfit self: `make archfit` unchanged verdict; goldens stable after final regen
- [ ] corpus repos left clean; `make all`; PR

### Task 6: [Final] Documentation

- [ ] JSON output reference: document `edge.path` group semantics, `files[]` existence guarantee, confidence inputs (scored fraction + unresolved ratio)
- [ ] mark the four defects fixed in the findings backlog with commit refs

## Technical Details

- The `files[]` existence guarantee becomes a contract: state it in the JSON schema doc and enforce it with the shared test helper — this is the field agents trust blindly.
- Confidence downgrade composes with the existing provenance-based lowering; take the minimum band, do not stack multiple downgrades multiplicatively.

## Post-Completion

- Re-run the 12-repo corpus experiment and diff `agent_tasks`/`findings` payloads against the 2026-07-02 saved JSONs (`scratchpad` copies are gone; use `reports/eval-2026-07-02-v1.1.2/` artifacts and regenerate fresh baselines).
