# archfit hybrid design: deterministic meta-linter + selective LLM (v0.1)

Status: Tranche 1 implemented (deterministic). Validation spike **RAN 2026-06-09** —
**split verdict**: LLM coupling refinement validated (build); LLM subdomain/volatility
drafting descoped. Tranche 2 revised accordingly in §7. Full record:
`docs/plans/notes/llm-spike/result.md`.
Supersedes nothing; extends `arch-fitness-architecture-v0.2.md` with the LLM-judgment boundary.

## 1. Goal

Make archfit a **mixture**:

1. a deterministic meta-linter that drives language-specific tools and external
   architecture linters, and
2. a **selective** LLM layer used **only** where human-like classification or
   judgment is needed and deterministic tools cannot decide.

The LLM consumes data archfit already collected, does **focused** classification
/ judgment / explanation, and never re-derives what tools can compute. Providers:
OpenAI, Anthropic, Ollama (local).

## 2. The load-bearing rule: LLM off the gate

archfit's value is a **deterministic, reproducible verdict** that gates CI via
baseline/delta. An LLM in the verdict path makes two runs differ and breaks the
gate. Therefore:

- **`check` (the gate) is pure deterministic.** It consumes reviewed, pinned
  config and computed metrics. No LLM call. Same commit + config → identical
  verdict.
- **LLM lives in two off-gate places only:**
  - **`init` / `enrich`** — the LLM _drafts config_ (subdomain, volatility,
    public/internal API globs, module map hints). A human reviews it; it lands
    in `.archfit.yaml` under version control. This is the same review step users
    already do (the generated config literally says
    `# TODO: review and promote gate: warn to gate: fail`).
  - **`explain`** — read-time narrative for humans/agents. Never gates.

This resolves the apparent conflict in the vision (LLM judgment **and** a CI
gate): the LLM runs once, output is reviewed and committed, the engine gates on
the pinned result.

Corollary: **LLM results are cached**, keyed by content hash, refreshed only on
`init`/`enrich`/explicit request. No per-CI-run LLM calls.

## 3. Can integration strength be measured? (Khononov)

Khononov is explicit: integration strength **categorizes** types of shared
knowledge; it is not a quantitative measurement. The four levels are
**unequally** automatable:

| Strength level        | Signal                                                                                                                      | Deterministic?                                                                                                                                                                   | Who decides                           |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| **Intrusive** (worst) | crosses to private/internal symbol: `/internal/` (Go), `_name`/no `__all__` (Py), non-exported, reflection, shared DB table | **Yes** — visibility is structural (SCIP/LSP + globs). Already done.                                                                                                             | tools                                 |
| **Contract** (best)   | edge depends on an interface/Protocol/abstract type / declared public API (Dependency Inversion)                            | **Mostly** — abstraction-vs-concretion is type-resolvable (SCIP/LSP). Mostly done.                                                                                               | tools                                 |
| **Model**             | edge passes a shared domain type (stamp coupling)                                                                           | **Signal yes** (is the crossed type a shared entity vs a DTO?), **label no** ("domain model vs transfer shape" is semantic)                                                      | tools propose → LLM labels            |
| **Functional**        | two modules encode the **same business rule**                                                                               | **Different pipeline.** Often _implicit_ — **no import edge exists**. Static graph can't see it. Signal = clone detection (jscpd/PMD-CPD) + co-change mining. Label is semantic. | tools surface candidates → LLM labels |

Key consequence: **functional coupling is not an edge attribute.** You cannot
model all four levels as "a property of an import edge", because functional
coupling lives where there is no edge. It needs a separate candidate pipeline
(duplicate-logic + co-change), then semantic labeling.

**Volatility** is the cleanest LLM case. Khononov says volatility must be judged
from the **business domain** (core / supporting / generic subdomain), **not commit
history** — churn is "accidental volatility" that bad design itself causes.
archfit currently derives volatility from churn (a proxy) and already supports a
hand-authored `subdomain`/`volatility` config field. The LLM should _draft_ the
subdomain classification at `init`; the human confirms; the gate uses the pinned
value.

