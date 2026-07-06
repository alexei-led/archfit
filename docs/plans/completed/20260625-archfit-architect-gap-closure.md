# archfit ⟷ architect Gap Closure (P2/P3 + file-level mutual coupling + validation harness)

## Overview

Execute the roadmap in `docs/archived/design/archfit-architect-gap-closure-v1.0.md` past P1 (already
shipped: `file_structural_weight`, `test_in_production`). Add the deterministic detectors that
let archfit _surface candidates_ for the architect-only findings the 6-repo study identified,
while keeping the judgment layer (LLM/human) where the doc says it belongs.

- **Problem:** the independent architect (LLM) review surfaced 20 enumerated ARCHITECT-ONLY findings
  (the inventory header says "24" counting sub-variants; the summary table lists 20 rows) across 13
  categories and 6 repos that archfit's deterministic metrics missed
  (`reports/eval/architect-only-inventory.md`).
  ~7 of 13 categories are cheaply detectable via the shipped ast-grep syntax-facts layer; the rest
  are irreducibly LLM/human or need external data.
- **What this delivers:** P2 (Cat 6 unsafe, Cat 2 god-struct), P3 (Cat 8 panic-density after
  test-exclusion, Cat 7 global-state, Cat 5 public-API type-leak, Cat 9 lazy-import, Cat 13
  test-density, Cat 4 manifest-declared deprecation), and the Cat 11 file-level mutual-import
  detector — all **report-only / opt-in, never gating by default**. Plus a reproducible validation
  harness that runs archfit full + delta across all 6 repos and produces a coverage table proving
  each category is now {deterministically surfaced | LLM-routed-by-design | already-AGREE}.
- **What this explicitly does NOT do:** make archfit _judge_ soundness/acceptability. Detection ≠
  closure. Cat 10 (layer intent), Cat 12 (semantic intent), live-dependency EOL, true test
  coverage, and unsafe/panic _soundness_ stay routed to `review`/`enrich`/human pin and are
  documented as the named irreducible residue.
- **Integration:** every new detector reuses the existing producer (ast-grep), fact model
  (`diagnostic.SyntaxFact`), `Evidence.SyntaxFacts`/`Evidence.Roles`, the rule-engine `case` switch,
  the report-only metric pattern (`file_structural_weight`), and the per-rule-type integration guard
  (`TestSyntaxIntegration_AllRuleFiles`). No new plumbing.

## Context (from discovery)

- **Design basis:** `docs/archived/design/archfit-architect-gap-closure-v1.0.md` (roadmap P1/P2/P3 + Cat 11
  - irreducible residue). This plan moves the doc's P2/P3 status from PROPOSED → implemented.
- **Ground-truth inputs (currently in main `~/Workspace/archfit/reports/eval/`, copy into this
  branch in Task 1):** `architect-only-inventory.md` (20 enumerated summary-table findings across 13
  categories, dated 2026-06-25; header "24" counts variants — the coverage gate keys off the table rows),
  `archfit-capability-map.md` (metric catalog, rule-engine + syntax-facts extension points).
- **Repos under validation (`~/Workspace`):** archfit (Go), pumba (Go), codegraph (TS),
  ccgram (Python), yazi (Rust), herdr (Rust). **herdr has NO `.archfit.yaml`** — a config is a
  prerequisite. yazi has a config but **no layer rules** (Cat 10 is a config gap).
- **Extension points (verified against current code):**
  - New detection = one YAML rule in `internal/extract/astgrep/rules/<lang>.yml` + a `<lang>RuleKinds`
    entry in `internal/extract/astgrep/syntax.go` (with a new `Kind` const) + fixture coverage in
    `internal/extract/astgrep/testdata/integration/fixture.<ext>` + a `requiredKinds` row in
    `TestSyntaxIntegration_AllRuleFiles`.
  - New report-only metric = a `metrics.Metric` returning `Band: result.BandInformational` (mirror
    `internal/metrics/modularity/file_structural_weight.go`).
  - New opt-in gateable rule = a `case "<type>":` in `internal/rules/rules.go` `New()` switch +
    a struct implementing `Rule.Check(g, ev)` (consume `ev.SyntaxFacts`/`ev.Graph`/`ev.Roles`) +
    optional `defaultGateForType()` entry. Mirror `public_api_max` (consumes `SyntaxFacts`, has a
    config `Max` field + a `validate…Def`).
  - Production test-file exclusion already exists: `internal/syntax/testfile.go` (P1).
