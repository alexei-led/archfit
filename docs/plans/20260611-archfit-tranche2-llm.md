# archfit Tranche 2 — off-gate LLM layer (provider, enrich, explain) + final docs

## Overview

Build the LLM layer the validation spike approved and the Tranche 1.5 gate
unblocked: a thin provider interface over official SDKs, the `enrich` command
(model-vs-functional coupling label drafts → human review → pinned labels the
deterministic gate consumes), and the `explain --llm` narrative. Then the final
documentation + skill pass over everything (this plan + the deterministic
completion plan).

Source of truth: `docs/design/hybrid-llm-strength-v0.1.md` §7 "Tranche 2
implementation design (locked 2026-06-11)".

**Hard constraint (the whole point):** `check` stays LLM-free. Enforced
STRUCTURALLY — the arch ring test forbids `internal/llm` outside `cmd`.
Approved labels are plain YAML read deterministically; draft labels are inert.

**Scope guard (spike verdicts):** coupling labels ONLY. No LLM
subdomain/volatility drafting (descoped — inter-judge ≈ chance). No frameworks —
`anthropic-sdk-go` + `openai-go` (Ollama via OpenAI-compatible base URL).

## Context (from discovery)

- Spike artifacts: `docs/plans/notes/llm-spike/{ground-truth.md,result.md}` —
  the enrich validation baseline (window_state_store functional→model,
  TelegramClient model→contract, upgrade.py→main intrusive find).
- archfit's deterministic strength is ~91% blanket-"functional" (call-edge
  heuristic) — that noise band is exactly what enrich refines.
- `classify.Run` precedence today: config globs > SCIP/extractor hint > unknown.
  Approved labels slot between globs and hints.
- `runPipeline` (cmd) is the shared evidence path; enrich reuses it.
- Existing patterns to follow: opt-in tool config (`tools.scip`), content-hash
  determinism, coverage records for absent tools, `exitError` codes.
- Check current SDK versions/APIs before coding (looking-up-docs / claude-api
  reference); pin in go.mod.

## Development Approach

- Testing approach: Regular + `moq`/httptest at the provider boundary (network
  is a system boundary — mock it; never call a live API in tests).
- Complete each task fully before the next; tests + `make lint` green each task.
- Temperature 0, fixed prompts, content-addressed cache → enrich replay is
  deterministic given the same evidence and cache.
- Secrets: API keys from env vars only; never in config, never logged.

## Testing Strategy

- Provider adapters: httptest servers asserting request shape (model, temp 0,
  system prompt) and exercising error paths (401, 429, timeout, malformed JSON).
- Cache: hit skips transport (count round-trips); key changes with
  provider/model/prompt; `--no-cache` bypasses.
- Labels: loader rejects unknown strength values; precedence table-driven
  (glob beats approved label beats hint); evidence-hash mismatch → label
  ignored + `labels/stale` advisory; DRAFT labels never consumed.
- enrich: mock provider returns canned JSON → draft file golden-compared;
  malformed LLM output → clean error, no file corruption (write temp+rename).
- Acceptance (Task 7): enrich on ccgram with a real provider, diff against the
  spike ground truth; approved-label → check precedence verified end-to-end;
  `check` double-run byte-identical with labels file present.

## Progress Tracking

- Mark completed items `[x]` immediately when done.
- New tasks: `+` prefix. Blockers: `WARN` prefix.
- Keep this file in sync; update scope if implementation deviates.

## Solution Overview

`internal/llm` (interface + 2 SDK adapters + cache) consumed only by two cmd
paths: `enrich` (batch refine → `.archfit-labels.yaml` drafts) and
`explain --llm` (narrative). `internal/labels` (model + loader + evidence hash)
is deterministic and consumed by classify — it has NO llm dependency, so the
gate reads pinned labels without touching the LLM ring.

## Technical Details

- **`internal/llm`:** `type Provider interface { Complete(ctx, Request) (Response, error) }`;
  `Request{System, User string; MaxTokens int}` (temperature pinned 0 inside
  adapters). Constructors: `NewAnthropic(model)`, `NewOpenAI(model)`,
  `NewOllama(baseURL, model)`. Missing key/unreachable host → typed error the
  commands turn into exit 3 with a doctor hint.
