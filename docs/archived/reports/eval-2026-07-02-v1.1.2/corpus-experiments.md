# archfit corpus experiment — synthesis (v1.1.2, commit 6140aba)

Run date: 2026-07-02. 12 repos (11 in the RUNS payload + ruff, cross-checked from raw
JSON since it was run concurrently but omitted from the RUNS array), 2 probes
(dogfood drift-injection, delta-mode/gate-determinism), 1 timing probe (cold vs warm
`--full` gate latency). Raw JSON: `scratchpad/experiments/*.json`.

## Headline

v1.1.2 fixed all 4 code-level defects the prior eval (v1.1.0) flagged as high-severity.
H2 and H3 are fully fixed. H1's *fabrication* (a fake 60/mixed on 0 scored edges) is
fixed everywhere it was seen (prefect, ruff) — but ruff's coupling signal is still thin:
SCIP/rust-analyzer still reports 0 files for ruff, so its 23 scored edges are entirely
clone-derived (`by_strength: {symmetric:23, unknown:143}`, jscpd → symmetric upgrades,
zero SCIP-derived strength) — a real but narrow, biased 13%-of-edges measurement, not
a fabrication. H4 (undeclared-volatility floods) is fixed as literally specified but
re-emerges as a triageability cliff via uniform *declared/inherited* volatility. What
v1.1.2 did **not** fix — because it isn't a bug, it's the
documented design — is M1: `agent_tasks[]` stays empty for every pure coupling-drift
signal, on every repo in the corpus, including repos whose flagship `coupling_balance`
is `poor` (25–40/100). That means the tool's own documented AI-agent workflow ("read
`agent_tasks[]` on exit 1") produces **nothing** for the single metric this config
schema is built around, on 10 of 12 repos. Two new high-severity gaps surfaced this
run: a silent macOS-path-case bug that flips a real "distributed monolith" verdict to
a false-reassuring "not a monolith" one on omni, and a hardcoded narrative string that
tells agents "high-strength coupling" for edges the tool's own data classifies as
low-strength, on ccgram. Feedback-loop latency is good for small/medium single-language
repos (archfit self: ~5.3s warm) and bad for large multi-crate/monorepo cases (prefect:
8m13s cold, no incremental `--base` shortcut — it re-scans both sides in full).

## Per-repo results