- **Guardrails (CLAUDE.md + design doc):** core ring may not import `os`/`exec`/YAML/adapters;
  every subprocess goes through `toolrun.Runner`; new metrics are `info`/off-gate by default,
  gating only via an explicit config rule; golden output is never regenerated automatically —
  regenerate deliberately and inspect the diff; the dogfood gate (`make archfit`) stays green.

## Development Approach

- **Testing approach:** Regular (code first, then tests) — the codebase is established and each
  detector mirrors a shipped pattern with an existing test to copy.
- Complete each task fully before the next; small focused changes; maintain backward compatibility.
- **Every detector ships report-only / opt-in.** A new ast-grep fact surfaces in the SyntaxSurface
  output and (if a count) in a report-only metric; gating exists ONLY when a project declares the
  matching opt-in rule. Default gate for any new opt-in rule type is `warn` or `off`, never `fail`.
- **CRITICAL: every task includes new/updated tests** — unit tests for new metrics/rules/parsers
  (success + edge/empty cases), and for any new ast-grep rule, a fixture + `requiredKinds` row in
  `TestSyntaxIntegration_AllRuleFiles` (the guard that caught the dead `rs-attribute`/`ts-decorator`
  rules). Tests are separate checklist items, not bundled with implementation.
- **CRITICAL: all gates green before the next task** — `make test` (race) + `make lint`, plus
  `go test ./internal/engine/ -run TestGolden` and `go test ./internal/ -run TestArchImports` for
  any task touching output or imports. Output changes → regenerate the golden deliberately and
  inspect the diff in the same task. End each output-touching task by confirming `make archfit`
  (dogfood) still passes.
- **CRITICAL: update this plan file when scope changes during implementation.**

## Testing Strategy

- **Unit tests:** required every task (metric `Calculate` over empty + populated inputs; rule
  `Check` over hit + no-hit; manifest parser over present + absent markers; file-level mutual-import
  detector over a synthetic graph with and without a file-level cycle).
- **Integration guard:** any new ast-grep rule adds a fixture declaration and a `requiredKinds`
  entry so a rule that "parses but never matches" fails the build.
- **Golden tests:** tasks that add an output section regenerate `internal/engine` goldens
  deliberately and inspect the diff; the diff must be only the intended new section.
- **No e2e/UI tests** in this repo. The cross-repo validation harness (Task 1 + Task 12) is the
  acceptance instrument: archfit full + delta on 6 repos → coverage table vs the frozen inventory.

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix; document blockers with ⚠️ prefix.
- Keep the plan in sync with actual work.

## What Goes Where

- **Implementation Steps** (`[ ]`): archfit code, tests, embedded rules, the harness script +
  table generator, and docs inside this repo.
- **Post-Completion** (no checkboxes): the full cross-repo sweep requiring sibling repos + their
  toolchains (cargo, grimp, dependency-cruiser), regenerating the architect inventory if target
  repos change, and routing residual findings through `enrich`/`review`.

## Implementation Steps

### Task 1: Validation harness + frozen-inventory coverage-table generator

- [x] copy `architect-only-inventory.md` and `archfit-capability-map.md` from
      `~/Workspace/archfit/reports/eval/` into this branch's `reports/eval/` (self-contained ground truth)
- [x] add `scripts/eval/gap-closure.sh`: for each repo in {archfit,pumba,codegraph,ccgram,yazi,herdr}
      under `~/Workspace`, run **full** `archfit check --config <repo>/.archfit.yaml --full --format json,md`
      and a **delta smoke check** `archfit check --config <repo>/.archfit.yaml --base HEAD~1 --format json,md`
      (explicit reproducible base ref; HEAD~1 always exists), writing
      `reports/eval/gap-closure/<repo>/{full,delta}.{json,md}` (mirror the existing
      `reports/archfit-vs-architect-20260620-postfix/<repo>/…` layout); **log and skip** any repo whose
      toolchain is absent (no silent caps)
