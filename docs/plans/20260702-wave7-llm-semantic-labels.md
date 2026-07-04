# Wave 7: LLM Semantic Layer — labels for what static analysis cannot see

## Overview

Wave 7 of 7 from `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` (top-10 item 7). Assumes Waves 1–6 merged. This is the hybrid syntactic+semantic vision: the deterministic core stays the only thing that scores and gates; the LLM fills the two holes static analysis provably cannot — and its output enters ONLY through the existing pinned-label path (`.archfit-labels.yaml`, provenance/confidence already consumed by the scorer).

Two semantic gaps, both measured on the corpus:

- **Abstained edges.** Strength classification abstains when no static signal exists (correct: abstain-not-fake). Cost: yazi scores only 66 of 210 classifiable edges (31% scored → confidence capped low); ruff's 23 scored edges are 100% clone-derived. Deciding whether an edge is contract/model/functional coupling is a question about what the code _means_ — LLM territory.
- **Volatility differentiation.** The book says volatility comes from the business domain (subdomain roles: core / supporting / generic), not commit history. Config-declared roles + inheritance produce the uniform-volatility triage cliff (herdr 100% high, tokio 99.7% medium). Assigning roles from READMEs, module names, and domain language is semantic judgment — LLM proposes, human pins.

**Invariants that must survive this wave (from CLAUDE.md, enforced structurally):**

