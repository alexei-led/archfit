# archfit Evaluation Findings — 2026-06-28

Seven repos, four languages (Go ×4, Rust ×2, Python ×1, TypeScript ×1), full + delta + LLM modes.
Independent Balanced-Coupling reviews provided for all seven.

---

## 1. Metric Collection

### What archfit measures correctly (per language)

**Go** (spotinfo, pumba, omni): dependency graph, LOC, blast-radius/fan-in/instability/propagation-cost, change-coupling, boundary violations, go test density, struct-field density via ast-grep. SCIP enriches cohesion_lcom and risk_hub when enabled; gocyclo or lizard covers complexity. All structurally-present metrics computed accurately when tools are enabled.

**Rust** (herdr, yazi): crate-level dependency graph from `cargo metadata` is accurate. With `tools.cargo-modules: on`, intra-crate module graph fires; with `tools.scip: on`, edge strength resolves. LOC, blast-radius, change-coupling, and clone detection all work — when CrateRoots are populated (see bugs below).

**Python** (ccgram): import graph (grimp), LOC, change-coupling, architecture-fitness, jscpd clones all work. lizard complexity works with `backend: lizard` explicit. SCIP strength is blocked by a Python-version gap (see structural gaps below).

**TypeScript** (codegraph): dependency-cruiser import graph, LOC, change-coupling, clone detection, complexity via ast-grep proxy all work. cohesion_lcom stays n/a despite SCIP running (wiring gap — IntraRefs empty).

### Genuine structural gaps (not bugs)

| Metric                                | Language               | Reason                                                                                                                                                                                                                                          |
| ------------------------------------- | ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| encapsulation                         | Go, Rust               | Compiler-enforced privacy; no intrusive cross-boundary edges exist by construction                                                                                                                                                              |
| unsafe_density / global_state_density | Go, Python, TS         | Rust-only concept; ast-grep rules don't cover Go/Python                                                                                                                                                                                         |
| panic_density / struct_field_density  | Python, TS             | Go/Rust-specific constructs                                                                                                                                                                                                                     |
| file_mutual_import                    | Python, Rust           | TypeScript-only metric                                                                                                                                                                                                                          |
| change_locality (metric)              | all                    | Delta-mode-only by design; n/a in full mode is correct                                                                                                                                                                                          |
| cohesion_lcom                         | Python (ccgram)        | scip-python 0.6.6 does not support Python 3.14; exits 0 with a 65-byte empty index delivering zero strength annotations. This is a tool-version gap, not a config error — no config change fixes it until scip-python ships Python 3.14 support |
| cohesion_lcom                         | TypeScript (codegraph) | SCIP runs and yields 5832 symbols, but `g.IntraRefs` is empty — `cohesion.go:79` gates on `len(g.IntraRefs)==0`; likely a wiring gap in how scip-typescript occurrence data is resolved to flat module keys                                     |

### Bugs causing wrong n/a or wrong values

**macOS case-path bug (highest impact):** `filepath.Abs` and `filepath.Rel` perform string comparison on macOS case-insensitive filesystems. When git rev-parse returns `/Users/alexei/Workspace/...` (capital W) and the `--root` arg or cargo metadata resolves to `/Users/alexei/workspace/...` (lowercase), `filepath.Rel` produces a `../../...` path, triggering defensive returns of empty string. This single root cause corrupts:

- `crateRoots()` in Rust analysis → CrateRoots empty → `structural_weight`, `change_coupling`, `change_amplification` all return n/a; `hidden_coupling` silently returns 0 instead of ~84-108 (herdr)
- scip-python `--cwd` argument (ccgram): capital-W path → 17 MB index; lowercase alias → 65-byte empty index; Linux CI unaffected (case-sensitive)
- `subtreePrefix` in scope.go for monorepo delta mode (omni): 11,253 monorepo files enter Scope.Changed instead of 1,251 subtree files; delta change_locality shows 0 cross-module edges (phantom green)

Fix: `filepath.EvalSymlinks` on both sides before `filepath.Rel`, in `crateRoots()`, the SCIP invocation, and `subtreePrefix`.

**jscpd Clusters parse failure (herdr):** jscpd finds 1,522 clone pairs (10.8% duplication) in a standalone run with identical args. Inside archfit `in.Duplication.Clusters` is empty → `functional_candidates` returns n/a. Distinct from CrateRoots. Likely a jscpd JSON format drift or absolute-path normalization failure.

**change_locality metric n/a while siblings compute (yazi):** `change_coupling` and `change_amplification` both compute from the same git history in the same run. The `change_locality` metric (distinct from the dimension) returns n/a. The shared name (metric and dimension, one n/a, the other 80/100) compounds the confusion.

