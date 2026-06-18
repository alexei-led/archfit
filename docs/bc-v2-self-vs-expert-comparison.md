# archfit v2 — automated self-analysis vs independent expert review

Date: 2026-06-18. Branch: `docs/bc-measurement-v2`. Compares two independent
assessments of the same codebase (archfit after the Balanced Coupling v2 work):

- **Tool**: `archfit` v2 run on its own code (full scan + delta vs `v0.2.0`),
  config updated to hierarchical module names (Task 19).
- **Expert**: an architecture-review agent that read the Go source directly and
  was explicitly forbidden from seeing archfit's output or the BC design docs.

The point: does the deterministic tool catch what a senior architect catches —
and where do they diverge? This is the validation of archfit as an
architecture-fitness feedback loop.

## 1. Where they AGREE (tool ≈ expert)

These are the structural/quantitative issues both surfaced independently — high
confidence they're real.

| Issue                                  | archfit (automated)                                                                                             | Expert                                                                                                 |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| **God-modules**                        | `structural_weight`: `cmd/archfit` 2282 LOC (11×), `internal/initcfg` 2018 (9×), `metrics/modularity` 1301 (6×) | F-MOD-1 (initcfg = 4 responsibilities), F-SIZE-1 (initcfg.go 760 + yamledit.go 742), god `cmd/archfit` |
| **Config as the widest hub**           | `blast_radius`: `internal/config` (35%, 18 deps); `risk_hub`: config #2 (154)                                   | F-BLAST-2 (config imported by ~everything; `ModuleDef` a structural hub)                               |
| **coupling/model as high-impact core** | `blast_radius`: `model/graph` 71%, `model/coupling` 51%                                                         | F-BLAST-1 (`model/coupling` highest-impact new pkg)                                                    |
| **Complexity hotspots**                | `complexity`: 16 funcs CCN>15 incl. new `change_coupling.Calculate` (20), `martin` paths                        | F-SIZE / §7 complexity in new metrics                                                                  |
| **`cmd/archfit` over-central**         | `instability` I=1.00, `propagation_cost` reach 94%, 13 advisories cmd→internal                                  | F-MOD-2 (business logic in cmd), F-TEST-2 (untestable cmd functions)                                   |

**Verdict:** strong agreement on _size, fan-in/out, complexity, and which packages
are structural hubs_. The tool's quantitative core matches expert structural intuition.

## 2. Where the TOOL is BLIND (expert caught, archfit missed)

These are **semantic/correctness** issues. archfit measures structure; it does not
read intent or verify logic — so it missed all of these. This is the load-bearing
finding of the comparison.