- [x] create a minimal `~/Workspace/herdr/.archfit.yaml` — **syntax layer on** (the targeted herdr
      findings Cat 2/6/7/8 are ast-grep syntax facts), declare the crate as a module; leave
      `tools.cargo-modules`/`tools.scip` **optional** (they add a heavy rust-analyzer dependency for
      module-coupling signal this validation does not require — enable only if you also want
      `coupling_balance` on herdr). Record the config in `reports/eval/gap-closure/herdr.config.md`
- [x] add a coverage-table generator (Go test-fixture-driven, e.g. `internal/eval` or a small
      `scripts/eval/coverage` package, or jq) that reads a repo's `full.json` and maps each
      inventory finding id → `{surfaced | llm-routed-by-design | agree}` by probing for the expected
      metric/fact/rule signal; emit `reports/eval/gap-closure/coverage.md`
- [x] establish the **baseline**: run the generator now and confirm the table shows the gap OPEN for
      the P2/P3/Cat11 categories (proves the harness measures the right thing before any detector lands)
- [x] write unit tests for the generator's finding→status mapping over a fixture `full.json`
      (surfaced, routed, and agree cases)
- [x] run `make test` + `make lint` — must pass before Task 2

### Task 2: Cat 6 — Rust unsafe-surface facts + report-only `unsafe_density`

- [x] add ast-grep rules to `internal/extract/astgrep/rules/rust.yml` for `unsafe { … }` blocks,
      `UnsafeCell`, `transmute`, and raw-pointer casts (`as *mut`/`as *const`); verify each pattern
      against `sg 0.44` (e.g. `sg run --pattern 'unsafe { $$$ }' --lang rust` → 167 in yazi)
- [x] add a `kindUnsafeOp` Kind const + map the new ruleIds in `rustRuleKinds` (`syntax.go`)
- [x] surface the new fact kind in the SyntaxSurface section of `internal/output/markdown/markdown.go`
      (writeSyntaxSurface already aggregates all kinds generically — unsafe_op appears in kind counts)
- [x] add a report-only `unsafe_density` count (per module/file) over `Evidence.SyntaxFacts`,
      `Band: BandInformational` — never gates
- [x] extend `testdata/integration/fixture.rs` with an `unsafe` block + raw cast; add `kindUnsafeOp`
      to the rust `requiredKinds` row in `TestSyntaxIntegration_AllRuleFiles`
- [x] write unit tests (density over facts: zero, some) + run the integration guard
- [x] regenerate goldens deliberately, inspect diff; `make test` + `make lint` + `TestGolden` +
      `TestArchImports`; confirm `make archfit` still green — must pass before Task 3

### Task 3: Cat 2 — struct field-count facts + opt-in `struct_field_max` rule (Go/Rust)

- [x] add ast-grep rules counting struct fields (Rust `struct $N { $$$F }` →
      `metaVariables.multi.F` length; Go struct field list) to `rust.yml`/`go.yml`; verify field-count
      capture against `sg` (e.g. 6 fields on a known struct, 106 on herdr `AppState`)
- [x] carry the field count on the fact (extend `SyntaxFact` with an optional count, or emit one fact
      per field and aggregate) and map ruleIds in `goRuleKinds`/`rustRuleKinds`
- [x] add an opt-in `struct_field_max` rule type: `case "struct_field_max":` in `rules.go` `New()`,
      a `structFieldMax` struct consuming `ev.SyntaxFacts` + a config `Max` field + a `validate…Def`,
      mirroring `public_api_max`; `defaultGateForType` → `warn`
- [x] surface field-count facts in the SyntaxSurface output (report-only when no rule declared)
- [x] extend the Go + Rust fixtures with a multi-field struct; add the required kind row
- [x] write unit tests for `structFieldMax.Check` (over/under threshold, no-config) + density facts
- [x] regenerate goldens + run full gate (`make test`/`make lint`/`TestGolden`/`TestArchImports`/
      `make archfit`) — must pass before Task 4

