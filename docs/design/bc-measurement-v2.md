# Balanced Coupling measurement engine — design v2.0

Date: 2026-06-18. Status: DRAFT — awaiting review. Supersedes the coupling-scoring
parts of `arch-fitness-architecture-v0.2.md` and `hybrid-llm-strength-v0.1.md`;
extends `agent-feedback-loop-v0.1.md`. Implementation plan to follow in
`docs/plans/` once approved.

## 1. Purpose

Make archfit's measurement engine a **faithful, deterministic instantiation of the
Balanced Coupling model** (Khononov) across Go, TypeScript, and Python, so an AI
coding agent gets architecture-level feedback the way a linter gives code-level
feedback. This design closes the gaps found in the 2026-06-18 self-analysis, adds
the standard coupling metrics the model leans on, and aligns the report with BC
vocabulary — without ever putting an LLM or non-determinism on the gate path.

### 1.1 Non-negotiables (carried from existing design, do not regress)

1. **Deterministic gate.** `check` is reproducible from repo state alone. No LLM,
   no clock, no network, no `Math.random`. Same commit + same config → same verdict.
2. **LLM is off-gate.** Only `enrich` (draft→human-review→pin) and `explain`
   (narrate) may call an LLM. `check` reads pinned config only.
3. **Core ring purity.** `classify`, `rules`, `metrics`, `model` stay free of
   `os`/`exec`/yaml; every subprocess goes through `toolrun.Runner`
   (`internal/arch_test.go` enforces this).
4. **Three languages, one model.** Every dimension must have a defined source per
   language or degrade to honest `unknown` — never a false zero.
5. **Report-only never gates.** New/uncertain signals are advisory until proven.

## 2. Source inputs

- Self-analysis run (`archfit scan`, v0.2.0-6-g4682765) and the v0.1.0→v0.2.0 delta.
- Gap analysis (this session), verified against `file:line`.
- Architecture-advisor opinion (additive scoring, deploy-unit detection, LLM-in-loop).
- Perplexity research: (a) OSS tooling per language; (b) continuous coupling-scoring
  formulas and how NDepend/Designite/Lattix/CodeScene/Martin quantify coupling.
- `methodology-balanced-coupling` skill (the canonical 3-dimension model).
- User decisions (2026-06-18): prototype-both-and-calibrate scoring; full scope
  (core + standard metrics + speculative signals report-only); hand-roll on the
  current toolset, add a tool only when a signal is otherwise unreachable;
  design-doc → execution-plan deliverable.

## 3. Current state — what's wrong, with evidence

| #   | Gap                                                                                                                                                                                                                                          | Evidence                                                                                               | Severity |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ | -------- |
| 1   | **Distance ignores code structure**; uses only owner + deploy. In a single/few-author repo the git-author fallback assigns one owner to every module, so every cross-module edge is `cross_module_same_owner` (low). Distance is a constant. | `classify.go:198-222`; `ownership.go:60-69,225-266` (git-author dominant-owner); no CODEOWNERS in repo | High     |
| 2   | **No deploy-unit detector.** `DeployUnit` is a config field only; the top distance level `cross_deploy_unit` is unreachable without hand-authoring.                                                                                          | `config.go:153`; no producer in `internal/extract/*`                                                   | High     |
| 3   | **Volatility falls back to git churn for the gate** — the one proxy BC forbids (accidental volatility/involatility).                                                                                                                         | `classify.go:224-253` → `effectiveModules()` fills churn band; `config/volatility.go:DeriveVolatility` | High     |
| 4   | **Formula discards ordinality.** 4 strength + 4 distance levels collapse to 2 bits.                                                                                                                                                          | `coupling.go:84-91` (`strengthIsHigh`/`distanceIsHigh`)                                                | High     |
| 5   | **XOR (modular) cases mis-graded** as `SeverityLow` instead of `SeverityNone`.                                                                                                                                                               | `coupling.go:137-138`                                                                                  | Medium   |
| 6   | **No connascence degrees; no runtime (sync/async) distance.** Generic-subdomain functional-vs-implementation volatility split is unrepresentable.                                                                                            | absent                                                                                                 | Medium   |
| 7   | **Report uses archfit jargon, not BC vocabulary**, and isn't structured as agent-actionable lint messages.                                                                                                                                   | `internal/engine` markdown writer                                                                      | Medium   |

