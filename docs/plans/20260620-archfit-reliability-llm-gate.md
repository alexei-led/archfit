# archfit: reliability, fail-loud coverage, and LLM self-driving

> **Executable ralphex plan.** Run with `ralphex` from the repo root. ralphex executes
> each `### Task` sequentially in an isolated subagent; every task must end green
> (build + `make test` + `make lint`) before the next starts. ralphex CLI is installed
> (`/opt/homebrew/bin/ralphex`). ralphex moves the plan to `docs/plans/completed/` on
> completion. Source blueprint: the 2026-06-20 archfit-vs-architect study under
> `reports/archfit-vs-architect-20260620 (claude)/` and
> `reports/archfit-vs-architect-20260620 (openai)/` — both reproduce the same core
> defects; the OpenAI run also applied a partial `review` fix (see Task 6).

## Overview

Make `archfit` trustworthy as an **architecture gate** — a linter/test for architecture
drift — by removing every path where _absence of evidence is scored as good
architecture_, surfacing exactly which tools are missing and what they unlock, and
making the tool more self-driving via LLM-assisted config authoring.

The study compared archfit's deterministic scoring against the `architecture:architect`
LLM skill across five repos (pumba, spotinfo, ccgram, codegraph, archfit-self) and found
archfit reports false-greens: an unanalysed repo scores "strong" because the coverage
metric returns 100% when no extractor ran; `coupling_balance` returns 90 on empty edges;
missing analyzers degrade scores silently; `encapsulation` is permanently n/a for Go
(modules omit `owner`) so `boundary_integrity` is frozen at 60; `architecture_fitness`
counts a Go module-cache test file; `analysis_confidence` reports 100/100 while three
dimensions are n/a; `score --full`/`scorecard --full` reject the flag; `review` overflows
its token budget on ccgram.

Outcome: every metric is correct or **honestly absent**; a machine-readable coverage-gap
block (tool → disabled metrics → install command) appears in markdown + JSON + stderr;
an opt-in hard gate (`tools.<x>.gate: fail` / `--require-tools`) lets CI block on missing
tools; the LLM layer reliably authors `owner`/`subdomain`/`volatility` (the root cause of
the n/a metrics) via `enrich` and a new `autopilot` drafter; and a final sweep authors
real per-repo configs and runs full + delta on all five repos with high-confidence rules
promoted to `fail`.

Verdict the study confirmed and this plan preserves: archfit = reproducible fitness
function; architect = trustworthy judge; combine via enrich. This closes archfit's side.

## Context (from discovery)

Confirmed defects with file:line (from three Explore passes + a design pass):

| #   | Defect                                                                                                                                                                                  | Location                                          |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| 1   | `totalApplicable==0` → `value=1.0` (100%/high/strong) when **no extractor ran** → HIGH confidence propagates to all 5 structural dimensions (the false-green origin)                    | `internal/metrics/boundary/coverage.go:35`        |
| 2   | `couplingBalance(edges)` empty edges → {90, medium, strong}, no coverage guard                                                                                                          | `internal/score/score.go:205`                     |
| 3   | loc/complexity/clones/gitnexus tool errors discarded with `_`; git history error swallowed; no warning surfaced                                                                         | `cmd/archfit/pipeline.go`                         |
| 4   | `analysisConfidence` scores 100/100 even when encapsulation/cohesion_lcom/change_locality are all n/a                                                                                   | `internal/score/score.go`                         |
| 5   | `doctor` omits lizard, jscpd, gitnexus (the semantic tools whose absence lowers analysis_confidence)                                                                                    | `cmd/archfit/doctor.go`                           |
| 6   | config-quality "N modules under-specified" lint warnings are **stderr-only** — absent from md/json, invisible to CI                                                                     | `cmd/archfit/pipeline.go` + outputs               |
| 7   | `encapsulation` always n/a for Go (modules omit `owner` → no cross-boundary edges) → `boundary_integrity` frozen at fixed 60                                                            | `internal/score/score.go` boundaryIntegrity       |
| 8   | `architecture_fitness` counts `pkg/mod/golang.org/x/tools@…/archive_test.go` (module cache) as an arch test; scores small healthy repos 0/critical                                      | `internal/fitness/fitness.go` detectArchTestFiles |
| 9   | `score --full` / `scorecard --full` reject the flag (rc=3 on all 5 repos)                                                                                                               | `cmd/archfit/score.go`                            |
| 10  | `review` overflow: `reviewMaxFindings=80` etc. declared but **not enforced** in `buildReviewPrompt`; `reviewMaxTokens=8192` → truncated JSON ("unexpected end of JSON input") on ccgram | `cmd/archfit/review.go:374`                       |

