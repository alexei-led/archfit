# Plan: Archfit Configuration Confidence

## Goal

Fix known correctness gaps before adding new behavior.

Then add two narrow features for coding agents:

1. Show which current repair tasks are new relative to a Git ref.
2. Compare two configuration files on the same source tree.

Keep the current command model. Extend `config update`. Add only `config compare`.

## User Flow

```text
archfit config update
  -> report structure changes, required decisions, and review suggestions

edit a candidate configuration

archfit config compare candidate.archfit.yaml
  -> compare two measurements on the same source tree

archfit check --base main --json
  -> show current repair tasks and their Git origin
```

## Scope

- Repair invalid CLI commands in active scripts, examples, tests, and guides.
- Remove the ineffective `baseline --base` flag.
- Add rubric compatibility to the baseline score snapshot.
- Keep `--no-advisories` effective for every output combination.
- Make `config update` clear and machine-readable.
- Add a Git finding delta to JSON output when `--base` is set.
- Add the report-only `config compare` command.
- Keep code, tests, help, examples, and guides aligned in each task.

## Out of Scope

- Do not add `config health`, `config suggest`, or a general `simulate` command.
- Do not add `--agent-tasks`. The `agent_tasks[]` field already exists.
- Do not add `architecture_actions[]`, automatic patches, split actions, or merge actions.
- Do not add a state file, architecture hash, history, or convergence status.
- Do not add a requirement graph.
- Do not change the coupling formula, ordinals, or gate rules.
- Do not add an LLM path to deterministic analysis.
- Do not optimize fact reuse for `config compare` in this plan.

## Command Contract

Keep these command forms:

```sh
archfit config update -c .archfit.yaml
archfit config update --json -c .archfit.yaml
archfit config compare candidate.archfit.yaml -c .archfit.yaml
archfit config compare candidate.archfit.yaml --json -c .archfit.yaml
archfit check --base main --json -c .archfit.yaml
```

Remove this ineffective form:

```sh
archfit baseline --base main
```

## Guardrails

- `.archfit.yaml` remains the source of architecture intent.
- `.archfit-baseline.json` remains the source of accepted findings for normal analysis.
- `config compare` does not use accepted findings. It compares raw measurements.
- A higher score does not mean that a candidate configuration is better.
- Reduced measurement always produces a warning.
- Missing evidence produces `unknown` or `not_comparable`, not a false success.
- New report data does not change the verdict or exit code.
- JSON lists use a stable order.
- Report-only commands keep configuration, baseline, labels, candidate, and policy files byte-identical.
- Normal content-addressed fact-cache reads and writes remain permitted.

## Validation Rule

Run the narrow checks in each task first. Then run:

```sh
make fmt && make test && make lint
make build
make archfit
```

Do not complete a task until all checks pass.

## Implementation Steps

### Task 1: Fix automation exit-code handling

**Reason:**

The benchmark hides parser failures. The evaluation script uses removed flags and mishandles warning results.

**Files:**

- Modify: `Makefile`
- Modify: `scripts/bench-gate.sh`
- Modify: `scripts/eval/gap-closure.sh`
- Create: `scripts/tests/cli_exit_contract_test.sh`

**Required behavior:**

- Both scripts call `archfit check` for a gate.
- The benchmark measures exit codes `0`, `1`, and `2`.
- The benchmark stops and returns exit code `3` for a parser, configuration, or tool error.
- The evaluation script keeps JSON and Markdown for exit codes `0`, `1`, and `2`.
- The evaluation script removes invalid JSON and Markdown after exit code `3`.
- The scripts accept a test-only binary-path override without changing their normal command.
- The shell regression test uses a fake Archfit binary and covers all four exit codes.

**Narrow validation:**

```sh
sh -n scripts/bench-gate.sh
bash -n scripts/eval/gap-closure.sh
bash scripts/tests/cli_exit_contract_test.sh
make bench-gate
```

- [x] Replace the removed gate command in both scripts and the Make help text.
- [x] Handle valid gate results `0`, `1`, and `2` explicitly.
- [x] Propagate exit code `3` and remove invalid evaluation output.
- [x] Add the narrow binary-path test seam.
- [x] Add a fake-binary regression test for all result codes and output retention.
- [x] Run the narrow checks and the full validation rule.

### Task 2: Align active CLI text and historical notices

**Reason:**

Active examples, comments, and test names still describe removed flags. Historical records must keep an explicit historical label.

**Files:**

- Modify: `examples/README.md`
- Modify: `examples/*.archfit.yaml`
- Modify: `CLAUDE.md`
- Modify: `docs/guide/concepts.md`
- Modify: `docs/design/bc-measurement-v4.md`
- Modify: `docs/design/fact-cache.md`
- Modify: `docs/spec/arch-fitness-spec-v0.4.md`
- Modify: `cmd/archfit/llmreview.go`
- Modify: `cmd/archfit/llmreview_test.go`
- Modify: `cmd/archfit/main_test.go`
- Modify: `cmd/archfit/root_test.go`
- Modify: `cmd/archfit/score_test.go`
- Modify: `cmd/archfit/couplinggate_test.go`
- Modify: `cmd/archfit/worktree.go`
- Modify: `cmd/archfit/pipeline_run.go`
- Modify: `cmd/archfit/enrich.go`
- Modify: `cmd/archfit/enrich_test.go`
- Modify: `cmd/archfit/autopilot_test.go`
- Modify: `internal/model/diagnostic/diagnostic.go`
- Modify: `internal/extract/manifest/manifest.go`
- Modify: `internal/agenttask/agenttask_test.go`
- Modify: `internal/engine/advisory_tasks_test.go`
- Modify: `internal/output/jsonout/jsonout_test.go`
- Modify: `internal/output/markdown/markdown_test.go`

