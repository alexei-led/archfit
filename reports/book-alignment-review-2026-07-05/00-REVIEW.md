# archfit book-alignment review — 2026-07-05

**Scope.** Current branch at `48d0553` (`fix/wave1-gate-integrity`). I built the CLI, ran `doctor`, ran focused and full tests, read `.book/9780137353576.epub` directly, and ran `analyze --full --json` on the corpus sample. I did not trust plan checkboxes or comments as proof; comments are cited only when matching executable code/tests.

**Book scope read.** Ch6–Ch13 + appendices from the EPUB XHTML. Core anchors used below:

- Ch6, **Connascence**: static degrees (name, type, meaning, algorithm, position) and dynamic degrees (execution, timing, value, identity).
- Ch7, **Integration Strength**: intrusive > functional > model > contract; symmetric functional coupling for duplicated business algorithms.
- Ch8, **Distance**: encapsulation boundary distance plus socio-technical ownership and runtime/lifecycle coupling.
- Ch9, **Volatility**: domain/subdomain role first; core high, supporting/generic low; source control is only corroborating; inferred volatility exists.
- Ch10, **Balancing Coupling on a Numeric Scale**: `BALANCE = max(|STRENGTH - DISTANCE|, 10 - VOLATILITY) + 1`; worked examples replayed in Appendix A.
- Ch11, **Rebalancing Coupling**: react to change vectors by reducing strength or distance; volatility is a domain condition, not a knob to “lower” in code.
- Ch12, **Fractal Modularity**: balanced coupling is the self-similarity rule at every abstraction level.
- Ch13, **Balanced Coupling in Practice**: practical cases across services, patterns, objects, and methods.

## 1. Verdict

archfit is now **close on the deterministic Balanced Coupling core**. The scorer uses the book formula and ordinals, abstains instead of fabricating scores, distinguishes same-module/local coupling from cross-boundary coupling, and can gate `coupling_balance` deterministically.

It is **not yet the full automated method promised by the product goal**. The weak spots are not the formula. They are the hard parts around it:

1. **Semantic/LLM config generation is shallow.** It can draft labels and module classifications, but `config init --llm`/`config update --llm` mostly send module paths, public globs, file basenames, and README/docs headings — not bounded ADR/design-doc/code evidence.
2. **Some book-relevant implicit coupling is report-only.** Clone-only duplicated knowledge maps to symmetric functional coupling but is explicitly kept out of `coupling_balance` and the gate.
3. **Distance still lacks runtime coupling in the score.** Ownership, deploy-unit, source structure, and declared external systems are measured; runtime async/sync evidence is report-only.
4. **Multi-language parity is uneven.** Go has compiler-grade strength hints. Python/Rust can measure useful edges with SCIP/grimp/cargo modules. TypeScript honestly reports `n/a` when resolution is poor, but that is still low coverage.

**Overall book-conformance score, core-weighted:** **4.1 / 5**.

**Overall tool-goal score:** **3.8 / 5**.

## 2. Scorecard

Scale: 0 absent · 1 wrong · 2 partial · 3 adapted-sound · 4 close · 5 verbatim/excellent.