Architecture ring constraints to preserve (`internal/arch_test.go`, `TestArchImports`):

- Core ring (`classify`, `rules`, `metrics/**`, `status`, `staleness`, `facts`, `score`,
  `scope`) must not import `os`/`os/exec`/YAML/adapter packages/`internal/llm`. Fixes
  there decide over facts already in the `Diagnostic`.
- `internal/model/**` = stdlib only (safe to add `CoverageGap` struct + fields).
- Every subprocess goes through `toolrun.Runner`. LLM only in `cmd/`. Coverage-gap
  population, gate enforcement, `.env` loading, autopilot, and config writing live in
  `cmd/` (they touch config/YAML/filesystem) — never the core ring.

Locked design decisions (approved):

1. **Missing-tool default = warn-loud + opt-in block.** n/a (never green/strong) +
   coverage-gaps block; exit 0 by default; hard-fail only via `gate: fail` / `--require-tools`.
2. **LLM scope = maximal autopilot** (`enrich --owner/--volatility`, `.env` autoload, new
   `archfit autopilot` full-config drafter — review-only, never auto-applied).
3. **Gate posture for the 5 repos = promote high-confidence rules to `fail`** (cycles /
   forbidden-dependency / layer-direction); keep noisy ones at `warn`. ccgram will FAIL.

## Development Approach

- **Testing approach**: Regular (code first, then tests), matching the repo's
  `go test -race` convention — tests are a **required deliverable of every task**.
- One logical unit per task; small focused changes; backward compatible (all new
  `Diagnostic`/config fields are additive + `omitempty`).
- **CRITICAL — determinism stays a first-class acceptance criterion** for any task that
  changes output: byte-identical double-run must hold; intended output changes bump the
  relevant `metric_version` and regenerate the golden (`internal/engine/golden_test.go`)
  **deliberately**, inspecting the diff — never auto-accept.
- **CRITICAL — every task ends with `make test` + `make lint` clean** before the next.
  `internal/arch_test.go` (ring invariants) and golden must stay green; re-run
  `TestArchImports` explicitly after Tasks 1, 5, 8, 9, 10.
- **Score drops are intended**: repos previously "strong" via false-green fall to
  n/a/mixed. Baselines are finding-level, not score-level, so `.archfit-baseline.json`
  files are unaffected — but note the shift in the final report.
- Update this plan as scope shifts (➕ new task, ⚠️ blocker).

## Testing Strategy