archfit also already **exceeds** BC in breadth (cycle, blast_radius, change_amplification,
hidden_coupling, structural_weight, complexity, risk_hub, coverage). Those stay;
hidden_coupling is the best BC-aligned addition (implicit functional coupling).

## 4. Target measurement model

### 4.1 Integration strength (likelihood of cascade)

Keep the four levels (`coupling.go:11`). Per-language sourcing, deterministic:

| Source               | Go                                                                                                                | TS              | Python                    |
| -------------------- | ----------------------------------------------------------------------------------------------------------------- | --------------- | ------------------------- |
| Contract / Intrusive | config `public`/`internal` globs + export visibility                                                              | same            | same + `_name` underscore |
| Model / Functional   | SCIP symbol kind (Protocol/ABC/interface→contract; struct/class→model; func/method→functional; private→intrusive) | scip-typescript | scip-python               |
| Refinement           | `enrich` LLM draft → human review → pinned label (`reviewed_at`)                                                  | same            | same                      |

**Connascence sub-grade (report-only, not scored).** Connascence is _vocabulary to
explain why an edge is risky_, layered on top of strength — never a separate score
axis. Detect only the statically cheap, high-signal degrees; hand-rolled via
ast-grep/SCIP (no OSS connascence tool exists for these languages — confirmed):

- **CoName** — implicit in every import edge. Free.
- **CoType (CoT)** — cross-module struct/interface/field use → already `model` strength via SCIP hint; tag the edge `connascence: type`.
- **CoAlgorithm (CoA)** — clone pairs (existing clones/jscpd signal) that cross a module boundary → tag `connascence: algorithm`.
- **CoPosition / CoMeaning / CoValue** — _optional_, ast-grep heuristics (long positional arg lists; shared magic literals). Noisy → report-only, behind a flag, default off.
- CoExecution/Timing/Identity — require runtime traces → **skip**.

### 4.2 Distance (cost of cascade) — the core fix

BC distance = **code structure + organization + runtime**. archfit has org + deploy
only, and org degenerates. Make distance a **composite of independent, deterministic
signals**, taking the strongest applicable one, so distance always carries signal
even with a single owner:

```
distance(edge) = max(
    code_structure_distance(from.path, to.path),   # always available
    ownership_distance(from.owner, to.owner),      # only when CODEOWNERS or multi-author
    deploy_distance(from.unit, to.unit)            # only when deploy units detected
) + runtime_adjust(edge)                            # async bridge => +1 level
```

- **code_structure_distance** (NEW, the always-available baseline). Deterministic
  function of the two module paths' position in the package/directory tree:
  `same_module → 0`; sibling packages under a shared parent → low; the fewer path
  segments shared, the higher. Normalized by tree depth. This is what makes
  `internal/metrics/boundary` ↔ `internal/metrics/modularity` _closer_ than
  `cmd/archfit` ↔ `internal/extract/py`, which archfit cannot currently distinguish.
- **ownership_distance.** Keep `ownership.go` (CODEOWNERS preferred; git-author
  fallback). **Change:** when the resolver yields a single owner for the whole repo
  (degenerate), treat ownership distance as _unknown/zero contribution_, not "same
  owner everywhere = low" — let code-structure distance dominate. CODEOWNERS in a
  real multi-team repo restores the socio-technical signal.
- **deploy_distance** (NEW detector, `internal/extract/deployunit`). Sources, all
  static, priority order: Go `main` packages (`go list`), TS workspaces
  (`package.json` `workspaces`, `bin`/`main`), Python build targets (`pyproject.toml`
  `[project]`), `Dockerfile` locations, k8s `Deployment`/`StatefulSet`. Output
  `map[path]→unit`, merged via a new `FillMissingDeployUnits` mirroring
  `FillMissingOwners` (`config.go:545`). Unresolved → `unknown`.
- **runtime_adjust** (NEW, report-only in v1). ast-grep per language: Go `go`/chan +
  MQ imports (sarama/kafka-go/nats/amqp); TS `kafkajs`/`amqplib`/`@google-cloud/pubsub`
  - `@MessagePattern`; Python `asyncio`/`celery`/`dramatiq` + `pika`/`confluent_kafka`.
    Async bridge raises effective distance by one level (decouples lifecycle). A curated
    `library → integration-kind` table (YAML, off-gate config) drives it; absence of
    signal ⇒ `confidence: low`, not "synchronous".

Distance confidence is reported per module (`owner_source`, `deploy_unit_source`,
`code_structure: always`) so the agent knows how much to trust it.

