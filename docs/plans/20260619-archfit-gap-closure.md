# archfit gap-closure: reliable, deterministic, multi-language architecture evaluation

> **Executable ralphex plan.** Run with `ralphex` from the repo root. ralphex executes
> each `### Task` sequentially in an isolated subagent; every task must end green
> (build + `make test` + `make lint`) before the next starts. ralphex CLI is installed
> (`/opt/homebrew/bin/ralphex`). ralphex moves the plan to `docs/plans/completed/` on
> completion.

## Overview

Close the gap between `archfit` (deterministic CLI) and the `architecture:architect`
skill (7-dimension evidence+LLM review), and make archfit's evaluation **correct,
deterministic, and meaningful across all three supported languages** (Go, Python,
TypeScript). Driven by a measured baseline: we ran both archfit and an independent
architect review on three real repos — archfit (Go), ccgram (Python), codegraph (TS) —
and catalogued exactly where archfit is blind and why.

Outcome: every metric archfit prints is correct or honestly absent; the BC advisory
flood is collapsed; per-language node-IDs/coverage/calibration are consistent; archfit
emits a banded scorecard aligned to the architect rubric; an off-gate grounded LLM
review (`archfit review --llm`) narrates and prioritizes strictly from collected
evidence; and a final validation re-runs **architect skill vs improved archfit on all
three projects** to prove the blind-zone reduction — with deterministic output.

## Context (from discovery)

Baseline three-language matrix (HEAD of each repo):

| Signal                  | Go (archfit) | Python (ccgram)                          | TS (codegraph)                      |
| ----------------------- | ------------ | ---------------------------------------- | ----------------------------------- |
| Verdict                 | pass         | fail (3 cycles gated)                    | pass (2 cycles NOT gated)           |
| scip                    | ok           | ok                                       | absent (no node_modules)            |
| lizard→complexity       | ok           | n/a                                      | n/a                                 |
| gitnexus                | n/a          | absent (index present, opt-in off)       | absent                              |
| encapsulation           | n/a          | 7.8/10                                   | n/a                                 |
| abstractness            | A=0          | A=0                                      | A>0.5 ×110                          |
| martin "zone of pain"   | leaf pkgs    | leaf modules                             | node builtins (fs/crypto/commander) |
| risk_hub                | 87/133       | 158/383                                  | n/a                                 |
| change_locality (delta) | n/a          | **0 "high confidence" (false)**          | n/a                                 |
| BC advisories           | 0            | **421 near-identical, lower_volatility** | 0                                   |
| Expert cycles found     | —            | 40+ (hidden by 307 lazy imports)         | hidden cross-internal cycle         |

Six blind-spot categories (root causes, with file:line):

1. **Coverage-driven** — scip needs node_modules (`ts.go:87`); lizard absent→complexity n/a; gitnexus opt-in ignores present index (`risk_hub.go:182`); clones opt-in off→functional_candidates n/a.
2. **Node-ID inconsistency** — Python `module:` IDs (`py.go:232,262`) vs `change_locality` hardcoded `"file:"+f` (`change_locality.go:97`) → false 0; TS leaks unresolved deps as `file:<name>` nodes (`ts.go:220-223`).
3. **Distance/calibration** — dotted Python names collapse to `DiffOwner` (`distance_structure.go:37`) → 421 flood; abstractness/martin not language-aware; risk_hub flags 65-100% of modules.
4. **Semantic/dynamic** — Python lazy/dynamic imports hide 40+ cycles; TS lazy `require()` undercounts fan-out; type-only vs runtime conflated; dead fields, formula bugs (deterministic boundary).
5. **No synthesis** — flat metrics as `display` strings (`diagnostic.go:17`), every `band:info`, advisories never gate (`engine.go:398`); no banded dimensions/score.
6. **Config-as-truth** — minimal config → noise; same code passes/fails by config; no declared-vs-observed drift detection.

Key mechanism facts (for executor grounding):

