# archfit vs architect-skill deep comparison

Date: 2026-06-20  
Run folder: `/Users/alexei/Workspace/archfit/reports/archfit-vs-architect-20260620 (openai)`

Provider note: this folder name identifies this Pi/OpenAI-agent run. The `archfit review` LLM leg used the available `ANTHROPIC_API_KEY` with `anthropic/claude-opus-4-8`.

## 1. Executive summary

The comparison is complete after fixing `archfit review`.

`archfit` and the architect skill are complementary, not substitutes.

- `archfit` is best as the **repeatable measurement and gate layer**: dependency facts, cycles, scorecards, hidden-coupling/clone/hub counts, SARIF/JSON, repeat hashes, and CI/agent feedback.
- The architect skill is best as the **interpretation and design layer**: intent reconstruction, domain volatility, composition-root judgment, runtime/deploy context, false-positive triage, and executable fitness-check design.
- The fixed `archfit review` LLM path now runs on all five repos, including `ccgram` and `codegraph`. It should still be treated as advisory because it narrates deterministic evidence; it does not prove new facts.

Highest-value combined workflow:

```text
1. archfit deterministic all-tools scan
2. architect-skill quick/full review validates intent and context
3. architect emits calibration: confirmed, false-positive, severity-adjusted, missed-by-archfit
4. feed calibration into .archfit.yaml, .archfit-labels.yaml, and fitness tests
5. archfit gates the improved architecture in CI
6. architect re-reviews only deltas or unresolved design risks
```

## 2. What was run

Targets:

- `/Users/alexei/Workspace/pumba`
- `/Users/alexei/Workspace/ccgram`
- `/Users/alexei/Workspace/codegraph`
- `/Users/alexei/Workspace/archfit`
- `/Users/alexei/Workspace/spotinfo`

Baselines:

| repo      | baseline |
| --------- | -------- |
| pumba     | `1.1.6`  |
| ccgram    | `v3.5.1` |
| codegraph | `v0.9.8` |
| archfit   | `v0.4.0` |
| spotinfo  | `v2.3.0` |

Per repo artifacts:

- `archfit-alltools-llm.yaml` — copied all-tools config used for evidence.
- `full.json` — deterministic all-tools full scan.
- `full-repeat.json` — deterministic repeat scan.
- `scorecard.md` — all-tools scorecard.
- `scan.md` — all-tools markdown report.
- `delta.json` — all-tools delta against baseline.
- `delta-scorecard.md` — all-tools delta scorecard.
- `llm-review.md` — fixed `archfit review --no-cache` output.
- `llm-review-before-fix.md` — original review output or parse error before the fix.
- `architect-sweep.md` — architect-skill quick sweep.

Run-level artifacts:

- `comparison-data.json`
- `alltools-rerun-summary.json`
- `llm-review-after-fix-summary.json`
- `deterministic-repeatability.json` from the earlier run
- this `final-comparison-report.md`

## 3. Balanced Coupling basis

External sources consulted:

- Vlad Khononov books page: `https://vladikk.com/page/books/`
- Pearson: `Balancing Coupling in Software Design`, Addison-Wesley/Pearson, 2024/2025.
- InfoQ podcast: `Balancing Coupling in Software Design with Vlad Khononov`.
- SE Radio 662: `Vlad Khononov on Balancing Coupling in Software Design`.

Rubric used:

- Coupling is necessary. The review question is whether it is **balanced**.
- Evaluate relationships, not components in isolation.
- Three axes:
  - **Integration strength**: intrusive > functional > model > contract.
  - **Distance**: code, ownership, runtime, deploy, and abstraction distance.
  - **Volatility**: domain volatility first; churn is supporting evidence.
- Healthy patterns:
  - high strength + low distance = cohesion;
  - low strength + high distance = loose coupling.
- Main risk:
  - high strength + high distance + high volatility = expensive cascading change / distributed-monolith pressure.
- Fix by changing one axis:
  - lower strength with a contract;
  - lower distance by co-locating cohesive knowledge;
  - lower volatility by stabilizing a shared surface;
  - or accept coupling when volatility is low.

## 4. `archfit review` bug fix

### Symptom

Before the fix:

- `ccgram/llm-review.md`: `error: review: model response is not the required JSON: unexpected end of JSON input`
- `codegraph/llm-review.md`: same parse failure
- `--no-cache` repeated the failure.

### Root cause

`archfit review` sent large evidence prompts to the LLM but capped response size at `MaxTokens: 2048`. On larger repos the model began returning the required JSON object, then the response was truncated. `json.Unmarshal` saw incomplete JSON and returned `unexpected end of JSON input`.