### 4.3 Volatility (probability of change) — two axes

Split the single band into the two BC actually describes, and route them differently:

| Axis                          | Source                                                                                                                             | Used by                                                      | Why                                                                                                     |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| **Domain volatility**         | config `volatility` → `subdomain` (core/supporting/generic) → deterministic path-pattern heuristic → else `unknown`. **No churn.** | **the gate** (scoring, severity)                             | BC: essential volatility from the domain, not commits                                                   |
| **Implementation volatility** | git churn (`DeriveVolatility`) + gitnexus historical impact                                                                        | **report-only metrics** (`risk_hub`, `change_amplification`) | BC: generic subdomains have low _functional_ but variable _implementation_ volatility (provider-switch) |

Concretely: **remove churn from `classifyVolatility`'s fallback** (gap 3). classify
sees domain volatility only; when neither config nor heuristic resolves it, emit
`VolatilityUnknown` (conservative: no discount). Churn stays, but only feeds the
report-only metrics. This makes the generic-subdomain split expressible: a `generic`
module with high implementation volatility (churning, provider-switchable) shows
`domain=low / impl=high` and triggers a _contract-recommended_ advisory when reached
by non-contract strength — exactly BC's anti-corruption-layer guidance.

Domain volatility is the one place human-like judgment helps: `enrich --subdomains`
drafts a subdomain per module (LLM, from name/paths/sampled doc), human reviews,
pins to config with `reviewed_at`/`reviewed_by`. The gate reads the pin. Stale pins
warn via existing `staleness.go`.

## 5. Scoring — prototype both, calibrate, freeze

Introduce a `Scorer` port so the formula is swappable and testable; ship two impls,
calibrate, lock the winner as named constants.

```go
// internal/model/coupling (pure; no I/O)
type Scorer interface {
    Score(Classification) EdgeScore
}
type EdgeScore struct {
    Value       int       // 0..10, integer (determinism: no float on the gate)
    Band        Severity  // none|low|medium|high|critical from thresholds
    Reason      string    // machine-parseable: "intrusive(+8) cross_deploy(+5) vol_high(-0)=13→10"
    Breakdown   ScoreBreakdown
    CheapestMove string   // deterministic: the one dimension change that drops the band most
}
```

Both impls map the 4 ordinal levels to numbers (frozen tables, cited to BC):

```
strength: contract 0  model 2  functional 5  intrusive 8   unknown 3
distance: same_module 0  same_owner 1  cross_module 3  cross_deploy 5   unknown 2
          (+1 async bridge)
volatility: low / medium / high / unknown
```

- **Impl A — additive (advisor).** `raw = strength + distance − vol_discount{low2,med1,high0,unk0}`,
  clamp [0,10]. Flags structurally-bad edges even when stable. Trivial `CheapestMove`.