- **Cache:** `.archfit-cache/llm/<sha256(provider|model|system|user)>.json`
  storing the Response; wraps any Provider (decorator). Committable.
- **`internal/labels`:** `Label{FromModule, ToModule, Kind, Strength, Rationale,
EvidenceHash, Status}`; `Load(path)`; `EvidenceHash` = sha256 over the sorted
  symbol-ref pairs of that edge (from the symbol graph) so code drift invalidates.
  Classify precedence insertion + stale advisory.
- **`enrich`:** runPipeline (scip required — exit 3 with message if off) →
  select refinable edges (heuristic functional/model, not glob-decided, not
  already approved) → batch prompt with module subdomain/volatility context →
  parse strict JSON → merge into labels file as drafts (approved entries
  untouched; temp+rename write).
- **`explain --llm`:** evidence bundle (finding + classification + related
  metrics + that module's FileFact) → narrative; plain `explain` unchanged.
- **Config:** `tools.llm: {provider, model, base_url}`. `doctor` reports
  provider/key/cache status.
- **arch_test:** `internal/llm` added to a forbidden-imports list for core ring
  - engine + classify + labels.

## What Goes Where

- **Implementation Steps** (`[ ]`): all code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): Tranche-3 candidates (MCP server, GitHub
  Action, plugin protocol) stay deferred per agent-feedback design §6.

## Implementation Steps

### Task 0: cheap fixes from the three-way review comparison

**Files:** see `docs/plans/notes/three-way-review-comparison.md` §4 (deterministic/cheap list)