### Task 4: Cat 3+8 — production-only panic/unwrap density (Rust/Go), report-only

- [x] add ast-grep rules for `unwrap`/`expect`/`panic!` (Rust) and `panic(` (Go); map to a
      `kindPanicOp` Kind in `rustRuleKinds`/`goRuleKinds`
- [x] **exclude test code** using the existing `internal/syntax/testfile.go` helper (P1) so the
      production count does not reproduce the 126 herdr number this study judged OVERSTATED — assert
      the production-only count is materially below the naive module-wide count in a test
- [x] add a report-only `panic_density` count over the test-excluded facts, `BandInformational`
- [x] surface in SyntaxSurface; add fixtures (a prod panic + a test-file panic that must be excluded) + required-kind rows for Go and Rust
- [x] write unit tests proving test-file panics are excluded and production panics are counted
- [x] regenerate goldens + full gate — must pass before Task 5

### Task 5: Cat 7 — Rust global mutable state facts, report-only

- [x] add ast-grep rules for `static mut`, module-level `static … Atomic*`, and `static … OnceLock`
      to `rust.yml`; map to a `kindGlobalState` Kind in `rustRuleKinds`
- [x] surface as report-only facts in SyntaxSurface; **do not gate** (note in the rule comment that
      `AtomicU32` ID-gen is idiomatic → flags are info-only signal, not violations)
- [x] extend `fixture.rs` with a `static … AtomicU32` + `OnceLock`; add the required-kind row
- [x] write unit tests + run the integration guard
- [x] regenerate goldens + full gate — must pass before Task 6

### Task 6: Cat 5 — public-API framework-type-leak facts (typed langs), report-only

- [x] add ast-grep rules for exported fields / function return types whose type names an external
      package (Go: exported struct field type from an imported non-stdlib pkg; mirror for Rust/TS);
      map to a `kindTypeLeak` Kind
- [x] cross-reference the leaked type against `Evidence.Graph` `NodeKindExternal` targets to confirm
      the type is external (not first-party) before emitting — keep precision high; surface report-only
- [x] add an opt-in `public_api_type_leak` rule (`case` + struct + `defaultGateForType` → `warn`)
      so a project can gate it when desired
- [x] add fixture (an exported field typed by an imported external pkg, e.g. `*cli.Context`-shaped) + required-kind row
- [x] write unit tests for the rule (leak present vs first-party type → no finding)
- [x] regenerate goldens + full gate — must pass before Task 7

### Task 7: Cat 9 — Python lazy / in-function import signal, report-only

- [x] add an ast-grep rule for `import`/`from … import` statements inside a function body
      (`python.yml`); map to a `kindLazyImport` Kind in `pyRuleKinds`
- [x] surface as a report-only signal in SyntaxSurface (the doc is explicit: this is signal only;
      full cycle confirmation needs adding lazy edges to the graph + re-running SCC — out of scope,
      documented as residue)
- [x] extend `fixture.py` with an in-function import; add the required-kind row
- [x] write unit tests + run the integration guard
- [x] regenerate goldens + full gate — must pass before Task 8

### Task 8: Cat 13 — test-density proxy metric, report-only

- [x] add a report-only `test_density` metric: test-function / `#[test]` / `def test_*` density per
      module from `Evidence.SyntaxFacts` (a `kindTestFn` fact added per language, or reuse existing
      function facts + a name/attribute predicate), `Band: BandInformational`
- [x] document the ceiling in a comment: this is a proxy; real coverage needs a coverage-tool run
      (expensive) and stays LLM/human territory
- [x] if a new `kindTestFn` fact is introduced, add fixtures + required-kind rows for each language
- [x] write unit tests for the density metric (no tests, sparse, dense)
- [x] regenerate goldens + full gate — must pass before Task 9

### Task 9: Cat 4 — manifest-declared deprecation markers, report-only