Verdict: **intrusive + contract are deterministic (largely built). Model label,
functional coupling, and volatility/subdomain are the genuine LLM cases.** The
fully-tooled review comparison (below) isolates exactly this remainder.

**Correction after code inspection (`internal/extract/scip/scip_reader.py`):**
archfit _already_ classifies all four levels deterministically at the **symbol**
level via SCIP — `_is_private` → intrusive, Protocol/ABC or internal abstract base
→ contract, type-reference → model, call-reference → functional. So the LLM does
**not** classify strength from scratch. Its role narrows to (a) **refining the
model-vs-functional and contract heuristics** where the structural proxy is weak
(a call into a shared module may be model coupling, not functional; a type may be
a transfer DTO, not a domain model), (b) **subdomain/volatility judgment**, and
(c) **explicit functional coupling with no edge** (the clone+co-change pipeline).
This makes the LLM tranche _smaller and better-grounded_ than first stated — it
corrects a heuristic, it doesn't replace a missing capability.

## 4. Review-only comparison, fully tooled

Re-scoped to **review** (drop design/plan). archfit allowed its full + advanced
toolset (SCIP, git churn, co-change, and — if integrated — gitnexus symbol impact).

When archfit is fully tooled, the **remaining** gap vs the architect LLM review
is precisely the semantic layer:

| Capability                                | archfit (fully tooled, deterministic)           | architect (LLM review)        |
| ----------------------------------------- | ----------------------------------------------- | ----------------------------- |
| god-modules / LOC skew                    | yes (`structural_weight`)                       | yes — **agree**               |
| cycles                                    | yes (whole-graph, incl. deferred imports)       | yes (top-level only)          |
| static blast radius                       | yes (`blast_radius`, module-level)              | yes                           |
| churn / co-change hotspots                | yes (`change_amplification`, `hidden_coupling`) | yes                           |
| intrusive/contract edge strength          | yes (SCIP + globs)                              | yes                           |
| **symbol-level impact ranking**           | **gap → §5 deterministic upgrade**              | yes (gitnexus)                |
| **model-vs-functional labeling**          | no                                              | yes — **needs LLM**           |
| **subdomain/volatility judgment**         | churn proxy only                                | yes — **needs LLM**           |
| **priority + cascading-change narrative** | no                                              | yes — **needs LLM (explain)** |

So the LLM's job is the bottom three rows. Everything above stays deterministic.

## 5. "Which dependency will hurt" — split cleanly

The architect review's #1 finding (`window_state_store`: 42 direct deps, 172
impacted symbols, CRITICAL) is something archfit **missed**. Root cause is **graph
granularity**, not missing LLM: archfit aggregates files into coarse config-modules
and counts _module-level_ imports, so a symbol-level hub disappears.