**Required behavior:**

- Report examples use `archfit analyze`.
- Gate examples use `archfit check`.
- Scorecard examples use `--format scorecard`.
- Current AI text uses `--ai-summary` or `--ai-classify`.
- The v0.4 specification stays at its current path.
- Its first screen states that it is superseded and links to the current command and configuration guides.
- Current guides do not describe the v0.4 specification as the current build specification.

**Preserve these references:**

- Keep removed-flag rejection tests in `cmd/archfit/check_test.go`.
- Keep migration tables in `README.md`, `docs/guide/commands.md`, and `docs/guide/troubleshooting.md`.
- Keep release notes, the CLI redesign record, completed plans, and archived reports unchanged.

**Narrow validation:**

```sh
bash -c 'if out=$(rg -n "analyze --gate|analyze .*--full|analyze --score|config init --llm|config update --llm|analyze --llm|explain --llm" examples CLAUDE.md docs/guide/concepts.md docs/design/bc-measurement-v4.md docs/design/fact-cache.md cmd/archfit/llmreview.go cmd/archfit/llmreview_test.go cmd/archfit/main_test.go cmd/archfit/root_test.go cmd/archfit/score_test.go cmd/archfit/worktree.go cmd/archfit/pipeline_run.go cmd/archfit/enrich.go cmd/archfit/enrich_test.go cmd/archfit/autopilot_test.go internal/model/diagnostic/diagnostic.go internal/extract/manifest/manifest.go internal/agenttask/agenttask_test.go internal/engine/advisory_tasks_test.go internal/output/jsonout/jsonout_test.go internal/output/markdown/markdown_test.go); then printf "%s\n" "$out"; exit 1; else rc=$?; test "$rc" -eq 1; fi'
bash -c 'if out=$(rg -n -- "--gate|--advisory|--require-tools" cmd/archfit/worktree.go); then printf "%s\n" "$out"; exit 1; else rc=$?; test "$rc" -eq 1; fi'
go test ./cmd/archfit ./internal/agenttask ./internal/engine ./internal/output/jsonout ./internal/output/markdown
```

- [x] Replace removed commands only in the listed active surfaces.
- [x] Fix stale test names and comments without weakening negative CLI tests.
- [x] Fix the advisory-mode baseline test so it covers `--no-advisories`.
- [x] Add one clear superseded notice to the v0.4 specification.
- [x] Preserve migration help and historical records.
- [x] Run the narrow checks and the full validation rule.

### Task 3: Make baseline and score-format behavior truthful

**Reason:**

The `baseline --base` flag does not change the saved baseline. The saved score also lacks its rubric version.

**Files:**

- Modify: `cmd/archfit/baseline.go`
- Modify: `cmd/archfit/analyze.go`
- Modify: `cmd/archfit/check.go`
- Modify: `cmd/archfit/score_test.go`
- Modify: `cmd/archfit/couplinggate_test.go`
- Modify: `cmd/archfit/check_test.go`
- Modify: `internal/baseline/baseline.go`
- Modify: `internal/baseline/baseline_test.go`
- Modify: `internal/score/score.go`
- Modify: `internal/output/scorecard/scorecard.go`
- Modify: `CLAUDE.md`
- Modify: `docs/guide/commands.md`
- Modify: `docs/guide/configuration-reference.md`

**Required behavior:**

- `archfit baseline` records one full current baseline.
- `BaselineCmd` has no `Base` field.
- `archfit baseline --base main` returns an unknown-flag error with exit code `3`.
- The baseline score snapshot records the scorer version and rubric version.
- An incompatible snapshot does not anchor `coupling.gate.max_drop`.
- A missing rubric version means rubric version `1` while the current rubric remains version `1`.
- The stale warning names the incompatible score-snapshot input.
- `--no-advisories` remains effective when JSON and scorecard outputs are both requested.
- Removing advisories does not change the score.
- `--base` help says that the comparison adds a delta to normal output.

**Narrow validation:**

```sh
go test ./internal/baseline ./internal/score ./internal/output/scorecard ./cmd/archfit
make build
.bin/archfit baseline --help
.bin/archfit analyze --help
.bin/archfit check --help
```

- [x] Remove `--base` from the baseline command, help, current examples, and tests.
- [x] Add scorer and rubric compatibility to the baseline score snapshot.
- [x] Replace scorer-only stale names and text with score-snapshot terms.
- [x] Remove the obsolete scorecard override that enables advisories.
- [x] Fix the score comment that still requires advisory findings.
- [x] Add legacy, current, incompatible, removed-flag, and mixed-output tests.
- [x] Run the narrow checks and the full validation rule.

### Task 4: Make `config update` clear and machine-readable

**Reason:**

`config update` already owns discovery and configuration review. Its preview can also hide a Rust setting that `--apply` later writes.

**Files:**

- Modify: `internal/initcfg/update.go`
- Modify: `internal/initcfg/update_test.go`
- Modify: `internal/initcfg/update_report_test.go`
- Create: `internal/initcfg/review.go`
- Create: `internal/initcfg/review_test.go`
- Modify: `cmd/archfit/update.go`
- Modify: `cmd/archfit/update_test.go`
- Modify: `docs/guide/commands.md`
- Modify: `docs/guide/configuration.md`
- Modify: `docs/guide/troubleshooting.md`
- Modify: `README.md`

**JSON contract:**