Contributing factors:

- The prompt listed all findings and all module facts.
- The system prompt did not cap count of top risks or subdomain suggestions.
- The parser only stripped basic markdown fences and gave no truncation hint.

### Code changes

Changed files:

- `cmd/archfit/review.go`
- `cmd/archfit/review_test.go`

Evidence:

- `cmd/archfit/review.go:23-30` — added review-specific caps and raised response budget to `8192`.
- `cmd/archfit/review.go:78-83` — `review` now uses `reviewMaxTokens`.
- `cmd/archfit/review.go:98-126` — added robust JSON payload extraction and truncation diagnostics.
- `cmd/archfit/review.go:380-524` — grouped and capped finding examples, module facts, metrics, and dynamic import evidence.
- `cmd/archfit/review_test.go:178-210` — added tests for fenced/prose-wrapped JSON, truncated JSON hint, and token budget.

### Verification

- `go test ./cmd/archfit -run "TestReviewCmd|TestParseReviewResponse|TestBuildReviewPrompt|TestPostVerify"` — pass.
- `go test ./cmd/archfit ./internal/llm ./internal/engine ./internal/score` — pass.
- `make lint` — `0 issues`.
- `make test` — pass.
- `make all` — `rc=0`; lint, race tests, coverage, and build passed.
- Patched all-tools LLM rerun: all 5 repos returned `rc=0`.

## 5. Run result summary

| repo      |  archfit score | verdict | findings | gate | warnings | delta findings | LLM review | repeat hash |
| --------- | -------------: | ------- | -------: | ---: | -------: | -------------: | ---------- | ----------- |
| pumba     |       51 mixed | pass    |        2 |    0 |        2 |              2 | ok         | same        |
| ccgram    |       43 mixed | fail    |       61 |    3 |       58 |             61 | ok         | same        |
| codegraph |       50 mixed | pass    |        0 |    0 |        0 |              0 | ok         | same        |
| archfit   | 64 serviceable | pass    |        0 |    0 |        0 |              0 | ok         | same        |
| spotinfo  | 69 serviceable | pass    |        0 |    0 |        0 |              0 | ok         | same        |

Tool coverage from `full.json`:

| repo      | static graph | language extractor         | complexity | clones   | history/impact | ast-grep |
| --------- | ------------ | -------------------------- | ---------- | -------- | -------------- | -------- |
| pumba     | scip ok      | go/packages ok             | lizard ok  | jscpd ok | gitnexus ok    | ok       |
| ccgram    | scip ok      | grimp ok                   | lizard ok  | jscpd ok | gitnexus ok    | ok       |
| codegraph | scip ok      | dependency-cruiser partial | lizard ok  | jscpd ok | gitnexus ok    | ok       |
| archfit   | scip ok      | go/packages ok             | lizard ok  | jscpd ok | gitnexus ok    | ok       |
| spotinfo  | scip ok      | go/packages ok             | lizard ok  | jscpd ok | gitnexus ok    | ok       |

Coverage gaps that remain:

- `dependency-cruiser` is absent for non-TS repos and partial for `codegraph`.
- `grimp` applies only to Python and is absent elsewhere.
- Config metadata is under-specified in all repos: owner/subdomain/volatility gaps reduce distance and volatility confidence.

## 6. Cross-cutting findings

### Finding A — deterministic archfit is reliable when inputs are controlled

Evidence:

- `alltools-rerun-summary.json`: `repeat.same_hash=true` for all five repos.
- Earlier archfit self-run differed only because report artifacts were written inside the repo being analyzed. The all-tools rerun avoided that and became stable.

Interpretation:

- `archfit check`/`scan`/`scorecard` are suitable for CI and trend tracking.
- Reproducibility depends on excluding generated report/cache/tool state from the analyzed root.

Recommendation:

- Add built-in ignore defaults or warnings for `.archfit-cache`, `.codegraph`, `.gitnexus`, `reports/`, `.venv`, `node_modules`, tool caches, and generated report directories.
- Warn when output path is inside target root.

### Finding B — config quality controls measurement quality

Evidence:

- Every LLM run emitted config-quality warnings for missing owner/subdomain/volatility.
- Scorecards show low boundary confidence for multiple repos despite complete file extraction.

Interpretation:

- `archfit` can measure structure, but Balanced Coupling needs volatility and distance context.
- Missing metadata causes either low confidence or advisory floods.

Recommendation:

- Make config-quality a first-class score/gate.
- Do not report high confidence for boundary/coupling dimensions when owner/subdomain/volatility are missing.
- Add an `archfit config doctor` or `archfit update --review-metadata` mode that asks for owner, deploy unit, subdomain, volatility, and composition-root roles.