- [x] `classifyStrength` iterates the sorted module index (hardening; silences a recurring reviewer false-alarm)
- [x] SCIP `Resolve()` 3-state confidence (low = no indexer / medium = indexer present, resolution unimplemented / high reserved)
- [x] LOC walk emits a `diagnostic.Coverage` record (tool "loc", ok, files seen)
- [x] `initcfg` round-trip fitness test (Render → config.Load → field assertions incl. starter rules)
- [x] self-config: `tools.complexity` enabled — immediately surfaces engine.Run CCN 28 + facts.Build CCN 26 (the architect review's F-03, now self-detected). `baseline` layer reclassification DEFERRED to the post-Tranche-2 refactoring (reclassifying before the status→baseline interface inversion would create warnings with no fix available)
- [x] run tests — green before Task 1

### Task 1: labels file — deterministic half (no LLM yet)

**Files:**

- Create: `internal/labels/labels.go` (+test)
- Modify: `internal/classify/classify.go` (+test), `internal/engine/engine.go` + `cmd/archfit/main.go` (load + thread labels), `internal/arch_test.go`

- [x] Label model, YAML loader, evidence-hash computation, strict validation — DEVIATION: evidence hashes the IMPORT-GRAPH edges of the module pair (config-module namespace), not symbol refs: labels are keyed by config module names for classify, and symbol-graph keys are a different namespace; the import graph is also available on every run (no SCIP needed). Freshness checked on full runs only (delta graphs are partial and would false-stale).
- [x] classify precedence: config globs > approved labels > hint; drafts inert (table-driven test incl. directionality)
- [x] stale evidence hash → label ignored + `labels/stale` advisory (engine emits; engine test)
- [x] arch ring: labels joins core-ring guards — DEVIATION: labels parses YAML (forbidden in core ring), so it is support-tier like config/baseline; the LLM-free guarantee comes from the Task-2 ring rule (internal/llm forbidden outside cmd), which covers labels too
- [x] tests per Testing Strategy; `check` double-run byte-identical with a labels file (CLI test; malformed labels file exits 3 loudly)
- [x] run tests — green before Task 2

### Task 2: SDK deps + provider interface + adapters

**Files:**

- Modify: `go.mod`
- Create: `internal/llm/llm.go`, `internal/llm/anthropic.go`, `internal/llm/openai.go` (+tests)
- Modify: `internal/arch_test.go` (llm forbidden outside cmd)

- [ ] verify current `anthropic-sdk-go` + `openai-go` versions/APIs (docs lookup); pin
- [ ] Provider interface + adapters, temperature 0, typed errors, env-key handling
- [ ] Ollama = OpenAI adapter with base URL override
- [ ] httptest unit tests: request shape + 401/429/timeout/malformed paths
- [ ] arch test proves `internal/llm` unreachable from engine/classify/core ring
- [ ] run tests — green before Task 3

### Task 3: content-hash cache

**Files:**

- Create: `internal/llm/cache.go` (+test)

- [ ] decorator Provider; key sha256(provider|model|system|user); store under `.archfit-cache/llm/`
- [ ] hit skips transport; `--no-cache` bypass; corrupt cache entry → refetch, not crash
- [ ] run tests — green before Task 4

### Task 4: enrich command

**Files:**

- Modify: `cmd/archfit/main.go` (EnrichCmd), `internal/config/config.go` (tools.llm) (+tests)
- Create: `cmd/archfit/enrich.go` or `internal/enrich/enrich.go` (+test) — pick the seam that keeps cmd thin

- [ ] config `tools.llm` + doctor reporting (provider, key present, cache dir)
- [ ] edge selection per Technical Details; batch prompts with subdomain/volatility context
- [ ] strict JSON response parsing; drafts merged into `.archfit-labels.yaml` (approved untouched; atomic write)
- [ ] suspected-intrusive flags emitted as draft labels with rationale
- [ ] tests with mock provider incl. malformed-response path
- [ ] run tests — green before Task 5

### Task 5: explain --llm narrative

**Files:**

- Modify: `cmd/archfit/main.go` (+test)

- [ ] `--llm` flag; evidence bundle → narrative via provider+cache; offline `explain` unchanged
- [ ] graceful: no tools.llm config → exit 3 with setup hint
- [ ] tests with mock provider
- [ ] run tests — green before Task 6

### Task 6: dogfood + guard rails

**Files:**

- Modify: `.archfit.yaml`, `.gitignore` (cache dir policy decision), `internal/initcfg/initcfg.go`

- [ ] `archfit init` emits a commented `tools.llm` stanza
- [ ] decide + document cache-dir VCS policy (committable for replay; default ignore)
- [ ] full suite + lint green
- [ ] run tests — green before Task 7

### Task 7: acceptance — enrich vs spike ground truth (the gate)

**Files:**

- Create: `docs/plans/notes/tranche2-enrich-validation.md`

- [ ] PRE-REGISTER: enrich on ccgram must (a) relabel window_state_store edges functional→model, (b) NOT relabel the 3 named contracts, (c) surface the upgrade.py→main intrusive suspicion — matching `docs/plans/notes/llm-spike/ground-truth.md`
- [ ] run with a real provider; record provider/model/cache hashes; diff vs bar
- [ ] approve a label subset; verify check consumes approved only, byte-identical double-run
- [ ] write result; mark PASS/FAIL (FAIL → stop, reassess prompts before docs)

### Task 8: final documentation + skills pass (both plans' features)

**Files:**

- Modify: `README.md`, `docs/guide/{commands,configuration,configuration-reference,overview,ci,quick-start,troubleshooting}.md`
- Create: `docs/guide/llm-enrich.md`, `docs/guide/agent-feedback.md`
- Modify: `skills/archfit/SKILL.md`, `skills/archfit/references/archfit-docs.md`
- Modify: `docs/design/hybrid-llm-strength-v0.1.md` (mark Tranche 2 implemented)

- [ ] document: enrich workflow (draft→review→approve), labels file, tools.llm, cache; agent_tasks block; SARIF; change_locality; updated metric list (13)
- [ ] SKILL.md teaches an agent the loop: run check → read agent_tasks/SARIF → fix → re-check; enrich/explain as human-in-the-loop extras
- [ ] kill remaining stale references (docs/output.md link etc. if not fixed earlier)
- [ ] full suite + lint; move BOTH 20260611 plans to `docs/plans/completed/`

## Post-Completion

Tranche-3 candidates (MCP server, GitHub Action wrapper, plugin protocol,
subdomain-suggestion experiment) remain deferred — revisit only after the JSON
contract has survived a release cycle of real agent use.