```json
{
  "schema_version": "archfit.config-review.v1",
  "status": "action_required",
  "structure": {
    "added_modules": [],
    "removed_modules": [],
    "path_drift": [],
    "settings": [{ "code": "add_rust_deep_analysis_defaults", "reason": "..." }]
  },
  "issues": [
    {
      "code": "missing_owner",
      "module": "payments",
      "reason": "...",
      "next_action": "..."
    }
  ],
  "review_suggestions": {
    "deploy_units": [],
    "distance_config": []
  }
}
```

**Required behavior:**

- A module needs `subdomain` or `volatility`, not both fields.
- Use this condition: `(!HasSubdomain && !HasVolatility) || (RequireLayer && !HasLayer)`.
- `RequireLayer` is true only when a `forbidden_layer_direction` rule has `gate` other than `off`.
- The preview and JSON show the Rust deep-analysis setting before `--apply` writes it.
- The JSON status is `action_required`, `review_available`, or `no_known_issues`.
- `action_required` has priority over the other statuses.
- `review_available` applies only when suggestions exist and no action is required.
- Every structure and suggestion list is a non-null JSON array.
- Issues sort by module and then code. Existing structure and suggestion types keep their stable order.
- The text report shows a short status before its existing details.
- The report does not claim that the configuration is complete or healthy.
- `--json` with `--apply`, `--ai-classify`, or `--refresh` returns a usage error with exit code `3`.
- Validate these conflicts before discovery, tool calls, cache access, or writes.

**First-version issue codes:**

- `missing_owner`
- `missing_volatility_input`
- `missing_layer`

**Narrow validation:**

```sh
go test ./internal/initcfg ./cmd/archfit
make build
.bin/archfit config update -c .archfit.yaml
bash -o pipefail -c '.bin/archfit config update --json -c .archfit.yaml | jq -e .'
```

- [x] Fix module classification and active-layer-policy logic.
- [x] Move the hidden Rust setting into the typed preview model.
- [x] Build one deterministic review model from discovery, lint, and suggestion data.
- [x] Render the short text status and the versioned JSON document.
- [x] Reject incompatible JSON flags before side effects.
- [x] Add status, order, issue, Rust-preview, apply-parity, and no-write tests.
- [x] Update the guides and run the narrow and full validation commands.

### Task 5: Give the pipeline one explicit run context

**Reason:**

The pipeline uses the configuration path for several unrelated paths. Comparison runs also need one shared evaluation time.

**Files:**

- Modify: `cmd/archfit/pipeline_run.go`
- Modify: `cmd/archfit/pipeline_run_test.go`
- Modify: `cmd/archfit/byteidentical_test.go`
- Modify: `cmd/archfit/analyze.go`
- Modify: `cmd/archfit/baseline.go`
- Modify: `cmd/archfit/enrich.go`
- Modify: `cmd/archfit/enrich_abstained.go`
- Modify: `cmd/archfit/explain.go`
- Modify: `cmd/archfit/worktree.go`
- Modify: `cmd/archfit/llmreview_test.go`

**Run-context contract:**

- `ConfigSource` selects the configuration hash and validation command.
- `BundleDir` selects labels and fact-cache locations.
- `ScanRoot` selects scope and on-disk path resolution.
- `EvaluatedAt` supplies one time to waiver and staleness logic.
- The accepted baseline remains a separate caller-supplied value.
- Replace the positional path arguments with one typed run context.
- Update every caller. Do not keep a second wrapper API.
- A zero `EvaluatedAt` value samples the current time once for a normal single run.

**Required value table:**

| Run               | Config source  | Bundle directory         | Scan root     | Accepted baseline |
| ----------------- | -------------- | ------------------------ | ------------- | ----------------- |
| Normal analysis   | current path   | current config directory | current tree  | persisted         |
| Git base          | current path   | current config directory | base worktree | empty             |
| Compare current   | current path   | current config directory | common tree   | empty             |
| Compare candidate | candidate path | current config directory | common tree   | empty             |

The Git head and base runs share one `EvaluatedAt` value. Task 8 uses the same field for both configuration-comparison runs.

**Narrow validation:**

```sh
go test ./cmd/archfit
go test ./cmd/archfit -run 'TestByteIdentical_(SingleModule|OneMemberWorkspace|ColdWarmNoCache)' -count=1
```

- [x] Add the typed run context and remove positional path arguments.
- [x] Route hash, validation, labels, cache, scope, and path resolution through the correct field.
- [x] Keep accepted baseline loading in callers.
- [x] Update all callers and remove the old call shape.
- [x] Add value-table tests for current callers and a shared-time Git test.
- [x] Keep normalized JSON fixtures stable and remove stale `Task 9` comments.
- [x] Run renderer, gate, narrow, and full validation checks.

### Task 6: Show the Git origin of current repair tasks

**Reason:**

The base pipeline already computes a diagnostic. Archfit discards it and keeps only the base scorecard.

**Files:**

- Modify: `internal/model/diagnostic/diagnostic.go`
- Modify: `internal/model/diagnostic/diagnostic_test.go`
- Modify: `internal/testdata/model_surface.golden`
- Modify: `cmd/archfit/worktree.go`
- Modify: `cmd/archfit/analyze.go`
- Modify: `cmd/archfit/check_test.go`
- Create: `cmd/archfit/git_finding_delta.go`
- Create: `cmd/archfit/git_finding_delta_test.go`
- Modify: `cmd/archfit/worktree_test.go`
- Modify: `cmd/archfit/byteidentical_test.go`
- Modify: `internal/output/jsonout/jsonout_test.go`
- Modify: `docs/guide/agent-feedback.md`
- Modify: `docs/guide/commands.md`
- Modify: `CLAUDE.md`

**JSON contract:**

```json
{
  "git_finding_delta": {
    "base_ref": "main",
    "comparison_status": "comparable",
    "introduced_finding_ids": ["finding-a"],
    "pre_existing_finding_ids": ["finding-b"],
    "unknown_origin_finding_ids": [],
    "comparison_reasons": []
  }
}
```