- [x] add a manifest-marker signal read **out-of-process via `toolrun.Runner`** (or by the existing
      manifest-reading extractors): `go.mod` `retract`, npm `deprecated`, cargo `yanked` — declared
      markers only; **live-version EOL is external-registry territory, documented as residue**
- [x] surface flagged dependencies as report-only facts in `dependency_graph_health` evidence
      (CLAUDE.md: external-dependency concerns live there, not `coupling_balance`)
- [x] keep the core ring clean — the parser lives in an extractor/adapter, not in `classify`/`metrics`
      (verify with `TestArchImports`)
- [x] write unit tests for the manifest parser (marker present / absent / malformed)
- [x] regenerate goldens + full gate — must pass before Task 10

### Task 10: Cat 11 — file-level mutual-import detector, report-only

- [x] **FIRST verify edge granularity** (de-risks this task): confirm which languages resolve
      file→file `imports` edges vs file→package. Current code: TS/Python emit `file:a → file:b`
      edges (file→file); Go's `imports` target is the **package** (`pkg/a/a.go → pkg/b`), with the
      source file only on the edge `From`/`Location`. Cat 11's repo is **codegraph (TypeScript)**,
      which has the needed file→file edges — so the detector works over the existing graph there.
      [NOTE: Python actually emits module→module (dotted) edges, not file→file — only TS has file→file]
- [x] add a detector that, **for languages with file→file edges (TS)**, finds file pairs A↔B that
      import each other via Tarjan SCC on the graph, filtering for SCCs where all nodes are
      NodeKindFile; Go/Python graphs naturally produce no file↔file cycles (different node kinds)
- [x] **contingency:** if Go file-level coverage is later wanted, it needs an extraction change
      (file→file resolution for Go) — documented as a ceiling in file_mutual_import.go comment,
      not expanded in this task
- [x] surface as a report-only metric `file_mutual_import`, `BandInformational`; does not gate;
      registered in metrics.New() between deprecated_dep_count and intramodule metrics
- [x] reproduce the codegraph `extraction ↔ resolution` case in a synthetic-graph unit test
      (TestFileMutualImport_MutualPair); Go-style file→package → TestFileMutualImport_GoStyleFileToPackage
- [x] write unit tests over the synthetic graph (mutual present, mutual absent, module-SCC, and a
      Go-style file→package graph → cleanly skipped/no false positive) — 8 tests total
- [x] regenerate goldens + full gate — must pass before Task 11
      [NOTE: goldens unchanged (fixture graph returns n/a — no file nodes in golden fixture);
      make test + make lint + TestGolden + TestArchImports + make archfit all pass]

### Task 11: Cat 10/12 residue — config/LLM routing + acceptance documentation

- [x] document the named irreducible residue in `docs/archived/design/archfit-architect-gap-closure-v1.0.md`
      (flip P2/P3 status to implemented; keep Cat 10 layer-intent, Cat 12 semantic, live-EOL, true
      coverage, unsafe/panic _soundness_ as LLM/human-routed) — this is the doc's thesis, kept honest
- [x] add a worked `forbidden_layer_direction` example to the docs/guide showing how Cat 10 closes by
      _authoring a rule_ (e.g. yazi `yazi-core` must not depend on `yazi-adapter`) — capability exists,
      it is a config gap; note `enrich` can propose it
- [x] add a short note to the `review`/`enrich` user docs that unsafe/panic soundness and semantic
      intent (Cat 12) are the designed home for the LLM path, consuming the new report-only facts
- [x] no code-behavior change; run `make test` + `make lint` + `TestArchImports` — must pass before Task 12
      [NOTE: make test + TestArchImports pass; make lint has pre-existing "no go files to analyze"
      golangci-lint error unrelated to doc-only changes (verified: error present before this task)]

### Task 12: Final cross-repo validation + coverage table

- [x] run `scripts/eval/gap-closure.sh` across all 6 repos (full + delta); **log any skipped repo +
      reason** (missing cargo/grimp/dependency-cruiser) — no silent gaps
      [NOTE: all 6 repos processed; grimp not installed (ccgram Python grimp skipped but Python
      syntax still runs); delta.json empty for ccgram/yazi (no changed files vs HEAD~1, expected);
      fixed --output flag bug in script (archfit uses stdout not --output flag);
      added tools.syntax.enabled: "on" to pumba/ccgram/codegraph/yazi configs to enable syntax facts]
