# Architecture guardrails v2.3.0 — implementation record

Approved scope: all five recommendations from the Codex/Claude review, regression tests, aligned documentation, RevMux review and the next minor release (v2.3.0 after v2.2.1).

## Work

- [x] Cache correctness: actual extractor input coverage, Go directory regression, TypeScript inherited configuration.
- [x] Honest evidence: Go load failures, required-rule evaluation status and machine-readable reasons.
- [x] Waivers: config validation, order-independent expiry, no implicit temporary-to-permanent debt promotion.
- [x] Agent repair: policy-correct goals, repair kind and effective validation replay.
- [x] Measurement profile: deterministic producer/tool identity, baseline compatibility and explicit unavailable comparisons.
- [x] Reject incomplete seam snapshots; align schemas, golden output, user/agent docs and migration guidance.
- [x] Run formatting, tests, lint, build, self-architecture gate and targeted CLI regressions.
- [x] Run RevMux; resolve actionable findings and review the final change.

## Acceptance

Warm and fresh graphs agree after relevant input changes. Missing required evidence never claims a passed rule. Invalid waivers fail at config load; valid waiver ordering does not change outcomes; capture does not silently make a temporary waiver permanent. Repair commands retain effective analyzers and tool strictness. Incompatible measurements produce named reasons rather than unsupported deltas. Existing accepted debt is not silently regenerated during profile migration.

GitHub App implementation, authenticated ownership, execution isolation, App E2E and a new context command remain outside this release. The engine owns its evidence and wire contracts.

## Validation record

Focused source and CLI regressions pass, including real Go cache invalidation,
real dependency-cruiser inherited-config changes, Go load failure, waiver
capture/ordering, validation replay, and measurement identity/comparison.
Schemas, public model surface and output fixtures were deliberately regenerated.
The first integration pass identified older fixtures needing complete evidence
or valid waiver metadata; these were corrected without weakening the assertions.
Full make test and make lint passed before review; make build and make archfit
passed. RevMux round 01 completed with 4/4 sources, no degradation, and five
verified findings (four major, one minor). Corrective work addresses bounded
cache costs, typed symmetric partial evidence, synthetic waiver support,
environment-bound baseline guidance, and accurate missing-baseline messaging.
Additional verification identified and removed stale Python fact/profile cache
replay. Round 02 completed with 2/2 sources and identified an opaque resolver
configuration parity issue, which was fixed with regressions. History identity
also now names its stable selection algorithm rather than observed window size.
Round 03 reviewed the final delta with 2/2 sources, no degradation, no retained
findings and no open questions. Final make fmt, make test, make lint, make build
and make archfit all passed. The self-architecture command can still report
disclosed unmeasured evidence; its successful exit does not claim every
architecture dimension has complete coverage.

The first Linux CI run exposed an environment-dependent empty pattern scan:
util-linux's `sg` was counted as a producer despite no configured patterns.
Empty pattern configuration now reports disabled without probing tools. A
no-probe regression and the regenerated output fixtures cover the correction.
All previously failing CLI test families also pass with an unrelated `sg`
first on PATH. The full local checks passed again. RevMux round 04 reviewed
this CI correction with 2/2 sources, no degradation and zero findings.

The shared synthetic rule contract lives in `internal/model/rule` and belongs
to the existing `architecture-policy` capability. It is stdlib-only and shared
by validation and finding producers; the `internal/policy` export cap stays 40.

## Publication

This record captures implementation readiness before integration and tagging.
Publication follows successful PR checks through the existing annotated-tag
release workflow. The published release and its assets are authoritative at
<https://github.com/alexei-led/archfit/releases/tag/v2.3.0>.
