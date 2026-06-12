# Three-way review comparison — archfit vs architect skill vs modularity skill

Date: 2026-06-11. Target: archfit itself (main @ post-completion-merge).
Inputs: archfit self-scan (SCIP on, 13 metrics, facts block), an
architect-methodology review (5 scored dimensions, 5 findings), a Balanced
Coupling modularity review (7 findings). Purpose: find archfit's blind spots
and derive improvements.

## 1. Where the three agree

| Human finding                                                           | archfit signal that points at the same thing                                                                   |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `cmd/archfit/main.go` god-file, mixed responsibilities (architect F-05) | `structural_weight`: cmd/archfit 885 LOC, 4× median; facts: cmd outbound destinations 26 (highest)             |
| `internal/metrics` size/extension hot spot (modularity M1 context)      | `structural_weight`: metrics 2075 LOC, 10× median (top god-module)                                             |
| `engine`/`metrics`/`cmd` churn together (architect change-locality)     | `hidden_coupling`: engine (8 pairs), metrics (6); facts co-change partners                                     |
| Model packages are clean, rings mostly hold (both reviews positive)     | `cycle` 0, `unbalanced_edge` 0, layer gate findings baselined and visible                                      |
| engine→scope layer inversion                                            | archfit's own gate found it first (5 `layer_inversion` findings, baselined with a documented Phase-2 fix note) |

The numeric signals (LOC skew, outbound fan-out, co-change) reliably point at
the same _locations_ the human reviews flag. What archfit does not produce is
the _diagnosis_ — "this is a composition root carrying install logic",
"orchestrator captured by subordinate domain".

## 2. What the human reviews found that archfit missed — and why

| #   | Finding (source)                                                                 | Why archfit missed it                                                                                                                                           | Gap class                                                                                           |
| --- | -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| 1   | Adapters import `engine` — port types live in the orchestrator (architect F-01)  | Self-config layer model allows outer→inner imports; the issue is _where types live_, a hexagonal-purity judgment. `arch_test.go` also lacks the rule.           | **Config gap** (add `forbidden_dependency extract/** → engine/**` after extracting a ports package) |
| 2   | `scope` imports `history/git` directly (architect F-02)                          | Same-layer import — allowed by the layer model; "scope is core-adjacent" is a classification judgment.                                                          | **Config granularity**                                                                              |
| 3   | `engine.Run` 14 params; `runPipeline` 94 lines (architect F-03)                  | No function-level signals in the default run — `complexity` (lizard) is OFF in the self-config.                                                                 | **Config gap** (enable complexity) + missing param-count signal                                     |
| 4   | SCIP `Resolve()` identity stub, confidence ladder unimplemented (architect F-04) | Behavioral stub — invisible to any structural analysis.                                                                                                         | **Tool feature gap** (fix Resolve 3-state confidence)                                               |
| 5   | `engine`←`metrics` input-type capture (modularity M1)                            | Dependency direction is allowed; "orchestrator should not depend on subordinate's types" is semantic.                                                           | **LLM-layer class**                                                                                 |
| 6   | `status`→`baseline` ring inversion (modularity M2)                               | Self-config classifies `baseline` as `support` (rank 1, below core) — the _config itself_ encodes the disputed judgment.                                        | **Config modeling** — exactly what Tranche-2 enrich should question                                 |
| 7   | `PatternMatch` duplicated in engine + rules (modularity M4)                      | Clone detection (`jscpd`) is OFF; struct-level duplication is below the file-level radar anyway.                                                                | **Config gap** + clone-tool granularity                                                             |
| 8   | `initcfg`/`config` implicit YAML contract (modularity M5)                        | `hidden_coupling` carries the right signal class (co-change, no edge) but the pair didn't rank in top display; the _contract_ judgment is semantic.             | **LLM-layer class** + cheap fitness fix (round-trip test)                                           |
| 9   | `sourceloc`/`complexityFuncs` misplaced in `cmd` (modularity M6)                 | "Should live in extract/" is a convention judgment. Facts DO show cmd as outbound hub — the same shape the Tranche-1.5 blind classifier used to spot grab-bags. | **LLM-layer class** (facts + code reading)                                                          |
| 10  | `metrics.New`/`ApplyVolatility` temporal ordering contract (modularity M7)       | Execution-order coupling — invisible to import/co-change analysis entirely.                                                                                     | **LLM-layer class** (code reading only)                                                             |