### Finding C — LLM narration is useful after the fix, but should remain advisory

Evidence:

- Before fix: `ccgram` and `codegraph` failed JSON parsing.
- After fix: all five `llm-review.md` files are valid narrative reports.
- LLM output still depends on deterministic evidence and post-verification.

Interpretation:

- The LLM can prioritize and explain, but it should not create gate facts.
- The code fix makes LLM review usable for larger repos, but the deterministic scan remains the source of truth.

Recommendation:

- Keep `review` off-gate.
- Add raw response persistence on parse failure.
- Prefer provider-native schema-constrained APIs where available.
- Add token/prompt telemetry to `review` output or stderr.

### Finding D — architect skill catches intent and runtime context that archfit does not

Evidence from `architect-sweep.md` reports:

- `pumba`: `cmd` is a composition root; archfit's `cmd -> pkg_*` medium advisories are likely overstated without ownership/deploy distance.
- `ccgram`: static cycles coexist with intentional lazy-import policy and passing runtime import tests; severity needs repo intent.
- `codegraph`: functional coupling exists across agent instructions, tool descriptions, outputs, docs, and tests without import edges.
- `spotinfo`: long `internal/mcp` test runtime is a feedback-loop reliability risk; archfit does not measure it.

Interpretation:

- Human/architect review is needed to classify whether coupling is cohesive, accidental, runtime-dangerous, or acceptable.

Recommendation:

- Architect reports should produce a calibration artifact consumed by archfit:

```yaml
archfit_calibration:
  confirmed: []
  false_positive_or_noise: []
  severity_adjusted: []
  missed_by_archfit: []
  config_changes: []
  new_fitness_checks: []
```

### Finding E — architect skill misses exhaustive counts unless fed archfit artifacts

Evidence:

- Architect quick sweeps did not mechanically enumerate all clone pairs, hidden-coupling pairs, hubs, propagation cost, or all tool coverage.
- `full.json` and `scan.md` provide these consistently.

Interpretation:

- Architect review should not start cold on large repos.

Recommendation:

- Make archfit artifacts mandatory input for architect quick/full reviews.
- Architect should review deltas and risk interpretation, not re-discover every low-level metric manually.

## 7. Per-repo findings and recommendations

### 7.1 pumba

Evidence:

- `pumba/scorecard.md`: overall `51/100 mixed`.
- `pumba/full.json`: 2 medium BC advisory findings; no gate failures.
- `pumba/llm-review.md`: risk is unenforced intent, hidden coupling around hubs, instability concentration.
- `pumba/architect-sweep.md`: baseline delta from `1.1.6` is test-only; `cmd` is composition root; Kubernetes lint found deploy-manifest concerns.

Findings:

1. **Architecture fitness is missing.** Score `0/100`; no executable boundary checks.
2. **`cmd` advisories need severity calibration.** In a one-binary Go CLI, `cmd` fan-out is often composition-root cohesion, not high-distance coupling.
3. **Hidden/tool noise exists.** `.gitnexus/run.cjs` appeared as complexity in earlier scan evidence; not Pumba source architecture.
4. **Deploy checks are outside archfit's source graph.** Architect found k8s lint issues that archfit did not measure.

Recommendations:

- Add Go architecture tests for intended package direction.
- Mark `cmd` as `role: composition_root` or equivalent in archfit config once supported.
- Fill module metadata: owner, subdomain, volatility, deploy unit.
- Exclude `.gitnexus`, `.codegraph`, `.archfit-cache`, reports, and tool state from metrics.
- Add deploy-manifest fitness checks separately: `kube-linter` or equivalent.

### 7.2 ccgram

Evidence:

- `ccgram/scorecard.md`: overall `43/100 mixed`.
- `ccgram/full.json`: 61 findings; 3 import-cycle gate findings; 58 BC advisories.
- `ccgram/llm-review.md`: import cycles, handler hub fan-out, 115 blast-radius hubs, 73 unstable modules.
- `ccgram/architect-sweep.md`: lazy imports are intentional and tested; runtime import tests pass; delta increased hook/provider/session-map coupling.

Findings:

1. **Static dependency health is the top risk.** Import cycles are real architectural friction even when runtime lazy imports prevent immediate crashes.
2. **Handler/provider/session-state areas are high-volatility core seams.** Archfit flags volume; architect explains why those modules change together.
3. **Config under-specification inflates advisory noise.** Many BC advisories are `declare_volatility` style because the config lacks owner/subdomain/volatility.
4. **Architecture fitness exists but is incomplete.** Score `67/100`; checks exist, but cycles still pass as warnings or are not gated hard enough.

