# Closing the archfit ⟷ architect gap

**Status:** ANALYSIS — P1 SHIPPED in this PR; P2/P3 PROPOSED (future, separate PR)
**Date:** 2026-06-25
**Basis:** 6-repo study (archfit/Go, pumba/Go, codegraph/TS, ccgram/Python, yazi/Rust, herdr/Rust).
Inputs: `reports/eval/architect-only-inventory.md` (20 findings, 13 categories) +
`reports/eval/archfit-capability-map.md` (existing metrics, rule-engine + syntax-facts extension points).
Closeability + cost verified with on-disk artifacts and live `sg` queries (see "Verification").

## The question

The independent architect (LLM) review surfaced 20 findings archfit's deterministic metrics
**missed** (ARCHITECT-ONLY). Which can archfit close, how, and what is irreducible?

## The honest answer

**Detection is mostly closeable; judgment is not — and detection ≠ closure.** archfit can
deterministically _surface a candidate_ for ~7 of 13 categories, and the ast-grep syntax-facts layer
makes that cheap (a new check is one embedded YAML rule, not new plumbing — verified: `sg` finds 167
`unsafe` blocks in yazi and can count struct fields). But for 3 of those 7 (unsafe, panic-density,
global-state) the raw candidates are **mostly noise without a judgment or test-exclusion layer** — a
naive panic counter even reproduces the herdr number this study already judged _overstated_. So the
gap shrinks from "archfit is blind to X" to "archfit flags X candidates; the LLM/human rules on
acceptability." Two categories turn out to overlap existing machinery (a config gap, and the existing
SCC cycle metric) rather than being new wins. Three residues are genuinely hard: dependency
EOL/deprecation (external registry), real test coverage (running the suite), and deep semantic intent.
That irreducible residue is the standing case for the deterministic+LLM hybrid, not a defeat of it.

## Closeability matrix

| #   | Category                         | Repos | Verdict                               | Mechanism / why                                                                                                                                                                                                                                                                                                                                                                                            |   Syntax-facts   | Residual LLM/human                                               |
| --- | -------------------------------- | :---: | ------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :--------------: | ---------------------------------------------------------------- |
| 1   | Single-file god-file             |  2–3  | **Close — cheap, clean**              | `file_structural_weight`/`file_size_max`: per-file LOC already gathered (`SizeInput.FileLOC`), only summed per module today. archfit missed its **own** `score.go` (1009 lines).                                                                                                                                                                                                                           |        no        | none (size threshold)                                            |
| 3   | Test/mock code in production     |   1   | **Close — moderate, clean**           | ast-grep: test-framework imports (`testify/mock`) in non-test files / `mock_*.go` w/o build tag → `test_in_production` rule. High severity when hit.                                                                                                                                                                                                                                                       |     **yes**      | none once pattern set                                            |
| 2   | God struct by field count        |   1   | **Detect — cheap; judgment LLM**      | ast-grep `struct $N { $$$F }` multi-capture counts fields (verified). `struct_field_max` opt-in.                                                                                                                                                                                                                                                                                                           |     **yes**      | "is this god-struct intentional here"                            |
| 6   | Unsafe code / safety seam        |   2   | **Detect — cheap; value is judgment** | ast-grep finds `unsafe{}`/`UnsafeCell`/`as *mut`/`transmute` (verified 167 in yazi). Report-only `unsafe_density`.                                                                                                                                                                                                                                                                                         |     **yes**      | **soundness of each `unsafe` — the actual value — is LLM/human** |
| 7   | Global mutable state             |   1   | **Detect — cheap but noisy**          | ast-grep `static mut`/module `Atomic*`/`OnceLock`. BUT `AtomicU32` for ID-gen is idiomatic → flags mostly legit code. Opt-in info only.                                                                                                                                                                                                                                                                    |     **yes**      | whether the shared state is a real flaw                          |
| 8   | Panic / error-handling           |   1   | **Detect — cheap but misleading**     | Raw `unwrap/expect/panic` count **reproduces the 126 herdr number this study judged OVERSTATED** (module-wide, mostly tests). Only useful _after_ excluding test code (depends on Cat 3), still weak.                                                                                                                                                                                                      |     **yes**      | acceptability of each panic                                      |
| 5   | Public-API framework-type leak   |   1   | **Partial — moderate**                | ast-grep exported field/return types × `NodeKindExternal` graph. Reliable only for typed langs w/ SCIP.                                                                                                                                                                                                                                                                                                    |     **yes**      | which leaks matter                                               |
| 9   | Dynamic/lazy-import hidden cycle |   1   | **Partial — signal only**             | ast-grep detects in-function imports cheaply (signal). Full closure needs adding lazy edges to the graph then re-running SCC.                                                                                                                                                                                                                                                                              | **yes** (signal) | confirming a real cycle                                          |
| 13  | Test-coverage blind spot         |   2   | **Partial — proxy only**              | Cheap proxy: ast-grep test-fn density. Real coverage needs a coverage-tool run (expensive).                                                                                                                                                                                                                                                                                                                | **yes** (proxy)  | coverage adequacy                                                |
| 11  | Non-cycle bidirectional coupling |   1   | **Not a new win — overlaps `cycle`**  | `extraction` IS a declared module; the existing `cycle` metric runs Tarjan SCC on the module graph (cycle=2). A true bidirectional _module_ dep is already a 2-node SCC it would catch. The miss is **file-level/granularity** — a module-level `mutual_dependency` rule inherits the same blind spot. Closing needs **file-level** mutual-import detection (a new finer-grained query), not a cheap rule. |        no        | —                                                                |
| 10  | Layer violation / dep direction  |   1   | **Config gap, not capability**        | archfit HAS `forbidden_layer_direction`/`forbidden_dependency`; they fire only on _declared_ rules; yazi declared none.                                                                                                                                                                                                                                                                                    |        no        | **inferring** intended layering → `enrich`/LLM/human             |
| 4   | Dependency deprecation / EOL     |   1   | **Mostly external-data gap**          | Manifest-declared markers cheap (`go.mod retract`, npm `deprecated`, cargo `yanked`); EOL of a live version needs an external registry/DB or config allowlist.                                                                                                                                                                                                                                             |        no        | which deprecations matter                                        |
| 12  | Semantic / intent judgment       |   3   | **Largely irreducible → LLM**         | A few convert to bespoke config rules (purity invariant → forbidden-import); the rest (key-format agreement, data-vs-behavior in config) need code semantics.                                                                                                                                                                                                                                              |     partial      | core LLM/human territory                                         |