- **Deterministic upgrade (no LLM) — the `risk_hub` metric:** rank modules by
  **cross-module surface-breadth × volatility** (explicit-subdomain volatility only;
  see churn-double-count fix in §7). Surface-breadth = the count of a module's own
  symbols referenced by at least one symbol from a _different_ module — how wide a
  module's externally-coupled surface is.
  **Implementation:** `scip_reader.py` was extended to emit per-symbol fan-in +
  cross-module symbol refs (the occurrence walk and internal-symbol filtering already
  existed); Go builds a `symbol.Graph` and the metric counts externally-referenced
  symbols per module.
  **Why surface-breadth, not transitive reverse-reachability** (the original Task 5
  design): validated on 4 repos (see `docs/plans/notes/risk_hub-validation.md`).
  Transitive reachability (a) just reproduces `blast_radius`, (b) is polluted by
  artifacts (`__init__` re-exports, barrel symbols, test symbols) because SCIP refs
  are **doc-scoped** — indexers don't populate `enclosing_range`, and (c) failed the
  gold standard (didn't surface ccgram `window_state_store`). Surface-breadth is
  distinct from blast_radius, clean, language-consistent (Go/TS/Py), and ranks
  `window_state_store` #1 — matching the architect review's #1 critical finding.
  **Coverage caveat:** needs a SCIP indexer + `uv` + `tools.scip.enabled: on`.
  No SCIP → metric is `n/a` (info), never a false zero.
- **LLM (explain only):** "_why_ does this hub hurt, what is the cascading-change
  scenario, what is the smallest fix" — the F3 narrative. Consumes the ranked
  deterministic output; adds no numbers.

Do not blur these: reaching for the LLM to fix a graph-granularity problem is the
wrong tool.

## 6. Provider abstraction (thin)

A small Go interface; concrete impls for the three providers. Do not over-spec.

```go
type Classifier interface {
    // Label semantic cases tools can't decide, given collected evidence.
    Classify(ctx, ClassifyRequest) (ClassifyResult, error) // model-vs-functional, subdomain
}
type Explainer interface {
    Explain(ctx, ExplainRequest) (string, error) // narrative for explain/enrich
}
```

- Impls: `openai`, `anthropic`, `ollama` (local). Config under `tools.llm`.
- Requests carry **collected data only** (edges, symbols, churn, candidates), not
  raw repo dumps → focused, cheap, reproducible prompts.
- Results cached by content hash; off the `check` hot path entirely.
- Prior art for _what good Khononov LLM judgment looks like_: the installed
  `modularity:review` skill. Reference it; don't reinvent the rubric.

**Library decision (researched via perplexity, 2026):** do **not** adopt a
multi-LLM framework — implement the thin interface above over **official SDKs**.

- **`openai-go`** (official) covers OpenAI **and Ollama** — Ollama exposes an
  OpenAI-compatible `/v1` endpoint, so the same client targets both via a base-URL
  swap. **`anthropic-sdk-go`** (official) covers Claude. Net dependency surface:
  ~2 SDKs + stdlib. Each provider lives in one file (`openai_provider.go`,
  `anthropic_provider.go`, `ollama_provider.go` — the last may just set `openai-go`'s
  base URL, or use plain `net/http` to `localhost:11434`).
- Rejected: **any-llm-go** (no first-class Anthropic/Ollama in 2026 — you write glue
  anyway), **gollm** (indie, not maintained at provider pace; JSON-mode lags),
  **langchaingo** (full agent/chain framework — overkill, heaviest deps; violates the
  minimal-deps stack decision).
- All three needs (JSON/structured output, tool use, streaming) are first-class in
  the official OpenAI + Anthropic SDKs; for Ollama, prompt-enforced JSON + strict
  unmarshal is robust for the small fixed classification schemas here.
- Caveat: the 2026 maintenance claims above are partly inferred — re-verify SDK
  status and import paths when Tranche 2 is built.

## 7. Decisions (confirmed) + sequence

Confirmed direction:

- **A — LLM off the gate, config-drafting pattern.** `check` never calls an LLM.
  LLM drafts config at `init`/`enrich` (human reviews, commits); gate runs on
  pinned config. Reproducible verdict is non-negotiable.
- **B — deterministic symbol-impact ranking is built first**, before any LLM.
- **C — own SCIP-based symbol impact is the deterministic core; gitnexus is an
  optional richer provider** when present (mirrors the existing SCIP opt-in
  pattern: `tools.gitnexus.enabled: on`, never auto).

Two tranches. **Deterministic now** (1–3, validated against real reviews);
**LLM after a validation spike** (the spike gates whether 4–6 are built as designed).

### Tranche 1 — deterministic (build now)

1. **Symbol-level impact ranking** (§5) — extend `scip_reader.py` to emit per-symbol
   fan-in + symbol-reference edges; build the symbol graph in Go; reverse-reachability;
   new metric (likely `risk_hub`, report-only/info first, optionally delta-gating later).
   **`risk_hub = symbol_impact × volatility` — NOT × churn.** Reason: with no
   hand-authored subdomains, `volatility` is itself derived from churn
   (`config/volatility.go`: churn ≥ 2/3 max → high), so `× churn × volatility`
   bootstraps to `symbol_impact × churn²` — a noisier duplicate of
   `change_amplification` (already `blast × churn`). Use **only subdomain-derived
   volatility** in `risk_hub`; until `enrich` (step 5) populates subdomains, treat
   volatility as neutral (1.0) so the metric is pure symbol-impact. This keeps
   `risk_hub` (symbol fan-in) and `change_amplification` (module churn) distinct.
   **Acceptance gate: reproduce known hubs across all 4 validation repos**
   (ccgram→window_state_store, plus codegraph/pumba/spotinfo), not ccgram alone —
   guard against overfitting to one data point.
2. **gitnexus optional provider** — when `tools.gitnexus.enabled: on`, enrich symbol
   impact with its graph; otherwise SCIP-only. Never on the auto path.
3. **Functional-coupling candidate pipeline** — clone detection + co-change into a
   candidate list (deterministic signal, no semantic label yet).

### Spike — validation gate (RAN 2026-06-09, ccgram-only)

Ran as a pre-registered, blind classification: ground-truth + thresholds frozen **before**
classifying; two independent blind subagents (structural-blind = src + archfit JSON only,
then framing-allowed = + README/architecture.md, firewalled by-document from every
review/modularity doc; transcript-verified zero leakage). ccgram is the only repo with an
architect review, so the spike is ccgram-only. Record:
`docs/plans/notes/llm-spike/{ground-truth.md,result.md,ccgram-evidence.json}`.

The design's gate rule is **per-capability** (broadly-matches → build; else → rethink), and
the two LLM jobs land on opposite sides:

- **Coupling model-vs-functional refinement — VALIDATED → build.** archfit's deterministic
  strength is ~91% noise (419/461 edges blanket-labeled "functional" because a call-edge maps
  to "functional"). Both blind agents corrected it where they had cross-module visibility
  (`window_state_store` functional→model, `TelegramClient` model→contract; 0/3 named contracts
  misflagged) and surfaced a real **intrusive** coupling (`upgrade.py → main._restart_requested`)
  that the architect review itself missed. Genuine value-add.
- **Subdomain / volatility drafting — DOES NOT MATCH → descope.** 50% firm on a 3-way split
  (chance 33%), **framing-invariant** (Stage 2 with the business docs did not move it), and the
  pre-designated Telegram core-vs-generic discriminator failed both stages. Deeper finding: two
  capable judges disagree ~50%, so subdomain-derived volatility is unreliable **regardless of
  producer** — a human reviewer relocates the coin-flip, it does not fix it.
- **Granularity gap (deterministic; blocks the coupling layer).** Both agents missed the
  intra-`handlers` hubs (`polling_state` mutable singleton, `directory_callbacks` low cohesion)
  even with a fan-out-aware prompt, because archfit's coarse config-modules hide them. The
  evidence package needs per-file / intra-module signal first.

### Tranche 1.5 — deterministic structural-facts block (IMPLEMENTED 2026-06-11; gate PASSED)

**Status: implemented and accepted.** The facts block ships as `Diagnostic.FileFacts`
(`file_facts` in JSON, a neutral "Structural facts" markdown section), assembled by
`internal/facts.Build` inside `engine.Run` from the symbol graph + change history.
File-level joins (LOC, co-change) are exact via the new `symbol.Graph.Path`
(per-symbol defining file, parsed from the reader's existing `path` field — no
`scip_reader.py` change). The acceptance spike re-run **PASSED**
(`docs/plans/notes/structural-facts-spike-rerun.md`): a firewalled blind classifier
ranked `polling_state` #2 (mutable shared-state) and `directory_callbacks` #3
(low-cohesion grab-bag) of 383 ccgram modules, and cleared the benign high-scorers —
without gitnexus. **Tranche 2 is unblocked.**

Goal: make the intra-module hubs the spike's blind classifier missed (`polling_state`,
`directory_callbacks`) visible to the Tranche-2 LLM. Plan + evidence:
`docs/plans/20260610-archfit-tranche1.5-structural-facts.md`,
`docs/plans/notes/intra-module-hub-validation.md`, and the signal probe notes.