- Metrics registry + filter: `internal/metrics/metrics.go:59-92` (all run unless `metrics.<name>.enabled:false`).
- Verdict: `internal/engine/engine.go:398-410` (gate findings fail; metric delta<0 warns; advisories never gate).
- Full evidence bundle (LLM seam): `diagnostic.Diagnostic` assembled at `internal/engine/engine.go:216-233`; LLM port `internal/llm/llm.go:34` (`Complete(system,user)`); `enrich.go`/`explain.go` already cite Balanced Coupling.
- Architect rubric: `~/.claude/plugins/.../architecture/0.3.2/templates/scorecard.yaml` (7 dimensions, bands 0-100, confidence caps, evidence-per-score) — archfit spec already references it (`docs/spec/arch-fitness-spec-v0.4.md:1553`).

## Development Approach

- **Testing approach**: Regular (code first, then tests), matching the repo's `go test -race` convention — but tests are a **required deliverable of every task**.
- One logical unit per task; small focused changes; backward compatible.
- **CRITICAL — determinism is a first-class acceptance criterion** for every task that changes output:
  - no wall-clock, no map-iteration-order leakage, no randomness in any emitted field;
  - a byte-identical double-run (`archfit check … > a; archfit check … > b; diff a b`) must hold;
  - `config_hash` stable for unchanged config;
  - any intended output change bumps the relevant `metric_version` and regenerates the golden (`internal/engine/golden_test.go`) **deliberately**, inspecting the diff.
- **CRITICAL — every task ends with tests passing** (`make test`) and lint clean (`make lint`) before the next task. Core-ring import invariants (`internal/arch_test.go`) and golden must stay green.
- Update this plan file as scope shifts (➕ new task, ⚠️ blocker).

## Testing Strategy

- **Unit tests**: required per task — success + error + per-language cases (Go/Python/TS fixtures) for any extractor/metric/classify change.
- **Determinism tests**: double-run byte-identical assertion added to the engine/integration test layer; golden regen is deliberate.
- **Validation tasks**: end-to-end re-runs on the three real repos + independent architect-skill review + comparison (Tasks 6, 19, 22).
- No new gate may depend on the LLM (`internal/arch_test.go` LLM-off-gate invariant must hold).

## Progress Tracking

- Mark `[x]` immediately when done; ➕ for new tasks; ⚠️ for blockers; keep in sync.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, docs achievable in-repo.
- **Post-Completion** (no checkboxes): API key provisioning, manual review, release.

## Balanced Coupling fidelity (Vlad Khononov) — non-negotiable

Every BC-related metric, formula, severity, and piece of user-facing language MUST
stay faithful to Vlad Khononov's Balanced Coupling model (coupling.dev; _Balancing
Coupling in Software Design_, 2024; _Learning Domain-Driven Design_). Additional
architecture metrics (Martin I/A, propagation cost, blast radius, risk_hub, etc.) are
allowed but MUST be clearly labelled **"supporting / non-BC"** and never relabelled in
BC vocabulary.

The model archfit must speak (use these exact terms):

- **Integration strength** = how much _knowledge_ two components share. Levels,
  strongest→weakest: **Intrusive → Functional → Model → Contract**. Map connascence to
  strength: static name/type/signature → Functional; shared domain model/meaning →
  Model; shared internals/state/DB/reflection → Intrusive; stable schema/event/DTO →
  Contract (weakest). Contract is the goal for high-distance integration.
- **Distance** = socio-technical separation, **not** just code-tree distance: code
  proximity + runtime/deployment independence + ownership (team) + **shared lifecycle**.
  It is "how many boundaries a change must cross." Ordinal: local (same module/
  deployable/team/lifecycle) → internal-remote (different module/service, often
  different team) → external (different system/org). archfit's existing chain
  (same-module → cross-module-same-owner → cross-module-diff-owner → cross-deploy-unit)
  is the implementation of this — keep it framed as Vlad's distance.
- **Volatility** = likelihood/frequency a component changes. Map DDD subdomains:
  **Core → high, Supporting → medium, Generic → low** (default; allow per-module
  override). Low volatility _neutralizes_ otherwise-unbalanced coupling.