**False positive both external reviews repeated:** `classifyStrength` raw-map
iteration was flagged as nondeterministic by BOTH reviews. Verified twice now:
the two-pass structure (all Public globs before any Internal glob) makes the
returned strength order-independent. archfit is right not to flag it — but two
independent reviewers tripping on the same code means it should use the sorted
index anyway (cheap hardening, silences a recurring alarm).

## 3. Conclusions

1. **The hybrid thesis is confirmed from the other side.** Every genuinely
   missed finding (rows 5–10) is the "architect POV" class: judgments about
   intent, ownership, contracts, and placement. archfit's facts/metrics put a
   finger on the right locations; the diagnosis layer is exactly what Tranche 2
   (enrich + explain over facts + code) is being built for. None of the missed
   findings argues for a new deterministic ranking metric — that approach
   already failed its gate once (cohesion_spread/shared_state_hub).
2. **Half the gap is config, not capability.** Rows 1–3, 6, 7 vanish or shrink
   with a better self-config (complexity on, clones on, an extract→engine
   forbidden rule, a reviewed layer classification for baseline/scope). This is
   itself a product lesson: `archfit init` should generate stricter starter
   rules, and the docs should teach config curation as the main tuning loop.
3. **Determinism guarantee held up.** Neither review found an actual
   output-nondeterminism; the one claim was a false positive. The byte-identical
   sweep stands.

## 4. Improvement backlog (derived)

Deterministic / cheap (fold into Tranche-2 plan as Task 0 or do alongside):

- [ ] self-config: enable `tools.complexity`; add `forbidden_dependency`
      extract/**→engine/** (after ports extraction below); revisit baseline's
      layer
- [ ] `classifyStrength`: iterate the sorted module index (hardening)
- [ ] SCIP `Resolve()`: 3-state confidence (low when tool absent / medium
      unresolved / high resolved-when-implemented) + partial coverage record
- [ ] LOC walk emits a `diagnostic.Coverage` record (only collector without one)
- [ ] `initcfg` round-trip fitness test (Render → config.Load → assert fields)

Refactoring backlog on archfit's own architecture (sequence AFTER Tranche 2;
each is behavior-preserving) — **ALL CLOSED 2026-06-12**, plan
`docs/plans/completed/20260612-archfit-refactoring-backlog.md`:

- [x] extract port types (`PatternMatch`, port interfaces) to a neutral package
      consumed by engine + adapters; then add engine to `adapterPrefixes` in
      arch_test — `internal/ports` + `internal/model/pattern`
- [x] move `MetricInput`/`ChangeHistory` carrier types toward `model/`
      (un-captures engine from metrics) — `internal/model/signal`
- [x] `status` takes a baseline-reader interface (ring direction) —
      `status.AcceptedSet`; baseline reclassified adapter
- [x] `engine.Run` options struct (14 params → 1) — `RunInput`; Run CCN 28→17
- [x] split `cmd/archfit/main.go` by command; move `sourceloc`/`complexity`
      collectors into `internal/extract/` — loc + complexity packages
- [x] `metrics.New` takes resolved volatility values (kills the ordering
      contract) — resolved structurally instead: two-store volatility
      (`Config.derivedVolatility`), `ModuleDef.Volatility` stays pristine
- [x] (bonus) scope decoupled from git via injected Resolver, reclassified
      support — all 5 baselined engine→scope inversions resolved; the
      self-scan baseline is now EMPTY

LLM-layer requirements confirmed for Tranche 2 (no scope change needed):
enrich/explain must consume the facts block + read code — the missed-findings
profile matches the already-designed workflow.