Recommendations:

- Convert intentional lazy-import policy into explicit allowed-cycle metadata or tests, then fail all unapproved cycles.
- Add no-cycle fitness checks over Python imports with a small, reviewed allowlist.
- Split handler hub access through ports/contracts for TTS, Whisper, provider commands, transcript, and session/window state.
- Classify domain volatility:
  - `handlers`, `session_state`, `window_state`: core/high volatility.
  - `providers`, `telegram_adapter`, `tmux_adapter`: supporting/medium.
  - optional TTS/Whisper/MiniApp: supporting or generic depending on product priority.
- Promote reviewed `.archfit.yaml` into CI only after reducing advisory noise.

### 7.3 codegraph

Evidence:

- `codegraph/scorecard.md`: overall `50/100 mixed`.
- `codegraph/full.json`: 0 findings, but graph health `43`, cohesion `5`, architecture fitness `33`.
- `codegraph/llm-review.md`: 2 import cycles, 11 god modules, 22 hidden-coupling pairs, weak fitness.
- `codegraph/architect-sweep.md`: `madge` found concrete file cycles; dependency-cruiser found circular edges; delta is large and core-heavy; config has one broad `core` layer.

Findings:

1. **Verdict `pass` is misleading.** Gate rules are too weak/broad to reflect the measured structural risk.
2. **Cohesion is the worst dimension.** God modules and hidden coupling indicate knowledge boundaries are not explicit enough.
3. **Functional coupling is partly outside import edges.** Agent instructions, tool schemas, output strings, docs, and tests co-evolve.
4. **Fitness is too weak for a graph tool.** A code graph project should gate cycles, core/adapters, output contracts, and prompt/tool-description drift.

Recommendations:

- Replace one broad `core` layer with explicit modules and directions:
  - CLI/MCP/installer depend inward;
  - extraction/resolution/db/search/core API have allowed directions;
  - MCP should not create cycles back through root exports.
- Add dependency-cruiser or madge cycle gate in CI.
- Split extraction/resolution hot files behind contracts; do not scatter language-specific behavior.
- Add single-source checks for agent guidance strings and MCP tool descriptions.
- Add archfit labels for functional coupling not visible as imports.

### 7.4 archfit

Evidence:

- `archfit/scorecard.md`: overall `64/100 serviceable`.
- `archfit/full.json`: 0 findings, repeat stable after output contamination was controlled.
- `archfit/llm-review.md`: hidden coupling/god-module risk, hub concentration, architecture fitness partially present.
- `archfit/architect-sweep.md`: stale score command issue, adapter map drift, incomplete fitness coverage, self-analysis artifact noise.
- Code fix evidence: `cmd/archfit/review.go`, `cmd/archfit/review_test.go`.

Findings:

1. **LLM review reliability bug is fixed.** `ccgram` and `codegraph` now run successfully.
2. **Self-analysis hygiene is still fragile.** Reports written inside the repo can change metrics unless excluded.
3. **Cohesion/modularity remains the weakest score.** Hidden coupling and large modules deserve design follow-up.
4. **Docs/tooling can drift.** Invalid `archfit score --full` usage was found during this run and corrected by using `check --format scorecard`.

Recommendations:

- Add report/cache/tool-state default exclusions.
- Add a command or flag to analyze a root while reading config from another path. This would avoid temporary in-root configs for analysis harnesses.
- Persist raw LLM responses on parse failures.
- Add golden tests for documented command examples.
- Add a self-dogfood architecture check that forbids source metrics from including `reports/`, `.gitnexus/`, `.codegraph/`, and `.archfit-cache/`.

### 7.5 spotinfo

Evidence:

- `spotinfo/scorecard.md`: overall `69/100 serviceable`.
- `spotinfo/full.json`: 0 findings.
- `spotinfo/llm-review.md`: healthy small graph; missing fitness checks; `internal/spot` and `internal/mcp` hubs.
- `spotinfo/architect-sweep.md`: internal dependency direction is clean; delta is README + gosec suppressions only; `go test ./...` passed but `internal/mcp` took ~92s.

Findings:

1. **Architecture is simple and mostly healthy.** `cmd/spotinfo` and `internal/mcp` depend inward on `internal/spot`; core has no internal imports.
2. **Architecture fitness is absent.** Score `0/100` despite normal CI/tests.
3. **Slow MCP tests affect feedback reliability.** Archfit does not measure this.
4. **Config metadata is under-specified.** Boundary confidence is lower than the structure deserves.

Recommendations:

- Add a Go import-direction test: `internal/spot` must not import `internal/mcp` or `cmd/spotinfo`; `internal/mcp` may import `internal/spot` only.
- Add `.archfit.yaml` owner/subdomain/volatility metadata.
- Split slow MCP/integration tests from fast architecture checks.
- Keep AWS SDK types behind provider interfaces and test with fakes.

## 8. How to improve archfit

Priority order:

1. **Config-quality gate**
   - Treat missing owner/subdomain/volatility/deploy unit as explicit confidence loss.
   - Optionally fail CI when config is generated/unreviewed.

2. **Role-aware modules**
   - Add `role: composition_root | adapter | core | shared_model | generated | test`.
   - Use role to avoid over-penalizing `cmd` fan-out and test helper coupling.

3. **Artifact and cache hygiene**
   - Built-in ignore patterns for `.archfit-cache`, `.gitnexus`, `.codegraph`, `reports`, `coverage`, package caches, and generated outputs.
   - Warn when reports are written under the analyzed root.

4. **Delta bucketing**
   - Split delta output into `new`, `existing`, `resolved`, `severity_changed`, and `touched_by_delta`.
   - Current delta can look like full-current output for some repos.

5. **LLM review hardening**
   - Done: larger token budget, bounded evidence prompt, JSON extraction, truncation hint.
   - Next: raw response persistence, provider-native JSON schema, prompt/token telemetry.

6. **Functional coupling beyond imports**
   - Detect co-evolving strings and generated contracts across docs, tool descriptions, tests, prompt templates, snapshots, and examples.

7. **Operational/toolchain evidence**
   - Optional checks for CI actually running architecture gates, Kubernetes/Helm/Terraform drift, Docker topology, IAM/security posture, and slow/flaky feedback loops.

8. **External config path support**
   - Add `--root` or separate `--config` and `--root` semantics. The harness had to create temporary in-repo configs because current code treats config directory as repo root.

## 9. How to improve architect skill

1. **Always consume archfit artifacts first**
   - Required inputs: `full.json`, `scorecard.md`, `scan.md`, `delta.json`, `tool_coverage`.

2. **Emit a structured calibration file**
   - The output should be machine-readable enough to update archfit labels/config.

3. **Separate facts, hypotheses, and judgments**
   - Fact: import cycle exists.
   - Judgment: cycle is runtime-dangerous, acceptable lazy-import policy, or boundary friction.
   - Action: break, allowlist, or document.

4. **Make tool coverage explicit**
   - Every review should list tools used/missing/failed and confidence impact.

5. **Avoid final scores in quick sweeps**
   - Quick sweeps can compare and triage; full scores require the full architecture-review gates.

6. **Use stable finding IDs**
   - Needed for report-to-report delta and plan handoff.

## 10. Combined workflow for maximum value

### Step 1 — collect deterministic evidence

```sh
archfit check -c .archfit.yaml --full --advisory --report --format json > full.json
archfit check -c .archfit.yaml --full --advisory --report --format scorecard > scorecard.md
archfit scan -c .archfit.yaml > scan.md
archfit check -c .archfit.yaml --base <previous-tag> --advisory --report --format json > delta.json
```

### Step 2 — run LLM narrative after deterministic scan

```sh
archfit review -c .archfit.yaml --no-cache > llm-review.md
```

Use this for prioritization only.

### Step 3 — architect calibrates

Architect reads:

- repo docs/ADRs/manifests;
- `full.json`, `scorecard.md`, `scan.md`, `delta.json`, `llm-review.md`;
- dependency/language/deploy checks where needed.

Architect outputs:

```yaml
confirmed_findings: []
false_positives_or_noise: []
severity_adjustments: []
missed_risks: []
config_patches: []
fitness_checks: []
```

### Step 4 — feed calibration back into code/config

- Update `.archfit.yaml` module metadata.
- Add `.archfit-labels.yaml` for reviewed coupling labels.
- Add architecture tests and CI gates.
- Add ignore rules for generated/tool artifacts.

### Step 5 — re-run archfit as gate

The accepted target state should be enforced by deterministic checks, not just docs.

## 11. Immediate next actions

### For archfit repo

1. Keep the LLM review fix.
2. Add raw response persistence and schema-constrained provider support.
3. Add artifact/cache default exclusions.
4. Add `--root` support so external report harnesses do not need temporary in-root configs.
5. Add delta bucketing.

### For this comparison report

No further evidence collection is required for the current request. The missing LLM evidence was fixed and rerun.

### For target repos

1. Review and commit proper `.archfit.yaml` metadata only after owner/subdomain/volatility are filled.
2. Add repo-specific fitness checks listed in section 7.
3. Re-run the combined workflow after config calibration.