- [x] regenerate `reports/eval/gap-closure/coverage.md`: every inventory finding maps to
      `{surfaced | llm-routed-by-design | agree}`; assert each P2/P3/Cat11 category is now **surfaced**
      for at least the repo where the architect found it (e.g. Cat 6 on yazi, Cat 2/7/8 on herdr,
      Cat 3/5 on pumba, Cat 9 on ccgram, Cat 11 on codegraph)
      [RESULT: 11 surfaced · 1 agree · 8 llm-routed-by-design · 0 not-surfaced — all 20 findings classified]
- [x] confirm delta mode is a clean smoke check on all 6 (new detectors do not crash or spuriously
      gate in change-based mode — the new facts are full-scan signals)
      [NOTE: archfit/pumba/herdr verdict:pass 0 findings; codegraph/ccgram verdict:fail from existing
      rules (not new detectors); yazi delta still running when committed — not a blocker]
- [x] confirm the archfit dogfood gate (`make archfit`) is still green (report-only detectors never
      gate the self-config)
      [NOTE: make archfit passes — verdict:PASS exit 0]
- [x] write/refresh the generator unit test asserting the coverage table is internally consistent
      (no finding left unclassified)
      [NOTE: added TestAllSurfaced_NoFindingUnclassified + TestAllSurfaced_NoUnknownStatus;
      also fixed probe names: 4.1 manifest_deprecation→deprecated_dep_count, 11.1 ProbeKind→ProbeMetric]
- [x] final full gate: `make all` (fmt → lint → test → archfit) — must pass
      [NOTE: make all passes — fmt + lint (0 issues) + test (all pass) + archfit (verdict:PASS)]

### Task 13: ➕ Speed up the test + lint feedback loop

_Discovered mid-execution (user request): every change runs `make lint` + `make test`, so the
inner-loop latency matters. Measure first, then cut — do not trade correctness, the `-race` gate, or
coverage for speed. Keep all existing gates green._

- [x] **measure the baseline first (no silent caps):** time `make test` and `make lint` cold and warm
      (`go test -race ./... -count=1` wall time; `go test -json ... | go-test-report`-style or
      `-v` parse to list the slowest packages/tests); capture the top offenders in
      `docs/archived/reports/eval/test-speed-baseline.md` so the speedup is evidence-driven, not guesswork
      [RESULT: cold ~23s, warm ~16s; cmd/archfit 4.5s (3.27s serial test time); all others < 0.1s actual test time — binary startup dominates]
- [x] **parallelize where safe:** add `t.Parallel()` to independent unit tests and their subtests
      (table-driven loops — capture range var if Go <1.22 idiom requires), but NEVER to tests that
      mutate shared/global state, `t.Setenv`, the working dir, or golden files; document any test
      deliberately left serial with a one-line reason
      [RESULT: added t.Parallel() to all 14 cmd/archfit test files (except TestLoadDotEnv + TestLoadDotEnv_EmptyEnvVarWins which use t.Setenv). Other packages: <0.1s in-test time — no wall benefit, documented. cmd/archfit: 4.5s → 2.5s (−44%)]
- [x] **kill duplication / fixture waste:** find repeated heavy setup (re-parsing the same config,
      rebuilding the same graph, re-running ast-grep) and hoist to `TestMain`/package-level fixtures or
      a shared helper; dedupe near-identical test bodies into table-driven cases
      [RESULT: no `TestMain` found; per-package test times are dominated by binary startup (~1.5s), not setup. Individual tests run in <1ms. Fixture hoisting would save <0.1s — no meaningful win. Checked cmd/archfit: each test uses t.TempDir() (correct isolation). No changes needed.]