- LLM SDKs stay off-gate: only `config enrich` / `config init --llm` / `config update --llm` / `analyze --llm` / `explain --llm` touch them; `internal/*` must never import `internal/llm` (arch_test enforces — extend, don't weaken).
- Determinism: with a committed `.archfit-labels.yaml`, two analyze runs are byte-identical; the LLM runs at enrich time, never at analyze/gate time.
- Provenance discipline: `provenance: llm` with confidence below `high` already lowers coupling_balance confidence one band — keep; never let LLM labels raise confidence above the static baseline.

## Context

- Existing label pipeline: `.archfit-labels.yaml` draft→review→pin flow (`config enrich`, `--apply`), `AcceptedSet`, provenance handling in `internal/score`.
- LLM plumbing: `internal/llm` (anthropic-sdk-go, openai-go) used from `cmd` only; API key comes from the repo `.env` (project memory `anthropic-key-from-env`).
- Abstained-edge data: `classified_edges` buckets + the abstained list with per-edge context (source/target module, sample locations).
- Volatility: config `modules.*.volatility`, synthetic-module override from Wave 5 Task 4.

## Development Approach

- Feature work, regular test-after approach; but every consumption path (labels → scorer) gets behavior tests, and the determinism test is non-negotiable.
- LLM calls are mocked at the `internal/llm` client seam in tests (system-boundary mock per project convention); one recorded-fixture test per prompt for shape regressions.
- Branch `feat/wave7-llm-labels`; `make test && make lint && make archfit` between tasks; PR at end.

## Implementation Steps

### Task 1: enrich — strength proposals for abstained edges

- [x] extend `config enrich` with an abstained-edge pass: batch abstained edges (cap per run, e.g. 100, named constant) with per-edge context — module names, both endpoints, up to N sample usage locations with a few lines of code around each
- [x] prompt design: ask for one of contract/model/functional/intrusive + one-sentence rationale + self-reported confidence (high/medium/low), citing the book's level definitions verbatim in the system prompt; require JSON output validated against a schema
- [x] write proposals as DRAFT labels (`provenance: llm`, confidence carried through) — never auto-pin; `--apply` pins after human review, same as today's flow
- [x] tests: batch construction (cap respected, context complete), JSON validation retry path, draft-not-pinned guarantee, mocked-client end-to-end
- [x] `make test && make lint && make archfit`; commit

### Task 2: scorer consumption + confidence discipline for llm labels

- [x] verify (test, likely exists) pinned llm-provenance strength labels feed classification exactly like human labels except the confidence lowering; add a test that an llm label NEVER overrides config-authoritative contract/intrusive or the Go type-info hint (same precedence as SCIP-for-Go: compiler-grade beats LLM)
- [x] add `classified_edges` evidence: `labeled_llm: N` bucket so the scored-fraction increase is visibly attributed
- [x] determinism test: fixture with committed labels file — two full runs byte-identical; delete labels → abstains return
- [x] `make test && make lint && make archfit`; commit

### Task 3: config update --llm — subdomain role proposals

- [x] extend `config update --llm` to propose per-module `subdomain: core|supporting|generic`, derived `volatility:`, layer, and optional architectural `role:` from repo evidence (README, module names, docs/ headings, public API shape) with per-module rationale — as a config DIFF for human review, never auto-applied
- [x] prompt encodes the book's subdomain heuristics (core = competitive advantage/changing; supporting = boring, stable; generic = solved problem, implementation may churn) — quote the definitions, demand rationale referencing repo evidence
- [x] synthetic-module coverage: proposals may target synthetic keys (Wave 5 Task 4 override mechanism) so herdr-shaped repos get differentiated volatility
- [x] tests: diff generation (no in-place mutation), rationale presence, mocked-client end-to-end
- [x] `make test && make lint && make archfit`; commit

### Task 4: Four-language validation on the corpus

- [x] Rust — yazi: run the enrich pass with the real key (from `.env`); target scored fraction ≥ 80% after human-style review of drafts (record accepted/rejected counts); coupling_balance confidence band reflects llm provenance correctly — scratch validation accepted 144/rejected 0, scored fraction reached 100%, and confidence stayed medium with llm-provenance lowering
- [x] Rust — herdr / tokio: role proposals differentiate volatility (no longer ~100% uniform); spot-check 5 proposals against your own domain judgment and record agreement rate in the PR — review-only synthetic proposals differentiated volatility; spot checks agreed 5/5
- [x] Python — prefect: enrich on the abstained bucket; verify no proposal contradicts a grimp-derived or config-authoritative classification — enrich reported no abstained cross-module edges to label, so no drafts contradicted authoritative classifications
- [x] Go — archfit self: abstained bucket is 0 (type-info covers Go) — assert enrich reports "nothing to label" rather than inventing work — enrich reported no abstained cross-module edges and drafted no labels
- [x] TypeScript — storybook: enrich respects the Wave 3 partial-coverage honesty (unresolved-specifier edges are NOT labelable — they are missing facts, not ambiguous facts; assert they are excluded from the batch) — enrich reported no abstained cross-module edges to label and did not batch unresolved-specifier gaps
- [x] determinism corpus check: with drafts pinned in a scratch copy, two analyze runs byte-identical; corpus repos left clean — yazi scratch labels produced byte-identical analyze output; corpus worktrees had only pre-existing untracked files
- [x] `make all`; PR (skipped - external PR action not automatable in this loop)

### Task 5: [Final] Documentation

- [x] `docs/guide/`: the semantic-labels workflow (enrich → review → pin → committed labels), cost/token expectations, precedence table (config-authoritative > compiler-grade > SCIP/heuristic static facts > llm label > abstain)
- [x] update findings backlog item 7; changelog entry

## Technical Details

- Precedence is the heart of the design — one table, tested: llm labels only fill cells that are `unknown` after all static sources; they never displace a static classification.
- Batch prompts include the book level definitions to anchor judgments; temperature 0; model pinned in config with the version recorded into each label for auditability.
- Cost control: enrich prints candidate counts, caps each abstained run at 100 edges, batches prompts, and relies on the LLM response cache; use provider price sheets for current token-cost estimates.

## Post-Completion

- Run the full 12-repo corpus eval once more (same harness as `reports/eval-2026-07-02-v1.1.2/`) and compare scored fractions, confidence bands, and triageability against the 2026-07-02 baseline — this closes the loop on the original goal: deterministic gate + semantic coverage.
- Consider publishing the labels workflow as the recommended AI-agent onboarding path (agent runs enrich, human reviews once, gate stays deterministic forever after).
