# New Wave 2: semantic architecture evidence pack and draft config/rules

> **Executable ralphex plan.** Run with `ralphex docs/plans/20260705-wave2-semantic-evidence-pack.md` from the repo root. ralphex executes one `### Task N:` section at a time. Completed historical waves in `docs/plans/completed/` are context only and do not affect this new wave numbering.
>
> This plan is off-gate by design. LLM output may draft labels, config, rules, and review text. It must never directly decide `analyze --gate`.

## Overview

Improve the semantic side of archfit so the LLM acts as an experienced software architect over repository intent: architecture/design/ADR documents, README files, comments, public APIs, and representative code. The deterministic core remains authoritative; LLM output is draft/proposal material that humans pin into config, labels, or rules before it can affect later deterministic runs.

This plan can run after New Wave 1 or in parallel if it does not consume Wave 1 output fields. If Wave 1 adds connascence or distance-basis evidence, include those fields in the LLM evidence pack before final validation.

## Source artifact

Primary source: `reports/book-alignment-review-2026-07-05/00-REVIEW.md`.

Source refs used by this plan:

- Report §1 verdict: semantic input is reviewable but shallow; LLM flows mostly see paths, globs, basenames, and Markdown headings.
- Report §5.2: module role, subdomain, volatility, owner meaning, abstained labels, and architecture intent need semantic judgment but must become pinned deterministic input before affecting score.
- Report §5.3: safety boundary is mostly right; the weak spot is evidence-pack quality, not gate separation.
- Report §6 P1: richer draft files/inline annotations need evidence refs.
- Report §6 P3: CLI/docs surface drift around review vs `analyze --llm` needs alignment.
- Existing implementation context: `cmd/archfit/init.go`, `cmd/archfit/update.go`, `cmd/archfit/enrich.go`, `cmd/archfit/enrich_abstained.go`, `cmd/archfit/llmreview.go`, `internal/initcfg/classify_targets.go`, `internal/labels/labels.go`, `internal/llm/*`, and `docs/plans/completed/20260702-wave7-llm-semantic-labels.md`.

## Success criteria

- LLM prompts receive bounded, cited architecture evidence from docs, comments, public APIs, config, and deterministic diagnostics.
- `config init --llm`, `config update --llm`, `config enrich ...`, and `analyze --llm` share the same evidence-pack builder where practical.
- LLM responses must cite evidence IDs and distinguish deterministic facts from semantic judgments.
- Draft outputs can propose `.archfit.yaml` modules, owners, subdomains, volatility, `external_systems`, rules, and labels without mutating config in plan/default modes.
- No LLM dependency enters `internal/*` scoring/gate packages. Existing arch ring tests still enforce this.
- Corpus validation proves draft quality improves without changing deterministic `analyze --gate` output unless drafts are human-approved and pinned.

## Validation Commands

- `go test ./internal/initcfg/... ./internal/labels/... ./internal/llm/...`
- `go test ./cmd/...`
- `go test ./internal/...`
- `make test`
- `make lint`
- `make archfit`
- `.bin/archfit config init --llm --root . -o /tmp/archfit-init-llm.yaml`
- `.bin/archfit config update --llm --root . --config .archfit.yaml`
- `.bin/archfit config enrich abstained --root . --config .archfit.yaml`
- `.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/archfit-deterministic.json`
- `.bin/archfit analyze --full --json --llm --config .archfit.yaml > /tmp/archfit-llm.json`

## Implementation Steps

### Task 1: Build a shared bounded architecture evidence pack

Justification: Report §5.3 says the gate boundary is right but evidence-pack quality is weak; report §5.2 says architecture intent from ADRs/docs/comments is legitimate semantic input.

Files:

- `internal/initcfg/classify_targets.go` — extend or delegate from current target collection.
- `internal/initcfg/evidence_pack.go` — add a pure evidence-pack builder with no LLM imports.
- `internal/initcfg/evidence_pack_test.go` — cover doc discovery, citation IDs, bounds, sorting, and redaction behavior.
- `cmd/archfit/init.go` — consume the shared pack for `config init --llm`.
- `cmd/archfit/update.go` — consume the shared pack for `config update --llm`.
- `cmd/archfit/enrich.go`, `cmd/archfit/enrich_abstained.go`, `cmd/archfit/enrich_values.go` — pass evidence pack slices to enrich prompts where useful.
- `docs/guide/llm-enrich.md` — document evidence sources and bounds.

Preconditions: Current LLM commands and tests pass; `.env` key is available only for manual/corpus validation, not unit tests.
Postconditions: All LLM command paths can request a deterministic, sorted, bounded evidence pack with citation IDs.

Fitness gate: None. This is off-gate. `make archfit` must be byte-identical before and after when no draft files are pinned.

Impact commands:

- `gitnexus impact BuildClassifyTargets --direction upstream --depth 4`
- `gitnexus impact InitCmd --direction upstream --depth 3`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/initcfg/...`
- `go test ./cmd/... -run 'Test.*Init|Test.*Update|Test.*Enrich|Test.*LLM'`
- `.bin/archfit analyze --gate --full --config .archfit.yaml`

Manual checks:

- Review the evidence pack for one medium repo. It should include useful intent, not large raw dumps.
- Confirm secrets and `.env` contents are never included.

- [x] Add evidence source discovery for README, docs/design, docs/architecture, ADR-like docs, package comments, exported names, config snippets, and deterministic diagnostics.
- [x] Assign stable evidence IDs such as `doc:...`, `api:...`, `diag:...`, `comment:...`.
- [x] Bound token budget by source type and include deterministic truncation/sorting.
- [x] Add tests proving stable output order and no secret file inclusion.
- [x] Wire the pack into `init`, `update`, and `enrich` prompt builders without importing `internal/llm` from core packages.

### Task 2: Require cited structured drafts for config and rule proposals

Justification: Report §6 P1 calls for richer draft files/inline annotations with evidence refs; the user goal asks LLM to create/update configuration and rules while deterministic tools supply deterministic facts.

Files:

- `internal/initcfg/subdomains_draft.go` — include evidence refs and rationale fields.
- `internal/initcfg/value_draft.go` — include evidence refs for owner/volatility drafts.
- `internal/initcfg/update.go` — extend drift report/config diff to carry cited suggestions.
- `internal/initcfg/yamledit*.go` — patch only reviewed/applied entries; plan mode remains inert.
- `cmd/archfit/update.go` — prompt/schema for modules, owners, volatility, external systems, and rule suggestions.
- `cmd/archfit/enrich_values.go` — prompt/schema for cited owner/volatility/subdomain drafts.
- `internal/config/types.go` — add draftable rule/config fields only if not already represented.
- `internal/configschema/schema.go` — regenerate schema if config structs change.
- `docs/guide/configuration.md`, `docs/guide/configuration-reference.md`, `docs/guide/llm-enrich.md` — document review/apply workflow.

Preconditions: Task 1 evidence pack is available.
Postconditions: LLM outputs structured drafts with evidence refs; plan/default modes do not mutate `.archfit.yaml`.

Fitness gate: None. Draft outputs are inert until human-approved and applied. Existing self `.archfit.yaml` should not change in this task unless explicitly updating docs/examples.

Impact commands:

- `gitnexus impact RenderUpdateReport --direction upstream --depth 4`
- `gitnexus impact EnrichVolatilityCmd --direction upstream --depth 3`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./internal/initcfg/... ./internal/config/... ./internal/configschema/...`
- `go test ./cmd/... -run 'Test.*Update|Test.*Enrich'`
- `make schema`
- `.bin/archfit config update --llm --root . --config .archfit.yaml > /tmp/archfit-update-llm.txt`

Manual checks:

- Review one draft and confirm each suggestion cites evidence and says whether it is deterministic fact or semantic judgment.

