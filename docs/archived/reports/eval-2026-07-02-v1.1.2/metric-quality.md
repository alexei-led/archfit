# archfit metric-quality audit — synthesis

Scope: every scored metric, gate rule, advisory, and agent-facing output surface in archfit,
audited independently (24 per-item audits) plus 3 cross-cutting audits (verdict/gating,
agent-consumption surfaces, config ergonomics). All findings below are code-verified or
empirically reproduced by the underlying audits (temporary scratch tests, live dogfood
runs against this repo) — not speculation.

## Verdict distribution

| Verdict | Count | Items |
|---|---|---|
| solid | 0 | — |
| acceptable | 13 | coupling_balance, metric_encapsulation, rule_forbidden_dependency, rule_new_cross_module_dependency, rule_public_api_change, rule_public_api_type_leak, advisory_bc_imbalanced_coupling, advisory_map_staleness, advisory_labels_stale, tool_coverage_and_gaps, classified_edges, structural_facts, risk_signal_facts |
| weak | 11 | run_verdict_summary, metric_unbalanced_edge, metric_cycle, metric_coverage, metric_blast_radius, rule_public_api_only, rule_forbidden_layer_direction, rule_internal_api_access, rule_cycle, rule_public_api_max, agent_tasks |

No item scored "solid." The best-scoring items ("acceptable") all still carry at least one
high-severity, reproduced defect — "acceptable" here means "usable with a known workaround,"
not "clean."

## Per-metric table