**A first attempt (two ranking metrics — `cohesion_spread`, `shared_state_hub`) FAILED its
4-repo gate**, and a follow-up signal probe on ccgram settled why and what works:

- **Deterministic code cannot RANK these hubs by risk.** Separating `config` (benign,
  read-only) from `polling_state` (risky, mutable), or `bootstrap` (wiring) from
  `directory_callbacks` (grab-bag), needs mutability/intent — the LLM's job, not a metric's.
- **`polling_state`'s SCIP symbol fan-in is too flat** (`PaneStatusStrategy` peaks at 2;
  file max 8 — noise floor). What surfaces it: SCIP **module-level inbound fan-in** (23
  importing modules) shows it is a hub; **GitNexus blast-radius** (13 direct / 41 transitive)
  carries the depth that is the real danger. No SCIP reader change recovers this.
- **`directory_callbacks` cannot be measured by SCIP LCOM** (Python SCIP omits
  `enclosing_range`, so a file's symbols collapse to one cohesion component). What surfaces
  it: **outbound distinct-destination count × LOC** (46 destinations) — i.e. the original
  `cohesion_spread` idea without its subsystem-collapse.

**Resulting design (validated by the probe): emit a per-file STRUCTURAL-FACTS block, not
metrics.** From already-collected data — **no `scip_reader.py` change** — assemble per file:
inbound module fan-in, outbound distinct-destination count, LOC, co-change partners, and
GitNexus blast-radius when the optional provider is on. Emitted as neutral evidence (JSON for
the LLM + compact markdown); **no band, no score, never on `check`.** The two failed ranking
metrics are removed (`shared_state_hub`) or folded into the facts block (`cohesion_spread`'s
outbound computation, at raw-destination granularity). **Acceptance is the spike re-run** (the
blind classifier must now surface both hubs from the enriched evidence), not a metric rank.

### Tranche 2 — LLM, off-gate (IMPLEMENTED 2026-06-11; acceptance PASS)

**Status: implemented and accepted** (`docs/plans/notes/tranche2-enrich-validation.md`).
Shipped: `internal/llm` provider layer (anthropic/openai/ollama, content-hash
cache), `internal/labels` pinned-label model with import-graph evidence
hashing, classify precedence (globs > approved labels > hint), `labels/stale`
advisories, `archfit enrich` (draft→review→pin), `explain --llm`. The
LLM-off-gate constraint is structurally enforced: the arch ring test forbids
any internal package from importing `internal/llm`.

1. **LLM provider interface** — thin `Classify`/`Explain` over official SDKs (§6); cached by
   content hash; never on `check`.
2. **`enrich` (coupling labels only)** — LLM drafts model-vs-functional strength refinements for
   cross-module edges; a human reviews and commits them; the gate runs on the pinned labels
   (the same draft → review → pin pattern, applied to coupling, not subdomain). Consumes
   human-authored subdomain/volatility as **context** to avoid over-flagging intended
   centralization (the spike saw the LLM over-flag generic hubs: `tmux`, bootstrap, `config`).
3. **`explain` upgrade** — LLM narrative over collected evidence + ranked hubs.

#### Tranche 2 implementation design (locked 2026-06-11)

Plan: `docs/plans/20260611-archfit-tranche2-llm.md`.

- **Package layout:** new `internal/llm` (provider interface + adapters + cache).
  The interface is one method deep: `Complete(ctx, Request) (Response, error)`
  where Request carries system+user content and a deterministic temperature=0
  config. `Classify`/`Explain` are archfit-side functions that BUILD prompts and
  PARSE responses — not provider methods. Adapters: `anthropic` (anthropic-sdk-go),
  `openai` (openai-go), `ollama` (openai-go with custom base URL). Verify current
  SDK versions at build time.
- **Config:** `tools.llm: {provider: anthropic|openai|ollama, model: <id>,
base_url: <ollama only>}`. API keys come from standard env vars ONLY
  (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) — never from `.archfit.yaml`.
- **Cache:** content-addressed at `.archfit-cache/llm/<sha256(provider|model|prompt)>.json`.
  Cache hits skip the network entirely. The cache directory is committable: a
  committed cache makes `enrich` replay byte-identical across machines (same
  draft from the same evidence). `--no-cache` forces refresh.
- **Pinned labels:** `enrich` writes `.archfit-labels.yaml` — one entry per
  cross-module edge the LLM relabeled:
  `{from_module, to_module, kind, strength, rationale, evidence_hash, status: draft}`.
  A human reviews and flips `status: approved` (or deletes). `classify.Run` reads
  the labels file deterministically with precedence
  **config public/internal globs > approved labels > SCIP/extractor hint > unknown** —
  draft labels are never consumed by the gate. `evidence_hash` is a content hash
  of the edge's symbol-reference evidence; on mismatch (the code changed since
  labeling) the label is ignored and a `labels/stale` advisory is emitted.
  This keeps `check` LLM-free AND label-fresh.