| Repo | Lang | Wall (s) | Verdict | coupling_balance | Dims measured / n/a | Top issue |
|---|---|---|---|---|---|---|
| archfit (self) | Go | 8.2 | pass | 39 / poor (high conf, 284 scored, 0 abstained) | 5 / 1 (encapsulation n/a) | `coupling_balance` is the only scored score dimension; volatility_cascade drives 100 "critical" edges that are mostly single-owner cascade, not distributed monolith (7 genuine) |
| pumba | Go | 7.1 | pass | 45 / mixed (high conf, 70 scored) | 5 / 1 | 1 critical + 9 medium coupling findings never gate (advisory-only); named func-type classified as `model` not `contract`, may overstate 1 critical finding |
| spotinfo | Go | 2.6 | pass | 44 / mixed (high conf, 4 scored) | 5 / 1 | Clean; small edge count so low statistical weight; gate-clean run means agent_tasks path never exercised |
| ccgram | Python | 27.2 | **warn** (exit 2) | 55 / mixed (high conf, 497 scored, 18 abstained) | 6 / 0 | **Confirmed narrative bug**: hardcoded "high-strength coupling to a volatile target" text on 15/16 critical edges whose actual strength is `model` (book ordinal 3, low); no-import-cycles findings (2, up to 30-node SCC) have `locations: null` |
| yazi | Rust | 34.9 | pass | 60 / mixed (**low** conf, capped from 67; 66 scored, 144 abstained, 31% scored fraction) | 5 / 1 | `cohesion` referenced in `coverage_gaps` but never emitted anywhere (silent, not even n/a); test-file clone (`mod tests`) inflates a production coupling edge to `symmetric` |
| herdr | Rust | 79.3 | pass (score 25/poor; no `rules:` → gate structurally inert) | 25 / poor (high conf, 630 scored, 97% scored fraction) | 5 / 1 | `distance=cross_module_different_owner` on 390/630 edges is **not ownership** — single-owner repo falls through to `code_structure` distance and reuses the same label name, misleading; `unbalanced_edge` reports a false "strong" green next to `coupling_balance:poor` |
| omni (subtree) | Go | 36.2 | warn (exit 2) | 40 / poor (correct-case run; high conf, 1845 scored) | 5 / 1 | **NEW confirmed bug**: lowercase `--root .../workspace/omni/...` (case-variant of git root) silently drops `SubtreePrefix`, collapses `owner_source` to `none`, and flips the verdict to 58/mixed "not a distributed monolith" with zero warning |
| prometheus | Go | 86.5 | pass (score 56/mixed; no `rules:`/`forbidden_layer_direction` → gate inert despite declared `layers:`) | 56 / mixed (high conf, 620 scored) | 4 / 2 (encapsulation, effectively unbalanced_edge too) | 23 critical findings, exit 0, agent_tasks=0 — gate cannot fail on this config at all |
| storybook (subtree) | TypeScript | 12.9 | pass | 48 / mixed (high conf, 310 scored — but 5797/10222 edges bucketed "external") | 5 / 1 | dependency-cruiser `status:"partial"`, 5612 unresolved specifiers, **no reason field, no stderr warning** — flagship metric silently measures on a fraction of real coupling (bare `@storybook/*` imports unresolved, no `node_modules`); H2 crash **confirmed fixed** (same repo+config crashed exit 3 in v1.1.0, now degrades gracefully) |
| prefect | Python | 493.4 | pass | 53 / mixed (high conf, 935 scored, 0 abstained) | 6 / 0 | H1 **confirmed fixed** (grimp files_seen 9→557, scored 0→935); but `edge.from/to.path` on grouped findings (`group_count>1`) frequently points to the wrong file — verified 2/2 sampled grouped findings mislead |
| tokio | Rust | 91.9 | pass | 45 / mixed (**medium** conf — cargo-modules partial on benches/examples/stress-test) | 5 / 1 | H3 **confirmed fixed** (owner_source=git, only 8 diff-owner edges, not a flood); but 915/920 findings collapse to severity=medium (99.5%), zero triage signal |
| ruff (raw JSON, not in RUNS payload) | Rust | 7.2 | pass | 49 / mixed (**low** conf, 23 scored / 166 internal = 13% scored fraction, 143 abstained) | 5 / 1 | H1 root cause **partially open**: SCIP still reports `files_seen:0`/`status:"partial"` for rust-analyzer on ruff (empty index, correctly flagged per the shipped fix) — no longer fabricates 60/mixed on 0 data, now reports a real but sparse 49/mixed/low; `cargo_modules` is deliberately off (51-crate workspace, documented tradeoff) |

All 12 runs: `repo_left_clean=true`, no crashes, no fabricated (invented) numbers found
by spot-check-against-source verification. `warn`/exit 2 on ccgram and omni are
advisory-severity gates (`min_severity`), not tool failures.

## Prior-eval gap tracking (v1.1.0 → this run)