**Required behavior:**

- The block appears only when `--base` is set.
- Its field type is a pointer with `json:"git_finding_delta,omitempty"`.
- Existing `agent_tasks[]` fields and meanings do not change.
- All three ID lists are non-null arrays and use a stable order.
- The head set contains `finding_id` values from current `agent_tasks[]`.
- The base set contains all observed base findings except `status=fixed` entries.
- Match stable finding IDs. Ignore lifecycle labels and gate or advisory promotion.
- Mark an exact base match as `pre_existing` even when other evidence is incomplete.
- Mark a synthetic `bc/coupling_gate` task as `unknown` before ID matching.
- The base sub-run receives the effective head configuration after `--lang` and `--min-severity` overrides.
- The base sub-run does not reload or reparse the configuration.
- Both sides use the same configuration source and one sampled evaluation time.
- A configuration-hash mismatch makes every unmatched task `unknown`.

**Evidence comparison:**

- Normalize all enabled finding-producing analyzer families.
- The first version covers primary graph tools, pattern `ast-grep`, syntax `ast-grep/syntax`, SCIP and `scip-symbols`, `jscpd`, and `cargo-modules`.
- A primary family is active when either source tree contains that language.
- The pattern family is active when the configuration contains rule patterns.
- Syntax, SCIP, clone, and cargo-module families are active only when their analyzer setting is enabled.
- Ignore an analyzer family that is disabled on both sides.
- Treat primary `absent` with no matching coverage gap as `not_applicable`.
- `ok/ok`, `ok/not_applicable`, and `not_applicable/ok` are comparable.
- Ignore `not_applicable/not_applicable`.
- Treat `absent` with a coverage gap, `partial`, `timed out`, a missing row, or a duplicate row as unavailable evidence.
- Treat an asymmetric disabled state or another status change as unavailable evidence.
- An unmatched task is `introduced` only when all active finding-producing families are comparable.
- Otherwise put the unmatched task in `unknown_origin_finding_ids`.
- Set `comparison_status` to `unknown` when any task has unknown origin. Otherwise set it to `comparable`.
- Sort `comparison_reasons`. Use one reason for each unavailable analyzer family.

**Isolation and errors:**

- Only base finding IDs, coverage, primary-tool names, and config hash feed the delta.
- Do not copy base task paths, locations, commands, or declarations into current output.
- A base-worktree or base-pipeline error returns exit code `3` and emits no partial JSON.
- Keep the existing worktree cleanup on every return path.
- Text, Markdown, scorecard, and SARIF output do not change in this task.
- The delta does not change the verdict or exit code.

**Surface update:**

After the intentional model edit, run this command once and inspect the diff:

```sh
ARCHFIT_UPDATE_SURFACE=1 go test ./internal/ -run TestModelSurfaceNoDrift -count=1
git diff -- internal/testdata/model_surface.golden
```

**Narrow validation:**

```sh
go test ./internal/model/diagnostic ./internal/output/jsonout ./cmd/archfit
go test ./internal/ -run TestModelSurfaceNoDrift -count=1
go test ./cmd/archfit -run 'TestByteIdentical_(SingleModule|OneMemberWorkspace|ColdWarmNoCache)' -count=1
make build
bash -o pipefail -c '.bin/archfit analyze --base HEAD~1 --json -c .archfit.yaml | jq -e ".git_finding_delta"'
```

- [x] Keep the base diagnostic and pass the effective head configuration to the base run.
- [x] Build the pure ID and analyzer-compatibility comparison.
- [x] Add the optional pointer field and inspect the intentional model-surface change.
- [x] Cover exact matches, fixed entries, lifecycle labels, promotions, synthetic tasks, and new-language applicability.
- [x] Cover every analyzer family, missing and duplicate rows, overrides, cleanup, and path isolation.
- [x] Test `check --base --json` with gate exit codes `0`, `1`, and `2`.
- [x] Keep output without `--base` stable, update the guides, and run all validation commands.

### Task 7: Define configuration comparison in the existing decision model

**Reason:**

The comparison is a pure decision over two completed diagnostics and scorecards. A new package would duplicate the existing decision boundary.

**Files:**

- Create: `internal/decision/config_compare.go`
- Create: `internal/decision/config_compare_test.go`

**Finding rules:**

- Compare observed finding IDs and exclude `status=fixed` entries.
- Treat gate and advisory forms as the same finding when IDs match.
- Use the names `current_only`, `candidate_only`, and `both`.
- Do not use temporal words such as `introduced` or `resolved` for alternative configurations.
- Sort all ID lists.

**Coverage rules:**

- Compare the union of analyzer names.
- A missing or duplicate row makes the result `not_comparable`.
- Ignore equal primary `absent` rows when neither side has a matching coverage gap.
- Equal `ok` rows are comparable.
- Equal `absent` or `disabled` rows produce `comparable_with_gaps`.
- A status change produces `not_comparable`.
- Any `partial` or `timed out` row produces `not_comparable`.
- Overall priority is `not_comparable`, then `comparable_with_gaps`, then `comparable`.
- Pin every status pair and one-sided missing row in table tests.

**Measurement rules:**

- A score with `BandNA` on either side has a `null` delta.
- Emit measurement-loss warnings without regard to score direction.
- Warn when the scored fraction falls, abstained edges rise, or external edges rise.
- Give each warning a stable code and short text.
- Output data never states that the candidate is better.

**Narrow validation:**

```sh
go test ./internal/decision
go test ./internal -run TestArchImports -count=1
```

