# v1.0.0 release-readiness fixes

Batch of fixes gating archfit v1.0.0, from the 2026-06-30 corpus eval + advisor review.
Decisions are baked in below. Cadence: build+test+**golden-inspect after each
output-affecting change** (not one big-bang regen); verify-then-fix each inherited
polish item on the current binary before patching; end gate = `make all` + `make
archfit` + re-run corpus eval. Do NOT `make build` ideas without running its tests.

Status legend: [ ] todo · [x] done · [~] in progress · [defer] out of scope

## Tranche A — cmd/init (no golden impact)

### Task A1: [x] init --root writes to the target dir, not CWD

`archfit init --root <dir>` currently writes `.archfit.yaml` to CWD. Resolve the
target dir from --root (default CWD) and write there. Verify: `init --root <tmp>`
creates `<tmp>/.archfit.yaml`, leaves CWD untouched.

### Task A2: [x] init no-clobber default + --force

If the target `.archfit.yaml` exists: load+validate it. If valid → print
"config exists and is valid; re-run with --force to overwrite" and **exit 0** (guard,
not error). If invalid → print the validation error + suggest --force. With `--force`
→ current behavior (backup + overwrite). Add the `--force` flag to the init command.
Tests: exists+valid→refuse+exit0+unchanged; exists+invalid→refuse+exit0; --force→overwrite+backup; absent→write.

### Task A3 (F3): [x] .env also loaded from the analyzed repo

`loadDotEnv(".env")` reads CWD only. Also load `<root>/.env` (root = --root or
config dir) so the documented in-repo LLM run picks up a key. CWD/real env still win.
Verify: key in `<root>/.env`, run from elsewhere → LLM configured.

## Tranche B — config contract (no golden impact)

### Task B1:[x] unknown `metrics` keys handled like `analyzers`

`metrics` is `map[string]MetricEntry` so unknown keys escape `DisallowUnknownField`.
Validate metric keys against the known set after decode; unknown (non-deprecated) key
→ config error (exit 3), consistent with analyzers. Pairs with B2's deprecation list.

### Task B2:[x] v0.12 migration = actionable hard-error

`tools:` is NOT a pure rename (old `tools:` carried `complexity`), so no shim. Make the
decode error actionable: detect top-level `tools:` and removed metric keys
(`risk_hub`, `functional_candidates`, `complexity`, `gitnexus`) and emit a clear
message: "`tools:` was renamed to `analyzers:` in v1.0; `analyzers.complexity`,
`metrics.risk_hub`, `metrics.functional_candidates` were removed — see docs/guide".
Update docs/guide with the migration note.

## Tranche C — ownership

### Task C1 (#3): [x] DOCUMENT the dominant-owner limitation (no code change)

[defer-code] Switching first-owner→all-owners does NOT fix prometheus/prefect collapse:
a catch-all `* @team` line makes that team dominant on every file regardless. Real fix
is owner-SET distance (disjoint sets) — a distance-model change, separate scoped task.
For 1.0: document in docs/guide + FINDINGS that owner-distance uses a single dominant
owner per module, so repos whose CODEOWNERS list a common team first / use a broad
catch-all under-report cross-team distance. File a follow-up task for owner-sets.

### Task C2 (#4): [x] make owner_source real (dedicated field, JSON-visible)

`Resolve` returns `(map, source)` where source = codeowners|git|none. Add a top-level
`distance_confidence.owner_source` (or similar) field to the Diagnostic (JSON + markdown),
populated at the single call site. Do NOT overload `Coverage.Status` (stays ok/partial/
absent for buildCoverageGaps). Update markdown to read the real field. Golden + JSON impact.

## Tranche D — output (GOLDEN IMPACT — inspect diff after each)

### Task D1 (F5): [x] JSON carries score + coupling_balance + delta

Embed the 0-100 score (value+band), the coupling_balance dimension, and (when --base)
a delta object into the JSON output. Score/scorecard computed in internal/score; thread
it onto the Diagnostic (or jsonout wrapper) so `--json` and `--base --json` expose it.
Additive fields → do NOT bump schema_version (grep showed only doc.go references it;
baseline has its own version). Verify: `--json|jq .score`, `--base --json|jq .delta`.

### Task D2:[x] render-only advisory cap

Cap/group advisories in console + markdown only ("+N more" or per-module rollup).
verdict, summary.warnings (FULL count), baseline matching, --severity filtering, JSON,
SARIF all see the FULL set. Trap: summary counts must reflect full, not capped.

### Task D3:[x] cycle display conveys Rust-module cycles

Display string should distinguish benign Rust intra-crate module cycles from real
import cycles (band is already language-aware; the display only shows the count).

### Task D4:[x] blast_radius scales to module-rich graphs

Threshold (>30% reverse-deps) flags most modules on 25-100-module graphs. Scale the
threshold or cap the hub list with "+N more" so the signal isn't diluted.

### Task D5:[x] delta progress counter overflow

Base-side scan reuses the head counter → `[7/6]`. Fix the total or use a separate
counter for the base pass.

## Tranche E — extract/classify

### Task E1:[x] drop *.test pseudo-modules from file_facts

Go test-binary pseudo-packages (`*.test`, loc 0, paths into go-build cache) leak into
file_facts. Filter them out (50 in archfit's own output).

### Task E2:[x] snap case-variant --root in worktree.go

Delta path recomputes rel(gitRoot, root) by string and rejects macOS case-variant
roots that the main scan path accepts (snapScanRoot/os.SameFile). Mirror the snap.

### Task E3:[x] Rust missing Cargo.toml → n/a, not exit 3

extract/rust hard-errors (exit 3) when Cargo.toml is absent. Degrade to an n/a coverage
record + continue, like other optional analyzers.

## Tranche F — finalize

- Regenerate goldens deliberately (after each D task), inspect diffs.
- `make all` (fmt+lint+test+archfit) green; `make archfit` dogfood gate green.
- Re-run corpus eval on the fixed binary; write reports/eval-2026-06-30-corpus/00-FINDINGS.md.
- Update docs/guide: config schema, migration note, init behavior, owner-distance limitation.
- Follow-up task (separate): owner-SET distance model (real fix for #3).