- **Impl B — multiplicative / BC-pure (Perplexity).** Map levels to [0,1]
  (strength/8, distance/5, vol{low0.2,med0.6,high1.0,unk0.5}); `R_mod = 1−|S−D|`
  (the continuous XOR); `R_edge = R_mod·V`; `score = round(10·R_edge)`; structural
  penalties (+cycle, +unstable-dep, +change-coupling≥65%); **intrusive floor** = at
  least `low` regardless of V (reconciles BC's "intrusive always implicit/fragile"
  with archfit's existing intrusive-always-surfaced rule, `coupling.go:106`).
  Volatility _masks_ stable imbalance exactly as BC says.

**Banding (both):** `0–2 none · 3–4 low · 5–6 medium (default gate) · 7–8 high · 9–10 critical`.

**Calibration protocol (decides the winner):**

1. Implement both behind `Scorer`; default stays today's `BalanceResult` until chosen.
2. Run on: archfit (Go) + one OSS TS repo + one OSS Python repo (each with real
   cross-module/cross-deploy coupling).
3. For a sampled set of edges, hand-judge "is this really architecturally bad?" and
   compare each impl's band. Score by agreement + false-positive rate on
   stable-but-ugly and cohesive-high-strength edges.
4. Lock the winner; freeze its tables as `const` with a BC-rationale comment; any
   table change is a breaking metric-version bump (`*.v2`).

**XOR fix (gap 5), independent of which impl wins:** the two modular quadrants
(high-strength+low-distance cohesive; low-strength+high-distance loose) return
`SeverityNone`. Replace `coupling.go:137-138`'s `SeverityLow`. Keep the
over-decoupled-volatile-seam (low+low+high-vol) at `medium`.

**Per-module rollup:** weighted mean of outbound edge scores (deploy-crossing ×3,
cross-module ×1.5, else ×1) **plus** a `p90` to expose one-bad-edge skew. Mean is
advisory; only explicit rules gate.

## 6. Additional dimensions (standard, deterministic, from the existing graph)

All derivable from data archfit already extracts (`go list` / dependency-cruiser /
grimp / SCIP / git). Clearly labeled in the report as **structural metrics that
support BC reasoning**, distinct from the three BC dimensions.

| Metric                          | Formula                                            | Source               | Band/role                                                                     |
| ------------------------------- | -------------------------------------------------- | -------------------- | ----------------------------------------------------------------------------- |
| **Instability**                 | `I = Ce/(Ca+Ce)`                                   | dep graph fan-in/out | report; feeds unstable-dep penalty                                            |
| **Abstractness**                | `A = abstract/(abstract+concrete)` types           | SCIP / exports       | report                                                                        |
| **Distance-from-main-sequence** | `Dms = abs(A+I−1)`                                 | derived              | report; `>0.5` = watch                                                        |
| **Unstable-dependency**         | `I(to) > I(from)`                                  | derived              | edge penalty (Designite smell)                                                |
| **Propagation cost**            | `PC = reachable_pairs/(N²−N)` (transitive closure) | dep graph            | system + per-module; the standard cascade metric                              |
| **Change coupling**             | `CC(A,B)=C_AB/min(C_A,C_B)`, flag `≥65%`           | git / gitnexus       | sharpens today's `hidden_coupling` with CodeScene's exact formula + threshold |

Note (carry the trap forward): raw Ca/Ce misclassifies shared data types (high Ca,
low I → "stable/good") — volatility and the strength label must moderate it. Already
handled by routing these as report-only and keeping strength/volatility on the gate.

## 7. LLM placement (strict)

| Stage                       | LLM?          | Role                                            |
| --------------------------- | ------------- | ----------------------------------------------- |
| `check` (gate)              | **never**     | reads pinned config + tool facts only           |
| `enrich --strength`         | yes, off-gate | draft strength labels → human review → pin      |
| `enrich --subdomains` (NEW) | yes, off-gate | draft subdomain/volatility → human review → pin |
| `explain <fingerprint>`     | yes, off-gate | narrate a finding + cheapest move in prose      |

Everything the LLM touches lands in config behind `reviewed_at`/`reviewed_by` and is
re-validated deterministically by the gate. Staleness gate already exists
(`staleness.go`). Determinism invariant unchanged.

## 8. Report — BC-aligned, agent-actionable

Markdown structured as lint messages an agent can parse without NLP. BC vocabulary
throughout (integration strength, distance, volatility, balanced/unbalanced,
cohesion, contract). One example advisory:

```
ARCHFIT[BC-UNBALANCED high] internal/payments/processor.go:42 → internal/users/repo.go
  integration strength: intrusive   distance: cross_deploy_unit   volatility: high (core)
  score: intrusive(+8) cross_deploy(+5) vol_high(-0) = 13→10/10 (critical)
  why: implementation-level coupling across a deploy boundary to a volatile core module
  cheapest move: lower strength intrusive→contract (−5) → extract a UserService contract
```

Report sections:

1. **Verdict** + `config_hash` (reproducibility) + tool/coverage.
2. **Gate violations** (rules) — `RULE_ID [sev] from→to · why · fix · evidence`.
3. **Balanced Coupling advisories** — per-edge scored, lint-message format above.
4. **The three dimensions** — distribution of strength/distance/volatility across edges.
5. **Supporting structural metrics** (clearly labeled "beyond Balanced Coupling"):
   cycle, blast_radius, propagation_cost, instability/abstractness/Dms,
   change_coupling, structural_weight, complexity, risk_hub, coverage.
6. **Distance confidence** — owner_source, deploy_unit_source, unresolved modules.
7. **`agent_tasks`** — existing repair blocks + SARIF (unchanged).

## 9. Determinism & cross-language parity

- Integer math on the gate; float only in report-only rollups.
- Every new detector (deploy-unit, runtime, code-distance) reads static repo state
  via `toolrun.Runner` — no clock/network/random. `internal/arch_test.go` extended
  to forbid `os`/`exec` in the new core code.
- Parity rule: each dimension defines a source per language _or_ returns `unknown`.
  A language with no source for a signal degrades that signal to report-only
  `unknown`, never a false zero; coverage block records it (as today).
- New metrics get `metric_version` strings; golden-output test regenerated
  deliberately with an inspected diff (existing `TestGolden` discipline).

## 10. Fitness checks / tests

- **Scorer**: table-driven tests over the 4×4×4 strength/distance/volatility matrix
  for both impls; assert XOR quadrants = none; assert intrusive floor; assert clamp.
- **Distance**: code_structure_distance unit tests (sibling vs distant trees);
  degenerate-single-owner test (owner contribution suppressed); deploy-unit detector
  per language fixtures.
- **Volatility**: gate sees no churn (assert `classifyVolatility` ignores churn store);
  generic-subdomain-high-impl-volatility triggers contract advisory.
- **Determinism**: same-repo twice → identical JSON (already a property); add the new
  detectors to it.
- **Calibration harness**: a `make calibrate` target running both scorers on the
  sample repos, emitting an agreement report (not a CI gate).
- **Golden**: regenerate after the report restructure; inspect diff.

## 11. Risks & trade-offs

1. **Score churn breaks baselines.** Every existing score/band shifts. Mitigation:
   `*.v2` metric versions; regenerate baselines deliberately; it's a v0.x tool.
2. **Calibration is judgment-laden.** Two repos per language is thin. Mitigation:
   document the sample, keep weights as named consts, allow re-calibration without
   code change to thresholds.
3. **Deploy-unit false positives** in monorepos with many Dockerfiles. Mitigation:
   `deploy_unit_confidence: inferred|config`; config always wins; report the source.
4. **Runtime/connascence false negatives** (wrapped MQ clients, semantic connascence).
   Mitigation: report-only, `confidence: low`, explicitly "absence of evidence ≠
   synchronous/decoupled". Never gates in v1.
5. **code_structure_distance vs deploy reality.** Two files in the same directory but
   different deploy units exist. Mitigation: `max()` over signals means deploy wins
   when detected; code-distance is the floor, not the ceiling.
6. **Cross-language strength sparsity for Go** (few model/functional hints) keeps
   encapsulation n/a. Accepted — it's a language property, documented, not a defect.

## 12. Self-review

- **Critical issues:** none.
- **Significant:** (a) the additive impl violates BC's volatility-masking; resolved by
  making the choice explicit via calibration rather than asserting one. (b) Suppressing
  degenerate single-owner ownership could hide a genuine one-team-owns-everything signal;
  acceptable because code-structure distance covers it and CODEOWNERS overrides.
- **Minor:** async-bridge raising distance is a heuristic (async lowers cascade
  _likelihood_ but lowers _visibility_); chosen conservative (+1), config-overridable,
  report-only in v1.
- **Open questions — RESOLVED** (2026-06-18, via Perplexity + advisor; see plan
  `docs/plans/20260618-bc-measurement-v2.md` "Resolved decisions"):
  (1) Calibration repos → TS `redwoodjs/redwood` (api+web deploy units), Py
  `saleor/saleor` (airflow too heavy for repeated indexing); Go archfit.
  (2) §6 metrics → ON by default (report-only, never gate; one-time golden update).
  (3) Scoring break → one-shot `*.v2` bump + baseline reset, shipped as **v0.3.0**,
  framed as breaking _scoring_ not _verdict_; no dual-emit, no flag.

## 13. Handoff

Recommended next: **architecture-plan** — sequence this into a phased, verifiable
execution plan in `docs/plans/`, citing this design's `file:line` evidence. Suggested
phases (low-risk first):

- **P1 (correctness, no new data):** XOR fix; remove churn from gate volatility;
  domain-volatility path-pattern heuristic; `Scorer` port + both impls behind it.
- **P2 (distance signal):** code_structure_distance; degenerate-owner suppression;
  `internal/extract/deployunit` (+ `FillMissingDeployUnits`). Establish golden + calibration harness.
- **P3 (standard metrics):** Instability/Abstractness/Dms, propagation_cost,
  change_coupling formula. Report-only.
- **P4 (speculative, report-only):** runtime async detection; connascence CoT/CoA;
  `enrich --subdomains`.
- **P5 (report + calibration decision):** BC-aligned report restructure; run
  calibration; lock the winning Scorer; regenerate golden + baselines.

Do not start implementation until this design is approved.