- [x] Add the pure comparison result in `internal/decision`.
- [x] Add current-only, candidate-only, both, and fixed-entry rules.
- [x] Add aggregate coverage priority and missing-row rules.
- [x] Add nullable score delta and stable measurement-warning codes.
- [x] Add identity, finding, coverage, unavailable-score, and measurement-loss tests.
- [x] Run the narrow checks and the full validation rule.

### Task 8: Add the report-only `config compare` command

**Reason:**

A configuration comparison is measurable. A general refactoring simulation needs facts about code that does not exist.

**Files:**

- Modify: `cmd/archfit/main.go`
- Create: `cmd/archfit/config_compare.go`
- Create: `cmd/archfit/config_compare_test.go`
- Modify: `README.md`
- Modify: `docs/guide/commands.md`
- Modify: `docs/guide/configuration.md`
- Modify: `docs/guide/troubleshooting.md`
- Modify: `docs/guide/quick-start.md`

**Command contract:**

```sh
archfit config compare <candidate> [-c <current>] [--root <root>] [--json]
```

**JSON contract:**

```json
{
  "schema_version": "archfit.config-compare.v1",
  "current": {
    "config_hash": "...",
    "scorecard": {},
    "classified_edges": {},
    "finding_count": 0
  },
  "candidate": {
    "config_hash": "...",
    "scorecard": {},
    "classified_edges": {},
    "finding_count": 0
  },
  "findings": {
    "current_only_ids": [],
    "candidate_only_ids": [],
    "both_ids": []
  },
  "coverage": {
    "status": "comparable",
    "details": []
  },
  "warnings": []
}
```

**Required behavior:**

- Run two full pipelines on one source tree.
- Use `Full: true`, `Advisory: true`, and `ReportOnly: true` for both runs.
- Use an empty accepted baseline for both runs.
- Do not read `.archfit-baseline.json` for the comparison.
- Share the current bundle directory for labels and fact cache.
- Use each configuration's own source bytes and hash.
- Share one evaluation time.
- Keep configuration, baseline, labels, candidate, and policy files byte-identical.
- Permit normal fact-cache reads and writes.
- Return exit code `0` after a successful comparison and `3` after an input or runtime error.
- Default text shows only changed measurements and all warnings.
- Identity text says `No measurement differences detected.`
- JSON keeps complete current and candidate measurement data.
- Identity with equal gaps may report `comparable_with_gaps` without a measurement-loss warning.
- Output never says that the candidate is better.

**Narrow validation:**

```sh
go test ./internal/decision ./cmd/archfit
make build
.bin/archfit config compare .archfit.yaml -c .archfit.yaml
bash -o pipefail -c '.bin/archfit config compare .archfit.yaml --json -c .archfit.yaml | jq -e .'
```

- [x] Register `compare` under the existing `config` group and update top-level help.
- [x] Run both configs with advisory facts, empty baselines, shared project inputs, and one time.
- [x] Render the pure decision result as short text and versioned JSON.
- [x] Add identity, warning, baseline-isolation, advisory-promotion, protected-file, and exit-code tests.
- [x] Update command selection, configuration, troubleshooting, and quick-start guides.
- [x] Run the narrow checks and the full validation rule.

### Task 9: Verify one complete product workflow

**Reason:**

Earlier tasks own all edits. This task only verifies the final command and report flow.

**Write scope:**

- No planned feature changes.
- A failed check permits a bounded integration fix in a file listed by Tasks 1 through 8.
- Do not add a new command, schema field, package, or behavior in this task.

**Final validation:**

```sh
bash -c 'if out=$(rg -n "analyze --gate|analyze .*--full|analyze --score|config init --llm|config update --llm|analyze --llm|explain --llm" examples CLAUDE.md docs/guide/concepts.md docs/design/bc-measurement-v4.md docs/design/fact-cache.md cmd/archfit/llmreview.go cmd/archfit/llmreview_test.go cmd/archfit/main_test.go cmd/archfit/root_test.go cmd/archfit/score_test.go cmd/archfit/worktree.go cmd/archfit/pipeline_run.go cmd/archfit/enrich.go cmd/archfit/enrich_test.go cmd/archfit/autopilot_test.go internal/model/diagnostic/diagnostic.go internal/extract/manifest/manifest.go internal/agenttask/agenttask_test.go internal/engine/advisory_tasks_test.go internal/output/jsonout/jsonout_test.go internal/output/markdown/markdown_test.go); then printf "%s\n" "$out"; exit 1; else rc=$?; test "$rc" -eq 1; fi'
bash -c 'if out=$(rg -n -- "--gate|--advisory|--require-tools" cmd/archfit/worktree.go); then printf "%s\n" "$out"; exit 1; else rc=$?; test "$rc" -eq 1; fi'
bash -c 'if out=$(rg -n "baseline --base" README.md docs/guide cmd/archfit/main.go cmd/archfit/baseline.go); then printf "%s\n" "$out"; exit 1; else rc=$?; test "$rc" -eq 1; fi'
make fmt && make test && make lint
make build
make archfit
bash -o pipefail -c '.bin/archfit config update --json -c .archfit.yaml | jq -e .'
bash -o pipefail -c '.bin/archfit config compare .archfit.yaml --json -c .archfit.yaml | jq -e .'
git rev-parse --verify HEAD~1
bash -o pipefail -c '.bin/archfit analyze --base HEAD~1 --json -c .archfit.yaml | jq -e ".git_finding_delta"'
git diff --check
git status --short
```

- [x] Verify command help, report labels, examples, and guides.
- [x] Verify every new JSON field, list, status, reason, and unknown state.
- [x] Verify that no out-of-scope command, state file, or action channel exists.
- [x] Run every final validation command and inspect its output.
- [x] Inspect `git status --short`; intentional plan and implementation paths may appear.
- [x] Record intentional model-surface and golden changes in the `Implementation Results` section.