- **Structural guarantee (not discipline):** `internal/arch_test.go` forbids
  `internal/llm` imports from the core ring, `internal/engine`, and
  `internal/classify` — only `cmd` (enrich/explain) may import it. The
  LLM-off-gate constraint becomes compiler/CI-enforced, per
  [[prefer-structural-over-discipline]].
- **`enrich` scope guard:** only edges whose deterministic strength came from the
  blanket call-edge heuristic (functional/model, not config-glob, not approved
  label) are sent for refinement; batched with module subdomain/volatility
  context. Suspected-intrusive flags from the LLM are emitted as draft labels
  too (the spike found a real one the architect review missed).
- **`explain --llm`:** narrative over the finding's full evidence (edge,
  classification, metrics, structural facts). Plain `explain` stays
  deterministic and offline.

**DROPPED by the spike:** LLM-drafted subdomain/volatility into `.archfit.yaml` (the original
`enrich` subdomain purpose). Subdomain/volatility stays a **human-authored** config field;
`risk_hub` keeps volatility neutral (1.0) absent explicit config. A thin, clearly-labeled
subdomain **suggestion** experiment may come later, but is never weighted into a metric.

### Feature inventory (resolved — scope of "ALL missing")

Confirmed 2026-06-09:

- **Language scope: all three (Go/TS/Py).** New metrics target every language with a
  SCIP indexer; validate per-language. Clone-detection tooling maturity differs —
  handle per-language, `n/a` where absent.
