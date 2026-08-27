# Archfit capability migration — final architecture re-review

Date: 2026-08-26
Scope: `docs/design/20260823-archfit-capability-map.md`, Tasks 1-5 of
`docs/plans/complete-capability-architecture-migration.md`.
Branch: `refactor/capability-contract-boundaries` (PR #33) vs `main`.
Binary: branch-built `.bin/archfit` (v1.7.1-14-gc3b2e19+).

## Verdict: GO

All five migration tasks are implemented, every fail-gate is green, and every
approved fitness requirement is enforced by a check that runs in CI. Five
residual risks are recorded — four below, plus the baseline non-idempotency in
"Baseline decision". All five are carried as accepted risks in
`docs/design/architecture-baseline.md`; none is blocking and none is converted
to a waiver.

This review was run inline by the executing agent rather than by an independent
`architecture-review` agent — subagent spawning was unavailable in the session.
That is a coverage gap in the review method, not in the evidence: every claim
below cites a check that a reviewer can re-run.

## Measured state

| Signal | Value |
| --- | --- |
| Verdict / exit | `pass` / 0 |
| Blocking findings | 0 |
| Advisory findings | 104 (`bc/imbalanced_coupling` 100, `bc/duplicated_knowledge` 4) |
| `agent_tasks` | 0 |
| `coupling_balance` | 41/100 `mixed`, confidence `high` |
| Classified edges | 1193 total · 362 scored cross-boundary · 143 same-module · 688 external |
| Package/module cycles | 0 |
| Tool coverage | `go/packages`, `scip`, `scip-symbols`, `loc`, `deploy-unit`, `jscpd`, `ast-grep`, `ast-grep/syntax` all `ok`; TS/Python/Rust `absent` (not present in this tree) |
| `git_finding_delta` vs `main` | `comparable`, 0 introduced, 0 unknown-origin |

Score effects, recorded separately as the plan requires:

- **Module-map effect: 0 points.** Current tree scored under the pre-Task-5
  config is also 41/`mixed`. Merging `evidence-stage-contracts` into
  `evidence-contracts` moved 6 edges from cross-module scored (368 → 362) into
  same-module `local_coupling` and removed 4 advisories; the headline number did
  not move (`config compare` against the prior config).
- **Label effect: 0 points.** `.archfit-labels.yaml` was empty before this task
  and is empty after.
- **Source-change effect: −5 points** (46 → 41, `analyze --base main` at fixed
  final config). Caveat: `main` still contains `internal/engine`,
  `internal/analysispipeline`, and `internal/view`, which the final module map
  does not own, so those packages classify external on the base side. The −5 is
  therefore a source-and-coverage delta, not a clean source delta. It is
  report-only and moves no gate.

## Re-review dimensions

**DDD ownership and ubiquitous language — PASS.** Seven bounded contexts, each
with exactly one physical owner. Policy owns architecture semantics;
`internal/config` owns the YAML lifecycle and projects into policy contracts
(`policy_no_config`, `TestPolicyDoesNotImportConfig`). Acquisition observes and
never judges (`acquisition_no_assessment_judgment`). Assessment owns judgment
and repair. The application owns sequencing. Naming matches the design: no
`engine`, `scan`, `pipeline`, or `view` vocabulary survives in source or in
`.archfit.yaml`.

**Modularity, cohesion, and dependency direction — PASS.** 15 production modules
over 76 Go packages; the module is the capability, not the directory. Layer
direction `model → support → core → application → adapter → cmd` is gated at
`fail` by `layer_inversion` and measures clean. `report-adapters` imports
`report-contract` and nothing else from `internal/`. Adapter→application edges
are port implementations (outer implements inner). 0 cycles.

**Balanced Coupling at each seam — PASS with a disclosed structural ceiling.**
Relationship→Assessment is 17 edges at `cross_module_same_owner`: strong
coupling at short distance, which is the balanced answer for two co-changing
core capabilities. All 362 scored edges sit on one distance rung because archfit
has one owner and one deploy unit, so the balance formula is driven almost
entirely by strength-vs-distance and the achievable score is capped by
construction. Raising it would require fabricating ownership or deploy-unit
boundaries. The 41/`mixed` is accepted as the honest read, not repaired.

**Change locality — PASS.** Verified by walking the six change recipes in
`docs/design/architecture-baseline.md`. Adding a metric touches
`internal/assessment` plus one config knob; adding a language touches
`internal/extract/<lang>` plus the CLI registry; adding an output format touches
one `internal/output` package plus, when a new field is needed,
`internal/application/report.go`; adding a CLI command touches `cmd/archfit`
only. No recipe requires editing a package outside its owning capability.

**Direct testability and failure-path coverage — PASS.** Task 3 added
owner-local behavior tests to `internal/relationship/analysis` and
`internal/assessment/evaluation`, which had 33.3% and 0% direct coverage at
review time (E3). Golden output, exit-contract, SCIP-reader, and byte-identical
suites all pass; 85 Go packages report `ok`.

**Archfit module/label/gate honesty — PASS.** `.archfit.yaml` now has zero dead
path globs, zero unowned Go packages, zero equal-specificity ownership ties,
zero dead rules outside the two declared guards, and zero `public:` entries
outside their own module — each proven by a test in
`internal/selfmodel_test.go` that was mutation-checked against the violation it
claims to catch. One dead entry was found and removed (`internal/assessment`, a
`public:` path with no Go source) and one dead entry corrected (`scripts/eval` →
`scripts/eval/coverage`). No waiver was added. `.archfit-labels.yaml` is empty
with a written abstain rationale.

**Cognitive workload and contract surface — PASS with residual risk 2.** The
published model surface is pinned by `TestModelSurfaceNoDrift`. The
`syntax_api_size_ceiling` advisory is a size ratchet, not an API budget; see
residual risk 2.

## Fitness requirements 1-12

All twelve pass. Each is mapped to its enforcing check in
`docs/design/20260823-archfit-capability-map.md#architecture-fitness-checks`
and to the invariant table in `docs/design/architecture-baseline.md`.
Requirement 3 was revised during implementation from "no production package
imports `internal/model/report`" to "no *domain* package imports it": the report
package was kept as the versioned external contract, so the enforceable rule is
that only the single application projector and the renderers may touch it.

## Residual risks (accepted, not waived)

1. `internal/extract/{ts,py}` import `relationship/coupling` for the strength
   constants they emit as hints. Shared vocabulary, not a decision. Upgrade
   trigger: if an extractor needs more of `coupling` than the strength enum,
   move the enum to `internal/model/evidence`.
2. `public_api_max` counts exported declarations in test files — 252 of
   `evidence-adapters`' 397 are `TestXxx` functions and moq fakes. The ceiling
   is documented as a growth ratchet (430) and the real surface is gated
   elsewhere. Changing the rule's semantics is a tool-behavior change and was
   deliberately not made during a behavior-preserving migration.
3. ~~`coupling_balance` evidence lists same-module pairs among "top module
   pairs".~~ Withdrawn on re-check: `addDriver`
   (`internal/relationship/analysis/summaries.go`) returns early on
   `DistanceSameModule`, so all three driver histograms — `by_balance_driver`,
   `by_critical_driver`, `by_module_pair` — count scored CROSS-BOUNDARY edges
   only and sum to the scored denominator printed beside them. Same-module
   coupling is reported in `local_coupling`, as
   `docs/design/architecture-baseline.md` states. No residual risk.
4. `config update` reports `action_required` with zero issues on archfit's own
   config, because discovery emits directory-grained module names while
   `.archfit.yaml` declares capability-grained ones. `Removed` is review-only
   and `--apply` never deletes a stanza, so this is noise, not a gap.

## Baseline decision: stay empty

`.archfit-baseline.json` remains `accepted: []`, `metrics: {}`.

`archfit baseline` was run against the final tree and the result was rejected on
inspection, because it does not converge:

| Round | Baseline entries written | Findings on the next `analyze` |
| --- | --- | --- |
| empty | 0 | 104, all `new` |
| 1 | 104 | 159 — 104 `baseline`, 55 `new` |
| 2 | 159 | 142 — 104 `baseline`, 38 `new` |

Accepting the 104 visible advisories surfaced 55 previously-grouped ones
(`bc/imbalanced_coupling` findings carrying `group_count > 1` expand once a
member is stored), and a second round did not reach a fixed point. Writing a
baseline whose immediate effect is a larger, oscillating finding set is an
unreviewed state change, not acceptance of a reviewed set — the plan's stop
condition for unexplained output changes.

Nothing needs suppressing: the gate is `pass` with 0 blocking findings and 0
`agent_tasks` against the empty baseline, so no finding is being accepted and no
architecture rule is being hidden. Verified directly — injecting
`internal/model/graph` and `internal/toolrun` imports into
`internal/assessment/finding` with the empty baseline in place makes
`archfit check` exit 1 with 2 blocking findings (`assessment_no_raw_graph`,
`layer_inversion`); removing them returns exit 0.

The 104 advisories are `bc/imbalanced_coupling` and `bc/duplicated_knowledge`
findings over genuinely model-coupled core capabilities at close distance. They
describe the accepted design, they do not gate, and they stay visible rather
than being baselined away.

Follow-up (not blocking, not part of this migration): the grouped-advisory
expansion above is tool behavior worth investigating on its own — a baseline
should be idempotent. Carried as accepted risk 5 in
`docs/design/architecture-baseline.md`.