| Area | Score | Judgment | Evidence |
|---|---:|---|---|
| Integration Strength | 4 | Close. Book ordinals are present; Go DTO/const/var edge cases are fixed; symmetric duplicate code exists. Still incomplete for semantic functional coupling beyond imports/clones and weaker outside Go. | Book Ch7. Ordinals in `internal/model/coupling/scorer_book.go:21-28`; Go object mapping in `internal/extract/golang/golang.go:337-375`; DTO handling in `internal/extract/golang/golang.go:414-445` and `internal/classify/classify.go:553-627`; clone symmetric upgrade in `internal/classify/classify.go:390-409`. |
| Distance | 4 | Close adapted model. Code structure, owner, deploy unit, and declared external D=10 are represented. Runtime/lifecycle distance remains report-only. | Book Ch8. Ordinals in `internal/model/coupling/scorer_book.go:30-39`; distance precedence in `internal/classify/classify.go:637-704`; structure fallback in `internal/classify/distance_structure.go:11-79`; deploy unit in `internal/classify/distance_structure.go:184-192`; external systems in `internal/classify/external_systems.go:32-59`. Runtime async is report-only: `internal/engine/engine_test.go:2155-2159`, docs note at `docs/design/bc-measurement-v4.md:366-368`. |
| Volatility | 4 | Close. Gate-time volatility is domain/config/subdomain based, not churn. Opt-in cascade exists. Semantic drafting helps, but automated domain evidence is shallow. | Book Ch9. `internal/classify/classify.go:707-758` maps explicit volatility or subdomain; `internal/classify/classify.go:775-833` implements single-hop cascade; scorer treats undeclared/unknown conservatively at `internal/model/coupling/scorer_book.go:41-49`. |
| Balance formula/bands | 5 | Formula and book examples replay exactly. Severity bands are an archfit overlay, but transparent and tested. | Book Ch10. Formula in `internal/model/coupling/scorer_book.go:97-116`; score definition/version in `internal/model/coupling/scorer.go:10-29`; severity bands in `internal/model/coupling/scorer.go:143-155`; focused tests passed. |
| Connascence | 2 | Partial. archfit uses connascence indirectly through strength levels, but it does not classify/report Ch6 degrees. The prior dead/wrong `ConnascenceAlgorithm` path is gone, which is good. | Book Ch6. No implementation hits beyond docs in `rg "connasc\|Connasc" internal docs`; strength hints collapse many Ch6 degrees into contract/model/functional/intrusive. |
| Abstain / n-a honesty | 5 | Excellent. Unknown strength/distance abstains; unmeasured coupling reports `n/a`, not a fabricated mid score. External/library edges are disclosed and excluded unless declared. | `internal/model/coupling/scorer_book.go:97-104`; `internal/score/score_boundary_coupling.go:57-74`; external exclusion in `internal/engine/assemble.go:182-190`; Storybook run produced `0/n/a/low` with warnings. |
| Fractal handling | 4 | Close. Same-module edges score into `local_coupling` but do not pollute cross-boundary `coupling_balance`. Only one local level is currently exposed. | Book Ch12. Same-module design in `internal/model/coupling/scorer_book.go:8-14`; local complexity in `internal/model/coupling/scorer_book.go:90-95`; same-module severity exclusion in `internal/classify/classify.go:75-84`; self-run emitted `local_coupling`. |
| Terminology | 4 | Mostly aligned. Core names use book terms. Some operational terms are archfit-specific (`cross_module_same_owner`, `declared_external`, score bands), but evidence strings explain them. | `internal/model/coupling/coupling.go:3-36`; score evidence in `internal/score/score_boundary_coupling.go:118-144`. |
| Gate / fitness-function | 4 | Strong deterministic gate path now exists. It is opt-in; report-only remains default. Gate never runs LLM. | Coupling gate projection in `cmd/archfit/pipeline_run.go:465-479`; evaluation in `internal/score/gate.go:26-57`; promotion to gate findings in `cmd/archfit/pipeline_run.go:503-532`; agent tasks run after gate at `cmd/archfit/pipeline_run.go:354-372`. |
| Complementary metrics | 3 | Useful but narrow. Implemented metrics are non-contaminating in measured runs, but many requested metrics are absent. `blast_radius` is used only as a degenerate-graph proxy. | Registry has only 5 metrics: `internal/metrics/metrics.go:45-62`. Non-contamination run: metrics-off self scan kept `coupling_balance=42/mixed/high` and identical `classified_edges`. Degenerate proxy in `internal/score/score.go:208-222`. |
| Semantic / LLM layer | 3 | Deterministic boundary is good; semantic maturity is medium. Draft labels are reviewable and provenance-aware. Config generation evidence is too thin for domain-subdomain decisions. | LLM import ban in `internal/arch_test.go:160-172`; labels are approved/draft with evidence hash at `internal/labels/labels.go:1-24`, `63-78`, `122-155`; LLM labels only fill unknown cells at `internal/classify/classify.go:570-585`; config init prompt at `cmd/archfit/init.go:155-166`; config evidence is file basenames/headings at `internal/initcfg/classify_targets.go:10-38` and `cmd/archfit/update.go:300-340`. |
| Multi-language parity | 3 | Honest but uneven. Go is strongest; Python and Rust measure useful edges; TS can fall to `n/a` under poor resolver coverage. | Go/Py/Rust/TS empirical runs in Appendix A. Go type hints: `internal/extract/golang/golang.go:337-375`; TS run disclosed `dependency-cruiser` partial and `scip` absent. |