- **Unit tests**: required per task (table-driven where there's an input/output matrix).
- **Golden / determinism**: `internal/engine/golden_test.go` double-run must stay
  byte-identical; regenerate deliberately when output schema changes.
- **Ring invariants**: `go test ./internal/ -run TestArchImports` is a gate on every
  task that adds imports.
- **No e2e/UI** in this repo; the CLI behavioural checks live under `cmd/archfit`.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- Keep this plan in sync with actual work.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, docs, and config files this repo or the
  five target repos can change, plus archfit runs that capture reports.
- **Post-Completion** (no checkboxes): human review of LLM-drafted classifications,
  installing optional analyzers locally, and communicating the intended score drops.

## Implementation Steps

### Task 1: Kill the false-green coverage core

- [x] `internal/metrics/boundary/coverage.go` `CoverageMetric.Calculate`: when
      `len(in.ToolCoverage)==0` (no extractor contributed any coverage record) return
      `result.NACount(...)` (band `n/a`, confidence low) — keep `value=1.0` **only** when
      entries exist but `totalApplicable==0` (extractors ran, nothing applicable)
- [x] `internal/score/score.go`: change `couplingBalance(edges)` →
      `couplingBalance(edges, base Confidence)`; thread `base` from `Synthesize`; on empty
      edges with `base==ConfidenceLow` return {value 50, confidence low, summary "no edges
      classified; extraction coverage insufficient"}; keep existing behaviour when base ≥ medium.
      Make the evidence string always state the **classified-edge count** so a 90 is legible
      (OpenAI reproduced `coupling_balance: 90` on codegraph _with all tools and a real import
      cycle_ — a 90 with zero BC-advisory edges must read as "no unbalanced coupling among N
      edges", not a blanket "great")
- [x] write/adjust `coverage_test.go`: `TestCoverage_NoExtractors` (empty slice → n/a) and
      keep `TestCoverage_ZeroApplicable` (entry present, zero applicable → 1.0)
- [x] write `score_test.go` cases: empty edges + low base → 50/low; empty edges + medium
      base → unchanged 90/medium
- [x] `make test` (`./internal/metrics/boundary/... ./internal/score/...`) +
      `go test ./internal/ -run TestArchImports` + `make lint` — must pass before Task 2

### Task 2: Honest analysis_confidence + complete doctor

- [x] `internal/score/score.go` `analysisConfidence`: when the `coverage` metric band is
      `n/a`, start at 60 (not `pct(0)=0`) and penalize each absent **primary** extractor
      (go/packages, dependency-cruiser, grimp) −15 (cap 45) on top of existing semantic-tool
      penalties, so an all-absent repo lands ≈ 0/critical
- [x] `cmd/archfit/doctor.go`: add `lizard`, `jscpd`, `gitnexus` rows to the tool table
      with detection + a one-line install hint each
- [x] write `score_test.go` `TestAnalysisConfidence` cases: coverage n/a + all primary
      absent → critical; coverage ok + semantic absent → graded drop
- [x] build + run `./.bin/archfit doctor` and confirm the three new rows render
- [x] `make test` + `make lint` — must pass before Task 3

### Task 3: Surface coverage gaps + config warnings (warn-loud default)

- [x] `internal/model/diagnostic/diagnostic.go`: add `CoverageGap{Tool, InstallCmd,
AffectedMetrics []string, Gate string}` and fields `CoverageGaps []CoverageGap` +
      `ConfigWarnings []string` on `Diagnostic` (both `json:",omitempty"`, stdlib only)
- [x] `cmd/archfit/pipeline.go`: add a static `toolAffectedMetrics` table (tool → metrics + install cmd); after the engine returns, build `CoverageGaps` from `ToolCoverage`
      entries with `Status=="absent"`; fill `ConfigWarnings` from `cfg.Lint()`; replace the
      `_`-discarded loc/complexity/clones/gitnexus errors with `ConfigWarnings` entries +
      stderr lines (extractors degrade gracefully — never error today — so `noteToolErr`
      surfaces the exceptional case; the absent-coverage records drive `CoverageGaps`)
- [x] `internal/output/markdown/*`: add a `## Coverage gaps` section before findings
      (plus `## Config warnings` so the lint warnings reach md/json, closing defect #6)
- [x] `internal/output/scorecard/*`: add a `## Required tools missing` section after dimensions
- [x] write output tests asserting both sections render when gaps/warnings present and are
      omitted when empty; write a `diagnostic` JSON round-trip test for the new fields
- [x] `go test ./internal/model/diagnostic/... ./internal/output/...` +
      `go test ./internal/engine/ -run TestGolden` (double-run determinism; omitempty keeps
      clean fixtures unchanged) + `make lint` — must pass before Task 3b

### Task 3b: Built-in artifact/cache excludes + output-path-inside-root warning

> From OpenAI Sec 8 item 3 / Sec 7.4. The self-scan "instability" was reports written
> inside the analyzed repo, and `.gitnexus/run.cjs` showed up as a complexity hotspot —
> archfit shouldn't measure its own tool artifacts.

- [x] add a built-in default exclusion set (merged with, not replacing, config
      `exclusions`): `.archfit-cache/`, `.archfit-baseline.json`, `.gitnexus/`, `.codegraph/`,
      `reports/`, `.venv/`, `node_modules/`, `vendor/`, dist/build/generated outputs. Keep it
      in the core ring as a const list consumed by `internal/scope` (no I/O) so it stays pure
- [x] `cmd/archfit/pipeline.go`: when an output/report path resolves **inside** the analyzed
      root, emit a `ConfigWarnings` entry + stderr line ("output written inside analyzed root —
      exclude it or use a path outside `--root` to keep scans deterministic")
- [x] write a `scope` test (defaults exclude `.gitnexus`/`reports` unless re-included) and a
      cmd test for the inside-root warning
- [x] `go test ./internal/scope/... ./cmd/archfit/...` +
      `go test ./internal/engine/ -run TestGolden` + `make lint` — must pass before Task 3c

### Task 3c: Delta output bucketing

> From OpenAI Sec 8 item 4 — "current delta can look like full-current output for some repos."

- [x] group delta-mode findings into `new` / `existing` / `resolved` / `severity_changed` /
      `touched_by_delta` buckets, reusing the lifecycle tags `internal/status` already assigns
      (add `severity_changed` detection there if absent — pure, in-ring)
- [x] render the buckets in `internal/output/{markdown,scorecard}` and add the grouping to the
      JSON delta payload (additive, omitempty)
- [x] write `status` + output tests covering each bucket and an all-empty delta
- [x] `go test ./internal/status/... ./internal/output/...` +
      `go test ./internal/engine/ -run TestGolden` + `make lint` — must pass before Task 4

### Task 4: Opt-in hard gate for missing required tools

- [ ] `internal/config/config.go`: add `GateMode` (`off`/`warn`/`fail`, default warn) and a
      `Gate GateMode` field on `ToolConfig`; validate the enum
- [ ] `cmd/archfit/check.go`: add `--require-tools` flag to check/scan
- [ ] `cmd/archfit/pipeline.go`: compute each gap's effective gate (config `gate:` or
      `--require-tools`); if any is `fail`, return `exitError{code:1}` (policy violation —
      **not** the code-3 tool-error) and stamp the verdict `fail`
- [ ] write `config_test.go` for `GateMode` parse/validate; write a pipeline/cmd test: a
      non-Go dir with `tools.go.gate: fail` exits 1, default exits 0 with a gap entry
- [ ] `go test ./internal/config/... ./cmd/archfit/...` +
      `go test ./internal/ -run TestArchImports` + `make lint` — must pass before Task 5

### Task 5: Scoring reliability fixes

> **Two distinct `architecture_fitness` problems — don't conflate.** (a) The `pkg/mod`
> false positive (a module-cache `_test.go` counted as an arch test) was reproduced by the
> Claude run; the OpenAI run did not hit it but does not dispute it — the path filter below
> still applies. (b) `architecture_fitness: 0` on pumba/spotinfo is **correct** (those repos
> genuinely have no enforcement signals), not a false positive — the n/a calibration below
> only changes the "scan didn't run" case, never the "ran, found 0/3" case.

- [ ] `internal/fitness/fitness.go` `detectArchTestFiles`: skip `<root>/pkg/mod/**` (Go
      module cache) and `**/testdata/**`; keep existing vendor/node_modules/hidden skips
- [ ] `internal/score/score.go` `architectureFitness`: when the metric is n/a, return
      {value 40, confidence low} (poor — "scan didn't run") instead of {10, critical};
      keep critical only when the metric ran and found 0/3
- [ ] `internal/score/score.go` `boundaryIntegrity`: when encapsulation is n/a, add an
      explicit evidence note (don't fabricate the value)
- [ ] write `fitness_test.go` case: a `pkg/mod/.../archive_test.go` and a `testdata/x_test.go`
      present are **not** counted; write `score_test.go` for the n/a fitness calibration
- [ ] `go test ./internal/fitness/... ./internal/score/...` +
      `go test ./internal/engine/ -run TestGolden` (regenerate golden deliberately, inspect
      the architecture_fitness diff) + `make lint` — must pass before Task 5b

### Task 5b: Role-aware module annotation

> From OpenAI Sec 8 item 2 / Sec 7.1. In a one-binary Go CLI, `cmd` fan-out is
> composition-root cohesion, not high-distance coupling — archfit currently penalizes it,
> producing false-positive BC advisories. A declared role lets distance classification treat
> wiring/generated/test modules correctly.

- [ ] `internal/config/config.go`: add a `Role` field to `ModuleDef`
      (`composition_root|adapter|core|shared_model|generated|test`, optional, validated enum);
      surface it on the classify config view
- [ ] `internal/classify` / distance scoring: when the source module is `composition_root`
      (or `generated`/`test`), do not score its outbound edges as high-distance/unbalanced —
      suppress or downgrade the BC advisory; document the rule. Keep it pure (role arrives via
      the config view, not I/O)
- [ ] `enrich`/`autopilot` (Task 7): include a suggested `role` per module in drafts so the
      LLM-authored config sets composition roots automatically
- [ ] write classify/metrics tests: a `composition_root` module's fan-out is not flagged;
      a `core`→`core` unbalanced edge still is
- [ ] `go test ./internal/config/... ./internal/classify/... ./internal/metrics/...` +
      `go test ./internal/ -run TestArchImports` + `make lint` — must pass before Task 6

### Task 6: CLI score --full + review robustness

> **Partly done (uncommitted, 2026-06-20).** `cmd/archfit/review.go` already enforces
> the `reviewMaxFindings`/`reviewMaxFindingTypes`/`reviewMaxModuleFacts`/`reviewMaxMetrics`/
> `reviewMaxDynamicFacts` caps in `buildReviewPrompt` (count-cap + ranked-by-fan-in/out/LOC,
> not the byte-budget originally specified — keep the count-cap approach), extracts JSON via
> `reviewJSONPayload` (first `{` → last `}`), gives a truncation hint, raises `MaxTokens` to
> `reviewMaxTokens=8192`, and adds `postVerify` to drop hallucinated claims. The OpenAI
> after-fix rerun confirms `archfit review` now exits 0 on all five repos (ccgram included).
> The remaining items below are the _only_ open work in this task.

- [ ] `cmd/archfit/score.go`: add `Full bool` to `ScoreCmd`, forward to the wrapped
      `CheckCmd` (`Full: c.Full || c.Base == ""`); confirm the `scorecard` path accepts it.
      **Still broken** — `ScoreCmd` has no `Full` field, so `score --full` fails kong parse
      (Claude reproduced rc=3; the OpenAI harness sidestepped it via `check --full`)
- [ ] `cmd/archfit/review.go`: persist the raw LLM response to a debug file (e.g.
      `.archfit-cache/llm/last-review.txt`) before parsing, so truncation/parse failures are
      diagnosable (OpenAI Sec 8 hardening item)
- [ ] (optional, if the SDK exposes it cleanly) constrain the review output with the
      provider-native JSON-schema / structured-output mode instead of post-hoc parsing only
- [ ] write `cmd/archfit` tests: `score --full` parses (rc 0); a synthetic large diagnostic
      keeps `buildReviewPrompt` within the caps; truncated-JSON repair recovers a valid payload
- [ ] build + `./.bin/archfit score --full -c .archfit.yaml` returns rc 0;
      `make test` + `make lint` — must pass before Task 6b

### Task 6b: --root flag separate from --config

> From OpenAI Sec 8 item 8 / Sec 11. Today the config directory is treated as the repo root,
> so external CI harnesses must plant a temporary config inside the analyzed repo. Decouple
> the two.

- [ ] add a `--root` flag (default: cwd, or the directory of `--config` for backward compat
      when `--root` is omitted) to check/scan/score/scorecard; resolve the scan root from
      `--root` and the config from `--config` independently
- [ ] thread the resolved root into `scope.Root` instead of deriving it from the config path;
      keep the no-`--root` behaviour identical to today (no breakage)
- [ ] write a cmd test: `--root <repo>` + `--config <elsewhere>/.archfit.yaml` scans the repo
      using the external config; omitting `--root` keeps current behaviour
- [ ] `go test ./cmd/archfit/...` + `go test ./internal/ -run TestArchImports` + `make lint`
      — must pass before Task 7

### Task 7: LLM self-driving — enrich owners/volatility, autopilot, .env

- [ ] `internal/initcfg/*`: add `OwnerDraft`/`VolatilityDraft` structs + load/save mirroring
      the existing subdomain draft+pin pattern (additive YAML merge into `.archfit.yaml`)
- [ ] `cmd/archfit/enrich.go`: add `--owner` and `--volatility` modes — draft via LLM from
      CODEOWNERS + git churn into `.archfit-owners.yaml` / `.archfit-volatility.yaml`; `--pin`
      writes approved entries into `modules.<name>` (never removes existing fields)
- [ ] `cmd/archfit/autopilot.go` (new) + wire into the kong CLI: one-shot scan → LLM-draft a
      full `.archfit.yaml` (modules, layers, owners, volatility, gates) to a review file;
      **never** auto-apply; reuse the pipeline capture + provider plumbing
- [ ] `cmd/archfit/main.go`: optional `.env` autoload at startup — set a key only when
      `os.Getenv(key)==""` (real env / CI secrets always win); add `.env` to `.gitignore`
- [ ] write `internal/initcfg` tests for owner/volatility draft+pin; write a cmd test that
      `autopilot` writes a draft and applies nothing
- [ ] `go test ./internal/initcfg/... ./cmd/archfit/...` +
      `go test ./internal/ -run TestArchImports` (llm stays cmd-only) + `make lint`;
      `./.bin/archfit enrich --help` and `./.bin/archfit autopilot --help` render — before Task 8

### Task 8: Docs + shipped skill

- [ ] new `docs/design/coverage-gate-and-autopilot-v0.1.md`: fail-loud mechanism, the
      coverage-gap model, `GateMode` semantics, role-aware modules (Task 5b), default excludes
      (Task 3b), delta bucketing (Task 3c), and autopilot review-only safety
- [ ] update `docs/guide/{commands,configuration-reference,llm-enrich,ci,metrics,troubleshooting}.md`
      for `--require-tools`, `tools.<x>.gate`, coverage-gaps output, `enrich --owner/--volatility`,
      `autopilot`, `.env`, `--root` (Task 6b), module `role:` (Task 5b), built-in excludes
      (Task 3b), and delta buckets (Task 3c)
- [ ] update `skills/archfit/SKILL.md` + `references/*` so the **shipped skill works
      standalone** (without the architect skill): coverage-gap interpretation, gate-promotion
      guidance, autopilot draft→review flow
- [ ] verify markdown lint clean, SKILL line counts within limits, cross-refs resolve
- [ ] `make lint` (+ docs link/markdown checks if defined) — must pass before Task 9

### Task 9: Self-config, deliberate golden regen, full gate green

- [ ] update archfit's own `.archfit.yaml` for the new gate posture if warranted
      (promote high-confidence rules to `fail`); keep dogfooding green or honestly failing
- [ ] fill `owner` (+ `subdomain`/`volatility` where missing) on archfit's own modules —
      the OpenAI self-run shows all 24 `internal/*` packages omit `owner`, so the self-dogfood
      currently floods config-quality warnings and leaves `encapsulation` n/a; use Task 7's
      `enrich --owner/--volatility` so the self-scan exercises the fixed measurable path
- [ ] regenerate the golden fixtures deliberately and inspect the full diff
- [ ] `make all` (fmt → lint → test → build) green; `go test ./internal/ -run TestArchImports`
- [ ] `make test` + `make lint` — must pass before Task 10

### Task 10: Five-repo config authoring + full/delta verification runs

- [ ] for each of `~/workspace/{archfit,spotinfo,pumba,ccgram,codegraph}`: author/refresh a
      proper `.archfit.yaml` informed by project design + module layout + git history — use
      `autopilot` / `enrich --owner --subdomains --volatility` to draft, then human-review
      (see Post-Completion), and promote cycle/forbidden-dep/layer-direction rules to `fail`
- [ ] per-repo specifics: ccgram set `python_package` + expect FAIL on 3 cycles; codegraph
      add `no-import-cycles` rule + install `node_modules` for SCIP; pumba/spotinfo declare
      owners+volatility on under-specified modules
- [ ] run `archfit doctor`, `check --full`, `check --base <default-branch>` (delta), `score`,
      and `scan` per repo; capture stdout+json into
      `reports/archfit-vs-architect-20260620-postfix/<repo>/` — write reports **outside** each
      analyzed repo (the OpenAI run showed the earlier self-scan "instability" was reports
      written _inside_ the repo being scanned; output-path-inside-root is also flagged by
      [3b]). The OpenAI all-tools rerun reports `same_hash=true` on all five repos, so a clean
      output path is the only determinism prerequisite
- [ ] write a short comparison (old vs new scorecards) asserting: no strong-on-thin-evidence,
      coverage-gaps surfaced for genuinely-missing tools, ccgram fails on cycles, exit codes meaningful
- [ ] run the negative case: fixed binary on a non-Go dir in `auto` mode → structural
      dimensions n/a (not strong), coverage-gaps lists go/packages + install hint, exit 0;
      with `--require-tools` → exit 1

### Task 11: Verify acceptance criteria

- [ ] verify every Overview outcome is implemented (no false-green path remains; gaps in
      md+json+stderr; opt-in hard gate; owners/volatility authoring; autopilot; review no overflow)
- [ ] `make all` green; `go test ./internal/ -run 'TestArchImports'`; golden double-run byte-identical
- [ ] confirm test coverage holds to the repo standard for changed packages

## Deferred / follow-on (not in this plan)

Surfaced by the OpenAI study but out of scope here — track as separate efforts:

- **Functional coupling beyond imports** (OpenAI Sec 8 item 6): detect co-evolving strings
  and generated contracts across docs, tool descriptions, tests, prompt templates, and
  snapshots. A new evidence source; needs its own design.
- **Operational / toolchain evidence** (Sec 8 item 7): optional checks that CI actually runs
  the arch gate, plus Kubernetes/Helm/Terraform drift and Docker topology — the architect
  skill caught pumba deploy-manifest concerns archfit can't see from the source graph.
- **Stable cross-report finding IDs** (Sec 9 item 6): durable IDs so report-to-report delta
  and architect-skill handoff line up — partly an architect-side concern.

## Technical Details

- **CoverageGap model** (stdlib-only): `{Tool, InstallCmd, AffectedMetrics []string,
Gate string}`; populated in `cmd/` from `Diagnostic.ToolCoverage` + a static
  tool→metrics map. Core ring never sees config or tool names beyond coverage facts.
- **Gate decision layering**: the core ring computes bands/confidence from facts; `cmd/`
  reads `tools.<x>.gate` + `--require-tools` and decides the exit code. Missing-required-tool
  = exit 1 (policy), distinct from exit 3 (tool/config error).
- **LLM authoring through-line**: filling `owner`/`subdomain`/`volatility` makes distance
  classification work → `encapsulation` becomes measurable → `boundary_integrity` and
  `coupling_balance` stop being n/a, and the "N modules under-specified" warnings clear.
- **Determinism**: any output schema change bumps the relevant `metric_version` and
  regenerates `internal/engine/golden_test.go` deliberately.

## Post-Completion

_Items requiring human intervention or external systems — informational, no checkboxes._

**Manual review (required before pinning LLM output):**

- Review every `enrich --owner/--volatility` and `autopilot` draft before `--pin`/apply —
  LLM classifications are proposals, never auto-trusted (matches the existing `--apply`
  safety rule in `skills/archfit/SKILL.md`).

**Local environment:**

- Install optional analyzers for full coverage on the target repos: `lizard`, `jscpd`,
  `dependency-cruiser`, `grimp`, `gitnexus`, plus `node_modules` in codegraph for SCIP.
- Ensure `ANTHROPIC_API_KEY` is available (`.env` autoload or shell/op) for the LLM tasks.

**Communicate:**

- Scorecards for the five repos will drop where they were falsely "strong" — this is the
  intended correctness fix, not a regression. Baselines are unaffected (finding-level).