## Acceptance Criteria

- Scripts and active examples use valid current CLI commands.
- Benchmarks cannot report parser failures as valid timings.
- `archfit baseline` has no ineffective Git-base mode.
- Baseline score comparisons include scorer and rubric compatibility.
- `--no-advisories` has one meaning for all output combinations.
- `config update` shows every change that `--apply` would make.
- `config update` emits one clear review in text or JSON.
- `check --base <ref> --json` classifies current task origin conservatively.
- Missing analyzer evidence never creates a false introduced task.
- `config compare` compares raw measurements without accepted-baseline bias.
- `config compare` reports coverage and measurement loss.
- New report data does not change the gate verdict.
- All mandatory validation commands pass.

## Completion Command

```sh
ralphex docs/plans/20260821-archfit-configuration-confidence.md
```

## Implementation Results

### Model-surface changes

`internal/testdata/model_surface.golden` changed in three hunks (5 net lines),
all from Task 6:

- `internal/model/diagnostic.Diagnostic` gains
  `GitFindingDelta *diagnostic.GitFindingDelta "json:\"git_finding_delta,omitempty\""`,
  placed before the existing `Delta` field.
- New type `internal/model/diagnostic.GitFindingDelta` with the fields
  `BaseRef`, `ComparisonStatus`, `IntroducedFindingIDs`, `PreExistingFindingIDs`,
  `UnknownOriginFindingIDs`, and `ComparisonReasons`.
- New constants `internal/model/diagnostic.GitComparisonComparable` and
  `internal/model/diagnostic.GitComparisonUnknown`.

The golden was regenerated once with `ARCHFIT_UPDATE_SURFACE=1` and the diff was
inspected. No other kernel or `internal/view` surface changed on the branch.

### Golden-output changes

None. No `internal/engine` golden file changed. `make test` passes, which runs
`TestGolden` in `internal/engine`, so the rendered output is unchanged.
`internal/testdata/model_surface.golden` is the only golden file the branch
touches. `internal/engine/advisory_tasks_test.go` changed by two lines, but it
is a test file, not a golden.

### Final validation

Every command in the Task 9 final-validation block ran and its output was
inspected. All pass.

- The three `rg` guard checks exit `0` with no matches: no removed CLI forms in
  the listed active surfaces, no gate or advisory flags in
  `cmd/archfit/worktree.go`, no `baseline --base` in the README, guides, or the
  baseline command.
- `make fmt` produces no diff. `make test` exits `0` (Go race suite plus
  `internal/extract/scip/scip_reader_test.py`), with no `FAIL` line;
  `cmd/archfit` at 84.7% and `internal` (arch ring plus model surface) both ok.
  `make lint` reports `0 issues`. `make build` succeeds.
- `make archfit` exits `0`: `PASS · 0 blocking`, 39 advisory warnings, score
  `71 / 100 serviceable`. The baseline was not regenerated.
- `config update --json -c .archfit.yaml` returns `archfit.config-review.v1`,
  status `action_required` (0 module issues, 75 structure changes: 31 added, 44
  removed), and all seven structure, issue, and suggestion lists marshal as JSON
  arrays, never `null`. The text form prints the short status line first.
- `config compare .archfit.yaml -c .archfit.yaml` exits `0`, reports identical
  config hashes, 39 findings on both sides, empty `current_only_ids` and
  `candidate_only_ids`, 39 `both_ids`, no warnings, and the exact identity label
  `No measurement differences detected.` All five JSON lists marshal as arrays.
  Coverage graded `not_comparable` at the time of this run because the two
  duplicate `ast-grep` rows made the pair unpairable; after the
  `ast-grep/syntax` rename it grades `comparable_with_gaps` (cargo-modules
  absent on both sides).
- `analyze --base HEAD~1 --json` and `check --base HEAD~1 --json` both emit
  `git_finding_delta` with `base_ref: HEAD~1`, `comparison_status: comparable`,
  and three sorted, unique `comparison_reasons`. All three ID lists render as
  non-null empty arrays because the head run has zero `agent_tasks[]`.
  `check --base` still exits `0`, so the block changes neither the verdict nor
  the exit code. `analyze --json` without `--base` omits the block entirely.
- `archfit baseline --base main` prints `archfit: unknown flag --base` and exits
  `3`. Both `analyze --help` and `check --help` describe `--base` as adding a
  base-vs-head delta to normal output.
- `--no-advisories` drops findings from `39` to `0` and advisory tasks from `13`
  to `0` in JSON while the scorecard still reports `71/100` with
  `Rubric version: 1`, so removing advisories does not change the score.
- The command set is exactly `analyze`, `check`, `baseline`, `explain`,
  `doctor`, and the `config` group (`init`, `update`, `compare`, `enrich *`).
  A repository grep finds no `config health`, `config suggest`,
  `architecture_actions`, `--agent-tasks`, or `architecture_hash`.
- `git diff --check` and `git status --short` are both clean after every
  report-only run, so config, baseline, labels, candidate, and policy files
  stayed byte-identical.

### Second-review corrections

A second critical review found three MAJOR defects. All three are fixed.