## 3. What is implemented and how good it is

### Deterministic leg

Implemented well:

- **Book scorer.** `BookScorer` uses `S={1,3,8,9,10}`, `D={2,4,7,9,10}`, `V={1,3,6,10}` and the Ch10 formula exactly (`internal/model/coupling/scorer_book.go:21-82`, `97-116`).
- **Go strength precision.** Interfaces map to contract, DTOs can remain contract across public boundaries, concrete types/consts/vars map to model, functions/behavior carriers map to functional (`internal/extract/golang/golang.go:337-375`; `internal/classify/classify.go:553-627`).
- **Distance composition.** Deploy unit outranks ownership/structure, ownership is used only when non-degenerate, structure falls back for ownerless/single-owner repos, and declared external systems score at D=10 (`internal/classify/classify.go:637-704`; `internal/classify/external_systems.go:32-59`).
- **Volatility source discipline.** Gate-time volatility comes from explicit config or subdomain; no churn-derived volatility (`internal/classify/classify.go:707-758`).
- **Abstention.** Unknown strength or distance is unscored (`internal/model/coupling/scorer_book.go:97-104`). Zero scored internal edges render `n/a` (`internal/score/score_boundary_coupling.go:57-74`).
- **Fractal separation.** Same-module edges are scored in `local_coupling`, but not in `coupling_balance` or gate (`internal/classify/classify.go:75-84`; self artifact `local_coupling`).
- **Deterministic gate.** `coupling.gate.min_band/max_drop` can fail verdicts and promote BC advisories to gate findings/agent tasks (`internal/score/gate.go:38-57`; `cmd/archfit/pipeline_run.go:503-532`).

Improvements vs `docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`:

- Baseline called out lossy classification: DTOs could not reach Contract, const/var reads jumped to Functional, no D=10, same-module hidden (`00-FINDINGS.md:16-38`). Current code fixes each in `bc_score.v4` and `local_coupling`.
- Baseline called out `coupling_balance` unable to gate (`00-FINDINGS.md:55-60`). Current `applyCouplingGate` fixes this.
- Baseline called out dead metric gate fields, bad config-init rule type, same-module public API false positive, TS coverage warning, grouped-finding path, clone-without-edge invisibility (`00-FINDINGS.md:61-102`). I verified the relevant current mechanisms exist, including gate wiring, TS partial warning, and clone-only report advisory.
- Baseline called out LLM volatility differentiation as a later wave. Current `config update --llm` can propose roles/subdomains/volatility, but evidence collection is still thin.

### Semantic / LLM leg

Good:

- **LLM is off-gate structurally.** `internal/*` cannot import `internal/llm`; the structural test passed (`internal/arch_test.go:160-172`; `go test ./internal -run TestArchImports`).
- **Labels are reviewable.** Draft labels are inert; only approved, fresh labels apply (`internal/labels/labels.go:63-78`, `108-155`).
- **LLM provenance is weaker than static evidence.** LLM labels fill only otherwise-unknown strength cells (`internal/classify/classify.go:570-585`). Non-high-confidence LLM edges lower `coupling_balance` confidence (`internal/score/score_boundary_coupling.go:107-114`, `207-224`).
- **LLM narrative cannot change gate exit.** `analyze --llm` appends narrative after deterministic rendering; LLM errors are warned and ignored (`cmd/archfit/analyze.go:234-244`).

Weak:

- `config init --llm` sends paths/public globs/file basenames, not repository docs or code snippets (`cmd/archfit/init.go:82-83`, `171-193`; `internal/initcfg/classify_targets.go:10-38`).
- `config update --llm` adds only README/docs Markdown headings, capped at 20, not ADR bodies or code content (`cmd/archfit/update.go:300-340`).
- `config init --llm --apply` can write LLM classifications directly to config (`cmd/archfit/init.go:23-24`, `103-132`). It is still pinned/reviewable in Git, but it skips a draft-review artifact.

## 4. Missing / wrong, severity-ranked

### P1 — semantic evidence is too shallow for volatility/subdomain fidelity