- **The balance rule (qualitative heuristic — Vlad publishes no literal equation).**
  Coupling is _balanced_ when integration strength and distance counterbalance, or when
  volatility is low. Worst case to flag first: **high strength + high distance + high
  volatility** → cascading changes → **distributed monolith**. Acceptable: **high
  strength + low distance + shared lifecycle = cohesion (good — do not "decouple" it)**;
  **low strength + high distance = loose coupling (good)**; low volatility makes strong/
  distant coupling tolerable. The cheapest balancing move is to change ONE dimension
  (lower strength via a contract, lower distance via co-location, or confirm low
  volatility) — never a rewrite.
- **Maintenance effort / the "one formula".** coupling.dev frames a single formula:
  maintenance effort rises monotonically with strength, distance, and volatility.
  archfit's numeric BC score (`scorer_multiplicative` ≈ `Effort ∝ Strength × Distance ×
Volatility`) is **archfit's deterministic implementation of Vlad's qualitative
  heuristic, not his literal equation** — say so in code comments, docs, and the score's
  `definition`. Ordinals: Strength {Contract 1, Model 2, Functional 3, Intrusive 4};
  Distance {local 1 → external 3}; Volatility {low 1, medium 2, high 3}. Severity bands
  must encode the balance rule (low volatility neutralizes; cohesion is low severity).
- **Signature vocabulary to adopt** in advisories, `explain`, `review`, and docs:
  _coupling = connection (not inherently bad)_, _modularity_, _cohesion ("the good
  coupling")_, _maintenance effort_, _cascading changes_, _shared lifecycle_, _knowledge
  boundaries_, _co-evolution_, _global vs local complexity_, _distributed monolith_,
  _big ball of mud_. Avoid generic "decouple everything" language.

Acceptance for any BC task: terminology, ordinals, severity, and balancing-move advice
match the rules above; the score `definition`/docs explicitly attribute the formula as
archfit's implementation of Vlad's qualitative model.

## Implementation Steps

### Phase 1 — Trust the output (P0 correctness + determinism)

### Task 1: Fix change_locality node-ID mismatch (Python false-zero)

- [x] in `internal/metrics/boundary/change_locality.go:85-130`, build BFS start keys + crossEdge keys by matching `graph.NodePath(n.ID())` to changed file paths instead of hardcoding `"file:"+f`
- [x] handle all node-ID schemes (Go `file:`, Python `module:`, TS `file:`) via the graph's own path mapping
- [x] add fixtures with `module:`-prefixed (Python) and `file:`-prefixed (Go/TS) graphs; assert non-zero reach + crossEdges when changed files participate in edges
- [x] add an error/edge case: changed file with no edges → genuine 0, distinguished from unmatched
- [x] run `make test` — must pass before next task

### Task 2: Emit the numeric BC score (currently computed then discarded)

- [x] in `internal/engine/engine.go:345-356` add `score_value` and `score_band` to advisory `MatchedBy` (keep `score`=scorer name)
- [x] in `internal/output/markdown/markdown.go:200-210` render `score: <value>/10 (<band>) [<scorer>]`; add the fields to JSON via existing struct
- [x] write tests asserting value+band reach markdown and JSON outputs
- [x] regenerate golden deliberately; inspect diff (no stored golden fixture — `TestGolden_DoubleRun` is a determinism double-run; new fields are deterministic, test stays green)
- [x] run `make test` + `make lint` — must pass before next task

### Task 3: Exclude external/unresolved nodes from first-party metrics

- [x] in `internal/extract/ts/ts.go:205-225`, do not emit unresolved/`CouldNotResolve` non-core targets as first-party `file:` nodes (or mark them external) — now emitted as `NodeKindExternal` nodes; edge kept (never `uses_internal`)
- [x] in `internal/metrics/modularity/martin.go:20-65,354-362`, exclude external/unresolved modules from the `firstParty` set used by instability/abstractness/martin — via new shared `modgraph.FirstPartyModules` (also used by `BlastRadius`)
- [x] write tests asserting node builtins / unresolved npm names (`fs`, `commander`) never appear in martin_distance / instability output — `TestExtract_ExternalNodes` (ts) + `TestMartin_ExternalNodesExcluded` (martin)
- [x] run `make test` — must pass before next task

### Task 4: Distance for dotted/flat module names

- [x] in `internal/classify/distance_structure.go:28-45`, treat dotted names (`a.b.c`) structurally (split on `.` as well as `/`) so dotted siblings are not all collapsed to `DiffOwner` — new `moduleSegments` splits on `/` when present, else `.` (slash wins, so dotted filenames like `src/utils.ts` keep their slash shape)
- [x] preserve the v0.3.0 explicit-owner precedence chain (`classify.go:275-312`); keep the chain framed as Vlad's **socio-technical distance** (code proximity → ownership/team → deploy/lifecycle), not bare tree distance — change is confined to the step-4 code-structure fallback; doc comment now frames it as the code-proximity dimension of Vlad's socio-technical distance
- [x] write table tests: dotted siblings → SameOwner; distant dotted trees → DiffOwner; flat single names unchanged
- [x] run `make test` — must pass before next task (passes; `make lint` 0 issues)

### Task 5: Dedup/aggregate BC advisories into grouped rollups

- [x] in `internal/engine/engine.go:328-360`, group advisories by `(fromModule,toModule,strength,distance,volatility)` with a stable count + representative IDs (deterministic ordering) — new `groupBCAdvisories`/`rollupFinding` run at the end of `collectAdvisories` (after status assignment, so baseline/excepted edges are not folded into "new" rollups); status is part of the key; rollups emitted in sorted key order; `group_count` + up to `bcAdvisoryRollupCap`=8 sorted member IDs in `MatchedBy`; all member `Locations` merged (deduped, sorted) so no file:line evidence is lost
- [x] update markdown + JSON renderers to show grouped rollups with counts, not one line per edge; phrase each rollup in Vlad's vocabulary, and never flag cohesion (high strength + low distance) as a problem — markdown header now `(N rollups, M edges)` + per-rollup `rollup: N same-shape edges (e.g. <ids>)` line; `bcAdvisoryWhy` now names the risk in Vlad's terms via `bcRiskClause` (critical → "cascading changes across a knowledge boundary → distributed-monolith risk"); cohesion never reaches the advisory pass (`coupling.BalanceResult` returns SeverityNone for high-strength+low-distance); JSON flows grouped findings through the generic Diagnostic marshal — `group_count`/`group_members` ride in `MatchedBy`
- [x] write tests: N identical-shape edges → 1 grouped advisory with count N; ordering deterministic — `TestRun_Advisory_GroupedRollups` (5 same-shape edges → 1 rollup, sorted members, merged locations, byte-identical double-run); markdown `TestRenderer_Render_BCRollup` + `TestRenderer_Render_BCNoRollupLineWhenSingle`
- [x] regenerate golden deliberately — no stored golden fixture; `TestGolden_DoubleRun` is a determinism double-run and stays green (grouping output is deterministic)
- [x] run `make test` + `make lint` — must pass before next task (full suite passes, `make lint` 0 issues, core-ring import invariant green)

### Task 6: Phase-1 validation run (correctness + determinism gate)

- [x] `make build`; run full+delta on all three repos (Go self, `~/Workspace/ccgram`, `~/Workspace/codegraph`) — delta bases Go `HEAD~5`, ccgram `HEAD~30` (221 changed files), codegraph `HEAD~30`
- [x] assert: ccgram delta `change_locality` ≠ false 0 (now **340** cross-module edges); martin lists no node builtins (codegraph + Go clean); advisories rendered as rollups (ccgram 421 edges → 58 rollups); BC score value/band visible (58 rollups carry `score_value`+`score_band`, markdown `score: 5/10 (medium)`). **Two residual bugs exposed and fixed** — src-layout false-0 in `change_locality.go` (Task-1 ceiling) and node-builtin source-entry leak in `ts.go` (Task-3 incomplete); both with regression tests
- [x] assert determinism: all six JSON double-runs byte-identical; `config_hash` stable per repo and identical full==delta
- [x] record results into `docs/plans/notes/gap-closure-phase1.md` (no checkbox files outside plan)
- [x] run `make test` — passes (race, full suite); `make lint` 0 issues; core-ring import + golden gates green

### Phase 2 — Reliability, meaning, and the architect bridge (P1)

### Task 7: Validate config enums at load

- [ ] in `internal/config/config.go:279` validate `bc_advisory_min_severity` ∈ {low,medium,high,critical} and rule `gate` values; return descriptive errors
- [ ] write tests for valid + invalid values (invalid → error, not silent pass-all)
- [ ] run `make test` — must pass before next task

### Task 8: TypeScript edge fidelity (type-only vs runtime)

- [ ] parse dependency-cruiser `dependencyTypes` in `internal/extract/ts/ts.go`; tag `type-only` and `dynamic` edges
- [ ] map `type-only` edges toward **Contract** integration strength (weakest), and value/runtime imports toward Functional/Model, per Vlad's strength ladder (Intrusive→Functional→Model→Contract)
- [ ] write tests with dep-cruiser JSON fixtures covering type-only, dynamic, value imports
- [ ] run `make test` — must pass before next task

### Task 9: Dynamic/lazy import detection + flag-as-risk (detect only, no graph edges)

- [ ] add `internal/extract/dynimports/` (mirror `internal/extract/runtime`): ast-grep pass for Python in-function `import`/`importlib`/`__import__` and TS `require()`/dynamic `import()`
- [ ] emit a report-only `hidden_cycle_risk` / `dynamic_coupling` signal (count per module + sample sites); do **not** modify the dependency graph or existing metrics
- [ ] surface it in markdown/JSON and feed it to the review bundle (Task 17)
- [ ] write tests: Python lazy-import fixture and TS require() fixture produce the signal; static graph unchanged
- [ ] run `make test` — must pass before next task

### Task 10: Volatility + BC scorer fidelity to Vlad's balance rule

- [ ] in `internal/classify/classify.go:325-357`, mark volatility `undeclared` (config omitted) distinctly from `unknown`; confirm the subdomain→volatility mapping is Vlad's default (Core→high, Supporting→medium, Generic→low) with per-module override
- [ ] in `internal/model/coupling/scorer_*.go`, replace `lower_volatility` for undeclared edges with "declare subdomain/volatility" guidance
- [ ] audit scorer ordinals + severity against the balance rule: Strength {Contract 1, Model 2, Functional 3, Intrusive 4}, Distance {local 1→external 3}, Volatility {low 1, medium 2, high 3}; low volatility neutralizes; cohesion (high strength + low distance) → low severity; worst = high+high+high → distributed-monolith risk. Update the score `definition` (and code comments) to attribute `Effort ∝ Strength × Distance × Volatility` as **archfit's implementation of Vlad's qualitative heuristic, not his literal equation**; bump `metric_version`
- [ ] write tests: undeclared edge yields declare-advice (not lower_volatility); severity table matches the balance rule (cohesion low; high+high+high worst; low-volatility neutralized)
- [ ] run `make test` — must pass before next task

### Task 11: scip-typescript bootstrap + coverage transparency

- [ ] in `internal/extract/scip/scip_strength.go`, detect missing `node_modules` for TS and emit an explicit coverage reason ("install deps for semantic strength") rather than silent absent
- [ ] generalize: when a headline metric is n/a due to a missing/opt-in tool, the report states the reason + enable step
- [ ] write tests for the coverage-reason messaging (TS no-node_modules; lizard absent; gitnexus present-but-disabled)
- [ ] run `make test` — must pass before next task

### Task 12: gitnexus auto-detect present index

- [ ] detect `.gitnexus/` / `.codegraph` index in target repo; use it when present (or warn it is present but disabled) instead of silent opt-in-off; refresh via `node .gitnexus/run.cjs analyze --index-only` (never regenerates CLAUDE.md/skills)
- [ ] write tests for detection + the "present but disabled" warning path
- [ ] run `make test` — must pass before next task

### Task 13: Enable complexity (lizard) for Python and TypeScript

- [ ] fix `internal/extract/complexity/` invocation/detection so lizard runs for Python and TS (it supports both); complexity no longer n/a when lizard is installed
- [ ] write tests / coverage assertions for Python + TS complexity extraction
- [ ] run `make test` — must pass before next task

### Task 14: Config-quality lint

- [ ] in `internal/config`, warn when modules omit `subdomain`/`volatility`/`owner` (explains degraded distance/volatility + advisory floods)
- [ ] write tests for the warning on under-specified configs
- [ ] run `make test` — must pass before next task

### Task 15: Per-dimension banded scorecard (`archfit score` / `--format scorecard`)

- [ ] add `internal/score/` that synthesizes metrics + gates into the architect's 7 dimensions (`boundary_integrity`, `coupling_balance`, `dependency_graph_health`, `cohesion_modularity`, `change_locality`, `architecture_fitness`, `analysis_confidence`) with value/band/confidence/evidence-refs aligned to `scorecard.yaml`
- [ ] derive `coupling_balance` strictly from Vlad's balance rule over BC edges (strength×distance×volatility maintenance-effort distribution + worst-case high/high/high count), not a generic metric average; `cohesion_modularity` must treat high-strength+low-distance as healthy cohesion
- [ ] add the `score` command and `--format scorecard`; deterministic output
- [ ] write tests enforcing band-matches-value, evidence-per-score, low-confidence caps; add a golden for the scorecard format
- [ ] run `make test` + `make lint` — must pass before next task

### Task 16: Gate noisy low-confidence metric displays

- [ ] in the markdown renderer, footnote (not headline) abstractness/martin when confidence is low; keep full data in JSON
- [ ] write tests asserting low-confidence metrics are demoted in human output but retained in JSON
- [ ] run `make test` — must pass before next task

### Task 17: Holistic off-gate `archfit review --llm`

- [ ] add `cmd/archfit/review.go`: feed the full `diagnostic.Diagnostic` bundle to the LLM with schema-constrained output; LLM may only narrate dimensions, classify volatility/subdomain, dedupe/prioritize **existing** findings, and propose the banded scorecard — never invent gate violations
- [ ] ground the system prompt in Vlad's Balanced Coupling model (integration strength ladder, socio-technical distance, volatility↔subdomain, balance rule, cohesion = good coupling, cascading changes, distributed monolith) so narratives reason and speak in his terms — extend the existing Khononov-grounded `explain` prompt
- [ ] post-verify every module/metric the LLM cites exists in the evidence; drop unsupported claims; never affects `check` (uphold LLM-off-gate invariant in `internal/arch_test.go`)
- [ ] write tests with a deterministic fake `llm.Provider` (canned response) asserting schema validation + entity post-check + that `check` is unaffected
- [ ] run `make test` + `make lint` — must pass before next task

### Task 18: Tooling, README, docs (Marvin review) + self-config

- [ ] README: lead with AI-agent before/after; positioning table vs dependency-cruiser/import-linter/ArchUnit; cut concept overload
- [ ] add self-dogfooding doc (signals vs violations) and per-language setup docs (scip needs node_modules; enable clones/gitnexus; install lizard)
- [ ] add starter `.archfit.yaml` templates (Go monolith, Python package, TS monorepo, DDD, microservices) to fix the "flat config → noise" trap; fix archfit's own config to surface its newest metrics (`architecture_fitness`/`functional_candidates`)
- [ ] sync install/CI snippets to current release
- [ ] run `make test` (docs-only changes still must not break tests/golden)

### Task 19: Phase-2 validation — architect skill vs improved archfit (all three repos)

- [ ] `make build`; re-run improved archfit (full + delta) on archfit(Go), ccgram(Python), codegraph(TS); capture markdown + JSON + `--format scorecard`
- [ ] re-run the independent architect-skill review on each repo (read-only, blind to archfit output)
- [ ] write `docs/plans/notes/gap-closure-phase2-comparison.md`: per-repo table of archfit banded dimensions vs expert bands; list blind zones now closed (deterministic) vs surfaced by `review --llm` vs still out-of-scope
- [ ] assert blind-zone reduction vs the baseline matrix in this plan; assert scorecard bands align with expert bands
- [ ] assert determinism (byte-identical double-run per repo); run `make test`+`make lint` — must pass before next task

### Phase 3 — Deeper (P2, eval-gated)

### Task 20: SCIP-symbol-graph cohesion metric (behind an eval)

- [ ] add `internal/metrics/modularity/cohesion.go`: edge-density LCOM proxy from the SCIP symbol graph (document the Python `enclosing_range` caveat)
- [ ] gate shipping on an eval comparing it to expert cohesion judgments on the three repos; if it fails the eval, keep it report-only/disabled (the prior `cohesion_spread` attempt failed — do not repeat)
- [ ] write tests + the eval harness; record the eval result in plan notes
- [ ] run `make test` — must pass before next task

### Task 21: Verify acceptance criteria (final gap-closure gate)

- [ ] verify every Phase-1/2 correctness assertion still holds on all three repos
- [ ] verify determinism end-to-end (double-run identical; golden stable; config_hash stable)
- [ ] run full `make all` (fmt → lint → test → build); core-ring + golden + LLM-off-gate invariants green
- [ ] confirm `review --llm` output post-verifies clean (no hallucinated entities) with a real key, or skip-with-note if key absent
- [ ] verify test coverage meets project standard

### Task 22: Update documentation + final comparison deliverable

- [ ] write `docs/v0.x-tool-vs-expert-gap-closure.md`: before/after blind-zone count per language, what is now deterministic, what remains LLM-only, what remains out-of-scope by design
- [ ] update guide/metrics + commands docs for new `score`/`review` commands and per-language behavior
- [ ] run `make test`

## Technical Details

- Baseline numbers to beat (from this investigation) — store in plan notes for Task 19:
  ccgram delta change_locality false-0 with 152 changed files; ccgram 421 BC advisories;
  codegraph martin flags `fs`/`crypto`/`commander`; codegraph scip absent (no node_modules);
  Go self-run: numeric BC score never emitted.
- Determinism levers already in archfit: `config_hash`, golden test (`internal/engine/golden_test.go`), `metric_version` per metric. Reuse, don't reinvent.
- LLM grounding pattern (from research): two-pass (deterministic rank → LLM narrate), schema-constrained output, post-check entities exist in evidence, LLM never discovers new gate violations. Reuse `internal/llm` cache for reproducibility of LLM runs.
- New tools considered: TS dead exports `knip` / `ts-prune` (optional, future); Python `vulture`/`ruff` (optional, future). Not required for this plan; dependency-cruiser already exposes `dependencyTypes` (Task 8).
- **Balanced Coupling sources of truth** (consult before touching any BC code): coupling.dev; Vlad Khononov, _Balancing Coupling in Software Design_ (2024) and _Learning Domain-Driven Design_. Integration strength = shared knowledge (Intrusive→Functional→Model→Contract); distance = socio-technical (code + deploy + ownership + lifecycle); volatility ↔ subdomain (Core/Supporting/Generic); the "one formula" = maintenance effort rising monotonically with all three (qualitative heuristic — no literal numeric equation is published, so archfit's `Strength×Distance×Volatility` score is explicitly our implementation of it). See the "Balanced Coupling fidelity" section above — it is the binding spec for every BC task.

## Post-Completion

_No checkboxes — external/manual._

**Provisioning**: `archfit review --llm` validation (Tasks 17, 21) needs
`ANTHROPIC_API_KEY` (currently missing per `archfit doctor`; `op` anthropic-team-key).
Deterministic work (Phases 1–2 minus the live LLM call) needs no key.

**Manual verification**: skim the Task 19 / Task 22 comparison docs; confirm the
architect-vs-archfit bands read sensibly and blind-zone reduction is real, not just
green assertions.

**Release**: tag-triggered only (`vX.Y.Z`) — never release manually; bump
`metric_version`/schema notes where outputs changed.