| Expert finding                                                                                                                        | Why archfit can't catch it                                                                                                       |
| ------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| **F-NEW-3: `derivedVolatility` computed every run, never read** (dead storage)                                                        | archfit has no dead-code / unused-field analysis. A graph metric can't see "nothing reads this."                                 |
| **F-NEW-2: `codeStructureDistance` returns SameOwner for all flat module names** (the tool's own distance bug)                        | A tool can't introspect the correctness of its own algorithm.                                                                    |
| **F-NEW-5: `MultiplicativeScorer` scores `(contract, same_module, high_vol)` as critical** (latent formula bug, guarded by call site) | Logic-level correctness; invisible to structural metrics.                                                                        |
| **F-NEW-1: `MartinDistance` duplicates `Abstractness`'s edge loop** (DRY)                                                             | In-function duplication of a loop; clones/hidden_coupling operate at file/module granularity.                                    |
| **F-DRIFT-1: value-receiver methods mutate shared map; `ForRules()` runs before the fills** (ordering fragility)                      | Semantic receiver/ordering analysis is beyond import-graph metrics.                                                              |
| **F-COUP-1: `engine` imports `labels` (an os+yaml I/O adapter), unguarded by arch_test**                                              | archfit _could_ catch this with a `forbidden_dependency` rule — but none is configured. A gap in the **config**, not the engine. |
| **F-NEW-4: `cheapest_move` steps through `Unknown` (a non-actionable state)**                                                         | UX/logic nuance, not structure.                                                                                                  |
| **F-NEW-6: SCIP runs the indexer twice per run** (perf)                                                                               | Runtime cost, not a static structural property.                                                                                  |

**Verdict:** archfit is **blind to semantic correctness, dead code, logic bugs,
and intent drift.** Every one of the expert's _Priority 1–2, 5, 7_ issues is a
class archfit structurally cannot detect. This is the boundary of a deterministic
structural tool — and exactly where the design says LLM/human review belongs.

## 3. Where the TOOL OVER-flags (false positives vs expert judgment)

- **13 `bc/imbalanced_coupling` medium advisories on `cmd/archfit → internal/*`.**
  The tool flags the composition root's functional coupling to internal packages
  as _medium unbalanced_. The expert treats wide fan-out from the composition root
  as **expected and correct** (that's a comp-root's job) — flagging F-MOD-2 only as
  a _testability/extraction_ nit, not a coupling violation.
  - Root cause 1: `BalanceResult` flags high-strength+high-distance as `medium`
    **regardless of volatility**, contradicting BC's "low volatility neutralizes
    unbalanced coupling." All 13 are `× low volatility` → should be neutralized.
  - Root cause 2: no notion that a `cmd`/composition-root layer is _expected_ to
    fan out; `code_structure` distance treats `cmd` vs `internal` as high distance.
  - **This is a precision (false-positive) problem the comparison exposes.**

## 4. Where the TOOL adds signal the expert DIDN'T (tool > expert)

- **Temporal coupling** (`change_coupling`, `hidden_coupling`): files that
  co-change without importing each other — the expert did no git-history analysis,
  so missed this entirely. (Caveat: see bug below.)
- **Quantified blast radius / propagation cost**: `PC=0.10`, per-module reach %.
  The expert reasoned qualitatively about hubs; the tool gives numbers and ranks.
- **Determinism & reproducibility**: byte-identical double-run + `config_hash` — a
  property an expert review can't provide.

## 5. Bugs the exercise found IN archfit's own v2 code

The self-run + expert review _together_ found **6 real defects in the rapidly-written
v2 engine** — the strongest validation that neither method alone is sufficient:

| #   | Defect                                                                                                  | Found by          | Severity |
| --- | ------------------------------------------------------------------------------------------------------- | ----------------- | -------- |
| 1   | `change_coupling` displays **CC=1000%** (formula maxes at 100% — scaling/display bug)                   | **tool self-run** | High     |
| 2   | `abstractness` reads **A=1.0 for 30/52 modules** (contract-inbound proxy saturates; non-discriminating) | **tool self-run** | Medium   |
| 3   | `codeStructureDistance` SameOwner for all flat module names                                             | expert            | High     |
| 4   | `derivedVolatility` computed-but-never-read (dead store, wasted work each run)                          | expert            | High     |
| 5   | `MultiplicativeScorer` wrong for `(contract, same_module, high_vol)` (guarded latent bug)               | expert            | Medium   |
| 6   | `MartinDistance`/`Abstractness` duplicated edge-walk (DRY)                                              | expert            | Medium   |

Tool found the two that show up as **absurd metric values** (1000%, A=1.0
everywhere). Expert found the four that are **invisible in output** (dead code,
guarded logic bugs, algorithm correctness). Complementary, not redundant.

## 6. Conclusion — what this says about archfit as a fitness tool

1. **archfit is a strong structural lens**: god-modules, hubs, fan-out, complexity,
   temporal coupling, blast radius — it agrees with expert structural judgment and
   adds quantification + reproducibility the human can't.
2. **archfit is blind to semantics**: dead code, logic/formula bugs, DRY, receiver
   semantics, ordering, intent drift. These need human/LLM review — which is why the
   design keeps `enrich`/`explain` (LLM, off-gate) for exactly this judgment layer.
3. **archfit currently over-flags the composition root**: the BC severity ignores
   volatility-neutralization, and there's no "comp-root is expected to fan out"
   notion. Precision needs work.
4. **A tool cannot fully audit itself**: 4 of 6 v2 defects were invisible to
   archfit's own run; the expert review caught them. Dogfooding + independent review
   are complementary, and both are needed.

### Recommended follow-ups (ranked)

1. **Fix `change_coupling` CC=1000%** (display/scaling) — tool produces an
   impossible value. (defect #1)
2. **Fix `codeStructureDistance` flat-name collapse** + the `(contract,same_module,
high_vol)` scorer guard — correctness of the new engine. (defects #3, #5)
3. **Neutralize low-volatility in `BalanceResult`** (or exempt composition-root /
   `cmd` layer) to stop the 13 false-positive comp-root advisories. (§3)
4. **Remove or wire `derivedVolatility`**; **dedupe** the Martin/Abstractness loop;
   reconsider the abstractness proxy (it saturates). (defects #4, #6, #2)
5. **Add a `forbidden_dependency` rule** for `engine → labels` (and consider
   `engine → config` I/O), so archfit's _own config_ catches F-COUP-1 next run —
   turning an expert finding into an automated gate.

This comparison report is the deliverable for the "run archfit + expert review +
compare" exercise. The underlying data: `/tmp/v2-self-report.md` (full scan),
`/tmp/v2-self-full.json`, and the expert review in the session transcript.