- **Book:** Ch9, **Domain Analysis** / **Volatility of Subdomains**. Volatility comes from business-domain role, not directory shape or file names.
- **Code:** `config init --llm` builds targets from module paths/public globs/file basenames (`internal/initcfg/classify_targets.go:10-38`) and calls `classifyModules` without repo evidence (`cmd/archfit/init.go:82-83`, `227-238`). `config update --llm` adds README/docs headings only (`cmd/archfit/update.go:300-340`).
- **Impact:** LLM can produce plausible subdomain/volatility config, but it is not yet reading the design docs/ADRs/code enough to satisfy the tool goal.
- **Fix:** Add a bounded evidence pack: README summary, ADR/design-doc snippets with citations, package doc comments, exported API names, and representative source snippets. Require each LLM module annotation to cite evidence refs. Keep output as draft unless `--apply` is explicitly chosen.

### P1 — clone-only duplicated knowledge is book-core but not in `coupling_balance`

- **Book:** Ch7, **Functional Coupling / Symmetric Functionality**; Ch10 Example 3 scores duplicated business rule across services as S=9, D=9, V=10, balance 1.
- **Code:** clone-only pairs are scored but “Report-only” and “never by coupling_balance or the gate” (`internal/classify/clone_only.go:21-25`). Existing-edge clones can upgrade strength (`internal/classify/classify.go:390-409`).
- **Impact:** A copy-pasted cross-module rule with no import edge can appear in findings but not degrade the flagship score or gate. That is a book-core undercount.
- **Fix:** Add a separate `implicit_edges`/`duplicated_knowledge` bucket to `classified_edges` and optionally include it in `coupling_balance` with provenance. If keeping report-only, label it explicitly as a core book gap, not just complementary.

### P2 — runtime/lifecycle distance is disclosed but not scored

- **Book:** Ch8, **Distance and Runtime Coupling** / **Asynchronous Communication and Cost of Change**.
- **Code:** distance scoring uses deploy unit, ownership, and code structure (`internal/classify/classify.go:637-704`; `internal/classify/distance_structure.go:11-79`). Runtime async evidence is grouped into `runtime_async` only; tests assert static graph/results are unchanged (`internal/engine/engine_test.go:2155-2159`).
- **Impact:** The tool cannot distinguish synchronous runtime coupling from async decoupling in D, even though the book says lifecycle coupling affects distance.
- **Fix:** Either make runtime coupling an explicit, opt-in distance signal with provenance/confidence, or keep it report-only but lower the Distance score/maturity claim.

### P2 — connascence is not first-class

- **Book:** Ch6, **Static Connascence** and **Dynamic Connascence**.
- **Code:** no internal implementation of connascence degrees; strength hints collapse everything into contract/model/functional/intrusive. Search found no real `connascence` implementation in `internal/`.
- **Impact:** The tool cannot explain whether functional coupling is execution/timing/value/identity, nor whether model coupling is name/type/meaning/algorithm/position. This limits Ch6 fidelity and recommendation specificity.
- **Fix:** Add optional connascence evidence fields on classified edges. Start with cheap syntactic signals: positional arguments, magic values/enums, duplicate algorithms/clones, transaction/identity co-commit patterns, ordering/timing markers.

### P2 — confidence ignores tiny measured samples

- **Book:** Ch10 warns the numeric scale is not exact science and requires judgment.
- **Code:** confidence is based mainly on scored fraction (`internal/score/score_boundary_coupling.go:80-88`). Spotinfo scored only 4 internal edges but reported high confidence.
- **Impact:** Small, fully classified graphs can look as trustworthy as broad measurements.
- **Fix:** Add a sample-size/module-coverage confidence cap or evidence line: e.g. high confidence requires both scored fraction and minimum scored edge/module count.

### P3 — TypeScript parity remains tool-fragile

- **Book:** Method is language-independent.
- **Empirical:** Storybook run with the stored config produced `coupling_balance: n/a/low`, `0 scored`, `11331 external`, `dependency-cruiser` partial at 55% unresolved, and SCIP absent.
- **Impact:** This is honest, not wrong, but TS still needs setup/config help before it can participate in the book method.
- **Fix:** Improve `config init/update` for TS workspaces: tsconfig/package boundary discovery, workspace package modules, dependency install hints, and path alias validation.