- **`architecture_fitness` self-measure: IN.** New deterministic metric — detect
  whether arch intent is enforced (presence of arch tests / import-linter / archfit
  in CI config). Closes archfit's own blind spot; mirrors the architect rubric.
- **Socio-technical distance: IN.** CODEOWNERS (git-author overlap as fallback) refines
  BC distance — different owners → higher effective distance. `n/a` when no ownership
  data; never fabricate ownership.
- **gitnexus: optional symbol-impact provider only.** Co-change stays archfit's own
  git-log pass. Narrowest dependency surface; opt-in, never auto.

### Status

Tranche 1 is **implemented**. All five deterministic features are built, tested, and
merged: `risk_hub` (SCIP-based symbol surface-breadth), `architecture_fitness`
(enforcement signal detector), socio-technical distance (ownership resolver),
`functional_candidates` (clone + co-change candidate pipeline), and optional gitnexus
provider. All new metrics are info-band (report-only); `check` remains LLM-free.

The validation spike **RAN 2026-06-09** (ccgram-only) with a **split verdict** (see §7 Spike
and `docs/plans/notes/llm-spike/result.md`): LLM coupling model-vs-functional refinement is
validated (build, off-gate); LLM subdomain/volatility drafting is descoped (inter-judge
agreement ≈ chance). Next is **Tranche 1.5** (deterministic intra-module cohesion signal),
which gates the LLM coupling layer; then the revised Tranche 2 (coupling `enrich` + `explain`).