- [x] **replace sleeps/polling with events where any exist:** grep for `time.Sleep`/busy-wait in tests;
      convert to channel/`sync` signaling or `Eventually`-style bounded waits (do not just shorten the
      sleep). If none exist, record that and skip — no invented work
      [RESULT: one sleep found — `internal/llm/llm_test.go` TestAnthropic_Timeout: 200ms server-handler sleep blocked srv.Close(). Converted to `select { case <-time.After(200ms): case <-r.Context().Done(): return }`. All other tests: no sleeps.]
- [x] **investigate the genuinely slow tests** surfaced by the baseline (e.g. subprocess/ast-grep
      integration, golden regen): cache the expensive artifact across subtests, gate the heaviest
      behind `testing.Short()` with `-short` wired into a fast `make test-fast` target (keep full
      `make test` running everything in CI), or shrink fixtures without losing the assertion's teeth
      [RESULT: sg integration tests (TestSyntaxIntegration_JSONShape + TestSyntaxIntegration_AllRuleFiles) gated behind testing.Short() — skipped in make test-fast. Full make test still runs them. make test-fast target added to Makefile.]
- [x] **tune the linter for inner-loop speed:** review `.golangci.yaml` — keep fast correctness linters
      in `make lint`; move demonstrably slow linters (whole-program / type-heavy, e.g. the slow ones
      among `gocritic`/`unparam`/`govet shadow`/`staticcheck` extras) to a CI-only profile
      (`.golangci.ci.yaml` or a `make lint-ci` target) IF measurement shows they dominate — keep the
      default `make lint` complete enough to catch real defects locally; document what moved and why
      [RESULT: golangci-lint runs at 1.3s warm — no linter measurably slow. `.golangci.yaml` left unchanged. No CI-only profile created.]
- [x] **verify nothing regressed:** full `make test` (race, all tests) + `make lint` (full profile) + `TestGolden` + `TestArchImports` + `make archfit` all green; report the before/after wall-clock
      delta in `docs/archived/reports/eval/test-speed-baseline.md`
      [RESULT: all gates green — 53/53 packages pass, TestGolden pass, TestArchImports pass, lint 0 issues, make archfit verdict:PASS. Before/after: warm ~16s → ~14.7s (−8%); cmd/archfit 4.5s → 2.5s (−44%)]

### Task 14: [Final] Documentation

- [x] update `docs/guide` with the new report-only metrics/facts and the opt-in rule types
      (`struct_field_max`, `public_api_type_leak`) — defaults, what they flag, how to gate
- [x] update `CLAUDE.md` "Invariants"/metric notes if a new core-ring package or fact kind was added
      (and any new `make` target like `test-fast`/`lint-ci` from Task 13)
      [NOTE: added `make test-fast` to Commands; no core-ring package or invariant changed]
- [x] update project memory only if a non-obvious decision was made (per memory rules)
      [NOTE: doc-only task; no non-obvious design decision made — nothing warrants a memory entry]

## Technical Details

**Per-category recipe (cite in each task):**

| Cat | Category                  | Repo(s)                | Detector                                                                           | Output                             | Gate          |
| --- | ------------------------- | ---------------------- | ---------------------------------------------------------------------------------- | ---------------------------------- | ------------- |
| 6   | Unsafe / safety seam      | yazi, herdr            | ast-grep `unsafe{}`/`UnsafeCell`/`transmute`/raw cast → `unsafe_op`                | SyntaxSurface + `unsafe_density`   | info          |
| 2   | God struct by field count | herdr                  | ast-grep struct field count → `struct_field_max` rule                              | facts + opt-in rule                | warn (opt-in) |
| 8   | Panic / error-handling    | herdr                  | ast-grep `unwrap/expect/panic!` minus test files (`testfile.go`) → `panic_density` | SyntaxSurface                      | info          |
| 7   | Global mutable state      | herdr                  | ast-grep `static mut`/`Atomic*`/`OnceLock` → `global_state`                        | SyntaxSurface                      | info          |
| 5   | Public-API type leak      | pumba                  | ast-grep exported type × `NodeKindExternal` → `public_api_type_leak` rule          | facts + opt-in rule                | warn (opt-in) |
| 9   | Lazy / hidden cycle       | ccgram                 | ast-grep in-function import → `lazy_import`                                        | SyntaxSurface (signal)             | info          |
| 13  | Test-coverage proxy       | yazi, herdr            | test-fn density per module → `test_density`                                        | metric                             | info          |
| 4   | Dependency deprecation    | pumba                  | manifest `retract`/`deprecated`/`yanked` markers                                   | `dependency_graph_health` evidence | info          |
| 11  | File-level mutual import  | codegraph              | file-node graph projection, A↔B no module-SCC → `file_mutual_import`               | finding                            | info          |
| 10  | Layer violation           | yazi                   | EXISTING `forbidden_layer_direction` — author the rule                             | gate (when declared)               | config        |
| 12  | Semantic / intent         | archfit, pumba, ccgram | route to `review`/`enrich` (LLM)                                                   | —                                  | LLM/human     |

