# archfit hybrid design: deterministic meta-linter + selective LLM (v0.1)

Status: direction draft (pending confirmation). Supersedes nothing; extends
`arch-fitness-architecture-v0.2.md` with the LLM-judgment boundary.

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

### Spike — validation gate before tranche 2

Cheap spike before designing the LLM subsystem: LLM-classify ccgram's cross-boundary
edges + subdomains, diff against the architect review's "Coupling review" section.
If it doesn't broadly match, the LLM tranche is rethought before it's planned.

### Tranche 2 — LLM, off-gate (build after spike passes)

1. **LLM provider interface + Ollama impl** — local first (free, private), then
   OpenAI/Anthropic. Thin `Classify`/`Explain` interface; cached by content hash.
2. **`enrich` command** — LLM drafts subdomain/volatility + refines model-vs-functional
   labels into `.archfit.yaml`; human reviews; gate stays deterministic.
3. **`explain` upgrade** — LLM narrative over collected evidence + ranked hubs.

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

Design is **planning-ready**. Plan Tranche 1 (deterministic: symbol-impact `risk_hub`,
`architecture_fitness` self-measure, socio-technical distance, functional-coupling
candidates, optional gitnexus provider) in full detail. Tranche 2 (LLM) is outlined
and **spike-gated** — detail it only after the classification spike passes.
