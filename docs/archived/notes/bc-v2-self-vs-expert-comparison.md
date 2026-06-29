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

## 7. Fix status (2026-06-18)

The three highest-value defects were fixed and verified this session; the rest are
a prioritized backlog (they need design judgment or refactoring, not patches, and
were deferred rather than rushed).

**Fixed & committed (verified: self-scan went 13 → 0 warnings, suite + lint green):**

- ✅ **defect #1 — `change_coupling` CC>100%** (`c8dfb5b`): module-level `C_AB`
  was the _sum_ of file-pair co-changes (over-counts one commit into many file
  pairs). Now uses the _max_ file-pair co-change as a bounded module proxy, clamped
  to 1.0, with the approximation + upgrade-trigger documented.
- ✅ **defect #5 — scorer same-module bug** (`c8dfb5b`): both scorers now return 0
  for `same_module` edges (not cross-boundary coupling), fixing the latent
  `(contract, same_module, high_vol)=critical` and `(intrusive, same_module)` cases.
- ✅ **§3 — composition-root false positives** (`3f7a2a4`): `BalanceResult` now
  neutralizes `high-strength + high-distance + low-volatility` to advisory `low`
  per BC (a stable target rarely fires the cascade). This removed all 13 false
  `cmd→internal` medium advisories; verdict unchanged (still pass).

**Also fixed (second round, advisor + Perplexity grounded):**

- ✅ **defect #2 — abstractness saturation** (`1874018`): switched the metric from
  the glob-classified strength (which marks every exported Go API "contract") to
  the raw SCIP `StrengthHint` (interface/protocol→contract, struct→model), with a
  fallback to the classified strength when no hint. archfit's A went from a
  saturated 1.0-for-30-modules to a discriminating 0.00 (it is a concrete-heavy Go
  codebase — the honest answer), and `martin_distance` now correctly flags
  concrete+stable "zone of pain" modules. `martin.go computeAbstractnessMap`.
- ✅ **defect #6 — Martin/Abstractness DRY** (`1874018`): both metrics now share
  the single `computeAbstractnessMap` helper.
- ✅ **defect #4 — `derivedVolatility` dead store** (`41825b9`): removed per YAGNI
  — 320 lines of the churn-banding chain (`DeriveVolatility`/`ApplyVolatility`/the
  field/the pipeline call) deleted, plus stale comments. Confirmed no reader;
  `change_amplification`/`risk_hub` use their own inputs. Two now-moot tests
  rewritten to assert the still-true invariants without the removed machinery.

**Still open — designs ready for a fresh focused session (both are high-blast):**

- ⏳ **defect #3 — flat-name distance (composite-precedence redesign)**: the correct
  fix (advisor + Perplexity): explicit config-declared `owner`/`deploy_unit` must
  take **precedence** over the code-structure default (not lose to it via `max()`),
  and `codeStructureDistance` should return `DiffOwner` for flat single-segment
  names. This requires tracking which owners are **hand-authored vs git-author
  resolver-filled** (an `explicitOwners` set populated in `config.Load`), threaded
  through `ClassifyConfig` into `classifyDistance` as a precedence chain
  (deploy boundary → explicit ownership → resolver multi-owner → code-structure
  fallback). Deferred because it touches the **gate's distance signal** and the
  test fixtures construct `Config` directly (bypassing `Load`), so the
  `explicitOwners` plumbing needs a test-friendly seam. `distance_structure.go`,
  `classify.go classifyDistance`, `config.go Load/ForClassify/ClassifyConfig`.
- ⏳ **engine→labels boundary**: split `internal/labels` into pure (`Key`,
  `Approved`, `HashItems`, types — no os/yaml) + `internal/labels/labelsio`
  (`Load`); point `cmd` at `labelsio`; add a `forbidden_dependency` rule
  (`engine → labelsio`) to `.archfit.yaml` and a boundary assertion to
  `internal/arch_test.go`. Attempted this session; reverted after the package split
  surfaced a `go.mod` (`gopkg.in/yaml.v2` "implicitly required") complication that
  needs a `go mod tidy` pass best done fresh. Clean and mechanical, just not worth
  rushing at the tail of a long session.

This comparison report is the deliverable for the "run archfit + expert review +
compare" exercise. The underlying data: `/tmp/v2-self-report.md` (full scan),
`/tmp/v2-self-full.json`, and the expert review in the session transcript.