## Verification (so "cheap"/"already-covered" aren't hand-waving)

- `sg run --pattern 'unsafe { $$$ }' --lang rust` on yazi → **167 matches**. Unsafe detection = one rule. (Cat 6)
- `sg run --pattern 'struct $NAME { $$$FIELDS }' --lang rust` → `metaVariables.multi.FIELDS` length yields the field count (e.g. 6 on `render_prof.rs`). Struct-field-count is feasible. (Cat 2)
- `codegraph-archfit.json`: `cycle` metric = 2 SCCs, band critical; `extraction` is a declared module. A bidirectional module dependency would already be a 2-node SCC → the existing metric covers the _module-level_ case; the extraction↔resolution miss is _file-level_. So Cat 11 is granularity, not a missing rule. (Cat 11)
- `pumba` mocks-in-prod confirmed earlier via `go list -f '{{.GoFiles}}'` (9 `mock_*.go` in production `GoFiles`). (Cat 3)

## What this says about the design

1. **The syntax-facts layer is the extension vehicle** for the genuinely syntactic categories
   (1-data, 2, 3, 5, 6, 9 — and 7/8 as noisy info): each is "one embedded YAML rule + a thin rule
   type/metric," reusing the producer, fact model, `Evidence.SyntaxFacts`, and the per-rule-type
   integration-test guard. This is the payoff of the layer beyond its original scope.
2. **Detection ≠ verdict.** For unsafe/panic/global-state the candidate facts are only useful as
   opt-in, report-only `info` with per-project thresholds (and Cat 8 needs test-exclusion first).
   The real value lives in the judgment layer — i.e. `review`/`enrich` (LLM) or a human pin.
3. **Two "gaps" aren't capability gaps:** layer-direction is already gateable (needs the rule
   authored — `enrich` can propose it); non-cycle bidirectional coupling overlaps the existing
   `cycle`/SCC metric at module grain and is really a file-level-granularity ask.
4. **Irreducible residue, named:** soundness/acceptability _judgment_, external-registry facts
   (EOL/deprecation), running tests for true coverage, deep semantic intent. archfit's job is to make
   the first cheap to _find_; the LLM path is where the judgment belongs.

## Roadmap (priority by value × cost)

- **P1 — SHIPPED IN THIS PR:** `file_structural_weight` report-only metric (Cat 1; now flags archfit's
  own `score.go`, 1017 LOC); `test_in_production` rule (Cat 3, default `gate: warn`; fires 14× on
  pumba's mock files). Both report-only/advisory by default so the dogfood stays green.
- **P2 — PROPOSED (separate follow-up PR) — cheap detect, opt-in report-only `info` (signal needs thresholds/judgment):** `unsafe_density`
  (Cat 6), `struct_field_max` (Cat 2). Ship as facts, never gate by default.
- **P3 — partial/proxy/dependent:** panic-density _after_ test-exclusion (Cat 8+3), global-state facts
  (Cat 7), public-API type-leak (Cat 5), lazy-import signal (Cat 9), test-density proxy (Cat 13),
  manifest-declared deprecation (Cat 4).
- **File-level cycle/mutual-import query** (Cat 11) — a real but non-trivial graph addition, not a rule.
- **Accept as LLM/human (route via `review`/`enrich`):** semantic/intent (Cat 12 core), unsafe/panic
  _soundness_, layer _intent_ inference, live-dep EOL, true coverage adequacy.

## Guardrail for any new check

Every new ast-grep rule ships with a per-rule-type assertion in `TestSyntaxIntegration_AllRuleFiles`
(the guard that caught the dead `rs-attribute` and `ts-decorator` rules). New metrics stay
`info`/off-gate by default; gating only via a config rule, consistent with existing `gate:` wiring.
Flag candidates; let the score/LLM/human judge.