| Gap | Status | Evidence |
|---|---|---|
| **H1** — `coupling_balance` 0-scored-but-reported-60/mixed on prefect+ruff | **Fabrication FIXED on both; ruff's underlying coverage gap still open** | prefect: `classified_edges` went from `{total:5, scored:0}` (v1.1.0) to `{total:2905, scored:935, abstained:0}` (this run) — grimp `files_seen` 9→557 (src-layout binding fix landed). coupling_balance is now a genuine 53/mixed/high-confidence measurement, not a placeholder. **Ruff**: no longer 0-scored (now 23/166 scored, 49/mixed/**low** confidence) — the "fabricated 60" sentinel is gone and the abstain-not-fake path works. But `by_strength` on ruff is `{symmetric:23, unknown:143}` — every single scored edge is a jscpd clone-derived `symmetric` upgrade; SCIP/rust-analyzer still reports `files_seen:0`/`status:"partial"` (a 51-crate cargo workspace), so there is zero SCIP-derived strength signal and the 23 scored edges are a narrow, biased subset (clone-coupled pairs only), not a representative sample. The fix makes the gap *visible and honestly scored*, it doesn't make SCIP actually index ruff. |
| **H2** — storybook exit-3 crash on solution tsconfig (`TS18003`) | **FIXED, confirmed on the original failing repo+config** | Re-ran the exact repo+config that crashed with exit 3 / 0-byte JSON in the 2026-06-30 eval. This run: exit 0, `dependency-cruiser` degrades to `status:"partial"` (5612 unresolved) instead of a fatal error. One residual defect found in the same code path (below): the partial-coverage gap on the flagship metric is silent (no `reason` field, no stderr line) even though it is large enough to hide most of the repo's real internal coupling as "external". |
| **H3** — git-owner timeout on tokio flooding diff-owner distance | **FIXED** | `owner_source=git` (not `none`), `by_distance: {cross_module_different_owner: 8, cross_module_same_owner: 971}` — 8 edges, not a flood. No analyzer timeout reported in `tool_coverage`. Residual (not a regression of H3, a separate finding): the 8 diff-owner edges come from a single-attribution dominant-git-author-per-crate heuristic that is a low-confidence ownership proxy on a single-team OSS repo — contained, not currently a severity distortion. |
| **H4** — undeclared-volatility floods | **Mostly fixed as literally specified; the underlying triageability-cliff symptom persists in a new form** | No repo in this run shows an `undeclared` volatility bucket flooding `classified_edges.by_volatility` (yazi/omni/tokio/herdr/prefect have zero `undeclared` entries; ruff has 59/166=36%, not called out as a flood). But severity/volatility *uniformity* still buries signal: tokio 976/979 (99.7%) volatility=medium and 915/920 (99.5%) severity=medium; herdr 646/646 (100%) volatility=high (inherited uniformly from one declared module + synthetic-submodule inheritance); prefect's 97 findings are 100% severity=medium. Root cause has shifted from "volatility wasn't declared" to "volatility inheritance makes every synthetic submodule look identical" — same practical effect (an agent cannot tell which of N findings to fix first), different mechanism. |
| **M1** — `agent_tasks[]` only populates on rule violations, never on coupling advisories | **Still present, unchanged, confirmed on every repo in the corpus** | archfit(self) 100 critical coupling edges → `agent_tasks:[]`; pumba 1 critical → `[]`; herdr score=25/poor → `[]`; prometheus 23 critical → `[]`; prefect 97 findings, encapsulation critical-band → `[]`; tokio 915 findings → `[]`. Confirmed by the dogfood probe as *working as designed* for actual rule violations (populates correctly, with file:line + fix goal, in ~5.3s) — the gap is specifically that BC/`imbalanced_coupling` findings are hardcoded `kind:"advisory"` and structurally cannot produce a task, regardless of severity or how many `rules:` a config declares. Every repo without a `forbidden_dependency`/`layer_direction` rule targeting coupling (10 of 12 in this corpus) gets zero machine-actionable tasks no matter how bad `coupling_balance` gets. |

### New gaps found this run (not in the prior H1–H4/M1 list)