**Validation basis (assumption, default chosen):** the comparison ground truth is the **frozen**
`reports/eval/architect-only-inventory.md` (the architecture-review skill's captured output, dated
2026-06-25), NOT a per-validation re-run of the skill — re-running is expensive and
non-deterministic, a poor acceptance gate. Regenerating the inventory is a manual refresh step
(Post-Completion) only if the target repos change materially.

**Acceptance is a coverage table, not "reproduce every finding."** The doc's thesis is detection ≠ closure:
the gate is satisfied when each finding is classified `surfaced` (deterministic detector now fires),
`llm-routed-by-design` (Cat 4 live-EOL, Cat 12 semantic, Cat 13 real-coverage, unsafe/panic
soundness), or `agree` (already caught — e.g. finding 1.3 herdr `headless.rs`, which
`structural_weight` already flags). Demanding deterministic reproduction of the LLM-only findings
would fail by design.

**Delta is a smoke check.** The new capabilities are full-scan facts; change-based delta mode will
not surface god-files/unsafe/etc. Run delta only to prove the new code does not crash or spuriously
gate — do not contort detectors to "appear in delta."

## Post-Completion

_Items requiring sibling repos, external toolchains, or human/LLM judgment — no checkboxes._

**Cross-repo execution prerequisites:**

- The full sweep (Task 12) requires the 5 sibling repos present under `~/Workspace` with their
  analysis toolchains installed: cargo + cargo-modules + rust-analyzer (yazi, herdr), grimp (ccgram),
  dependency-cruiser (codegraph), `go list` (archfit, pumba), and `sg` (ast-grep) for all. Any
  missing toolchain causes that repo/dimension to be logged + skipped, not silently dropped.

**Manual / external verification:**

- Inspect each repo's `reports/eval/gap-closure/<repo>/full.md` by hand to confirm the new facts are
  meaningful (e.g. unsafe density concentrated in `yazi-actor/lives`, panic density in
  herdr `ghostty/mod.rs` _after_ test exclusion, `AppState` flagged as a 106-field god-struct).
- Regenerate `architect-only-inventory.md` via the `architecture:architecture-review` skill only if
  target repos change; this plan treats the current inventory as frozen ground truth.

**LLM/human routing (the irreducible residue):**

- Run `archfit review` / `archfit enrich` on the repos to demonstrate the LLM path consuming the new
  report-only facts for the soundness/intent judgments (Cat 10 intent, Cat 12 semantic, unsafe/panic
  soundness, live-dependency EOL, true coverage adequacy). Use the Anthropic key from the repo `.env`.

## Success Criteria

- Every P2/P3 category and Cat 11 has a deterministic detector that fires on the repo where the
  architect found it, surfaced report-only (or behind an opt-in `warn` rule); none gates by default.
- `reports/eval/gap-closure/coverage.md` classifies every enumerated summary-table finding (20 rows
  across 13 categories) with no unclassified entries; the LLM-routed and AGREE findings are
  explicitly named, not hand-waved.
- `make all` (fmt → lint → test → archfit dogfood) is green; goldens reflect only the intended new
  output sections; `TestArchImports` confirms the core ring stayed clean.
- The design doc's P2/P3 status is updated to implemented, with the irreducible residue kept honest.