**Synthetic .test modules inflating fan-in (spotinfo):** `go/packages` with `NeedTypesInfo` registers `cmd/spotinfo.test` as a distinct module. `inbound_module_fanin` for `internal/spot` = 3 (includes the test module); blast_radius/instability/change_amplification = 2 (correctly exclude it). Cross-metric inconsistency.

**tools.syntax.enabled gate (all Go repos):** `panic_density`, `test_density`, `struct_field_density` require `tools.syntax.enabled: on`. The ast-grep binary shows `ok` in `tool_coverage` and `files_seen=0` is the only indicator the pass was skipped. All three density metrics are silently absent without opt-in. None of the seven evaluated configs had this enabled.

**lizardExcludes disconnected from scope.DefaultExclusions:** `internal/extract/complexity/complexity.go` has a hardcoded exclusion list that does not include `.gitnexus/**` or `.codegraph/**`. Latent; reproduces on any project with a `.gitnexus` directory (not present in the seven evaluated repos).

### Cross-repo patterns

- Tools are disabled by default. All seven repos needed `tools.scip`, `tools.complexity`, and `tools.clones` enabled explicitly. `metrics.risk_hub` and `metrics.functional_candidates` also need separate opt-in. A repo with all tools installed but no explicit config delivers roughly half the available metrics.
- When tools are missing from PATH and the config uses `backend: auto`, archfit silently n/a's complexity (spotinfo: gocyclo not on PATH). Error message says "install gocyclo" even when lizard is present and would work.
- omni (largest repo, ~1001 packages) had no config at all. Zero module definitions caused coupling_balance = 60/low-confidence from 0 scored edges — a phantom serviceable score masking a critical architectural debt.

---

## 2. Metric Correctness

Every confirmed wrong number from the adversarial spot-checks, with repo and evidence.

### coupling_balance = 60/mixed (ccgram HEAD, low confidence)

Phantom score. 1 of 502 edges scored (0.2% coverage) due to the macOS SCIP path bug producing an empty index. The displayed 60/mixed is a hedge value, not a measurement — it reads as MORE balanced than the v4.0.0 measurement (40/poor, which had a real SCIP index at a different path). **A hedge that looks better than the true architecture is directionally misleading**, not just imprecise.

### coupling_balance version diff: 40→60 (+20) for ccgram HEAD vs v4.0.0

Entirely a tool artifact from the macOS SCIP path bug, not an architectural change. The reliable drift signals (cohesion_modularity 11→5 worse, change_locality 84→66 worse) are buried under the phantom coupling improvement.

### Two severity models disagreeing for clone-upgraded symmetric edges (codegraph)

`BalanceResult()` (discrete legacy model, drives the gate-facing `Severity` field) and `BookScorer.Score()` (formula model, drives `score_band` and the metric) co-exist and disagree for `StrengthSymmetric` (S=9) edges. Example: S=9, D=7 (DiffOwner), V=high → `BalanceResult` → `SeverityCritical`; BookScorer → `max(|9-7|, 10-8)+1 = 3` → `ScoreBand(3)` = `SeverityHigh`. The CI gate rejects at `critical` while the metric reports `high`. Three codegraph edges are affected. Root cause: `BalanceResult` was not updated when `StrengthSymmetric` (S=9) was added via the symmetric-from-clones rule.

### explain severity=high vs check severity=medium for same fingerprint (ccgram, finding 2c5200c6)