1. **CONFIRMED, high severity — macOS case-variant subtree `--root` silently corrupts CODEOWNERS distance on omni.** `--root /Users/alexei/workspace/omni/server/...` (lowercase, case-differs from git root's canonical `/Users/alexei/Workspace/omni`) drops `SubtreePrefix` to `""` (the `filepath.Rel` `..`-guard trips on the case mismatch before `os.SameFile`-based `snapScanRoot` ever runs, because `snapScanRoot` only snaps a `--root` that points at the git root itself, not a deeper subtree with a case-variant ancestor). Effect: `owner_source` flips from `codeowners` to `none`, every cross-module edge collapses to `cross_module_same_owner`, and `coupling_balance` flips from the true `40/poor` ("distributed-monolith risk present") to a false `58/mixed` ("not a distributed monolith") — with no warning anywhere in stderr or `config_warnings`. `classified_edges.total` is byte-identical between the two runs, isolating the bug to ownership resolution, not file scoping. This is a real risk for any agent/CI job that types a subtree path in a different case than the checkout.
2. **CONFIRMED, medium severity — hardcoded severity narrative on ccgram.** `internal/engine/advisories.go` (`bcRiskClause`) emits "high-strength coupling to a volatile target..." for every `SeverityCritical` edge at non-high distance, regardless of the edge's actual `strength` value. Verified 15/16 critical edges in the ccgram run have `strength=model` (book ordinal 3 of 10, a **low** strength value) — the prose asserts "high-strength" for weak-strength edges. An agent trusting the text over `matched_by.strength`/`cheapest_move` would pick the wrong remediation.
3. **CONFIRMED, medium severity — `edge.from.path`/`edge.to.path` unreliable on grouped findings (prefect).** For findings with `group_count>1`, the labeled path is an arbitrary representative of the group and frequently does not match the actual import at the finding's own `locations[]`. Verified on 2 sampled findings (group_count 17 and 38) that most locations import something entirely different from the labeled `to.path`. An agent editing "the labeled file" would edit the wrong one; `locations[]` (not `edge.path`) is the only reliable pointer.
4. **CONFIRMED, low-medium severity — `--base` + `--format scorecard` silently drops the delta.** `cmd/archfit/analyze.go` computes the full base-side score (~7-13s of extra worktree checkout+scan) via `scoreBaseRef` but `scorecard.Render(diag, w)` has no base parameter, so the output is indistinguishable from a plain non-base scorecard run — contradicting the `--help` text's own example (`archfit --base origin/main # scorecard delta vs base ref`).
5. **Confirmed, low severity — silent metric (`cohesion`) referenced but never emitted (yazi)** — appears only inside `coverage_gaps.affected_metrics` for the disabled `cargo-modules` analyzer; no key, value, or `n/a` entry exists anywhere in `metrics[]`. Inconsistent with `cycle`/`blast_radius`/`encapsulation`, which all get an explicit `n/a` entry under the same gap.
6. **Confirmed, low severity — a genuine clone-detection blind spot found by the dogfood probe.** Cross-module code duplication is only surfaced (upgraded to `symmetric` strength) when a graph edge *already exists* between the two modules (`classify.go:414-432`). A ~24-line clone injected between two modules with **no** existing import relationship produced a byte-identical PASS to baseline — no metric, finding, or score changed. There is no standalone duplication metric independent of an existing coupling edge. This is exactly the shape of drift an AI agent doing a copy-paste extraction into an unrelated package would create, and it is currently invisible end-to-end.

## Probe outcomes

**Probe 1 — dogfood-gate drift-injection (`passed=true`, all 5 steps matched expected outcome):**
- Baseline: exit 0 PASS, 82 advisory findings, `coupling_balance=39/poor`, 9.50s (cold).
- Rule-boundary drift (forbidden `core→adapter` import injected): exit 1, FAIL, ~5.25s warm. Output gave file:line (`internal/classify/classify.go:13`), rule_id, matched edge, and `agent_tasks[]` with a concrete fix goal + a ready-to-run validation command. **An agent could fix this from the JSON alone.**
- Metric-degradation drift (24-line cross-module clone injected between two modules with no existing import edge): byte-identical PASS to baseline — the clone-detection blind spot (new gap #6 above), independently confirmed by running `jscpd` directly (it *does* see the clone; archfit just never surfaces it because no edge exists to upgrade).
- Revert: clean tree confirmed, gate PASS restored, ~5.28s.
- Warm per-edit-cycle latency for this repo converges to **~5.3s** regardless of pass/fail outcome; only the very first run pays a ~9.5s cold SCIP/ast-grep index cost.

**Probe 2 — delta-mode (`--base`) + gate determinism (`passed=true`):**
- `analyze --base HEAD~10` renders a correct dimension delta (only `coupling_balance` is scored on this config: 37→39, overall +2) across text/json/markdown, worktrees always cleaned up.
- n/a-side handling verified: `--base 3bca570` (repo's first commit, config globs don't match that ancient tree) scores `base_overall=0/n/a`, delta renders "overall no change" / `score_delta.delta:0` — **no phantom ±39 swing**, matching the documented n/a-aware delta contract.
- `analyze --gate --full` run twice back-to-back: byte-identical stdout, exit 0 both times (determinism confirmed, stronger than same-verdict).
- Real gap found (new gap #4 above): `--format scorecard` + `--base` silently discards the computed delta while still paying its cost.
- Minor: one stale `archfit-base-*` temp dir found from a prior abnormal termination — `defer os.RemoveAll` doesn't fire on external kill (known, narrow failure mode, not exercised by this probe's own clean runs).

## Timing table

| Scenario | Wall time | Note |
|---|---|---|
| archfit (self, Go, `--full`, cold) | 6.28s | meets <30s per-edit target |
| archfit (self, Go, `--full`, warm) | 6.02s | no measurable cold/warm delta — **no gate-level cache exists** |
| archfit (self, dogfood probe, warm steady-state) | ~5.3s | 3 consecutive runs converge here regardless of pass/fail |
| ccgram (Python, `--full`, cold) | 30.99s | over the <30s target |
| ccgram (Python, `--full`, warm) | 30.30s | no cold/warm delta |
| prometheus (Go, `--full`, cold, isolated) | 47.15s | over target |
| prometheus (Go, `--full`, warm, isolated) | 47.45s | warm *slower* than cold — noise, confirms no caching effect |
| prometheus (Go, `--full`, corpus run w/ concurrent load) | 86.47s | ~1.8x slower under concurrent contention — CPU-bound subprocess fan-out |
| omni (Go subtree, correct-case, `--full`) | 36.18s | |
| omni (Go subtree, case-bug repro, under heavier concurrent load) | 86.29s | not a real regression — concurrency noise per task's own caveat |
| storybook (TS subtree, `--full`) | 12.91s | fastest large-ish repo, but see coverage-gap issue above |
| yazi (Rust, `--full`) | 34.93s | |
| herdr (Rust, `--full`) | 79.28s | |
| tokio (Rust, `--full`) | 91.91s | |
| prefect (Python, `--full`) | **493.44s (8m13s)** | largest repo in corpus; far outside any per-edit loop budget |
| ruff (Rust, `--full`) | 7.19s | fast, but SCIP is empty (0 files) — cheap because most of the graph is abstained, not because the tool is efficient here |
| `--base HEAD~10` (text/json/markdown/scorecard), archfit self | 13.0–14.7s | ~2x the plain `--full` cost (5.3s) — pays for a full second-side scan via temp worktree, not an incremental diff |
| `--base <first commit>` (n/a base side), archfit self | ~9s | cheaper because the base side scores 0 edges fast |
| `archfit --help` | 0.02s | startup overhead is negligible; 100% of wall time is the analysis pipeline |

**Cold vs warm:** confirmed no persistent cache exists for the `--gate`/`--full` pipeline
(`--no-cache` only affects the off-gate `--llm` path's `.archfit-cache/llm`). Cold and
warm times are within noise on every repo tested; the bottleneck is CPU-bound external
tool subprocess fan-out (`go list`/dependency-cruiser/ast-grep/jscpd/SCIP/grimp/cargo),
confirmed by user-time-exceeds-real-time on the larger repos.

## Feedback-loop verdict vs the stated goal

**Goal: "fast feedback loop for AI agents to prevent architecture drift."**

Two separable claims, scored separately:

1. **When drift is a real rule violation the tool is configured to catch, the loop
   works well and is fast.** The dogfood probe is the clean positive case: ~5.3s warm,
   exit 1, file:line-precise `agent_tasks[]` with a fix goal and a rerun command. This
   is the intended CI/pre-push shape and it delivers.
2. **For the flagship metric (`coupling_balance`) the loop is largely broken as an
   agent-actionable signal, independent of speed.** `agent_tasks[]` never populates
   from coupling drift (M1, unchanged, confirmed on every one of 12 repos), so an
   agent following the tool's own documented protocol sees nothing even when
   `coupling_balance` is `poor` (25–40/100, herdr/archfit/omni) or the run carries
   dozens of critical-severity findings (prometheus 23, prefect encapsulation
   critical-band, ccgram 15/16 critical mislabeled). The agent must instead parse
   `findings[]` directly — which works (locations are real and mostly accurate,
   confirmed by spot-checks) but is not the paved path the tool advertises, and two
   confirmed defects (ccgram's mislabeled "high-strength" narrative, prefect's
   unreliable grouped-edge path) can actively misdirect an agent that trusts the
   convenient fields over `locations[]`/`matched_by`.
3. **Latency does not scale to large repos and there is no incremental fast-path.**
   Small Go repos (archfit, spotinfo, pumba) and scip-light Rust repos (ruff) finish
   in single-digit seconds. Medium repos (ccgram, storybook, omni, yazi) land in the
   12–37s range — borderline for a tight per-edit loop. Large repos (prometheus,
   herdr, tokio, and especially prefect at 8m13s) are well outside any reasonable
   per-edit budget, and `--base` (the one delta/incremental-shaped flag) does not
   actually shortcut this — it re-runs the full pipeline on both worktree sides, so
   for prefect an agent using `--base` would pay roughly 2x the already-prohibitive
   full-scan cost. There is no cache to amortize repeated runs on an unchanged repo.
4. **One real drift class is currently invisible end-to-end regardless of speed or
   agent_tasks wiring**: cross-module duplication introduced between two modules that
   don't already share an import edge (probe 1, gap #6) — exactly the pattern an
   agent doing a copy-paste extraction would create.

Net: the tool is a fast, reliable, well-evidenced *fitness function for hard rule
violations* on small-to-medium repos, and that part of the loop is genuinely good.
It is not yet a fast feedback loop for the coupling-drift dimension it is named and
architected around — that dimension is accurate and honestly confidence-scored
(the v1.1.2 abstain-not-fake work paid off: no fabricated numbers found anywhere
in 12 repos) but structurally disconnected from the agent_tasks contact surface, and
too slow on large repos to run per-edit even if it were connected.