| id | verdict | key issue |
|---|---|---|
| run_verdict_summary | weak | coupling_balance is not a `diagnostic.Metrics` entry and BC advisories are explicitly excluded from `computeVerdict` — the flagship metric can never gate, contradicting its own docs |
| coupling_balance | acceptable | `score_delta` (the `--base` JSON payload) fabricates a numeric `0` for an n/a base/head side with no band field to disambiguate from a genuine critical-band 0 |
| metric_encapsulation | acceptable | `metrics.<name>.gate/min_delta/max_new/max_new_high` are schema-validated but have zero consumers — configuring them is a silent no-op |
| metric_unbalanced_edge | weak | Confident `0/strong` (not n/a) when zero edges are ever classified `intrusive` — reproduced live on archfit's own repo alongside `encapsulation: n/a` on the identical edge set |
| metric_cycle | weak | `ComputeDelta` sign inversion: a real new cycle (delta>0) is silently ignored by `computeVerdict`; a fixed cycle (delta<0) falsely triggers WARN |
| metric_coverage | weak | Ratio is structurally incapable of reporting anything but n/a or ≥100% — every extractor except cargo-modules sets `FilesApplicable == FilesSeen`; cargo-modules itself inverts num/denom, producing a fabricated "125% coverage / strong" on a real partial failure |
| metric_blast_radius | weak | Disabling or merely misconfiguring `blast_radius` silently forces the flagship `coupling_balance` dimension to n/a (`degenerateGraph` proxy), discarding real, well-measured coupling data |
| rule_forbidden_dependency | acceptable | `resolveEvidence` unconditionally overwrites the rule's own `SeverityHigh` with a BC-derived severity that can only ever be High or Medium — misordering triage without affecting gating |
| rule_public_api_only | weak | Fires on same-module Go `internal/` imports with a false "Cross-module access" claim — reproduced 310/310 false findings on archfit's own repo under the documented example config, with default `gate:fail` |
| rule_forbidden_layer_direction | weak | `archfit config init`'s own generated "layer-aware" rules use `type: forbidden_dependency` (wrong type) — the generated gate produces 0 findings forever regardless of real violations |
| rule_internal_api_access | weak | Go's `uses_internal` edge tagging is a hardcoded substring check on ScanRoot-relative paths that structurally can never fire for the most common Go layout (root-level `internal/` tree, including archfit's own) |
| rule_new_cross_module_dependency | acceptable | Fails OPEN: any module-map glob mismatch (e.g. slash-style glob vs. Python's dotted node paths) silently produces 0 findings and a green PASS, indistinguishable from a well-bounded architecture |
| rule_cycle | weak | Fabricates a non-existent representative `Edge`/`Why` arrow chain for multi-member cycles (reproduced: reported edge `a→m` when no such edge exists) and never populates `Locations` |
| rule_public_api_max | weak | Counts exported declarations with zero test-file filtering — 88.6% (955/1078) of matched Go declarations in the self-scan are `_test.go` `TestXxx` functions, forcing the ceiling to be calibrated on test noise, not real API surface |
| rule_public_api_change | acceptable | Fingerprint is `(module, name)` only, no file/kind — two unrelated declarations sharing a name in one module collapse to one fingerprint, silently classifying a genuinely new declaration as already-baselined |
| rule_public_api_type_leak | acceptable | `versionSegmentRe` strips real `vN`-named Go packages (e.g. `k8s.io/apimachinery/.../v1`) as if they were module-version suffixes, producing a reproduced false negative on an idiomatic, unaliased import |
| advisory_bc_imbalanced_coupling | acceptable | The BC anti-corruption-layer signal (`ContractRecommended`) is computed and unit-tested but never read by `advisories.go` — a real, already-computed signal never reaches any output |
| advisory_map_staleness | acceptable | `map/uncovered_path` structurally never fires for Python — its Kind filter (`Package`/`File`) excludes `NodeKindModule`, the only kind the Python extractor emits |
| advisory_labels_stale | acceptable | `applyPinnedLabels` runs before module auto-registration (`AugmentModulesFromGraph`/Go-workspace/Rust `::` crates) — a pinned label for an auto-registered module pair can never be checked for staleness and silently always passes |
| agent_tasks | weak | `Files[]` fabricates non-file strings (dotted Python module IDs, bare module-config keys, `crate::mod` tokens) as repair-target file paths for a config-reachable class of findings (Python structural rules; public_api_max/change/type_leak in any language) |
| tool_coverage_and_gaps | acceptable | `coverage_gaps[]` only fires on `Status==absent`; a tool that ran and silently produced nothing (e.g. one broken `go.work` member aborting the whole extraction) gets no gap entry even though it zeroes coupling_balance/cycle/blast_radius |
| classified_edges | acceptable | `LLMApproved` (a count of approved *labels*, module-pair granularity) is compared as a percentage of `Scored` (*edge*-count granularity) to decide confidence-lowering — the mismatch means the documented "LLM provenance lowers confidence" invariant essentially never fires on realistically sized repos |
| structural_facts | acceptable | `public_api_max` — the one rule in this family with a default **blocking** gate — inherits the same unfiltered test-declaration counting bug as `rule_public_api_max` (61% of self-scan syntax_facts are from `_test.go`) |
| risk_signal_facts | acceptable | None of the three risk-signal detectors (dynamic_imports, runtime_async, deprecated_deps) exclude `testdata/`, unlike the project's own `scope.DefaultExclusions` — reproduced: archfit's own ast-grep test fixture is reported as a real production dynamic-import site |

---

## All High/Medium issues, with evidence

### run_verdict_summary
- **[HIGH]** docs claim coupling_balance is verdict-affecting; it isn't — it's not a `diagnostic.Metrics` entry (only 5 metrics registered: encapsulation, unbalanced_edge, cycle, coverage, blast_radius) and BC advisories are explicitly excluded from `activeRuleAdvisories` ("must not flip the verdict"). *Evidence:* `internal/metrics/metrics.go:54-60`, `internal/engine/engine.go:259-261`, `TestRun_Advisory_VerdictUnchanged`, `docs/guide/metrics.md:8-19,89-101`.
- **[HIGH]** `unbalanced_edge`'s baseline-delta sign is inverted relative to `computeVerdict`'s rule: a falling (improving) count triggers WARN; a rising (regressing) count is ignored. *Evidence:* `internal/metrics/boundary/unbalanced_edge.go` (`value := float64(newHigh)`), `internal/metrics/internal/result/result.go:130-140` (`current-baseline`), `internal/engine/engine.go` (`*m.Delta < 0`), contradicts `docs/guide/metrics.md:118`.
- **[MEDIUM]** `Diagnostic.Delta` (the delta report: new/existing/resolved findings) is unreachable via the documented `analyze --base <ref>` workflow — `Full: c.Full || c.Base != ""` forces `ModeFull`, never `ModeDelta`, whenever `--base` is set. *Evidence:* `cmd/archfit/analyze.go:182`, `internal/scope/scope.go` Resolve(); empirically reproduced (`--full=false --base=HEAD~5` → no `delta` key).
- **[MEDIUM]** `decision.Report.Band` (NEEDS_ATTENTION/HEALTHY) and `Diagnostic.Verdict` (the exit-code driver) can silently diverge: raising `coupling.min_severity` can zero out advisory findings (`Warnings==0`, `verdict=pass`, exit 0) while the unfiltered `ClassifiedEdgeSummary` still lands the score in `poor`/`critical`. *Evidence:* `internal/decision/decision.go` decideBand(), `internal/engine/advisories.go` collectAdvisories() filter vs. `internal/engine/assemble.go` buildClassifiedEdgeSummary() (no filter).

### coupling_balance
- **[HIGH]** `score_delta`'s JSON payload fabricates a numeric `0` for an n/a base/head side with no `Base`/`Head` band field anywhere to disambiguate it from a genuine critical-band 0. *Evidence:* `internal/output/jsonout/jsonout.go:39-51,79-105`; `internal/decision/decision.go:282-315` has the identical gap; reproduced end-to-end (n/a base, 78/serviceable head → `score_delta.dimensions[0]={base:0,head:78,delta:0}`).
- **[HIGH]** The flagship dimension's findings never produce an `agent_task`, regardless of severity — `agenttask.Build` only processes `Kind=="gate"`, and every BC finding is `Kind=="advisory"`. *Evidence:* `internal/agenttask/agenttask.go:48`; `internal/engine/advisories.go:67`; reproduced: 23 critical bc/imbalanced_coupling findings, `agent_tasks` length 0.
- **[MEDIUM]** No config path can ever promote a coupling problem to a blocking gate finding (unlike other rule types' `gate: off|warn|fail`). *Evidence:* `cmd/archfit/analyze.go:237`, `internal/engine/engine.go:259-261`.
- **[MEDIUM]** The only value-level safety cap keys on critical/DM prevalence; a codebase whose edges are a meaningful minority `high`-band (not critical) can still average to Strong/high-confidence uncapped. *Evidence:* `internal/score/score_boundary_coupling.go:98-105` (only reads `BySeverity[critical]` and DistributedMonolith).
- **[MEDIUM]** Three n/a-related branches with documented invariants have zero regression tests (jsonout.buildDelta suppression, decision.buildDelta suppression, decideBand's `!Unmeasured()` guard). *Evidence:* grep for BandNA/Unmeasured across `*_test.go` hits only `score_test.go`.

### metric_encapsulation
- **[MEDIUM]** `metrics.<name>.gate/min_delta/max_new/max_new_high` are schema-validated (`config.go:191`) but read nowhere outside `internal/config` — `gate: off` doesn't silence the delta-warn, `gate: fail` can never fail a build, `min_delta` is ignored. *Evidence:* `internal/config/types.go:11-17`, `internal/config/views.go:159-163` (ForMetric called only by config_test.go), `docs/guide/configuration-reference.md:747-767`.
- **[MEDIUM]** All four distinct n/a root causes (no signal / all-unknown-strength / degenerate graph / compiler-boundary-zero-intrusive) collapse to one identical, generic Definition string — the dominant real-world outcome (9/11 eval repos) is also the least actionable. *Evidence:* `internal/metrics/boundary/encapsulation.go:191-207`.

### metric_unbalanced_edge
- **[HIGH]** False "strong"/high-confidence 0 when zero edges are EVER classified strength=intrusive, instead of n/a (sibling `encapsulation.go` has this exact guard; `unbalanced_edge.go` omits it). *Evidence:* reproduced live on this repo's own dogfood run — `encapsulation:{band:"n/a"}` and `unbalanced_edge:{value:0,band:"strong",confidence:"high"}` in the same run over the same edges, because `.archfit.yaml` declares zero `internal:` globs.
- **[HIGH]** Configured gate policy (`gate/min_delta/max_new_high/max_new`) is dead code, and the only live path (generic `computeVerdict` delta rule) is directionally backwards for this lower-is-better count metric. *Evidence:* `internal/config/types.go:11-17`, `internal/engine/engine.go:456-459`.
- **[MEDIUM]** Undeclared-volatility candidates are silently excluded from the high-risk count once any other candidate has resolved volatility — contradicting how `coupling_balance` treats the identical config gap (worst-case V=10). *Evidence:* `internal/model/coupling/coupling.go:65-67` vs `unbalanced_edge.go:100,107-113`.
- **[MEDIUM]** Metric output carries no per-edge evidence (`Value`/`Display`/`Band` only) — a nonzero count gives an agent no way to locate offending edges. *Evidence:* `internal/model/diagnostic/diagnostic.go:58-68`.

### metric_cycle
- **[HIGH]** Delta-sign inversion (same root cause/pattern as `unbalanced_edge`): baseline=1→now 0 (a fix) triggers false WARN; baseline=0→now 1 real cycle (critical) is silently PASS. Live on every gated run since `.archfit-baseline.json` loads unconditionally. *Evidence:* `internal/engine/engine.go:452-465`, `internal/metrics/internal/result/result.go:130-140`, `internal/metrics/boundary/cycle.go:63`.
- **[HIGH]** `metrics.cycle.gate/min_delta/max_new/max_new_high` are schema-validated with zero consumers; combined with the sign bug, there is no working way to make `cycle` alone gate CI on regressions — only the fully separate, opt-in `rules: type: cycle` works, and archfit's own self-config declares neither. *Evidence:* `internal/config/config.go:184-191`, `internal/config/config.go:148-150` (own doc-comment forbids this exact anti-pattern).
- **[MEDIUM]** `CycleMetric` never abstains and never consults extraction coverage; Confidence is hardcoded "high" in every branch — a silently incomplete dependency graph is indistinguishable from a genuinely acyclic repo. *Evidence:* `internal/metrics/boundary/cycle.go:34`.
- **[MEDIUM]** AI-feedback value depends entirely on whether a separate `type:cycle` rule is also configured; the metric alone gives count+band only, no files, no AgentTask. *Evidence:* `internal/metrics/boundary/cycle.go:59-62`, `internal/agenttask/agenttask.go:14-16,28`.

### metric_coverage
- **[HIGH]** The ratio can structurally only ever read exactly 1.0 (100%) or n/a — every contributing extractor except cargo-modules sets `FilesApplicable == FilesSeen`. *Evidence:* `internal/metrics/boundary/coverage.go:27-57`; paired assignments in golang.go:286-287, ts.go:492-493, py.go:354-355, rust.go:337-338, clones.go, loc.go, scip files.
- **[HIGH]** cargo-modules' coverage record inverts numerator/denominator on partial failure, inflating the blended ratio above 1.0. Reproduced: `{FilesSeen:5,FilesApplicable:5,ok}` + `{FilesSeen:5,FilesApplicable:3,partial}` → `value=1.25, Display="125% coverage", Band="strong"`. *Evidence:* `internal/extract/rust/modules.go:89-93`.
- **[HIGH]** A total single-tool crash (`Status=partial`, zero counts) is dropped from the ratio by the same `FilesApplicable==0` guard used for auxiliary tools, and never produces a CoverageGap (that block only fires on `Status==absent`). *Evidence:* `internal/metrics/boundary/coverage.go:40-42`, `cmd/archfit/pipeline_coverage.go:132-136`.
- **[MEDIUM]** Documented `metrics.coverage.gate/min_delta` are parsed/validated but never consumed; the only verdict effect is the generic gate-agnostic "any negative delta → warn," inert without a baseline. *Evidence:* `docs/guide/configuration-reference.md:763-766`, `internal/engine/engine.go:452-467`.

### metric_blast_radius
- **[HIGH]** `score.degenerateGraph(mi) = !mi.measured("blast_radius")` forces the flagship `coupling_balance` to n/a whenever blast_radius is absent/disabled — reproduced with real, well-measured `ClassifiedEdges{Scored:100,MeanBalance:9.0}` returning `coupling_balance.Band=="n/a"` with a factually false evidence string. A config author who merely touches `metrics: blast_radius: {gate:warn}` (without `enabled:true`) triggers this with no error/warning. *Evidence:* `internal/score/score.go` degenerateGraph, `internal/score/score_boundary_coupling.go`, `internal/config/types.go` MetricEntry.Enabled zero-value.
- **[MEDIUM]** Cross-language module granularity (Go package dir vs. Python/TS file vs. Rust crate/module) combined with a fixed 30% hub threshold produces wildly non-comparable signal density: storybook (TS) 0% hubs vs. prefect (Python) 64%, ccgram (Python) 44% — not fixable by tuning one global constant. *Evidence:* eval corpus `reports/eval-2026-07-01-fix-multilang/*/full.json`.
- **[MEDIUM]** `modgraph.BlastRadius` panics (nil map write) if any edge's `From` node was never added to the graph — an unenforced, unguarded invariant; reproduced with a synthetic dangling edge. *Evidence:* `internal/metrics/internal/modgraph/modgraph.go`.

### rule_forbidden_dependency
- **[HIGH]** `engine.resolveEvidence` unconditionally overwrites every rule finding's `Severity` — including this rule's own `SeverityHigh` — with a BC-derived value that can only be High or Medium, because `classify.Run` indexes every graph edge unconditionally. *Evidence:* `internal/rules/rules_dependency.go:40`, `internal/classify/classify.go:64-77`, `internal/engine/engine.go:404-436`, `internal/engine/advisories.go:256-266`.
- **[MEDIUM]** From/To globs must match the target language's node-path convention (slash vs. dotted vs. crate-name); `doublestar.Match`'s error is discarded and `config.validate()` never checks glob syntax — a wrong-convention rule silently matches 0 edges forever, indistinguishable from "no violations." *Evidence:* `internal/rules/rules_dependency.go:33-34`, `docs/guide/languages.md:232-239,321-322`.

### rule_public_api_only
- **[HIGH]** The rule never checks module identity (no ModuleMap, no fromModule==toModule check, unlike its sibling `newCrossModuleDependency`) yet unconditionally emits "Cross-module access to internal path X." Reproduced: the doc's own example config against archfit's own repo → 310/310 findings, all with `from.module==to.module`, `verdict=fail`. *Evidence:* `internal/rules/rules_dependency.go:60-93`, `docs/guide/configuration-reference.md:92-94`.
- **[MEDIUM]** `Finding.Severity` is hardcoded High but silently downgraded to Medium by the same `resolveEvidence` join described above whenever the edge's independently-derived coupling strength isn't "intrusive" — for Go specifically this decoupling means the severity downgrade fires almost by default. *Evidence:* `internal/engine/engine.go:407-431`, `internal/classify/classify.go:538-549`.

### rule_forbidden_layer_direction
- **[HIGH]** `archfit config init` auto-generates "layer-aware" rules with `type: forbidden_dependency` but only `from_layer`/`to_layer` set (never `from`/`to`) — `forbiddenDependency.Check()` only reads `def.From/To`, so `doublestar.Match("", path)` is always false and these gate stanzas produce 0 findings forever. Reproduced: `config init` on a full copy of this repo → 0/91 findings from the generated rules; fixing only the `type:` field immediately caught a real violation. *Evidence:* `internal/initcfg/initcfg.go:357-360`, `internal/rules/rules_dependency.go:28-49`; locked in as "expected" by `initcfg_test.go:473-476`.
- **[HIGH]** `RuleDef.FromLayer/ToLayer` — documented as per-rule layer-pair scoping — are never read by `forbiddenLayerDirection.Check()`; every rule instance checks the entire global `layers:` ordering. Reproduced: 4 distinct per-pair rule IDs all fired on the same single edge (4x duplicate findings for what should be 1). *Evidence:* `internal/rules/rules_dependency.go:97-152`, `docs/spec/arch-fitness-spec-v0.4.md:424-428`.
- **[MEDIUM]** No n/a/coverage signal distinguishes "genuinely checked, 0 violations" from a vacuous no-op (empty `layers:`, unlabeled modules, typo'd layer name) — all three silently short-circuit to 0 findings forever with no config error. *Evidence:* `internal/config/config.go:151-213`, `internal/rules/rules_dependency.go:114-130`.
- **[MEDIUM]** The rule's `SeverityHigh` is silently overwritten to Medium for most real violations by the same `resolveEvidence` join. *Evidence:* `internal/rules/rules_dependency.go:141` vs `internal/engine/engine.go:426-435`.

### rule_internal_api_access
- **[HIGH]** Go's `uses_internal` detection is a hardcoded import-path-substring heuristic that ignores `config.ExtractConfig.Internal` entirely and cannot fire for a root-level `internal/` tree (relDir="."). Reproduced end-to-end: `forbidden_dependency` on the same from/to finds a real hit; `internal_api_access` on the identical edge finds 0 — this repo's own self-scan shows 0/284 scored edges as "intrusive." *Evidence:* `internal/extract/golang/golang.go:211-228,350`.
- **[HIGH]** Zero direct unit test coverage for `internalAPIAccess.Check()` — its byte-identical twin `publicAPIOnly` has a thorough table-driven test; this type appears in exactly one test, only for the shared goal-string template. *Evidence:* `internal/rules/rules_test.go:182-263` vs. absence for `internal_api_access`.

### rule_new_cross_module_dependency
- **[MEDIUM]** Fails OPEN: any module-map miss (slash globs against dotted Python/Rust node paths, empty `modules:` block, catch-all glob) produces 0 findings and a green PASS — reproduced (slash globs against dotted `module:pkga.foo` edge → 0 findings; dotted globs → 1 finding). No rule-level coverage counter exists to flag this. *Evidence:* `internal/rules/rules_dependency.go:229-235`.
- **[LOW]** The rule's own matching logic has no direct unit test, unlike every sibling rule type in the same file. *Evidence:* `internal/rules/rules_test.go` (missing `TestNewCrossModuleDependency`).

### rule_cycle
- **[HIGH]** Fabricates its representative Edge/Why arrow chain instead of using a real graph edge — reproduced: a 3-cycle (a→z→m→a) reports `Why: "a → m → z"` and `Edge{From:a,To:m}`, neither of which is a real edge. *Evidence:* `internal/rules/rules_dependency.go:272-274,290`.
- **[HIGH]** `Finding.Locations` is never populated for cycle findings (unlike every other rule in the same file), removing file:line evidence from JSON/SARIF/agent_task output for the finding type that most needs it. *Evidence:* `internal/rules/rules_dependency.go:275-292`; SARIF/agenttask fall back to the fabricated Edge endpoints.
- **[HIGH]** No test exercises `cycleRule.Check()` at all — ID stability, severity, representative-edge selection, and the empty-Locations gap are all untested. *Evidence:* repo-wide grep for `cycleRule{`, `cycleFingerprintID` in test files returns zero hits.
- **[MEDIUM]** Waiver from/to glob matching keys on the fabricated representative Edge, not a stable "the" edge — a waiver written against a real, visible cycle edge can silently fail to match. *Evidence:* `internal/status/status.go:125-148`.
- **[MEDIUM]** `rule_cycle` (and the shared `graph.Cycles()` primitive) is structurally inert for pure-Go facts (Go emits only bipartite file→package edges), and the existing "Go cycle stays critical" regression test doesn't exercise the real Go extractor's edge shape. *Evidence:* `internal/extract/golang/golang.go:359-368`, `internal/metrics/boundary/cycle_test.go:77-101`.

### rule_public_api_max
- **[HIGH]** Go/Python export detection is name-convention-based with zero test/generated-file exclusion. Reproduced on this repo: 955/1078 (88.6%) go-func matches are in `_test.go` files, all named `TestXxx`. This forced `.archfit.yaml`'s own `internal` module ceiling to 1600 — a value almost entirely defined by test-function count. *Evidence:* `internal/extract/astgrep/syntax.go` Syntax() (no FileClass/Production filter, unlike `internal/extract/loc/loc.go:112-117`); `.archfit.yaml:562-567`.
- **[HIGH]** `publicAPIMax`/siblings put the MODULE NAME into `Endpoint.Path` (not a real path) and leave `Locations` nil — inverted from the documented Edge contract, breaking `agenttask.declarationsFor` (always misses) and SARIF `artifactLocation.uri` (emits a module name as a file). *Evidence:* `internal/rules/rules_api.go:88-91`.
- **[MEDIUM]** `public_api_max` applies a single global `max` to every module in one rule instance — no per-module scoping despite RuleDef having From/To fields (unused here). Documented as a known limitation in archfit's own dogfood config. *Evidence:* `.archfit.yaml:562-567`, `docs/guide/configuration-reference.md:667-669`.

### rule_public_api_change
- **[HIGH]** Fingerprint key is `(module, name)` only — no file, no kind. Two unrelated exported declarations sharing a name in one module hash to the same fingerprint; the second is silently classified `StatusBaseline` (not new) forever once the first is accepted. Directly defeats the rule's purpose. *Evidence:* `internal/rules/rules_api.go:146-152`, `internal/status/status.go:104`; contrast with `newCrossModuleDependency`'s full-path fingerprint.
- **[HIGH]** `AgentTask.Files`/`Declarations` are empty/wrong once `gate:fail` is set on this rule — `Edge.From/To.Path` holds the module key, not a file, and `Locations` is never set. Reproduced: `Files:["domain"]` instead of `"domain/a.go"`. *Evidence:* `internal/rules/rules_api.go:159-162`, `internal/agenttask/agenttask.go:128-147`.
- **[MEDIUM]** `finding.Locations` never populated for this rule at all, unlike every graph-edge rule in the same package. *Evidence:* `internal/rules/rules_api.go:153-172`.

### rule_public_api_type_leak
- **[MEDIUM]** `versionSegmentRe` (matches trailing `^v\d+$` segments to strip Go-module version suffixes) cannot distinguish that from a package whose real name is `v1`/`v2` (a common Go convention, e.g. `k8s.io/apimachinery/.../v1`). Reproduced: an unaliased import of such a package produces 0 findings for a real type leak (`extPkgs=={"meta"}` instead of including `"v1"`). *Evidence:* `internal/rules/rules_api.go:182,202-251`.
- **[LOW]** `MatchedBy["module"]` is not a stable module-key field — falls back to the raw file path when unowned, unlike sibling rules which skip unowned files; undocumented asymmetry. *Evidence:* `internal/rules/rules_api.go:295-298`.

### advisory_bc_imbalanced_coupling
- **[MEDIUM]** `Classification.ContractRecommended` (BC anti-corruption-layer signal) is computed and unit-tested in classify.go but never read by `advisories.go` — absent from MatchedBy, Why, and every output surface — despite the code's own comment saying it exists "so the engine can emit a dedicated advisory finding." For unknown-strength edges into generic subdomains the edge is also abstained entirely (StrengthUnknown), so the recommendation is permanently invisible for that whole class. *Evidence:* `internal/classify/classify.go:466-484`, `internal/model/coupling/coupling.go:108-118`, `internal/engine/advisories.go:28-101`.

### advisory_map_staleness
- **[HIGH]** `map/uncovered_path` never fires for Python: it only inspects `NodeKindPackage`/`NodeKindFile`, but the Python extractor is the sole extractor that tags every node `NodeKindModule`. Reproduced: a Python `NodeKindModule` node with no matching module glob → 0 uncovered_path findings. *Evidence:* `internal/staleness/staleness.go:56-59`, `internal/extract/py/py.go:299-304`.
- **[MEDIUM]** `staleness.Check` reads the raw, un-augmented module map — the three `Augment*` passes that register synthetic modules (Go workspace, Rust `::`, Cargo crate nodes) never reach it, because `StalenessConfig.Modules` is captured before augmentation runs. On any auto-registered multi-module repo, every package under the synthetic-but-undeclared module gets an individual (un-rolled-up) uncovered_path advisory contradicting what coupling_balance just treated as covered. *Evidence:* `internal/engine/engine.go:147-162`, `cmd/archfit/pipeline_run.go:279`.
- **[MEDIUM]** `module_review.gate: fail` behaves identically to `warn` — both only flip a boolean `Enabled`; staleness findings can never affect the exit code either way, contradicting the field's own doc comment. *Evidence:* `internal/config/views.go:237-252`, `internal/config/types.go:22-28`.

### advisory_labels_stale
- **[MEDIUM]** `applyPinnedLabels` runs BEFORE `AugmentModulesFromGraph`/`AugmentGoWorkspaceModules`/`AugmentCargoCrateNodes` and before the ModuleMap rebuild. A pinned label naming an auto-registered-module pair can never have its evidence hash computed; `isEffective`'s "no current evidence → moot, pass" branch silently trusts the label forever regardless of drift. Reproduced: a Rust auto-registered pair with a garbage `EvidenceHash` never triggers a stale advisory, and the pinned strength silently overrides the real extractor hint. *Evidence:* `internal/engine/engine.go:141-165` (ordering), `internal/labels/labels.go:111-117`.
- **[LOW]** A label naming a renamed/removed module becomes a silent orphan — neither flagged stale nor applied, with no advisory warning the pin is now dead weight. *Evidence:* `internal/labels/labels.go:106-117`.

### agent_tasks
- **[HIGH]** `Files[]` (and the Declarations enrichment) fabricates a non-file string as the repair location for a config-reachable class of findings: Python structural findings use dotted grimp module IDs; `public_api_max/change/type_leak` (any language) use the bare module-config key; opt-in Rust cargo-modules edges use `crate::mod` tokens — none of these are openable paths, and the real file is often available elsewhere on the Finding (`MatchedBy["file"]`) but unused. *Evidence:* `internal/agenttask/agenttask.go:126-147`; live production JSON shows `"path":"prefect.blocks.core"` reaching a Finding.
- **[HIGH]** Cycle findings truncate `Files[]` to only 2 of N cycle members even though the rule already records the full list in `MatchedBy["cycle_members"]`. *Evidence:* `internal/rules/rules_dependency.go:272-292`, `internal/agenttask/agenttask.go:126-147`.
- **[MEDIUM]** The "public surface of module X" hint in Constraints is convention-dependent (only appears when a module's config key textually equals its own `**`-suffixed glob prefix) — silently drops with no error for common naming styles. *Evidence:* `internal/engine/engine.go:420-436`, `internal/config/modules.go:126-142`.
- **[MEDIUM]** `goalFor`'s rule-type switch (6 of 9 rule types) has no compile-time link to `internal/rules`' own type switch — `public_api_max/change/type_leak` fall through to a generic/boilerplate goal, exactly the 3 types whose Files[] is also broken. *Evidence:* `internal/agenttask/agenttask.go:74-97` vs `internal/rules/rules.go:60-92`.

### tool_coverage_and_gaps
- **[HIGH]** `coverage_gaps[]` only fires on `Status==absent`. A tool that ran and produced nothing (one broken `go.work` member aborts the entire go/packages extraction via errgroup cancellation, discarding facts from every successfully-loaded member; TS/Python have no `filesSeen==0→absent` guard) produces no gap entry, even though it zeroes coupling_balance/cycle/blast_radius. *Evidence:* `cmd/archfit/pipeline_coverage.go:134`, `internal/extract/golang/golang.go:153-156`, `internal/extract/golang/golang_test.go:82-106` (`TestExtract_MemberLoadFailure`, whose own comment calls this "degrades to a partial coverage gap" — a promise the wiring doesn't keep).
- **[MEDIUM]** `toolAffectedMetrics[cargo-modules]` lists a metric ("cohesion") that was deleted from the codebase in the v1.0 restructure — a stale reference that will never resolve for anyone following the install instruction. *Evidence:* `cmd/archfit/pipeline_coverage.go:62`; metric removed in commit af3858a.

### classified_edges
- **[HIGH]** `LLMApproved` (a count of approved *labels*, module-pair granularity) is compared as `LLMApproved*100/Scored >= 20` against `Scored` (edge-count granularity) to decide confidence-lowering. Since one approved label can override the strength of dozens/hundreds of edges between that pair, the ratio is essentially always <20% on any realistically sized repo — the documented "LLM-provenance labels lower confidence" invariant silently never fires in practice. *Evidence:* `internal/score/score_boundary_coupling.go:111-115`, `internal/labels/labels.go:147-158`, `internal/classify/classify.go:389-398`.
- **[MEDIUM]** `BySeverity` uses `coupling.SeverityNone` (empty string `""`) as a literal JSON map key for well-balanced edges — renders as `"by_severity": {"": N, ...}` in JSON output, an easy-to-miss key for any human or agent looking up `by_severity.none`. *Evidence:* `internal/engine/assemble.go:193`, `internal/model/coupling/coupling.go:149`.

### structural_facts
- **[HIGH]** `public_api_max` — the one rule in this family with a default **blocking** gate — counts exported SyntaxFacts with zero test-file filtering. Reproduced on this repo: 987/1609 (61%) of syntax_facts originate from `_test.go` files, including exported mock-struct methods, all counted toward the module's exported-declaration ceiling. *Evidence:* `internal/extract/astgrep/syntax.go` Syntax() (no Production filter, unlike `internal/extract/loc/loc.go:112-117`).
- **[MEDIUM]** `file_facts` inbound/outbound fan-in only excludes scip-go's synthetic `.test` pseudo-module, not Go's own `_test` external-test-package convention. Reproduced: 38/96 file_facts modules in the self-scan are `_test` pseudo-modules with nonzero outbound_destinations phantom-inflating both their own rank and every production module's inbound fan-in they test-import. *Evidence:* `internal/facts/facts.go:41,70-90`.
- **[MEDIUM]** No config-lint/CoverageGap flags "a public_api_* rule is declared while `analyzers.syntax.enabled` is false" — a default-gate(fail) `public_api_max` rule silently always passes if a user forgets the opt-in; separately, "ast-grep" is absent from `toolAffectedMetrics`, so if `sg` disappears from a CI image later, no CoverageGap surfaces it either. *Evidence:* `internal/rules/rules_api.go:46,129,266`, `cmd/archfit/pipeline_coverage.go:59-68`.

### risk_signal_facts
- **[HIGH]** None of the three detectors (dynimports, runtime, manifest) exclude `testdata/`, unlike `scope.DefaultExclusions`. Reproduced: `dynimports.Detect` on this repo returns `internal/extract/astgrep/testdata/integration/fixture.py:40,46` as real production lazy-import sites — the tool's own ast-grep test fixture. *Evidence:* `internal/extract/dynimports/dynimports.go:186-189`, `internal/extract/runtime/runtime.go:390-393`, `internal/extract/manifest/manifest.go:200-204` vs. `internal/scope/scope.go:52-77`.
- **[MEDIUM]** `runtime_async` and `deprecated_deps` are invisible to the off-gate LLM review prompt, and `runtime_async` is also absent from the markdown report entirely — despite the package's own doc framing this evidence as feeding exactly that LLM review. *Evidence:* `cmd/archfit/llmreview.go` buildReviewPrompt/buildValidModules (DynamicImports only); `internal/output/markdown/markdown.go:90-92` (no runtime_async writer).
- **[MEDIUM]** `runtime.go`'s doc comments claim ast-grep-backed detection with a `toolrun.Runner` fallback, but the runner is stored and never invoked — detection is 100% naive line-based regex/prefix text scan, capable of misclassifying an unrelated quoted-string or docstring line as a real import. *Evidence:* `internal/extract/runtime/runtime.go:1-11,61-64,67,116`.

---

## Cross-cutting findings

### 1. Verdict + gating (computeVerdict, baselines, `--base` delta mode)
Gate-finding FAIL path is solid and uniformly wired. Everything downstream of it leaks:
- **Delta-sign inversion is systemic**, not a one-off bug: it hits every count-mode, higher-is-worse metric run through `computeVerdict`'s generic `Delta<0→WARN` rule (confirmed for both `cycle` and `unbalanced_edge`). Verified directly: a temporary unit test calling `computeVerdict` with `unbalanced_edge{Value:5,Band:critical,Delta:+5}` returned `VerdictPass`.
- **The documented "promote a metric to a hard gate" mechanism doesn't exist.** `metrics.<name>.gate/min_delta/max_new/max_new_high` are schema-validated everywhere but consumed nowhere outside `internal/config`, across all 5 metrics simultaneously. This is not a per-metric bug; it's a whole config-surface stub that has existed unwired since the earliest config commit.
- **Waivers can silently disable the entire FAIL path.** `config.validate()` never enforces non-empty reason/approver/expiry (despite docs claiming this is required "deliberate human friction"), and `matchWaiver` treats empty rule/from/to as match-any while `isExpired` treats empty `expires` as never-expiring. One loosely-written waiver entry permanently waives every current and future gate finding.
- **`docs/guide/metrics.md`'s "report-only, never change the verdict" claim is false for 3 of 4 named metrics** (cycle, encapsulation, coverage all compute real Delta values that DO feed the generic WARN rule; only blast_radius matches the doc). This is the root cause of the team's own previously-logged "re-baseline phantom-delta PASS→WARN" operational gotcha.
- `--base <ref>` and baseline persistence themselves (worktree isolation, schema versioning) are structurally sound — the risk is entirely in what feeds the verdict, not in the mechanism that computes deltas.

### 2. Agent-consumption surfaces (JSON/SARIF/markdown/agent_tasks/explain)
- **Baseline acceptance of grouped BC-coupling advisories is broken.** `archfit baseline --advisory` only persists the fingerprint of the rollup's *representative* (smallest-ID) edge, not all N member edges. Reproduced end-to-end: baselining a 5-edge rollup, then re-running, splits into 1 "baseline" finding (group_count=1) + 1 "new" finding (group_count=4) — 4/5 already-accepted edges resurface as new warnings.
- **agent_tasks structurally excludes the flagship metric's detail.** Confirmed live: 82 findings (all `bc/imbalanced_coupling` advisories) on this repo's own self-scan → `agent_tasks: []`. An agent driving off "fix everything in agent_tasks" sees nothing to do even when coupling_balance is critical.
- **`explain` pays the full pipeline cost (5.4s on this repo) per finding lookup**, with no cached/from-JSON mode, for data already present in a single `--json` run's `findings[]`.
- Smaller: dead `Confidence` field on every finding (never assigned anywhere in the codebase); `syntax_facts` dominates JSON payload size (~80%) yet is orphaned whenever agent_tasks is empty; SARIF `level` ignores `Severity` (keyed only on Kind+Status), so a `critical` coupling advisory and a `low` one both render as `warning`.

### 3. Config ergonomics (init/update/enrich, doctor --fix, dotted-glob trap)
- Strict decoding, deprecated-key hints, catch-all-shadowing (most-specific-glob-wins), and no-clobber/backup ergonomics are all genuinely solid and verified.
- **The one serious gap is completely silent.** A Python (or any dotted-node-language) module glob written in the wrong shape (slash instead of dotted — the exact landmine already documented in CLAUDE.md) produces zero decode errors, zero config_warnings, zero `cfg.Lint()` hits. Every edge whose endpoints don't resolve is bucketed into the same `External` count used for genuine third-party dependencies. The only visible symptom is `coupling_balance: n/a`, and the emitted evidence string ("external deps are not internal coupling seams") actively misdirects the reader away from the real cause — even though the code's own adjacent comment names the correct hypothesis ("file-path globs that never match dotted/crate node names"), that diagnosis never reaches the user-facing evidence. `config init`'s auto-discovery avoids this by construction; nothing protects a hand-authored or agent-authored config.

---

## Which metrics can an AI agent TRUST for drift decisions today?

**Nothing scored "solid."** Ranking what remains, from most to least trustworthy:

**Cautiously trustworthy for reading (not for gating), with a known caveat:**
1. **`classified_edges` (raw counts/distributions)** — honest n/a behavior, well-tested aggregation; just don't rely on the LLM-confidence-lowering signal, and remember `BySeverity[""]` is the "none" bucket.
2. **`coupling_balance` (the number itself, read via `analyze --json`, not `--base` delta)** — the per-edge book formula is verified verbatim against Khononov's own worked examples, and the n/a-vs-fabricated-number discipline holds at this layer. Avoid the `score_delta`/`--base` path (fabricates 0 for n/a sides) and don't expect it to ever gate CI (it structurally can't).
3. **`advisory_bc_imbalanced_coupling` findings (per-edge detail)** — solid book-alignment, honest abstention, good file:line evidence for Go/Python. Read `findings[]` directly since `agent_tasks[]` will be empty for these.
4. **`rule_forbidden_dependency` and `rule_new_cross_module_dependency`** — trustworthy when they fire, but both can fail open (silently match nothing) on a glob-convention mismatch with zero warning; verify the glob shape matches the language's node-path convention before trusting a clean run.

**Advisory-only — read, discuss, never automate a decision on:**
- `metric_encapsulation` (honest but mostly n/a in practice; config gate is dead so treat as informational only).
- `rule_public_api_change` / `rule_public_api_type_leak` (real signal when they fire, but with known collision/false-negative gaps — treat a "clean" result as weak evidence, not proof).
- `advisory_map_staleness` / `advisory_labels_stale` (correctly never gate; useful hygiene nudges but with real language/ordering blind spots — don't trust a "clean" result on Python or auto-registered-module repos).
- `tool_coverage_and_gaps` (useful for absent-tool cases; don't trust silence as "coverage complete" — it misses "ran but saw nothing").

**Noise — do not use for any decision, human or agent, until fixed:**
- **`metric_unbalanced_edge`** — false-confident 0/strong is the common case on any repo without hand-authored `internal:` globs (which is most repos); delta-driven verdict is sign-inverted.
- **`metric_cycle`** — same sign-inversion bug; a real new cycle silently produces PASS via the metric-delta path (the separate opt-in `rules: type: cycle` is the only trustworthy cycle signal, and even that rule fabricates its representative edge and drops Locations).
- **`metric_coverage`** — mathematically incapable of reporting a real number between 0% and 100%; a documented partial-failure case reports a fabricated 125%/"strong".
- **`metric_blast_radius`** — not just noisy on its own (0%–64% swing purely from language granularity) but actively dangerous because it silently masks the flagship coupling_balance metric when absent/misconfigured.
- **`rule_public_api_only`** — false-positives on ordinary same-module Go code under the tool's own documented example config, with a default blocking gate.
- **`rule_forbidden_layer_direction`** — the tool's own `config init` generates a rule of the wrong type, producing a permanently-vacuous blocking gate that looks configured but never checks anything.
- **`rule_internal_api_access`** — structurally inert for the most common Go project layout (including archfit's own repo).
- **`rule_public_api_max` / `structural_facts` (syntax_facts)** — counts are dominated (61-89% in this repo's own self-scan) by test-file declarations; the number does not measure what it claims to measure, yet carries a default blocking gate.
- **`agent_tasks`** — for the affected rule types (Python structural findings; public_api_max/change/type_leak in any language; multi-member cycles) the `Files[]` field is not reliably a list of real files, so automated "open these files and fix them" loops will fail silently on exactly those inputs.

**Bottom line for an agent driving CI off archfit today:** trust the gate-blocking rule findings when the underlying glob/edge-kind convention is verified correct for the target language (forbidden_dependency, cycle-as-rule with manual cross-check of Locations); treat every metric-delta-driven WARN/PASS as unreliable until the sign-inversion and dead-gate-config issues are fixed; and never let `agent_tasks[]` emptiness be read as "nothing to fix" — always cross-check `findings[]` directly, especially for coupling advisories.