`archfit explain` and `archfit check` produce different severity for the same finding. Root cause: explain re-derives severity via a separate code path that does not apply the same volatility cascade pass check uses. (The `why` field's directional claim — "high volatility → high severity" — is consistent with the formula: high V shrinks the `10-V` term, enabling lower balance values that map to higher severity bands. The claim is not wrong; the severity value itself is what diverges between the two commands.)

### file_extraction_coverage = 1.02 (codegraph)

A ratio >1.0 is impossible by definition. SCIP indexes 404 files (including `.d.ts` and node_modules type declarations) while LOC counts 131 source files. Numerator and denominator use different file scopes.

### inbound_module_fanin = 3 for internal/spot (spotinfo)

The third "importer" is `cmd/spotinfo.test`, a synthetic test-compilation package from `go/packages NeedTypesInfo`. Production importers: 2. blast_radius and instability correctly exclude it. Fan-in and blast_radius report contradictory numbers for the same node.

### panic_density = 208 (pumba) — 100% false positive

All 208 panics are in moq-generated mock files: `mocks/` (170), `pkg/container/mock_*.go` (32), `pkg/runtime/mock_*.go` (6). Real production panic count: 0. The low-confidence flag is present but does not disclose the root cause.

### functional_candidates = 13 (pumba) — 12/13 pairs are test or generated code

jscpd runs on 255 files (production + `_test.go` + mock files). Only 1 of 13 cross-module pairs is production code (iptables vs netem, 6 instances). The resulting 25-point drop in cohesion_modularity (58→33) is driven almost entirely by noise.

### hidden_coupling = 0 (herdr HEAD) — silently wrong, should be ~84-108

Git co-change data IS populated (hidden_coupling did not return n/a — it returned 0). The CrateRoots bug causes `ModuleKeyResolver` to return `''` for all file paths; pairs where `a==''` or `b==''` are filtered, producing 0 rather than n/a. v0.6.9 measured 108 co-changing module pairs under the same binary. **This is the worst class of error: wrong and presented as measured.**

### structural_weight = n/a (herdr HEAD) — should find ~10-20 god modules

v0.6.9 measured 15 god modules; `headless.rs` (7,843 LOC), `integration/mod.rs` (6,365 LOC), and `app/actions.rs` (5,223 LOC) are well above any 3×-median threshold. CrateRoots bug causes `modLOC` to be empty → `len(modLOC)<2` guard returns `NACount`. Result: herdr HEAD scores cohesion_modularity 60/mixed instead of the true ~5/critical.

### change_locality delta mode = 0 cross-module edges (omni — phantom green)

11,253 monorepo files enter Scope.Changed (all files in the monorepo) instead of 1,251 subtree files. All 11,253 match no configured module → 0 cross-module edges. The full-mode `change_coupling` (which uses `git.History` with the correct `-- path/to/subtree` filter) is unaffected.

### coupling_balance distance inflation for flat single-segment module names (codegraph, spotinfo)

When `isDegenerateOwnerMap()` fires (all modules share one owner), archfit falls back to `codeStructureDistance`. For flat single-segment names (`extraction`, `resolution`, `mcp`, `spot`), the code path returns `DiffOwner` (ordinal 7) for every cross-module pair. Under book semantics, a single-team co-deployed codebase has minimum coordination cost → `SameOwner` (ordinal 4). With DiffOwner: functional+high → balance=2 (critical). With SameOwner: functional+high → balance=5 (medium). Estimated coupling_balance under correct semantics: codegraph ~60-65 vs reported 40; spotinfo's mcp→spot edge → medium vs reported critical. The archfit `.archfit.yaml` comment claiming single-owner always resolves to `cross_module_same_owner` is factually wrong.

---

## 3. Interpretation and Calibration

### Verdict pattern: in every disagreement the architect is more accurate, and the error direction is predictable

**archfit too HARSH (distance inflation):** spotinfo coupling_balance 31/poor vs architect adequate/62; codegraph 40/poor vs architect 55/mixed. Both are single-team co-deployed codebases with flat module names. The flat-name DiffOwner bug inflates severity; the architect correctly applied low-distance treatment. Two independent sources (architect + archfit's own estimated correction) agree on the error direction.

**archfit too LENIENT (macOS path bugs hiding structural problems):** herdr cohesion_modularity 60/mixed (HEAD, CrateRoots bug) vs architect 40/poor and v0.6.9 measurement 5/critical. omni change_locality 100/strong (phantom from 11,253-file subtree leak) vs architect's assessment of an uncontrolled shared-kernel catch-all. Without the macOS bugs, archfit's overall scores for both repos would land closer to the architect's weaker verdicts.

**archfit too LENIENT (jscpd test/generated noise):** pumba cohesion_modularity 33/poor vs architect 65/mixed. The 25-point drop is driven by 12/13 cross-module pairs in test or generated code. Architect correctly assessed cohesion as mixed by reading production code.

### Per-repo calibration table

| Repo      | archfit overall               | Architect overall                     | Coupling direction                                                                                     | Verdict                                                     |
| --------- | ----------------------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------- |
| spotinfo  | 62/serviceable                | adequate/62                           | archfit harsher (31 vs 62) — flat-name DiffOwner bug                                                   | architect correct                                           |
| pumba     | 54/mixed                      | 72/mixed (coupling 72; overall mixed) | archfit harsher on cohesion (33 vs 65) — jscpd noise                                                   | architect correct                                           |
| ccgram    | 37/poor (phantom coupling 60) | ~42/mixed (bc ~40)                    | archfit phantom lenient on coupling                                                                    | architect correct; SCIP gap hid critical findings           |
| codegraph | 34/poor                       | ~55/mixed                             | archfit harsher (coupling 40 vs 55) — flat-name DiffOwner; archfit too lenient on boundary (10 vs ~52) | architect correct overall                                   |
| herdr     | 37/poor                       | 38/poor                               | close agreement; archfit hid cohesion (60 vs true ~5) — macOS bug                                      | architect correct on cohesion                               |
| yazi      | 51/mixed                      | 46/weak                               | archfit mildly lenient; both agree on critical dds/pubsub smell                                        | architect correct; archfit missed topological layer finding |
| omni      | 52/mixed                      | ~38/weak                              | archfit lenient overall; phantom change_locality 100; coupling_balance accurate after module defs      | architect correct overall                                   |

### Intrusive coupling detection gap

ccgram's 46-file intrusive singleton bypass (window_store/thread_router accessed as shared mutable singletons from handler layer) was the architect's primary finding. archfit found 1 intrusive edge (1/502 scored). With SCIP absent (empty index), archfit had no strength signal to detect the pattern. Conversely, herdr's LLM review hallucinated intrusive coupling ("plugin subsystem intrusive coupling") when all 364 findings are functional strength — the LLM invented evidence the tool didn't find, and the post-verification pass didn't catch it.

### Architectural smell archfit has the data to surface but doesn't

The config alignment for yazi correctly placed yazi-config at topological layer 11. archfit has the layer assignments, and `forbidden_layer_direction` catches some violations. But no finding is emitted for the pattern: "this module's observed topological rank (11) is far from its expected role rank (2 for a config module)." The architect flagged this as the headline design smell; the LLM review missed it because no metric surfaces it.

---

## 4. Drift Detection

### What `--base` delta mode actually does

Delta mode analyzes the CURRENT codebase and compares the finding ID set against a stored baseline at `--base <ref>`. It does NOT check out the old commit, score it, or compare metric values. The change_locality metric is the only metric that changes in delta mode (files changed since base ref). All other metrics reflect the current tree.

Consequence: `--base v1.0.0 --advisory` and a full-mode run on HEAD produce identical metric blocks for all structural dimensions. Three repos confirmed this:

- **pumba**: 9 files changed since v1.1.4; all 3 findings appeared in both modes
- **herdr**: 110/115 `.rs` files changed since v0.6.9; delta ≡ full
- **codegraph**: 154 changed files (essentially the full codebase); delta ≡ full

### The "new" label misleads

`status: new` means "not in baseline fingerprints", which users read as "newly introduced regression." In every evaluated repo with prior archfit usage, pre-existing findings appeared as `new` because no `.archfit-baseline.json` had been committed. First-run delta output is indistinguishable from genuine regression output. When no baseline exists, delta mode should emit a prominent warning: "no baseline found — all N findings are untracked; commit a baseline to enable drift detection."

### True version comparison requires a worktree script

To actually compare architecture between two versions, the evaluator needed:

1. `git worktree add` at the old tag
2. Run `archfit score` on the old worktree
3. Run `archfit score` on HEAD
4. Manual diff of JSON outputs

This is brittle (worktree path placement affected SCIP output on macOS; node_modules absence in old TS worktrees changes SCIP file count). For omni (57k-commit monorepo), step 1 alone takes several minutes and risks workspace corruption; stage G was skipped.

**Recommendation: implement `archfit diff <ref1> <ref2> --format scorecard`** as a native subcommand. It should manage its own clean temp worktrees, normalize paths via `filepath.EvalSymlinks`, and emit a structured metric delta table. This is the single UX gap that forced the most manual workaround work across all seven repos.

### What delta mode is genuinely good for

Delta mode adds real value when the changed file set is small (PR-level, <20 files) and a committed baseline exists. The finding-ID comparison correctly identifies regressions on those changed modules. The problem is not the design — it is the absence of a complementary scorecard-diff mode for version-over-version tracking.

---

## 5. Explanation Quality

### Deterministic `explain` (no --llm)

Across all seven repos: `explain` without `--llm` restates the fields already visible in `scan` output (edge, rule, why formula recap, cheapest_move). The `cheapest_move` is mechanically correct per the formula but not actionable — "lower_volatility" for a high-volatility core module is not a real option. No remediation options, no code context, no concrete next step. Value-add over `scan`: zero.

### `explain --llm` quality

Good narrative quality when the underlying finding is real. Correctly identified the herdr api::client → api::schema cycle and recommended an anti-corruption layer; correctly identified the codegraph tree-sitter.ts bidirectional cycle and its cascade impact. No hallucinations in most explain outputs.

**Systematic hallucination: organizational distance framing.** When a finding carries `distance: cross_module_different_owner`, both `review --llm` and `explain --llm` generate organizational framing: "cross-team coordination cost", "separate teams", "knowledge-boundary friction." For single-owner repos (spotinfo, codegraph, herdr synthetic modules), this is invented. The root cause is the misleading label emitted when `isDegenerateOwnerMap()` fires and falls back to code-structure distance — the `distance_basis: code_structure` field exists in JSON but is absent from the human-readable `why` text and is not visible to the LLM. Two independent LLM passes (review + explain) generated this hallucination for the same finding. The fix is in the label, not the LLM: add `(degenerate_owner_map)` qualifier to the distance label when code-structure fallback is used.

**Confirmed hallucination surviving post-verification (herdr):** LLM review claimed "plugin subsystem intrusive coupling." The self-check header said "3 unsupported claims dropped" but the intrusive claim was not one of them. All 364 herdr findings are `functional` strength. The post-verification pass is not reliable for strength-attribute claims.

### `review --llm` quality

Fidelity to deterministic facts is consistently high (herdr, codegraph, omni all verified). One systematic error: codegraph review stated "absence of lizard means cyclomatic complexity data is unavailable" when the scorecard shows 118 complex functions (CCN>15) via the gocyclo+ast-grep backend. `lizard: absent` in `analysis_confidence` is misread as "complexity unmeasured." The metric and tool-coverage display create this confusion — `lizard: absent` should be `lizard: not configured` to distinguish it from a metric that is actually n/a.

Value-add beyond scorecard restatement: moderate but real. Risk clustering, blast-radius urgency framing, and concrete architectural move suggestions (interface extraction, anti-corruption layer patterns) add context not derivable from raw numbers alone. The subdomain suggestions conflict with explicit config in one case (omni: review suggested cloudproviders as core; config marks it supporting) without flagging the conflict.

### `explain --root` missing — breaks monorepo use

`archfit explain` has no `--root` flag. `ExplainCmd` in `cmd/archfit/explain.go` hardcodes `root=""`, causing `runPipeline` to expand scope to GitRoot (entire monorepo). In omni: `check --root .../scheduled-tasks` produced 102 findings; `explain` (no `--root`) re-analyzed the full monorepo with scheduled-tasks config, finding a completely different service's edges. The LLM narrative was architecturally sound for the wrong codebase. This is a correctness bug: finding IDs from a scoped check cannot match an unscoped explain re-run.

---

## 6. Config Experience

### Per-repo tuning summary

| Repo      | Changes needed | Primary gaps                                                                                                          |
| --------- | -------------- | --------------------------------------------------------------------------------------------------------------------- |
| spotinfo  | 10             | scip/complexity/clones disabled; no-op forbidden rule; wrong module classification (mcp as core); gocyclo not on PATH |
| pumba     | 4              | complexity/scip/clones disabled; no-op forbidden rule                                                                 |
| ccgram    | 1              | backend: lizard not set (SCIP gap unfixable by config)                                                                |
| codegraph | 5              | scip/clones/complexity disabled; no-op forbidden rule; unmodeled top-level files                                      |
| herdr     | 4              | cargo-modules/scip/clones disabled; missing owner field                                                               |
| yazi      | 6              | 26 wrong layer ranks; wrong role annotations; clones/complexity disabled; N→N+1 rule gaps                             |
| omni      | from scratch   | No config at all; 10 modules defined; all tools; 5 rules                                                              |

### Patterns that `archfit init` should handle automatically

1. **Detect installed tools and emit `enabled: on`**: scip-go, scip-typescript, scip-python, jscpd, lizard, cargo-modules are all installed but disabled in every baseline config. The init command should run `archfit doctor` output and emit the corresponding `tools.*` block.

2. **Warn on no-op forbidden_dependency rules**: Three repos (spotinfo, pumba, codegraph) had rules with `type: forbidden_dependency` but no `from:` or `to:` fields. `doublestar.Match("", path)` never matches; the rule provides zero protection despite `gate: fail`. A schema-level validation error would catch this before any run.

3. **Detect gocyclo absence and suggest `backend: lizard`**: Auto-backend silently n/a's complexity for Go when gocyclo is absent. If gocyclo is not on PATH and lizard is installed, emit a warning with the fix.

4. **Emit `volatility_cascade_enabled: true` when a core/high-volatility module has fan-in >2**: The cascade (BC Ch9) was missing from spotinfo and yazi; adding it is a one-line config change that enables the book's propagation semantics.

5. **Suggest module boundaries from package structure**: omni required hand-crafting 10 modules covering ~1001 packages. A heuristic grouping (top-level directories + frequency clustering from git history) would give users a starting point rather than an empty modules section.

6. **Set `metrics.risk_hub` and `metrics.functional_candidates` together with their tool dependencies**: These metric-level flags are non-obvious companions to `tools.scip` and `tools.clones`. They should be emitted automatically when the tool is enabled.

---

## 7. Prioritized Recommendations

Ranked by: (correctness impact) × (breadth of repos affected) × (fix effort).

### P1 — Fix flat single-segment module distance: use SameOwner floor when isDegenerateOwnerMap fires

**Evidence:** codegraph (coupling_balance 40 vs correct ~60-65), spotinfo (mcp→spot labeled cross_module_different_owner; architect says low-distance co-deployed). Affects every single-team repo with flat module names (common in Go and TypeScript projects). The archfit `.archfit.yaml` comment claiming "single-owner always resolves to SameOwner" is factually wrong.

**Fix:** When `isDegenerateOwnerMap()` is true, return `SameOwner` (ordinal 4) as a floor before applying `codeStructureDistance`, or document that flat naming inflates severity and recommend nested paths.

**Effort:** Small (one function in classify.go + distance tests).

### P2 — macOS case-path normalization via filepath.EvalSymlinks

**Evidence:** herdr (structural_weight n/a, hidden_coupling=0 silently wrong, cohesion_modularity 60 vs true ~5), ccgram (SCIP empty index → coupling phantom 60), omni (delta change_locality 0 phantom green from 11,253-file scope).

**Fix:** `filepath.EvalSymlinks` in `crateRoots()`, the SCIP `--cwd` argument builder, and `subtreePrefix` in `scope.go`. Note: Linux CI is unaffected (case-sensitive filesystem); this is a macOS-local correctness issue but impacts the primary developer workflow.

**Effort:** Small (3 call sites).

### P3 — Retire BalanceResult() as severity source; derive severity from BookScorer.Score().Band

**Evidence:** codegraph — 3 symmetric-strength edges (S=9, clone-upgraded) show `severity=critical` (BalanceResult) while `score_band=high` (BookScorer, balance=3). The CI gate rejects at `critical`; the book formula says `high`. The fix also resolves the explain vs check severity discrepancy (ccgram finding 2c5200c6).

**Fix:** In `classify.Run`, set `finding.Severity = ScoreBand(edgeScore.Balance)` instead of `BalanceResult(c)`. BalanceResult was written before BookScorer existed; the book formula is now the canonical severity source.

**Effort:** Small-medium (change classify.go + update BalanceResult tests to confirm it's no longer called for severity).

### P4 — Add --root to explain command (fix ExplainCmd hardcoded root="")

**Evidence:** omni — explain analyzed a different service than check because `ExplainCmd` has no `Root` field and calls `runPipeline` with an empty string. LLM narrative described the wrong codebase. Affects every monorepo subtree user.

**Fix:** Add a `Root` string field with yaml and kong tags to `ExplainCmd` and pass it to `runPipeline`, identical to `CheckCmd` and `ScanCmd`.

**Effort:** Trivial (one-line struct addition + pass-through).

### P5 — Exclude generated/mock/test files from jscpd scope

**Evidence:** pumba — 12/13 functional_candidates pairs are `_test.go` or `mock_*.go` files; cohesion_modularity drops 25 points from false signal. pumba panic_density = 208 (100% moq-generated files). jscpd runs on 255 files where 175 are non-production.

**Fix:** Pass `--ignore` patterns to jscpd covering `*_test.go`, `mock_*.go`, files in modules with `role: generated`, and test directories. Alternatively, honor the project's `.gitignore`-equivalent exclude list.

**Effort:** Medium (jscpd invocation + integration test with test files present).

### P6 — Flag SCIP empty index as warn, not ok

**Evidence:** ccgram — scip-python exits 0 with a 65-byte empty index; `tool_coverage` reports `scip: ok`; coupling_balance confidence stays low but appears measured. Users cannot distinguish SCIP working from SCIP silently failing.

**Fix:** After SCIP runs, check index file size (< 1 KB threshold) or document occurrence count; report `scip: warn (empty index — 0 occurrences)` instead of `scip: ok`.

**Effort:** Small.

### P7 — Fix jscpd Clusters parse failure for Rust repos

**Evidence:** herdr — jscpd finds 1,522 clone pairs (10.8%) standalone; `in.Duplication.Clusters` is empty inside archfit; `functional_candidates` = n/a. Silent failure with no user-visible error.

**Fix:** Add an integration test that verifies `Clusters` is non-empty after a jscpd run that reported duplicates. Diagnose whether this is a JSON format drift or a path normalization failure in the cluster parser.

**Effort:** Small (test first, then fix).

### P8 — Implement native archfit diff for scorecard comparison

**Evidence:** All 6 repos where version comparison was attempted required manual worktrees + two score runs + manual diff. macOS path placement affected results. omni's 57k-commit history made the workaround impractical; stage G was skipped entirely.

**Fix:** `archfit diff <ref>` subcommand that creates a clean temp worktree at the base ref, runs `score` on both trees with `filepath.EvalSymlinks` normalization, and emits a structured before/after metric delta table.

**Effort:** Large (new subcommand + worktree lifecycle management).

### P9 — Add "no baseline found" warning to delta mode

**Evidence:** All 7 repos — delta mode on first use shows all findings as `status: new` with no resolved/existing classification. Users read `new` as newly introduced regression rather than "not in baseline fingerprints."

**Fix:** When `--base <ref>` is given but no `.archfit-baseline.json` exists at that ref, emit: `no baseline found at <ref> — all N findings are untracked; run archfit baseline to enable drift detection`. Consider auto-seeding on first full run.

**Effort:** Small.

### P10 — Owner inheritance for synthetic cargo-modules submodules

**Evidence:** herdr — all 300 auto-registered submodules lack an owner field; every inter-submodule edge defaults to `cross_module_different_owner` (D=10, maximum distance). All 364 DM advisories show max distance for a single-team project, overstating severity.

**Fix:** In `classify.AugmentModulesFromGraph`, propagate `owner` from the nearest config-declared ancestor module to auto-registered submodules.

**Effort:** Small-medium.

### P11 — Emit a finding when observed topological layer diverges from module role expectation

**Evidence:** yazi — `yazi-config` is correctly placed at topological layer 11 by config alignment (depends on yazi-widgets → yazi-dds), far from the expected layer 2 for a configuration module. This is the architect's headline finding; archfit has the layer ranks but emits no finding. The LLM review missed it.

**Fix:** For each module, compute its observed topological rank from the import DAG and compare to the expected rank implied by its declared role (config/generic → low rank, core → mid rank, entry-point → high rank). Emit a warning when the delta exceeds a threshold (e.g., 3 layers).

**Effort:** Medium.

### P12 — tools.syntax.enabled default-on or explicit skipped status

**Evidence:** All seven evaluated repos had `panic_density`, `test_density`, `struct_field_density` silently absent. `ast-grep: ok` in tool_coverage does not distinguish presence from execution. `files_seen=0` is the only tell and it is not documented.

**Fix (option A):** Default `tools.syntax.enabled: true` and add `tools.syntax.enabled: false` opt-out.

**Fix (option B):** In tool_coverage, report `syntax-pass: skipped (tools.syntax.enabled absent)` when the pass was not run, distinguishing it from `ast-grep: ok` (binary present).

**Effort:** Small.

---

## Summary Table

| Finding class                          | Repos affected       | Highest-impact item                                                        |
| -------------------------------------- | -------------------- | -------------------------------------------------------------------------- |
| Distance inflation (flat module names) | spotinfo, codegraph  | coupling_balance inflated by 10-25 pts; LLM hallucination cascade          |
| macOS path normalization               | herdr, ccgram, omni  | structural_weight/hidden_coupling wrong; SCIP phantom; delta phantom green |
| Two severity models                    | codegraph, ccgram    | Gate says critical, book formula says high for symmetric edges             |
| explain --root missing                 | omni (all monorepos) | explain analyzed wrong service; LLM narrative for wrong codebase           |
| jscpd includes test/generated files    | pumba                | 25-pt cohesion drop from noise; 208 false panic_density                    |
| SCIP empty index reported ok           | ccgram               | coupling_balance phantom 60; true value ~40                                |
| tools off by default                   | all 7                | Half the metrics missing from baseline configs                             |
| No native version diff                 | all 7                | Manual worktree workaround brittle; omni stage G skipped                   |

---

## Appendix — Independent Verification (orchestrator, post-synthesis)

These findings evaluate archfit itself, so the synthesis was re-checked against source
code and raw JSON before acceptance. Confirmation bias is the risk here; the architect
anchors were also Claude subagents (shared-model bias), so the **independent** ground
truth is the source code plus the Balanced Coupling model — not "the architect agreed."

| Finding                                                                                                       | Checked against                                                                                                                            | Verdict                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **P4** explain `--root`                                                                                       | `cmd/archfit/explain.go:18-23` (no `Root` field), `:41` (`runPipeline(..., c.Config, "", ...)`)                                            | **Confirmed bug.** Trivial fix.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| **P1** flat-name distance                                                                                     | `internal/classify/distance_structure.go:45-47` returns `DiffOwner` for two single-segment names, with a comment asserting it is "correct" | **Confirmed — and it is a BC-correctness error, not a heuristic preference.** Two modules with the same owner + same deploy unit are at most `cross_module_same_owner`; labeling them `cross_module_different_owner` is false and turns high cohesion (`strength XOR distance`) into false "tight coupling." Inflates severity on every flat-named single-team Go/TS repo. Fix line 46: return `DistanceCrossModuleSameOwner` for the single-segment pair (the degenerate-owner path already guarantees one owner). |
| **P2** macOS case path                                                                                        | On disk `/Users/alexei/Workspace` (capital W); `git rev-parse` returns capital-W; eval `--root` passed lowercase `workspace`               | **Trigger real on this host.** herdr `hidden_coupling=0` and `structural_weight` band=`n/a` confirmed in raw JSON. Nuance: induced by non-canonical path **case** in `--root`; `cd ~/workspace` (lowercase) hits it, the canonical case does not. archfit should `EvalSymlinks`-normalize both sides so metrics are path-case-invariant.                                                                                                                                                                            |
| **P3** two severity models                                                                                    | `classify.go:326` `cl.Severity = coupling.BalanceResult(cl)` (legacy) coexists with BookScorer `score_band`                                | **Two code paths confirmed.** The S=9 → critical-vs-high numeric example is the agent's worked case (ordinals not re-derived here); the dual-source-of-truth smell is real.                                                                                                                                                                                                                                                                                                                                         |
| Metric values (`hidden_coupling=0`, `structural_weight=n/a`, `panic_density=208`, `functional_candidates=13`) | Parsed directly from raw `*-check-full.json`                                                                                               | **Faithful to data** — synthesis invented no numbers. archfit _did_ set `confidence=low` on the bad herdr metrics, so they are partly hedged, not presented as fully measured.                                                                                                                                                                                                                                                                                                                                      |

**Magnitude caveat:** "coupling_balance 40 → ~60-65" and architect "overall" scores are model
**estimates** (agents could not recompile archfit). Treat directions as solid, exact deltas as
approximate; re-confirm each finding when its fix is implemented.

**Housekeeping:** eval temp config removed from the omni service dir; no git worktrees left
behind; the six on-disk `.archfit.yaml` files were aligned in place (all untracked — review
diffs before committing).

---

## Fixes → findings cross-reference

Mapping every P-item and in-text bug from this report to its implementation task
and commit on the `main` branch. Verdicts from `reports/eval-2026-06-28/REVALIDATION.md`.

| Finding | Description | Task | Commit | Verdict |
| ------- | ----------- | ---- | ------ | ------- |
| **P1** | Flat single-segment distance → `CrossModuleSameOwner` floor when `isDegenerateOwnerMap` fires | 3 | `8786071` | **PASS** — codegraph `coupling_balance` 40→66 (+26 pts) |
| **P2** | macOS APFS case-path normalization via `filepath.EvalSymlinks` | 2 | `00fc1af` | **FAIL** — `EvalSymlinks` non-functional on APFS case-insensitive FS; herdr `hidden_coupling` unchanged at 0 with lowercase `--root`. Deferred to Task 25. |
| **P3** | Retire `BalanceResult` as severity source; derive severity from `BookScorer.Score().Band` | 4 | `637039c` | **PASS** — codegraph 8 critical → 0 critical; `explain` and `check` severities agree |
| **P4** | Add `--root` to `explain` command (fix hardcoded `root=""`) | 1 | `fca5acd` | **PASS (indirect)** — mechanism verified on omni monorepo fixture |
| **P5** | Exclude generated/mock/test files from jscpd clone scope | 7 | `a30f18a` | **PASS** — pumba clone pairs 13→3 (−77%); codegraph 86→65 (−24%) |
| **P6** | Flag SCIP empty index as `warn`/`partial`, not `ok` | 10 | `68ca542` | **PASS** — ccgram `scip: ok` → `scip: partial` with reason string |
| **P7** | Fix jscpd clusters parse failure for Rust repos | 11 | `439bd2d` | **PASS** — shipped via Task 12 tests; see Task 11 commit |
| **P8** | Implement native `archfit diff <ref>` scorecard comparison | 17+18 | `7a7cf42` + `d738da7` | **PASS** — `archfit diff` produces dimension delta table on herdr and codegraph |
| **P9** | Add "no baseline found" warning to delta mode | 16 | `05e46c2` | Shipped — Task 16 tests verify the warning; no controlled revalidation repo |
| **P10** | Owner inheritance for synthetic cargo-modules submodules | 15 | `822dab9` | **PASS** — herdr `cross_module_same_owner` 0→240; `cross_module_different_owner` correctly reclassified |
| **P11** | Emit finding when observed topological layer diverges from declared role (`layer_role_divergence` rule) | 17 | `7a7cf42` | Shipped — rule added and tested; no controlled revalidation repo |
| **P12** | Explicit `syntax-pass: skipped` coverage row when `tools.syntax.enabled` absent | 12 | `1bae6f1` | Shipped — Task 12 tests verify skipped row presence; no controlled revalidation repo |
| **In-text** `panic_density=208` (pumba mock panics) | `panic_density` excludes `Generated`/`Mock` via `FileClass`; reports excluded count | 5+6 | `c785dae` + `112e3fc` | **PASS** — pumba production panic count 208→38 (170 mocks excluded) |
| **In-text** `functional_candidates=13` (pumba test/gen pairs) | Included above under P5 | 7 | `a30f18a` | **PASS** |
| **In-text** `inbound_module_fanin=3` (spotinfo SCIP test refs) | `inbound_module_fanin` drops test packages from SCIP refs | 9 | `0087de1` | **PASS** — spotinfo fanin 3→2 |
| **In-text** `file_extraction_coverage=1.02` (codegraph) | Cap `file_extraction_coverage` at 1.0 | 13 | `7f55fd9` | **PASS** — codegraph 1.02→1.00; omni 1.04→1.00 |
| **In-text** `structural_weight=n/a` (herdr) | `structural_weight` excludes `Generated` files; path-case fix (P2) needed for full recovery | 8 | `16b01ae` | Partial — Generated exclusion shipped; path-case root cause deferred (P2/Task 25) |
| **In-text** LLM org-distance hallucination + strength-claim fabrication | LLM fidelity: distance basis grounding + strength-claim verification pass | 14 | `09513ab` | Shipped — verified via `review` output on ideal configs |