## 5. Complementary metrics table

Controlled non-contamination check: I copied `.archfit.yaml` and `.archfit-labels.yaml`, disabled all registered metrics, and reran self-analysis. Result stayed **exactly** `coupling_balance=42/mixed/high`, `337` scored edges, mean balance `4.786350148367952`, and identical `classified_edges`; metrics count changed from 5 to 0. This proves the registered metrics do not feed S/D/V or the measured score on a normal graph.

| Signal | Implemented? | Verdict | Non-contamination | Value / issue | Evidence |
|---|---:|---|---|---|---|
| Cycles | Yes | Keep | Does not feed S/D/V or score. Separate metric only. | Useful architecture smell. Rust cycles are called out as language-permitted. | Registry `internal/metrics/metrics.go:45-62`; calculator `internal/metrics/boundary/cycle.go:26-76`. |
| Blast radius | Yes | Refine | Does not feed S/D/V; code uses its `n/a` only as a degenerate-graph proxy. Metrics-off self run unchanged. | Useful hub/change-amplification signal. Clarify proxy behavior in docs. | Calculator `internal/metrics/modularity/blast_radius.go:14-36`; proxy `internal/score/score.go:208-222`. |
| Encapsulation | Yes | Keep/refine | Separate metric; no score input. | Good boundary signal, but often `n/a` in compiler-enforced languages and excludes functional/model edges by design. | `internal/metrics/boundary/encapsulation.go:39-52`, `72-91`. |
| Unbalanced edge count | Yes | Refine/drop later | Separate metric; no score input. | Now partly redundant with `coupling_balance`; keep only if it remains a simple raw count for operators. | `internal/metrics/boundary/unbalanced_edge.go:39-51`. |
| Coverage | Yes | Keep | Tool-health signal. It can cap confidence via tool coverage, but does not alter S/D/V. | Valuable honesty signal; TS partial warning worked. | `internal/metrics/boundary/coverage.go:15-66`; TS cap in `internal/score/score.go:177-205`. |
| Clones / duplicated knowledge | Yes, as analyzer/advisory | Refine | Existing-edge clones affect strength; clone-only pairs are report-only. | Book-relevant, not merely complementary. Should be explicit in score policy. | `internal/classify/classify.go:390-409`; `internal/classify/clone_only.go:14-25`, `86-99`. |
| Cyclomatic complexity | No | Consider | N/A | Useful local maintainability signal, not BC core. Keep opt-in/report-only if added. | Not in `internal/metrics/metrics.go:45-62`. |
| Cohesion / LCOM | No direct LCOM | Consider | N/A | Book-relevant through local complexity/cohesion; current `local_coupling` is a better BC-specific start. | `local_coupling` output; `internal/model/coupling/scorer_book.go:90-95`. |
| Instability / abstractness | No | Drop by default | N/A | Martin metrics are not book-core and can distract. Add only if clearly labeled report-only. | Not registered. |
| God-struct | No | Consider | N/A | Could support local complexity, but not BC score. | Not registered. |
| Global state | No | Consider | N/A | Can indicate intrusive/common coupling. Add as report-only or strength evidence only with high precision. | Not registered. |
| Unsafe / panic density | No | Drop from BC | N/A | Reliability metric, not BC. Keep outside this tool unless archfit broadens scope. | Not registered. |
| Test density | No | Drop from BC | N/A | Quality metric, not BC. | Not registered. |

## 6. Recommended next waves

### Wave 8 — semantic evidence pack

Goal: make LLM-generated config worthy of Ch9 domain volatility.

- Feed bounded ADR/design-doc/README snippets, package docs, exported API names, and representative code snippets.
- Require evidence citations in LLM responses.
- Output draft review artifacts by default for `init --llm` and `update --llm`; keep `--apply` but warn harder or write a separate reviewed patch.
- Add tests that prompt content includes ADR/body snippets, not only headings.

### Wave 9 — implicit coupling into score policy

Goal: decide how book-core duplicate knowledge affects the flagship metric.

- Add `classified_edges.implicit` or `classified_edges.duplicated_knowledge` for clone-only pairs.
- Include or explicitly exclude them from `coupling_balance` with a config flag and evidence line.
- Extend beyond textual clones later: semantic duplicate business rules via LLM draft labels, human-approved before score impact.