- [x] Extend LLM response schemas to require evidence refs for every proposed module/subdomain/volatility/owner/rule change.
- [x] Add rule suggestions for existing deterministic rule types only, e.g. `forbidden_dependency`, `forbidden_role_dependency`, `public_api_max`, `public_api_change`, and `coupling.gate` tuning.
- [x] Add tests that missing evidence refs reject or retry the response.
- [x] Add tests that default/plan mode leaves `.archfit.yaml` byte-unchanged.
- [x] Update docs for draft-review-apply semantics.

### Task 3: Strengthen `analyze --llm` as an architect review, not a scorer

Justification: Report §5.2 says narrative review and prioritization must stay advisory and post-verified against deterministic evidence; report §6 P3 says prompt/surface drift around review/analyze needs alignment.

Files:

- `cmd/archfit/analyze.go` — keep deterministic report first, then optional LLM review.
- `cmd/archfit/llmreview.go` — require citations to deterministic findings/evidence-pack refs and classify assertions.
- `cmd/archfit/llmreview_test.go` — mocked provider tests for citation validation, unsupported claim rejection, and deterministic output unaffected.
- `cmd/archfit/help_test.go`, `cmd/archfit/main.go` — align help text for `analyze --llm` and avoid stale `review` command language.
- `docs/guide/commands.md`, `docs/guide/agent-feedback.md`, `docs/guide/llm-enrich.md` — document review semantics.

Preconditions: Tasks 1-2 have shared evidence/citation machinery.
Postconditions: `analyze --llm` produces cited advisory review text and cannot alter verdict, findings, metrics, or scorecard.

Fitness gate: Existing deterministic gate. Add a byte-identity assertion that `analyze --gate` output is unchanged by presence/absence of LLM config unless `--llm` is explicitly passed, and even then deterministic sections remain unchanged.

Impact commands:

- `gitnexus impact runLLMReview --direction upstream --depth 4`
- `gitnexus impact AnalyzeCmd --direction upstream --depth 3`
- `gitnexus detect-changes --scope all`

Verification commands:

- `go test ./cmd/... -run 'Test.*LLMReview|Test.*Analyze|Test.*Help'`
- `make archfit`
- `.bin/archfit analyze --full --json --config .archfit.yaml > /tmp/deterministic.json`
- `.bin/archfit analyze --full --json --llm --config .archfit.yaml > /tmp/with-llm.json`

Manual checks:

- Review one LLM report. It should read like an architect review but cite deterministic facts and source evidence.

- [x] Add prompt/schema fields for claim type: deterministic fact, semantic interpretation, recommendation, or unknown.
- [x] Reject or flag recommendations that lack finding IDs, metric IDs, or evidence refs.
- [x] Add tests proving verdict/score/finding JSON is unchanged by LLM review.
- [x] Align CLI help/docs around `analyze --llm` instead of any stale `review` command wording.

### Task 4: Corpus validation with real LLM key and deterministic controls

Justification: Report §7 did not run a fresh external corpus sweep; the user goal explicitly asks for validation against projects from the test corpus.

Files:

- `docs/plans/notes/wave2-semantic-evidence-validation.md` — validation table and accepted/rejected draft samples.
- `scripts/` optional helper — only if needed to repeat corpus LLM draft checks.
- `reports/book-alignment-review-2026-07-05/` optional comparison artifacts — only if preserving run outputs is useful.

Preconditions: Tasks 1-3 pass mocked tests; real provider key is available in `.env` for manual validation.
Postconditions: Corpus validation proves draft quality and deterministic controls.

Fitness gate: `make archfit` must pass before and after corpus LLM runs. Corpus worktrees must remain clean except intentional scratch draft files kept outside the repo or recorded as artifacts.

Impact commands:

- `gitnexus detect-changes --scope all`

Verification commands:

- `make build`
- `make archfit`
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`
- `.bin/archfit config init --llm --root . -o /tmp/archfit-self-init-llm.yaml`
- `.bin/archfit config update --llm --root . --config .archfit.yaml > /tmp/archfit-self-update-llm.txt`
- `.bin/archfit config enrich abstained --root . --config .archfit.yaml`

Manual checks:

- Corpus spot checks: Go `archfit`, Python `ccgram` or `prefect`, Rust `herdr` or `yazi`, TypeScript `storybook`.
- For each language, inspect at least five non-trivial LLM suggestions and record accepted/rejected counts plus reason.

- [x] Run deterministic corpus attribution before any LLM commands.
- [x] Run `config init --llm`, `config update --llm`, and relevant `config enrich` flows on scratch copies or temp output paths.
- [x] Confirm deterministic `analyze --gate` output is byte-identical before/after when drafts are not pinned.
- [x] Record accepted/rejected samples and key false positives in `docs/plans/notes/wave2-semantic-evidence-validation.md`.
- [x] Confirm all corpus worktrees are clean or only contain pre-existing untracked files.

### Task 5: Final docs, examples, and re-review handoff

Justification: The user goal requires documentation updates; report §6 P3 flags prompt/surface drift and practical usage gaps.

Files:

- `docs/guide/llm-enrich.md` — evidence-pack workflow, review/apply semantics, citation requirements.
- `docs/guide/commands.md` — CLI examples for off-gate LLM commands.
- `docs/guide/configuration.md` — draft config/rules workflow.
- `docs/guide/agent-feedback.md` — agent use of deterministic facts plus LLM draft suggestions.
- `docs/guide/troubleshooting.md` — LLM/key/cost/cache failures and deterministic fallback.
- `docs/spec/arch-fitness-spec-v0.4.md` — update if public behavior changes.
- `CLAUDE.md` — update project invariants only if new invariants were added.

Preconditions: Tasks 1-4 verification commands pass.
Postconditions: Docs match behavior; off-gate boundary and human-review workflow are clear.

Fitness gate: Final self gate must pass. Docs changes must not alter deterministic output.

Impact commands:

- `gitnexus detect-changes --scope all`

Verification commands:

- `make all`
- `.bin/archfit doctor`
- `.bin/archfit analyze --gate --full --config .archfit.yaml --base origin/main`

Manual checks:

- Read docs as a new user: it must be clear which commands mutate config, which only draft, and which affect CI.

- [x] Update docs and examples for every changed LLM command.
- [x] Document cost/cache/secret boundaries for evidence packs.
- [x] Run final validation commands.
- [x] Record the scoped `architecture-review` follow-up.

## Acceptance criteria

- All new LLM prompts are covered by mocked tests and schema validation.
- Every LLM suggestion in new draft formats carries evidence refs.
- Plan/default modes leave `.archfit.yaml` unchanged.
- `internal/*` gate/scoring packages still do not import `internal/llm`.
- `make all` passes.
- Corpus validation includes Go, Python, Rust, and TypeScript scratch runs.
- Docs distinguish deterministic facts, semantic drafts, human-approved pinned inputs, and advisory LLM review.

## Safety notes

LLM calls can spend provider credits and may expose repository text to the configured provider. Do not include secrets, `.env`, credentials, generated dependency caches, or large raw files in evidence packs. Drafts are not authoritative until reviewed and pinned. No destructive steps or data migrations are expected.

The architect does not apply this plan. An engineer, mutator agent, or ralphex executes it after approval.

## Re-review

After implementation, run a scoped `architecture-review` on the semantic boundary:

- `cmd/archfit/init.go`
- `cmd/archfit/update.go`
- `cmd/archfit/enrich*.go`
- `cmd/archfit/llmreview.go`
- `internal/initcfg/*`
- `internal/labels/*`
- `internal/llm/*`
- `internal/arch_test.go`

Acceptance signals for re-review: LLM remains off-gate, evidence refs are useful and bounded, default modes are inert, and deterministic analyze output is unchanged unless humans pin drafts into config/labels.

Recorded handoff (2026-07-05): run scoped `architecture-review` on the files above after this wave lands, using the Task 5 validation as the deterministic baseline (`make all`, `archfit doctor`, and `archfit analyze --gate --full --config .archfit.yaml --base origin/main`).