1. **`partial` was treated as unusable evidence on both pairing paths.**
   dependency-cruiser and grimp mark a COMPLETED run `partial` as soon as one
   import specifier fails to resolve, so the git-origin delta was permanently
   all-`unknown` and `config compare` permanently `not_comparable` on every
   TypeScript and Python repo. `Coverage.Unresolved` is the structural
   discriminator: ts/py set it on a completed run; every other partial producer
   (failed extractor, rejected ast-grep rule file, empty SCIP index, failed
   jscpd) leaves it zero. A SYMMETRIC unresolved-specifier partial now pairs
   (`comparable_with_gaps` / `familyPairedDegraded`) and always emits a
   disclosure reason; every other partial and every timed-out row stays
   unavailable. This DEVIATES from the Task 6 and Task 7 required-behavior lines
   ("`partial` … unavailable evidence", "Any `partial` or `timed out` row
   produces `not_comparable`"), which were written before the steady-state
   meaning of `partial` was understood.
2. **`projectMarkerPresent` checked only the scan-root directory**, so a repo
   whose Go lives under `services/api/go.mod` answered "no Go here", the
   coverage gap was suppressed, and a real analyzer failure became
   `not_applicable` — the one absent shape that pairs with `ok`. Fixed in gap
   derivation (`primaryToolProjectProbe`), not in either consumer. Go now probes
   the tree; the other three primaries stay root-only because their analyzers
   resolve from a root manifest. `coverage_gaps` on this repo is byte-identical
   before and after (root `go.mod` short-circuits the walk).
3. **`config update --apply` emptied the module map.** On this repo it commented
   out all 44 configured modules and added 31 bare stanzas, because
   `DiffModules` matches by NAME and the config/discovery conventions differ
   (`internal/agenttask` vs `agenttask`). `initcfg.ResolveNameDrift` now
   reclassifies 1:1 add/remove pairs with equal normalized path sets as
   `NameDrift`, and `Removed` became review-only across `reviewStatus`,
   `hasActionableEdits`, and `buildUpdateEdits`. Verified on a scratchpad copy:
   `--apply` now writes one added stanza (7 lines) instead of rewriting 305.
   `config update --json` on this repo reports 1 added, 14 unmatched, 30 name
   drift, 0 issues.

Two MINOR items are also fixed: extractor-failure coverage rows now use
`ports.Extractor.CoverageTool()` instead of `Name()` (so a failed Go extractor
files under `go/packages`, not a phantom `go`), and the `BundleDir` bullet at
`CLAUDE.md:187` is un-corrupted.

Residual, accepted: symmetric-partial pairing in the git delta can still produce
a false `introduced` on a badly misconfigured TypeScript repo where most
specifiers are unresolved on both sides. The alternative is a permanently inert
feature. SUPERSEDED in the third review: the claim that "the disclosure reason
covers it" was false — the reason rendered only the status. It now carries both
sides' unresolved magnitudes. Point 1's claim that `Coverage.Unresolved` is the
discriminator is also WRONG and was corrected; see "Third-review corrections".

`internal/testdata/model_surface.golden` did not move — none of these fixes
touch `internal/model/*` or `internal/view`.

### Known limitations recorded, not fixed

- ~~`DiffModules` skips every `Removed` module from both `Unclassified` and
  `Issues`~~ FIXED in the third review. The cost claim ("needs a redesign this
  plan rules out") was wrong: `NameDrift` carries its source `ExistingModule` —
  one struct field — and `ResolveNameDrift` re-runs the field checks. Only
  unpaired removals stay unchecked, and they are now disclosed by name in
  `unchecked_modules`.
- ~~`analyze --base` in a worktree checkout reports
  `go/packages: head ok, base absent` … It is pre-existing and unrelated to this
  plan.~~ FIXED in the fourth review, and the framing was wrong twice over.

  The root cause predates the branch, but a NEW feature being 100% inert on the
  project's own repository is a property of that feature, not an inherited
  defect — and "discloses this honestly" understated the damage: the base side
  going absent made EVERY task `unknown_origin`, which the guide tells agents to
  treat as possibly introduced. A finding that provably pre-dated the base ref
  was reported as maybe-yours. Reach was every Go repo following `go help work`'s
  recommendation not to commit `go.work`.

  `DiscoverMembers` already decided correctly (it falls back past an
  in-scope-empty `go.work`); the decision simply never reached the toolchain. It
  now rides `Members.GoWorkOff` into `packages.Config.Env` AND into the scip
  indexer subprocess — `scip-go` was a second victim of the same cause, not a
  separate defect: in a base-worktree-shaped directory it indexed 1 package
  (182-byte, empty index → `scip: partial`) versus 154 packages with `GOWORK=off`.
  Fixing only the in-process loader would have satisfied a `go/packages`-only
  check while `scip` and `scip-symbols` kept the feature inert.

- The base-side worktree parent moved from the config directory to `gitRoot`
  (fourth review), so the checkout always sits inside the analyzed repo and
  inherits its gitignored resolution inputs (`node_modules`, generated code).
  Ceiling: this writes `.archfit-cache/worktrees/` into the analyzed repo even
  for a user who deliberately keeps all archfit state outside it. On a read-only
  checkout the `os.MkdirTemp` fallback still fires, and there the inherited-input
  rescue does NOT apply — a TypeScript repo analyzed that way can still report a
  base-side `partial (n/n unresolved)`. Disclosed in `comparison_reasons`, not
  silent, and no known layout hits it in CI.

### Post-review corrections

Code review found that Task 6 and Task 7 implemented the same "pair two runs'
coverage rows and decide comparability" rule with opposite results on identical
input. The two paths now agree:

- `internal/extract/astgrep/syntax.go` labels the syntax pass `ast-grep/syntax`
  (`syntaxToolName`), so `tool_coverage` no longer carries two `ast-grep` rows.
  The duplicate-row disagreement disappears at its source: both paths treat a
  repeated coverage name as an unpairable duplicate, and the git-origin delta's
  combined ast-grep family splits into two independent single-tool families.
- The coverage-gap condition is per-side on both paths. A gap on one side only
  is an asymmetry (`not_comparable` / unavailable), not shared blindness.
- A gapless `absent` row on a NON-primary analyzer is no longer unconditionally
  unavailable in the git-origin delta. It pairs with itself: symmetric absence
  means neither side produced findings from that analyzer, so it cannot hide an
  origin. It still never pairs with `ok`, which would call a head finding
  introduced on a base side the analyzer never examined. Without this, enabling
  `scip` with no indexer installed made every repair task unknown-origin.

### Third-review corrections

A third critical-only review found one CRITICAL defect and four MAJOR ones, all
introduced or left standing by the second round (`839670d`). All are fixed.

1. **CRITICAL — the `Unresolved > 0` discriminator was factually wrong.** The
   second round rested on "only dependency-cruiser and grimp set
   `Coverage.Unresolved` on a completed run". `internal/extract/golang` sets it
   too (`golang.go:264,279`), counting whole packages `collectNodesEdges`
   SKIPPED because they failed to load — the exact "did not finish" meaning the
   rule was supposed to route to unavailable. A Go partial therefore graded
   `comparable_with_gaps` in `config compare` (with a reason that falsely named
   import specifiers) and could produce a false `introduced` in `--base`. The
   discriminator is now the TOOL NAME, in one shared predicate
   (`decision.PartialFromUnresolvedSpecifiers`) both paths call, so they cannot
   diverge again. The false invariant was also written into two code comments
   and `CLAUDE.md:155-168`; all three now state the real rule and name
   `go/packages` explicitly.
2. **MAJOR — symmetric gapped-absent made the git delta inert.** An enabled
   analyzer whose tool is not installed reports `absent` + gap on BOTH sides;
   `normalizeCoverage` mapped that to `evidenceUnavailable`, which pairs with
   nothing, so every repair task landed in `unknown_origin_finding_ids`.
   `decision.gradeTool` already had the correct rule (fail only on ASYMMETRY),
   so this was the same two-paths-two-rules defect a fourth time. Symmetric
   gapped-absent now routes to `familyPairedDegradedAbsent` — comparable, and
   always disclosed, never a silent `familyPaired`. Asymmetric absence stays
   unavailable, gapped-absent still never pairs with `ok`, and timeouts plus
   `partial`-without-unresolved stay unavailable (a timeout is flaky, not
   structural, so symmetry proves nothing there). Verified end-to-end on a
   Go+Rust fixture with no cargo installed: `comparison_status` moved from
   `unknown` to `comparable` with the new violation in
   `introduced_finding_ids`, and both blind analyzers named in
   `comparison_reasons`. Reach: archfit's own runtime image ships without the
   Rust toolchain, so every repo with a `Cargo.toml` was permanently inert
   there. `git_finding_delta_test.go` CHANGED: the case
   "non-primary absent with a coverage gap is unavailable" pinned the buggy
   outcome and now asserts comparable-and-disclosed.
3. **MAJOR — the disclosure that justified the accepted residual risk did not
   exist.** Both reason strings rendered only `Status`, so `unresolved=3` and
   `unresolved=5000` were indistinguishable. Both now carry each side's
   magnitude, with the specifier denominator when the extractor tracks one
   (`decision.UnresolvedMagnitude`). `CoverageDetail.Current/Candidate` stay raw
   statuses — they are typed machine fields, not prose. The magnitude-asymmetry
   grading the reviewer offered as an alternative was NOT added: disclosure was
   the stated either/or, and a ratio threshold is new policy.
4. **MAJOR — `markerWalkPrune` diverged from `scope.DefaultExclusions`.** It
   missed `testdata/` and `reports/`, so a `go.mod` under `testdata/` (routine
   in Go repos) was excluded by the extractor but FOUND by the marker walk, and
   the resulting gap reinstated the inertness of item 2. The prune set is now
   derived from the effective exclusions (`scope.ExcludedDirNames`), and the
   marker's repo-relative path is matched against those globs with the same
   doublestar matcher the extractors use — so config entries like
   `services/legacy/**`, which name no single directory, are honoured too.
   `target/` was dropped from the hand-written list: it is NOT in
   `DefaultExclusions`, so pruning it was itself a divergence.
5. **MAJOR — `config update` reported `issues: []` for modules it never
   checked** (44 of 45 on this repo's own config). See the corrected "Known
   limitations" bullet above. Verified: stripping `owner`, `subdomain`, and
   `volatility` from the name-drifted `internal/agenttask` stanza now yields
   `missing_owner` + `missing_volatility_input`; before the fix it yielded
   nothing. Unpaired removals stay unchecked by design (discovery found no code
   for them) and are listed in the new `unchecked_modules` field, which the text
   status line also counts.

Minor, also fixed: `updateEvidenceDiagnostics` now includes `name_drift`, so the
`--ai-classify` evidence pack no longer reads `added=0 removed=0 path_drift=0
structurally_in_sync=false`.

Structural guard added: `TestGitFindingDelta/cross_path_agreement` drives one
table of coverage shapes through BOTH `pairFamily` and `decision.gradeTool` and
fails when they disagree on any shape not explicitly marked divergent. Three
rounds found the same class of defect; the agreement is now asserted rather than
restated in prose. The one specified divergence (`ok` vs gapless-`absent`
primary — `--base` compares two trees, `config compare` compares one) is table
data. Symmetric `disabled` is not a divergence at the pairing level: both paths
call it comparable; the git path additionally drops disabled families in
`analyzerFamilies`, which is Task 7 required behavior.

`internal/testdata/model_surface.golden` did not move: no fix touches
`internal/model/*` or `internal/view`. `.archfit-baseline.json` was not
regenerated.