### Wave 10 — distance parity

Goal: close Ch8 distance gaps.

- Add opt-in runtime coupling evidence to distance: sync RPC/HTTP/DB coupling vs async/message coupling.
- Keep confidence lower for heuristic runtime detection.
- Improve owner resolution and TS workspace config generation.

### P2/P3 backlog

- Add connascence degree annotations as edge metadata.
- Add sample-size confidence caps.
- Improve TS setup diagnostics and module discovery.
- Keep complementary metrics report-only unless a metric is directly serving S/D/V evidence.

## Appendix A — Empirical runs

### Build / doctor / tests

| Check | Result |
|---|---|
| `make build` | PASS. Built `.bin/archfit` as `v1.1.2-72-g48d0553`. |
| `.bin/archfit doctor` | PASS. git, uv, ast-grep, jscpd, Go, scip-go, Node, scip-typescript, Python, scip-python, cargo, rust-analyzer, cargo-modules all `ok`; LLM provider configured off-gate. |
| `go test ./internal/model/coupling -run 'TestBookScorer_(BookExamples\|DeclaredExternal\|Abstain\|NoVolatilityLever\|DefaultScorer)' -count=1` | PASS. |
| `go test ./internal -run TestArchImports -count=1` | PASS. LLM/labels I/O import guards hold. |
| `make test` | PASS. Includes Go race/coverage package tests and SCIP Python reader tests. |

### Ch10 worked-example replay through `BookScorer`

Run via a temp Go module importing `github.com/alexei-led/archfit/internal/model/coupling` from this checkout.

| Book example | Book S/D/V | archfit output | Match |
|---|---:|---:|---|
| Example 1: model copied to cross-vendor ML service | 3 / 10 / 10 | balance 8, low | Yes |
| Example 1 alternative: contract to same vendor service | 1 / 10 / 10 | balance 10, none | Yes |
| Example 2: `SupportCase`/`Message` transactional cohesion | 8 / 2 / 10 | balance 7, low | Yes |
| Example 3: duplicated rule across services | 9 / 9 / 10 | balance 1, critical | Yes |
| Example 4: intrusive access to frozen legacy system | 10 / 9 / 1 | balance 10, none | Yes |

### Corpus analyze runs

Raw JSON/stderr artifacts are under `reports/book-alignment-review-2026-07-05/artifacts/`.

| Repo | Lang | Command shape | Result |
|---|---|---|---|
| archfit | Go | `.bin/archfit analyze -c .archfit.yaml --root /Users/alexei/Workspace/archfit --full --json --quiet` | `pass`; `coupling_balance=42/mixed/high`; `337` scored, `0` abstained, `572` external; mean 4.8. |
| pumba | Go | same with repo config | `pass`; `45/mixed/high`; `70` scored, `0` abstained, `428` external. |
| spotinfo | Go | same with repo config | `pass`; `36/poor/high`; `4` scored, `0` abstained, `66` external. High confidence on tiny sample should be refined. |
| ccgram | Python | same with repo config | `warn`; `55/mixed/high`; `497` scored, `18` abstained; strengths include functional/model/intrusive/symmetric/unknown. |
| herdr | Rust | same with repo config | `pass`; `29/poor/high`; `630` scored, `16` abstained, `21` external. |
| storybook | TS | stored corpus config `reports/eval-2026-06-30-corpus/configs/storybook.yaml` | `pass`; `0/n/a/low`; `0` scored, `11331` external; `dependency-cruiser` partial at 55% unresolved and SCIP absent. Honest n/a, low parity. |

### Metrics non-contamination replay

- Baseline self run: `coupling_balance=42/mixed/high`, `classified_edges.scored=337`, mean `4.786350148367952`.
- Metrics-off self run with all registered metrics disabled and labels copied beside the temp config: exact same `coupling_balance` and exact same `classified_edges`; metrics list length `0`.
- Conclusion: registered complementary metrics do not feed S/D/V or the measured score in normal measured graphs. The only score-package dependency on a metric is the `blast_radius` `n/a` degenerate-graph proxy (`internal/score/score.go:208-222`).
